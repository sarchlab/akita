package writearound

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
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
	read            mem.ReadReq
	readToBottom    mem.ReadReq
	write           mem.WriteReq
	writeToBottom   mem.WriteReq

	preCoalesceTransactions []*transaction

	bankAction            bankActionType
	block                 *cache.Block
	data                  []byte
	writeFetchedDirtyMask []bool

	fetchAndWrite bool
	done          bool
}

func (t *transaction) Address() uint64 {
	switch t.transactionType {
	case transactionTypeRead:
		return t.read.Address
	case transactionTypeWrite:
		return t.write.Address
	}

	panic("invalid transaction type")
}

func (t *transaction) PID() vm.PID {
	switch t.transactionType {
	case transactionTypeRead:
		return t.read.PID
	case transactionTypeWrite:
		return t.write.PID
	}

	panic("invalid transaction type")
}
