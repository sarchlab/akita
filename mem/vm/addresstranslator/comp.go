package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec contains immutable configuration for the AddressTranslator.
type Spec struct {
	// Mem routing configuration.
	Bottom mem.AddressMapper `json:"bottom"`

	// Translation routing configuration.
	Translation mem.AddressMapper `json:"translation"`

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
	Translation messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol.requester"`

	// Control receives enable, pause, drain, reset, hold, and release
	// commands.
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
	Address        uint64 `json:"address"`
	AccessByteSize uint64 `json:"access_byte_size"`
	PID            vm.PID `json:"pid"`
	Data           []byte `json:"data,omitempty"`
	DirtyMask      []bool `json:"dirty_mask,omitempty"`
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

	// PID and VAddr locate the page of the access, so that a Hold of the
	// page can wait for it.
	PID   vm.PID `json:"pid"`
	VAddr uint64 `json:"vaddr"`
}

// heldPageState is a page whose accesses wait. Held is set by a Hold and
// Faulted by a fault response; a Release clears both.
type heldPageState struct {
	PID     vm.PID `json:"pid"`
	VAddr   uint64 `json:"vaddr"`
	Held    bool   `json:"held"`
	Faulted bool   `json:"faulted"`
}

// pendingHoldState is a Hold that is not acknowledged yet: an access to one
// of its pages, or a translation of one, is still in flight.
type pendingHoldState struct {
	ID     uint64               `json:"id"`
	Src    messaging.RemotePort `json:"src"`
	PID    vm.PID               `json:"pid"`
	VAddrs []uint64             `json:"vaddrs"`
}

// state contains mutable runtime data for the AddressTranslator.
type state struct {
	ControlState        controlState         `json:"control_state"`
	CurrentCmdID        uint64               `json:"current_cmd_id"`
	CurrentCmdSrc       messaging.RemotePort `json:"current_cmd_src"`
	Transactions        []transactionState   `json:"transactions"`
	InflightReqToBottom []reqToBottomState   `json:"inflight_req_to_bottom"`

	// HeldPages are the pages whose accesses wait, PendingHolds the Holds
	// not acknowledged yet. HeldReqs is the held list: the waiting accesses
	// in arrival order. They use no transaction slot, so accesses to other
	// pages keep flowing. ReplayReqs are the accesses that a Release let go,
	// waiting to be sent for translation again.
	HeldPages    []heldPageState    `json:"held_pages"`
	PendingHolds []pendingHoldState `json:"pending_holds"`
	HeldReqs     []incomingReqState `json:"held_reqs"`
	ReplayReqs   []incomingReqState `json:"replay_reqs"`

	// NumHeldReqs counts the accesses that ever waited, NumFaults the fault
	// responses.
	NumHeldReqs uint64 `json:"num_held_reqs"`
	NumFaults   uint64 `json:"num_faults"`
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
	case memprotocol.WriteReq:
		s.Address = content.Address
		s.PID = content.PID
		s.Data = content.Data
		s.DirtyMask = content.DirtyMask
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
		},
			ID:  newID(),
			Src: bottomPortRemote,
			Dst: memProviderMapper.Find(addr),

			TrafficBytes: 12,
		}

		return clone
	case "memprotocol.WriteReq":
		clone := messaging.Msg{Payload: memprotocol.WriteReq{
			Data:      reqState.Data,
			DirtyMask: reqState.DirtyMask,
			Address:   addr,
			PID:       0,
		},
			ID:  newID(),
			Src: bottomPortRemote,
			Dst: memProviderMapper.Find(addr),

			TrafficBytes: len(reqState.Data) + 12,
		}

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
		PID:             reqState.PID,
		VAddr:           reqState.Address,
	}
}

// findHeldPage returns the index of the held page that holds vAddr of
// process pid, or -1.
func findHeldPage(state *state, pid vm.PID, vAddr, log2PageSize uint64) int {
	page := addrToPageID(vAddr, log2PageSize)

	for i, p := range state.HeldPages {
		if p.PID == pid && p.VAddr == page {
			return i
		}
	}

	return -1
}

// isPageHeld reports whether the accesses to the page of vAddr must wait.
func isPageHeld(state *state, pid vm.PID, vAddr, log2PageSize uint64) bool {
	return findHeldPage(state, pid, vAddr, log2PageSize) >= 0
}

// holdPage marks the page of vAddr held (by a Hold) or faulted.
func holdPage(state *state, pid vm.PID, vAddr, log2PageSize uint64, fault bool) {
	i := findHeldPage(state, pid, vAddr, log2PageSize)
	if i < 0 {
		state.HeldPages = append(state.HeldPages, heldPageState{
			PID:   pid,
			VAddr: addrToPageID(vAddr, log2PageSize),
		})
		i = len(state.HeldPages) - 1
	}

	if fault {
		state.HeldPages[i].Faulted = true
	} else {
		state.HeldPages[i].Held = true
	}
}

// Comp is the AddressTranslator component.
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]
