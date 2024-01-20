package bloom

import (
	"errors"
	"fmt"
)

const (
	ClassicBloom ProbabilisticEngine = iota + 1
	StableBloom
	CountingBloom
	CuckooBloom
)

var NotImplementedError = errors.New("function not implemented")

type ProbabilisticEngine uint8

type Filter interface {
	Engine() ProbabilisticEngine
	Add(value string)
	Test(value string) bool
	Drop() error
}

// MakeEngine TODO попробовать реализовать это через Cobra, а уже потом через API
func MakeEngine(name string) (Filter, error) {
	file := "x"
	switch name {
	case "postgres":
		return CreateFilter(file, false), nil

	default:
		return nil, fmt.Errorf("unknown stucture type: `%s`", name)
	}
}
