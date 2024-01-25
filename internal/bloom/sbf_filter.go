package bloom

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	boom "github.com/tylertreat/BoomFilters"

	"bloom-du/internal/utils"
)

const (
	// M Размер битового массива фильтра Блума.
	M = 1_000_000_000

	// fpRate The desired rate of false positives.
	fpRate        = 0.0001
	bootstrapName = "bootstrap"
)

type StableBloomFilter struct {
	SBF            *boom.StableBloomFilter
	dumpFilepath   string
	mux            sync.RWMutex
	needCheckpoint bool // true if new element added. False if not AND last checkpoint success
	isReady        bool
	LogCh          chan utils.LogEvent
}

// NewStableBloomFilter creating and bootstrap SBF from struct file if exist OR loading text data as source
func NewStableBloomFilter(sourceFile string, force bool, logCh chan utils.LogEvent, checkpointPath string) *StableBloomFilter {
	defaultSbf := boom.NewStableBloomFilter(M, 1, fpRate)

	filter := StableBloomFilter{SBF: defaultSbf, dumpFilepath: checkpointPath, LogCh: logCh}
	filter.Boostrap(sourceFile, force)
	filter.printLogStat()
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
		f.LogCh <- utils.LogEvent{Level: zerolog.DebugLevel, Name: "add", Count: 1.0}
	}

	return !result
}

func (f *StableBloomFilter) GetDumpSize() uint64 {
	file, err := os.OpenFile(f.dumpFilepath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Error().Err(err).Send()
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
		f.LogCh <- utils.LogEvent{Level: zerolog.DebugLevel, Name: "checkpoint", Msg: "Checkpoint is not necessary now."}
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
		f.LogCh <- utils.LogEvent{
			Level: zerolog.ErrorLevel,
			Name:  "checkpoint",
			Msg:   fmt.Sprintf("Error to save Checkpoint: %v", err),
		}
		return false
	}

	f.needCheckpoint = false
	return true
}

func (f *StableBloomFilter) IsReady() bool {
	return f.isReady
}

func (f *StableBloomFilter) Boostrap(sourceFile string, force bool) {
	f.setIsReady(false)
	defer f.setIsReady(true)

	forceLoadFromSource := force && sourceFile != ""
	defaultDumpLoad := !force && f.isDumpExist()
	defaultSourceLoad := !force && sourceFile != "" && !f.isDumpExist()
	emptyLoad := sourceFile == "" && !f.isDumpExist()

	if forceLoadFromSource {
		f.LogCh <- utils.LogEvent{
			Level: zerolog.InfoLevel,
			Name:  bootstrapName,
			Msg:   fmt.Sprintf("Try force load data from: %s!", sourceFile),
		}
		f.bootstrap(sourceFile)
	}

	if defaultDumpLoad {
		f.LogCh <- utils.LogEvent{
			Level: zerolog.InfoLevel,
			Name:  bootstrapName,
			Msg:   fmt.Sprintf("%s exist. Load ...", f.dumpFilepath),
		}
		_, err := f.loadFromDumpFile()
		if err != nil {
			log.Error().Msg(fmt.Sprintf("Error load from dump file: %s", f.dumpFilepath))
		}
	}

	if defaultSourceLoad {
		f.LogCh <- utils.LogEvent{
			Level: zerolog.InfoLevel,
			Name:  bootstrapName,
			Msg:   fmt.Sprintf("Try load data from: %s", sourceFile),
		}
		f.bootstrap(sourceFile)
	}

	if emptyLoad {
		f.LogCh <- utils.LogEvent{
			Level: zerolog.InfoLevel,
			Name:  bootstrapName,
			Msg:   "Start empty filter",
		}
	}
}

func (f *StableBloomFilter) Engine() ProbabilisticEngine {
	return StableBloom
}

func (f *StableBloomFilter) Drop() error {
	log.Info().Msg("Drop() not implemented")
	return nil
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

	return numBytes, err
}

func (f *StableBloomFilter) isDumpExist() bool {
	_, err := os.Stat(f.dumpFilepath)
	if err != nil {
		return os.IsExist(err)
	}
	return true
}

func (f *StableBloomFilter) bootstrap(filename string) {
	f.mux.Lock()
	defer f.mux.Unlock()

	// todo maybe implemented progressbar
	totalLines, _ := lineCounter(filename)

	file, err := os.Open(filename)
	if err != nil {
		f.LogCh <- utils.LogEvent{Level: zerolog.ErrorLevel, Name: bootstrapName, Msg: fmt.Sprintf("Load from file err: %v", err)}
		return
	}

	defer file.Close()

	added, scanned := 0, 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		scanned++
		if !f.SBF.TestAndAdd(scanner.Bytes()) {
			added++
			f.needCheckpoint = false
			f.LogCh <- utils.LogEvent{Level: zerolog.InfoLevel, Name: "add", Count: 1.0}
			if added%1_000_000 == 0 {
				f.LogCh <- utils.LogEvent{
					Level: zerolog.InfoLevel,
					Name:  bootstrapName,
					Msg:   fmt.Sprintf("Добавлено: %s из [%s]", utils.HumInt(added), utils.HumInt(totalLines)),
				}
			}
		}
	}

	if err = scanner.Err(); err != nil {
		f.LogCh <- utils.LogEvent{Level: zerolog.ErrorLevel, Name: bootstrapName, Msg: fmt.Sprintf("Load from file err: %v", err)}
	}

	f.needCheckpoint = true

	skipped := scanned - added
	f.LogCh <- utils.LogEvent{
		Level: zerolog.InfoLevel,
		Name:  bootstrapName,
		Msg:   fmt.Sprintf("Добавлено: [%s] Пропущено [%s]", utils.HumInt(added), utils.HumInt(skipped)),
		// Count: float64(added),
	}

}

func (f *StableBloomFilter) printLogStat() {
	msg := fmt.Sprintf("[P: %d] [K: %d] Cells: %s, Stable point: %f, FalsePositiveRate: %f",
		f.SBF.P(),
		f.SBF.K(),
		utils.HumInt(int(f.SBF.Cells())),
		f.SBF.StablePoint(),
		f.SBF.FalsePositiveRate(),
	)
	f.LogCh <- utils.LogEvent{Level: zerolog.DebugLevel, Name: bootstrapName, Msg: msg}
}

func lineCounter(filename string) (int, error) {
	count := 0
	file, err := os.Open(filename)
	if err != nil {
		return count, err
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	lineSep := []byte{'\n'}

	for {
		c, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return count, err
		}

		count += bytes.Count(buf[:c], lineSep)

		if err == io.EOF {
			break
		}
	}

	return count, nil
}
