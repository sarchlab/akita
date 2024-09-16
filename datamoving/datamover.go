package datamoving

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// A DataMoveRequest asks DataMover to transfer data
type DataMoveRequest struct {
	sim.MsgMeta
	srcAddress   uint64
	dstAddress   uint64
	srcDirection string
	dstDirection string
	byteSize     uint64
}

func NewDataMoveRequest(
	sourcePort sim.Port,
	destinationPort sim.Port,
	sourceAddress uint64,
	destinationAddress uint64,
	sourceDirection string,
	destinationDirection string,
	size uint64,
) *DataMoveRequest {
	req := &DataMoveRequest{}
	req.ID = sim.GetIDGenerator().Generate()
	req.Src = sourcePort
	req.Dst = destinationPort
	req.srcAddress = sourceAddress
	req.dstAddress = destinationAddress
	req.srcDirection = sourceDirection
	req.dstDirection = destinationDirection
	req.byteSize = size

	return req
}

func (req *DataMoveRequest) Meta() *sim.MsgMeta {
	return &req.MsgMeta
}

type DataMover struct {
	*sim.TickingComponent

	writeRequests   []sim.Msg
	readRequests    []sim.Msg
	pendingRequests []sim.Msg

	toReadSrc   sim.Port
	toWriteDst  sim.Port
	controlPort sim.Port

	localDataSource mem.LowModuleFinder

	writeDone bool
}

func (d *DataMover) SetLocalDataSource(s mem.LowModuleFinder) {
	return
}

func (d *DataMover) Tick(now sim.VTimeInSec) bool {
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

func (d *DataMover) parseFromOutside(
	now sim.VTimeInSec,
) bool {
	req := d.controlPort.PeekIncoming()
	if req == nil {
		return false
	}

	return false
}

func (d *DataMover) parseFromReadSource(
	now sim.VTimeInSec,
) bool {
	req := d.toReadSrc.RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		d.processDataReadyRsp(now)
	default:
		log.Panicf("can not handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (d *DataMover) parseFromWriteDst(
	now sim.VTimeInSec,
) bool {
	req := d.toWriteDst.RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.WriteDoneRsp:
		d.processWriteDoneRsp(now)
	default:
		log.Panicf("can not handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (d *DataMover) processDataMovingReq(now sim.VTimeInSec) {

}

func (d *DataMover) processDataReadyRsp(now sim.VTimeInSec) {

}

func (d *DataMover) processWriteDoneRsp(now sim.VTimeInSec) {

}
