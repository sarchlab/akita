package memaccessagent

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type agentMiddleware struct {
	agent *MemAccessAgent
}

func (m *agentMiddleware) memPort() messaging.Port {
	return m.agent.GetPortByName("Mem")
}

// Tick updates the states of the agent and issues new read and write requests.
func (m *agentMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.processMsgRsp() || madeProgress

	state := &m.agent.State
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
	msgI := m.memPort().RetrieveIncoming()
	if msgI == nil {
		return false
	}

	state := &m.agent.State

	switch msg := msgI.(type) {
	case mem.WriteDoneRsp:
		if dumpLog {
			write := state.PendingWriteReq[msg.RspTo]
			log.Printf("%d, agent, write complete, 0x%X\n",
				m.agent.CurrentTime(), write.Address)
		}

		req := state.PendingWriteReq[msg.RspTo]
		tracing.TraceReqFinalize(req, m.agent)
		delete(state.PendingWriteReq, msg.RspTo)

		if m.agent.writeProgressBar != nil {
			m.agent.writeProgressBar.MoveInProgressToFinished(1)
		}

		return true
	case mem.DataReadyRsp:
		req := state.PendingReadReq[msg.RspTo]

		if dumpLog {
			log.Printf("%d, agent, read complete, 0x%X, %v\n",
				m.agent.CurrentTime(), req.Address, msg.Data)
		}

		m.checkReadResult(req, msg, state)

		tracing.TraceReqFinalize(req, m.agent)
		delete(state.PendingReadReq, msg.RspTo)

		if m.agent.readProgressBar != nil {
			m.agent.readProgressBar.MoveInProgressToFinished(1)
		}

		return true
	default:
		log.Panicf("cannot process message of type %s", reflect.TypeOf(msgI))
	}

	return false
}

func (m *agentMiddleware) checkReadResult(
	read mem.ReadReq,
	dataReady mem.DataReadyRsp,
	state *State,
) {
	found := false

	var (
		i     int
		value uint32
	)

	result := bytesToUint32(dataReady.Data)

	for i, value = range state.KnownMemValue[read.Address] {
		if value == result {
			found = true
			break
		}
	}

	if found {
		state.KnownMemValue[read.Address] = state.KnownMemValue[read.Address][i:]
	} else {
		log.Panicf("Mismatch when read 0x%X", read.Address)
	}
}

func (m *agentMiddleware) float64() float64 {
	if m.agent.rng != nil {
		return m.agent.rng.Float64()
	}
	return globalFloat64()
}

func (m *agentMiddleware) uint64() uint64 {
	if m.agent.rng != nil {
		return m.agent.rng.Uint64()
	}
	return globalUint64()
}

func (m *agentMiddleware) uint32r() uint32 {
	if m.agent.rng != nil {
		return m.agent.rng.Uint32()
	}
	return globalUint32()
}

func (m *agentMiddleware) shouldRead() bool {
	state := &m.agent.State

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
	state := &m.agent.State
	address := m.randomReadAddress(state)

	if m.isAddressInPendingReq(state, address) {
		return false
	}

	readReq := mem.ReadReq{}
	readReq.ID = timing.GetIDGenerator().Generate()
	readReq.Src = m.memPort().AsRemote()
	readReq.Dst = m.agent.LowModule.AsRemote()
	readReq.Address = address
	readReq.AccessByteSize = 4
	readReq.PID = 1
	readReq.TrafficBytes = 12
	readReq.TrafficClass = "mem.ReadReq"

	err := m.memPort().Send(readReq)
	if err == nil {
		tracing.TraceReqInitiate(readReq, m.agent, 0)

		state.PendingReadReq[readReq.ID] = readReq
		state.ReadLeft--

		if m.agent.readProgressBar != nil {
			m.agent.readProgressBar.IncrementInProgress(1)
		}

		if dumpLog {
			log.Printf("%d, agent, read, 0x%X\n", m.agent.CurrentTime(), address)
		}

		return true
	}

	return false
}

func (m *agentMiddleware) randomReadAddress(state *State) uint64 {
	spec := m.agent.Spec()

	var addr uint64

	for {
		addr = m.uint64() % (spec.MaxAddress / 4) * 4

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
		if write.Address == addr {
			return true
		}
	}

	return false
}

func (m *agentMiddleware) isAddressInPendingRead(state *State, addr uint64) bool {
	for _, read := range state.PendingReadReq {
		if read.Address == addr {
			return true
		}
	}

	return false
}

func (m *agentMiddleware) doWrite() bool {
	state := &m.agent.State
	spec := m.agent.Spec()

	address := m.uint64() % (spec.MaxAddress / 4) * 4

	data := m.uint32r()

	if m.isAddressInPendingReq(state, address) {
		return false
	}

	writeData := uint32ToBytes(data)
	writeReq := mem.WriteReq{}
	writeReq.ID = timing.GetIDGenerator().Generate()
	writeReq.Src = m.memPort().AsRemote()
	writeReq.Dst = m.agent.LowModule.AsRemote()
	writeReq.Address = address
	writeReq.PID = 1
	writeReq.Data = writeData
	writeReq.TrafficBytes = len(writeData) + 12
	writeReq.TrafficClass = "mem.WriteReq"

	err := m.memPort().Send(writeReq)
	if err == nil {
		tracing.TraceReqInitiate(writeReq, m.agent, 0)

		state.WriteLeft--
		m.addKnownValue(state, address, data)
		state.PendingWriteReq[writeReq.ID] = writeReq

		if m.agent.writeProgressBar != nil {
			m.agent.writeProgressBar.IncrementInProgress(1)
		}

		if dumpLog {
			log.Printf("%d, agent, write, 0x%X, %v\n",
				m.agent.CurrentTime(), address, writeReq.Data)
		}

		return true
	}

	return false
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
