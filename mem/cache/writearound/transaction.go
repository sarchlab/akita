package writearound

import (
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/vm"
)

type bankActionType int

const (
	bankActionInvalid bankActionType = iota
	bankActionReadHit
	bankActionWrite
	bankActionWriteFetched
)

type transactionType int

const (
	transactionTypeRead transactionType = iota
	transactionTypeWrite
)

type transaction struct {
	id string

	transactionType transactionType

	read         mem.ReadReq
	readToBottom mem.ReadReq

	write         mem.WriteReq
	writeToBottom mem.WriteReq

	preCoalesceTransactions []*transaction

	bankAction            bankActionType
	block                 *cache.Block
	data                  []byte
	writeFetchedDirtyMask []bool

	fetchAndWrite bool
	done          bool
}

func (t *transaction) Address() uint64 {
	if t.transactionType == transactionTypeRead {
		return t.read.Address
	}

	return t.write.Address
}

func (t *transaction) PID() vm.PID {
	if t.transactionType == transactionTypeRead {
		return t.read.PID
	}

	return t.write.PID
}
