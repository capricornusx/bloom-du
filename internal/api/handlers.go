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
	"bloom-du/internal/utils"
)

const (
	ContentType     = "Content-Type"
	ContentTypeJSON = "application/json; charset=utf-8"
	MsgJSONError    = "JSON encode error"
	valueMinLen     = 5
)

var Filter *bloom.StableBloomFilter

type Response struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

func Start() {
	sourceFile := viper.GetString("source")
	Filter = bloom.CreateFilter(sourceFile)
	FilterProperty.WithLabelValues("size").Set(float64(Filter.SBF.Cells()))
	CurrentConfig.WithLabelValues(
		fmt.Sprintf("%d", Filter.SBF.Cells()),
		fmt.Sprintf("%d", Filter.SBF.K()),
		fmt.Sprintf("%f", Filter.SBF.FalsePositiveRate()),
		"manual",
	).Set(1)
}

func Checkpoint() {
	start := time.Now()
	Filter.Checkpoint()
	elapsed := utils.StopWatchLog(start, "Checkpoint done")
	QueryDuration.WithLabelValues("Checkpoint").Observe(elapsed)
}

func handleCheck(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	value := r.URL.Query().Get("value")

	err := queryValidate(w, value)
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

	elapsed := utils.StopWatchLog(start, "Время поиска")
	QueryDuration.WithLabelValues("check").Observe(elapsed)

	httpRespond(w, status, msg)
}

func handleAdd(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	value := r.URL.Query().Get("value")

	err := queryValidate(w, value)
	if err != nil {
		return
	}

	if !Filter.TestAndAdd(value) {
		Elements.WithLabelValues("add_after_test").Inc()
		elapsed := utils.StopWatchLog(start, "✅ Время поиска + добавления")
		QueryDuration.WithLabelValues("add_after_test").Observe(elapsed)
		FilterProperty.WithLabelValues("stable_point").Set(Filter.SBF.StablePoint())
		Filter.PrintLogStat()
		httpRespond(w, http.StatusCreated, "✅ Success!")
	} else {
		elapsed := utils.StopWatchLog(start, "✅ Время поиска")
		QueryDuration.WithLabelValues("add_false_test").Observe(elapsed)
		notModified(w)
	}
}

func handleCheckpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		Checkpoint()
		httpRespond(w, http.StatusCreated, "Success!")
	}
}

func queryValidate(w http.ResponseWriter, query string) error {
	if len(query) <= valueMinLen {
		httpRespond(w, http.StatusBadRequest, fmt.Sprintf("value bust be >= %d", valueMinLen))
		return errors.New("FAIL")
	}
	return nil
}

func httpRespond(w http.ResponseWriter, statusCode int, value string) {
	w.Header().Set(ContentType, ContentTypeJSON)
	w.WriteHeader(statusCode)

	jsonResp := Response{
		Message: value,
		Status:  statusCode,
	}

	if err := json.NewEncoder(w).Encode(jsonResp); err != nil {
		log.Printf("%s %v", MsgJSONError, err)
	}
}

func notModified(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotModified)
	_, _ = w.Write([]byte("ok"))
}
