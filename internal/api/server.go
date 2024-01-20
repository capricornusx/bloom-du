package api

import (
	"errors"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	metricsNamespace   = "bloom_du"
	defaultTimeout     = 5 * time.Second
	defaultIdleTimeout = 2 * defaultTimeout
	labelQuery         = "type"
)

var apiHandlersFunc = map[string]http.HandlerFunc{
	"/api/check":      handleCheck,
	"/api/add":        handleAdd,
	"/api/bulk":       handleBulkLoad,
	"/api/checkpoint": handleCheckpoint,
	"/health":         healthHandler,
}

var labelNames = []string{labelQuery}

// todo сделать массивы (counters), (gauges).. и потом их добавлять циклом, видел в какой-то репе
var (
	CurrentConfig = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "config_info",
			Help:      "config",
		}, []string{"cells", "k", "fpRate", "stablePoint", "build"},
	)
	Elements = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "elements_total",
			Help:      "Добавлено элементов",
		}, labelNames,
	)
	FilterProperty = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "filter_properties",
			Help:      "filter properties",
		}, labelNames,
	)
	QueryDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  metricsNamespace,
			Subsystem:  "storage",
			Name:       "query_duration_seconds",
			Help:       "A Summary of successful query durations in seconds.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}, labelNames)
)

// initMetrics Prometheus metrics
func initMetrics() {
	prometheus.MustRegister(CurrentConfig)
	prometheus.MustRegister(Elements)
	prometheus.MustRegister(QueryDuration)
	prometheus.MustRegister(FilterProperty)
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := w.Write([]byte("OK"))
	if err != nil {
		return
	}
}

func getMux() *http.ServeMux {
	var HandlerDebug, HandlerPrometheus = 1, 1

	mux := http.NewServeMux()

	if HandlerDebug != 0 {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	if HandlerPrometheus != 0 {
		initMetrics()
		mux.Handle("/metrics", promhttp.Handler())
	}

	for path, handler := range apiHandlersFunc {
		mux.Handle(path, handler)
	}

	return mux
}

// RunHTTPServers Возвращает список серверов, чтобы потом мы могли корректно остановить их по сигналу
func RunHTTPServers() (*http.Server, error) {
	httpAddress := viper.GetString("address")
	httpPort := viper.GetString("port")

	server := &http.Server{
		Addr:         net.JoinHostPort(httpAddress, httpPort),
		Handler:      getMux(),
		ReadTimeout:  defaultTimeout,
		WriteTimeout: defaultTimeout,
		IdleTimeout:  defaultIdleTimeout,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Fatal().Msgf("ListenAndServe: %v", err)
			}
		}
	}()

	return server, nil
}
