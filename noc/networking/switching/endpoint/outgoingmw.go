package endpoint

import (
	"math"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/packetization"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"

	// msgMetaToFlits converts a MsgMeta into a slice of packetization.Flit entries.
	"github.com/sarchlab/akita/v5/messaging"
)

func msgMetaToFlits(
	meta messaging.MsgMeta,
	spec Spec,
	networkPortRemote messaging.RemotePort,
	defaultSwitchDst messaging.RemotePort,
	msgTaskID uint64,
) []packetization.Flit {
	numFlit := 1
	if meta.TrafficBytes > 0 {
		trafficByte := meta.TrafficBytes
		trafficByte += int(math.Ceil(
			float64(trafficByte) * spec.EncodingOverhead))
		numFlit = (trafficByte-1)/spec.FlitByteSize + 1
	}

	flits := make([]packetization.Flit, numFlit)
	for i := 0; i < numFlit; i++ {
		flits[i] = packetization.Flit{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: networkPortRemote,
				Dst: defaultSwitchDst,
			},
			SeqID:        i,
			NumFlitInMsg: numFlit,
			Msg: messaging.MsgMeta{
				ID:           meta.ID,
				Src:          meta.Src,
				Dst:          meta.Dst,
				RspTo:        meta.RspTo,
				TrafficClass: meta.TrafficClass,
				TrafficBytes: meta.TrafficBytes,
			},
			MsgTaskID: msgTaskID,
		}
	}

	return flits
}

// outgoingMW handles the device→network path:
// sendFlitOut, prepareMsg, prepareFlits.
type outgoingMW struct {
	comp             *modeling.Component[Spec, State, modeling.None]
	devicePorts      []messaging.Port
	defaultSwitchDst messaging.RemotePort
}

// networkPort resolves the endpoint's network port by name. The instance is
// assigned externally after Build, so it is resolved lazily.
func (m *outgoingMW) networkPort() messaging.Port {
	return m.comp.GetPortByName("NetworkPort")
}

// Tick runs the outgoing stages.
func (m *outgoingMW) Tick() bool {
	madeProgress := false

	madeProgress = m.sendFlitOut() || madeProgress
	madeProgress = m.prepareMsg() || madeProgress
	madeProgress = m.prepareFlits() || madeProgress

	return madeProgress
}

func (m *outgoingMW) sendFlitOut() bool {
	madeProgress := false
	spec := m.comp.Spec()
	state := &m.comp.State

	numSent := 0

	for i := 0; i < spec.NumOutputChannels; i++ {
		if numSent >= len(state.FlitsToSend) {
			break
		}

		flit := state.FlitsToSend[numSent]

		if !m.networkPort().CanSend() {
			break
		}

		m.networkPort().Send(flit)

		// The flit waited in FlitsToSend until the outgoing network port could
		// accept it; charge that span to the flit_e2e task.
		if m.comp.NumHooks() > 0 {
			tracing.AddMilestone(m.comp, tracing.Milestone{
				TaskID: flit.MsgMeta.ID,
				Kind:   tracing.MilestoneKindNetworkBusy,
				What:   m.comp.Name() + ".NetworkPort",
			})
		}

		numSent++
		madeProgress = true
	}

	if numSent > 0 {
		state.FlitsToSend = state.FlitsToSend[numSent:]

		if len(state.FlitsToSend) == 0 {
			for _, p := range m.devicePorts {
				p.NotifyAvailable()
			}
		}
	}

	return madeProgress
}

// maxMsgOutBufSize limits the number of messages buffered before flit
// conversion. This prevents the serialisable state from growing
// unboundedly.
const maxMsgOutBufSize = 16

func (m *outgoingMW) prepareMsg() bool {
	madeProgress := false
	state := &m.comp.State

	for i := 0; i < len(m.devicePorts); i++ {
		// Backpressure: stop accepting new messages when the outgoing
		// message buffer is already large enough.
		if len(state.MsgOutBuf) >= maxMsgOutBufSize {
			break
		}

		port := m.devicePorts[i]
		if port.PeekOutgoing() == nil {
			continue
		}

		msg := port.RetrieveOutgoing()
		state.MsgOutBuf = append(state.MsgOutBuf, msg.Meta())

		madeProgress = true
	}

	return madeProgress
}

// maxFlitsToBuffer limits the number of flits held in FlitsToSend at once.
// This prevents the serialisable state from growing unboundedly.
const maxFlitsToBuffer = 64

func (m *outgoingMW) prepareFlits() bool {
	madeProgress := false
	spec := m.comp.Spec()
	state := &m.comp.State
	networkPortRemote := m.networkPort().AsRemote()

	for {
		if len(state.MsgOutBuf) == 0 {
			return madeProgress
		}

		// Apply backpressure: don't convert more messages to flits while
		// the flit send buffer is already large.
		if len(state.FlitsToSend) >= maxFlitsToBuffer {
			return madeProgress
		}

		meta := state.MsgOutBuf[0]
		state.MsgOutBuf = state.MsgOutBuf[1:]

		// One msg_e2e task per message. Its ID is generated here (unique, not
		// meta.ID — a sender's req_out already owns meta.ID in an integrated
		// simulation), travels in every flit as MsgTaskID, and is the parent of
		// each per-flit flit_e2e task. It is parented to the message's own ID so
		// it nests under that req_out when one exists.
		msgTaskID := timing.GetIDGenerator().Generate()
		flits := msgMetaToFlits(
			meta, spec, networkPortRemote, m.defaultSwitchDst, msgTaskID)

		state.FlitsToSend = append(state.FlitsToSend, flits...)

		m.logMsgE2EStart(meta, msgTaskID)
		for _, fs := range flits {
			m.logFlitE2ETask(fs, false, &meta, msgTaskID)
		}

		madeProgress = true
	}
}

// logMsgE2EStart opens the per-message msg_e2e task that the receiving endpoint
// closes once the message is reassembled. It is the parent of the message's
// flit_e2e tasks.
func (m *outgoingMW) logMsgE2EStart(meta messaging.MsgMeta, msgTaskID uint64) {
	if m.comp.NumHooks() == 0 {
		return
	}

	tracing.StartTask(m.comp, tracing.TaskStart{
		ID:       msgTaskID,
		ParentID: meta.ID,
		Kind:     "msg_e2e",
		What:     "msg_e2e",
		Detail:   meta,
	})
}

func (m *outgoingMW) logFlitE2ETask(
	fs packetization.Flit, isEnd bool, meta *messaging.MsgMeta, msgE2ETaskID uint64,
) {
	if m.comp.NumHooks() == 0 {
		return
	}

	if isEnd {
		tracing.EndTask(m.comp, tracing.TaskEnd{ID: fs.MsgMeta.ID})
		return
	}

	flit := packetization.Flit{
		MsgMeta:      fs.MsgMeta,
		SeqID:        fs.SeqID,
		NumFlitInMsg: fs.NumFlitInMsg,
		Msg:          *meta,
	}

	tracing.StartTask(m.comp, tracing.TaskStart{
		ID:       fs.MsgMeta.ID,
		ParentID: msgE2ETaskID,
		Kind:     "flit_e2e",
		What:     "flit_e2e",
		Location: m.comp.Name() + ".FlitBuf",
		Detail:   flit,
	})
}
