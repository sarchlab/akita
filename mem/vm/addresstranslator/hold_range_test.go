package addresstranslator

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

// controlReq sends a control request and returns its ID.
func (h *holdHarness) controlReq(req memcontrolprotocol.Req) uint64 {
	msg := messaging.Msg{Payload: req, ID: h.sim.NewID(),
		Src: "Driver", Dst: h.ctrl.AsRemote()}
	h.ctrl.Deliver(msg)

	return msg.ID
}

// controlRange sends a range command of process 1 and returns its ID.
func (h *holdHarness) controlRange(
	cmd memcontrolprotocol.Command,
	start, end uint64,
) uint64 {
	return h.controlReq(memcontrolprotocol.Req{Command: cmd, PID: 1, HasRange: true,
		Range: memcontrolprotocol.AddressRange{Start: start, End: end}})
}

// mustAck ticks once and returns the response to the command id.
func (h *holdHarness) mustAck(id uint64) memcontrolprotocol.Rsp {
	h.t.Helper()

	h.tick()
	rsp, rspTo, ok := h.ack()
	require.True(h.t, ok)
	require.Equal(h.t, id, rspTo)

	return rsp
}

// translated returns the pages of the translation requests sent so far.
func (h *holdHarness) translated() []uint64 {
	var pages []uint64
	for {
		req, ok := h.translationReq()
		if !ok {
			return pages
		}
		pages = append(pages, req.Payload.(addrtransprotocol.Req).VAddr)
	}
}

// accessAll sends accesses in order, ticking until the translator took each.
func (h *holdHarness) accessAll(vAddrs ...uint64) {
	for _, vAddr := range vAddrs {
		h.access(vAddr)
		h.tick()
		h.tick()
	}
}

func TestRangeHoldHoldsEveryIntersectingPage(t *testing.T) {
	h := newHoldHarness(t)

	// The range touches the last byte of page 0x10000 and the first byte
	// of page 0x12000.
	rsp := h.mustAck(h.controlRange(memcontrolprotocol.CmdHold, 0x10fff, 0x12001))
	require.Equal(t, memcontrolprotocol.Rsp{Command: memcontrolprotocol.CmdHold, Success: true}, rsp)

	h.accessAll(0xffc0, 0x10000, 0x11040, 0x12ffc, 0x13000)

	require.Equal(t, []uint64{0xf000, 0x13000}, h.translated(),
		"the pages before and after the range are not held")
	require.Len(t, h.at.State.HeldReqs, 3)
	require.Empty(t, h.at.State.HeldPages, "a range is not expanded into its pages")
	require.Equal(t, []heldRangeState{{PID: 1, First: 0x10000, Last: 0x12000}},
		h.at.State.HeldRanges)

	// An end on a page boundary excludes that page.
	rsp = h.mustAck(h.controlRange(memcontrolprotocol.CmdRelease, 0x10000, 0x12000))
	require.Equal(t, memcontrolprotocol.Rsp{Command: memcontrolprotocol.CmdRelease, Success: true}, rsp)
	h.tick()
	require.Equal(t, []uint64{0x10000, 0x11000}, h.translated(),
		"the released accesses are replayed in arrival order")
	require.Len(t, h.at.State.HeldReqs, 1)
	require.Equal(t, []heldRangeState{{PID: 1, First: 0x12000, Last: 0x12000}},
		h.at.State.HeldRanges)
}

func TestRangeHoldOfTheAddressSpaceIsBounded(t *testing.T) {
	h := newHoldHarness(t)

	holdID := h.controlRange(memcontrolprotocol.CmdHold, 1, ^uint64(0))
	h.at.Middlewares.Ctrl.handleIncoming()
	require.Len(t, h.at.State.PendingHolds, 1)
	require.Empty(t, h.at.State.PendingHolds[0].Pages)
	require.True(t, h.mustAck(holdID).Success)

	const top = ^uint64(0) &^ 0xfff
	require.Empty(t, h.at.State.HeldPages)
	require.Equal(t, []heldRangeState{{PID: 1, First: 0, Last: top}}, h.at.State.HeldRanges)

	h.accessAll(0, 0x10040, top+0xff0)
	require.Empty(t, h.translated())
	require.Len(t, h.at.State.HeldReqs, 3)

	// Another process is not held.
	h.top.Deliver(messaging.Msg{
		Payload: memprotocol.ReadReq{Address: 0x10040, AccessByteSize: 4, PID: 2},
		ID:      h.sim.NewID(),
		Src:     "Agent",
		Dst:     h.top.AsRemote(),
	})
	h.tick()
	require.Equal(t, []uint64{0x10000}, h.translated())

	// Repeated and overlapping Holds do not grow the state.
	for _, r := range [][2]uint64{{1, ^uint64(0)}, {0x5000, 0x9000}, {0, 1}} {
		require.True(t, h.mustAck(h.controlRange(memcontrolprotocol.CmdHold, r[0], r[1])).Success)
	}
	require.Equal(t, []heldRangeState{{PID: 1, First: 0, Last: top}}, h.at.State.HeldRanges)

	// A Release of the last page splits it off, without overflow.
	require.True(t, h.mustAck(h.controlRange(memcontrolprotocol.CmdRelease, top, ^uint64(0))).Success)
	require.Equal(t, []heldRangeState{{PID: 1, First: 0, Last: top - 0x1000}}, h.at.State.HeldRanges)
	h.tick()
	require.Equal(t, []uint64{top}, h.translated())

	require.True(t, h.mustAck(h.controlRange(memcontrolprotocol.CmdRelease, 0, ^uint64(0))).Success)
	require.Empty(t, h.at.State.HeldRanges)
	h.tick()
	require.Equal(t, []uint64{0, 0x10000}, h.translated())
	require.Empty(t, h.at.State.HeldReqs)
}

func TestRangeHoldAcksAfterTheAccessesInFlightComplete(t *testing.T) {
	h := newHoldHarness(t)

	// One access to the range is issued to the bottom, and the translation
	// of another is in flight.
	h.accessAll(0x10040)
	req, _ := h.translationReq()
	h.answer(req, false)
	h.tick()
	h.accessAll(0x11040)
	pending, _ := h.translationReq()

	holdID := h.controlRange(memcontrolprotocol.CmdHold, 0x10800, 0x11001)
	h.tick()
	// A later Hold of a page of the range is acknowledged after the range
	// Hold, also when its own page is drained first.
	laterID := h.controlAt(memcontrolprotocol.CmdHold, 0x10000)
	h.tick()
	_, _, ok := h.ack()
	require.False(t, ok, "the Hold waits for both accesses")

	h.complete()
	h.tick()
	h.tick()
	_, _, ok = h.ack()
	require.False(t, ok, "the Hold waits for the translation in flight")
	require.False(t, h.at.Middlewares.Ctrl.inFlight(h.at.State.PendingHolds[1]),
		"the later Hold only waits for the earlier one")

	h.answer(pending, false)
	h.tick()
	_, sent := h.bottom.RetrieveOutgoing()
	require.False(t, sent, "the access translated after the Hold is held")

	require.True(t, h.mustAck(holdID).Success)
	require.True(t, h.mustAck(laterID).Success)
	require.Len(t, h.at.State.HeldReqs, 1)
}

func TestReleaseTakesPagesOutOfAHeldRange(t *testing.T) {
	h := newHoldHarness(t)

	require.True(t, h.mustAck(h.controlRange(memcontrolprotocol.CmdHold, 0x10000, 0x14000)).Success)
	h.accessAll(0x13040, 0x10040, 0x12040, 0x11040)
	require.Empty(t, h.translated())

	// A range Release in the middle keeps both ends held.
	require.True(t, h.mustAck(h.controlRange(memcontrolprotocol.CmdRelease, 0x11fff, 0x12001)).Success)
	require.Equal(t, []heldRangeState{
		{PID: 1, First: 0x10000, Last: 0x10000},
		{PID: 1, First: 0x13000, Last: 0x13000},
	}, h.at.State.HeldRanges)
	h.tick()
	require.Equal(t, []uint64{0x12000, 0x11000}, h.translated(),
		"the released accesses are replayed in arrival order")

	// A Release by address releases a page that a range holds.
	require.True(t, h.mustAck(h.controlAt(memcontrolprotocol.CmdRelease, 0x13abc)).Success)
	require.Equal(t, []heldRangeState{{PID: 1, First: 0x10000, Last: 0x10000}},
		h.at.State.HeldRanges)
	h.tick()
	require.Equal(t, []uint64{0x13000}, h.translated())

	// Another process's Release does not release the page.
	require.True(t, h.mustAck(h.controlReq(memcontrolprotocol.Req{
		Command: memcontrolprotocol.CmdRelease, PID: 2, HasRange: true,
		Range: memcontrolprotocol.AddressRange{Start: 0, End: 0x100000},
	})).Success)
	h.tick()
	require.Empty(t, h.translated())
	require.Len(t, h.at.State.HeldReqs, 1)
}

func TestRangeReleaseEndsListHoldsAndFaults(t *testing.T) {
	h := newHoldHarness(t)

	// Page 0x10000 faults, page 0x11000 is held by address, and page
	// 0x20000 is held by address outside the released range.
	h.accessAll(0x10040)
	req, _ := h.translationReq()
	h.answer(req, true)
	h.tick()
	require.True(t, h.mustAck(h.controlAt(memcontrolprotocol.CmdHold, 0x11000)).Success)
	require.True(t, h.mustAck(h.controlAt(memcontrolprotocol.CmdHold, 0x20000)).Success)
	h.accessAll(0x11040, 0x20040)
	require.Len(t, h.at.State.HeldReqs, 3)
	require.Len(t, h.at.State.HeldPages, 3)

	require.True(t, h.mustAck(h.controlRange(memcontrolprotocol.CmdRelease, 0x10001, 0x11001)).Success)
	h.tick()
	require.Equal(t, []uint64{0x10000, 0x11000}, h.translated())
	require.Len(t, h.at.State.HeldPages, 1)
	require.Len(t, h.at.State.HeldReqs, 1)
}

// addresses returns the addresses of the accesses in a held or replay list.
func addresses(reqs []incomingReqState) []uint64 {
	addrs := make([]uint64, 0, len(reqs))
	for _, r := range reqs {
		addrs = append(addrs, r.Address)
	}
	return addrs
}

// An access whose translation is in flight at the Hold is older than the
// accesses that arrive under the Hold. It joins the held list when its
// translation returns, and a Release must replay it first.
func TestReleaseReplaysAnAccessHeldLateBeforeTheLaterAccesses(t *testing.T) {
	h := newHoldHarness(t)

	h.accessAll(0x10040)
	older, sent := h.translationReq()
	require.True(t, sent)

	holdID := h.controlRange(memcontrolprotocol.CmdHold, 0x10000, 0x12000)
	h.tick()
	_, _, ok := h.ack()
	require.False(t, ok, "the Hold waits for the translation in flight")

	// The later accesses, to the same page and to another page of the
	// range, are held at once.
	h.accessAll(0x10080, 0x11040)
	require.Equal(t, []uint64{0x10080, 0x11040}, addresses(h.at.State.HeldReqs))

	h.answer(older, false)
	h.tick()
	require.Equal(t, []uint64{0x10040, 0x10080, 0x11040}, addresses(h.at.State.HeldReqs),
		"the older access takes its place before the later ones")
	require.True(t, h.mustAck(holdID).Success)

	require.True(t, h.mustAck(h.controlRange(memcontrolprotocol.CmdRelease, 0x10000, 0x12000)).Success)
	replayed := make([]uint64, 0, 3)
	for range 6 {
		h.tick()
		replayed = append(replayed, h.translated()...)
	}
	require.Equal(t, []uint64{0x10000, 0x10000, 0x11000}, replayed)
	require.Equal(t, []uint64{0x10040, 0x10080, 0x11040}, func() []uint64 {
		addrs := make([]uint64, 0, 3)
		for _, tr := range h.at.State.Transactions {
			addrs = append(addrs, addresses(tr.IncomingReqs)...)
		}
		return addrs
	}(), "the accesses are translated again in the order they arrived")
}

// The order holds across translations that return out of order, a partial
// Release, a checkpoint, and a later Release of older accesses.
func TestPartialReleasesKeepTheArrivalOrderAcrossACheckpoint(t *testing.T) {
	h := newHoldHarness(t)

	// A and B are in flight at the Hold; C and D arrive under it.
	h.accessAll(0x10040, 0x12040)
	transA, _ := h.translationReq()
	transB, _ := h.translationReq()
	holdID := h.controlRange(memcontrolprotocol.CmdHold, 0x10000, 0x13000)
	h.tick()
	h.accessAll(0x11040, 0x12080)

	// B returns before A.
	h.answer(transB, false)
	h.tick()
	h.answer(transA, false)
	h.tick()
	require.Equal(t, []uint64{0x10040, 0x12040, 0x11040, 0x12080},
		addresses(h.at.State.HeldReqs))
	require.True(t, h.mustAck(holdID).Success)

	// The Release of the last page lets B and D go. Only the control
	// middleware runs, so they still wait for replay at the checkpoint.
	releaseID := h.controlRange(memcontrolprotocol.CmdRelease, 0x12000, 0x13000)
	h.at.Middlewares.Ctrl.Handle(nil)
	_, rspTo, ok := h.ack()
	require.True(t, ok)
	require.Equal(t, releaseID, rspTo)
	require.Equal(t, []uint64{0x10040, 0x11040}, addresses(h.at.State.HeldReqs))
	require.Equal(t, []uint64{0x12040, 0x12080}, addresses(h.at.State.ReplayReqs))

	var buf bytes.Buffer
	require.NoError(t, h.at.SaveCheckpoint(&buf))
	restored := newHoldHarness(t)
	require.NoError(t, restored.at.LoadCheckpoint(&buf))
	require.Equal(t, h.at.State, restored.at.State)

	// The Release of the other pages lets the older A and C go: they are
	// replayed before and between the accesses released earlier.
	restored.controlRange(memcontrolprotocol.CmdRelease, 0x10000, 0x12000)
	restored.at.Middlewares.Ctrl.Handle(nil)
	require.Equal(t, []uint64{0x10040, 0x12040, 0x11040, 0x12080},
		addresses(restored.at.State.ReplayReqs))

	replayed := make([]uint64, 0, 4)
	for range 8 {
		restored.tick()
		replayed = append(replayed, restored.translated()...)
	}
	require.Equal(t, []uint64{0x10000, 0x12000, 0x11000, 0x12000}, replayed)

	// A new access follows the replayed ones.
	restored.accessAll(0x13040)
	require.Equal(t, uint64(4), restored.at.State.Transactions[4].IncomingReqs[0].Seq)
}

func TestCheckpointKeepsHeldRangesAndPendingRangeHolds(t *testing.T) {
	h := newHoldHarness(t)

	h.accessAll(0x11040)
	pending, _ := h.translationReq()
	require.True(t, h.mustAck(h.controlRange(memcontrolprotocol.CmdHold, 0x30000, 0x50000)).Success)
	holdID := h.controlRange(memcontrolprotocol.CmdHold, 0x10001, 0x11001)
	h.at.Middlewares.Ctrl.Handle(nil)

	require.Len(t, h.at.State.PendingHolds, 1)
	require.Len(t, h.at.State.HeldRanges, 2)

	var buf bytes.Buffer
	require.NoError(t, h.at.SaveCheckpoint(&buf))

	restored := newHoldHarness(t)
	require.NoError(t, restored.at.LoadCheckpoint(&buf))
	require.Equal(t, h.at.State, restored.at.State)

	// The restored translator holds the access whose translation returns,
	// acks the Hold, and holds a new access to the other range.
	restored.answer(pending, false)
	restored.tick()
	_, sent := restored.bottom.RetrieveOutgoing()
	require.False(t, sent)
	require.True(t, restored.mustAck(holdID).Success)

	restored.accessAll(0x4fffc, 0x50000)
	require.Equal(t, []uint64{0x50000}, restored.translated())
	require.Len(t, restored.at.State.HeldReqs, 2)
}

func TestResetCancelsPendingRangeHoldsAndDropsHeldRanges(t *testing.T) {
	h := newHoldHarness(t)

	h.accessAll(0x10040)
	holdID := h.controlRange(memcontrolprotocol.CmdHold, 0x10000, 0x20000)
	h.tick()
	_, _, ok := h.ack()
	require.False(t, ok)

	resetID := h.controlReq(memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset})
	require.True(t, h.mustAck(resetID).Success)

	rsp := h.mustAck(holdID)
	require.Equal(t, memcontrolprotocol.Rsp{Command: memcontrolprotocol.CmdHold,
		Error: memcontrolprotocol.ErrCanceledByReset}, rsp)
	require.Empty(t, h.at.State.HeldRanges)

	h.translated()
	h.accessAll(0x10040)
	require.Equal(t, []uint64{0x10000}, h.translated(), "the range is not held after the Reset")
}

func TestReleaseOfARangeBeforeItsHoldCompletesPanics(t *testing.T) {
	for _, hold := range []memcontrolprotocol.Req{
		{Command: memcontrolprotocol.CmdHold, PID: 1, HasRange: true,
			Range: memcontrolprotocol.AddressRange{Start: 0x10000, End: 0x20000}},
		{Command: memcontrolprotocol.CmdHold, PID: 1, Addresses: []uint64{0x10000}},
	} {
		h := newHoldHarness(t)
		h.accessAll(0x10040)
		h.controlReq(hold)
		h.tick()

		// The Release overlaps the Hold in one page only.
		h.controlRange(memcontrolprotocol.CmdRelease, 0x1000, 0x10001)
		require.Panics(t, h.tick)
	}
}

func TestHoldAndReleaseRejectInvalidRangeAddressing(t *testing.T) {
	type AR = memcontrolprotocol.AddressRange
	cases := []struct {
		name string
		req  memcontrolprotocol.Req
		want string
	}{
		{"empty zero range", memcontrolprotocol.Req{HasRange: true}, memcontrolprotocol.ErrInvalidRange},
		{"empty range", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 0x10000, End: 0x10000}},
			memcontrolprotocol.ErrInvalidRange},
		{"reversed range", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 0x20000, End: 0x10000}},
			memcontrolprotocol.ErrInvalidRange},
		{"range without HasRange", memcontrolprotocol.Req{Range: AR{Start: 0x10000, End: 0x11000}},
			memcontrolprotocol.ErrInvalidRange},
		{"range and addresses", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 0x10000, End: 0x11000},
			Addresses: []uint64{0x10000}}, memcontrolprotocol.ErrAmbiguousAddressing},
	}

	for _, tc := range cases {
		h := newHoldHarness(t)

		// A rejected Hold holds nothing.
		tc.req.PID = vm.PID(1)
		tc.req.Command = memcontrolprotocol.CmdHold
		rsp := h.mustAck(h.controlReq(tc.req))
		require.Equal(t, memcontrolprotocol.Rsp{Command: memcontrolprotocol.CmdHold, Error: tc.want}, rsp, tc.name)
		require.Empty(t, h.at.State.PendingHolds)
		require.Empty(t, h.at.State.HeldPages)
		require.Empty(t, h.at.State.HeldRanges)

		// A rejected Release releases nothing.
		require.True(t, h.mustAck(h.controlAt(memcontrolprotocol.CmdHold, 0x10000)).Success)
		h.accessAll(0x10040)
		tc.req.Command = memcontrolprotocol.CmdRelease
		rsp = h.mustAck(h.controlReq(tc.req))
		require.Equal(t, memcontrolprotocol.Rsp{Command: memcontrolprotocol.CmdRelease, Error: tc.want}, rsp, tc.name)
		h.tick()
		require.Empty(t, h.translated(), tc.name)
		require.Len(t, h.at.State.HeldReqs, 1)
	}
}
