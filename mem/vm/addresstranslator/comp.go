package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling/ticking"
	"github.com/sarchlab/akita/v5/simulation/timing"
)

// Spec contains immutable configuration for the AddressTranslator.
type Spec struct {
	Freq           timing.Freq `json:"freq"`
	Log2PageSize   uint64      `json:"log2_page_size"`
	DeviceID       uint64      `json:"device_id"`
	NumReqPerCycle int         `json:"num_req_per_cycle"`
}

// Ports holds the AddressTranslator's ports.
type Ports struct {
	// Top receives memory requests with virtual addresses and returns their
	// responses.
	Top messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`

	// Bottom sends the translated memory requests to the memory providers.
	Bottom messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`

	// Translation sends translation requests to the translation providers.
	Translation messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/vm/vmprotocol.requester"`

	// Control receives enable, pause, drain, flush, and reset commands.
	Control messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.responder"`
}

// middlewares holds the AddressTranslator's behavior, run in field order
// every cycle.
type middlewares struct {
	// Ctrl handles control commands.
	Ctrl *ctrlMiddleware

	// ParseTranslate accepts requests from Top and starts their translation.
	ParseTranslate *parseTranslateMW

	// RespondPipeline handles translation responses, sends the translated
	// requests, and returns the memory responses to Top.
	RespondPipeline *respondPipelineMW
}

// Resources holds the external wiring referenced by the AddressTranslator. The
// mappers tell the translator where to send memory and translation requests.
type Resources struct {
	MemProviderMapper         mem.AddressToPortMapper `json:"-"`
	TranslationProviderMapper mem.AddressToPortMapper `json:"-"`
}

// incomingReqState is a serializable representation of an incoming request.
type incomingReqState struct {
	ID    uint64               `json:"id"`
	Src   messaging.RemotePort `json:"src"`
	Dst   messaging.RemotePort `json:"dst"`
	RspTo uint64               `json:"rsp_to"`
	Type  string               `json:"type"`

	// Fields preserved for translated request creation.
	Address            uint64 `json:"address"`
	AccessByteSize     uint64 `json:"access_byte_size"`
	PID                vm.PID `json:"pid"`
	Data               []byte `json:"data,omitempty"`
	DirtyMask          []bool `json:"dirty_mask,omitempty"`
	CanWaitForCoalesce bool   `json:"can_wait_for_coalesce"`
}

// transactionState is a serializable representation of a runtime transaction.
type transactionState struct {
	IncomingReqs      []incomingReqState   `json:"incoming_reqs"`
	TranslationReqID  uint64               `json:"translation_req_id"`
	TranslationReqSrc messaging.RemotePort `json:"translation_req_src"`
	TranslationReqDst messaging.RemotePort `json:"translation_req_dst"`
	TranslationDone   bool                 `json:"translation_done"`
	Page              vm.Page              `json:"page"`
}

// reqToBottomState is a serializable representation of a runtime reqToBottom.
type reqToBottomState struct {
	ReqFromTopID    uint64               `json:"req_from_top_id"`
	ReqFromTopSrc   messaging.RemotePort `json:"req_from_top_src"`
	ReqFromTopDst   messaging.RemotePort `json:"req_from_top_dst"`
	ReqFromTopType  string               `json:"req_from_top_type"`
	ReqToBottomID   uint64               `json:"req_to_bottom_id"`
	ReqToBottomSrc  messaging.RemotePort `json:"req_to_bottom_src"`
	ReqToBottomDst  messaging.RemotePort `json:"req_to_bottom_dst"`
	ReqToBottomType string               `json:"req_to_bottom_type"`
}

// state contains mutable runtime data for the AddressTranslator.
type state struct {
	ControlState        memcontrolprotocol.State `json:"control_state"`
	CurrentCmdID        uint64                   `json:"current_cmd_id"`
	CurrentCmdSrc       messaging.RemotePort     `json:"current_cmd_src"`
	Transactions        []transactionState       `json:"transactions"`
	InflightReqToBottom []reqToBottomState       `json:"inflight_req_to_bottom"`
}

// Helper functions

func addrToPageID(addr, log2PageSize uint64) uint64 {
	return (addr >> log2PageSize) << log2PageSize
}

func msgToIncomingReqState(msg messaging.Msg) incomingReqState {
	meta := msg
	s := incomingReqState{
		ID:    meta.ID,
		Src:   meta.Src,
		Dst:   meta.Dst,
		RspTo: meta.RspTo,
		Type:  fmt.Sprintf("%T", msg.Payload),
	}

	switch content := msg.Payload.(type) {
	case memprotocol.ReadReq:
		s.Address = content.Address
		s.AccessByteSize = content.AccessByteSize
		s.PID = content.PID
		s.CanWaitForCoalesce = content.CanWaitForCoalesce
	case memprotocol.WriteReq:
		s.Address = content.Address
		s.PID = content.PID
		s.Data = content.Data
		s.DirtyMask = content.DirtyMask
		s.CanWaitForCoalesce = content.CanWaitForCoalesce
	default:
		log.Panicf("cannot convert message of type %T", msg.Payload)
	}

	return s
}

func createTranslatedReq(
	newID func() uint64,
	reqState incomingReqState,
	page vm.Page,
	log2PageSize uint64,
	bottomPortRemote messaging.RemotePort,
	memProviderMapper mem.AddressToPortMapper,
) messaging.Msg {
	offset := reqState.Address % (1 << log2PageSize)
	addr := page.PAddr + offset

	switch reqState.Type {
	case "memprotocol.ReadReq":
		clone := messaging.Msg{Payload: memprotocol.ReadReq{
			Address:        addr,
			AccessByteSize: reqState.AccessByteSize,
			PID:            0,

			CanWaitForCoalesce: reqState.CanWaitForCoalesce},
			ID:  newID(),
			Src: bottomPortRemote,
			Dst: memProviderMapper.Find(addr),

			TrafficBytes: 12,
			TrafficClass: "memprotocol.ReadReq"}

		return clone
	case "memprotocol.WriteReq":
		clone := messaging.Msg{Payload: memprotocol.WriteReq{
			Data:      reqState.Data,
			DirtyMask: reqState.DirtyMask,
			Address:   addr,
			PID:       0,

			CanWaitForCoalesce: reqState.CanWaitForCoalesce},
			ID:  newID(),
			Src: bottomPortRemote,
			Dst: memProviderMapper.Find(addr),

			TrafficBytes: len(reqState.Data) + 12,
			TrafficClass: "memprotocol.WriteReq"}

		return clone
	default:
		log.Panicf("cannot translate request of type %s", reqState.Type)
		return messaging.Msg{}
	}
}

// restoreMemMsg reconstructs a concrete mem message from saved metadata. It
// preserves the original message ID so tracing lookups (keyed by the message
// ID) resolve to the same task as the original incoming message.
func restoreMemMsg(
	id uint64, src, dst messaging.RemotePort, rspTo uint64, typ string,
) messaging.Msg {
	meta := messaging.Msg{ID: id, Src: src, Dst: dst, RspTo: rspTo}
	switch typ {
	case "memprotocol.WriteReq":
		meta.Payload = memprotocol.WriteReq{}
		return meta
	default:
		meta.Payload = memprotocol.ReadReq{}
		return meta
	}
}

func findTransactionByReqID(transactions []transactionState, id uint64) int {
	for i, t := range transactions {
		if t.TranslationReqID == id {
			return i
		}
	}
	return -1
}

func removeTransaction(state *state, idx int) {
	state.Transactions = append(
		state.Transactions[:idx],
		state.Transactions[idx+1:]...)
}

func isReqInBottomByID(inflight []reqToBottomState, id uint64) bool {
	for _, r := range inflight {
		if r.ReqToBottomID == id {
			return true
		}
	}
	return false
}

func findReqToBottomByID(inflight []reqToBottomState, id uint64) reqToBottomState {
	for _, r := range inflight {
		if r.ReqToBottomID == id {
			return r
		}
	}
	panic("req to bottom not found")
}

func removeReqToBottomByID(state *state, id uint64) {
	for i, r := range state.InflightReqToBottom {
		if r.ReqToBottomID == id {
			state.InflightReqToBottom = append(
				state.InflightReqToBottom[:i],
				state.InflightReqToBottom[i+1:]...)
			return
		}
	}
	panic("req to bottom not found")
}

// buildReqToBottom creates a reqToBottomState from the incoming request
// and the translated outgoing request.
func buildReqToBottom(
	reqState incomingReqState, translatedReq messaging.Msg,
) reqToBottomState {
	return reqToBottomState{
		ReqFromTopID:    reqState.ID,
		ReqFromTopSrc:   reqState.Src,
		ReqFromTopDst:   reqState.Dst,
		ReqFromTopType:  reqState.Type,
		ReqToBottomID:   translatedReq.ID,
		ReqToBottomSrc:  translatedReq.Src,
		ReqToBottomDst:  translatedReq.Dst,
		ReqToBottomType: fmt.Sprintf("%T", translatedReq.Payload),
	}
}

// Comp is the AddressTranslator component.
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]
