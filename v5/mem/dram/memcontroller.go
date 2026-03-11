package dram

import (
	"github.com/sarchlab/akita/v5/mem/dram/internal/addressmapping"
	"github.com/sarchlab/akita/v5/mem/dram/internal/cmdq"
	"github.com/sarchlab/akita/v5/mem/dram/internal/org"
	"github.com/sarchlab/akita/v5/mem/dram/internal/signal"
	"github.com/sarchlab/akita/v5/mem/dram/internal/trans"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
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

// Spec contains immutable configuration for the DRAM memory controller.
type Spec struct{}

// State contains mutable runtime data for the DRAM memory controller.
type State struct {
	Transactions  []transactionState  `json:"transactions"`
	SubTransQueue subTransQueueState  `json:"sub_trans_queue"`
	CommandQueues commandQueueState   `json:"command_queues"`
	BankStates    bankStatesFlat      `json:"bank_states"`
}

// Comp is a MemController handles read and write requests.
type Comp struct {
	*modeling.Component[Spec, State]

	topPort sim.Port

	storage             *mem.Storage
	addrConverter       mem.AddressConverter
	subTransSplitter    trans.SubTransSplitter
	addrMapper          addressmapping.Mapper
	subTransactionQueue trans.SubTransactionQueue
	cmdQueue            cmdq.CommandQueue
	channel             org.Channel

	inflightTransactions []*signal.Transaction
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
	msgI := m.topPort.PeekIncoming()
	if msgI == nil {
		return false
	}

	trans := &signal.Transaction{}

	switch msg := msgI.(type) {
	case *mem.ReadReq:
		trans.Read = msg
	case *mem.WriteReq:
		trans.Write = msg
	}

	m.assignTransInternalAddress(trans)
	m.subTransSplitter.Split(trans)

	if !m.subTransactionQueue.CanPush(len(trans.SubTransactions)) {
		return false
	}

	m.subTransactionQueue.Push(trans)
	m.inflightTransactions = append(m.inflightTransactions, trans)
	m.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msgI, m.Comp)

	for _, st := range trans.SubTransactions {
		tracing.StartTaskWithSpecificLocation(
			st.ID,
			tracing.MsgIDAtReceiver(msgI, m.Comp),
			m.Comp,
			"sub-trans",
			"sub-trans",
			m.Comp.Name()+".SubTransQueue",
			nil,
		)
	}

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
	if t.Write != nil {
		done = m.finalizeWriteTrans(t, i)
		if done {
			tracing.TraceReqComplete(t.Write, m.Comp)
		}
	} else {
		done = m.finalizeReadTrans(t, i)
		if done {
			tracing.TraceReqComplete(t.Read, m.Comp)
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

	writeDone := &mem.WriteDoneRsp{}
	writeDone.ID = sim.GetIDGenerator().Generate()
	writeDone.Src = m.topPort.AsRemote()
	writeDone.Dst = t.Write.Src
	writeDone.RspTo = t.Write.ID
	writeDone.TrafficBytes = 4
	writeDone.TrafficClass = "mem.WriteDoneRsp"

	sendErr := m.topPort.Send(writeDone)
	if sendErr == nil {
		m.inflightTransactions = append(
			m.inflightTransactions[:i],
			m.inflightTransactions[i+1:]...)

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

	dataReady := &mem.DataReadyRsp{}
	dataReady.ID = sim.GetIDGenerator().Generate()
	dataReady.Src = m.topPort.AsRemote()
	dataReady.Dst = t.Read.Src
	dataReady.Data = data
	dataReady.RspTo = t.Read.ID
	dataReady.TrafficBytes = len(data) + 4
	dataReady.TrafficClass = "mem.DataReadyRsp"

	sendErr := m.topPort.Send(dataReady)
	if sendErr == nil {
		m.inflightTransactions = append(
			m.inflightTransactions[:i],
			m.inflightTransactions[i+1:]...)

		return true
	}

	return false
}
