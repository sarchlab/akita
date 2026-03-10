package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

type action int

const (
	actionInvalid action = iota
	bankReadHit
	bankWriteHit
	bankEvict
	bankEvictAndWrite
	bankEvictAndFetch
	bankWriteFetched
	writeBufferFetch
	writeBufferEvictAndFetch
	writeBufferEvictAndWrite
	writeBufferFlush
)

type transaction struct {
	action

	id                string
	read              *sim.GenericMsg // payload: *mem.ReadReqPayload
	write             *sim.GenericMsg // payload: *mem.WriteReqPayload
	flush             *sim.GenericMsg // payload: *cache.FlushReqPayload
	block             *cache.Block
	victim            *cache.Block
	fetchPID          vm.PID
	fetchAddress      uint64
	fetchedData       []byte
	fetchReadReq      *sim.GenericMsg // payload: *mem.ReadReqPayload
	evictingPID       vm.PID
	evictingAddr      uint64
	evictingData      []byte
	evictingDirtyMask []bool
	evictionWriteReq  *sim.GenericMsg // payload: *mem.WriteReqPayload
	mshrEntry         *cache.MSHREntry
}

func (t transaction) accessReq() mem.AccessReqPayload {
	if t.read != nil {
		return sim.MsgPayload[mem.ReadReqPayload](t.read)
	}

	if t.write != nil {
		return sim.MsgPayload[mem.WriteReqPayload](t.write)
	}

	return nil
}

func (t transaction) req() *sim.GenericMsg {
	if t.read != nil {
		return t.read
	}

	if t.write != nil {
		return t.write
	}

	if t.flush != nil {
		return t.flush
	}

	return nil
}
