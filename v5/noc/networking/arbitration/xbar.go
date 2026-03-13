package arbitration

import (
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/queueing"
)

// FlitBuffer is a buffer that can peek at its first flit element.
// This is used by the XBar arbiter to inspect destination routing.
type FlitBuffer interface {
	queueing.BufferState
	PeekFlit() *messaging.Flit
}

// NewXBarArbiter creates a new XBar arbiter.
func NewXBarArbiter() Arbiter {
	return &xbarArbiter{}
}

type xbarArbiter struct {
	buffers    []queueing.BufferState
	nextPortID int
}

func (a *xbarArbiter) AddBuffer(buf queueing.BufferState) {
	a.buffers = append(a.buffers, buf)
}

func (a *xbarArbiter) Arbitrate() []queueing.BufferState {
	startingPortID := a.nextPortID
	selectedPort := make([]queueing.BufferState, 0)
	occupiedOutputPort := make(map[int]bool)

	for i := 0; i < len(a.buffers); i++ {
		currPortID := (startingPortID + i) % len(a.buffers)
		buf := a.buffers[currPortID]

		if buf.Size() == 0 {
			continue
		}

		flitBuf, ok := buf.(FlitBuffer)
		if !ok {
			// If the buffer doesn't implement FlitBuffer, skip it.
			continue
		}

		flit := flitBuf.PeekFlit()
		if flit == nil {
			continue
		}

		if occupiedOutputPort[flit.OutputBufIdx] {
			continue
		}

		selectedPort = append(selectedPort, buf)
		occupiedOutputPort[flit.OutputBufIdx] = true
	}

	a.nextPortID = (a.nextPortID + 1) % len(a.buffers)

	return selectedPort
}
