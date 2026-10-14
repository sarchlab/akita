package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/internal/addressmapping"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec contains immutable configuration for the AddressTranslator.
type Spec struct {
	// Mem routing configuration.
	MemMapperType                mem.AddressMapperType  `json:"mem_mapper_type"`
	MemProviders                 []messaging.RemotePort `json:"mem_providers"`
	MemInterleavingSize          uint64                 `json:"mem_interleaving_size"`
	MemBankSize                  uint64                 `json:"mem_bank_size"`
	MemUseAddressSpaceLimitation bool                   `json:"mem_use_address_space_limitation"`
	MemLowAddress                uint64                 `json:"mem_low_address"`
	MemHighAddress               uint64                 `json:"mem_high_address"`
	MemModuleForOtherAddresses   messaging.RemotePort   `json:"mem_module_for_other_addresses"`

	// Translation routing configuration.
	TranslationMapperType                mem.AddressMapperType  `json:"translation_mapper_type"`
	TranslationProviders                 []messaging.RemotePort `json:"translation_providers"`
	TranslationInterleavingSize          uint64                 `json:"translation_interleaving_size"`
	TranslationBankSize                  uint64                 `json:"translation_bank_size"`
	TranslationUseAddressSpaceLimitation bool                   `json:"translation_use_address_space_limitation"`
	TranslationLowAddress                uint64                 `json:"translation_low_address"`
	TranslationHighAddress               uint64                 `json:"translation_high_address"`
	TranslationModuleForOtherAddresses   messaging.RemotePort   `json:"translation_module_for_other_addresses"`

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

// Resources is empty; routing configuration belongs to Spec.
type Resources struct{}

// incomingReqState is a serializable representation of an incoming request.
type incomingReqState struct {
	RecvTaskID uint64 `json:"recv_task_id"`

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
	TranslationTaskID uint64 `json:"translation_task_id"`

	IncomingReqs      []incomingReqState   `json:"incoming_reqs"`
	TranslationReqID  uint64               `json:"translation_req_id"`
	TranslationReqSrc messaging.RemotePort `json:"translation_req_src"`
	TranslationReqDst messaging.RemotePort `json:"translation_req_dst"`
	TranslationDone   bool                 `json:"translation_done"`
	Page              vm.Page              `json:"page"`
}

// reqToBottomState is a serializable representation of a runtime reqToBottom.
type reqToBottomState struct {
	RecvTaskID uint64 `json:"recv_task_id"`
	SendTaskID uint64 `json:"send_task_id"`

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
		RecvTaskID:      reqState.RecvTaskID,
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

func (s Spec) memMapping() addressmapping.Config {
	return addressmapping.Config{Kind: s.MemMapperType, Ports: s.MemProviders,
		InterleavingSize:          s.MemInterleavingSize,
		BankSize:                  s.MemBankSize,
		UseAddressSpaceLimitation: s.MemUseAddressSpaceLimitation,
		LowAddress:                s.MemLowAddress,
		HighAddress:               s.MemHighAddress,
		ModuleForOtherAddresses:   s.MemModuleForOtherAddresses}
}

func (s Spec) translationMapping() addressmapping.Config {
	return addressmapping.Config{Kind: s.TranslationMapperType, Ports: s.TranslationProviders,
		InterleavingSize:          s.TranslationInterleavingSize,
		BankSize:                  s.TranslationBankSize,
		UseAddressSpaceLimitation: s.TranslationUseAddressSpaceLimitation,
		LowAddress:                s.TranslationLowAddress,
		HighAddress:               s.TranslationHighAddress,
		ModuleForOtherAddresses:   s.TranslationModuleForOtherAddresses}
}
