package monitoring

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/sim/queueing"
)

type cacheBufferInventory struct {
	State struct {
		Directory cache.DirectoryState
		Queue     queueing.Buffer[int]
	}
}

func (*cacheBufferInventory) Name() string { return "Cache" }

// Directory entries do not contain buffers. Inspection cost should depend on
// the actual buffers, not the number of cache lines simulated.
func BenchmarkBufferInventoryCacheDirectory(b *testing.B) {
	for _, lines := range []int{32, 32768} {
		b.Run(fmt.Sprintf("lines=%d", lines), func(b *testing.B) {
			c := &cacheBufferInventory{}
			cache.DirectoryReset(&c.State.Directory, lines/8, 8, 64)
			c.State.Queue = queueing.MakeBuffer[int](7)
			m := &Monitor{components: []Component{c}}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				buffers := m.collectBuffers()
				if len(buffers) != 1 || buffers[0].Capacity() != 7 {
					b.Fatal("lost the State queue while inspecting cache data")
				}
			}
		})
	}
}
