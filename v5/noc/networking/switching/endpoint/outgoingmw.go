package endpoint

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/messaging"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// msgMetaToFlits converts a MsgMeta into a slice of messaging.Flit entries.
func msgMetaToFlits(
	meta sim.MsgMeta,
	spec Spec,
	networkPortRemote sim.RemotePort,
	defaultSwitchDst sim.RemotePort,
) []messaging.Flit {
	numFlit := 1
	if meta.TrafficBytes > 0 {
		trafficByte := meta.TrafficBytes
		trafficByte += int(math.Ceil(
			float64(trafficByte) * spec.EncodingOverhead))
		numFlit = (trafficByte-1)/spec.FlitByteSize + 1
	}

	flits := make([]messaging.Flit, numFlit)
	for i := 0; i < numFlit; i++ {
		flits[i] = messaging.Flit{
			MsgMeta: sim.MsgMeta{
				ID:  fmt.Sprintf("flit-%d-msg-%s-%s", i, meta.ID, sim.GetIDGenerator().Generate()),
				Src: networkPortRemote,
				Dst: defaultSwitchDst,
			},
			SeqID:        i,
			NumFlitInMsg: numFlit,
			Msg: sim.MsgMeta{
				ID:           meta.ID,
				Src:          meta.Src,
				Dst:          meta.Dst,
				RspTo:        meta.RspTo,
				TrafficClass: meta.TrafficClass,
				TrafficBytes: meta.TrafficBytes,
			},
		}
	}

	return flits
}

// outgoingMW handles the device→network path:
// sendFlitOut, prepareMsg, prepareFlits.
type outgoingMW struct {
	comp             *modeling.Component[Spec, State]
	devicePorts      []sim.Port
	networkPort      sim.Port
	defaultSwitchDst sim.RemotePort
}

// Tick runs the outgoing stages.
func (m *outgoingMW) Tick() bool {
	madeProgress := false

	madeProgress = m.sendFlitOut() || madeProgress
	madeProgress = m.prepareMsg() || madeProgress
	madeProgress = m.prepareFlits() || madeProgress

	return madeProgress
}

func (m *outgoingMW) msgTaskID(msgID string) string {
	return fmt.Sprintf("msg_%s_e2e", msgID)
}

func (m *outgoingMW) flitTaskID(flitID string) string {
	return fmt.Sprintf("%s_e2e", flitID)
}

func (m *outgoingMW) sendFlitOut() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	state := m.comp.GetNextState()

	numSent := 0

	for i := 0; i < spec.NumOutputChannels; i++ {
		if numSent >= len(state.FlitsToSend) {
			break
		}

		flit := &state.FlitsToSend[numSent]

		err := m.networkPort.Send(flit)
		if err == nil {
			numSent++
			madeProgress = true
		}
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
	state := m.comp.GetNextState()

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
		state.MsgOutBuf = append(state.MsgOutBuf, *msg.Meta())

		madeProgress = true
	}

	return madeProgress
}

// maxFlitsToBuffer limits the number of flits held in FlitsToSend at once.
// This prevents the serialisable state from growing unboundedly.
const maxFlitsToBuffer = 64

func (m *outgoingMW) prepareFlits() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	state := m.comp.GetNextState()
	networkPortRemote := m.networkPort.AsRemote()

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
		flits := msgMetaToFlits(meta, spec, networkPortRemote, m.defaultSwitchDst)
		state.FlitsToSend = append(state.FlitsToSend, flits...)

		for _, fs := range flits {
			m.logFlitE2ETask(fs, false, &meta)
		}

		madeProgress = true
	}
}

func (m *outgoingMW) logFlitE2ETask(
	fs messaging.Flit, isEnd bool, meta *sim.MsgMeta,
) {
	if m.comp.NumHooks() == 0 {
		return
	}

	if isEnd {
		tracing.EndTask(m.flitTaskID(fs.ID), m.comp)
		return
	}

	flit := &messaging.Flit{
		MsgMeta:      fs.MsgMeta,
		SeqID:        fs.SeqID,
		NumFlitInMsg: fs.NumFlitInMsg,
		Msg:          *meta,
	}

	tracing.StartTaskWithSpecificLocation(
		m.flitTaskID(fs.ID), m.msgTaskID(meta.ID),
		m.comp, "flit_e2e", "flit_e2e", m.comp.Name()+".FlitBuf", flit,
	)
}
