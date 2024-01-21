package bloom

import (
	"fmt"
	"testing"

	boom "github.com/tylertreat/BoomFilters"

	"bloom-du/internal/utils"
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

			for i := 0; i < 50000; i++ {
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
			logCh := make(chan utils.LogEvent, 5)
			filter := NewStableBloomFilter("", false, logCh)

			for i := 0; i < 50000; i++ {
				value := fmt.Sprintf("test_%d", i)
				filter.Add(value)
			}

			if err := filter.Test(test.value); (!err) != test.wantErr {
				t.Errorf("filter.Test() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
