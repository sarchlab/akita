package cachemiddlewares

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
)

// //////////////////////////////////////////////////////////////////////////////
// Define interfaces that the Coalescer expects from the Cache component.
// //////////////////////////////////////////////////////////////////////////////
// The interface for coalescer middleware
type Transaction interface {
	Address() uint64
	PID() vm.PID
	IsRead() bool
	IsWrite() bool
	GetReadReq() *mem.ReadReq
	GetWriteReq() *mem.WriteReq
	ID() string
}

// CoalescerComp is the abstract interface that the cache provides to the coalescer.
type CoalescerComp interface {
	TopPort() sim.Port
	DirBufCanPush() bool
	DirBufPush(item interface{})
	AddTransaction(txn Transaction)
	PostCoalesce(txn Transaction)
	Log2BlockSize() int
	Name() string
	AcceptHook(hook sim.Hook)
	Hooks() []sim.Hook
	InvokeHook(ctx sim.HookCtx)
	NumHooks() int
	Transactions() []Transaction
	MaxNumConcurrentTrans() int
	// 让缓存组件自己创建适合的 transaction
	CreateTransaction(req mem.AccessReq) Transaction
	// 让缓存组件自己处理合并逻辑
	CreateCoalescedTransaction(transactions []Transaction) Transaction
}

////////////////////////////////////////////////////////////////////////////////
// main body of the Coalescer
////////////////////////////////////////////////////////////////////////////////

type coalescer struct {
	cache      CoalescerComp
	toCoalesce []Transaction
}

func (c *coalescer) Reset() {
	c.toCoalesce = nil
}

func (c *coalescer) Tick() bool {
	req := c.cache.TopPort().PeekIncoming()
	if req == nil {
		return false
	}

	return c.processReq(req.(mem.AccessReq))
}

func (c *coalescer) processReq(req mem.AccessReq) bool {
	if len(c.cache.Transactions()) >= c.cache.MaxNumConcurrentTrans() {
		return false
	}

	if c.isReqLastInWave(req) {
		if len(c.toCoalesce) == 0 || c.canReqCoalesce(req) {
			return c.processReqLastInWaveCoalescable(req)
		}

		return c.processReqLastInWaveNoncoalescable(req)
	}

	if len(c.toCoalesce) == 0 || c.canReqCoalesce(req) {
		return c.processReqCoalescable(req)
	}

	return c.processReqNoncoalescable(req)
}

func (c *coalescer) processReqCoalescable(req mem.AccessReq) bool {
	txn := c.cache.CreateTransaction(req)
	c.toCoalesce = append(c.toCoalesce, txn)
	c.cache.AddTransaction(txn)
	c.cache.TopPort().RetrieveIncoming()
	return true
}

func (c *coalescer) processReqNoncoalescable(req mem.AccessReq) bool {
	if !c.cache.DirBufCanPush() {
		return false
	}

	c.coalesceAndSend()

	txn := c.cache.CreateTransaction(req)
	c.toCoalesce = append(c.toCoalesce, txn)
	c.cache.AddTransaction(txn)
	c.cache.TopPort().RetrieveIncoming()
	return true
}

func (c *coalescer) processReqLastInWaveCoalescable(req mem.AccessReq) bool {
	if !c.cache.DirBufCanPush() {
		return false
	}

	txn := c.cache.CreateTransaction(req)
	c.toCoalesce = append(c.toCoalesce, txn)
	c.cache.AddTransaction(txn)
	c.coalesceAndSend()
	c.cache.TopPort().RetrieveIncoming()
	return true
}

func (c *coalescer) processReqLastInWaveNoncoalescable(req mem.AccessReq) bool {
	if !c.cache.DirBufCanPush() {
		return false
	}

	c.coalesceAndSend()

	if !c.cache.DirBufCanPush() {
		return true
	}

	txn := c.cache.CreateTransaction(req)
	c.toCoalesce = append(c.toCoalesce, txn)
	c.cache.AddTransaction(txn)
	c.coalesceAndSend()
	c.cache.TopPort().RetrieveIncoming()
	return true
}

func (c *coalescer) canReqCoalesce(req mem.AccessReq) bool {
	if len(c.toCoalesce) == 0 {
		return true
	}
	blockSize := uint64(1 << c.cache.Log2BlockSize())
	return req.GetAddress()/blockSize == c.toCoalesce[0].Address()/blockSize
}

func (c *coalescer) isReqLastInWave(req mem.AccessReq) bool {
	switch r := req.(type) {
	case *mem.ReadReq:
		return !r.CanWaitForCoalesce
	case *mem.WriteReq:
		return !r.CanWaitForCoalesce
	default:
		return false
	}
}

func (c *coalescer) coalesceAndSend() {
	if len(c.toCoalesce) == 0 {
		return
	}

	txn := c.cache.CreateCoalescedTransaction(c.toCoalesce)
	c.cache.DirBufPush(txn)
	c.cache.PostCoalesce(txn)
	c.toCoalesce = nil
}

// ToCoalesce returns the current transactions to coalesce (for testing)
func (c *coalescer) ToCoalesce() []Transaction {
	return c.toCoalesce
}

// NewCoalescer creates a new coalescer.
func NewCoalescer(cache CoalescerComp) *coalescer {
	return &coalescer{
		cache:      cache,
		toCoalesce: make([]Transaction, 0),
	}
}
