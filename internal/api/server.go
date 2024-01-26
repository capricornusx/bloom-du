package api

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
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
	metricsPath        = "/metrics"
	healthPath         = "/health"
)

var apiHandlersFunc = map[string]http.HandlerFunc{
	"/api/check":      handleCheck,
	"/api/fcheck":     handleFastCheck,
	"/api/add":        handleAdd,
	"/api/bulk":       handleBulkLoad,
	"/api/checkpoint": handleCheckpoint,
	"/health":         healthHandler,
}

var (
	labelNames = []string{labelQuery}
	objectives = map[float64]float64{0.5: 0.05, 0.9: 0.01}
	buckets    = []float64{.1, .25, .5, 1}
)

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
	requestDurationSummary = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  metricsNamespace,
			Subsystem:  "api",
			Name:       "http_request_duration_seconds",
			Help:       "Time (in seconds) spent serving HTTP requests",
			Objectives: objectives,
		},
		[]string{"path"},
	)
	requestDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: "api",
			Name:      "http_request_hist_duration_seconds",
			Help:      "Histogram of duration HTTP requests",
			Buckets:   buckets, // prometheus.DefBuckets,
		}, []string{"path"})
	responseCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "api",
			Name:      "http_response_codes_total",
			Help:      "Number of HTTP responses sent, partitioned by status code and HTTP method.",
		},
		[]string{"code"},
	)
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RunHTTPServers Возвращает список серверов, чтобы потом мы могли корректно остановить их по сигналу
func RunHTTPServers() (*http.Server, error) {
	httpAddress := viper.GetString("address")
	httpPort := viper.GetString("port")

	mux := getMux()

	server := &http.Server{
		Addr:         net.JoinHostPort(httpAddress, httpPort),
		Handler:      measureHandler(mux),
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

func RunUnixSocket(path string) (net.Listener, error) {
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			conn, errs := listener.Accept()
			if errs != nil {
				log.Fatal().Err(errs).Send()
				break
			}
			go handleSocket(conn)
		}
	}()
	return listener, nil
}

// TODO add metrics
func handleSocket(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 1024)
	for {
		// Читаем данные из соединения.
		data, err := conn.Read(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Error().Err(err).Send()
			}
			return
		}

		log.Info().Msg(string(buf[:data]))

		// Отправляем данные обратно в соединение.
		_, err = conn.Write(buf[:data])
		if err != nil {
			log.Error().Err(err).Send()
		}
	}
}

// initMetrics Prometheus metrics
func initMetrics() {
	prometheus.MustRegister(CurrentConfig)
	prometheus.MustRegister(Elements)
	prometheus.MustRegister(requestDurationSummary)
	prometheus.MustRegister(requestDurationHistogram)
	prometheus.MustRegister(responseCounter)
}

func measureHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		path := r.URL.Path
		// Создаем writer, который будет отслеживать код ответа
		writer := &responseWriter{ResponseWriter: w}

		if path == metricsPath || path == healthPath {
			next.ServeHTTP(writer, r)
			return
		}

		_, ok := apiHandlersFunc[path]
		if !ok {
			next.ServeHTTP(writer, r)
			return
		}

		next.ServeHTTP(writer, r)

		observe(started, path)
		responseCounter.WithLabelValues(strconv.Itoa(writer.statusCode)).Inc()
	})
}

func observe(started time.Time, path string) {
	duration := time.Since(started).Seconds()
	requestDurationSummary.WithLabelValues(path).Observe(duration)
	requestDurationHistogram.WithLabelValues(path).Observe(duration)
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := w.Write([]byte("OK"))
	if err != nil {
		return
	}
}

func getMux() *http.ServeMux {
	var HandlerDebug = 1

	mux := http.NewServeMux()

	if HandlerDebug != 0 {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	initMetrics()
	mux.Handle(metricsPath, promhttp.Handler())

	for path, handler := range apiHandlersFunc {
		mux.Handle(path, handler)
	}

	return mux
}
