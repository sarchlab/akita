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

// transactionState is the canonical runtime transaction type.
// All stages work with *transactionState directly.
type transactionState struct {
	action

	id    string
	read  *mem.ReadReq
	write *mem.WriteReq
	flush *cache.FlushReq

	// Block reference (into directoryState)
	blockSetID int
	blockWayID int
	hasBlock   bool

	// Victim data (inlined, not a pointer to cache.Block)
	victimPID          vm.PID
	victimTag          uint64
	victimCacheAddress uint64
	victimDirtyMask    []bool
	hasVictim          bool

	fetchPID     vm.PID
	fetchAddress uint64
	fetchedData  []byte
	fetchReadReq *mem.ReadReq

	evictingPID       vm.PID
	evictingAddr      uint64
	evictingData      []byte
	evictingDirtyMask []bool
	evictionWriteReq  *mem.WriteReq

	// MSHR entry reference (into mshrState.Entries)
	mshrEntryIndex int
	hasMSHREntry   bool
}

func (t *transactionState) accessReq() mem.AccessReq {
	if t.read != nil {
		return t.read
	}

	if t.write != nil {
		return t.write
	}

	return nil
}

func (t *transactionState) req() sim.Msg {
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
