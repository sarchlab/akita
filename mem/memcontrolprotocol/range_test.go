package memcontrolprotocol_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

func TestAddressingError(t *testing.T) {
	type AR = memcontrolprotocol.AddressRange
	const max = ^uint64(0)

	cases := []struct {
		name string
		req  memcontrolprotocol.Req
		want string
	}{
		{"no addressing", memcontrolprotocol.Req{}, ""},
		{"addresses only", memcontrolprotocol.Req{Addresses: []uint64{0, 64}}, ""},
		{"one byte", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 5, End: 6}}, ""},
		{"range from zero", memcontrolprotocol.Req{HasRange: true, Range: AR{End: 1}}, ""},
		{"largest range", memcontrolprotocol.Req{HasRange: true, Range: AR{End: max}}, ""},
		{"range with an empty address list",
			memcontrolprotocol.Req{HasRange: true, Range: AR{End: 64}, Addresses: []uint64{}}, ""},

		{"empty zero range", memcontrolprotocol.Req{HasRange: true}, memcontrolprotocol.ErrInvalidRange},
		{"empty range", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 64, End: 64}},
			memcontrolprotocol.ErrInvalidRange},
		{"empty range at the top", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: max, End: max}},
			memcontrolprotocol.ErrInvalidRange},
		{"reversed range", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 65, End: 64}},
			memcontrolprotocol.ErrInvalidRange},
		{"wrapped range", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: max, End: 0}},
			memcontrolprotocol.ErrInvalidRange},
		{"range without HasRange", memcontrolprotocol.Req{Range: AR{End: 64}},
			memcontrolprotocol.ErrInvalidRange},
		{"start without HasRange", memcontrolprotocol.Req{Range: AR{Start: 64}},
			memcontrolprotocol.ErrInvalidRange},
		{"range without HasRange, with addresses",
			memcontrolprotocol.Req{Range: AR{End: 64}, Addresses: []uint64{0}},
			memcontrolprotocol.ErrInvalidRange},
		{"range and addresses",
			memcontrolprotocol.Req{HasRange: true, Range: AR{End: 64}, Addresses: []uint64{0}},
			memcontrolprotocol.ErrAmbiguousAddressing},
		{"invalid range and addresses",
			memcontrolprotocol.Req{HasRange: true, Addresses: []uint64{0}},
			memcontrolprotocol.ErrInvalidRange},
	}

	for _, tc := range cases {
		if got := tc.req.AddressingError(); got != tc.want {
			t.Errorf("%s: AddressingError() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestHasAddressFilter(t *testing.T) {
	type AR = memcontrolprotocol.AddressRange

	if (memcontrolprotocol.Req{}).HasAddressFilter() ||
		(memcontrolprotocol.Req{Addresses: []uint64{}}).HasAddressFilter() {
		t.Error("a request with no addresses has no address filter")
	}
	if !(memcontrolprotocol.Req{Addresses: []uint64{0}}).HasAddressFilter() ||
		!(memcontrolprotocol.Req{HasRange: true, Range: AR{End: 1}}).HasAddressFilter() {
		t.Error("addresses and a range are address filters")
	}
}

func TestAddressRangeBounds(t *testing.T) {
	const max = ^uint64(0)

	cases := []struct {
		start, end  uint64
		log2        uint64
		first, last uint64
	}{
		{0, 1, 6, 0, 0},
		{0, 64, 6, 0, 0},
		{0, 65, 6, 0, 64},
		{63, 64, 6, 0, 0},
		{63, 65, 6, 0, 64},
		{64, 128, 6, 64, 64},
		{0x1fff, 0x2001, 12, 0x1000, 0x2000},
		{0, max, 6, 0, max &^ 63},
		{max - 1, max, 12, max &^ 0xfff, max &^ 0xfff},
		{max - 0x1000, max, 12, max&^0xfff - 0x1000, max &^ 0xfff},
		{5, 6, 0, 5, 5},
	}

	for _, tc := range cases {
		r := memcontrolprotocol.AddressRange{Start: tc.start, End: tc.end}
		first, last := r.Bounds(tc.log2)
		if first != tc.first || last != tc.last {
			t.Errorf("[%#x, %#x).Bounds(%d) = %#x, %#x, want %#x, %#x",
				tc.start, tc.end, tc.log2, first, last, tc.first, tc.last)
		}
	}
}

// A request in a message that a checkpoint saves keeps its addressing.
func TestReqRoundTripsInACheckpointedMessage(t *testing.T) {
	reqs := []memcontrolprotocol.Req{
		{Command: memcontrolprotocol.CmdFlush, PID: 3, HasRange: true,
			Range: memcontrolprotocol.AddressRange{Start: 1, End: ^uint64(0)}},
		{Command: memcontrolprotocol.CmdInvalidate, Addresses: []uint64{64, ^uint64(0)}},
		{Command: memcontrolprotocol.CmdHold, PID: 1, HasRange: true,
			Range: memcontrolprotocol.AddressRange{Start: 0x1000, End: 0x1001}},
		{Command: memcontrolprotocol.CmdDrain},
	}

	for _, req := range reqs {
		msg := messaging.Msg{Payload: req, ID: 7, Src: "A", Dst: "B"}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}

		var got messaging.Msg
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(msg, got) {
			t.Errorf("message changed in the round trip:\n got %+v\nwant %+v", got, msg)
		}
	}
}

// controlReq issues a control request and ticks until its response.
func (h *cacheOverDRAM) controlReq(t *testing.T, req memcontrolprotocol.Req) memcontrolprotocol.Rsp {
	t.Helper()

	msg := messaging.Msg{Payload: req, ID: h.cache.NewID(), Src: h.agent, Dst: h.ctrl.AsRemote()}
	h.ctrl.Deliver(msg)

	for range 4096 {
		h.tick()
		if out, ok := h.ctrl.RetrieveOutgoing(); ok && out.RspTo == msg.ID {
			return out.Payload.(memcontrolprotocol.Rsp)
		}
	}
	t.Fatalf("control request %+v never acked", req)
	return memcontrolprotocol.Rsp{}
}

// A range Flush of a running cache persists the lines of the range, and only
// those; a range Invalidate then makes the cache fetch them again.
func TestRangeFlushAndInvalidate_RunningCacheOverMemory(t *testing.T) {
	h := buildCacheOverDRAM(t)

	const n = 8
	want := map[uint64][]byte{}
	for i := range n {
		addr := uint64(i) * cpBlockSize
		want[addr] = []byte{byte(i + 1), 0xAB, 0xCD, byte(0xF0 + i)}
		h.write(t, addr, want[addr])
	}

	// The range starts in the last byte of line 2 and ends after the first
	// byte of line 4.
	r := memcontrolprotocol.AddressRange{Start: 3*cpBlockSize - 1, End: 4*cpBlockSize + 1}
	flushed := func(addr uint64) bool { return addr >= 2*cpBlockSize && addr <= 4*cpBlockSize }

	rsp := h.controlReq(t, memcontrolprotocol.Req{
		Command: memcontrolprotocol.CmdFlush, HasRange: true, Range: r})
	if !rsp.Success {
		t.Fatalf("range Flush failed: %q", rsp.Error)
	}
	for addr, data := range want {
		got := h.dramStorage.Read(addr, uint64(len(data)))
		if flushed(addr) != bytes.Equal(got, data) {
			t.Errorf("after the range Flush, backing memory[%#x] = %v (in range: %v)",
				addr, got, flushed(addr))
		}
	}

	rsp = h.controlReq(t, memcontrolprotocol.Req{
		Command: memcontrolprotocol.CmdInvalidate, HasRange: true, Range: r})
	if !rsp.Success {
		t.Fatalf("range Invalidate failed: %q", rsp.Error)
	}
	kept := 0
	for si := range h.cache.State.DirectoryState.Sets {
		for _, b := range h.cache.State.DirectoryState.Sets[si].Blocks {
			if b.IsValid && flushed(b.Tag) {
				t.Errorf("the range Invalidate left line %#x valid", b.Tag)
			}
			if b.IsValid && b.IsDirty {
				kept++
			}
		}
	}
	if kept != n-3 {
		t.Errorf("%d dirty lines outside the range remain, want %d", kept, n-3)
	}

	// Every line still reads its data: from the cache, or again from the
	// memory that the Flush wrote.
	for addr, data := range want {
		if got := h.read(t, addr, uint64(len(data))); !bytes.Equal(got, data) {
			t.Errorf("read[%#x] = %v, want %v", addr, got, data)
		}
	}
}
