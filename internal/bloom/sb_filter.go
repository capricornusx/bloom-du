package bloom

import (
	"bufio"
	"log"
	"os"
	"sync"

	"github.com/dustin/go-humanize"
	boom "github.com/tylertreat/BoomFilters"
)

type BFilter struct {
	BF             *boom.BloomFilter
	dumpFilepath   string
	mux            sync.RWMutex
	needCheckpoint bool // true if new element added. False if not AND last checkpoint success
}

// Create creating and bootstrap SBF from struct file if exist OR loading text data as source
func Create(sourceFile string) *BFilter {
	defaultBf := boom.NewBloomFilter(1_000_000_000, 0.0001)

	filter := BFilter{BF: defaultBf, dumpFilepath: "sbData.bloom"}
	return filter.Boostrap(sourceFile)
}

func (f *BFilter) Add(value string) {
	f.BF.Add([]byte(value))
	f.needCheckpoint = true
}

func (f *BFilter) Test(value string) bool {
	return f.BF.Test([]byte(value))
}

func (f *BFilter) TestAndAdd(value string) bool {
	result := f.BF.TestAndAdd([]byte(value))
	if !result {
		f.needCheckpoint = true
	}
	return result
}

func (f *BFilter) Checkpoint() {
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
	_, err = f.BF.WriteTo(file)
	if err != nil {
		log.Printf("Error to save Checkpoint: %v", err)
	}
	f.needCheckpoint = false
}

func (f *BFilter) loadStructFromFile() (int64, error) {
	f.mux.Lock()
	defer f.mux.Unlock()
	file, err := os.OpenFile(f.dumpFilepath, os.O_RDONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	numBytes, err := f.BF.ReadFrom(file)
	f.PrintLogStat()

	return numBytes, err
}

func (f *BFilter) Boostrap(sourceFile string) *BFilter {
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

func (f *BFilter) PrintLogStat() {
	// StablePoint returns the limit of the expected fraction of zeros in the Filter
	log.Printf("[Capacity:%d] [K: %d] EstimatedFillRatio: %f\n",
		f.BF.Capacity(),
		f.BF.K(),
		f.BF.EstimatedFillRatio(),
	)
}

func (f *BFilter) bootstrap(filename string) {
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
		f.BF.Add(scanner.Bytes())
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
func (f *BFilter) printCounter(counter int) {
	log.Printf("Добавлено: %s\n", humanize.FormatInteger(integerFormat, counter))
}
