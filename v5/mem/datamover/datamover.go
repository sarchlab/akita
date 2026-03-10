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
	req           *sim.Msg // payload: *DataMoveRequestPayload
	reqPayload    *DataMoveRequestPayload
	nextReadAddr  uint64
	nextWriteAddr uint64
	pendingRead   map[string]*sim.Msg // payload: *mem.ReadReqPayload
	pendingWrite  map[string]*sim.Msg // payload: *mem.WriteReqPayload
}

func newDataMoverTransaction(req *sim.Msg) *dataMoverTransaction {
	payload := sim.MsgPayload[DataMoveRequestPayload](req)
	return &dataMoverTransaction{
		req:           req,
		reqPayload:    payload,
		nextReadAddr:  payload.SrcAddress,
		nextWriteAddr: payload.DstAddress,
		pendingRead:   make(map[string]*sim.Msg),
		pendingWrite:  make(map[string]*sim.Msg),
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

// snapshotState converts the Comp's runtime state into a serializable State.
func (c *Comp) snapshotState() State {
	s := State{
		SrcByteGranularity: c.srcByteGranularity,
		DstByteGranularity: c.dstByteGranularity,
	}

	// Determine src/dst side strings from port identity.
	if c.srcPort == c.insidePort {
		s.SrcSide = "inside"
	} else if c.srcPort == c.outsidePort {
		s.SrcSide = "outside"
	}

	if c.dstPort == c.insidePort {
		s.DstSide = "inside"
	} else if c.dstPort == c.outsidePort {
		s.DstSide = "outside"
	}

	// Snapshot buffer.
	if c.buffer != nil {
		s.Buffer = bufferState{
			Offset:      c.buffer.offset,
			Granularity: c.buffer.granularity,
		}
		for _, chunk := range c.buffer.data {
			if chunk == nil {
				s.Buffer.Chunks = append(s.Buffer.Chunks, dataChunk{
					Valid: false,
				})
			} else {
				dataCopy := make([]byte, len(chunk))
				copy(dataCopy, chunk)
				s.Buffer.Chunks = append(s.Buffer.Chunks, dataChunk{
					Data:  dataCopy,
					Valid: true,
				})
			}
		}
	}

	// Snapshot current transaction.
	if c.currentTransaction != nil {
		trans := c.currentTransaction
		ts := dataMoverTransactionState{
			Active:        true,
			ReqID:         trans.req.ID,
			ReqSrc:        trans.req.Src,
			ReqDst:        trans.req.Dst,
			SrcAddress:    trans.reqPayload.SrcAddress,
			DstAddress:    trans.reqPayload.DstAddress,
			ByteSize:      trans.reqPayload.ByteSize,
			SrcSide:       string(trans.reqPayload.SrcSide),
			DstSide:       string(trans.reqPayload.DstSide),
			NextReadAddr:  trans.nextReadAddr,
			NextWriteAddr: trans.nextWriteAddr,
			PendingRead:   make(map[string]pendingReadState, len(trans.pendingRead)),
			PendingWrite:  make(map[string]pendingWriteState, len(trans.pendingWrite)),
		}

		for id, msg := range trans.pendingRead {
			payload := sim.MsgPayload[mem.ReadReqPayload](msg)
			ts.PendingRead[id] = pendingReadState{
				ID:      msg.ID,
				Src:     msg.Src,
				Dst:     msg.Dst,
				Address: payload.Address,
			}
		}

		for id, msg := range trans.pendingWrite {
			payload := sim.MsgPayload[mem.WriteReqPayload](msg)
			dataCopy := make([]byte, len(payload.Data))
			copy(dataCopy, payload.Data)
			ts.PendingWrite[id] = pendingWriteState{
				ID:      msg.ID,
				Src:     msg.Src,
				Dst:     msg.Dst,
				Address: payload.Address,
				Data:    dataCopy,
			}
		}

		s.CurrentTransaction = ts
	}

	return s
}

// restoreFromState restores the Comp's runtime state from a serializable State.
func (c *Comp) restoreFromState(s State) {
	c.srcByteGranularity = s.SrcByteGranularity
	c.dstByteGranularity = s.DstByteGranularity

	// Restore port assignments from side strings.
	switch s.SrcSide {
	case "inside":
		c.srcPort = c.insidePort
		c.srcPortMapper = c.insidePortMapper
	case "outside":
		c.srcPort = c.outsidePort
		c.srcPortMapper = c.outsidePortMapper
	}

	switch s.DstSide {
	case "inside":
		c.dstPort = c.insidePort
		c.dstPortMapper = c.insidePortMapper
	case "outside":
		c.dstPort = c.outsidePort
		c.dstPortMapper = c.outsidePortMapper
	}

	// Restore buffer.
	buf := &buffer{
		offset:      s.Buffer.Offset,
		granularity: s.Buffer.Granularity,
	}
	for _, chunk := range s.Buffer.Chunks {
		if !chunk.Valid {
			buf.data = append(buf.data, nil)
		} else {
			dataCopy := make([]byte, len(chunk.Data))
			copy(dataCopy, chunk.Data)
			buf.data = append(buf.data, dataCopy)
		}
	}
	c.buffer = buf

	// Restore current transaction.
	if !s.CurrentTransaction.Active {
		c.currentTransaction = nil
		return
	}

	ts := s.CurrentTransaction
	payload := &DataMoveRequestPayload{
		SrcAddress: ts.SrcAddress,
		DstAddress: ts.DstAddress,
		ByteSize:   ts.ByteSize,
		SrcSide:    DateMovePort(ts.SrcSide),
		DstSide:    DateMovePort(ts.DstSide),
	}
	req := &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  ts.ReqID,
			Src: ts.ReqSrc,
			Dst: ts.ReqDst,
		},
		Payload: payload,
	}

	trans := &dataMoverTransaction{
		req:           req,
		reqPayload:    payload,
		nextReadAddr:  ts.NextReadAddr,
		nextWriteAddr: ts.NextWriteAddr,
		pendingRead:   make(map[string]*sim.Msg, len(ts.PendingRead)),
		pendingWrite:  make(map[string]*sim.Msg, len(ts.PendingWrite)),
	}

	for id, ps := range ts.PendingRead {
		msg := &sim.Msg{
			MsgMeta: sim.MsgMeta{
				ID:  ps.ID,
				Src: ps.Src,
				Dst: ps.Dst,
			},
			Payload: &mem.ReadReqPayload{
				Address:        ps.Address,
				AccessByteSize: c.srcByteGranularity,
			},
		}
		trans.pendingRead[id] = msg
	}

	for id, ps := range ts.PendingWrite {
		dataCopy := make([]byte, len(ps.Data))
		copy(dataCopy, ps.Data)
		msg := &sim.Msg{
			MsgMeta: sim.MsgMeta{
				ID:  ps.ID,
				Src: ps.Src,
				Dst: ps.Dst,
			},
			Payload: &mem.WriteReqPayload{
				Address: ps.Address,
				Data:    dataCopy,
			},
		}
		trans.pendingWrite[id] = msg
	}

	c.currentTransaction = trans
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
	req := m.ctrlPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	if m.currentTransaction != nil {
		return false
	}

	_, ok := req.Payload.(*DataMoveRequestPayload)
	if !ok {
		log.Panicf("can't process request of type %s", reflect.TypeOf(req.Payload))
	}

	trans := newDataMoverTransaction(req)
	m.currentTransaction = trans

	m.setSrcSide(trans.reqPayload)
	m.setDstSide(trans.reqPayload)

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

	transEndAddr := trans.reqPayload.SrcAddress + trans.reqPayload.ByteSize
	if addr > transEndAddr {
		return false
	}

	req := mem.ReadReqBuilder{}.
		WithAddress(addr).
		WithSrc(m.srcPort.AsRemote()).
		WithDst(m.srcPortMapper.Find(addr)).
		WithByteSize(m.srcByteGranularity).
		WithPID(0).
		Build()

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

	rsp := m.srcPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	readRspPayload, ok := rsp.Payload.(*mem.DataReadyRspPayload)
	if !ok {
		// it can be write done rsp if src and dst is the same side. So ignore.
		return false
	}

	originalReq, ok := m.currentTransaction.pendingRead[rsp.RspTo]
	if !ok {
		log.Panicf("can't find original request for response %s",
			rsp.RspTo)
	}

	originalReqPayload := sim.MsgPayload[mem.ReadReqPayload](originalReq)
	offset := originalReqPayload.Address - m.currentTransaction.reqPayload.SrcAddress
	m.buffer.addData(offset, readRspPayload.Data)

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
	offset := trans.nextWriteAddr - trans.reqPayload.DstAddress
	data, ok := m.buffer.extractData(offset, m.dstByteGranularity)

	if !ok {
		return false
	}

	req := mem.WriteReqBuilder{}.
		WithAddress(m.currentTransaction.nextWriteAddr).
		WithData(data).
		WithSrc(m.dstPort.AsRemote()).
		WithDst(m.dstPortMapper.Find(m.currentTransaction.nextWriteAddr)).
		WithPID(0).
		Build()

	err := m.dstPort.Send(req)
	if err != nil {
		return false
	}

	m.currentTransaction.nextWriteAddr += m.dstByteGranularity
	m.currentTransaction.pendingWrite[req.ID] = req
	m.buffer.moveOffsetForwardTo(trans.nextWriteAddr - trans.reqPayload.DstAddress)

	tracing.TraceReqInitiate(req, m,
		tracing.MsgIDAtReceiver(m.currentTransaction.req, m))

	return true
}

// processWriteDoneFromDst processes write done from destination
func (m *dataMoverMiddleware) processWriteDoneFromDst() bool {
	if m.currentTransaction == nil {
		return false
	}

	rsp := m.dstPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	_, ok := rsp.Payload.(*mem.WriteDoneRspPayload)
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

	if trans.nextWriteAddr < trans.reqPayload.DstAddress+trans.reqPayload.ByteSize {
		return false
	}

	rsp := &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: trans.req.Dst,
			Dst: trans.req.Src,
		},
		RspTo: trans.req.ID,
	}

	err := m.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	m.currentTransaction = nil
	m.buffer = &buffer{
		offset:      alignAddress(trans.reqPayload.SrcAddress, m.srcByteGranularity),
		granularity: m.srcByteGranularity,
	}

	tracing.TraceReqComplete(rsp, m)

	return true
}

func (m *dataMoverMiddleware) setSrcSide(moveReq *DataMoveRequestPayload) {
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

func (m *dataMoverMiddleware) setDstSide(moveReq *DataMoveRequestPayload) {
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
