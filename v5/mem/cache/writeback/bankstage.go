package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type bankStage struct {
	cache  *pipelineMW
	bankID int

	pipelineWidth      int
	inflightTransCount int

	// Count the trans that needs to be sent to the write buffer.
	downwardInflightTransCount int
}

func (s *bankStage) Tick() (madeProgress bool) {
	spec := s.cache.comp.GetSpec()

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = s.finalizeTrans() || madeProgress
	}

	madeProgress = s.tickPipeline() || madeProgress

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = s.pullFromBuf() || madeProgress
	}

	return madeProgress
}

func (s *bankStage) tickPipeline() bool {
	next := s.cache.comp.GetNextState()
	bankPipeline := &next.BankPipelines[s.bankID]
	bankPostBuf := &next.BankPostPipelineBufs[s.bankID]
	return bankPipeline.Tick(bankPostBuf)
}

func (s *bankStage) Reset() {
	next := s.cache.comp.GetNextState()
	next.DirToBankBufs[s.bankID].Clear()
	next.BankPipelines[s.bankID].Stages = nil
	next.BankPostPipelineBufs[s.bankID].Clear()
	s.inflightTransCount = 0
}

func (s *bankStage) pullFromBuf() bool {
	next := s.cache.comp.GetNextState()
	spec := s.cache.comp.GetSpec()

	if !s.canAcceptIntoPipeline(*next) {
		return false
	}

	// Check write buffer to bank buffer first
	wbBuf := &next.WriteBufferToBankBufs[s.bankID]
	if len(wbBuf.Elements) > 0 {
		transIdx, _ := wbBuf.PopTyped()
		s.acceptIntoPipeline(next, spec, transIdx)
		s.inflightTransCount++
		return true
	}

	// Do not jam the writeBufferBuffer
	if !next.WriteBufferBuf.CanPush() {
		return false
	}

	// Always reserve one lane for up-going transactions
	if s.downwardInflightTransCount >= s.pipelineWidth-1 {
		return false
	}

	return s.pullFromDirBuffer(next, spec)
}

func (s *bankStage) canAcceptIntoPipeline(cur State) bool {
	spec := s.cache.comp.GetSpec()

	if spec.BankLatency > 0 {
		return cur.BankPipelines[s.bankID].CanAccept()
	}

	// No pipeline - check post-buf capacity
	return cur.BankPostPipelineBufs[s.bankID].CanPush()
}

func (s *bankStage) pullFromDirBuffer(next *State, spec Spec) bool {
	dirBuf := &next.DirToBankBufs[s.bankID]
	if len(dirBuf.Elements) == 0 {
		return false
	}

	transIdx, _ := dirBuf.PopTyped()
	t := s.cache.inFlightTransactions[transIdx]

	if t.action == writeBufferFetch {
		next.WriteBufferBuf.PushTyped(transIdx)
		return true
	}

	s.acceptIntoPipeline(next, spec, transIdx)
	s.inflightTransCount++

	switch t.action {
	case bankEvict, bankEvictAndFetch, bankEvictAndWrite:
		s.downwardInflightTransCount++
	}

	return true
}

func (s *bankStage) acceptIntoPipeline(next *State, spec Spec, transIdx int) {
	if spec.BankLatency > 0 {
		next.BankPipelines[s.bankID].Accept(transIdx)
	} else {
		// Bypass pipeline: put directly in post-pipeline buffer
		next.BankPostPipelineBufs[s.bankID].PushTyped(transIdx)
	}
}

func (s *bankStage) finalizeTrans() bool {
	next := s.cache.comp.GetNextState()
	postBuf := &next.BankPostPipelineBufs[s.bankID]

	for i, idx := range postBuf.Elements {
		trans := s.cache.inFlightTransactions[idx]

		done := false

		switch trans.action {
		case bankReadHit:
			done = s.finalizeReadHit(trans)
		case bankWriteHit:
			done = s.finalizeWriteHit(trans)
		case bankWriteFetched:
			done = s.finalizeBankWriteFetched(trans)
		case bankEvictAndFetch, bankEvictAndWrite, bankEvict:
			done = s.finalizeBankEviction(trans)
		default:
			panic("bank action not supported")
		}

		if done {
			postBuf.Elements = append(postBuf.Elements[:i], postBuf.Elements[i+1:]...)
			return true
		}
	}

	return false
}

func (s *bankStage) finalizeReadHit(trans *transactionState) bool {
	if !s.cache.topPort.CanSend() {
		return false
	}

	spec := s.cache.comp.GetSpec()
	next := s.cache.comp.GetNextState()

	read := trans.read
	addr := read.Address
	_, offset := getCacheLineID(addr, spec.Log2BlockSize)
	nextBlock := &next.DirectoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]

	data, err := s.cache.storage.Read(
		nextBlock.CacheAddress+offset, read.AccessByteSize)
	if err != nil {
		panic(err)
	}

	s.removeTransaction(trans)

	s.inflightTransCount--
	s.downwardInflightTransCount--

	nextBlock.ReadCount--

	dataReady := &mem.DataReadyRsp{}
	dataReady.ID = sim.GetIDGenerator().Generate()
	dataReady.Src = s.cache.topPort.AsRemote()
	dataReady.Dst = read.Src
	dataReady.RspTo = read.ID
	dataReady.Data = data
	dataReady.TrafficBytes = len(data) + 4
	dataReady.TrafficClass = "mem.DataReadyRsp"
	s.cache.topPort.Send(dataReady)

	tracing.TraceReqComplete(read, s.cache.comp)

	return true
}

func (s *bankStage) finalizeWriteHit(trans *transactionState) bool {
	if !s.cache.topPort.CanSend() {
		return false
	}

	spec := s.cache.comp.GetSpec()
	next := s.cache.comp.GetNextState()

	write := trans.write
	addr := write.Address
	_, offset := getCacheLineID(addr, spec.Log2BlockSize)
	nextBlock := &next.DirectoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]

	dirtyMask := s.writeData(nextBlock, write, offset, spec.Log2BlockSize)

	nextBlock.IsValid = true
	nextBlock.IsLocked = false
	nextBlock.IsDirty = true
	nextBlock.DirtyMask = dirtyMask

	s.removeTransaction(trans)

	s.inflightTransCount--
	s.downwardInflightTransCount--

	done := &mem.WriteDoneRsp{}
	done.ID = sim.GetIDGenerator().Generate()
	done.Src = s.cache.topPort.AsRemote()
	done.Dst = trans.write.Src
	done.RspTo = trans.write.ID
	done.TrafficBytes = 4
	done.TrafficClass = "mem.WriteDoneRsp"
	s.cache.topPort.Send(done)

	tracing.TraceReqComplete(trans.write, s.cache.comp)

	return true
}

func (s *bankStage) writeData(
	block *cache.BlockState,
	write *mem.WriteReq,
	offset uint64,
	log2BlockSize uint64,
) []bool {
	data, err := s.cache.storage.Read(
		block.CacheAddress, 1<<log2BlockSize)
	if err != nil {
		panic(err)
	}

	dirtyMask := block.DirtyMask
	if dirtyMask == nil {
		dirtyMask = make([]bool, 1<<log2BlockSize)
	}

	for i := 0; i < len(write.Data); i++ {
		if write.DirtyMask == nil || write.DirtyMask[i] {
			index := offset + uint64(i)
			data[index] = write.Data[i]
			dirtyMask[index] = true
		}
	}

	err = s.cache.storage.Write(block.CacheAddress, data)
	if err != nil {
		panic(err)
	}

	return dirtyMask
}

func (s *bankStage) finalizeBankWriteFetched(
	trans *transactionState,
) bool {
	next := s.cache.comp.GetNextState()
	mshrBuf := &next.MSHRStageBuf

	if !mshrBuf.CanPush() {
		return false
	}

	nextBlock := &next.DirectoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]

	// Push the transaction index to MSHR stage
	transIdx := s.findTransIdx(trans)
	mshrBuf.PushTyped(transIdx)

	err := s.cache.storage.Write(nextBlock.CacheAddress, trans.mshrData)
	if err != nil {
		panic(err)
	}
	nextBlock.IsLocked = false
	nextBlock.IsValid = true

	s.inflightTransCount--

	return true
}

func (s *bankStage) removeTransaction(trans *transactionState) {
	for i, t := range s.cache.inFlightTransactions {
		if trans == t {
			s.cache.inFlightTransactions[i] = nil
			return
		}
	}

	now := s.cache.comp.Engine.CurrentTime()

	fmt.Printf("%.10f, %s, Transaction %s not found\n",
		now, s.cache.comp.Name(), trans.id)

	panic("transaction not found")
}

func (s *bankStage) finalizeBankEviction(
	trans *transactionState,
) bool {
	spec := s.cache.comp.GetSpec()
	next := s.cache.comp.GetNextState()
	wbBuf := &next.WriteBufferBuf

	if !wbBuf.CanPush() {
		return false
	}

	data, err := s.cache.storage.Read(
		trans.victimCacheAddress, 1<<spec.Log2BlockSize)
	if err != nil {
		panic(err)
	}

	trans.evictingData = data

	switch trans.action {
	case bankEvict:
		trans.action = writeBufferFlush
	case bankEvictAndFetch:
		trans.action = writeBufferEvictAndFetch
	case bankEvictAndWrite:
		trans.action = writeBufferEvictAndWrite
	default:
		panic("unsupported action")
	}

	delete(s.cache.evictingList, trans.evictingAddr)

	transIdx := s.findTransIdx(trans)
	wbBuf.PushTyped(transIdx)

	s.inflightTransCount--
	s.downwardInflightTransCount--

	return true
}

func (s *bankStage) findTransIdx(trans *transactionState) int {
	for i, t := range s.cache.inFlightTransactions {
		if t == trans {
			return i
		}
	}
	panic("transaction not found in inFlightTransactions")
}
