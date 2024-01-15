package utils

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// StopWatchLog todo поискать готовую либу
func StopWatchLog(start time.Time, text string) float64 {
	elapsed := time.Since(start)
	log.Info().Msg(fmt.Sprintf("%s [%s]", text, elapsed))
	return float64(elapsed)
}
