package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type bankStage struct {
	cache  *middleware
	bankID int

	pipeline           queueing.Pipeline
	pipelineWidth      int
	postPipelineBuf    queueing.Buffer
	inflightTransCount int

	// Count the trans that needs to be sent to the write buffer.
	downwardInflightTransCount int
}

type bankPipelineElem struct {
	trans *transactionState
}

func (e bankPipelineElem) TaskID() string {
	return e.trans.req().Meta().ID + "_write_back_bank_pipeline"
}

func (s *bankStage) Tick() (madeProgress bool) {
	for i := 0; i < s.cache.numReqPerCycle; i++ {
		madeProgress = s.finalizeTrans() || madeProgress
	}

	madeProgress = s.pipeline.Tick() || madeProgress

	for i := 0; i < s.cache.numReqPerCycle; i++ {
		madeProgress = s.pullFromBuf() || madeProgress
	}

	return madeProgress
}

func (s *bankStage) Reset() {
	s.cache.dirToBankBuffers[s.bankID].Clear()
	s.pipeline.Clear()
	s.postPipelineBuf.Clear()
	s.inflightTransCount = 0
}

func (s *bankStage) pullFromBuf() bool {
	if !s.pipeline.CanAccept() {
		return false
	}

	inBuf := s.cache.writeBufferToBankBuffers[s.bankID]

	trans := inBuf.Pop()
	if trans != nil {
		s.pipeline.Accept(bankPipelineElem{trans: trans.(*transactionState)})

		s.inflightTransCount++

		return true
	}

	// Do not jam the writeBufferBuffer
	if !s.cache.writeBufferBuffer.CanPush() {
		return false
	}

	// Always reserve one lane for up-going transactions
	if s.downwardInflightTransCount >= s.pipelineWidth-1 {
		return false
	}

	inBuf = s.cache.dirToBankBuffers[s.bankID]
	trans = inBuf.Pop()

	if trans != nil {
		t := trans.(*transactionState)

		if t.action == writeBufferFetch {
			s.cache.writeBufferBuffer.Push(trans)
			return true
		}

		s.pipeline.Accept(bankPipelineElem{trans: t})

		s.inflightTransCount++

		switch t.action {
		case bankEvict, bankEvictAndFetch, bankEvictAndWrite:
			s.downwardInflightTransCount++
		}

		return true
	}

	return false
}

func (s *bankStage) finalizeTrans() bool {
	elems := queueing.SnapshotBuffer(s.postPipelineBuf)

	for i, e := range elems {
		trans := e.(bankPipelineElem).trans

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
			remaining := make([]interface{}, 0, len(elems)-1)
			remaining = append(remaining, elems[:i]...)
			remaining = append(remaining, elems[i+1:]...)
			queueing.RestoreBuffer(s.postPipelineBuf, remaining)

			return true
		}
	}

	return false
}

func (s *bankStage) finalizeReadHit(trans *transactionState) bool {
	if !s.cache.topPort.CanSend() {
		return false
	}

	read := trans.read
	addr := read.Address
	_, offset := getCacheLineID(addr, s.cache.log2BlockSize)
	block := &s.cache.directoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]

	data, err := s.cache.storage.Read(
		block.CacheAddress+offset, read.AccessByteSize)
	if err != nil {
		panic(err)
	}

	s.removeTransaction(trans)

	s.inflightTransCount--
	s.downwardInflightTransCount--
	block.ReadCount--

	dataReady := &mem.DataReadyRsp{}
	dataReady.ID = sim.GetIDGenerator().Generate()
	dataReady.Src = s.cache.topPort.AsRemote()
	dataReady.Dst = read.Src
	dataReady.RspTo = read.ID
	dataReady.Data = data
	dataReady.TrafficBytes = len(data) + 4
	dataReady.TrafficClass = "mem.DataReadyRsp"
	s.cache.topPort.Send(dataReady)

	tracing.TraceReqComplete(read, s.cache)

	return true
}

func (s *bankStage) finalizeWriteHit(trans *transactionState) bool {
	if !s.cache.topPort.CanSend() {
		return false
	}

	write := trans.write
	addr := write.Address
	_, offset := getCacheLineID(addr, s.cache.log2BlockSize)
	block := &s.cache.directoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]

	dirtyMask := s.writeData(block, write, offset)

	block.IsValid = true
	block.IsLocked = false
	block.IsDirty = true
	block.DirtyMask = dirtyMask

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

	tracing.TraceReqComplete(trans.write, s.cache)

	return true
}

func (s *bankStage) writeData(
	block *cache.BlockState,
	write *mem.WriteReq,
	offset uint64,
) []bool {
	data, err := s.cache.storage.Read(
		block.CacheAddress, 1<<s.cache.log2BlockSize)
	if err != nil {
		panic(err)
	}

	dirtyMask := block.DirtyMask
	if dirtyMask == nil {
		dirtyMask = make([]bool, 1<<s.cache.log2BlockSize)
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
	if !s.cache.mshrStageBuffer.CanPush() {
		return false
	}

	// Use block reference from the transaction itself (MSHR entry may have been removed)
	block := &s.cache.directoryState.Sets[trans.blockSetID].Blocks[trans.blockWayID]

	// Push the transaction itself to MSHR stage (it carries mshrTransactions and mshrData)
	s.cache.mshrStageBuffer.Push(trans)

	err := s.cache.storage.Write(block.CacheAddress, trans.mshrData)
	if err != nil {
		panic(err)
	}

	block.IsLocked = false
	block.IsValid = true

	s.inflightTransCount--

	return true
}

func (s *bankStage) removeTransaction(trans *transactionState) {
	for i, t := range s.cache.inFlightTransactions {
		if trans == t {
			s.cache.inFlightTransactions = append(
				(s.cache.inFlightTransactions)[:i],
				(s.cache.inFlightTransactions)[i+1:]...)

			return
		}
	}

	now := s.cache.comp.Engine.CurrentTime()

	fmt.Printf("%.10f, %s, Transaction %s not found\n",
		now, s.cache.Name(), trans.id)

	panic("transaction not found")
}

func (s *bankStage) finalizeBankEviction(
	trans *transactionState,
) bool {
	if !s.cache.writeBufferBuffer.CanPush() {
		return false
	}

	data, err := s.cache.storage.Read(
		trans.victimCacheAddress, 1<<s.cache.log2BlockSize)
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
	s.cache.writeBufferBuffer.Push(trans)

	s.inflightTransCount--
	s.downwardInflightTransCount--

	return true
}
