package datamoving

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type DataMover struct {
	*sim.TickingComponent

	Log2AccessSize uint64

	toA                []*sim.Msg
	toB                []*sim.Msg
	processingRequests []*sim.Msg
	pendingRequests    []*sim.Msg

	moveRequest *DataMoveRequest
	maxReqCount uint64

	portA    sim.Port
	portB    sim.Port
	ctrlPort sim.Port

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

func (d *DataMover) parseFromA(now sim.VTimeInSec) bool {
	req := d.portA.RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		d.processDataReadyRsp(now, req)
	case *mem.WriteDoneRsp:
		d.processWriteDoneRsp(now, req)
	default:
		log.Panicf("can not handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (d *DataMover) parseFromB(now sim.VTimeInSec) bool {
	req := d.portB.RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		d.processDataReadyRsp(now, req)
	case *mem.WriteDoneRsp:
		d.processWriteDoneRsp(now, req)
	default:
		log.Panicf("can not handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (d *DataMover) processDataReadyRsp(
	now sim.VTimeInSec,
	rsp *mem.DataReadyRsp,
) {
}

func (d *DataMover) processWriteDoneRsp(
	now sim.VTimeInSec,
	rsp *mem.WriteDoneRsp,
) {

}

func (d *DataMover) parseFromCtrlPort(now sim.VTimeInSec) bool {
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
		return d.handleMoveRequest(now, req)
	default:
		log.Panicf("can't process request of type %s", reflect.TypeOf(req))
	}
	return false
}

func (d *DataMover) handleMoveRequest(
	now sim.VTimeInSec,
	req *DataMoveRequest,
) bool {
	d.moveRequest = req
	return true
}

func (d *DataMover) processMoveRequest(
	now sim.VTimeInSec,
) bool {
	return false
}

func (d *DataMover) processOutside()

func (d *DataMover) processInside()
