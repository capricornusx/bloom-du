package bloom

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	sourceFilepath string
	dumpFilepath   string
	mux            sync.RWMutex
	needCheckpoint bool // true if new element added. False if not AND last checkpoint success
	LogCh          chan utils.LogEvent
}

// NewStableBloomFilter creating and bootstrap SBF from struct file if exist OR loading text data as source
func NewStableBloomFilter(sourceFile string, force bool, logCh chan utils.LogEvent, checkpointPath string) *StableBloomFilter {
	defaultSbf := boom.NewStableBloomFilter(M, 1, fpRate)

	filter := StableBloomFilter{
		SBF:            defaultSbf,
		sourceFilepath: sourceFile,
		dumpFilepath:   checkpointPath,
		LogCh:          logCh,
	}
	filter.Boostrap(force)
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
	file, err := os.OpenFile(f.dumpFilepath, os.O_RDONLY, 0644)
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

func (f *StableBloomFilter) Checkpoint() bool {
	if !f.needCheckpoint {
		f.LogCh <- utils.LogEvent{Level: zerolog.DebugLevel, Name: "checkpoint", Msg: "Checkpoint is not necessary now."}
		return false
	}

	file, err := os.OpenFile(f.dumpFilepath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal().Err(err).Send()
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

func (f *StableBloomFilter) Boostrap(force bool) {
	sourceFile := f.sourceFilepath

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
		if f.isDumpExist() {
			_ = f.loadDump()
		}
		f.bootstrap()
	}

	if defaultDumpLoad {
		f.LogCh <- utils.LogEvent{
			Level: zerolog.InfoLevel,
			Name:  bootstrapName,
			Msg:   fmt.Sprintf("%s exist. Load ...", f.dumpFilepath),
		}
		err := f.loadDump()
		if err != nil {
			log.Error().Msgf("Error load from dump file: %s", f.dumpFilepath)
		}
	}

	if defaultSourceLoad {
		f.LogCh <- utils.LogEvent{
			Level: zerolog.InfoLevel,
			Name:  bootstrapName,
			Msg:   fmt.Sprintf("Try load data from: %s", sourceFile),
		}
		f.bootstrap()
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

func (f *StableBloomFilter) loadDump() error {
	file, err := os.OpenFile(f.dumpFilepath, os.O_RDONLY, 0644)
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	f.mux.Lock()
	defer f.mux.Unlock()
	defer file.Close()

	_, err = f.SBF.ReadFrom(file)

	return err
}

func (f *StableBloomFilter) isDumpExist() bool {
	_, err := os.Stat(f.dumpFilepath)
	if err != nil {
		return os.IsExist(err)
	}
	return true
}

func (f *StableBloomFilter) isGzSource() bool {
	ext := filepath.Ext(f.sourceFilepath)
	return ext == ".gz"
}

func (f *StableBloomFilter) getLineCount() int {
	if f.isGzSource() {
		return lineCounterGz(f.sourceFilepath)
	}
	return lineCounterPlain(f.sourceFilepath)
}

func (f *StableBloomFilter) bootstrap() {
	filename := f.sourceFilepath
	f.mux.Lock()
	defer f.mux.Unlock()

	file, err := os.Open(filename)
	if err != nil {
		f.LogCh <- utils.LogEvent{Level: zerolog.ErrorLevel, Name: bootstrapName, Msg: fmt.Sprintf("Load from file err: %v", err)}
		return
	}

	defer file.Close()

	added, scanned := 0, 0
	var scanner *bufio.Scanner

	if f.isGzSource() {
		gz, errs := gzip.NewReader(file)
		if errs != nil {
			log.Error().Err(errs).Send()
		}
		defer gz.Close()
		f.LogCh <- utils.LogEvent{
			Level: zerolog.InfoLevel,
			Name:  bootstrapName,
			Msg:   "Gzip detected",
		}
		scanner = bufio.NewScanner(gz)
	} else {
		// todo maybe implemented progressbar
		scanner = bufio.NewScanner(file)
	}

	lineCount := f.getLineCount()

	for scanner.Scan() {
		scanned++
		if !f.SBF.TestAndAdd(scanner.Bytes()) {
			added++
			f.needCheckpoint = false
			f.LogCh <- utils.LogEvent{Level: zerolog.InfoLevel, Name: "add", Count: 1.0}
			if added%10_000_000 == 0 {
				f.LogCh <- utils.LogEvent{
					Level: zerolog.InfoLevel,
					Name:  bootstrapName,
					Msg:   fmt.Sprintf("Добавлено: %s из [%s]", utils.HumInt(added), utils.HumInt(lineCount)),
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
