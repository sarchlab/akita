// Package acceptancetests provides utility data structure definitions for
// writing memory system acceptance tests.
package memaccessagent

import (
	"encoding/binary"
	"log"
	"math/rand"
	"reflect"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
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

	// Rand is the random source used by the agent. If nil, the global
	// math/rand functions are used (non-deterministic in Go 1.22+).
	Rand *rand.Rand
}

func (a *MemAccessAgent) checkReadResult(read *mem.ReadReq,
	dataReady *mem.DataReadyRsp,
) {
	found := false

	var (
		i     int
		value uint32
	)

	result := bytesToUint32(dataReady.Data)

	for i, value = range a.KnownMemValue[read.Address] {
		if value == result {
			found = true
			break
		}
	}

	if found {
		a.KnownMemValue[read.Address] = a.KnownMemValue[read.Address][i:]
	} else {
		log.Panicf("Mismatch when read 0x%X", read.Address)
	}
}

func bytesToUint32(data []byte) uint32 {
	a := uint32(0)
	a += uint32(data[0])
	a += uint32(data[1]) << 8
	a += uint32(data[2]) << 16
	a += uint32(data[3]) << 24

	return a
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
	msgI := a.memPort.RetrieveIncoming()
	if msgI == nil {
		return false
	}

	switch msg := msgI.(type) {
	case *mem.WriteDoneRsp:
		if dumpLog {
			write := a.PendingWriteReq[msg.RspTo]
			log.Printf("%.10f, agent, write complete, 0x%X\n",
				a.CurrentTime(), write.Address)
		}

		delete(a.PendingWriteReq, msg.RspTo)

		return true
	case *mem.DataReadyRsp:
		req := a.PendingReadReq[msg.RspTo]
		delete(a.PendingReadReq, msg.RspTo)

		if dumpLog {
			log.Printf("%.10f, agent, read complete, 0x%X, %v\n",
				a.CurrentTime(), req.Address, msg.Data)
		}

		a.checkReadResult(req, msg)

		return true
	default:
		log.Panicf("cannot process message of type %s", reflect.TypeOf(msgI))
	}

	return false
}

func (a *MemAccessAgent) float64() float64 {
	if a.Rand != nil {
		return a.Rand.Float64()
	}
	return rand.Float64()
}

func (a *MemAccessAgent) uint64() uint64 {
	if a.Rand != nil {
		return a.Rand.Uint64()
	}
	return rand.Uint64()
}

func (a *MemAccessAgent) uint32r() uint32 {
	if a.Rand != nil {
		return a.Rand.Uint32()
	}
	return rand.Uint32()
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

	dice := a.float64()

	return dice > 0.5
}

func (a *MemAccessAgent) doRead() bool {
	address := a.randomReadAddress()

	if a.isAddressInPendingReq(address) {
		return false
	}

	readReq := &mem.ReadReq{}
	readReq.ID = sim.GetIDGenerator().Generate()
	readReq.Src = a.memPort.AsRemote()
	readReq.Dst = a.LowModule.AsRemote()
	readReq.Address = address
	readReq.AccessByteSize = 4
	readReq.PID = 1
	readReq.TrafficBytes = 12
	readReq.TrafficClass = "mem.ReadReq"

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
		addr = a.uint64() % (a.MaxAddress / 4) * 4

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
	address := a.uint64() % (a.MaxAddress / 4) * 4

	data := a.uint32r()

	if a.isAddressInPendingReq(address) {
		return false
	}

	writeData := uint32ToBytes(data)
	writeReq := &mem.WriteReq{}
	writeReq.ID = sim.GetIDGenerator().Generate()
	writeReq.Src = a.memPort.AsRemote()
	writeReq.Dst = a.LowModule.AsRemote()
	writeReq.Address = address
	writeReq.PID = 1
	writeReq.Data = writeData
	writeReq.TrafficBytes = len(writeData) + 12
	writeReq.TrafficClass = "mem.WriteReq"

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
func NewMemAccessAgent(engine sim.EventScheduler) *MemAccessAgent {
	agent := new(MemAccessAgent)
	agent.TickingComponent = sim.NewTickingComponent(
		"Agent", engine, 1*sim.GHz, agent)

	agent.ReadLeft = 10000
	agent.WriteLeft = 10000
	agent.KnownMemValue = make(map[uint64][]uint32)
	agent.PendingWriteReq = make(map[string]*mem.WriteReq)
	agent.PendingReadReq = make(map[string]*mem.ReadReq)

	return agent
}
