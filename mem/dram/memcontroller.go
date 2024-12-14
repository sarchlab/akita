package dram

import (
	"reflect"

	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/dram/internal/addressmapping"
	"github.com/sarchlab/akita/v4/mem/dram/internal/cmdq"
	"github.com/sarchlab/akita/v4/mem/dram/internal/org"
	"github.com/sarchlab/akita/v4/mem/dram/internal/signal"
	"github.com/sarchlab/akita/v4/mem/dram/internal/trans"
	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

// Protocol defines the category of the memory controller.
type Protocol int

// A list of all supported DRAM protocols.
const (
	DDR3 Protocol = iota
	DDR4
	GDDR5
	GDDR5X
	GDDR6
	LPDDR
	LPDDR3
	LPDDR4
	HBM
	HBM2
	HMC
)

func (p Protocol) isGDDR() bool {
	return p == GDDR5 || p == GDDR5X || p == GDDR6
}

func (p Protocol) isHBM() bool {
	return p == HBM || p == HBM2
}

// Comp is a MemController handles read and write requests.
type Comp struct {
	*modeling.TickingComponent
	modeling.MiddlewareHolder

	topPort modeling.Port

	storage             *mem.Storage
	addrConverter       mem.AddressConverter
	subTransSplitter    trans.SubTransSplitter
	addrMapper          addressmapping.Mapper
	subTransactionQueue trans.SubTransactionQueue
	cmdQueue            cmdq.CommandQueue
	channel             org.Channel

	inflightTransactions []*signal.Transaction
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

// Tick updates memory controller's internal state.
func (m *middleware) Tick() (madeProgress bool) {
	madeProgress = m.respond() || madeProgress
	madeProgress = m.respond() || madeProgress
	madeProgress = m.channel.Tick() || madeProgress
	madeProgress = m.issue() || madeProgress
	madeProgress = m.subTransactionQueue.Tick() || madeProgress
	madeProgress = m.parseTop() || madeProgress

	return madeProgress
}

func (m *middleware) parseTop() (madeProgress bool) {
	msg := m.topPort.PeekIncoming()
	if msg == nil {
		return false
	}

	trans := &signal.Transaction{}
	switch msg := msg.(type) {
	case mem.ReadReq:
		trans.Read = msg
		trans.Type = signal.TransactionTypeRead
	case mem.WriteReq:
		trans.Write = msg
		trans.Type = signal.TransactionTypeWrite
	}

	m.assignTransInternalAddress(trans)
	m.subTransSplitter.Split(trans)

	if !m.subTransactionQueue.CanPush(len(trans.SubTransactions)) {
		return false
	}

	m.subTransactionQueue.Push(trans)
	m.inflightTransactions = append(m.inflightTransactions, trans)
	m.topPort.RetrieveIncoming()

	m.traceTransactionStart(msg)

	// fmt.Printf("%.10f, %s, start transaction, %s, %x\n",
	// 	now, c.Name(), msg.Meta().ID, trans.InternalAddress)

	return true
}

func (m *middleware) assignTransInternalAddress(trans *signal.Transaction) {
	if m.addrConverter != nil {
		trans.InternalAddress = m.addrConverter.ConvertExternalToInternal(
			trans.GlobalAddress())
		return
	}

	trans.InternalAddress = trans.GlobalAddress()
}

func (m *middleware) issue() (madeProgress bool) {
	cmd := m.cmdQueue.GetCommandToIssue()
	if cmd == nil {
		return false
	}

	m.channel.StartCommand(cmd)
	m.channel.UpdateTiming(cmd)

	return true
}

func (m *middleware) respond() (madeProgress bool) {
	for i, t := range m.inflightTransactions {
		if t.IsCompleted() {
			done := m.finalizeTransaction(t, i)
			if done {
				return true
			}
		}
	}

	return false
}

func (m *middleware) finalizeTransaction(
	t *signal.Transaction,
	i int,
) (done bool) {
	if t.Type == signal.TransactionTypeWrite {
		done = m.finalizeWriteTrans(t, i)
		if done {
			m.traceTransactionComplete(t)
		}
	} else {
		done = m.finalizeReadTrans(t, i)
		if done {
			m.traceTransactionComplete(t)
		}
	}

	return done
}

func (m *middleware) finalizeWriteTrans(
	t *signal.Transaction,
	i int,
) (done bool) {
	err := m.storage.Write(t.InternalAddress, t.Write.Data)
	if err != nil {
		panic(err)
	}

	writeDone := mem.WriteDoneRsp{
		MsgMeta: modeling.MsgMeta{
			Src: m.topPort.AsRemote(),
			Dst: t.Write.Src,
		},
		RespondTo: t.Write.ID,
	}

	sendErr := m.topPort.Send(writeDone)
	if sendErr == nil {
		m.inflightTransactions = append(
			m.inflightTransactions[:i],
			m.inflightTransactions[i+1:]...)

		// fmt.Printf("%.10f, %s, finish transaction %s, %x\n",
		// 	now, c.Name(), t.Write.ID, t.InternalAddress)
		return true
	}

	return false
}

func (m *middleware) finalizeReadTrans(
	t *signal.Transaction,
	i int,
) (done bool) {
	data, err := m.storage.Read(t.InternalAddress, t.Read.AccessByteSize)
	if err != nil {
		panic(err)
	}

	dataReady := mem.DataReadyRsp{
		MsgMeta: modeling.MsgMeta{
			Src: m.topPort.AsRemote(),
			Dst: t.Read.Src,
		},
		RespondTo: t.Read.ID,
		Data:      data,
	}

	sendErr := m.topPort.Send(dataReady)
	if sendErr == nil {
		m.inflightTransactions = append(
			m.inflightTransactions[:i],
			m.inflightTransactions[i+1:]...)

		// fmt.Printf("%.10f, %s, finish transaction %s, %x\n",
		// 	now, c.Name(), t.Read.ID, t.InternalAddress)
		return true
	}

	return false
}

func (m *middleware) traceTransactionStart(msg modeling.Msg) {
	ctx := hooking.HookCtx{
		Domain: m.Comp,
		Pos:    hooking.HookPosTaskStart,
		Item: hooking.TaskStart{
			ID:       modeling.ReqInTaskID(msg.Meta().ID),
			ParentID: modeling.ReqOutTaskID(msg.Meta().ID),
			Kind:     "req_in",
			What:     reflect.TypeOf(msg).Name(),
		},
	}

	m.Comp.InvokeHook(ctx)
}

func (m *middleware) traceTransactionComplete(t *signal.Transaction) {
	ctx := hooking.HookCtx{
		Domain: m.Comp,
		Pos:    hooking.HookPosTaskEnd,
	}

	switch t.Type {
	case signal.TransactionTypeWrite:
		ctx.Item = hooking.TaskEnd{
			ID: modeling.ReqInTaskID(t.Write.Meta().ID),
		}
	case signal.TransactionTypeRead:
		ctx.Item = hooking.TaskEnd{
			ID: modeling.ReqInTaskID(t.Read.Meta().ID),
		}
	}

	m.Comp.InvokeHook(ctx)
}
