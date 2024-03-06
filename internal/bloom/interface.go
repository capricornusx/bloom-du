package bloom

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	// M Размер битового массива фильтра Блума.
	M = 1_000_000_000

	// fpRate The desired rate of false positives.
	fpRate        = 0.01
	bootstrapName = "bootstrap"
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
	TestAndAdd(value string) bool
	GetDumpSize() uint64
	Checkpoint() bool
	LogCh() chan<- LogEvent
}

type LogEvent struct {
	Level zerolog.Level
	Name  string
	Msg   string
	Count float64
}

//type dataSource interface {
//	Data(query string) ([]datastore.DataItem, error)
//}
//
//func NewService(ds dataSource) SomeService {
//	return SomeService{
//		dataSource: ds,
//	}
//}

// MakeEngine TODO попробовать реализовать это через Cobra, а уже потом через API
// TODO https://mycodesmells.com/post/accept-interfaces-return-struct-in-go
// TODO реализовать классический фильтр и сравнить производительность с redis
func MakeEngine(
	name ProbabilisticEngine,
	source string,
	force bool,
	logCh chan LogEvent,
	checkpointPath string,
) (Filter, error) {
	switch name {
	case StableBloom:
		return NewStableBloomFilter(source, force, logCh, checkpointPath), nil
	case ClassicBloom:
		return NewClassicBloomFilter(source, force, logCh, checkpointPath), nil
	default:
		return nil, fmt.Errorf("unknown stucture type: `%d`", name)
	}
}

func getDumpSize(dumpFilepath string) uint64 {
	file, err := os.OpenFile(dumpFilepath, os.O_RDONLY, 0644)
	if err != nil {
		log.Error().Err(err).Send()
		return 0
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return 0
	}
	return uint64(stat.Size())
}

func getLineCount(sourceFilepath string) int {
	if isGzSource(sourceFilepath) {
		return lineCounterGz(sourceFilepath)
	}
	return lineCounterPlain(sourceFilepath)
}

func isGzSource(sourceFilepath string) bool {
	ext := filepath.Ext(sourceFilepath)
	return ext == ".gz"
}

func lineCounterPlain(filePath string) int {
	count := 0
	file, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	lineSep := []byte{'\n'}

	for {
		c, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return count
		}

		count += bytes.Count(buf[:c], lineSep)

		if err == io.EOF {
			break
		}
	}

	return count
}

func lineCounterGz(filePath string) int {
	count := 0
	file, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	lineSep := []byte{'\n'}

	gz, err := gzip.NewReader(file)
	if err != nil {
		return 0
	}
	defer gz.Close()

	for {
		n, err := gz.Read(buf)
		if err != nil && err != io.EOF {
			return count
		}

		count += bytes.Count(buf[:n], lineSep)
		if err == io.EOF {
			break
		}
	}

	return count
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
