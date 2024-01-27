package utils

import (
	"fmt"
	"os"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const integerFormat = "#,###."

// todo скорее всего можно выпилить после экспериментов (оставить только HTTP метрики)
type LogEvent struct {
	Level zerolog.Level
	Name  string
	Msg   string
	Count float64
}

// StopWatchLog todo скорее всего можно выпилить после экспериментов (оставить только HTTP метрики)
func StopWatchLog(ch chan<- LogEvent, start time.Time, text string) float64 {
	elapsed := time.Since(start)

	ch <- LogEvent{
		Level: zerolog.DebugLevel,
		Name:  "api",
		Msg:   fmt.Sprintf("%s [%s]", text, elapsed),
	}

	return elapsed.Seconds()
}

func HumInt(num int) string {
	return humanize.FormatInteger(integerFormat, num)
}

func HumByte(s *uint64) string {
	return humanize.Bytes(*s)
}

func AssertReadPermission(filePath string) {
	if filePath == "" {
		return
	}
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatal().Err(err).Send()
	}

	_ = file.Close()
}

func AssertWritePermission(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		file, err = os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			log.Fatal().Err(err).Send()
		}
		_ = os.Remove(filePath)
	}
	_ = file.Close()
}
