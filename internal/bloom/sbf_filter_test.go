package bloom

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"

	boom "github.com/tylertreat/BoomFilters"
)

func TestTestAddValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   []byte
		want    bool
		wantErr bool
	}{
		{"Test 1", []byte("test_0"), true, false},
		{"Test 2", []byte("test_1"), true, false},
		{"Test 3", []byte("test_10000"), true, false},
		{"Test 4", []byte("test_20024"), true, false},
		{"Test 5", []byte("sample value"), false, true},
	}

	for _, tt := range tests {
		test := tt
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			filter := boom.NewDefaultStableBloomFilter(M, fpRate)

			for i := 0; i < 30_000; i++ {
				value := fmt.Sprintf("test_%d", i)
				filter.Add([]byte(value))
			}

			if err := filter.Test(test.value); (!err) != test.wantErr {
				t.Errorf("filter.Test() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestTestAddValue2(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		want    bool
		wantErr bool
	}{
		{"Test 1", "test_0", true, false},
		{"Test 2", "test_1", true, false},
		{"Test 3", "test_10000", true, false},
		{"Test 4", "test_20024", true, false},
		{"Test 5", "sample value", false, true},
	}

	for _, tt := range tests {
		test := tt
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			logCh := make(chan LogEvent, 50)
			filter := NewStableBloomFilter("", false, logCh, "")

			for i := 0; i < 30_000; i++ {
				value := fmt.Sprintf("test_%d", i)
				filter.Add(value)
			}

			if err := filter.Test(test.value); (!err) != test.wantErr {
				t.Errorf("filter.Test() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestGetDumpSize(t *testing.T) {
	t.Parallel()
	filePath := "test.file"
	file, _ := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)

	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	defer func(name string) {
		_ = os.Remove(name)
	}(filePath)

	_ = binary.Write(
		file,
		binary.BigEndian,
		[]byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Vivamus sit amet neque ac lorem dapibus ac."),
	)

	f := StableBloomFilter{dumpFilepath: filePath}

	size := f.GetDumpSize()

	if size != 100 {
		t.Errorf("Expected size 100, got %v", size)
	}
}

func TestLineCounter(t *testing.T) {
	t.Parallel()
	filePath := "!test.file"
	file, _ := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)

	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	defer func(name string) {
		_ = os.Remove(name)
	}(filePath)

	_ = binary.Write(
		file,
		binary.BigEndian,
		[]byte("One\nTwo\nThree\nFour\nFive\n"),
	)

	f := StableBloomFilter{sourceFilepath: filePath}

	count := f.getLineCount()

	if count != 5 {
		t.Errorf("Expected size 5, got %v", count)
	}
}

func TestEngine(t *testing.T) {
	t.Parallel()
	f := StableBloomFilter{}
	if f.Engine() != StableBloom {
		t.Errorf("Expected Engine StableBloom, got %v", f.Engine())
	}
}
