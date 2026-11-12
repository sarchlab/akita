package addresstranslator

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/stretchr/testify/require"
)

// listHold returns a Hold of the pages at addrs of process pid.
func listHold(pid vm.PID, addrs ...uint64) memcontrolprotocol.Req {
	return memcontrolprotocol.Req{Command: memcontrolprotocol.CmdHold, PID: pid,
		Addresses: addrs}
}

// rangeHold returns a Hold of the pages that the bytes from start to end
// (exclusive) of process pid intersect.
func rangeHold(pid vm.PID, start, end uint64) memcontrolprotocol.Req {
	return memcontrolprotocol.Req{Command: memcontrolprotocol.CmdHold, PID: pid,
		HasRange: true,
		Range:    memcontrolprotocol.AddressRange{Start: start, End: end}}
}

// noAck checks that the translator sent no control response.
func (h *holdHarness) noAck(msg string) {
	h.t.Helper()

	_, _, ok := h.ack()
	require.False(h.t, ok, msg)
}

// holdAcks returns the IDs of the Holds that the responses sent so far
// acknowledge, in the order of the responses.
func (h *holdHarness) holdAcks() []uint64 {
	h.t.Helper()

	var ids []uint64
	for {
		rsp, rspTo, ok := h.ack()
		if !ok {
			return ids
		}

		require.Equal(h.t, memcontrolprotocol.Rsp{
			Command: memcontrolprotocol.CmdHold, Success: true}, rsp)
		ids = append(ids, rspTo)
	}
}

// pendingHoldIDs returns the IDs of the Holds not acknowledged yet, in the
// order the state keeps them.
func (h *holdHarness) pendingHoldIDs() []uint64 {
	ids := make([]uint64, 0, len(h.at.State.PendingHolds))
	for _, hold := range h.at.State.PendingHolds {
		ids = append(ids, hold.ID)
	}

	return ids
}

func TestHoldAcksBeforeAnEarlierHoldOfOtherPages(t *testing.T) {
	h := newHoldHarness(t)

	// An access to page 0x10000 is issued to the bottom and not answered.
	h.accessAll(0x10040)
	req, _ := h.translationReq()
	h.answer(req, false)
	h.tick()

	holdA := h.controlAt(memcontrolprotocol.CmdHold, 0x10000)
	h.tick()
	h.noAck("Hold A waits for the outstanding access")

	// Hold B, of a page with nothing in flight, does not wait for Hold A.
	holdB := h.controlAt(memcontrolprotocol.CmdHold, 0x20000)
	require.Equal(t, memcontrolprotocol.Rsp{
		Command: memcontrolprotocol.CmdHold, Success: true}, h.mustAck(holdB))
	require.Equal(t, []uint64{holdA}, h.pendingHoldIDs())

	// The page of Hold B stays held after the ack.
	h.accessAll(0x20040)
	require.Empty(t, h.translated())
	require.Equal(t, []uint64{0x20040}, addresses(h.at.State.HeldReqs))

	// Its Release is legal while Hold A waits, and leaves the page of Hold A
	// held.
	require.True(t, h.mustAck(h.controlAt(memcontrolprotocol.CmdRelease, 0x20000)).Success)
	h.tick()
	require.Equal(t, []uint64{0x20000}, h.translated())
	require.True(t, isPageHeld(&h.at.State, 1, 0x10000, h.at.Spec.Log2PageSize))
	h.noAck("Hold A still waits")

	h.complete()
	h.tick()
	require.True(t, h.mustAck(holdA).Success)
	require.Empty(t, h.at.State.PendingHolds)
}

func TestHoldsOfOnePageAckInArrivalOrder(t *testing.T) {
	h := newHoldHarness(t)

	// An access to page 0x10000 is issued to the bottom and not answered.
	h.accessAll(0x10040)
	req, _ := h.translationReq()
	h.answer(req, false)
	h.tick()

	// The Holds name the page by different bytes and selectors.
	holdA := h.controlAt(memcontrolprotocol.CmdHold, 0x10fff)
	h.tick()
	holdB := h.controlRange(memcontrolprotocol.CmdHold, 0x10800, 0x10801)
	h.tick()
	holdC := h.controlAt(memcontrolprotocol.CmdHold, 0x10000)
	h.tick()
	h.noAck("every Hold waits for the outstanding access")

	h.complete()
	h.tick()
	for _, id := range []uint64{holdA, holdB, holdC} {
		require.True(t, h.mustAck(id).Success)
	}
}

// Hold A waits for a translation. Hold B is ready and shares a page with A.
// Hold C is ready and shares a page with B, not with A, so it waits for a
// Hold that itself only waits for another Hold; Hold E does the same behind
// C. Hold D shares no page with them.
func TestHoldWaitsForEveryEarlierPendingHoldOfItsPages(t *testing.T) {
	for _, ctrlBuf := range []int{1, 8} {
		h := newHoldHarnessWithControlBuf(t, ctrlBuf)

		h.accessAll(0x10040)
		pending, sent := h.translationReq()
		require.True(t, sent)

		// Pages 0x10000 and 0x11000.
		holdA := h.controlRange(memcontrolprotocol.CmdHold, 0x10000, 0x12000)
		h.tick()
		// Pages 0x11000 and 0x12000.
		holdB := h.controlReq(listHold(1, 0x11800, 0x12000))
		h.tick()
		// Pages 0x12000 and 0x13000.
		holdC := h.controlRange(memcontrolprotocol.CmdHold, 0x12fff, 0x13001)
		h.tick()
		h.noAck("Holds B and C wait for the Holds before them")

		holdD := h.controlAt(memcontrolprotocol.CmdHold, 0x14000)
		require.True(t, h.mustAck(holdD).Success)

		holdE := h.controlAt(memcontrolprotocol.CmdHold, 0x13000)
		h.tick()
		h.tick()
		h.noAck("Hold E waits for Hold C")
		require.Equal(t, []uint64{holdA, holdB, holdC, holdE}, h.pendingHoldIDs())
		slots := h.at.State.PendingHolds

		// The translation returns and its access is held: Hold A is ready,
		// and the Holds behind it follow in arrival order, as far as the
		// Control port has room.
		h.answer(pending, false)
		h.tick()
		h.tick()
		acked := h.holdAcks()
		require.Len(t, acked, min(ctrlBuf, 4))
		for range 3 {
			h.tick()
			acked = append(acked, h.holdAcks()...)
		}
		require.Equal(t, []uint64{holdA, holdB, holdC, holdE}, acked)
		require.Empty(t, h.at.State.PendingHolds)
		require.Equal(t, make([]pendingHoldState, 4), slots,
			"the state does not keep the page lists of the answered Holds")
	}
}

// The earlier Hold covers page 0x10000 of process 1, whose translation is
// in flight, and waits. The later Hold does not cover the page and is ready:
// it is acknowledged first unless the two Holds share a page. Pages are
// compared, not bytes, and a range is never expanded into its pages.
func TestHoldSelectorsShareAPageByPageAndProcess(t *testing.T) {
	const top = ^uint64(0)

	cases := []struct {
		name    string
		earlier memcontrolprotocol.Req
		later   memcontrolprotocol.Req
		share   bool
	}{
		{"lists of other pages",
			listHold(1, 0x10000), listHold(1, 0x20000), false},
		{"lists that name other bytes of one page",
			listHold(1, 0x10000, 0x20000), listHold(1, 0x30000, 0x20fff), true},
		{"list of no page",
			listHold(1, 0x10000), listHold(1), false},
		{"list of the same page of another process",
			listHold(1, 0x10000), listHold(2, 0x10000), false},

		{"list after a range that ends on the boundary of its page",
			rangeHold(1, 0x10000, 0x20000), listHold(1, 0x20000), false},
		{"list after a range that enters its page by one byte",
			rangeHold(1, 0x10000, 0x20001), listHold(1, 0x20fff), true},
		{"list after a range of the address space",
			rangeHold(1, 0, top), listHold(1, 0x20000), true},
		{"list after a range of another process",
			rangeHold(1, 0x10000, 0x30000), listHold(2, 0x20000), false},

		{"range after a list of the page before it",
			listHold(1, 0x10fff), rangeHold(1, 0x11000, 0x12000), false},
		{"range of one byte after a list that names another byte of its page",
			listHold(1, 0x10abc, 0x30abc), rangeHold(1, 0x30fff, 0x31000), true},
		{"range to the top of the address space after a list below it",
			listHold(1, 0x10000), rangeHold(1, 0x11000, top), false},
		{"range to the top of the address space after a list of its last page",
			listHold(1, 0x10000, top), rangeHold(1, 0x11000, top), true},
		{"range of the address space of another process after a list",
			listHold(1, 0x10000), rangeHold(2, 0, top), false},

		{"adjacent ranges",
			rangeHold(1, 0x10000, 0x11000), rangeHold(1, 0x11000, 0x12000), false},
		{"ranges with bytes apart in one page",
			rangeHold(1, 0x10000, 0x11001), rangeHold(1, 0x11fff, 0x13000), true},
		{"ranges of the address space",
			rangeHold(1, 0, top), rangeHold(1, 0x11000, top), true},
		{"ranges of the address space of two processes",
			rangeHold(1, 0, top), rangeHold(2, 0, top), false},
	}

	for _, tc := range cases {
		h := newHoldHarness(t)

		h.accessAll(0x10040)
		pending, sent := h.translationReq()
		require.True(t, sent, tc.name)

		earlier := h.controlReq(tc.earlier)
		h.tick()
		h.noAck(tc.name)

		// Only the control middleware accepts the later Hold, so that both
		// Holds can be compared before any is acknowledged.
		later := h.controlReq(tc.later)
		ctrl := h.at.Middlewares.Ctrl
		ctrl.handleIncoming()
		holds := h.at.State.PendingHolds
		require.Len(t, holds, 2, tc.name)
		require.False(t, ctrl.inFlight(holds[1]), tc.name)
		require.Equal(t, tc.share, holds[0].overlaps(holds[1]), tc.name)
		require.Equal(t, tc.share, holds[1].overlaps(holds[0]), tc.name)

		// The later Hold is acknowledged at once unless it waits for the
		// earlier one, which the returning translation makes ready.
		h.tick()
		acked := h.holdAcks()
		want := []uint64{earlier, later}
		if !tc.share {
			require.Equal(t, []uint64{later}, acked, tc.name)
			want = []uint64{later, earlier}
		} else {
			require.Empty(t, acked, tc.name)
		}

		h.answer(pending, false)
		for range 4 {
			h.tick()
			acked = append(acked, h.holdAcks()...)
		}
		require.Equal(t, want, acked, tc.name)
		require.Empty(t, h.at.State.PendingHolds, tc.name)
	}
}

func TestHoldAcksWaitForRoomOnTheControlPort(t *testing.T) {
	h := newHoldHarness(t)

	h.accessAll(0x10040)
	pending, sent := h.translationReq()
	require.True(t, sent)

	holdA := h.controlAt(memcontrolprotocol.CmdHold, 0x10000)
	h.tick()

	// The ack of Hold B stays on the Control port, which buffers one
	// response.
	holdB := h.controlAt(memcontrolprotocol.CmdHold, 0x20000)
	h.tick()

	// Hold C is accepted and holds its page, but its ack waits for room.
	holdC := h.controlAt(memcontrolprotocol.CmdHold, 0x30000)
	h.tick()
	h.tick()
	require.Equal(t, []uint64{holdA, holdC}, h.pendingHoldIDs())
	h.accessAll(0x30040)
	require.Empty(t, h.translated())

	// Hold A becomes ready while the port is full.
	h.answer(pending, false)
	h.tick()
	h.tick()
	require.Equal(t, []uint64{holdA, holdC}, h.pendingHoldIDs())

	// With room on the port, the ready Holds are acknowledged in arrival
	// order.
	require.Equal(t, []uint64{holdB}, h.holdAcks())
	require.True(t, h.mustAck(holdA).Success)
	require.True(t, h.mustAck(holdC).Success)
	require.Empty(t, h.at.State.PendingHolds)
}

func TestResetCancelsThePendingHoldsInArrivalOrder(t *testing.T) {
	h := newHoldHarness(t)

	h.accessAll(0x10040)

	// Hold A waits for the translation, Hold B is acknowledged before it,
	// and Hold C waits for Hold A.
	holdA := h.controlRange(memcontrolprotocol.CmdHold, 0x10000, 0x12000)
	h.tick()
	holdB := h.controlAt(memcontrolprotocol.CmdHold, 0x20000)
	require.True(t, h.mustAck(holdB).Success)
	holdC := h.controlAt(memcontrolprotocol.CmdHold, 0x11000)
	h.tick()
	h.noAck("Hold C waits for Hold A")

	// The Reset ack fills the Control port, so the canceled Holds are still
	// pending when Hold D arrives.
	resetID := h.controlReq(memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset})
	h.tick()
	holdD := h.controlAt(memcontrolprotocol.CmdHold, 0x30000)
	h.tick()
	require.Equal(t, []uint64{holdA, holdC, holdD}, h.pendingHoldIDs())

	rsp, rspTo, ok := h.ack()
	require.True(t, ok)
	require.Equal(t, resetID, rspTo)
	require.Equal(t, memcontrolprotocol.Rsp{
		Command: memcontrolprotocol.CmdReset, Success: true}, rsp)

	canceled := memcontrolprotocol.Rsp{Command: memcontrolprotocol.CmdHold,
		Error: memcontrolprotocol.ErrCanceledByReset}
	require.Equal(t, canceled, h.mustAck(holdA))
	require.Equal(t, canceled, h.mustAck(holdC))
	require.True(t, h.mustAck(holdD).Success)
	h.tick()
	h.noAck("Hold B is not answered again")
	require.Empty(t, h.at.State.PendingHolds)

	// Only the page of Hold D is held after the Reset.
	require.Empty(t, h.at.State.HeldRanges)
	h.translated()
	h.accessAll(0x10040, 0x20040, 0x30040)
	require.Equal(t, []uint64{0x10000, 0x20000}, h.translated())
	require.Equal(t, []uint64{0x30040}, addresses(h.at.State.HeldReqs))
}

func TestCheckpointKeepsTheOrderOfThePendingHolds(t *testing.T) {
	h := newHoldHarness(t)

	h.accessAll(0x10040)
	pending, sent := h.translationReq()
	require.True(t, sent)

	// Hold B, between Holds A and C, is acknowledged before them.
	holdA := h.controlRange(memcontrolprotocol.CmdHold, 0x10000, 0x12000)
	h.tick()
	holdB := h.controlAt(memcontrolprotocol.CmdHold, 0x20000)
	require.True(t, h.mustAck(holdB).Success)
	holdC := h.controlAt(memcontrolprotocol.CmdHold, 0x11000)
	h.tick()
	require.Equal(t, []uint64{holdA, holdC}, h.pendingHoldIDs())

	var buf bytes.Buffer
	require.NoError(t, h.at.SaveCheckpoint(&buf))

	restored := newHoldHarness(t)
	require.NoError(t, restored.at.LoadCheckpoint(&buf))
	require.Equal(t, h.at.State, restored.at.State)

	// The restored translator still holds the page of Hold B, and acks the
	// other Holds in arrival order once the translation returns.
	restored.accessAll(0x20040)
	require.Empty(t, restored.translated())

	restored.answer(pending, false)
	restored.tick()
	require.True(t, restored.mustAck(holdA).Success)
	require.True(t, restored.mustAck(holdC).Success)
	require.Equal(t, []uint64{0x10040, 0x20040}, addresses(restored.at.State.HeldReqs))
}
