package datamoving

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// A RequestCollection contains a data moving request from a single
// source/destination with Read/Write requests correspond to it.
type RequestCollection struct {
	topReq      sim.Msg
	subReqIDs   []string
	subReqCount int
}

func (rqC *RequestCollection) isFinished() bool {
	return rqC.subReqCount == 0
}

func (rqC *RequestCollection) decreSubIfExists(inputID string) bool {
	for _, id := range rqC.subReqIDs {
		if inputID == id {
			rqC.subReqCount -= 1
			return true
		}
	}
	return false
}

func (rqC *RequestCollection) appendSubReq(inputID string) {
	rqC.subReqIDs = append(rqC.subReqIDs, inputID)
	rqC.subReqCount += 1
}

func (rqC *RequestCollection) getTopReq() sim.Msg {
	return rqC.topReq
}

func (rqC *RequestCollection) getTopID() string {
	return rqC.topReq.Meta().ID
}

func newRequestCollection(
	inputReq sim.Msg,
) *RequestCollection {
	rqC := new(RequestCollection)
	rqC.topReq = inputReq
	rqC.subReqCount = 0
	return rqC
}

type DataMover struct {
	*sim.TickingComponent

	Log2AccessSize uint64

	toSrc              []sim.Msg
	toDst              []sim.Msg
	toCP               []sim.Msg
	processingRequests []*RequestCollection
	pendingRequests    []sim.Msg

	moveRequest *DataMoveRequest
	maxReqCount uint64

	srcPort  sim.Port
	dstPort  sim.Port
	ctrlPort sim.Port

	localDataSource mem.LowModuleFinder

	writeDone bool
}

func (d *DataMover) SetLocalDataSource(s mem.LowModuleFinder) {
	return
}

func (d *DataMover) Tick() bool {
	madeProgess := false

	return madeProgess
}

func (d *DataMover) send(
	// now sim.VTimeInSec,
	port sim.Port,
	reqs *[]sim.Msg,
) bool {
	if len(*reqs) == 0 {
		return false
	}

	req := (*reqs)[0]
	err := port.Send(req)
	if err == nil {
		*reqs = (*reqs)[1:]
		return true
	}
	return false
}

func (d *DataMover) parseFromSrc() bool {
	req := d.srcPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		d.processDataReadyRsp(req)
	case *mem.WriteDoneRsp:
		d.processWriteDoneRsp(req)
	default:
		log.Panicf("can not handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (d *DataMover) parseFromDst() bool {
	req := d.dstPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		d.processDataReadyRsp(req)
	case *mem.WriteDoneRsp:
		d.processWriteDoneRsp(req)
	default:
		log.Panicf("can not handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (d *DataMover) removeReqFromPendingReqs(
	id string,
) sim.Msg {
	var targetReq sim.Msg
	newList := make([]sim.Msg, 0, len(d.pendingRequests)-1)
	for _, req := range d.pendingRequests {
		if req.Meta().ID == id {
			targetReq = req
		} else {
			newList = append(newList, req)
		}
	}
	d.pendingRequests = newList

	if targetReq == nil {
		panic("request not found")
	}

	return targetReq
}

func (d *DataMover) removeReqFromProcessingReqs(
	id string,
) {
	found := false
	newList := make([]*RequestCollection, 0, len(d.processingRequests)-1)
	for _, req := range d.processingRequests {
		if req.getTopID() == id {
			found = true
		} else {
			newList = append(newList, req)
		}
	}
	d.processingRequests = newList

	if !found {
		panic("request not found")
	}
}

func (d *DataMover) processDataReadyRsp(
	rsp *mem.DataReadyRsp,
) {
	req := d.removeReqFromPendingReqs(rsp.RespondTo).(*mem.ReadReq)
	tracing.TraceReqFinalize(req, d)

	found := false
	result := &RequestCollection{}
	for _, rqC := range d.processingRequests {
		if rqC.decreSubIfExists(req.Meta().ID) {
			result = rqC
			found = true
		}
	}

	if !found {
		panic("Request is not found ")
	}

	processing := result.getTopReq().(*DataMoveRequest)
	d.dataTransfer(req, processing, rsp)

	if result.isFinished() {
		tracing.TraceReqComplete(processing, d)
		d.removeReqFromProcessingReqs(processing.Meta().ID)

		rsp := sim.GeneralRspBuilder{}.
			WithSrc(processing.Dst).
			WithDst(processing.Src).
			WithOriginalReq(processing).
			Build()
		d.toCP = append(d.toCP, rsp)
	}
}

func (d *DataMover) processWriteDoneRsp(
	rsp *mem.WriteDoneRsp,
) {
	req := d.removeReqFromPendingReqs(rsp.RespondTo)
	tracing.TraceReqFinalize(req, d)

	found := false
	result := &RequestCollection{}
	for _, rqC := range d.processingRequests {
		if rqC.decreSubIfExists(req.Meta().ID) {
			result = rqC
			found = true
		}
	}

	if !found {
		panic("could not find requst collection")
	}

	processing := result.getTopReq().(*DataMoveRequest)
	if result.isFinished() {
		tracing.TraceReqComplete(processing, d)
		d.removeReqFromProcessingReqs(processing.Meta().ID)

		rsp := sim.GeneralRspBuilder{}.
			WithSrc(processing.Dst).
			WithDst(processing.Src).
			WithOriginalReq(processing).
			Build()
		d.toCP = append(d.toCP, rsp)
	}
}

func (d *DataMover) dataTransfer(
	req *mem.ReadReq,
	dmr *DataMoveRequest,
	rsp *mem.DataReadyRsp,
) {
	var offset uint64
	srcDirection := dmr.srcDirection
	dstDirection := dmr.dstDirection

	if srcDirection == "out" && dstDirection == "in" {
		offset = req.Address - dmr.srcAddress
		copy(dmr.dstBuffer[offset:], rsp.Data)
	} else if srcDirection == "in" && dstDirection == "out" {
		offset = req.Address - dmr.dstAddress
		copy(dmr.srcBuffer[offset:], rsp.Data)
	} else {
		panic("data move request invalid")
	}
}

func (d *DataMover) parseFromCP() bool {
	if len(d.processingRequests) >= int(d.maxReqCount) {
		return false
	}

	req := d.ctrlPort.RetrieveIncoming()
	if req == nil {
		return false
	}
	tracing.TraceReqReceive(req, d)

	rqC := newRequestCollection(req)
	d.processingRequests = append(d.processingRequests, rqC)

	switch req := req.(type) {
	case *DataMoveRequest:
		return d.processMoveRequest(req, rqC)
	default:
		log.Panicf("can't process request of type %s", reflect.TypeOf(req))
		return false
	}
}

func (d *DataMover) processMoveRequest(
	req *DataMoveRequest,
	rqC *RequestCollection,
) bool {
	if req == nil {
		return false
	}

	srcAction := false
	dstAction := true

	if req.srcDirection == "in" {
		d.processSrcIn(req, rqC)
		srcAction = true
	} else if req.srcDirection == "out" {
		d.processSrcOut(req, rqC)
		srcAction = true
	} else {
		log.Panicf("can't process direction of type %s", req.srcDirection)
	}

	if req.dstDirection == "in" {
		d.processDstIn(req, rqC)
		dstAction = true
	} else if req.dstDirection == "out" {
		d.processDstOut(req, rqC)
		dstAction = true
	} else {
		log.Panicf("can't process direction of type %s", req.dstDirection)
	}

	return srcAction || dstAction
}

func (d *DataMover) processSrcIn(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := uint64(len(req.srcBuffer))
	addr := req.srcAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << d.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << d.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := d.localDataSource.Find(addr)
		reqToSrcPort := mem.ReadReqBuilder{}.
			WithSrc(d.srcPort).
			WithDst(module).
			WithAddress(addr).
			WithByteSize(length).
			Build()
		d.toSrc = append(d.toDst, reqToSrcPort)
		d.pendingRequests = append(d.pendingRequests, reqToSrcPort)
		rqC.appendSubReq(reqToSrcPort.Meta().ID)

		tracing.TraceReqInitiate(reqToSrcPort, d,
			tracing.MsgIDAtReceiver(req, d))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

func (d *DataMover) processSrcOut(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := uint64(len(req.srcBuffer))
	addr := req.srcAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << d.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << d.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := d.localDataSource.Find(addr)
		reqToSrcPort := mem.WriteReqBuilder{}.
			WithSrc(d.srcPort).
			WithDst(module).
			WithAddress(addr).
			WithData(req.srcBuffer[offset : offset+length]).
			Build()
		d.toSrc = append(d.toDst, reqToSrcPort)
		d.pendingRequests = append(d.pendingRequests, reqToSrcPort)
		rqC.appendSubReq(reqToSrcPort.Meta().ID)

		tracing.TraceReqInitiate(reqToSrcPort, d,
			tracing.MsgIDAtReceiver(req, d))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

func (d *DataMover) processDstIn(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := uint64(len(req.dstBuffer))
	addr := req.dstAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << d.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << d.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := d.localDataSource.Find(addr)
		reqToDstPort := mem.ReadReqBuilder{}.
			WithSrc(d.dstPort).
			WithDst(module).
			WithAddress(addr).
			WithByteSize(length).
			Build()
		d.toDst = append(d.toSrc, reqToDstPort)
		d.pendingRequests = append(d.pendingRequests, reqToDstPort)
		rqC.appendSubReq(reqToDstPort.Meta().ID)

		tracing.TraceReqInitiate(reqToDstPort, d,
			tracing.MsgIDAtReceiver(req, d))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

func (d *DataMover) processDstOut(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := uint64(len(req.dstBuffer))
	addr := req.dstAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << d.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << d.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := d.localDataSource.Find(addr)
		reqToDstPort := mem.WriteReqBuilder{}.
			WithSrc(d.dstPort).
			WithDst(module).
			WithAddress(addr).
			WithData(req.dstBuffer[offset : offset+length]).
			Build()
		d.toDst = append(d.toSrc, reqToDstPort)
		d.pendingRequests = append(d.pendingRequests, reqToDstPort)
		rqC.appendSubReq(reqToDstPort.Meta().ID)

		tracing.TraceReqInitiate(reqToDstPort, d,
			tracing.MsgIDAtReceiver(req, d))

		addr += length
		lengthLeft -= length
		offset += length
	}
}
