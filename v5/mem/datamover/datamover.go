package datamover

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the data mover.
type Spec struct {
	BufferSize             uint64 `json:"buffer_size"`
	InsideByteGranularity  uint64 `json:"inside_byte_granularity"`
	OutsideByteGranularity uint64 `json:"outside_byte_granularity"`

	InsideMapperKind             string           `json:"inside_mapper_kind"`
	InsideMapperPorts            []sim.RemotePort `json:"inside_mapper_ports"`
	InsideMapperInterleavingSize uint64           `json:"inside_mapper_interleaving_size"`

	OutsideMapperKind             string           `json:"outside_mapper_kind"`
	OutsideMapperPorts            []sim.RemotePort `json:"outside_mapper_ports"`
	OutsideMapperInterleavingSize uint64           `json:"outside_mapper_interleaving_size"`
}

// dataChunk wraps a single []byte slot. This avoids [][]byte which fails
// ValidateState. Valid distinguishes a nil slot from an empty one.
type dataChunk struct {
	Data  []byte `json:"data"`
	Valid bool   `json:"valid"`
}

// bufferState is the serializable representation of a buffer.
type bufferState struct {
	Offset      uint64      `json:"offset"`
	Granularity uint64      `json:"granularity"`
	Chunks      []dataChunk `json:"chunks"`
}

// pendingReadState captures the serializable fields of a pending read request.
type pendingReadState struct {
	ID      string         `json:"id"`
	Src     sim.RemotePort `json:"src"`
	Dst     sim.RemotePort `json:"dst"`
	Address uint64         `json:"address"`
}

// pendingWriteState captures the serializable fields of a pending write request.
type pendingWriteState struct {
	ID      string         `json:"id"`
	Src     sim.RemotePort `json:"src"`
	Dst     sim.RemotePort `json:"dst"`
	Address uint64         `json:"address"`
	Data    []byte         `json:"data"`
}

// dataMoverTransactionState is the serializable representation of a
// dataMoverTransaction.
type dataMoverTransactionState struct {
	ReqID         string                       `json:"req_id"`
	ReqSrc        sim.RemotePort               `json:"req_src"`
	ReqDst        sim.RemotePort               `json:"req_dst"`
	SrcAddress    uint64                       `json:"src_address"`
	DstAddress    uint64                       `json:"dst_address"`
	ByteSize      uint64                       `json:"byte_size"`
	SrcSide       string                       `json:"src_side"`
	DstSide       string                       `json:"dst_side"`
	NextReadAddr  uint64                       `json:"next_read_addr"`
	NextWriteAddr uint64                       `json:"next_write_addr"`
	PendingRead   map[string]pendingReadState  `json:"pending_read"`
	PendingWrite  map[string]pendingWriteState `json:"pending_write"`
	Active        bool                         `json:"active"`
}

// State contains mutable runtime data for the data mover.
type State struct {
	CurrentTransaction dataMoverTransactionState `json:"current_transaction"`
	Buffer             bufferState               `json:"buffer"`
	SrcByteGranularity uint64                    `json:"src_byte_granularity"`
	DstByteGranularity uint64                    `json:"dst_byte_granularity"`
	SrcSide            string                    `json:"src_side"`
	DstSide            string                    `json:"dst_side"`
}

func alignAddress(addr, granularity uint64) uint64 {
	return addr / granularity * granularity
}

func addressMustBeAligned(addr, granularity uint64) {
	if addr%granularity != 0 {
		log.Panicf("address %d must be aligned to %d", addr, granularity)
	}
}

// findPort resolves a port mapper lookup from Spec fields.
func findPort(
	kind string,
	ports []sim.RemotePort,
	interleavingSize uint64,
	addr uint64,
) sim.RemotePort {
	switch kind {
	case "single":
		return ports[0]
	case "interleaved":
		number := addr / interleavingSize % uint64(len(ports))
		return ports[number]
	default:
		log.Panicf("unknown mapper kind %q", kind)
		return ""
	}
}

// bufferAddData adds data to the buffer at the given offset.
func bufferAddData(bs *bufferState, offset uint64, data []byte) {
	addressMustBeAligned(offset, bs.Granularity)

	slot := (offset - bs.Offset) / bs.Granularity
	for i := uint64(len(bs.Chunks)); i <= slot; i++ {
		bs.Chunks = append(bs.Chunks, dataChunk{Valid: false})
	}

	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	bs.Chunks[slot] = dataChunk{Data: dataCopy, Valid: true}
}

// bufferExtractData extracts data from the buffer at the given offset.
func bufferExtractData(
	bs *bufferState, offset, size uint64,
) ([]byte, bool) {
	data := make([]byte, size)

	sizeLeft := size
	relOffset := offset - bs.Offset
	slot := relOffset / bs.Granularity
	slotOffset := relOffset - slot*bs.Granularity

	for i := slot; i < uint64(len(bs.Chunks)); i++ {
		if !bs.Chunks[i].Valid {
			return nil, false
		}

		bytesToCopy := bs.Granularity - slotOffset
		if sizeLeft < bytesToCopy {
			bytesToCopy = sizeLeft
		}

		copy(data[size-sizeLeft:],
			bs.Chunks[i].Data[slotOffset:slotOffset+bytesToCopy])
		sizeLeft -= bytesToCopy
		slotOffset = 0

		if sizeLeft == 0 {
			return data, true
		}
	}

	return nil, false
}

// bufferMoveOffsetForwardTo moves the buffer offset forward, discarding chunks.
func bufferMoveOffsetForwardTo(bs *bufferState, newOffset uint64) {
	alignedNewStart := (newOffset / bs.Granularity) * bs.Granularity

	if alignedNewStart <= bs.Offset {
		return
	}

	discardChunks := (alignedNewStart - bs.Offset) / bs.Granularity
	if discardChunks > uint64(len(bs.Chunks)) {
		bs.Chunks = bs.Chunks[:0]
	} else {
		bs.Chunks = bs.Chunks[discardChunks:]
	}

	bs.Offset = alignedNewStart
}

// Comp helps moving data from designated source and destination
// following the given move direction
type Comp struct {
	*modeling.Component[Spec, State]
}

// dataMoverMiddleware wraps the modeling.Component and implements the Tick()
// logic as a middleware.
type dataMoverMiddleware struct {
	comp *modeling.Component[Spec, State]
}

func (m *dataMoverMiddleware) Name() string {
	return m.comp.Name()
}

func (m *dataMoverMiddleware) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *dataMoverMiddleware) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *dataMoverMiddleware) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *dataMoverMiddleware) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

func (m *dataMoverMiddleware) ctrlPort() sim.Port {
	return m.comp.GetPortByName("Control")
}

func (m *dataMoverMiddleware) insidePort() sim.Port {
	return m.comp.GetPortByName("Inside")
}

func (m *dataMoverMiddleware) outsidePort() sim.Port {
	return m.comp.GetPortByName("Outside")
}

func (m *dataMoverMiddleware) srcPort() sim.Port {
	next := m.comp.GetNextState()
	switch next.SrcSide {
	case "inside":
		return m.insidePort()
	case "outside":
		return m.outsidePort()
	default:
		return nil
	}
}

func (m *dataMoverMiddleware) dstPort() sim.Port {
	next := m.comp.GetNextState()
	switch next.DstSide {
	case "inside":
		return m.insidePort()
	case "outside":
		return m.outsidePort()
	default:
		return nil
	}
}

func (m *dataMoverMiddleware) findSrcPort(addr uint64) sim.RemotePort {
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()
	switch next.SrcSide {
	case "inside":
		return findPort(spec.InsideMapperKind, spec.InsideMapperPorts,
			spec.InsideMapperInterleavingSize, addr)
	case "outside":
		return findPort(spec.OutsideMapperKind, spec.OutsideMapperPorts,
			spec.OutsideMapperInterleavingSize, addr)
	default:
		log.Panicf("unknown src side %q", next.SrcSide)
		return ""
	}
}

func (m *dataMoverMiddleware) findDstPort(addr uint64) sim.RemotePort {
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()
	switch next.DstSide {
	case "inside":
		return findPort(spec.InsideMapperKind, spec.InsideMapperPorts,
			spec.InsideMapperInterleavingSize, addr)
	case "outside":
		return findPort(spec.OutsideMapperKind, spec.OutsideMapperPorts,
			spec.OutsideMapperInterleavingSize, addr)
	default:
		log.Panicf("unknown dst side %q", next.DstSide)
		return ""
	}
}

// Tick ticks
func (m *dataMoverMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.finishTransaction() || madeProgress
	madeProgress = m.processWriteDoneFromDst() || madeProgress
	madeProgress = m.writeToDst() || madeProgress
	madeProgress = m.processDataReadyFromSrc() || madeProgress
	madeProgress = m.readFromSrc() || madeProgress
	madeProgress = m.parseFromCP() || madeProgress

	return madeProgress
}

// resolveByteGranularity returns the byte granularity for a given port side.
func resolveByteGranularity(spec Spec, side DateMovePort) uint64 {
	switch side {
	case "inside":
		return spec.InsideByteGranularity
	case "outside":
		return spec.OutsideByteGranularity
	default:
		log.Panicf("can't process port side %s", side)
		return 0
	}
}

// parseFromCP retrieves Msg from ctrlPort
func (m *dataMoverMiddleware) parseFromCP() bool {
	reqI := m.ctrlPort().RetrieveIncoming()
	if reqI == nil {
		return false
	}

	req, ok := reqI.(*DataMoveRequest)
	if !ok {
		log.Panicf("can't process request of type %s", reflect.TypeOf(reqI))
	}

	next := m.comp.GetNextState()
	if next.CurrentTransaction.Active {
		return false
	}

	spec := m.comp.GetSpec()

	srcByteGranularity := resolveByteGranularity(spec, req.SrcSide)
	addressMustBeAligned(req.SrcAddress, srcByteGranularity)

	dstByteGranularity := resolveByteGranularity(spec, req.DstSide)
	addressMustBeAligned(req.DstAddress, dstByteGranularity)

	next.SrcSide = string(req.SrcSide)
	next.DstSide = string(req.DstSide)
	next.SrcByteGranularity = srcByteGranularity
	next.DstByteGranularity = dstByteGranularity

	next.CurrentTransaction = dataMoverTransactionState{
		Active:        true,
		ReqID:         req.ID,
		ReqSrc:        req.Src,
		ReqDst:        req.Dst,
		SrcAddress:    req.SrcAddress,
		DstAddress:    req.DstAddress,
		ByteSize:      req.ByteSize,
		SrcSide:       string(req.SrcSide),
		DstSide:       string(req.DstSide),
		NextReadAddr:  req.SrcAddress,
		NextWriteAddr: req.DstAddress,
		PendingRead:   make(map[string]pendingReadState),
		PendingWrite:  make(map[string]pendingWriteState),
	}

	next.Buffer = bufferState{
		Granularity: srcByteGranularity,
	}

	tracing.TraceReqReceive(req, m)

	return true
}

// readFromSrc reads data from source
func (m *dataMoverMiddleware) readFromSrc() bool {
	next := m.comp.GetNextState()
	if !next.CurrentTransaction.Active {
		return false
	}

	trans := &next.CurrentTransaction
	addr := alignAddress(trans.NextReadAddr, next.SrcByteGranularity)

	spec := m.comp.GetSpec()
	bufEndAddr := next.Buffer.Offset + spec.BufferSize
	if addr >= bufEndAddr {
		return false
	}

	transEndAddr := trans.SrcAddress + trans.ByteSize
	if addr > transEndAddr {
		return false
	}

	srcP := m.srcPort()

	req := &mem.ReadReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Address = addr
	req.Src = srcP.AsRemote()
	req.Dst = m.findSrcPort(addr)
	req.AccessByteSize = next.SrcByteGranularity
	req.PID = 0
	req.TrafficBytes = 12
	req.TrafficClass = "mem.ReadReq"

	err := srcP.Send(req)
	if err != nil {
		return false
	}

	trans.NextReadAddr += next.SrcByteGranularity
	trans.PendingRead[req.ID] = pendingReadState{
		ID:      req.ID,
		Src:     req.Src,
		Dst:     req.Dst,
		Address: req.Address,
	}

	tracing.TraceReqInitiate(req, m,
		tracing.MsgIDAtReceiver(m.transactionAsMsg(trans), m))

	return true
}

// processDataReadyFromSrc processes data ready from source
func (m *dataMoverMiddleware) processDataReadyFromSrc() bool {
	next := m.comp.GetNextState()
	if !next.CurrentTransaction.Active {
		return false
	}

	srcP := m.srcPort()
	rspI := srcP.PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp, ok := rspI.(*mem.DataReadyRsp)
	if !ok {
		// it can be write done rsp if src and dst is the same side. So ignore.
		return false
	}

	trans := &next.CurrentTransaction
	originalReq, ok := trans.PendingRead[rsp.RspTo]
	if !ok {
		log.Panicf("can't find original request for response %s", rsp.RspTo)
	}

	offset := originalReq.Address - trans.SrcAddress
	bufferAddData(&next.Buffer, offset, rsp.Data)

	delete(trans.PendingRead, rsp.RspTo)
	srcP.RetrieveIncoming()

	// Create a temporary msg for tracing
	traceReq := &mem.ReadReq{}
	traceReq.ID = originalReq.ID
	traceReq.Src = originalReq.Src
	traceReq.Dst = originalReq.Dst
	tracing.TraceReqFinalize(traceReq, m)

	return true
}

// writeToDst sends data to destination
func (m *dataMoverMiddleware) writeToDst() bool {
	next := m.comp.GetNextState()
	if !next.CurrentTransaction.Active {
		return false
	}

	trans := &next.CurrentTransaction
	offset := trans.NextWriteAddr - trans.DstAddress
	data, ok := bufferExtractData(&next.Buffer, offset, next.DstByteGranularity)

	if !ok {
		return false
	}

	dstP := m.dstPort()

	req := &mem.WriteReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Address = trans.NextWriteAddr
	req.Data = data
	req.Src = dstP.AsRemote()
	req.Dst = m.findDstPort(trans.NextWriteAddr)
	req.PID = 0
	req.TrafficBytes = len(data) + 12
	req.TrafficClass = "mem.WriteReq"

	err := dstP.Send(req)
	if err != nil {
		return false
	}

	trans.NextWriteAddr += next.DstByteGranularity
	trans.PendingWrite[req.ID] = pendingWriteState{
		ID:      req.ID,
		Src:     req.Src,
		Dst:     req.Dst,
		Address: req.Address,
		Data:    data,
	}
	bufferMoveOffsetForwardTo(&next.Buffer, trans.NextWriteAddr-trans.DstAddress)

	tracing.TraceReqInitiate(req, m,
		tracing.MsgIDAtReceiver(m.transactionAsMsg(trans), m))

	return true
}

// processWriteDoneFromDst processes write done from destination
func (m *dataMoverMiddleware) processWriteDoneFromDst() bool {
	next := m.comp.GetNextState()
	if !next.CurrentTransaction.Active {
		return false
	}

	dstP := m.dstPort()
	rspI := dstP.PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp, ok := rspI.(*mem.WriteDoneRsp)
	if !ok {
		return false
	}

	trans := &next.CurrentTransaction
	originalReq, ok := trans.PendingWrite[rsp.RspTo]
	if !ok {
		log.Panicf("can't find original request for response %s", rsp.RspTo)
	}

	delete(trans.PendingWrite, rsp.RspTo)
	dstP.RetrieveIncoming()

	// Create a temporary msg for tracing
	traceReq := &mem.WriteReq{}
	traceReq.ID = originalReq.ID
	traceReq.Src = originalReq.Src
	traceReq.Dst = originalReq.Dst
	tracing.TraceReqFinalize(traceReq, m)

	return false
}

// finishTransaction finishes the current transaction
func (m *dataMoverMiddleware) finishTransaction() bool {
	next := m.comp.GetNextState()
	if !next.CurrentTransaction.Active {
		return false
	}

	trans := &next.CurrentTransaction

	if trans.NextWriteAddr < trans.DstAddress+trans.ByteSize {
		return false
	}

	rsp := &DataMoveResponse{
		MsgMeta: sim.MsgMeta{
			ID:    sim.GetIDGenerator().Generate(),
			Src:   trans.ReqDst,
			Dst:   trans.ReqSrc,
			RspTo: trans.ReqID,
		},
	}

	err := m.ctrlPort().Send(rsp)
	if err != nil {
		return false
	}

	// Reset transaction
	next.CurrentTransaction = dataMoverTransactionState{}
	next.Buffer = bufferState{
		Offset:      alignAddress(trans.SrcAddress, next.SrcByteGranularity),
		Granularity: next.SrcByteGranularity,
	}

	tracing.TraceReqComplete(rsp, m)

	return true
}

// transactionAsMsg creates a temporary DataMoveRequest for tracing purposes.
func (m *dataMoverMiddleware) transactionAsMsg(
	trans *dataMoverTransactionState,
) *DataMoveRequest {
	req := &DataMoveRequest{
		SrcAddress: trans.SrcAddress,
		DstAddress: trans.DstAddress,
		ByteSize:   trans.ByteSize,
		SrcSide:    DateMovePort(trans.SrcSide),
		DstSide:    DateMovePort(trans.DstSide),
	}
	req.ID = trans.ReqID
	req.Src = trans.ReqSrc
	req.Dst = trans.ReqDst
	return req
}
