package memaccessagent

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/daisen"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

type agentMiddleware struct {
	comp *Comp

	// writeProgressBar and readProgressBar observe the run. CreateProgressBars
	// attaches them after Build; they are not simulation state.
	writeProgressBar *daisen.ProgressBar
	readProgressBar  *daisen.ProgressBar
}

func (m *agentMiddleware) memPort() messaging.Port {
	return m.comp.Ports.Mem
}

func (m *agentMiddleware) lowModule() messaging.Port {
	return m.comp.Resources.LowModule
}

// Handle updates the states of the agent and issues new read and write
// requests.
func (m *agentMiddleware) Handle(_ timing.Event) bool {
	madeProgress := false

	madeProgress = m.processMsgRsp() || madeProgress

	state := &m.comp.State
	if state.ReadLeft == 0 && state.WriteLeft == 0 {
		return madeProgress
	}

	if m.shouldRead() {
		madeProgress = m.doRead() || madeProgress
	} else {
		madeProgress = m.doWrite() || madeProgress
	}

	return madeProgress
}

func (m *agentMiddleware) processMsgRsp() bool {
	msgI, ok := m.memPort().RetrieveIncoming()
	if !ok {
		return false
	}

	state := &m.comp.State

	switch content := msgI.Payload.(type) {
	case memprotocol.WriteDoneRsp:
		msg := msgI

		if dumpLog {
			write := state.PendingWriteReq[msg.RspTo]
			log.Printf("%d, agent, write complete, 0x%X\n",
				m.comp.CurrentTime(), write.Payload.(memprotocol.WriteReq).Address)
		}

		req := state.PendingWriteReq[msg.RspTo]
		tracing.TraceReqFinalize(m.comp, req)
		delete(state.PendingWriteReq, msg.RspTo)

		if m.writeProgressBar != nil {
			m.writeProgressBar.MoveInProgressToFinished(1)
		}

		return true
	case memprotocol.DataReadyRsp:
		msg := msgI

		req := state.PendingReadReq[msg.RspTo]

		if dumpLog {
			log.Printf("%d, agent, read complete, 0x%X, %v\n",
				m.comp.CurrentTime(), req.Payload.(memprotocol.ReadReq).Address, content.Data)
		}

		m.checkReadResult(req, msg, state)

		tracing.TraceReqFinalize(m.comp, req)
		delete(state.PendingReadReq, msg.RspTo)

		if m.readProgressBar != nil {
			m.readProgressBar.MoveInProgressToFinished(1)
		}

		return true
	default:
		log.Panicf("cannot process message of type %s", reflect.TypeOf(msgI.Payload))
	}

	return false
}

func (m *agentMiddleware) checkReadResult(
	read messaging.Msg,
	dataReady messaging.Msg,
	state *State,
) {
	request := read.Payload.(memprotocol.ReadReq)

	found := false

	var (
		i     int
		value uint32
	)

	result := bytesToUint32(dataReady.Payload.(memprotocol.DataReadyRsp).Data)

	for i, value = range state.KnownMemValue[request.Address] {
		if value == result {
			found = true
			break
		}
	}

	if found {
		state.KnownMemValue[request.Address] = state.KnownMemValue[request.Address][i:]
	} else {
		log.Panicf("Mismatch when read 0x%X", request.Address)
	}
}

func (m *agentMiddleware) float64() float64 {
	return m.comp.State.RNG.Float64()
}

func (m *agentMiddleware) uint64() uint64 {
	return m.comp.State.RNG.Uint64()
}

func (m *agentMiddleware) uint32r() uint32 {
	return m.comp.State.RNG.Uint32()
}

func (m *agentMiddleware) shouldRead() bool {
	state := &m.comp.State

	if len(state.KnownMemValue) == 0 {
		return false
	}

	if state.ReadLeft == 0 {
		return false
	}

	if state.WriteLeft == 0 {
		return true
	}

	dice := m.float64()

	return dice > 0.5
}

func (m *agentMiddleware) doRead() bool {
	state := &m.comp.State
	address := m.randomReadAddress(state)

	if m.isAddressInPendingReq(state, address) {
		return false
	}

	readReq := messaging.Msg{Payload: memprotocol.ReadReq{
		Address:        address,
		AccessByteSize: 4,
		PID:            1},
		ID:  m.comp.NewID(),
		Src: m.memPort().AsRemote(),
		Dst: m.lowModule().AsRemote(),

		TrafficBytes: 12,
		TrafficClass: "memprotocol.ReadReq"}

	if !m.memPort().CanSend() {
		return false
	}

	m.memPort().Send(readReq)

	tracing.TraceReqInitiate(m.comp, readReq, 0)

	state.PendingReadReq[readReq.ID] = readReq
	state.ReadLeft--

	if m.readProgressBar != nil {
		m.readProgressBar.IncrementInProgress(1)
	}

	if dumpLog {
		log.Printf("%d, agent, read, 0x%X\n", m.comp.CurrentTime(), address)
	}

	return true
}

func (m *agentMiddleware) randomReadAddress(state *State) uint64 {
	spec := m.comp.Spec

	var addr uint64

	for {
		addr = spec.AddressOffset + m.uint64()%(spec.MaxAddress/4)*4

		if _, written := state.KnownMemValue[addr]; written {
			return addr
		}
	}
}

func (m *agentMiddleware) isAddressInPendingReq(state *State, addr uint64) bool {
	return m.isAddressInPendingWrite(state, addr) || m.isAddressInPendingRead(state, addr)
}

func (m *agentMiddleware) isAddressInPendingWrite(state *State, addr uint64) bool {
	for _, write := range state.PendingWriteReq {
		if write.Payload.(memprotocol.WriteReq).Address == addr {
			return true
		}
	}

	return false
}

func (m *agentMiddleware) isAddressInPendingRead(state *State, addr uint64) bool {
	for _, read := range state.PendingReadReq {
		if read.Payload.(memprotocol.ReadReq).Address == addr {
			return true
		}
	}

	return false
}

func (m *agentMiddleware) doWrite() bool {
	state := &m.comp.State
	spec := m.comp.Spec

	address := spec.AddressOffset + m.uint64()%(spec.MaxAddress/4)*4

	data := m.uint32r()

	if m.isAddressInPendingReq(state, address) {
		return false
	}

	writeData := uint32ToBytes(data)
	writeReq := messaging.Msg{Payload: memprotocol.WriteReq{
		Address: address,
		PID:     1,
		Data:    writeData},
		ID:  m.comp.NewID(),
		Src: m.memPort().AsRemote(),
		Dst: m.lowModule().AsRemote(),

		TrafficBytes: len(writeData) + 12,
		TrafficClass: "memprotocol.WriteReq"}

	if !m.memPort().CanSend() {
		return false
	}

	m.memPort().Send(writeReq)

	tracing.TraceReqInitiate(m.comp, writeReq, 0)

	state.WriteLeft--
	m.addKnownValue(state, address, data)
	state.PendingWriteReq[writeReq.ID] = writeReq

	if m.writeProgressBar != nil {
		m.writeProgressBar.IncrementInProgress(1)
	}

	if dumpLog {
		log.Printf("%d, agent, write, 0x%X, %v\n",
			m.comp.CurrentTime(), address, writeReq.Payload.(memprotocol.WriteReq).Data)
	}

	return true
}

func (m *agentMiddleware) addKnownValue(state *State, address uint64, data uint32) {
	valueList, exist := state.KnownMemValue[address]
	if !exist {
		valueList = make([]uint32, 0)
		state.KnownMemValue[address] = valueList
	}

	valueList = append(valueList, data)
	state.KnownMemValue[address] = valueList
}
