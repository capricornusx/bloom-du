package bloom

import (
	"fmt"

	"bloom-du/internal/utils"
)

const (
	ClassicBloom ProbabilisticEngine = iota + 1
	StableBloom
	CountingBloom
	CuckooBloom
)

type ProbabilisticEngine uint8

type Filter interface {
	Engine() ProbabilisticEngine
	Add(value string)
	Test(value string) bool
	Drop() error
}

// MakeEngine TODO попробовать реализовать это через Cobra, а уже потом через API
func MakeEngine(name string) (Filter, error) {
	switch name {
	case "postgres":
		logCh := make(chan utils.LogEvent, 5)
		return NewStableBloomFilter("", false, logCh, "sbfData.bloom"), nil

	default:
		return nil, fmt.Errorf("unknown stucture type: `%s`", name)
	}
}
