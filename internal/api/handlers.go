package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"bloom-du/internal/bloom"
	"bloom-du/internal/build"
	"bloom-du/internal/utils"
)

const (
	ContentType     = "Content-Type"
	ContentTypeJSON = "application/json; charset=utf-8"
	MsgJSONError    = "JSON encode error"
	valueMinLen     = 2
)

const (
	searchMsg    = "✅ Время поиска"
	searchAddMsg = "✅ Время поиска + добавления"
)

var Filter *bloom.StableBloomFilter
var isReady bool

type RequestData struct {
	Value   string `json:"value"`
	Options string `json:"options"`
}

type RequestBulkData struct {
	Data []string `json:"data"`
}

type ResponseData struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

func init() {
	isReady = false
}

func Start() {
	logCh := make(chan utils.LogEvent, 10)
	go handleLogs(logCh)

	sourceFile := viper.GetString("source")
	force := viper.GetBool("force")
	checkpointPath := viper.GetString("checkpoint_path")
	Filter = bloom.NewStableBloomFilter(
		sourceFile,
		force,
		logCh,
		checkpointPath,
	)
	isReady = true

	CurrentConfig.WithLabelValues(
		fmt.Sprintf("%d", Filter.SBF.Cells()),
		fmt.Sprintf("%d", Filter.SBF.K()),
		fmt.Sprintf("%f", Filter.SBF.FalsePositiveRate()),
		fmt.Sprintf("%f", Filter.SBF.StablePoint()),
		build.Version,
	).Set(1)
}

func Checkpoint() {
	start := time.Now()
	if isReady && Filter.Checkpoint() {
		utils.StopWatchLog(Filter.LogCh, start, "📍 Checkpoint done "+utils.HumByte(Filter.GetDumpSize()))
	}
}

// Эксперименты с каналами
func handleLogs(logCh chan utils.LogEvent) {
	for msg := range logCh {
		switch msg.Name {
		case "bootstrap":
			if msg.Count != 0 {
				Elements.WithLabelValues("add_after_test").Add(msg.Count)
			}
			log.WithLevel(msg.Level).Msgf("[Bootstrap] %s", msg.Msg)
		case "api":
			log.WithLevel(msg.Level).Msgf("[API] %s", msg.Msg)
		case "add":
			Elements.WithLabelValues("add_after_test").Add(msg.Count)
		default:
			log.WithLevel(msg.Level).Msgf("[%s]%s", msg.Name, msg.Msg)
		}
	}
}

// TODO почему-то всё равно летят ошибки, если пытаться слать запросы к API
// до того, как фильтр будет готов их принимать
func checkIsReady(w http.ResponseWriter) error {
	if !isReady {
		msg := "Filter is not ready now, please wait"
		httpRespond(w, http.StatusTooEarly, msg)
		return errors.New("FAIL")
	}

	return nil
}

func decodeInputJSON(w http.ResponseWriter, r *http.Request) RequestData {
	var data RequestData
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	return data
}

func handleFastCheck(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if r.Method != http.MethodHead {
		httpRespond(w, http.StatusMethodNotAllowed, "")
	}

	value := r.URL.Query().Get("value")
	result := Filter.Test(value)
	msg := "Absolutely NOT exist!"
	status := http.StatusNotFound
	if result {
		msg = "May be exist!"
		status = http.StatusOK
	}

	utils.StopWatchLog(Filter.LogCh, start, searchMsg)

	httpRespond(w, status, msg)
}

func handleCheck(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	err := checkIsReady(w)
	if err != nil {
		return
	}

	value := decodeInputJSON(w, r).Value

	err = queryValidate(w, value)
	if err != nil {
		return
	}

	result := Filter.Test(value)
	msg := "Absolutely NOT exist!"
	status := http.StatusNotFound
	if result {
		msg = "May be exist!"
		status = http.StatusOK
	}

	utils.StopWatchLog(Filter.LogCh, start, searchMsg)

	httpRespond(w, status, msg)
}

func handleAdd(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	value := decodeInputJSON(w, r).Value

	err := queryValidate(w, value)
	if err != nil {
		return
	}

	if Filter.TestAndAdd(value) {
		utils.StopWatchLog(Filter.LogCh, start, searchAddMsg)
		httpRespond(w, http.StatusCreated, "✅ Добавлено!")
	} else {
		utils.StopWatchLog(Filter.LogCh, start, searchMsg)
		httpNotModified(w)
	}
}

func handleBulkLoad(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var bulk RequestBulkData
	err := json.NewDecoder(r.Body).Decode(&bulk)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	added := 0
	for _, entity := range bulk.Data {
		if Filter.TestAndAdd(entity) {
			added++
		}
	}
	skipped := len(bulk.Data) - added
	msg := fmt.Sprintf("[bulk] ✅ Добавлено: %d, Пропущено: %d", added, skipped)
	utils.StopWatchLog(Filter.LogCh, start, msg)

	if added == 0 {
		httpNotModified(w)
	} else {
		httpRespond(w, http.StatusCreated, msg)
	}
}

func handleCheckpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		Checkpoint()
		httpRespond(w, http.StatusCreated, "Success!")
	}
}

func queryValidate(w http.ResponseWriter, value string) error {
	if len(value) <= valueMinLen {
		httpRespond(w, http.StatusBadRequest, fmt.Sprintf("value lenght must be >= %d", valueMinLen))
		return errors.New("FAIL")
	}
	return nil
}

func httpRespond(w http.ResponseWriter, statusCode int, msg string) {
	w.Header().Set(ContentType, ContentTypeJSON)
	w.WriteHeader(statusCode)

	jsonResponse := ResponseData{
		Message: msg,
		Status:  statusCode,
	}

	if err := json.NewEncoder(w).Encode(&jsonResponse); err != nil {
		log.Error().Msgf("%s: %v", MsgJSONError, err)
	}
}

func httpNotModified(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotModified)
}
