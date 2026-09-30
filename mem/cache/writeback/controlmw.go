package writeback

import (
	"github.com/sarchlab/akita/v5/timing"
)

// controlMW runs the flusher, which takes Flush commands from the Control
// port and walks the cache state through the flush.
type controlMW struct {
	comp    *Comp
	flusher *flusher
}

// Handle runs the flusher.
func (m *controlMW) Handle(_ timing.Event) bool {
	return m.flusher.Tick()
}
