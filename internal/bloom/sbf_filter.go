package bloom

import (
	"bufio"
	"fmt"
	"os"
	"sync"

	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog/log"
	boom "github.com/tylertreat/BoomFilters"
)

const (
	// M Размер битового массива фильтра Блума.
	M = 1_000_000_000

	// fpRate The desired rate of false positives.
	fpRate        = 0.0001
	dumpFileName  = "sbfData.bloom"
	integerFormat = "#,###."
)

type StableBloomFilter struct {
	SBF            *boom.StableBloomFilter
	dumpFilepath   string
	mux            sync.RWMutex
	needCheckpoint bool // true if new element added. False if not AND last checkpoint success
	isReady        bool
}

// CreateFilter creating and bootstrap SBF from struct file if exist OR loading text data as source
func CreateFilter(sourceFile string, force bool) *StableBloomFilter {
	defaultSbf := boom.NewStableBloomFilter(M, 1, fpRate)

	filter := StableBloomFilter{SBF: defaultSbf, dumpFilepath: dumpFileName}
	filter.Boostrap(sourceFile, force)
	return &filter
}

func (f *StableBloomFilter) Add(value string) {
	f.SBF.Add([]byte(value))
	f.needCheckpoint = true
}

func (f *StableBloomFilter) Test(value string) bool {
	return f.SBF.Test([]byte(value))
}

func (f *StableBloomFilter) TestAndAdd(value string) bool {
	result := f.SBF.TestAndAdd([]byte(value))
	if !result {
		f.needCheckpoint = true
	}
	return result
}

func (f *StableBloomFilter) GetDumpSize() uint64 {
	file, err := os.OpenFile(f.dumpFilepath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Panic().Err(err).Send()
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return 0
	}
	return uint64(stat.Size())
}

func (f *StableBloomFilter) Checkpoint() bool {
	if !f.needCheckpoint {
		log.Debug().Msg("Checkpoint is not necessary now.")
		return false
	}

	file, err := os.OpenFile(f.dumpFilepath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Panic().Err(err).Send()
	}
	defer file.Close()

	f.mux.Lock()
	defer f.mux.Unlock()
	_, err = f.SBF.WriteTo(file)
	if err != nil {
		log.Error().Msg(fmt.Sprintf("Error to save Checkpoint: %v", err))
		return false
	}

	f.needCheckpoint = false
	return true
}

func (f *StableBloomFilter) IsReady() bool {
	return f.isReady
}

func (f *StableBloomFilter) setIsReady(state bool) {
	f.isReady = state
}

func (f *StableBloomFilter) loadFromDumpFile() (int64, error) {
	f.mux.Lock()
	defer f.mux.Unlock()
	file, err := os.OpenFile(f.dumpFilepath, os.O_RDONLY, 0644)
	if err != nil {
		log.Panic().Err(err).Send()
	}
	defer file.Close()

	numBytes, err := f.SBF.ReadFrom(file)
	f.PrintLogStat()

	return numBytes, err
}

func (f *StableBloomFilter) isDumpExist() bool {
	_, err := os.Stat(f.dumpFilepath)
	if err != nil {
		return os.IsExist(err)
	}
	return true
}

func (f *StableBloomFilter) Boostrap(sourceFile string, force bool) {
	f.setIsReady(false)
	defer f.setIsReady(true)

	forceLoadFromSource := force && sourceFile != ""
	defaultDumpLoad := !force && f.isDumpExist()
	defaultSourceLoad := !force && sourceFile != "" && !f.isDumpExist()
	emptyLoad := sourceFile == "" && !f.isDumpExist()

	if forceLoadFromSource {
		log.Info().Msg(fmt.Sprintf("Try force load data from: %s!", sourceFile))
		f.bootstrap(sourceFile)
	}

	if defaultDumpLoad {
		log.Info().Msg(fmt.Sprintf("%s exist. Load ...", f.dumpFilepath))
		_, err := f.loadFromDumpFile()
		if err != nil {
			log.Error().Msg(fmt.Sprintf("Error load from dump file: %s", f.dumpFilepath))
		}
	}

	if defaultSourceLoad {
		log.Info().Msg(fmt.Sprintf("Try load data from: %s", sourceFile))
		f.bootstrap(sourceFile)
	}

	if emptyLoad {
		log.Info().Msg("Start empty filter")
	}
}

func (f *StableBloomFilter) PrintLogStat() {
	// StablePoint returns the limit of the expected fraction of zeros in the Filter
	log.Debug().Msg(fmt.Sprintf("[P:%d] [K: %d] Cells: %s, Stable point: %f, FalsePositiveRate: %f",
		f.SBF.P(),
		f.SBF.K(),
		humanize.FormatInteger(integerFormat, int(f.SBF.Cells())),
		f.SBF.StablePoint(),
		f.SBF.FalsePositiveRate(),
	))
}

func (f *StableBloomFilter) bootstrap(filename string) {
	f.mux.Lock()
	defer f.mux.Unlock()

	file, err := os.Open(filename)
	if err != nil {
		log.Error().Msg(fmt.Sprintf("Load from file err: %v", err))
		return
	}
	defer file.Close()

	counter := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		f.SBF.Add(scanner.Bytes())
		counter++
		f.needCheckpoint = false
		if counter%10_000_000 == 0 {
			f.printCounter(counter)
		}
	}

	if err = scanner.Err(); err != nil {
		log.Error().Msg(fmt.Sprintf("Load from file err: %v", err))
	}

	f.needCheckpoint = true
	f.PrintLogStat()
	f.printCounter(counter)
}

// printCounter P returns the number of cells decremented on every add.
func (f *StableBloomFilter) printCounter(counter int) {
	log.Info().Msg(fmt.Sprintf("Добавлено: %s", humanize.FormatInteger(integerFormat, counter)))
}

func (f *StableBloomFilter) Engine() ProbabilisticEngine {
	return StableBloom
}

func (f *StableBloomFilter) Drop() error {
	log.Info().Msg("Drop() not implemented")
	return nil
}
