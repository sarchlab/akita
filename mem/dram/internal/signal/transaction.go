package signal

import "github.com/sarchlab/akita/v4/mem/mem"

type TransactionType int

const (
	TransactionTypeRead TransactionType = iota
	TransactionTypeWrite
)

// Transaction is the state associated with the processing of a read or write
// request.
type Transaction struct {
	Type  TransactionType
	Read  mem.ReadReq
	Write mem.WriteReq

	InternalAddress uint64
	SubTransactions []*SubTransaction
}

// GlobalAddress returns the address that the transaction is accessing.
func (t *Transaction) GlobalAddress() uint64 {
	if t.Type == TransactionTypeRead {
		return t.Read.Address
	}

	return t.Write.Address
}

// AccessByteSize returns the number of bytes that the transaction is accessing.
func (t *Transaction) AccessByteSize() uint64 {
	if t.Type == TransactionTypeRead {
		return t.Read.AccessByteSize
	}

	return uint64(len(t.Write.Data))
}

// IsRead returns true if the transaction is a read transaction.
func (t *Transaction) IsRead() bool {
	return t.Type == TransactionTypeRead
}

// IsWrite returns true if the transaction is a write transaction.
func (t *Transaction) IsWrite() bool {
	return t.Type == TransactionTypeWrite
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
