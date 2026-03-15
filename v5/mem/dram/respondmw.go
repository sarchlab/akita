package dram

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type respondMW struct {
	comp    *modeling.Component[Spec, State]
	topPort sim.Port
	storage *mem.Storage
}

// Tick runs the respond stage twice (matching original execution order).
func (m *respondMW) Tick() bool {
	next := m.comp.GetNextState()
	spec := m.comp.GetSpec()

	progress := m.respond(&spec, next)
	progress = m.respond(&spec, next) || progress

	return progress
}

func (m *respondMW) respond(spec *Spec, next *State) bool {
	for i := range next.Transactions {
		t := &next.Transactions[i]
		if isTransactionCompleted(t) {
			done := m.finalizeTransaction(spec, next, t, i)
			if done {
				return true
			}
		}
	}

	return false
}

func (m *respondMW) finalizeTransaction(
	spec *Spec,
	state *State,
	t *transactionState,
	i int,
) bool {
	if t.HasWrite {
		done := m.finalizeWriteTrans(state, t, i)
		if done {
			tracing.TraceReqComplete(&t.WriteMsg, m.comp)
		}
		return done
	}

	done := m.finalizeReadTrans(state, t, i)
	if done {
		tracing.TraceReqComplete(&t.ReadMsg, m.comp)
	}
	return done
}

func (m *respondMW) finalizeWriteTrans(
	state *State,
	t *transactionState,
	i int,
) bool {
	err := m.storage.Write(t.InternalAddress, t.WriteMsg.Data)
	if err != nil {
		panic(err)
	}

	writeDone := &mem.WriteDoneRsp{}
	writeDone.ID = sim.GetIDGenerator().Generate()
	writeDone.Src = m.topPort.AsRemote()
	writeDone.Dst = t.WriteMsg.Src
	writeDone.RspTo = t.WriteMsg.ID
	writeDone.TrafficBytes = 4
	writeDone.TrafficClass = "mem.WriteDoneRsp"

	sendErr := m.topPort.Send(writeDone)
	if sendErr == nil {
		state.TotalWriteLatencyCycles += state.TickCount - t.ArrivalTick
		state.BytesWritten += uint64(len(t.WriteMsg.Data))
		state.CompletedWrites++
		m.removeTransaction(state, i)
		return true
	}

	return false
}

func (m *respondMW) finalizeReadTrans(
	state *State,
	t *transactionState,
	i int,
) bool {
	data, err := m.storage.Read(
		t.InternalAddress, t.ReadMsg.AccessByteSize)
	if err != nil {
		panic(err)
	}

	dataReady := &mem.DataReadyRsp{}
	dataReady.ID = sim.GetIDGenerator().Generate()
	dataReady.Src = m.topPort.AsRemote()
	dataReady.Dst = t.ReadMsg.Src
	dataReady.Data = data
	dataReady.RspTo = t.ReadMsg.ID
	dataReady.TrafficBytes = len(data) + 4
	dataReady.TrafficClass = "mem.DataReadyRsp"

	sendErr := m.topPort.Send(dataReady)
	if sendErr == nil {
		state.TotalReadLatencyCycles += state.TickCount - t.ArrivalTick
		state.BytesRead += uint64(t.ReadMsg.AccessByteSize)
		state.CompletedReads++
		m.removeTransaction(state, i)
		return true
	}

	return false
}

// removeTransaction removes a transaction and remaps all references.
func (m *respondMW) removeTransaction(state *State, idx int) {
	// Remove the transaction
	state.Transactions = append(
		state.Transactions[:idx],
		state.Transactions[idx+1:]...,
	)

	// Remap sub-trans queue references
	newEntries := state.SubTransQueue.Entries[:0]
	for _, ref := range state.SubTransQueue.Entries {
		if ref.TransIndex == idx {
			continue // remove refs to deleted transaction
		}
		if ref.TransIndex > idx {
			ref.TransIndex--
		}
		newEntries = append(newEntries, ref)
	}
	state.SubTransQueue.Entries = newEntries

	// Remap command queue references
	newCmdEntries := state.CommandQueues.Entries[:0]
	for _, e := range state.CommandQueues.Entries {
		if e.Command.SubTransRef.TransIndex == idx {
			continue
		}
		if e.Command.SubTransRef.TransIndex > idx {
			e.Command.SubTransRef.TransIndex--
		}
		newCmdEntries = append(newCmdEntries, e)
	}
	state.CommandQueues.Entries = newCmdEntries

	// Remap bank current command references
	for i := range state.BankStates.Entries {
		bs := &state.BankStates.Entries[i].Data
		if bs.HasCurrentCmd {
			if bs.CurrentCmd.SubTransRef.TransIndex == idx {
				// The transaction is done, so this cmd should already
				// be completed. But just in case, update.
				bs.CurrentCmd.SubTransRef.TransIndex = -1
			} else if bs.CurrentCmd.SubTransRef.TransIndex > idx {
				bs.CurrentCmd.SubTransRef.TransIndex--
			}
		}
	}

	// Also update TransactionIndex in SubTransactions
	for ti := range state.Transactions {
		for si := range state.Transactions[ti].SubTransactions {
			state.Transactions[ti].SubTransactions[si].TransactionIndex = ti
		}
	}
}
