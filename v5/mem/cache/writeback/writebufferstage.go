package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type writeBufferStage struct {
	cache *middleware

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
	item := wb.cache.writeBufferBuffer.Peek()
	if item == nil {
		return false
	}

	trans := item.(*transactionState)
	switch trans.action {
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
		if e.evictingAddr == trans.fetchAddress {
			trans.fetchedData = e.evictingData
			return true
		}
	}

	for _, e := range wb.pendingEvictions {
		if e.evictingAddr == trans.fetchAddress {
			trans.fetchedData = e.evictingData
			return true
		}
	}

	return false
}

func (wb *writeBufferStage) sendFetchedDataToBank(
	trans *transactionState,
) bool {
	bankNum := bankID(trans.blockSetID, trans.blockWayID,
		wb.cache.wayAssociativity,
		len(wb.cache.dirToBankBuffers))
	bankBuf := wb.cache.writeBufferToBankBuffers[bankNum]

	if !bankBuf.CanPush() {
		trans.fetchedData = nil
		return false
	}

	if !trans.hasMSHREntry {
		panic("sendFetchedDataToBank without MSHR entry")
	}

	mshrEntry := &wb.cache.mshrState.Entries[trans.mshrEntryIndex]
	mshrEntry.Data = trans.fetchedData
	trans.action = bankWriteFetched
	wb.combineData(trans.mshrEntryIndex)

	// Resolve MSHR transaction pointers before removal (indices shift after remove)
	trans.mshrData = make([]byte, len(mshrEntry.Data))
	copy(trans.mshrData, mshrEntry.Data)
	trans.mshrTransactions = wb.resolveEntryTransactions(mshrEntry)

	cache.MSHRRemove(&wb.cache.mshrState,
		vm.PID(mshrEntry.PID), mshrEntry.Address)

	bankBuf.Push(trans)

	wb.cache.writeBufferBuffer.Pop()

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

	lowModulePort := wb.cache.addressToPortMapper.Find(trans.fetchAddress)
	read := &mem.ReadReq{}
	read.ID = sim.GetIDGenerator().Generate()
	read.Src = wb.cache.bottomPort.AsRemote()
	read.Dst = lowModulePort
	read.PID = trans.fetchPID
	read.Address = trans.fetchAddress
	read.AccessByteSize = 1 << wb.cache.log2BlockSize
	read.TrafficBytes = 12
	read.TrafficClass = "mem.ReadReq"
	wb.cache.bottomPort.Send(read)

	trans.fetchReadReq = read
	wb.inflightFetch = append(wb.inflightFetch, trans)
	wb.cache.writeBufferBuffer.Pop()

	tracing.TraceReqInitiate(read, wb.cache,
		tracing.MsgIDAtReceiver(trans.req(), wb.cache))

	return true
}

func (wb *writeBufferStage) processWriteBufferEvictAndWrite(
	trans *transactionState,
) bool {
	if wb.writeBufferFull() {
		return false
	}

	bankNum := bankID(
		trans.blockSetID, trans.blockWayID,
		wb.cache.wayAssociativity,
		len(wb.cache.dirToBankBuffers),
	)
	bankBuf := wb.cache.writeBufferToBankBuffers[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	trans.action = bankWriteHit
	bankBuf.Push(trans)

	wb.pendingEvictions = append(wb.pendingEvictions, trans)
	wb.cache.writeBufferBuffer.Pop()

	return true
}

func (wb *writeBufferStage) processWriteBufferFetchAndEvict(
	trans *transactionState,
) bool {
	ok := wb.processWriteBufferFlush(trans, false)
	if ok {
		trans.action = writeBufferFetch
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
		wb.cache.writeBufferBuffer.Pop()
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

	lowModulePort := wb.cache.addressToPortMapper.Find(trans.evictingAddr)
	write := &mem.WriteReq{}
	write.ID = sim.GetIDGenerator().Generate()
	write.Src = wb.cache.bottomPort.AsRemote()
	write.Dst = lowModulePort
	write.PID = trans.evictingPID
	write.Address = trans.evictingAddr
	write.Data = trans.evictingData
	write.DirtyMask = trans.evictingDirtyMask
	write.TrafficBytes = len(trans.evictingData) + 12
	write.TrafficClass = "mem.WriteReq"
	wb.cache.bottomPort.Send(write)

	trans.evictionWriteReq = write
	wb.pendingEvictions = wb.pendingEvictions[1:]
	wb.inflightEviction = append(wb.inflightEviction, trans)

	tracing.TraceReqInitiate(write, wb.cache,
		tracing.MsgIDAtReceiver(trans.req(), wb.cache))

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
	trans := wb.findInflightFetchByFetchReadReqID(msg.RspTo)
	bankIndex := bankID(
		trans.blockSetID, trans.blockWayID,
		wb.cache.wayAssociativity,
		len(wb.cache.dirToBankBuffers),
	)
	bankBuf := wb.cache.writeBufferToBankBuffers[bankIndex]

	if !bankBuf.CanPush() {
		return false
	}

	if !trans.hasMSHREntry {
		panic("processDataReadyRsp without MSHR entry")
	}

	trans.fetchedData = msg.Data
	trans.action = bankWriteFetched
	mshrEntry := &wb.cache.mshrState.Entries[trans.mshrEntryIndex]
	mshrEntry.Data = msg.Data
	wb.combineData(trans.mshrEntryIndex)

	// Resolve MSHR transaction pointers before removal (indices shift after remove)
	trans.mshrData = make([]byte, len(mshrEntry.Data))
	copy(trans.mshrData, mshrEntry.Data)
	trans.mshrTransactions = wb.resolveEntryTransactions(mshrEntry)

	cache.MSHRRemove(&wb.cache.mshrState,
		vm.PID(mshrEntry.PID), mshrEntry.Address)

	bankBuf.Push(trans)

	wb.removeInflightFetch(trans)
	wb.cache.bottomPort.RetrieveIncoming()

	tracing.TraceReqFinalize(trans.fetchReadReq, wb.cache)

	return true
}

func (wb *writeBufferStage) combineData(mshrIdx int) {
	mshrEntry := &wb.cache.mshrState.Entries[mshrIdx]
	block := &wb.cache.directoryState.Sets[mshrEntry.BlockSetID].Blocks[mshrEntry.BlockWayID]

	block.DirtyMask = make([]bool, 1<<wb.cache.log2BlockSize)

	for _, transIdx := range mshrEntry.TransactionIndices {
		if transIdx < 0 || transIdx >= len(wb.cache.inFlightTransactions) {
			continue
		}

		trans := wb.cache.inFlightTransactions[transIdx]
		if trans.read != nil {
			continue
		}

		block.IsDirty = true
		write := trans.write
		_, offset := getCacheLineID(write.Address, wb.cache.log2BlockSize)

		for i := 0; i < len(write.Data); i++ {
			if write.DirtyMask == nil || write.DirtyMask[i] {
				index := offset + uint64(i)
				mshrEntry.Data[index] = write.Data[i]
				block.DirtyMask[index] = true
			}
		}
	}
}

func (wb *writeBufferStage) findInflightFetchByFetchReadReqID(
	id string,
) *transactionState {
	for _, t := range wb.inflightFetch {
		if t.fetchReadReq.ID == id {
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
		if e.evictionWriteReq.ID == msg.RspTo {
			wb.inflightEviction = append(
				wb.inflightEviction[:i],
				wb.inflightEviction[i+1:]...,
			)
			wb.cache.bottomPort.RetrieveIncoming()
			tracing.TraceReqFinalize(e.evictionWriteReq, wb.cache)

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
	wb.cache.writeBufferBuffer.Clear()
}

// resolveEntryTransactions collects the actual transaction pointers from
// the MSHR entry's TransactionIndices. This must be done before any
// removeTransaction calls, since those shift the slice indices.
func (wb *writeBufferStage) resolveEntryTransactions(
	entry *cache.MSHREntryState,
) []*transactionState {
	result := make([]*transactionState, 0, len(entry.TransactionIndices))
	for _, transIdx := range entry.TransactionIndices {
		if transIdx >= 0 && transIdx < len(wb.cache.inFlightTransactions) {
			result = append(result, wb.cache.inFlightTransactions[transIdx])
		}
	}
	return result
}
