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

func newRequestCollection(
	inputReq sim.Msg,
) *RequestCollection {
	rqC := new(RequestCollection)
	rqC.topReq = inputReq
	rqC.subReqCount = 0
	return rqC
}

func (rqC *RequestCollection) getTopReq() sim.Msg {
	return rqC.topReq
}

func (rqC *RequestCollection) getTopID() string {
	return rqC.topReq.Meta().ID
}

type DataMover struct {
	*sim.TickingComponent

	Log2AccessSize uint64

	toSrc              []sim.Msg
	toDst              []sim.Msg
	processingRequests []sim.Msg
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
	// req.Meta().SendTime = now
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
		d.processDataReadyRsp(req, d.srcPort)
	case *mem.WriteDoneRsp:
		d.processWriteDoneRsp(req, d.srcPort)
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
		d.processDataReadyRsp(req, d.dstPort)
	case *mem.WriteDoneRsp:
		d.processWriteDoneRsp(req, d.dstPort)
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
) sim.Msg {
	var targetReq sim.Msg
	newList := make([]sim.Msg, 0, len(d.pendingRequests)-1)
	for _, req := range d.processingRequests {
		if req.Meta().ID == id {
			targetReq = req
		} else {
			newList = append(newList, req)
		}
	}
	d.processingRequests = newList

	if targetReq == nil {
		panic("request not found")
	}

	return targetReq
}

func (d *DataMover) processDataReadyRsp(
	rsp *mem.DataReadyRsp,
	receiver sim.Port,
) {
	req := d.removeReqFromPendingReqs(rsp.RespondTo).(*mem.ReadReq)
	tracing.TraceReqFinalize(req, d)

	found := false

	if !found {
		panic("Request is not found ")
	}
}

func (d *DataMover) processWriteDoneRsp(
	rsp *mem.WriteDoneRsp,
	receiver sim.Port,
) {

}

func (d *DataMover) parseFromCtrlPort() bool {
	if len(d.processingRequests) >= int(d.maxReqCount) {
		return false
	}

	req := d.ctrlPort.RetrieveIncoming()
	if req == nil {
		return false
	}
	tracing.TraceReqReceive(req, d)

	switch req := req.(type) {
	case *DataMoveRequest:
		return d.processMoveRequest(req)
	default:
		log.Panicf("can't process request of type %s", reflect.TypeOf(req))
		return false
	}
}

func (d *DataMover) processMoveRequest(
	req *DataMoveRequest,
) bool {
	if req == nil {
		return false
	}

	srcAction := false
	dstAction := true

	if req.srcDirection == "in" {
		d.processSrcIn(req)
		srcAction = true
	} else if req.srcDirection == "out" {
		d.processSrcOut(req)
		srcAction = true
	} else {
		log.Panicf("can't process direction of type %s", req.srcDirection)
	}

	if req.dstDirection == "in" {
		d.processDstIn(req)
		dstAction = true
	} else if req.dstDirection == "out" {
		d.processDstOut(req)
		dstAction = true
	} else {
		log.Panicf("can't process direction of type %s", req.dstDirection)
	}

	return srcAction || dstAction
}

func (d *DataMover) processSrcIn(
	req *DataMoveRequest,
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

		tracing.TraceReqInitiate(reqToSrcPort, d,
			tracing.MsgIDAtReceiver(req, d))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

func (d *DataMover) processSrcOut(
	req *DataMoveRequest,
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

		tracing.TraceReqInitiate(reqToSrcPort, d,
			tracing.MsgIDAtReceiver(req, d))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

func (d *DataMover) processDstIn(
	req *DataMoveRequest,
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

		tracing.TraceReqInitiate(reqToDstPort, d,
			tracing.MsgIDAtReceiver(req, d))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

func (d *DataMover) processDstOut(
	req *DataMoveRequest,
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

		tracing.TraceReqInitiate(reqToDstPort, d,
			tracing.MsgIDAtReceiver(req, d))

		addr += length
		lengthLeft -= length
		offset += length
	}
}
