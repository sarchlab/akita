package ping

import (
	"fmt"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// pingReq is a ping request message.
type pingReq struct {
	SeqID int
}

// pingRsp is a ping response message.
type pingRsp struct {
	SeqID int
}

// pingMW sends scheduled pings, delivers matured responses, and processes
// incoming messages.
type pingMW struct {
	comp *Comp
}

// Handle runs on every wakeup. Work that is not due yet asks for a wakeup at
// its time.
func (m *pingMW) Handle(e timing.Event) bool {
	now := e.Time()
	progress := false

	progress = m.sendScheduledPings(now) || progress
	progress = m.deliverPendingResponses(now) || progress
	progress = m.processIncoming(now) || progress

	return progress
}

func (m *pingMW) sendScheduledPings(now timing.VTimeInPicoSec) bool {
	state := &m.comp.State
	out := m.comp.Ports.Out
	progress := false
	remaining := make([]scheduledPing, 0, len(state.ScheduledPings))

	for _, sp := range state.ScheduledPings {
		if sp.SendAt > now {
			remaining = append(remaining, sp)
			m.comp.WakeAt(sp.SendAt)
			continue
		}

		if !out.CanSend() {
			remaining = append(remaining, sp)
			continue
		}

		pingMsg := messaging.Msg{ID: m.comp.NewID(),
			Src: out.AsRemote(),
			Dst: sp.Dst, Payload: pingReq{
				SeqID: state.NextSeqID,
			}}

		out.Send(pingMsg)

		state.StartTimes = append(state.StartTimes, now)
		state.NextSeqID++
		progress = true
	}

	state.ScheduledPings = remaining

	return progress
}

func (m *pingMW) deliverPendingResponses(now timing.VTimeInPicoSec) bool {
	state := &m.comp.State
	out := m.comp.Ports.Out
	progress := false
	remaining := make([]pendingResponse, 0, len(state.PendingResponses))

	for _, pr := range state.PendingResponses {
		if pr.DeliverAt > now {
			remaining = append(remaining, pr)
			m.comp.WakeAt(pr.DeliverAt)
			continue
		}

		if !out.CanSend() {
			remaining = append(remaining, pr)
			continue
		}

		rsp := messaging.Msg{ID: m.comp.NewID(),
			Src:   out.AsRemote(),
			Dst:   pr.Dst,
			RspTo: pr.OrigMsgID, Payload: pingRsp{
				SeqID: pr.SeqID,
			}}

		out.Send(rsp)
		progress = true
	}

	state.PendingResponses = remaining

	return progress
}

func (m *pingMW) processIncoming(now timing.VTimeInPicoSec) bool {
	state := &m.comp.State
	progress := false

	for {
		msg, ok := m.comp.Ports.Out.RetrieveIncoming()
		if !ok {
			break
		}

		switch content := msg.Payload.(type) {
		case pingReq:

			state.PendingResponses = append(state.PendingResponses,
				pendingResponse{
					DeliverAt: now + 2_000_000_000_000,
					Dst:       msg.Src,
					OrigMsgID: msg.ID,
					SeqID:     content.SeqID,
				})
			m.comp.WakeAt(now + 2_000_000_000_000)
			progress = true
		case pingRsp:

			seqID := content.SeqID
			startTime := state.StartTimes[seqID]
			duration := now - startTime

			fmt.Printf("Ping %d, %d ps\n", seqID, duration)
			progress = true
		}
	}

	return progress
}

var _ = messaging.DefineProtocol(messaging.RoleDef{Name: "peer", Sends: []any{pingReq{}, pingRsp{}}})
