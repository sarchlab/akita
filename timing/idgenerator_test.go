package timing

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEngineIDNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func() Engine
	}{
		{"serial", func() Engine { return NewSerialEngine() }},
		{"parallel", func() Engine { return NewParallelEngine() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := tc.build(), tc.build()
			require.NotSame(t, a.GetIDGenerator(), b.GetIDGenerator())
			const workers, perWorker = 8, 1024
			var wg sync.WaitGroup
			results := make([][]uint64, workers)
			for worker := range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					results[worker] = make([]uint64, perWorker)
					for i := range perWorker {
						results[worker][i] = a.NewID()
					}
				}()
			}
			wg.Wait()
			seen := make(map[uint64]bool)
			for _, batch := range results {
				for _, id := range batch {
					require.NotZero(t, id)
					require.False(t, seen[id], "duplicate ID %d", id)
					seen[id] = true
				}
			}
			require.Len(t, seen, workers*perWorker)
			require.Equal(t, uint64(workers*perWorker+1), a.NewID())
			require.Equal(t, uint64(1), b.NewID())
			require.Equal(t, uint64(2), b.GetIDGenerator().NewID())
		})
	}
}

func BenchmarkIDGeneratorNewID(b *testing.B) {
	var ids IDGenerator
	b.ReportAllocs()
	for b.Loop() {
		ids.NewID()
	}
}
