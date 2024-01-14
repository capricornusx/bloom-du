package utils

import (
	"log"
	"time"
)

// StopWatchLog todo поискать готовую либу
func StopWatchLog(start time.Time, text string) float64 {
	elapsed := time.Since(start)
	log.Printf("%s [%s]\n", text, elapsed)
	return float64(elapsed)
}
