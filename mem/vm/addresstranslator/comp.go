package addresstranslator

import (
	"fmt"
	"log"
	"slices"
	"strconv"

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

	// Seq numbers the requests in the order they were taken from Top. The
	// held and replay lists are kept in this order.
	Seq uint64 `json:"seq"`

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

// heldRangeState is a run of pages, from First to Last (both page addresses,
// both inclusive), that a range Hold holds. A range is kept as an interval
// and never expanded into its pages.
type heldRangeState struct {
	PID   vm.PID `json:"pid"`
	First uint64 `json:"first"`
	Last  uint64 `json:"last"`
}

// pendingHoldState is a Hold that is not acknowledged yet: an access to one
// of its pages, or a translation of one, is still in flight, or an earlier
// Hold of one of its pages is not acknowledged yet. Pages is keyed
// by the page address; a range Hold sets HasRange and covers the pages from
// First to Last instead. A Reset cancels the Hold: it is then answered with
// memcontrolprotocol.ErrCanceledByReset.
type pendingHoldState struct {
	ID       uint64               `json:"id"`
	Src      messaging.RemotePort `json:"src"`
	PID      vm.PID               `json:"pid"`
	Pages    map[uint64]bool      `json:"pages"`
	HasRange bool                 `json:"has_range"`
	First    uint64               `json:"first"`
	Last     uint64               `json:"last"`
	Canceled bool                 `json:"canceled"`
}

// covers reports whether the Hold is of the page at address page of pid.
func (h pendingHoldState) covers(pid vm.PID, page uint64) bool {
	if h.PID != pid {
		return false
	}
	if h.HasRange {
		return h.First <= page && page <= h.Last
	}
	return h.Pages[page]
}

// overlaps reports whether the Hold and other hold a page of the same
// process in common. First, Last, and the keys of Pages are page addresses,
// so two Holds that name different bytes of one page overlap. A range is
// compared by its bounds and is never expanded into its pages.
func (h pendingHoldState) overlaps(other pendingHoldState) bool {
	if h.PID != other.PID {
		return false
	}

	switch {
	case h.HasRange && other.HasRange:
		return h.First <= other.Last && other.First <= h.Last
	case h.HasRange:
		return other.hasPageIn(h.First, h.Last)
	case other.HasRange:
		return h.hasPageIn(other.First, other.Last)
	}

	// The shorter list is searched in the longer one.
	few, many := h.Pages, other.Pages
	if len(many) < len(few) {
		few, many = many, few
	}

	for page := range few {
		if many[page] {
			return true
		}
	}

	return false
}

// hasPageIn reports whether a page of the list of the Hold is one of the
// pages from first to last.
func (h pendingHoldState) hasPageIn(first, last uint64) bool {
	for page := range h.Pages {
		if first <= page && page <= last {
			return true
		}
	}

	return false
}

// state contains mutable runtime data for the AddressTranslator.
type state struct {
	ControlState        controlState         `json:"control_state"`
	CurrentCmdID        uint64               `json:"current_cmd_id"`
	CurrentCmdSrc       messaging.RemotePort `json:"current_cmd_src"`
	Transactions        []transactionState   `json:"transactions"`
	InflightReqToBottom []reqToBottomState   `json:"inflight_req_to_bottom"`

	// HeldPages are the pages whose accesses wait, keyed by heldPageKey;
	// HeldRanges are the page runs that range Holds hold. PendingHolds are
	// the Holds not acknowledged yet, in arrival order: a Hold that is
	// acknowledged before an earlier one leaves the order of the others
	// unchanged. HeldReqs is the held list: the waiting
	// accesses in arrival order. They use no transaction slot, so accesses to other
	// pages keep flowing. ReplayReqs are the accesses that a Release let go,
	// waiting to be sent for translation again. Both lists are in the order
	// of incomingReqState.Seq, also when an access joins late: one whose
	// translation returns after a later access was held, or one that a
	// later Release lets go. NextReqSeq is the Seq of the next request.
	HeldPages    map[string]heldPageState `json:"held_pages"`
	HeldRanges   []heldRangeState         `json:"held_ranges"`
	PendingHolds []pendingHoldState       `json:"pending_holds"`
	HeldReqs     []incomingReqState       `json:"held_reqs"`
	ReplayReqs   []incomingReqState       `json:"replay_reqs"`
	NextReqSeq   uint64                   `json:"next_req_seq"`

	// NumHeldReqs counts the accesses that ever waited, NumFaults the fault
	// responses.
	NumHeldReqs uint64 `json:"num_held_reqs"`
	NumFaults   uint64 `json:"num_faults"`
}

// Helper functions

func addrToPageID(addr, log2PageSize uint64) uint64 {
	return (addr >> log2PageSize) << log2PageSize
}

// insertBySeq adds req to reqs, a list in the order the requests were taken
// from Top, and keeps that order. The newest request is appended, so the
// search starts at the end.
func insertBySeq(reqs []incomingReqState, req incomingReqState) []incomingReqState {
	i := len(reqs)
	for i > 0 && reqs[i-1].Seq > req.Seq {
		i--
	}

	return slices.Insert(reqs, i, req)
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

// heldPageKey is the key of the page of vAddr of process pid in
// State.HeldPages.
func heldPageKey(pid vm.PID, vAddr, log2PageSize uint64) string {
	return strconv.FormatUint(uint64(pid), 10) + "/" +
		strconv.FormatUint(addrToPageID(vAddr, log2PageSize), 16)
}

// isPageHeld reports whether the accesses to the page of vAddr must wait.
func isPageHeld(state *state, pid vm.PID, vAddr, log2PageSize uint64) bool {
	page := addrToPageID(vAddr, log2PageSize)
	for _, r := range state.HeldRanges {
		if r.PID == pid && r.First <= page && page <= r.Last {
			return true
		}
	}

	if len(state.HeldPages) == 0 {
		return false
	}

	_, ok := state.HeldPages[heldPageKey(pid, vAddr, log2PageSize)]

	return ok
}

// holdRange marks the pages from first to last of process pid held. The
// ranges of a process that overlap are merged, so that repeating a Hold does
// not grow the state.
func holdRange(state *state, pid vm.PID, first, last uint64) {
	kept := state.HeldRanges[:0]
	for _, r := range state.HeldRanges {
		if r.PID != pid || r.Last < first || last < r.First {
			kept = append(kept, r)
			continue
		}

		first, last = min(first, r.First), max(last, r.Last)
	}

	state.HeldRanges = append(kept,
		heldRangeState{PID: pid, First: first, Last: last})
}

// releasePages ends the Hold and the faults of the pages from first to last
// of process pid. A held range that is released in part keeps its other
// pages.
func releasePages(state *state, pid vm.PID, first, last, log2PageSize uint64) {
	if first == last {
		delete(state.HeldPages, heldPageKey(pid, first, log2PageSize))
	} else {
		for key, p := range state.HeldPages {
			if p.PID == pid && first <= p.VAddr && p.VAddr <= last {
				delete(state.HeldPages, key)
			}
		}
	}

	pageSize := uint64(1) << log2PageSize
	var kept []heldRangeState
	for _, r := range state.HeldRanges {
		if r.PID != pid || r.Last < first || last < r.First {
			kept = append(kept, r)
			continue
		}

		if r.First < first {
			kept = append(kept,
				heldRangeState{PID: pid, First: r.First, Last: first - pageSize})
		}
		if last < r.Last {
			kept = append(kept,
				heldRangeState{PID: pid, First: last + pageSize, Last: r.Last})
		}
	}
	state.HeldRanges = kept
}

// holdPage marks the page of vAddr held (by a Hold) or faulted.
func holdPage(state *state, pid vm.PID, vAddr, log2PageSize uint64, fault bool) {
	if state.HeldPages == nil {
		state.HeldPages = make(map[string]heldPageState)
	}

	key := heldPageKey(pid, vAddr, log2PageSize)
	p, ok := state.HeldPages[key]
	if !ok {
		p = heldPageState{PID: pid, VAddr: addrToPageID(vAddr, log2PageSize)}
	}

	if fault {
		p.Faulted = true
	} else {
		p.Held = true
	}

	state.HeldPages[key] = p
}

// Comp is the AddressTranslator component.
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]
