package writearound

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

type bankActionType int

const (
	bankActionInvalid bankActionType = iota
	bankActionReadHit
	bankActionWrite
	bankActionWriteFetched
)

type transaction struct {
	id string

	read         *sim.GenericMsg // payload: *mem.ReadReqPayload
	readToBottom *sim.GenericMsg // payload: *mem.ReadReqPayload

	write         *sim.GenericMsg // payload: *mem.WriteReqPayload
	writeToBottom *sim.GenericMsg // payload: *mem.WriteReqPayload

	preCoalesceTransactions []*transaction

	bankAction            bankActionType
	block                 *cache.Block
	data                  []byte
	writeFetchedDirtyMask []bool

	fetchAndWrite bool
	done          bool
}

func (t *transaction) Address() uint64 {
	if t.read != nil {
		return sim.MsgPayload[mem.ReadReqPayload](t.read).Address
	}

	return sim.MsgPayload[mem.WriteReqPayload](t.write).Address
}

func (t *transaction) PID() vm.PID {
	if t.read != nil {
		return sim.MsgPayload[mem.ReadReqPayload](t.read).PID
	}

	return sim.MsgPayload[mem.WriteReqPayload](t.write).PID
}
