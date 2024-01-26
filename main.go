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
	var (
		force              bool
		checkpointInterval time.Duration
		socketPath         string
	)

	var rootCmd = &cobra.Command{
		Use:   "bloom-du",
		Short: "bloom-du - Bloom Filter implementation",
		Long:  `bloom-du - Bloom Filter implementation`,
		Run: func(cmd *cobra.Command, args []string) {

			viper.SetDefault("source", "")
			viper.SetDefault("port", 8515)
			viper.SetDefault("address", "0.0.0.0")
			viper.SetDefault("log_level", "info")
			viper.SetDefault("log_file", "")
			viper.SetDefault("force", false)
			viper.SetDefault("checkpoint_interval", 600*time.Second)
			viper.SetDefault("checkpoint_path", "/var/lib/bloom-du/sbfData.bloom")
			viper.SetDefault("socket_path", "/tmp/bloom-du.sock")

			bindPFlags := []string{
				"source", "port", "address", "log_level", "log_file", "force",
				"checkpoint_interval", "socket_path", "checkpoint_path",
			}
			for _, flag := range bindPFlags {
				_ = viper.BindPFlag(flag, cmd.Flags().Lookup(flag))
			}

			file := setupLogging()
			if file != nil {
				defer func() { _ = file.Close() }()
			}

			assertPermissions()

			httpServer, err := api.RunHTTPServers()
			if err != nil {
				log.Fatal().Msgf("error running HTTP server: %v", err)
				os.Exit(1)
			} else {
				log.Info().Msgf("listen and serve on: %s", httpServer.Addr)
			}

			_, err = api.RunUnixSocket(socketPath)
			if err != nil {
				log.Fatal().Msgf("error running Socket: %v", err)
			} else {
				log.Info().Msgf("listen on socket: %s", socketPath)
			}

			go handleSignals(httpServer)

			api.Start()

			log.Info().
				Str("version", build.Version).
				Str("runtime", runtime.Version()).
				Int("pid", os.Getpid()).
				Int("gomaxprocs", runtime.GOMAXPROCS(0)).
				Str("log_level", viper.GetString("log_level")).
				Str("checkpoint_interval", checkpointInterval.String()).
				Str("checkpoint_path", viper.GetString("checkpoint_path")).
				Msg("starting")

			ticker := time.NewTicker(checkpointInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					api.Checkpoint()
				}
			}
		},
	}

	rootCmd.Flags().StringP("source", "s", "", "path to source data file")
	rootCmd.PersistentFlags().BoolVarP(&force, "force", "f", false, "force load from source file, ignoring a dump")
	rootCmd.Flags().StringP("address", "a", "0.0.0.0", "address to serve")
	rootCmd.Flags().Int("port", 8515, "port to serve on")
	rootCmd.PersistentFlags().StringVarP(&socketPath, "socket_path", "u", "/tmp/bloom-du.sock", "Unix socket path")
	rootCmd.Flags().StringP("log_level", "", "info", "log level: trace, debug, info, error, fatal or none")
	rootCmd.Flags().StringP("log_file", "l", "", "log file path")
	rootCmd.PersistentFlags().DurationVarP(&checkpointInterval, "checkpoint_interval", "i", 600*time.Second, "checkpoint")
	rootCmd.Flags().StringP("checkpoint_path", "o", "/var/lib/bloom-du/sbfData.bloom", "checkpoint path")

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show bloom-du version information",
		Long:  `Show bloom-du version information`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("bloom-du v%s (Go version: %s)\n", build.Version, runtime.Version())
		},
	}

	var serveCmd = &cobra.Command{
		Use:   "check",
		Short: "check item in the Filter",
		Long:  `check item in the Filter`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Привет, здесь будет функция проверки элемента в фильтре")
		},
	}

	var filterCmd = &cobra.Command{
		Use:   "filter",
		Short: "Creating probability filter",
		Long:  `Creating one of probability filter`,
		Run: func(cmd *cobra.Command, args []string) {
			// TODO нужен глобальный список фильтров, с общим доступом.
			// ещё лучше иметь возможность загружать это всё из конфигурации
			fmt.Println("Здесь будет функция создания выбранного фильтра")
		},
	}

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(filterCmd)
	_ = rootCmd.Execute()
}

func handleSignals(httpServer *http.Server) {
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
			log.Info().Msg("TODO reloading configuration")
			// tryReadConfig()
		case syscall.SIGINT, syscall.SIGTERM, os.Interrupt:
			log.Info().Msg("Shutting down ...")

			shutdownTimeout := 3 * time.Second
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
			api.Checkpoint()
			cleanup()
		case syscall.SIGUSR2:
			log.Info().Msg("Test SIGUSR2")
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

// TODO add map and for range check
func assertPermissions() {
	checkpointPath := viper.GetString("checkpoint_path")
	source := viper.GetString("source")

	checkReadPermission(source)
	checkWritePermission(checkpointPath)
}

func checkReadPermission(filePath string) {
	if filePath == "" {
		return
	}
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatal().Err(err).Send()
	}

	_ = file.Close()
}

func checkWritePermission(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		file, err = os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			log.Fatal().Err(err).Send()
		}
		_ = os.Remove(filePath)
	}
	_ = file.Close()
}

func cleanup() {
	socketPath := viper.GetString("socket_path")
	_ = os.Remove(socketPath)
	// os.Exit(1)
}
