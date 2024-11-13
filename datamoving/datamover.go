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

	Log2AccessSize uint64
	isProcessing   bool
	currentRequest *RequestCollection

	toSrc           []sim.Msg
	toDst           []sim.Msg
	toCP            []sim.Msg
	pendingRequests []sim.Msg
	buffer          []byte

	maxReqCount uint64

	SrcPort  sim.Port
	DstPort  sim.Port
	CtrlPort sim.Port

	localDataSource mem.LowModuleFinder
}

// Tick ticks
func (sdm *StreamingDataMover) Tick() bool {
	madeProgress := false

	madeProgress = sdm.send(sdm.SrcPort, &sdm.toSrc) || madeProgress
	madeProgress = sdm.send(sdm.DstPort, &sdm.toDst) || madeProgress
	madeProgress = sdm.send(sdm.CtrlPort, &sdm.toCP) || madeProgress
	madeProgress = sdm.parseFromCP() || madeProgress
	madeProgress = sdm.parseFromSrc() || madeProgress
	madeProgress = sdm.parseFromDst() || madeProgress

	return madeProgress
}

// SetLocalDataSource sets the table that maps from address to port that can
// provide the data
func (sdm *StreamingDataMover) SetLocalDataSource(s mem.LowModuleFinder) {
	sdm.localDataSource = s
}

// GetMaxRequestCount returns the number of max requests streaming data mover
// can process
func (sdm *StreamingDataMover) GetMaxRequestCount() int {
	return int(sdm.maxReqCount)
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
	req := sdm.SrcPort.RetrieveIncoming()
	if req == nil {
		return false
	}
	if sdm.isProcessing {
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

	sdm.isProcessing = true
	return true
}

// parseFromDst retrieves Msg from dstPort
func (sdm *StreamingDataMover) parseFromDst() bool {
	req := sdm.DstPort.RetrieveIncoming()
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
		offset -= processing.srcAddress
	case "d2s":
		offset -= processing.dstAddress
	default:
		panic("Transfer direction is not supported")
	}
	copy(sdm.buffer[offset:], rsp.Data)

	if result.isFinished() {
		tracing.TraceReqComplete(processing, sdm)
		sdm.currentRequest = nil

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
		sdm.isProcessing = false

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
	req := sdm.CtrlPort.RetrieveIncoming()
	if req == nil {
		return false
	}
	tracing.TraceReqReceive(req, sdm)

	rqC := NewRequestCollection(req)
	sdm.currentRequest = rqC

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

	if req.direction == "s2d" {
		sdm.processSrcOut(req, rqC)
		srcAction = true
		sdm.processDstIn(req, rqC)
		dstAction = true
	} else if req.direction == "d2s" {
		sdm.processDstOut(req, rqC)
		dstAction = true
		sdm.processSrcIn(req, rqC)
		srcAction = true
	} else {
		log.Panicf("can't process direction of type %s", req.direction)
	}

	return srcAction || dstAction
}

// processSrcIn sends read request from data mover to source
func (sdm *StreamingDataMover) processSrcOut(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	lengthLeft := req.byteSize
	addr := req.srcAddress

	for lengthLeft > 0 {
		lengthUnit := req.srcTransferSize
		length := lengthUnit
		if length > lengthLeft {
			length = lengthLeft
			lengthLeft = 0
		} else {
			lengthLeft -= lengthUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToSrcPort := mem.ReadReqBuilder{}.
			WithSrc(sdm.SrcPort).
			WithDst(module).
			WithAddress(addr).
			WithByteSize(length).
			Build()
		sdm.toSrc = append(sdm.toSrc, reqToSrcPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToSrcPort)
		rqC.appendSubReq(reqToSrcPort.Meta().ID)

		tracing.TraceReqInitiate(reqToSrcPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
	}
}

// processSrcOut processes writing request to source
func (sdm *StreamingDataMover) processSrcIn(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := req.byteSize
	addr := req.srcAddress

	for lengthLeft > 0 {
		lengthUnit := req.srcTransferSize
		length := lengthUnit
		if length > lengthLeft {
			length = lengthLeft
			lengthLeft = 0
		} else {
			lengthLeft -= lengthUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToSrcPort := mem.WriteReqBuilder{}.
			WithSrc(sdm.SrcPort).
			WithDst(module).
			WithAddress(addr).
			WithData(sdm.buffer[offset : offset+length]).
			Build()
		sdm.toSrc = append(sdm.toSrc, reqToSrcPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToSrcPort)
		rqC.appendSubReq(reqToSrcPort.Meta().ID)

		tracing.TraceReqInitiate(reqToSrcPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
		offset += length
	}
}

// processDstIn sends read request from data mover to destination
func (sdm *StreamingDataMover) processDstOut(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	lengthLeft := req.byteSize
	addr := req.dstAddress

	for lengthLeft > 0 {
		lengthUnit := req.dstTransferSize
		length := lengthUnit
		if length > lengthLeft {
			length = lengthLeft
			lengthLeft = 0
		} else {
			lengthLeft -= lengthUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToDstPort := mem.ReadReqBuilder{}.
			WithSrc(sdm.DstPort).
			WithDst(module).
			WithAddress(addr).
			WithByteSize(length).
			Build()
		sdm.toSrc = append(sdm.toDst, reqToDstPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToDstPort)
		rqC.appendSubReq(reqToDstPort.Meta().ID)

		tracing.TraceReqInitiate(reqToDstPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
	}
}

// processDstOut requests writing request to destination
func (sdm *StreamingDataMover) processDstIn(
	req *DataMoveRequest,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := req.byteSize
	addr := req.dstAddress

	for lengthLeft > 0 {
		lengthUnit := req.dstTransferSize
		length := lengthUnit
		if length > lengthLeft {
			length = lengthLeft
			lengthLeft = 0
		} else {
			lengthLeft -= lengthUnit
		}

		module := sdm.localDataSource.Find(addr)
		reqToDstPort := mem.WriteReqBuilder{}.
			WithSrc(sdm.DstPort).
			WithDst(module).
			WithAddress(addr).
			WithData(sdm.buffer[offset : offset+length]).
			Build()
		sdm.toDst = append(sdm.toDst, reqToDstPort)
		sdm.pendingRequests = append(sdm.pendingRequests, reqToDstPort)
		rqC.appendSubReq(reqToDstPort.Meta().ID)

		tracing.TraceReqInitiate(reqToDstPort, sdm,
			tracing.MsgIDAtReceiver(req, sdm))

		addr += length
		offset += length
	}
}

// NewSDMEngine creates a new streaming data mover, injecting an engine and a
// "LowModuleFinder" that helps with locating the module that holds the data
func NewSDMEngine(
	name string,
	engine sim.Engine,
	localDataSource mem.LowModuleFinder,
) *StreamingDataMover {
	sdm := new(StreamingDataMover)
	sdm.TickingComponent = sim.NewTickingComponent(
		name, engine, 1*sim.GHz, sdm)

	sdm.Log2AccessSize = 6
	sdm.localDataSource = localDataSource

	sdm.maxReqCount = 4

	sdm.CtrlPort = sim.NewLimitNumMsgPort(sdm, 40960000, name+".CtrlPort")
	sdm.SrcPort = sim.NewLimitNumMsgPort(sdm, 64, name+".SrcPort")
	sdm.DstPort = sim.NewLimitNumMsgPort(sdm, 64, name+".DstPort")

	return sdm
}
