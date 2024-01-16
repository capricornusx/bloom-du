package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"bloom-du/internal/api"
	"bloom-du/internal/build"
	"bloom-du/internal/utils"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "bloom-du",
		Short: "bloom-du - Bloom Filter implementation",
		Long:  `bloom-du - Bloom Filter implementation`,
		Run: func(cmd *cobra.Command, args []string) {

			viper.SetDefault("source", "")
			viper.SetDefault("port", "8515")
			viper.SetDefault("address", "0.0.0.0")
			viper.SetDefault("log_level", "info")

			bindPFlags := []string{"source", "port", "address", "log_level"}
			for _, flag := range bindPFlags {
				_ = viper.BindPFlag(flag, cmd.Flags().Lookup(flag))
			}

			file := setupLogging()
			if file != nil {
				defer func() { _ = file.Close() }()
			}

			httpServer, err := api.RunHTTPServers()
			if err != nil {
				log.Fatal().Msgf("error running HTTP server: %v", err)
			} else {
				log.Info().Msgf("listen and serve on: %s", httpServer.Addr)
			}

			infoCh := make(chan os.Signal, 1)
			go handleSignals(infoCh, httpServer)

			api.Start()

			log.Info().
				Str("version", build.Version).
				Str("runtime", runtime.Version()).
				Int("pid", os.Getpid()).
				Int("gomaxprocs", runtime.GOMAXPROCS(0)).Msg("starting bloom-du")

			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					api.Checkpoint()
				case <-infoCh:
					// TODO, сохранять дамп по сигналу SIGHUP или через API
					// если сигнал пришёл от CTRL+C то наверное и чекпоинт не нужен
					// тем более на этапе bootstrap
					api.Checkpoint()
				}
			}
		},
	}

	rootCmd.Flags().StringP("source", "s", "source.txt", "path to source data file")
	rootCmd.Flags().StringP("port", "p", "8515", "port to serve on")
	rootCmd.Flags().StringP("address", "a", "0.0.0.0", "interface to serve")
	rootCmd.Flags().StringP("log_level", "", "info", "set the log level: trace, debug, info, error, fatal or none")

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "bloom-du version information",
		Long:  `Print the version information of bloom-du`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("bloom-du v%s (Go version: %s)\n", build.Version, runtime.Version())
		},
	}

	var serveCmd = &cobra.Command{
		Use:   "check",
		Short: "check item in the Filter",
		Long:  `check item in the Filter`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Привет, здесь будет функция проверки")
		},
	}

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
	_ = rootCmd.Execute()
}

func handleSignals(infoCh chan<- os.Signal, httpServer *http.Server) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh,
		os.Interrupt,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGUSR1,
		syscall.SIGUSR2,
	)

	for {
		sig := <-sigCh
		log.Info().Msgf("signal received: %v", sig)
		switch sig {
		case syscall.SIGHUP:
			log.Info().Msg("reloading configuration")
			// tryReadConfig()
		case syscall.SIGINT, syscall.SIGTERM, os.Interrupt:
			log.Info().Msg("Shutting down ...")
			infoCh <- syscall.SIGTERM
			go time.AfterFunc(3*time.Second, func() { os.Exit(0) })
		case syscall.SIGUSR2:
			log.Info().Msg("Test shutting down ...")

			shutdownTimeout := 5 * time.Second
			go time.AfterFunc(shutdownTimeout, func() { os.Exit(1) })

			var wg sync.WaitGroup

			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			wg.Add(1)
			go func(srv *http.Server) {
				defer wg.Done()
				_ = srv.Shutdown(ctx)
			}(httpServer)

			wg.Wait()
			cancel()

			os.Exit(0)
		default:
		}
	}
}

func configureConsoleWriter() {
	if isTerminalAttached() {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:                 os.Stdout,
			TimeFormat:          "2006-01-02 15:04:05",
			FormatLevel:         utils.ConsoleFormatLevel(),
			FormatErrFieldName:  utils.ConsoleFormatErrFieldName(),
			FormatErrFieldValue: utils.ConsoleFormatErrFieldValue(),
		})
	}
}

func isTerminalAttached() bool {
	//goland:noinspection GoBoolExpressions – Goland is not smart enough here.
	return isatty.IsTerminal(os.Stdout.Fd()) && runtime.GOOS != "windows"
}

func setupLogging() *os.File {
	configureConsoleWriter()
	var logLevelMatches = map[string]zerolog.Level{
		"NONE":  zerolog.NoLevel,
		"TRACE": zerolog.TraceLevel,
		"DEBUG": zerolog.DebugLevel,
		"INFO":  zerolog.InfoLevel,
		"WARN":  zerolog.WarnLevel,
		"ERROR": zerolog.ErrorLevel,
		"FATAL": zerolog.FatalLevel,
	}
	logLevel, ok := logLevelMatches[strings.ToUpper(viper.GetString("log_level"))]
	if !ok {
		logLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevel)
	if viper.IsSet("log_file") && viper.GetString("log_file") != "" {
		f, err := os.OpenFile(viper.GetString("log_file"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal().Msgf("error opening log file: %v", err)
		}
		log.Logger = log.Output(f)
		return f
	}

	return nil

}
