package gmmu

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// respondMW handles the bottom→top response path:
// fetchFromBottom, handleTranslationRsp.
type respondMW struct {
	comp *modeling.Component[Spec, State]
}

func (m *respondMW) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *respondMW) bottomPort() sim.Port {
	return m.comp.GetPortByName("Bottom")
}

// Tick runs the respond stage.
func (m *respondMW) Tick() bool {
	return m.fetchFromBottom()
}

func (m *respondMW) fetchFromBottom() bool {
	if !m.topPort().CanSend() {
		return false
	}

	rspI := m.bottomPort().RetrieveIncoming()
	if rspI == nil {
		return false
	}

	switch rsp := rspI.(type) {
	case *vm.TranslationRsp:
		tracing.TraceReqReceive(rsp, m.comp)
		return m.handleTranslationRsp(rsp)
	default:
		log.Panicf("gmmu cannot handle request of type %s",
			fmt.Sprintf("%T", rspI))
		return false
	}
}

func (m *respondMW) handleTranslationRsp(rsp *vm.TranslationRsp) bool {
	state := m.comp.GetNextState()

	reqTransaction, exists := state.RemoteMemReqs[rsp.RspTo]

	if !exists || reqTransaction.ReqID == 0 {
		log.Panicf("Cannot find matching request for response %+v", rsp)
	}

	if !m.topPort().CanSend() {
		return false
	}

	rspToTop := &vm.TranslationRsp{
		Page: rsp.Page,
	}
	rspToTop.ID = sim.GetIDGenerator().Generate()
	rspToTop.Src = m.topPort().AsRemote()
	rspToTop.Dst = reqTransaction.ReqSrc
	rspToTop.RspTo = rsp.ID
	rspToTop.TrafficClass = "vm.TranslationRsp"

	m.topPort().Send(rspToTop)

	delete(state.RemoteMemReqs, rsp.RspTo)

	return true
}
