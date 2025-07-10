package writeback

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
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
	read              *mem.ReadReq
	write             *mem.WriteReq
	flush             *cache.FlushReq
	block             *cache.Block
	victim            *cache.Block
	fetchPID          vm.PID
	fetchAddress      uint64
	fetchedData       []byte
	fetchReadReq      *mem.ReadReq
	evictingPID       vm.PID
	evictingAddr      uint64
	evictingData      []byte
	evictingDirtyMask []bool
	evictionWriteReq  *mem.WriteReq
	mshrEntry         *cache.MSHREntry
}

func (t transaction) accessReq() mem.AccessReq {
	if t.read != nil {
		return t.read
	}

	if t.write != nil {
		return t.write
	}

	return nil
}

func (t transaction) req() sim.Msg {
	if t.accessReq() != nil {
		return t.accessReq()
	}

	if t.flush != nil {
		return t.flush
	}

	return nil
}

// Implementation of the Transaction interface for coalescer middleware
func (t transaction) Address() uint64 {
	if t.read != nil {
		return t.read.Address
	}
	if t.write != nil {
		return t.write.Address
	}
	return 0
}

func (t transaction) PID() vm.PID {
	if t.read != nil {
		return t.read.PID
	}
	if t.write != nil {
		return t.write.PID
	}
	return 0
}

func (t transaction) IsRead() bool {
	return t.read != nil
}

func (t transaction) IsWrite() bool {
	return t.write != nil
}

func (t transaction) GetReadReq() *mem.ReadReq {
	return t.read
}

func (t transaction) GetWriteReq() *mem.WriteReq {
	return t.write
}

func (t transaction) ID() string {
	return t.id
}
