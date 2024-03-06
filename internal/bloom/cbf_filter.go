package bloom

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	boom "github.com/tylertreat/BoomFilters"

	"bloom-du/internal/utils"
)

type ClassicBloomFilter struct {
	CBF            *boom.BloomFilter
	sourceFilepath string
	dumpFilepath   string
	mux            sync.RWMutex
	needCheckpoint bool // true if new element added. False if not AND last checkpoint success
	logCh          chan LogEvent
}

// NewClassicBloomFilter creating and bootstrap from struct file if exist OR loading text data as source
func NewClassicBloomFilter(sourceFile string, force bool, logCh chan LogEvent, checkpointPath string) *ClassicBloomFilter {
	filter := ClassicBloomFilter{
		CBF:            boom.NewBloomFilter(200_000_000, 0.1),
		sourceFilepath: sourceFile,
		dumpFilepath:   checkpointPath,
		logCh:          logCh,
	}
	filter.Boostrap(force)
	filter.printLogStat()

	return &filter
}

func (f *ClassicBloomFilter) LogCh() chan<- LogEvent {
	return f.logCh
}

func (f *ClassicBloomFilter) Add(value string) {
	f.CBF.Add([]byte(value))
	f.needCheckpoint = true
}

func (f *ClassicBloomFilter) Test(value string) bool {
	return f.CBF.Test([]byte(value))
}

func (f *ClassicBloomFilter) TestAndAdd(value string) bool {
	result := f.CBF.TestAndAdd([]byte(value))
	if !result {
		f.needCheckpoint = true
		f.LogCh() <- LogEvent{Level: zerolog.DebugLevel, Name: "add", Count: 1.0}
	}

	return !result
}

func (f *ClassicBloomFilter) GetDumpSize() uint64 {
	return getDumpSize(f.dumpFilepath)
}

func (f *ClassicBloomFilter) Checkpoint() bool {
	if !f.needCheckpoint {
		f.LogCh() <- LogEvent{Level: zerolog.DebugLevel, Name: "checkpoint", Msg: "Checkpoint is not necessary now."}
		return false
	}

	file, err := os.OpenFile(f.dumpFilepath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	defer file.Close()

	f.mux.Lock()
	defer f.mux.Unlock()
	_, err = f.CBF.WriteTo(file)
	if err != nil {
		f.LogCh() <- LogEvent{
			Level: zerolog.ErrorLevel,
			Name:  "checkpoint",
			Msg:   fmt.Sprintf("Error to save Checkpoint: %v", err),
		}
		return false
	}

	f.needCheckpoint = false
	return true
}

func (f *ClassicBloomFilter) Boostrap(force bool) {
	sourceFile := f.sourceFilepath

	forceLoadFromSource := force && sourceFile != ""
	defaultDumpLoad := !force && f.isDumpExist()
	defaultSourceLoad := !force && sourceFile != "" && !f.isDumpExist()
	emptyLoad := sourceFile == "" && !f.isDumpExist()

	if forceLoadFromSource {
		if f.isDumpExist() {
			f.loadDump()
		}
		f.bootstrap()
	}

	if defaultDumpLoad {
		f.loadDump()
	}

	if defaultSourceLoad {
		f.LogCh() <- LogEvent{
			Level: zerolog.InfoLevel,
			Name:  bootstrapName,
			Msg:   fmt.Sprintf("Try load data from: %s", sourceFile),
		}
		f.bootstrap()
	}

	if emptyLoad {
		f.LogCh() <- LogEvent{
			Level: zerolog.InfoLevel,
			Name:  bootstrapName,
			Msg:   "Start empty filter",
		}
	}
}

func (f *ClassicBloomFilter) Engine() ProbabilisticEngine {
	return ClassicBloom
}

func (f *ClassicBloomFilter) loadDump() {
	file, err := os.OpenFile(f.dumpFilepath, os.O_RDONLY, 0644)
	if err != nil {
		log.Error().Err(err).Send()
	}
	f.mux.Lock()
	defer f.mux.Unlock()
	defer file.Close()
	f.LogCh() <- LogEvent{
		Level: zerolog.InfoLevel,
		Name:  bootstrapName,
		Msg:   fmt.Sprintf("Try load dump: %s!", f.dumpFilepath),
	}

	_, err = f.CBF.ReadFrom(file)

	if err != nil {
		log.Error().Err(err).Send()
	}
}

func (f *ClassicBloomFilter) isDumpExist() bool {
	_, err := os.Stat(f.dumpFilepath)
	if err != nil {
		return os.IsExist(err)
	}
	return true
}

func (f *ClassicBloomFilter) getLineCount() int {
	return getLineCount(f.sourceFilepath)
}

func (f *ClassicBloomFilter) bootstrap() {
	filename := f.sourceFilepath
	f.mux.Lock()
	defer f.mux.Unlock()

	file, err := os.Open(filename)
	if err != nil {
		f.LogCh() <- LogEvent{Level: zerolog.ErrorLevel, Name: bootstrapName, Msg: fmt.Sprintf("Load from file err: %v", err)}
		return
	}

	defer file.Close()

	added, scanned := 0, 0
	var scanner *bufio.Scanner

	if isGzSource(f.sourceFilepath) {
		gz, errs := gzip.NewReader(file)
		if errs != nil {
			log.Error().Err(errs).Send()
		}
		defer gz.Close()
		f.LogCh() <- LogEvent{
			Level: zerolog.InfoLevel,
			Name:  bootstrapName,
			Msg:   "Gzip source detected",
		}
		scanner = bufio.NewScanner(gz)
	} else {
		// todo maybe implemented progressbar
		scanner = bufio.NewScanner(file)
	}

	lineCount := f.getLineCount()

	for scanner.Scan() {
		scanned++
		if scanned%1_000_000 == 0 {
			f.LogCh() <- LogEvent{
				Level: zerolog.InfoLevel,
				Name:  bootstrapName,
				Msg:   fmt.Sprintf("Прочитано: %s из [%s]", utils.HumInt(scanned), utils.HumInt(lineCount)),
			}
		}

		if !f.CBF.TestAndAdd(scanner.Bytes()) {
			added++
			f.needCheckpoint = false
			f.LogCh() <- LogEvent{Level: zerolog.InfoLevel, Name: "add", Count: 1.0}
			if added%10_000_000 == 0 {
				f.LogCh() <- LogEvent{
					Level: zerolog.InfoLevel,
					Name:  bootstrapName,
					Msg:   fmt.Sprintf("Добавлено: %s из [%s]", utils.HumInt(added), utils.HumInt(lineCount)),
				}
			}
		}
	}

	if err = scanner.Err(); err != nil {
		f.LogCh() <- LogEvent{Level: zerolog.ErrorLevel, Name: bootstrapName, Msg: fmt.Sprintf("Load from file err: %v", err)}
	}

	f.needCheckpoint = true

	skipped := scanned - added
	f.LogCh() <- LogEvent{
		Level: zerolog.InfoLevel,
		Name:  bootstrapName,
		Msg:   fmt.Sprintf("Добавлено: [%s] Пропущено [%s]", utils.HumInt(added), utils.HumInt(skipped)),
		// Count: float64(added),
	}
}

func (f *ClassicBloomFilter) printLogStat() {
	msg := fmt.Sprintf("[Capacity: %s] [K: %d] Count: %s, FillRatio: %f, EstimatedFillRatio: %f",
		utils.HumInt(int(f.CBF.Capacity())),
		f.CBF.K(),
		utils.HumInt(int(f.CBF.Count())),
		f.CBF.FillRatio(),
		f.CBF.EstimatedFillRatio(),
	)
	f.LogCh() <- LogEvent{Level: zerolog.DebugLevel, Name: bootstrapName, Msg: msg}
}
