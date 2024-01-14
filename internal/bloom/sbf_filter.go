package bloom

import (
	"bufio"
	"log"
	"os"
	"sync"

	"github.com/dustin/go-humanize"
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
	defaultSbf := boom.NewDefaultStableBloomFilter(M, fpRate)

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

func (f *StableBloomFilter) Checkpoint() {
	if !f.needCheckpoint {
		log.Println("Checkpoint is not necessary now.")
		return
	}

	file, err := os.OpenFile(f.dumpFilepath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Panic(err)
	}
	defer file.Close()

	f.mux.Lock()
	defer f.mux.Unlock()
	_, err = f.SBF.WriteTo(file)
	if err != nil {
		log.Printf("Error to save Checkpoint: %v", err)
	}
	f.needCheckpoint = false
}

func (f *StableBloomFilter) loadStructFromFile() (int64, error) {
	f.mux.Lock()
	defer f.mux.Unlock()
	file, err := os.OpenFile(f.dumpFilepath, os.O_RDONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	numBytes, err := f.SBF.ReadFrom(file)
	f.PrintLogStat()

	return numBytes, err
}

func (f *StableBloomFilter) Boostrap(sourceFile string) *StableBloomFilter {
	if sourceFile == "" {
		log.Println("Start empty filter")
		return f
	}

	_, err := os.Stat(f.dumpFilepath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Load bootstrap data from %s!", sourceFile)
			f.bootstrap(sourceFile)
		}
	} else {
		log.Printf("[%s] exist! Skip boostrap!", f.dumpFilepath)
		_, err = f.loadStructFromFile()
		if err != nil {
			return f
		}
	}
	return f
}

func (f *StableBloomFilter) PrintLogStat() {
	// StablePoint returns the limit of the expected fraction of zeros in the Filter
	log.Printf("[P:%d] [K: %d] Cells: %s, Stable point: %f\n",
		f.SBF.P(),
		f.SBF.K(),
		humanize.FormatInteger(integerFormat, int(f.SBF.Cells())),
		f.SBF.StablePoint(),
	)
}

func (f *StableBloomFilter) bootstrap(filename string) {
	f.mux.Lock()
	defer f.mux.Unlock()

	file, err := os.Open(filename)
	if err != nil {
		log.Panic(err)
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
		log.Panic(err)
	}

	f.needCheckpoint = true
	f.PrintLogStat()
	f.printCounter(counter)
}

// printCounter P returns the number of cells decremented on every add.
func (f *StableBloomFilter) printCounter(counter int) {
	log.Printf("Добавлено: %s\n", humanize.FormatInteger(integerFormat, counter))
}
