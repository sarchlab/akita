package tlb

import (
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// rangeTestTLB is a paused TLB of 4 KB pages that holds pages.
func rangeTestTLB(pages []vm.Page) (*testsim.Simulation, *Comp) {
	sim := testsim.New(timing.NewSerialEngine(), false)

	spec := DefaultSpec()
	spec.NumSets = 2
	spec.NumWays = 8
	spec.Log2PageSize = 12
	spec.Bottom.Destinations = []messaging.RemotePort{"RemotePort"}

	comp := modeling.MakeBuilder[*Comp](spec).WithSimulation(sim).Build("TLB")
	ports := defaultPorts(sim, "TLB")
	comp.BindPort("Top", ports.Top)
	comp.BindPort("Bottom", ports.Bottom)
	comp.BindPort("Control", ports.Control)
	plugNoopConn(comp)
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	comp.State.TLBState = tlbStatePause
	for i, page := range pages {
		set := &comp.State.Sets[page.VAddr>>spec.Log2PageSize%uint64(spec.NumSets)]
		setUpdate(set, i, page)
		setVisit(set, i)
	}

	return sim, comp
}

// control sends req to the TLB and returns its response.
func control(
	t *testing.T,
	sim *testsim.Simulation,
	comp *Comp,
	req memcontrolprotocol.Req,
) memcontrolprotocol.Rsp {
	t.Helper()

	msg := messaging.Msg{Payload: req, ID: sim.NewID(),
		Src: "Agent", Dst: comp.Ports.Control.AsRemote()}
	comp.Ports.Control.Deliver(msg)
	require.True(t, comp.Middlewares.Ctrl.handleIncomingCommands())

	out, ok := comp.Ports.Control.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, msg.ID, out.RspTo)
	require.Equal(t, messaging.RemotePort("Agent"), out.Dst)

	return out.Payload.(memcontrolprotocol.Rsp)
}

func TestRangeInvalidateDropsEveryIntersectingPage(t *testing.T) {
	const top = ^uint64(0) &^ 0xfff
	pages := []vm.Page{
		{PID: 1, VAddr: 0, Valid: true},
		{PID: 1, VAddr: 0x1000, Valid: true},
		{PID: 1, VAddr: 0x2000, Valid: true},
		{PID: 2, VAddr: 0x2000, Valid: true},
		{PID: 1, VAddr: 0x3000, Valid: true},
		{PID: 1, VAddr: top, Valid: true},
	}

	cases := []struct {
		name       string
		pid        vm.PID
		start, end uint64
		want       []int // indices of the dropped pages
	}{
		{"unaligned inside two pages", 1, 0x1fff, 0x2001, []int{1, 2}},
		{"end on a page boundary excludes that page", 1, 0x1000, 0x2000, []int{1}},
		{"one byte, every PID", 0, 0x2abc, 0x2abd, []int{2, 3}},
		{"another PID", 2, 0, 0x4000, []int{3}},
		{"no cached page", 0, 0x4000, 0x9000, nil},
		{"nearly the whole address space", 0, 1, ^uint64(0), []int{0, 1, 2, 3, 4, 5}},
		{"from the middle to the top", 1, 0x2fff, ^uint64(0), []int{2, 4, 5}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sim, comp := rangeTestTLB(pages)

			rsp := control(t, sim, comp, memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdInvalidate, PID: tc.pid, HasRange: true,
				Range: memcontrolprotocol.AddressRange{Start: tc.start, End: tc.end},
			})
			require.Equal(t,
				memcontrolprotocol.Rsp{Command: memcontrolprotocol.CmdInvalidate, Success: true}, rsp)

			for i, page := range pages {
				set := &comp.State.Sets[page.VAddr>>12%2]
				_, got, found := setLookup(set, page.PID, page.VAddr)
				require.Equal(t, !slices.Contains(tc.want, i), found && got.Valid,
					"page %d (PID %d, %#x)", i, page.PID, page.VAddr)
			}
		})
	}
}

func TestInvalidateRejectsInvalidRangeAddressing(t *testing.T) {
	type AR = memcontrolprotocol.AddressRange
	cases := []struct {
		name string
		req  memcontrolprotocol.Req
		want string
	}{
		{"empty zero range", memcontrolprotocol.Req{HasRange: true}, memcontrolprotocol.ErrInvalidRange},
		{"empty range", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 0x1000, End: 0x1000}},
			memcontrolprotocol.ErrInvalidRange},
		{"reversed range", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 0x2000, End: 0x1000}},
			memcontrolprotocol.ErrInvalidRange},
		{"range without HasRange", memcontrolprotocol.Req{Range: AR{Start: 0, End: 0x1000}},
			memcontrolprotocol.ErrInvalidRange},
		{"range and addresses", memcontrolprotocol.Req{HasRange: true, Range: AR{Start: 0, End: 0x1000},
			Addresses: []uint64{0}}, memcontrolprotocol.ErrAmbiguousAddressing},
	}

	for _, tc := range cases {
		for _, st := range []string{tlbStatePause, tlbStateEnable} {
			sim, comp := rangeTestTLB([]vm.Page{{PID: 1, VAddr: 0, Valid: true}})
			comp.State.TLBState = st

			tc.req.Command = memcontrolprotocol.CmdInvalidate
			rsp := control(t, sim, comp, tc.req)
			require.Equal(t, memcontrolprotocol.Rsp{
				Command: memcontrolprotocol.CmdInvalidate, Error: tc.want}, rsp, tc.name)

			_, page, found := setLookup(&comp.State.Sets[0], 1, 0)
			require.True(t, found && page.Valid, "%s: a rejected command must change nothing", tc.name)
			require.Equal(t, st, comp.State.TLBState)
		}
	}
}
