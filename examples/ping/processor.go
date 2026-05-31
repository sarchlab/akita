package ping

import (
	"fmt"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// PingReq is a ping request message.
type PingReq struct {
	messaging.MsgMeta
	SeqID int
}

// PingRsp is a ping response message.
type PingRsp struct {
	messaging.MsgMeta
	SeqID int
}

// PingProcessor implements modeling.EventProcessor[PingSpec, PingState, modeling.None].
type PingProcessor struct{}

// Process handles all ping logic: sending scheduled pings, delivering
// matured responses, and processing incoming messages.
func (p *PingProcessor) Process(
	comp *modeling.EventDrivenComponent[PingSpec, PingState, modeling.None],
	now timing.VTimeInSec,
) bool {
	progress := false
	state := &comp.State
	spec := comp.Spec

	progress = p.sendScheduledPings(comp, state, spec, now) || progress
	progress = p.deliverPendingResponses(comp, state, spec, now) || progress
	progress = p.processIncoming(comp, state, spec, now) || progress

	return progress
}

func (p *PingProcessor) sendScheduledPings(
	comp *modeling.EventDrivenComponent[PingSpec, PingState, modeling.None],
	state *PingState,
	spec PingSpec,
	now timing.VTimeInSec,
) bool {
	progress := false
	remaining := make([]ScheduledPing, 0, len(state.ScheduledPings))

	for _, sp := range state.ScheduledPings {
		if sp.SendAt <= now {
			pingMsg := &PingReq{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: spec.OutPort.AsRemote(),
					Dst: sp.Dst,
				},
				SeqID: state.NextSeqID,
			}

			spec.OutPort.Send(pingMsg)

			state.StartTimes = append(state.StartTimes, now)
			state.NextSeqID++
			progress = true
		} else {
			remaining = append(remaining, sp)
			comp.ScheduleWakeAt(sp.SendAt)
		}
	}

	state.ScheduledPings = remaining

	return progress
}

func (p *PingProcessor) deliverPendingResponses(
	comp *modeling.EventDrivenComponent[PingSpec, PingState, modeling.None],
	state *PingState,
	spec PingSpec,
	now timing.VTimeInSec,
) bool {
	progress := false
	remaining := make([]PendingResponse, 0, len(state.PendingResponses))

	for _, pr := range state.PendingResponses {
		if pr.DeliverAt <= now {
			rsp := &PingRsp{
				MsgMeta: messaging.MsgMeta{
					ID:    timing.GetIDGenerator().Generate(),
					Src:   spec.OutPort.AsRemote(),
					Dst:   pr.Dst,
					RspTo: pr.OrigMsgID,
				},
				SeqID: pr.SeqID,
			}

			spec.OutPort.Send(rsp)
			progress = true
		} else {
			remaining = append(remaining, pr)
			comp.ScheduleWakeAt(pr.DeliverAt)
		}
	}

	state.PendingResponses = remaining

	return progress
}

func (p *PingProcessor) processIncoming(
	comp *modeling.EventDrivenComponent[PingSpec, PingState, modeling.None],
	state *PingState,
	spec PingSpec,
	now timing.VTimeInSec,
) bool {
	progress := false

	for {
		msg := spec.OutPort.RetrieveIncoming()
		if msg == nil {
			break
		}

		switch m := msg.(type) {
		case *PingReq:
			state.PendingResponses = append(state.PendingResponses,
				PendingResponse{
					DeliverAt: now + 2_000_000_000_000,
					Dst:       m.Src,
					OrigMsgID: m.Meta().ID,
					SeqID:     m.SeqID,
				})
			comp.ScheduleWakeAt(now + 2_000_000_000_000)
			progress = true
		case *PingRsp:
			seqID := m.SeqID
			startTime := state.StartTimes[seqID]
			duration := now - startTime

			fmt.Printf("Ping %d, %d ps\n", seqID, duration)
			progress = true
		}
	}

	return progress
}
