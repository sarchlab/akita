package addresstranslator

import (
	"fmt"
	"strconv"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// AddressTranslatorVisTracer traces the AddressTranslator visualization
type AddressTranslatorVisTracer struct {
	*tracing.DBTracer
	translator *Comp
	timeTeller sim.TimeTeller
}

// NewAddressTranslatorVisTracer creates a new AddressTranslator visualization tracer
func NewAddressTranslatorVisTracer(
	timeTeller sim.TimeTeller,
	backend tracing.Tracer,
	translator *Comp,
) *AddressTranslatorVisTracer {
	t := &AddressTranslatorVisTracer{
		DBTracer:   backend.(*tracing.DBTracer),
		translator: translator,
		timeTeller: timeTeller,
	}
	return t
}

// StartTask overrides StartTask to trace port status
func (t *AddressTranslatorVisTracer) StartTask(task tracing.Task) {
	t.DBTracer.StartTask(task)
	t.AddMilestone(tracing.Milestone{
		ID:               strconv.FormatUint(tracing.GenerateMilestoneID(), 10),
		TaskID:           task.ID,
		BlockingCategory: "Port Status",
		BlockingReason:   "Task Enqueued",
		BlockingLocation: task.Where,
		Time:             float64(t.timeTeller.CurrentTime()),
	})
}

// OnPortUpdate is called when the port status is updated
func (t *AddressTranslatorVisTracer) OnPortUpdate(port sim.Port, item interface{}) {
	if item == nil {
		return
	}

	if req, ok := item.(mem.AccessReq); ok {
		taskID := tracing.MsgIDAtReceiver(req, t.translator)
		fmt.Printf("AddressTranslatorVisTracer: Port Update - Port: %s, TaskID: %s\n",
			port.Name(), taskID)

		t.AddMilestone(tracing.Milestone{
			ID:               strconv.FormatUint(tracing.GenerateMilestoneID(), 10),
			TaskID:           taskID,
			BlockingCategory: "Port Status",
			BlockingReason:   "Task Front of Port",
			BlockingLocation: port.Name(),
			Time:             float64(t.timeTeller.CurrentTime()),
		})
	}
}
