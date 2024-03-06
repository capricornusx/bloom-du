package utils

import (
	"os"

	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog/log"
)

const integerFormat = "#,###."

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
