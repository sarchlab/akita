package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the AddressTranslator.
type Spec struct {
	Freq           timing.Freq `json:"freq"`
	Log2PageSize   uint64      `json:"log2_page_size"`
	DeviceID       uint64      `json:"device_id"`
	NumReqPerCycle int         `json:"num_req_per_cycle"`

	// CanFault lets the translations of this translator fault when the page
	// is on another device (vmprotocol.TranslationReq.CanFault). A faulted
	// access waits until a Release of its page.
	CanFault bool `json:"can_fault"`
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

	// PID and VAddr locate the page of the access, so that a Hold of the
	// page can wait for it.
	PID   vm.PID `json:"pid"`
	VAddr uint64 `json:"vaddr"`
}

// heldPageState is a page whose accesses wait. Held is set by a Hold and
// Faulted by a faulted translation; a Release clears both.
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

// State contains mutable runtime data for the AddressTranslator.
type State struct {
	ControlState        memcontrolprotocol.State `json:"control_state"`
	CurrentCmdID        uint64                   `json:"current_cmd_id"`
	CurrentCmdSrc       messaging.RemotePort     `json:"current_cmd_src"`
	Transactions        []transactionState       `json:"transactions"`
	InflightReqToBottom []reqToBottomState       `json:"inflight_req_to_bottom"`

	// HeldPages are the pages whose accesses wait, PendingHolds the Holds
	// not acknowledged yet. HeldReqs are the waiting accesses, in arrival
	// order; ReplayReqs are the accesses that a Release let go, waiting to
	// be translated again.
	HeldPages    []heldPageState    `json:"held_pages"`
	PendingHolds []pendingHoldState `json:"pending_holds"`
	HeldReqs     []incomingReqState `json:"held_reqs"`
	ReplayReqs   []incomingReqState `json:"replay_reqs"`

	// NumHeldReqs counts the accesses that ever waited, NumFaults the
	// translations that faulted.
	NumHeldReqs uint64 `json:"num_held_reqs"`
	NumFaults   uint64 `json:"num_faults"`
}

// Helper functions

func addrToPageID(addr, log2PageSize uint64) uint64 {
	return (addr >> log2PageSize) << log2PageSize
}

func msgToIncomingReqState(msg messaging.Msg) incomingReqState {
	meta := msg.Meta()
	s := incomingReqState{
		ID:    meta.ID,
		Src:   meta.Src,
		Dst:   meta.Dst,
		RspTo: meta.RspTo,
		Type:  fmt.Sprintf("%T", msg),
	}

	switch req := msg.(type) {
	case memprotocol.ReadReq:
		s.Address = req.Address
		s.AccessByteSize = req.AccessByteSize
		s.PID = req.PID
		s.CanWaitForCoalesce = req.CanWaitForCoalesce
	case memprotocol.WriteReq:
		s.Address = req.Address
		s.PID = req.PID
		s.Data = req.Data
		s.DirtyMask = req.DirtyMask
		s.CanWaitForCoalesce = req.CanWaitForCoalesce
	default:
		log.Panicf("cannot convert message of type %T", msg)
	}

	return s
}

func createTranslatedReq(
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
		clone := memprotocol.ReadReq{}
		clone.ID = timing.GetIDGenerator().Generate()
		clone.Src = bottomPortRemote
		clone.Dst = memProviderMapper.Find(addr)
		clone.Address = addr
		clone.AccessByteSize = reqState.AccessByteSize
		clone.PID = 0
		clone.TrafficBytes = 12
		clone.TrafficClass = "memprotocol.ReadReq"
		clone.CanWaitForCoalesce = reqState.CanWaitForCoalesce
		return clone
	case "memprotocol.WriteReq":
		clone := memprotocol.WriteReq{}
		clone.ID = timing.GetIDGenerator().Generate()
		clone.Src = bottomPortRemote
		clone.Dst = memProviderMapper.Find(addr)
		clone.Data = reqState.Data
		clone.DirtyMask = reqState.DirtyMask
		clone.Address = addr
		clone.PID = 0
		clone.TrafficBytes = len(reqState.Data) + 12
		clone.TrafficClass = "memprotocol.WriteReq"
		clone.CanWaitForCoalesce = reqState.CanWaitForCoalesce
		return clone
	default:
		log.Panicf("cannot translate request of type %s", reqState.Type)
		return nil
	}
}

// restoreMemMsg reconstructs a concrete mem message from saved metadata. It
// preserves the original message ID so tracing lookups (keyed by the message
// ID) resolve to the same task as the original incoming message.
func restoreMemMsg(
	id uint64, src, dst messaging.RemotePort, rspTo uint64, typ string,
) messaging.Msg {
	meta := messaging.MsgMeta{ID: id, Src: src, Dst: dst, RspTo: rspTo}
	switch typ {
	case "memprotocol.WriteReq":
		return memprotocol.WriteReq{MsgMeta: meta}
	default:
		return memprotocol.ReadReq{MsgMeta: meta}
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

func removeTransaction(state *State, idx int) {
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

func removeReqToBottomByID(state *State, id uint64) {
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
		ReqToBottomID:   translatedReq.Meta().ID,
		ReqToBottomSrc:  translatedReq.Meta().Src,
		ReqToBottomDst:  translatedReq.Meta().Dst,
		ReqToBottomType: fmt.Sprintf("%T", translatedReq),
		PID:             reqState.PID,
		VAddr:           reqState.Address,
	}
}

// findHeldPage returns the index of the held page that holds vAddr of
// process pid, or -1.
func findHeldPage(state *State, pid vm.PID, vAddr, log2PageSize uint64) int {
	page := addrToPageID(vAddr, log2PageSize)

	for i, p := range state.HeldPages {
		if p.PID == pid && p.VAddr == page {
			return i
		}
	}

	return -1
}

// isPageHeld reports whether the accesses to the page of vAddr must wait.
func isPageHeld(state *State, pid vm.PID, vAddr, log2PageSize uint64) bool {
	return findHeldPage(state, pid, vAddr, log2PageSize) >= 0
}

// holdPage marks the page of vAddr held (by a Hold) or faulted.
func holdPage(state *State, pid vm.PID, vAddr, log2PageSize uint64, fault bool) {
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

// topTaskID returns the tracing task of the request received from the top.
func topTaskID(comp *Comp, reqState incomingReqState) uint64 {
	fakeFromTop := restoreMemMsg(reqState.ID, reqState.Src, reqState.Dst,
		reqState.RspTo, reqState.Type)

	return tracing.MsgIDAtReceiver(fakeFromTop, comp)
}

// Comp is the AddressTranslator component.
type Comp = modeling.Component[Spec, State, Resources]
