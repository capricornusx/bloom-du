package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dustin/go-humanize"
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

var Filter *bloom.StableBloomFilter

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

func Start() {
	sourceFile := viper.GetString("source")
	force := viper.GetBool("force")
	Filter = bloom.CreateFilter(sourceFile, force)

	FilterProperty.WithLabelValues("cells").Set(float64(Filter.SBF.Cells()))
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
	if Filter.Checkpoint() {
		elapsed := utils.StopWatchLog(start, "Checkpoint done: "+humanize.Bytes(Filter.GetDumpSize()))
		QueryDuration.WithLabelValues("Checkpoint").Observe(elapsed)
	}
}

func checkIsReady(w http.ResponseWriter) error {
	if !Filter.IsReady() {
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

func handleCheck(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	value := decodeInputJSON(w, r).Value

	err := queryValidate(w, value)
	if err != nil {
		return
	}
	err = checkIsReady(w)
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

	elapsed := utils.StopWatchLog(start, " ✅ Время поиска")
	QueryDuration.WithLabelValues("check").Observe(elapsed)

	httpRespond(w, status, msg)
}

func handleAdd(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	value := decodeInputJSON(w, r).Value

	err := queryValidate(w, value)
	if err != nil {
		return
	}

	err = checkIsReady(w)
	if err != nil {
		return
	}

	if !Filter.TestAndAdd(value) {
		elapsed := utils.StopWatchLog(start, "✅ Время поиска + добавления")
		QueryDuration.WithLabelValues("add_after_test").Observe(elapsed)
		Elements.WithLabelValues("add_after_test").Inc()
		FilterProperty.WithLabelValues("stable_point").Set(Filter.SBF.StablePoint())
		FilterProperty.WithLabelValues("cells").Set(float64(Filter.SBF.Cells()))
		FilterProperty.WithLabelValues("fpr").Set(Filter.SBF.FalsePositiveRate())
		Filter.PrintLogStat()
		httpRespond(w, http.StatusCreated, "✅ Добавлено!")
	} else {
		elapsed := utils.StopWatchLog(start, "✅ Время поиска")
		QueryDuration.WithLabelValues("add_false_test").Observe(elapsed)
		httpNotModified(w)
	}
}

func handleBulkLoad(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	err := checkIsReady(w)
	if err != nil {
		return
	}

	var bulk RequestBulkData
	err = json.NewDecoder(r.Body).Decode(&bulk)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	added := 0
	for _, entity := range bulk.Data {
		if !Filter.TestAndAdd(entity) {
			added++
		}
	}
	elapsed := utils.StopWatchLog(start, "✅ [bulk] Время поиска + добавления")
	QueryDuration.WithLabelValues("add_bulk").Observe(elapsed)
	FilterProperty.WithLabelValues("stable_point").Set(Filter.SBF.StablePoint())
	FilterProperty.WithLabelValues("cells").Set(float64(Filter.SBF.Cells()))
	FilterProperty.WithLabelValues("fpr").Set(Filter.SBF.FalsePositiveRate())
	Filter.PrintLogStat()

	skipped := len(bulk.Data) - added
	msg := fmt.Sprintf("✅ [bulk] Добавлено: %d, пропущено: %d", added, skipped)
	log.Debug().Msg(msg)

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

func httpRespond(w http.ResponseWriter, statusCode int, value string) {
	w.Header().Set(ContentType, ContentTypeJSON)
	w.WriteHeader(statusCode)

	jsonResp := ResponseData{
		Message: value,
		Status:  statusCode,
	}

	if err := json.NewEncoder(w).Encode(jsonResp); err != nil {
		log.Printf("%s %v", MsgJSONError, err)
	}
}

func httpNotModified(w http.ResponseWriter) {
	w.Header().Set(ContentType, ContentTypeJSON)
	w.WriteHeader(http.StatusNotModified)
	_, _ = w.Write([]byte(""))
}
