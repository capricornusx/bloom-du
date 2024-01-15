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
}

// CreateFilter creating and bootstrap SBF from struct file if exist OR loading text data as source
func CreateFilter(sourceFile string) *StableBloomFilter {
	defaultSbf := boom.NewStableBloomFilter(M, 1, fpRate)

	filter := StableBloomFilter{SBF: defaultSbf, dumpFilepath: dumpFileName}
	return filter.Boostrap(sourceFile)
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

func (f *StableBloomFilter) loadStructFromFile() (int64, error) {
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

func (f *StableBloomFilter) checkF() bool {
	_, err := os.Stat(f.dumpFilepath)
	if err != nil {
		return os.IsExist(err)
	}
	return true
}

func (f *StableBloomFilter) Boostrap(sourceFile string) *StableBloomFilter {
	if sourceFile == "" && !f.checkF() {
		log.Info().Msg("Start empty filter")
		return f
	}

	if sourceFile != "" {
		log.Info().Msg(fmt.Sprintf("Load bootstrap data from %s!", sourceFile))
		f.bootstrap(sourceFile)
	}

	if sourceFile == "" && f.checkF() {
		log.Info().Msg(fmt.Sprintf("%s exist. Skip boostrap!", f.dumpFilepath))
		_, err := f.loadStructFromFile()
		if err != nil {
			return f
		}
	}

	return f
}

func (f *StableBloomFilter) PrintLogStat() {
	// StablePoint returns the limit of the expected fraction of zeros in the Filter
	log.Info().Msg(fmt.Sprintf("[P:%d] [K: %d] Cells: %s, Stable point: %f",
		f.SBF.P(),
		f.SBF.K(),
		humanize.FormatInteger(integerFormat, int(f.SBF.Cells())),
		f.SBF.StablePoint(),
	))
}

func (f *StableBloomFilter) bootstrap(filename string) {
	f.mux.Lock()
	defer f.mux.Unlock()

	file, err := os.Open(filename)
	if err != nil {
		log.Panic().Err(err).Send()
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
		log.Panic().Err(err).Send()
	}

	f.needCheckpoint = true
	f.PrintLogStat()
	f.printCounter(counter)
}

// printCounter P returns the number of cells decremented on every add.
func (f *StableBloomFilter) printCounter(counter int) {
	log.Info().Msg(fmt.Sprintf("Добавлено: %s", humanize.FormatInteger(integerFormat, counter)))
}
