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

// isFinished tells whether all subrequests in this collection have
// been finished
func (rqC *RequestCollection) isFinished() bool {
	return rqC.subReqCount == 0
}

// decreSubIfExists compare the input ID with those of subrequests, and
// decrease subrequest count by one if the ID matches
func (rqC *RequestCollection) decreSubIfExists(inputID string) bool {
	for _, id := range rqC.subReqIDs {
		if inputID == id {
			rqC.subReqCount -= 1
			return true
		}
	}
	return false
}

// appendSubReq add new IDs into the slice of subrequest IDs
func (rqC *RequestCollection) appendSubReq(inputID string) {
	rqC.subReqIDs = append(rqC.subReqIDs, inputID)
	rqC.subReqCount += 1
}

// getTopReq returns DataMoveRequest that invokes all subrequests
// in that collection
func (rqC *RequestCollection) getTopReq() sim.Msg {
	return rqC.topReq
}

// getTopID returns the ID of DataMoveRequest
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

// StreamingDataMover helps moving data from designated source and destination
// following the given move direction
type StreamingDataMover struct {
	*sim.TickingComponent

	Log2AccessSize uint64

	toSrc              []sim.Msg
	toDst              []sim.Msg
	toCP               []sim.Msg
	processingRequests []*RequestCollection
	pendingRequests    []sim.Msg

	maxReqCount uint64

	srcPort  sim.Port
	dstPort  sim.Port
	ctrlPort sim.Port

	localDataSource mem.LowModuleFinder
}

// Tick ticks
func (sdm *StreamingDataMover) Tick() bool {
	madeProgress := false

	madeProgress = sdm.send(sdm.srcPort, &sdm.toSrc) || madeProgress
	madeProgress = sdm.send(sdm.dstPort, &sdm.toDst) || madeProgress
	madeProgress = sdm.send(sdm.ctrlPort, &sdm.toCP) || madeProgress
	madeProgress = sdm.parseFromCP() || madeProgress
	madeProgress = sdm.parseFromSrc() || madeProgress
	madeProgress = sdm.parseFromDst() || madeProgress

	return madeProgress
}

// send sends the Msg to the given port
func (sdm *StreamingDataMover) send(
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

// parseFromSrc retrieves Msg from srcPort
func (sdm *StreamingDataMover) parseFromSrc() bool {
	req := sdm.srcPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		sdm.processDataReadyRsp(req)
	case *mem.WriteDoneRsp:
		sdm.processWriteDoneRsp(req)
	default:
		log.Panicf("can not handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

// parseFromDst retrieves Msg from dstPort
func (sdm *StreamingDataMover) parseFromDst() bool {
	req := sdm.dstPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		sdm.processDataReadyRsp(req)
	case *mem.WriteDoneRsp:
		sdm.processWriteDoneRsp(req)
	default:
		log.Panicf("can not handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

// removeReqFromPendingReqs remove request of certain ID from the
// slice of pending requests
func (sdm *StreamingDataMover) removeReqFromPendingReqs(
	id string,
) sim.Msg {
	var targetReq sim.Msg
	newList := make([]sim.Msg, 0, len(sdm.pendingRequests)-1)
	for _, req := range sdm.pendingRequests {
		if req.Meta().ID == id {
			targetReq = req
		} else {
			newList = append(newList, req)
		}
	}
	sdm.pendingRequests = newList

	if targetReq == nil {
		panic("request not found")
	}

	return targetReq
}

// removeReqFromProcessingReqs remove request of certain ID from the
// collection of processing requests
func (sdm *StreamingDataMover) removeReqFromProcessingReqs(
	id string,
) {
	found := false
	newList := make([]*RequestCollection, 0, len(sdm.processingRequests)-1)
	for _, req := range sdm.processingRequests {
		if req.getTopID() == id {
			found = true
		} else {
			newList = append(newList, req)
		}
	}
	sdm.processingRequests = newList

	if !found {
		panic("request not found")
	}
}

// processDataReadyRsp process every DataReadyRsp received after
// requesting to read
func (sdm *StreamingDataMover) processDataReadyRsp(
	rsp *mem.DataReadyRsp,
) {
	req := sdm.removeReqFromPendingReqs(rsp.RespondTo).(*mem.ReadReq)
	tracing.TraceReqFinalize(req, sdm)

	found := false
	result := &RequestCollection{}
	for _, rqC := range sdm.processingRequests {
		if rqC.decreSubIfExists(req.Meta().ID) {
			result = rqC
			found = true
		}
	}

	if !found {
		panic("Request is not found ")
	}

	processing := result.getTopReq().(*DataMoveRequest)
	sdm.dataTransfer(req, processing, rsp)

	if result.isFinished() {
		tracing.TraceReqComplete(processing, sdm)
		sdm.removeReqFromProcessingReqs(processing.Meta().ID)

		rsp := sim.GeneralRspBuilder{}.
			WithSrc(processing.Dst).
			WithDst(processing.Src).
			WithOriginalReq(processing).
			Build()
		sdm.toCP = append(sdm.toCP, rsp)
	}
}

// processWriteDoneRsp process every WriteDoneRsp received after
// requesting to write
func (sdm *StreamingDataMover) processWriteDoneRsp(
	rsp *mem.WriteDoneRsp,
) {
	req := sdm.removeReqFromPendingReqs(rsp.RespondTo)
	tracing.TraceReqFinalize(req, sdm)

	found := false
	result := &RequestCollection{}
	for _, rqC := range sdm.processingRequests {
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
		tracing.TraceReqComplete(processing, sdm)
		sdm.removeReqFromProcessingReqs(processing.Meta().ID)

		rsp := sim.GeneralRspBuilder{}.
			WithSrc(processing.Dst).
			WithDst(processing.Src).
			WithOriginalReq(processing).
			Build()
		sdm.toCP = append(sdm.toCP, rsp)
	}
}

// dataTransfer transfers data from buffer in source/destination to
// buffer in source/destination following the given direction for every
// read and write request
func (sdm *StreamingDataMover) dataTransfer(
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

// parseFromCP retrieves Msg from ctrlPort
func (sdm *StreamingDataMover) parseFromCP() bool {
	if len(sdm.processingRequests) >= int(sdm.maxReqCount) {
		return false
	}

	req := sdm.ctrlPort.RetrieveIncoming()
	if req == nil {
		return false
	}
	tracing.TraceReqReceive(req, sdm)

	rqC := newRequestCollection(req)
	sdm.processingRequests = append(sdm.processingRequests, rqC)

	switch req := req.(type) {
	case *DataMoveRequest:
		return sdm.processMoveRequest(req, rqC)
	default:
		log.Panicf("can't process request of type %s", reflect.TypeOf(req))
		return false
	}
}

// processMoveRequest process data move requests from ctrlPort
func (sdm *StreamingDataMover) processMoveRequest(
	req *DataMoveRequest,
	rqC *RequestCollection,
) bool {
	if req == nil {
		return false
	}

	srcAction := false
	dstAction := true

	if req.srcDirection == "in" {
		sdm.processSrcIn(req, rqC)
		srcAction = true
	} else if req.srcDirection == "out" {
		sdm.processSrcOut(req, rqC)
		srcAction = true
	} else {
		log.Panicf("can't process direction of type %s", req.srcDirection)
	}

	if req.dstDirection == "in" {
		sdm.processDstIn(req, rqC)
		dstAction = true
	} else if req.dstDirection == "out" {
		sdm.processDstOut(req, rqC)
		dstAction = true
	} else {
		log.Panicf("can't process direction of type %s", req.dstDirection)
	}

	return srcAction || dstAction
}

// processSrcIn processes reading request to source
func (sdm *StreamingDataMover) processSrcIn(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := uint64(len(req.srcBuffer))
	addr := req.srcAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << sdm.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << sdm.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToSrcPort := mem.ReadReqBuilder{}.
			WithSrc(sdm.srcPort).
			WithDst(module).
			WithAddress(addr).
			WithByteSize(length).
			Build()
		sdm.toSrc = append(sdm.toDst, reqToSrcPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToSrcPort)
		rqC.appendSubReq(reqToSrcPort.Meta().ID)

		tracing.TraceReqInitiate(reqToSrcPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

// processSrcOut processes writing request to source
func (sdm *StreamingDataMover) processSrcOut(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := uint64(len(req.srcBuffer))
	addr := req.srcAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << sdm.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << sdm.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToSrcPort := mem.WriteReqBuilder{}.
			WithSrc(sdm.srcPort).
			WithDst(module).
			WithAddress(addr).
			WithData(req.srcBuffer[offset : offset+length]).
			Build()
		sdm.toSrc = append(sdm.toDst, reqToSrcPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToSrcPort)
		rqC.appendSubReq(reqToSrcPort.Meta().ID)

		tracing.TraceReqInitiate(reqToSrcPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

// processDstIn processes reading request to destination
func (sdm *StreamingDataMover) processDstIn(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := uint64(len(req.dstBuffer))
	addr := req.dstAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << sdm.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << sdm.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToDstPort := mem.ReadReqBuilder{}.
			WithSrc(sdm.dstPort).
			WithDst(module).
			WithAddress(addr).
			WithByteSize(length).
			Build()
		sdm.toDst = append(sdm.toSrc, reqToDstPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToDstPort)
		rqC.appendSubReq(reqToDstPort.Meta().ID)

		tracing.TraceReqInitiate(reqToDstPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

// processDstOut requests writing request to destination
func (sdm *StreamingDataMover) processDstOut(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := uint64(len(req.dstBuffer))
	addr := req.dstAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << sdm.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << sdm.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToDstPort := mem.WriteReqBuilder{}.
			WithSrc(sdm.dstPort).
			WithDst(module).
			WithAddress(addr).
			WithData(req.dstBuffer[offset : offset+length]).
			Build()
		sdm.toDst = append(sdm.toSrc, reqToDstPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToDstPort)
		rqC.appendSubReq(reqToDstPort.Meta().ID)

		tracing.TraceReqInitiate(reqToDstPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
		lengthLeft -= length
		offset += length
	}
}
