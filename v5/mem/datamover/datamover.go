package datamover

import (
	"io"
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

// A dataMoverTransaction contains a data moving request from a single
// source/destination with Read/Write requests correspond to it.
type dataMoverTransaction struct {
	req           *DataMoveRequest
	nextReadAddr  uint64
	nextWriteAddr uint64
	pendingRead   map[string]*mem.ReadReq
	pendingWrite  map[string]*mem.WriteReq
}

func newDataMoverTransaction(req *DataMoveRequest) *dataMoverTransaction {
	return &dataMoverTransaction{
		req:           req,
		nextReadAddr:  req.SrcAddress,
		nextWriteAddr: req.DstAddress,
		pendingRead:   make(map[string]*mem.ReadReq),
		pendingWrite:  make(map[string]*mem.WriteReq),
	}
}

func alignAddress(addr, granularity uint64) uint64 {
	return addr / granularity * granularity
}

func addressMustBeAligned(addr, granularity uint64) {
	if addr%granularity != 0 {
		log.Panicf("address %d must be aligned to %d", addr, granularity)
	}
}

// Comp helps moving data from designated source and destination
// following the given move direction
type Comp struct {
	*modeling.Component[Spec, State]

	ctrlPort    sim.Port
	insidePort  sim.Port
	outsidePort sim.Port

	insidePortMapper  mem.AddressToPortMapper
	outsidePortMapper mem.AddressToPortMapper

	// Runtime-only fields (not serialized)
	srcPort            sim.Port
	dstPort            sim.Port
	srcPortMapper      mem.AddressToPortMapper
	dstPortMapper      mem.AddressToPortMapper
	srcByteGranularity uint64
	dstByteGranularity uint64
	currentTransaction *dataMoverTransaction
	buffer             *buffer
}

// snapshotPortSide returns the side string for a port.
func (c *Comp) snapshotPortSide(port sim.Port) string {
	switch port {
	case c.insidePort:
		return "inside"
	case c.outsidePort:
		return "outside"
	default:
		return ""
	}
}

// snapshotBuffer converts the runtime buffer into a serializable bufferState.
func (c *Comp) snapshotBuffer() bufferState {
	bs := bufferState{
		Offset:      c.buffer.offset,
		Granularity: c.buffer.granularity,
	}
	for _, chunk := range c.buffer.data {
		if chunk == nil {
			bs.Chunks = append(bs.Chunks, dataChunk{Valid: false})
		} else {
			dataCopy := make([]byte, len(chunk))
			copy(dataCopy, chunk)
			bs.Chunks = append(bs.Chunks, dataChunk{Data: dataCopy, Valid: true})
		}
	}
	return bs
}

// snapshotTransaction converts the runtime transaction into a serializable state.
func (c *Comp) snapshotTransaction() dataMoverTransactionState {
	trans := c.currentTransaction
	ts := dataMoverTransactionState{
		Active:        true,
		ReqID:         trans.req.ID,
		ReqSrc:        trans.req.Src,
		ReqDst:        trans.req.Dst,
		SrcAddress:    trans.req.SrcAddress,
		DstAddress:    trans.req.DstAddress,
		ByteSize:      trans.req.ByteSize,
		SrcSide:       string(trans.req.SrcSide),
		DstSide:       string(trans.req.DstSide),
		NextReadAddr:  trans.nextReadAddr,
		NextWriteAddr: trans.nextWriteAddr,
		PendingRead:   make(map[string]pendingReadState, len(trans.pendingRead)),
		PendingWrite:  make(map[string]pendingWriteState, len(trans.pendingWrite)),
	}

	for id, msg := range trans.pendingRead {
		ts.PendingRead[id] = pendingReadState{
			ID: msg.ID, Src: msg.Src, Dst: msg.Dst,
			Address: msg.Address,
		}
	}

	for id, msg := range trans.pendingWrite {
		dataCopy := make([]byte, len(msg.Data))
		copy(dataCopy, msg.Data)
		ts.PendingWrite[id] = pendingWriteState{
			ID: msg.ID, Src: msg.Src, Dst: msg.Dst,
			Address: msg.Address, Data: dataCopy,
		}
	}

	return ts
}

// snapshotState converts the Comp's runtime state into a serializable State.
func (c *Comp) snapshotState() State {
	s := State{
		SrcByteGranularity: c.srcByteGranularity,
		DstByteGranularity: c.dstByteGranularity,
		SrcSide:            c.snapshotPortSide(c.srcPort),
		DstSide:            c.snapshotPortSide(c.dstPort),
	}

	if c.buffer != nil {
		s.Buffer = c.snapshotBuffer()
	}

	if c.currentTransaction != nil {
		s.CurrentTransaction = c.snapshotTransaction()
	}

	return s
}

// restorePortSide restores port and mapper from a side string.
func (c *Comp) restorePortSide(side string) (sim.Port, mem.AddressToPortMapper) {
	switch side {
	case "inside":
		return c.insidePort, c.insidePortMapper
	case "outside":
		return c.outsidePort, c.outsidePortMapper
	default:
		return nil, nil
	}
}

// restoreBuffer rebuilds the runtime buffer from a serializable bufferState.
func (c *Comp) restoreBuffer(bs bufferState) *buffer {
	buf := &buffer{
		offset:      bs.Offset,
		granularity: bs.Granularity,
	}
	for _, chunk := range bs.Chunks {
		if !chunk.Valid {
			buf.data = append(buf.data, nil)
		} else {
			dataCopy := make([]byte, len(chunk.Data))
			copy(dataCopy, chunk.Data)
			buf.data = append(buf.data, dataCopy)
		}
	}
	return buf
}

// restoreTransaction rebuilds the runtime transaction from its serializable state.
func (c *Comp) restoreTransaction(
	ts dataMoverTransactionState,
) *dataMoverTransaction {
	req := &DataMoveRequest{
		SrcAddress: ts.SrcAddress,
		DstAddress: ts.DstAddress,
		ByteSize:   ts.ByteSize,
		SrcSide:    DateMovePort(ts.SrcSide),
		DstSide:    DateMovePort(ts.DstSide),
	}
	req.ID = ts.ReqID
	req.Src = ts.ReqSrc
	req.Dst = ts.ReqDst

	trans := &dataMoverTransaction{
		req:           req,
		nextReadAddr:  ts.NextReadAddr,
		nextWriteAddr: ts.NextWriteAddr,
		pendingRead:   make(map[string]*mem.ReadReq, len(ts.PendingRead)),
		pendingWrite:  make(map[string]*mem.WriteReq, len(ts.PendingWrite)),
	}

	for id, ps := range ts.PendingRead {
		r := &mem.ReadReq{
			Address:        ps.Address,
			AccessByteSize: c.srcByteGranularity,
		}
		r.ID = ps.ID
		r.Src = ps.Src
		r.Dst = ps.Dst
		trans.pendingRead[id] = r
	}

	for id, ps := range ts.PendingWrite {
		dataCopy := make([]byte, len(ps.Data))
		copy(dataCopy, ps.Data)
		w := &mem.WriteReq{
			Address: ps.Address,
			Data:    dataCopy,
		}
		w.ID = ps.ID
		w.Src = ps.Src
		w.Dst = ps.Dst
		trans.pendingWrite[id] = w
	}

	return trans
}

// restoreFromState restores the Comp's runtime state from a serializable State.
func (c *Comp) restoreFromState(s State) {
	c.srcByteGranularity = s.SrcByteGranularity
	c.dstByteGranularity = s.DstByteGranularity

	c.srcPort, c.srcPortMapper = c.restorePortSide(s.SrcSide)
	c.dstPort, c.dstPortMapper = c.restorePortSide(s.DstSide)

	c.buffer = c.restoreBuffer(s.Buffer)

	if !s.CurrentTransaction.Active {
		c.currentTransaction = nil
		return
	}

	c.currentTransaction = c.restoreTransaction(s.CurrentTransaction)
}

// GetState converts runtime mutable data into a serializable State.
func (c *Comp) GetState() State {
	state := c.snapshotState()
	c.Component.SetState(state)
	return state
}

// SetState restores runtime mutable data from a serializable State.
func (c *Comp) SetState(state State) {
	c.Component.SetState(state)
	c.restoreFromState(state)
}

// SaveState marshals the component's spec and state as JSON, ensuring the
// runtime fields are synced into State first.
func (c *Comp) SaveState(w io.Writer) error {
	c.GetState()
	return c.Component.SaveState(w)
}

// LoadState reads JSON from r and restores both the base state and the
// runtime fields.
func (c *Comp) LoadState(r io.Reader) error {
	if err := c.Component.LoadState(r); err != nil {
		return err
	}
	c.SetState(c.Component.GetState())
	return nil
}

// dataMoverMiddleware wraps the Comp and implements the Tick() logic
// as a middleware.
type dataMoverMiddleware struct {
	*Comp
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

// parseFromCP retrieves Msg from ctrlPort
func (m *dataMoverMiddleware) parseFromCP() bool {
	reqI := m.ctrlPort.RetrieveIncoming()
	if reqI == nil {
		return false
	}

	req, ok := reqI.(*DataMoveRequest)
	if !ok {
		log.Panicf("can't process request of type %s", reflect.TypeOf(reqI))
	}

	if m.currentTransaction != nil {
		return false
	}

	trans := newDataMoverTransaction(req)
	m.currentTransaction = trans

	m.setSrcSide(req)
	m.setDstSide(req)

	m.buffer = &buffer{
		granularity: m.srcByteGranularity,
	}

	tracing.TraceReqReceive(req, m)

	return true
}

// readFromSrc reads data from source
func (m *dataMoverMiddleware) readFromSrc() bool {
	if m.currentTransaction == nil {
		return false
	}

	trans := m.currentTransaction
	addr := alignAddress(trans.nextReadAddr, m.srcByteGranularity)

	spec := m.Component.GetSpec()
	bufEndAddr := m.buffer.offset + spec.BufferSize
	if addr >= bufEndAddr {
		return false
	}

	transEndAddr := trans.req.SrcAddress + trans.req.ByteSize
	if addr > transEndAddr {
		return false
	}

	req := &mem.ReadReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Address = addr
	req.Src = m.srcPort.AsRemote()
	req.Dst = m.srcPortMapper.Find(addr)
	req.AccessByteSize = m.srcByteGranularity
	req.PID = 0
	req.TrafficBytes = 12
	req.TrafficClass = "mem.ReadReq"

	err := m.srcPort.Send(req)
	if err != nil {
		return false
	}

	trans.nextReadAddr += m.srcByteGranularity
	trans.pendingRead[req.ID] = req

	tracing.TraceReqInitiate(req, m, tracing.MsgIDAtReceiver(trans.req, m))

	return true
}

// processDataReadyFromSrc processes data ready from source
func (m *dataMoverMiddleware) processDataReadyFromSrc() bool {
	if m.currentTransaction == nil {
		return false
	}

	rspI := m.srcPort.PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp, ok := rspI.(*mem.DataReadyRsp)
	if !ok {
		// it can be write done rsp if src and dst is the same side. So ignore.
		return false
	}

	originalReq, ok := m.currentTransaction.pendingRead[rsp.RspTo]
	if !ok {
		log.Panicf("can't find original request for response %s",
			rsp.RspTo)
	}

	offset := originalReq.Address - m.currentTransaction.req.SrcAddress
	m.buffer.addData(offset, rsp.Data)

	delete(m.currentTransaction.pendingRead, rsp.RspTo)
	m.srcPort.RetrieveIncoming()
	tracing.TraceReqFinalize(originalReq, m)

	return true
}

// writeToDst sends data to destination
func (m *dataMoverMiddleware) writeToDst() bool {
	if m.currentTransaction == nil {
		return false
	}

	trans := m.currentTransaction
	offset := trans.nextWriteAddr - trans.req.DstAddress
	data, ok := m.buffer.extractData(offset, m.dstByteGranularity)

	if !ok {
		return false
	}

	req := &mem.WriteReq{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Address = m.currentTransaction.nextWriteAddr
	req.Data = data
	req.Src = m.dstPort.AsRemote()
	req.Dst = m.dstPortMapper.Find(m.currentTransaction.nextWriteAddr)
	req.PID = 0
	req.TrafficBytes = len(data) + 12
	req.TrafficClass = "mem.WriteReq"

	err := m.dstPort.Send(req)
	if err != nil {
		return false
	}

	m.currentTransaction.nextWriteAddr += m.dstByteGranularity
	m.currentTransaction.pendingWrite[req.ID] = req
	m.buffer.moveOffsetForwardTo(trans.nextWriteAddr - trans.req.DstAddress)

	tracing.TraceReqInitiate(req, m,
		tracing.MsgIDAtReceiver(m.currentTransaction.req, m))

	return true
}

// processWriteDoneFromDst processes write done from destination
func (m *dataMoverMiddleware) processWriteDoneFromDst() bool {
	if m.currentTransaction == nil {
		return false
	}

	rspI := m.dstPort.PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp, ok := rspI.(*mem.WriteDoneRsp)
	if !ok {
		return false
	}

	originalReq, ok := m.currentTransaction.pendingWrite[rsp.RspTo]
	if !ok {
		log.Panicf("can't find original request for response %s",
			rsp.RspTo)
	}

	delete(m.currentTransaction.pendingWrite, rsp.RspTo)
	m.dstPort.RetrieveIncoming()

	tracing.TraceReqFinalize(originalReq, m)

	return false
}

// finishTransaction finishes the current transaction
func (m *dataMoverMiddleware) finishTransaction() bool {
	if m.currentTransaction == nil {
		return false
	}

	trans := m.currentTransaction

	if trans.nextWriteAddr < trans.req.DstAddress+trans.req.ByteSize {
		return false
	}

	rsp := &DataMoveResponse{
		MsgMeta: sim.MsgMeta{
			ID:    sim.GetIDGenerator().Generate(),
			Src:   trans.req.Dst,
			Dst:   trans.req.Src,
			RspTo: trans.req.ID,
		},
	}

	err := m.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	m.currentTransaction = nil
	m.buffer = &buffer{
		offset:      alignAddress(trans.req.SrcAddress, m.srcByteGranularity),
		granularity: m.srcByteGranularity,
	}

	tracing.TraceReqComplete(rsp, m)

	return true
}

func (m *dataMoverMiddleware) setSrcSide(moveReq *DataMoveRequest) {
	spec := m.Component.GetSpec()
	switch moveReq.SrcSide {
	case "inside":
		m.srcPort = m.insidePort
		m.srcPortMapper = m.insidePortMapper
		m.srcByteGranularity = spec.InsideByteGranularity
	case "outside":
		m.srcPort = m.outsidePort
		m.srcPortMapper = m.outsidePortMapper
		m.srcByteGranularity = spec.OutsideByteGranularity
	default:
		log.Panicf("can't process source port of type %s", moveReq.SrcSide)
	}

	addressMustBeAligned(moveReq.SrcAddress, m.srcByteGranularity)
}

func (m *dataMoverMiddleware) setDstSide(moveReq *DataMoveRequest) {
	spec := m.Component.GetSpec()
	switch moveReq.DstSide {
	case "inside":
		m.dstPort = m.insidePort
		m.dstPortMapper = m.insidePortMapper
		m.dstByteGranularity = spec.InsideByteGranularity
	case "outside":
		m.dstPort = m.outsidePort
		m.dstPortMapper = m.outsidePortMapper
		m.dstByteGranularity = spec.OutsideByteGranularity
	default:
		log.Panicf("can't process destination port of type %s", moveReq.DstSide)
	}

	addressMustBeAligned(moveReq.DstAddress, m.dstByteGranularity)
}
