// Package acceptancetests provides utility data structure definitions for
// writing memory system acceptance tests.
package memaccessagent

import (
	"encoding/binary"
	"log"
	"math/rand"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

var dumpLog = false

// A MemAccessAgent is a Component that can help testing the cache and the the
// memory controllers by generating a large number of read and write requests.
type MemAccessAgent struct {
	*sim.TickingComponent

	LowModule  sim.Port
	MaxAddress uint64

	WriteLeft       int
	ReadLeft        int
	KnownMemValue   map[uint64][]uint32
	PendingReadReq  map[string]*mem.ReadReq
	PendingWriteReq map[string]*mem.WriteReq

	memPort           sim.Port
	UseVirtualAddress bool
}

// Tick updates the states of the agent and issues new read and write requests.
func (a *MemAccessAgent) Tick() bool {
	madeProgress := false

	madeProgress = a.processMsgRsp() || madeProgress

	if a.ReadLeft == 0 && a.WriteLeft == 0 {
		return madeProgress
	}

	if a.shouldRead() {
		madeProgress = a.doRead() || madeProgress
	} else {
		madeProgress = a.doWrite() || madeProgress
	}

	return madeProgress
}

func (a *MemAccessAgent) processMsgRsp() bool {
	msg := a.memPort.RetrieveIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *mem.WriteDoneRsp:
		if dumpLog {
			write := a.PendingWriteReq[msg.RespondTo]
			log.Printf("%.10f, agent, write complete, 0x%X\n",
				a.CurrentTime(), write.Address)
		}

		delete(a.PendingWriteReq, msg.RespondTo)

		return true
	case *mem.DataReadyRsp:
		req := a.PendingReadReq[msg.RespondTo]
		delete(a.PendingReadReq, msg.RespondTo)

		if dumpLog {
			log.Printf("%.10f, agent, read complete, 0x%X, %v\n",
				a.CurrentTime(), req.Address, msg.Data)
		}

		// a.checkReadResult(req, msg)

		return true
	default:
		log.Panicf("cannot process message of type %s", reflect.TypeOf(msg))
	}

	return false
}

func (a *MemAccessAgent) shouldRead() bool {
	if len(a.KnownMemValue) == 0 {
		return false
	}

	if a.ReadLeft == 0 {
		return false
	}

	if a.WriteLeft == 0 {
		return true
	}

	dice := rand.Float64()

	return dice > 0.5
}

func (a *MemAccessAgent) doRead() bool {
	var address uint64
	if a.UseVirtualAddress {
		address = a.randomVirtualAddress()
	} else {
		address = a.randomReadAddress()
	}

	if a.isAddressInPendingReq(address) {
		return false
	}

	readReq := mem.ReadReqBuilder{}.
		WithSrc(a.memPort.AsRemote()).
		WithDst(a.LowModule.AsRemote()).
		WithAddress(address).
		WithByteSize(4).
		WithPID(1).
		Build()

	err := a.memPort.Send(readReq)
	if err == nil {
		a.PendingReadReq[readReq.ID] = readReq
		a.ReadLeft--

		if dumpLog {
			log.Printf("%.10f, agent, read, 0x%X\n", a.CurrentTime(), address)
		}

		return true
	}

	return false
}

func (a *MemAccessAgent) randomReadAddress() uint64 {
	var addr uint64

	for {
		if a.UseVirtualAddress {
			addr = 0x100000000 + rand.Uint64()%(a.MaxAddress/4)*4 // e.g., start virtual at 0x100000000
		} else {
			addr = rand.Uint64() % (a.MaxAddress / 4) * 4
		}
		if _, written := a.KnownMemValue[addr]; written {
			return addr
		}
	}
}

func (a *MemAccessAgent) isAddressInPendingReq(addr uint64) bool {
	return a.isAddressInPendingWrite(addr) || a.isAddressInPendingRead(addr)
}

func (a *MemAccessAgent) isAddressInPendingWrite(addr uint64) bool {
	for _, write := range a.PendingWriteReq {
		if write.Address == addr {
			return true
		}
	}

	return false
}

func (a *MemAccessAgent) isAddressInPendingRead(addr uint64) bool {
	for _, read := range a.PendingReadReq {
		if read.Address == addr {
			return true
		}
	}

	return false
}

func uint32ToBytes(data uint32) []byte {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, data)

	return bytes
}

func (a *MemAccessAgent) doWrite() bool {
	var address uint64
	if a.UseVirtualAddress {
		address = rand.Uint64() % (a.MaxAddress / 4) * 4
	} else {
		address = rand.Uint64() % (a.MaxAddress / 4) * 4
	}
	data := rand.Uint32()

	if a.isAddressInPendingReq(address) {
		return false
	}

	writeReq := mem.WriteReqBuilder{}.
		WithSrc(a.memPort.AsRemote()).
		WithDst(a.LowModule.AsRemote()).
		WithAddress(address).
		WithPID(1).
		WithData(uint32ToBytes(data)).
		Build()

	err := a.memPort.Send(writeReq)
	if err == nil {
		a.WriteLeft--
		a.addKnownValue(address, data)
		a.PendingWriteReq[writeReq.ID] = writeReq

		if dumpLog {
			log.Printf("%.10f, agent, write, 0x%X, %v\n",
				a.CurrentTime(), address, writeReq.Data)
		}

		return true
	}

	return false
}

func (a *MemAccessAgent) addKnownValue(address uint64, data uint32) {
	valueList, exist := a.KnownMemValue[address]
	if !exist {
		valueList = make([]uint32, 0)
		a.KnownMemValue[address] = valueList
	}

	valueList = append(valueList, data)
	a.KnownMemValue[address] = valueList
}

// NewMemAccessAgent creates a new MemAccessAgent.
func NewMemAccessAgent(engine sim.Engine) *MemAccessAgent {
	agent := new(MemAccessAgent)
	agent.TickingComponent = sim.NewTickingComponent(
		"Agent", engine, 1*sim.GHz, agent)

	agent.memPort = sim.NewPort(agent, 1, 1, "Agent.MemPort")
	agent.AddPort("Mem", agent.memPort)

	agent.ReadLeft = 10000
	agent.WriteLeft = 10000
	agent.KnownMemValue = make(map[uint64][]uint32)
	agent.PendingWriteReq = make(map[string]*mem.WriteReq)
	agent.PendingReadReq = make(map[string]*mem.ReadReq)

	return agent
}
