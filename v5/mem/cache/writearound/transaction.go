package writearound

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
)

type bankActionType int

const (
	bankActionInvalid bankActionType = iota
	bankActionReadHit
	bankActionWrite
	bankActionWriteFetched
)

// transactionState is the canonical transaction type for the writearound cache.
// All stages work with *transactionState directly. The snapshot/restore layer
// in state.go converts between transactionState (runtime) and
// transactionSnapshot (serializable) for persistence.
type transactionState struct {
	id string

	read         *mem.ReadReq
	readToBottom *mem.ReadReq

	write         *mem.WriteReq
	writeToBottom *mem.WriteReq

	preCoalesceTransactions []*transactionState

	bankAction            bankActionType
	blockSetID            int
	blockWayID            int
	hasBlock              bool
	data                  []byte
	writeFetchedDirtyMask []bool

	fetchAndWrite   bool
	done            bool
	bottomWriteDone bool
	bankDone        bool
}

func (t *transactionState) Address() uint64 {
	if t.read != nil {
		return t.read.Address
	}

	return t.write.Address
}

func (t *transactionState) PID() vm.PID {
	if t.read != nil {
		return t.read.PID
	}

	return t.write.PID
}
