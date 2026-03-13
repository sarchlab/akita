package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type writeBufferStage struct {
	cache *pipelineMW

	writeBufferCapacity int
	maxInflightFetch    int
	maxInflightEviction int

	pendingEvictions []*transactionState
	inflightFetch    []*transactionState
	inflightEviction []*transactionState
}

func (wb *writeBufferStage) Tick() bool {
	madeProgress := false

	madeProgress = wb.write() || madeProgress
	madeProgress = wb.processReturnRsp() || madeProgress
	madeProgress = wb.processNewTransaction() || madeProgress

	return madeProgress
}

func (wb *writeBufferStage) processNewTransaction() bool {
	next := wb.cache.comp.GetNextState()
	wbBuf := &next.WriteBufferBuf

	item := wbBuf.Peek()
	if item == nil {
		return false
	}

	transIdx := item.(int)
	trans := wb.cache.inFlightTransactions[transIdx]

	switch trans.Action {
	case writeBufferFetch:
		return wb.processWriteBufferFetch(trans)
	case writeBufferEvictAndWrite:
		return wb.processWriteBufferEvictAndWrite(trans)
	case writeBufferEvictAndFetch:
		return wb.processWriteBufferFetchAndEvict(trans)
	case writeBufferFlush:
		return wb.processWriteBufferFlush(trans, true)
	default:
		panic("unknown transaction action")
	}
}

func (wb *writeBufferStage) processWriteBufferFetch(
	trans *transactionState,
) bool {
	if wb.findDataLocally(trans) {
		return wb.sendFetchedDataToBank(trans)
	}

	return wb.fetchFromBottom(trans)
}

func (wb *writeBufferStage) findDataLocally(trans *transactionState) bool {
	for _, e := range wb.inflightEviction {
		if e.EvictingAddr == trans.FetchAddress {
			trans.FetchedData = e.EvictingData
			return true
		}
	}

	for _, e := range wb.pendingEvictions {
		if e.EvictingAddr == trans.FetchAddress {
			trans.FetchedData = e.EvictingData
			return true
		}
	}

	return false
}

func (wb *writeBufferStage) sendFetchedDataToBank(
	trans *transactionState,
) bool {
	spec := wb.cache.comp.GetSpec()
	next := wb.cache.comp.GetNextState()
	bankNum := bankID(trans.BlockSetID, trans.BlockWayID,
		spec.WayAssociativity,
		len(next.WriteBufferToBankBufs))
	bankBuf := &next.WriteBufferToBankBufs[bankNum]

	if !bankBuf.CanPush() {
		trans.FetchedData = nil
		return false
	}

	if !trans.HasMSHREntry {
		panic("sendFetchedDataToBank without MSHR entry")
	}

	mshrIdx := wb.lookupMSHRIndex(trans)
	mshrEntry := &next.MSHRState.Entries[mshrIdx]
	mshrEntry.Data = trans.FetchedData
	trans.Action = bankWriteFetched
	wb.combineData(mshrIdx)

	// Resolve MSHR transaction pointers before removal
	trans.MSHRData = make([]byte, len(mshrEntry.Data))
	copy(trans.MSHRData, mshrEntry.Data)
	trans.MSHRTransactionIndices = wb.resolveEntryTransactionIndices(mshrEntry)

	cache.MSHRRemove(&next.MSHRState,
		vm.PID(mshrEntry.PID), mshrEntry.Address)

	transIdx := wb.findTransIdx(trans)
	bankBuf.PushTyped(transIdx)

	next.WriteBufferBuf.Pop()

	return true
}

func (wb *writeBufferStage) fetchFromBottom(
	trans *transactionState,
) bool {
	if wb.tooManyInflightFetches() {
		return false
	}

	if !wb.cache.bottomPort.CanSend() {
		return false
	}

	spec := wb.cache.comp.GetSpec()
	lowModulePort := wb.cache.findPort(trans.FetchAddress)
	read := &mem.ReadReq{}
	read.ID = sim.GetIDGenerator().Generate()
	read.Src = wb.cache.bottomPort.AsRemote()
	read.Dst = lowModulePort
	read.PID = trans.FetchPID
	read.Address = trans.FetchAddress
	read.AccessByteSize = 1 << spec.Log2BlockSize
	read.TrafficBytes = 12
	read.TrafficClass = "mem.ReadReq"
	wb.cache.bottomPort.Send(read)

	trans.HasFetchReadReq = true
	trans.FetchReadReqMeta = read.MsgMeta
	wb.inflightFetch = append(wb.inflightFetch, trans)

	next := wb.cache.comp.GetNextState()
	next.WriteBufferBuf.Pop()

	reqMeta := trans.reqMeta()
	tracing.TraceReqInitiate(read, wb.cache.comp,
		tracing.MsgIDAtReceiver(&reqMeta, wb.cache.comp))

	return true
}

func (wb *writeBufferStage) processWriteBufferEvictAndWrite(
	trans *transactionState,
) bool {
	if wb.writeBufferFull() {
		return false
	}

	spec := wb.cache.comp.GetSpec()
	next := wb.cache.comp.GetNextState()
	bankNum := bankID(
		trans.BlockSetID, trans.BlockWayID,
		spec.WayAssociativity,
		len(next.WriteBufferToBankBufs),
	)
	bankBuf := &next.WriteBufferToBankBufs[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	trans.Action = bankWriteHit
	transIdx := wb.findTransIdx(trans)
	bankBuf.PushTyped(transIdx)

	wb.pendingEvictions = append(wb.pendingEvictions, trans)
	next.WriteBufferBuf.Pop()

	return true
}

func (wb *writeBufferStage) processWriteBufferFetchAndEvict(
	trans *transactionState,
) bool {
	ok := wb.processWriteBufferFlush(trans, false)
	if ok {
		trans.Action = writeBufferFetch
		return true
	}

	return false
}

func (wb *writeBufferStage) processWriteBufferFlush(
	trans *transactionState,
	popAfterDone bool,
) bool {
	if wb.writeBufferFull() {
		return false
	}

	wb.pendingEvictions = append(wb.pendingEvictions, trans)

	if popAfterDone {
		next := wb.cache.comp.GetNextState()
		next.WriteBufferBuf.Pop()
	}

	return true
}

func (wb *writeBufferStage) write() bool {
	if len(wb.pendingEvictions) == 0 {
		return false
	}

	trans := wb.pendingEvictions[0]

	if wb.tooManyInflightEvictions() {
		return false
	}

	if !wb.cache.bottomPort.CanSend() {
		return false
	}

	lowModulePort := wb.cache.findPort(trans.EvictingAddr)
	write := &mem.WriteReq{}
	write.ID = sim.GetIDGenerator().Generate()
	write.Src = wb.cache.bottomPort.AsRemote()
	write.Dst = lowModulePort
	write.PID = trans.EvictingPID
	write.Address = trans.EvictingAddr
	write.Data = trans.EvictingData
	write.DirtyMask = trans.EvictingDirtyMask
	write.TrafficBytes = len(trans.EvictingData) + 12
	write.TrafficClass = "mem.WriteReq"
	wb.cache.bottomPort.Send(write)

	trans.HasEvictionWriteReq = true
	trans.EvictionWriteReqMeta = write.MsgMeta
	wb.pendingEvictions = wb.pendingEvictions[1:]
	wb.inflightEviction = append(wb.inflightEviction, trans)

	reqMeta := trans.reqMeta()
	tracing.TraceReqInitiate(write, wb.cache.comp,
		tracing.MsgIDAtReceiver(&reqMeta, wb.cache.comp))

	return true
}

func (wb *writeBufferStage) processReturnRsp() bool {
	msg := wb.cache.bottomPort.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *mem.DataReadyRsp:
		return wb.processDataReadyRsp(msg)
	case *mem.WriteDoneRsp:
		return wb.processWriteDoneRsp(msg)
	default:
		panic("unknown msg type")
	}
}

func (wb *writeBufferStage) processDataReadyRsp(
	msg *mem.DataReadyRsp,
) bool {
	spec := wb.cache.comp.GetSpec()
	next := wb.cache.comp.GetNextState()

	trans := wb.findInflightFetchByFetchReadReqID(msg.RspTo)
	bankIndex := bankID(
		trans.BlockSetID, trans.BlockWayID,
		spec.WayAssociativity,
		len(next.WriteBufferToBankBufs),
	)
	bankBuf := &next.WriteBufferToBankBufs[bankIndex]

	if !bankBuf.CanPush() {
		return false
	}

	if !trans.HasMSHREntry {
		panic("processDataReadyRsp without MSHR entry")
	}

	mshrIdx := wb.lookupMSHRIndex(trans)
	trans.FetchedData = msg.Data
	trans.Action = bankWriteFetched
	mshrEntry := &next.MSHRState.Entries[mshrIdx]
	mshrEntry.Data = msg.Data
	wb.combineData(mshrIdx)

	// Resolve MSHR transaction pointers before removal
	trans.MSHRData = make([]byte, len(mshrEntry.Data))
	copy(trans.MSHRData, mshrEntry.Data)
	trans.MSHRTransactionIndices = wb.resolveEntryTransactionIndices(mshrEntry)

	cache.MSHRRemove(&next.MSHRState,
		vm.PID(mshrEntry.PID), mshrEntry.Address)

	transIdx := wb.findTransIdx(trans)
	bankBuf.PushTyped(transIdx)

	wb.removeInflightFetch(trans)
	wb.cache.bottomPort.RetrieveIncoming()

	tracing.TraceReqFinalize(&trans.FetchReadReqMeta, wb.cache.comp)

	return true
}

func (wb *writeBufferStage) combineData(mshrIdx int) {
	spec := wb.cache.comp.GetSpec()
	next := wb.cache.comp.GetNextState()
	mshrEntry := &next.MSHRState.Entries[mshrIdx]
	block := &next.DirectoryState.Sets[mshrEntry.BlockSetID].Blocks[mshrEntry.BlockWayID]

	block.DirtyMask = make([]bool, 1<<spec.Log2BlockSize)

	for _, transIdx := range mshrEntry.TransactionIndices {
		if transIdx < 0 || transIdx >= len(wb.cache.inFlightTransactions) {
			continue
		}

		trans := wb.cache.inFlightTransactions[transIdx]
		if trans.HasRead {
			continue
		}

		block.IsDirty = true
		_, offset := getCacheLineID(trans.WriteAddress, spec.Log2BlockSize)

		for i := 0; i < len(trans.WriteData); i++ {
			if trans.WriteDirtyMask == nil || trans.WriteDirtyMask[i] {
				index := offset + uint64(i)
				mshrEntry.Data[index] = trans.WriteData[i]
				block.DirtyMask[index] = true
			}
		}
	}
}

func (wb *writeBufferStage) findInflightFetchByFetchReadReqID(
	id string,
) *transactionState {
	for _, t := range wb.inflightFetch {
		if t.FetchReadReqMeta.ID == id {
			return t
		}
	}

	panic("inflight read not found")
}

func (wb *writeBufferStage) removeInflightFetch(f *transactionState) {
	for i, trans := range wb.inflightFetch {
		if trans == f {
			wb.inflightFetch = append(
				wb.inflightFetch[:i],
				wb.inflightFetch[i+1:]...,
			)

			return
		}
	}

	panic("not found")
}

func (wb *writeBufferStage) processWriteDoneRsp(
	msg *mem.WriteDoneRsp,
) bool {
	for i := len(wb.inflightEviction) - 1; i >= 0; i-- {
		e := wb.inflightEviction[i]
		if e.EvictionWriteReqMeta.ID == msg.RspTo {
			wb.inflightEviction = append(
				wb.inflightEviction[:i],
				wb.inflightEviction[i+1:]...,
			)
			wb.cache.bottomPort.RetrieveIncoming()
			tracing.TraceReqFinalize(&e.EvictionWriteReqMeta, wb.cache.comp)

			return true
		}
	}

	panic("write request not found")
}

func (wb *writeBufferStage) writeBufferFull() bool {
	numEntry := len(wb.pendingEvictions) + len(wb.inflightEviction)
	return numEntry >= wb.writeBufferCapacity
}

func (wb *writeBufferStage) tooManyInflightFetches() bool {
	return len(wb.inflightFetch) >= wb.maxInflightFetch
}

func (wb *writeBufferStage) tooManyInflightEvictions() bool {
	return len(wb.inflightEviction) >= wb.maxInflightEviction
}

func (wb *writeBufferStage) Reset() {
	next := wb.cache.comp.GetNextState()
	next.WriteBufferBuf.Clear()
}

// lookupMSHRIndex finds the current index of the MSHR entry for this
// transaction.  The stored MSHREntryIndex may be stale because MSHRRemove
// shifts entries, so we re-query by PID+Address.
func (wb *writeBufferStage) lookupMSHRIndex(trans *transactionState) int {
	next := wb.cache.comp.GetNextState()
	idx, found := cache.MSHRQuery(&next.MSHRState, trans.FetchPID, trans.FetchAddress)
	if !found {
		panic("lookupMSHRIndex: MSHR entry not found")
	}
	return idx
}

// resolveEntryTransactionIndices collects the transaction indices from
// the MSHR entry's TransactionIndices.
func (wb *writeBufferStage) resolveEntryTransactionIndices(
	entry *cache.MSHREntryState,
) []int {
	result := make([]int, 0, len(entry.TransactionIndices))
	for _, transIdx := range entry.TransactionIndices {
		if transIdx >= 0 && transIdx < len(wb.cache.inFlightTransactions) {
			result = append(result, transIdx)
		}
	}
	return result
}

// findTransIdx finds the index of trans in the inFlightTransactions list.
func (wb *writeBufferStage) findTransIdx(trans *transactionState) int {
	for i, t := range wb.cache.inFlightTransactions {
		if t == trans {
			return i
		}
	}
	panic("transaction not found in inFlightTransactions")
}
