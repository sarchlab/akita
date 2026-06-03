package ping

import (
	"fmt"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// pingReq is a ping request message.
type pingReq struct {
	messaging.MsgMeta
	SeqID int
}

// pingRsp is a ping response message.
type pingRsp struct {
	messaging.MsgMeta
	SeqID int
}

// pingProcessor implements modeling.EventProcessor[Spec, State, modeling.None].
type pingProcessor struct{}

// outPort returns the component's internal "Out" port.
func outPort(
	comp *modeling.EventDrivenComponent[Spec, State, modeling.None],
) messaging.Port {
	return comp.GetPortByName("Out")
}

// Process handles all ping logic: sending scheduled pings, delivering
// matured responses, and processing incoming messages.
func (p *pingProcessor) Process(
	comp *modeling.EventDrivenComponent[Spec, State, modeling.None],
	now timing.VTimeInSec,
) bool {
	progress := false
	state := &comp.State

	progress = p.sendScheduledPings(comp, state, now) || progress
	progress = p.deliverPendingResponses(comp, state, now) || progress
	progress = p.processIncoming(comp, state, now) || progress

	return progress
}

func (p *pingProcessor) sendScheduledPings(
	comp *modeling.EventDrivenComponent[Spec, State, modeling.None],
	state *State,
	now timing.VTimeInSec,
) bool {
	progress := false
	remaining := make([]scheduledPing, 0, len(state.ScheduledPings))

	for _, sp := range state.ScheduledPings {
		if sp.SendAt <= now {
			pingMsg := pingReq{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: outPort(comp).AsRemote(),
					Dst: sp.Dst,
				},
				SeqID: state.NextSeqID,
			}

			outPort(comp).Send(pingMsg)

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

func (p *pingProcessor) deliverPendingResponses(
	comp *modeling.EventDrivenComponent[Spec, State, modeling.None],
	state *State,
	now timing.VTimeInSec,
) bool {
	progress := false
	remaining := make([]pendingResponse, 0, len(state.PendingResponses))

	for _, pr := range state.PendingResponses {
		if pr.DeliverAt <= now {
			rsp := pingRsp{
				MsgMeta: messaging.MsgMeta{
					ID:    timing.GetIDGenerator().Generate(),
					Src:   outPort(comp).AsRemote(),
					Dst:   pr.Dst,
					RspTo: pr.OrigMsgID,
				},
				SeqID: pr.SeqID,
			}

			outPort(comp).Send(rsp)
			progress = true
		} else {
			remaining = append(remaining, pr)
			comp.ScheduleWakeAt(pr.DeliverAt)
		}
	}

	state.PendingResponses = remaining

	return progress
}

func (p *pingProcessor) processIncoming(
	comp *modeling.EventDrivenComponent[Spec, State, modeling.None],
	state *State,
	now timing.VTimeInSec,
) bool {
	progress := false

	for {
		msg := outPort(comp).RetrieveIncoming()
		if msg == nil {
			break
		}

		switch m := msg.(type) {
		case pingReq:
			state.PendingResponses = append(state.PendingResponses,
				pendingResponse{
					DeliverAt: now + 2_000_000_000_000,
					Dst:       m.Src,
					OrigMsgID: m.Meta().ID,
					SeqID:     m.SeqID,
				})
			comp.ScheduleWakeAt(now + 2_000_000_000_000)
			progress = true
		case pingRsp:
			seqID := m.SeqID
			startTime := state.StartTimes[seqID]
			duration := now - startTime

			fmt.Printf("Ping %d, %d ps\n", seqID, duration)
			progress = true
		}
	}

	return progress
}
