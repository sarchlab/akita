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

	var taskID string
	var msgType string

	switch msg := item.(type) {
	case mem.AccessReq:
		taskID = tracing.MsgIDAtReceiver(msg, t.translator)
		msgType = "Access Request"
	case mem.AccessRsp:
		taskID = tracing.MsgIDAtReceiver(msg, t.translator)
		msgType = "Access Response"
	case *mem.ControlMsg:
		taskID = msg.Meta().ID
		msgType = "Control Message"
	default:
		return
	}

	fmt.Printf("AddressTranslatorVisTracer: Port Update - Port: %s, TaskID: %s, Type: %s\n",
		port.Name(), taskID, msgType)

	t.AddMilestone(tracing.Milestone{
		ID:               strconv.FormatUint(tracing.GenerateMilestoneID(), 10),
		TaskID:           taskID,
		BlockingCategory: "Port Status",
		BlockingReason:   fmt.Sprintf("%s at %s", msgType, port.Name()),
		BlockingLocation: port.Name(),
		Time:             float64(t.timeTeller.CurrentTime()),
	})
}
