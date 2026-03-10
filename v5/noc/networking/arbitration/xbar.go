package arbitration

import (
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// NewXBarArbiter creates a new XBar arbiter.
func NewXBarArbiter() Arbiter {
	return &xbarArbiter{}
}

type xbarArbiter struct {
	buffers    []queueing.Buffer
	nextPortID int
}

func (a *xbarArbiter) AddBuffer(buf queueing.Buffer) {
	a.buffers = append(a.buffers, buf)
}

func (a *xbarArbiter) Arbitrate() []queueing.Buffer {
	startingPortID := a.nextPortID
	selectedPort := make([]queueing.Buffer, 0)
	occupiedOutputPort := make(map[queueing.Buffer]bool)

	for i := 0; i < len(a.buffers); i++ {
		currPortID := (startingPortID + i) % len(a.buffers)
		buf := a.buffers[currPortID]
		item := buf.Peek()

		if item == nil {
			continue
		}

		flitMsg := item.(*sim.GenericMsg)
		flitPayload := sim.MsgPayload[messaging.FlitPayload](flitMsg)
		if _, ok := occupiedOutputPort[flitPayload.OutputBuf]; ok {
			continue
		}

		selectedPort = append(selectedPort, buf)
		occupiedOutputPort[flitPayload.OutputBuf] = true
	}

	a.nextPortID = (a.nextPortID + 1) % len(a.buffers)

	return selectedPort
}
