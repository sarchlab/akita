package modeling

import (
	"bytes"
	"encoding/gob"
	"testing"
)

// Representative complex State struct mimicking a writeback cache state
// with nested slices, maps, and embedded structs.

type benchCacheLine struct {
	Tag       uint64
	SetID     uint32
	WayID     uint32
	Data      [64]byte
	Dirty     bool
	Valid     bool
	ReadCount int
}

type benchMSHREntry struct {
	Address    uint64
	CycleAdded uint64
	Waiting    []uint64
}

type benchState struct {
	CacheLines   []benchCacheLine
	MSHR         []benchMSHREntry
	TagLookup    map[uint64]int
	PendingReads map[uint64][]uint64
	Stats        benchStats
	Cycle        uint64
	Name         string
}

type benchStats struct {
	Hits       uint64
	Misses     uint64
	Evictions  uint64
	WriteBacks uint64
}

func makeBenchState() benchState {
	state := benchState{
		CacheLines: make([]benchCacheLine, 256),
		MSHR:       make([]benchMSHREntry, 16),
		TagLookup:  make(map[uint64]int, 256),
		PendingReads: make(map[uint64][]uint64, 8),
		Stats: benchStats{
			Hits:       12345,
			Misses:     678,
			Evictions:  90,
			WriteBacks: 42,
		},
		Cycle: 100000,
		Name:  "L1Cache",
	}

	for i := range state.CacheLines {
		state.CacheLines[i] = benchCacheLine{
			Tag:       uint64(i) * 64,
			SetID:     uint32(i / 4),
			WayID:     uint32(i % 4),
			Dirty:     i%3 == 0,
			Valid:     true,
			ReadCount: i,
		}
		for j := range state.CacheLines[i].Data {
			state.CacheLines[i].Data[j] = byte((i + j) % 256)
		}
		state.TagLookup[uint64(i)*64] = i
	}

	for i := range state.MSHR {
		state.MSHR[i] = benchMSHREntry{
			Address:    uint64(i) * 4096,
			CycleAdded: uint64(99000 + i),
			Waiting:    []uint64{uint64(i), uint64(i + 1), uint64(i + 2)},
		}
	}

	for i := 0; i < 8; i++ {
		state.PendingReads[uint64(i)*4096] = []uint64{
			uint64(i * 10), uint64(i*10 + 1),
		}
	}

	return state
}

// gobDeepCopy is the old gob-based implementation kept for benchmarking comparison.
func gobDeepCopy[T any](src T) T {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&src); err != nil {
		panic("gobDeepCopy: encode failed: " + err.Error())
	}

	var dst T
	if err := gob.NewDecoder(&buf).Decode(&dst); err != nil {
		panic("gobDeepCopy: decode failed: " + err.Error())
	}

	return dst
}

func BenchmarkDeepCopyReflect(b *testing.B) {
	state := makeBenchState()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = deepCopy(state)
	}
}

func BenchmarkDeepCopyGob(b *testing.B) {
	state := makeBenchState()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = gobDeepCopy(state)
	}
}
