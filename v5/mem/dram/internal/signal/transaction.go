package signal

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
)

// Transaction is the state associated with the processing of a read or write
// request.
type Transaction struct {
	Read  *sim.GenericMsg // payload: *mem.ReadReqPayload
	Write *sim.GenericMsg // payload: *mem.WriteReqPayload

	InternalAddress uint64
	SubTransactions []*SubTransaction
}

// GlobalAddress returns the address that the transaction is accessing.
func (t *Transaction) GlobalAddress() uint64 {
	if t.Read != nil {
		return sim.MsgPayload[mem.ReadReqPayload](t.Read).Address
	}

	return sim.MsgPayload[mem.WriteReqPayload](t.Write).Address
}

// AccessByteSize returns the number of bytes that the transaction is accessing.
func (t *Transaction) AccessByteSize() uint64 {
	if t.Read != nil {
		return sim.MsgPayload[mem.ReadReqPayload](t.Read).AccessByteSize
	}

	return uint64(len(sim.MsgPayload[mem.WriteReqPayload](t.Write).Data))
}

// IsRead returns true if the transaction is a read transaction.
func (t *Transaction) IsRead() bool {
	return t.Read != nil
}

// IsWrite returns true if the transaction is a write transaction.
func (t *Transaction) IsWrite() bool {
	return t.Write != nil
}

// IsCompleted returns true if the transaction is fully ready to be returned.
func (t *Transaction) IsCompleted() bool {
	for _, st := range t.SubTransactions {
		if !st.Completed {
			return false
		}
	}

	return true
}
