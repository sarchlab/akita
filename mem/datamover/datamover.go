package datamover

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

// NewRequestCollection creates a new RequestCollection.
func NewRequestCollection(
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

	insidePort  sim.Port
	outsidePort sim.Port
	ctrlPort    sim.Port

	toSrc           []sim.Msg
	toDst           []sim.Msg
	toCP            []sim.Msg
	pendingRequests []sim.Msg
	buffer          []byte
	localDataSource mem.AddressToPortMapper

	srcPort        sim.Port
	dstPort        sim.Port
	currentRequest *RequestCollection
}

// Tick ticks
func (sdm *StreamingDataMover) Tick() bool {
	madeProgress := false

	madeProgress = sdm.send(sdm.insidePort, &sdm.toSrc) || madeProgress
	madeProgress = sdm.send(sdm.outsidePort, &sdm.toDst) || madeProgress
	madeProgress = sdm.send(sdm.ctrlPort, &sdm.toCP) || madeProgress
	madeProgress = sdm.parseFromCP() || madeProgress
	madeProgress = sdm.parseFromSrc() || madeProgress
	madeProgress = sdm.parseFromDst() || madeProgress

	return madeProgress
}

// SetLocalDataSource sets the table that maps from address to port that can
// provide the data
func (sdm *StreamingDataMover) SetLocalDataSource(s mem.AddressToPortMapper) {
	sdm.localDataSource = s
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
	req := sdm.insidePort.RetrieveIncoming()
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
	req := sdm.outsidePort.RetrieveIncoming()
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

// processDataReadyRsp process every DataReadyRsp received after
// requesting to read
func (sdm *StreamingDataMover) processDataReadyRsp(
	rsp *mem.DataReadyRsp,
) {
	req := sdm.removeReqFromPendingReqs(rsp.RespondTo).(*mem.ReadReq)
	tracing.TraceReqFinalize(req, sdm)

	found := false
	result := sdm.currentRequest
	if result.decreSubIfExists(req.Meta().ID) {
		found = true
	}

	if !found {
		panic("Request is not found ")
	}

	processing := result.getTopReq().(*DataMoveRequest)
	transferFrom := processing.direction
	offset := req.Address
	switch transferFrom {
	case "s2d":
		offset -= processing.SrcAddress
	case "d2s":
		offset -= processing.DstAddress
	default:
		panic("Transfer direction is not supported")
	}
	copy(sdm.buffer[offset:], rsp.Data)

	if result.isFinished() {
		tracing.TraceReqComplete(processing, sdm)

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
	result := sdm.currentRequest
	if result.decreSubIfExists(req.Meta().ID) {
		found = true
	}

	if !found {
		panic("Request is not found")
	}

	processing := result.getTopReq().(*DataMoveRequest)
	if result.isFinished() {
		tracing.TraceReqComplete(processing, sdm)
		sdm.currentRequest = nil
		sdm.buffer = nil

		rsp := sim.GeneralRspBuilder{}.
			WithSrc(processing.Dst).
			WithDst(processing.Src).
			WithOriginalReq(processing).
			Build()
		sdm.toCP = append(sdm.toCP, rsp)
	}
}

// parseFromCP retrieves Msg from ctrlPort
func (sdm *StreamingDataMover) parseFromCP() bool {
	req := sdm.ctrlPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	if sdm.currentRequest != nil {
		return false
	}

	moveReq, ok := req.(*DataMoveRequest)
	if !ok {
		log.Panicf("can't process request of type %s", reflect.TypeOf(req))
	}

	rqC := NewRequestCollection(moveReq)
	sdm.currentRequest = rqC

	tracing.TraceReqReceive(req, sdm)

}

// processMoveRequest process data move requests from ctrlPort
func (sdm *StreamingDataMover) processMoveRequest(
	req *DataMoveRequest,
) bool {
	if req == nil {
		return false
	}

	srcAction := false
	dstAction := true

	if req.direction == "s2d" {
		sdm.processSrcOut(req)
		srcAction = true
		sdm.processDstIn(req)
		dstAction = true
	} else if req.direction == "d2s" {
		sdm.processDstOut(req)
		dstAction = true
		sdm.processSrcIn(req)
		srcAction = true
	} else {
		log.Panicf("can't process direction of type %s", req.direction)
	}

	return srcAction || dstAction
}

// processSrcIn sends read request from data mover to source
func (sdm *StreamingDataMover) processSrcOut(
	req *DataMoveRequest,
) {
	lengthLeft := req.ByteSize
	addr := req.SrcAddress

	for lengthLeft > 0 {
		lengthUnit := req.SrcTransferSize
		length := lengthUnit
		if length > lengthLeft {
			length = lengthLeft
			lengthLeft = 0
		} else {
			lengthLeft = lengthLeft - lengthUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToSrcPort := mem.ReadReqBuilder{}.
			WithSrc(sdm.insidePort).
			WithDst(module).
			WithAddress(addr).
			WithByteSize(length).
			Build()
		sdm.toSrc = append(sdm.toSrc, reqToSrcPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToSrcPort)
		sdm.currentRequest.appendSubReq(reqToSrcPort.Meta().ID)
		sdm.send(sdm.insidePort, &sdm.toSrc)

		tracing.TraceReqInitiate(reqToSrcPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
	}
}

// processSrcOut processes writing request to source
func (sdm *StreamingDataMover) processSrcIn(
	req *DataMoveRequest,
) {
	offset := uint64(0)
	lengthLeft := req.ByteSize
	addr := req.SrcAddress

	for lengthLeft > 0 {
		lengthUnit := req.SrcTransferSize
		length := lengthUnit
		if length > lengthLeft {
			length = lengthLeft
			lengthLeft = 0
		} else {
			lengthLeft -= lengthUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToSrcPort := mem.WriteReqBuilder{}.
			WithSrc(sdm.insidePort).
			WithDst(module).
			WithAddress(addr).
			WithData(sdm.buffer[offset : offset+length]).
			Build()
		sdm.toSrc = append(sdm.toSrc, reqToSrcPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToSrcPort)
		sdm.currentRequest.appendSubReq(reqToSrcPort.Meta().ID)
		sdm.send(sdm.insidePort, &sdm.toSrc)

		tracing.TraceReqInitiate(reqToSrcPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
		offset += length
	}
}

// processDstIn sends read request from data mover to destination
func (sdm *StreamingDataMover) processDstOut(
	req *DataMoveRequest,
) {
	lengthLeft := req.ByteSize
	addr := req.DstAddress

	for lengthLeft > 0 {
		lengthUnit := req.DstTransferSize
		length := lengthUnit
		if length > lengthLeft {
			length = lengthLeft
			lengthLeft = 0
		} else {
			lengthLeft -= lengthUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToDstPort := mem.ReadReqBuilder{}.
			WithSrc(sdm.outsidePort).
			WithDst(module).
			WithAddress(addr).
			WithByteSize(length).
			Build()
		sdm.toDst = append(sdm.toDst, reqToDstPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToDstPort)
		sdm.currentRequest.appendSubReq(reqToDstPort.Meta().ID)
		sdm.send(sdm.outsidePort, &sdm.toDst)

		tracing.TraceReqInitiate(reqToDstPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
	}
}

// processDstOut requests writing request to destination
func (sdm *StreamingDataMover) processDstIn(
	req *DataMoveRequest,
) {
	offset := uint64(0)
	lengthLeft := req.ByteSize
	addr := req.DstAddress

	for lengthLeft > 0 {
		lengthUnit := req.DstTransferSize
		length := lengthUnit
		if length > lengthLeft {
			length = lengthLeft
			lengthLeft = 0
		} else {
			lengthLeft -= lengthUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToDstPort := mem.WriteReqBuilder{}.
			WithSrc(sdm.outsidePort).
			WithDst(module).
			WithAddress(addr).
			WithData(sdm.buffer[offset : offset+length]).
			Build()
		sdm.toDst = append(sdm.toDst, reqToDstPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToDstPort)
		sdm.currentRequest.appendSubReq(reqToDstPort.Meta().ID)
		sdm.send(sdm.outsidePort, &sdm.toDst)

		tracing.TraceReqInitiate(reqToDstPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
		offset += length
	}
}
