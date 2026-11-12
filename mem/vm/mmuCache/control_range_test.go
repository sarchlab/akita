package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// rangeTestWidth is the segment width of a two-level cache of 4 KB pages.
const rangeTestWidth = 26

// rangeTestPage is a page whose walk is cached.
type rangeTestPage struct {
	pid   vm.PID
	vAddr uint64
}

// rangeTestPages share and differ in their segments: pages 1 and 5 share
// the level-0 segment, pages 6 to 8 cross a level-1 boundary, where the
// level-0 segment wraps around.
var rangeTestPages = []rangeTestPage{
	{1, 0x5000},
	{1, 0x6000},
	{1, 0x7000},
	{2, 0x7000},
	{1, 0x9000},
	{1, 3<<(rangeTestWidth+12) | 0x6000},
	{1, 1<<(rangeTestWidth+12) - 0x1000},
	{1, 1 << (rangeTestWidth + 12)},
	{1, 1<<(rangeTestWidth+12) + 0x1000},
	{1, ^uint64(0) &^ 0xfff},
}

// rangeTestCache is a paused cache that holds the segments of
// rangeTestPages at both levels.
func rangeTestCache() (*testsim.Simulation, *Comp) {
	sim := testsim.New(timing.NewSerialEngine(), false)

	spec := DefaultSpec()
	spec.NumBlocks = 32
	spec.NumLevels = 2
	spec.PageSize = 4096
	spec.Log2PageSize = 12
	spec.LowModule = "LowModule"
	spec.UpModule = "UpModule"

	comp := modeling.MakeBuilder[*Comp](spec).WithSimulation(sim).Build("MMUCache")
	ports := defaultPorts(sim, "MMUCache")
	comp.BindPort("Top", ports.Top)
	comp.BindPort("Bottom", ports.Bottom)
	comp.BindPort("Control", ports.Control)
	for _, p := range []messaging.Port{ports.Top, ports.Bottom, ports.Control} {
		(&noopConn{}).BindPort(p)
	}
	if err := sim.Initialize(); err != nil {
		panic(err)
	}

	comp.State.CurrentState = mmuCacheStatePause
	for level := range comp.State.Table {
		set := &comp.State.Table[level]
		way := 0
		for _, page := range rangeTestPages {
			seg := segForLevelSpec(spec, level, page.vAddr)
			if _, found := setLookup(set, page.pid, seg); found {
				continue
			}
			setUpdate(set, way, page.pid, seg)
			setVisit(set, way)
			way++
		}
	}

	return sim, comp
}

// control sends req to the cache and returns its response.
func control(
	t *testing.T,
	sim *testsim.Simulation,
	comp *Comp,
	req memcontrolprotocol.Req,
) memcontrolprotocol.Rsp {
	t.Helper()

	msg := messaging.Msg{Payload: req, ID: sim.NewID(),
		Src: "Requester", Dst: comp.Ports.Control.AsRemote()}
	comp.Ports.Control.Deliver(msg)
	require.True(t, comp.Middlewares.Ctrl.handleIncomingCommands())

	out, ok := comp.Ports.Control.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, msg.ID, out.RspTo)
	require.Equal(t, messaging.RemotePort("Requester"), out.Dst)

	return out.Payload.(memcontrolprotocol.Rsp)
}

// cachedSegments reports, by level and by page, whether the segment of the
// page is cached.
func cachedSegments(comp *Comp) [][]bool {
	cached := make([][]bool, len(comp.State.Table))
	for level := range comp.State.Table {
		for _, page := range rangeTestPages {
			_, found := setLookup(&comp.State.Table[level], page.pid,
				segForLevelSpec(comp.Spec, level, page.vAddr))
			cached[level] = append(cached[level], found)
		}
	}
	return cached
}

// A range drops what the list of its pages drops. The expected state comes
// from the address-list path, which the earlier tests cover.
func TestRangeInvalidateMatchesTheListOfItsPages(t *testing.T) {
	const boundary = uint64(1) << (rangeTestWidth + 12)
	cases := []struct {
		name       string
		pid        vm.PID
		start, end uint64
	}{
		{"unaligned inside two pages", 1, 0x6fff, 0x7001},
		{"end on a page boundary excludes that page", 1, 0x6000, 0x7000},
		{"one byte, every PID", 0, 0x7abc, 0x7abd},
		{"another PID", 2, 0x5000, 0xa000},
		{"no cached page", 0, 0x20000, 0x30000},
		{"across a level-1 boundary", 1, boundary - 1, boundary + 1},
		{"up to a level-1 boundary", 1, boundary - 0x1000, boundary},
		{"from a level-1 boundary", 0, boundary, boundary + 0x2000},
		{"the last page", 0, ^uint64(0) - 1, ^uint64(0)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var pages []uint64
			for page := tc.start &^ 0xfff; ; page += 0x1000 {
				pages = append(pages, page)
				if page == (tc.end-1)&^0xfff {
					break
				}
			}

			sim, listed := rangeTestCache()
			require.True(t, control(t, sim, listed, memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdInvalidate, PID: tc.pid, Addresses: pages,
			}).Success)

			sim, ranged := rangeTestCache()
			before := cachedSegments(ranged)
			rsp := control(t, sim, ranged, memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdInvalidate, PID: tc.pid, HasRange: true,
				Range: memcontrolprotocol.AddressRange{Start: tc.start, End: tc.end},
			})
			require.Equal(t,
				memcontrolprotocol.Rsp{Command: memcontrolprotocol.CmdInvalidate, Success: true}, rsp)

			require.Equal(t, cachedSegments(listed), cachedSegments(ranged))
			if tc.name != "no cached page" {
				require.NotEqual(t, before, cachedSegments(ranged), "the range must drop a segment")
			}
		})
	}
}

// A range of nearly every page is not expanded into its pages: it drops
// every segment of the process in a walk over the cached blocks.
func TestRangeInvalidateOfTheAddressSpaceIsBounded(t *testing.T) {
	sim, comp := rangeTestCache()

	rsp := control(t, sim, comp, memcontrolprotocol.Req{
		Command: memcontrolprotocol.CmdInvalidate, PID: 1, HasRange: true,
		Range: memcontrolprotocol.AddressRange{Start: 1, End: ^uint64(0)},
	})
	require.True(t, rsp.Success, rsp.Error)

	for level, cached := range cachedSegments(comp) {
		for i, found := range cached {
			require.Equal(t, rangeTestPages[i].pid == 2, found,
				"level %d, page %d", level, i)
		}
	}

	// A range of more than a level-0 cycle of pages drops every level-0
	// segment, and only the level-1 segments of its pages.
	sim, comp = rangeTestCache()
	rsp = control(t, sim, comp, memcontrolprotocol.Req{
		Command: memcontrolprotocol.CmdInvalidate, HasRange: true,
		Range: memcontrolprotocol.AddressRange{Start: 0x8000, End: 2 << (rangeTestWidth + 12)},
	})
	require.True(t, rsp.Success, rsp.Error)

	cached := cachedSegments(comp)
	for i := range rangeTestPages {
		require.False(t, cached[0][i], "level 0, page %d", i)
		seg := segForLevelSpec(comp.Spec, 1, rangeTestPages[i].vAddr)
		require.Equal(t, seg > 1, cached[1][i], "level 1, page %d", i)
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
		for _, st := range []string{mmuCacheStatePause, mmuCacheStateEnable} {
			sim, comp := rangeTestCache()
			comp.State.CurrentState = st
			before := cachedSegments(comp)

			tc.req.Command = memcontrolprotocol.CmdInvalidate
			rsp := control(t, sim, comp, tc.req)
			require.Equal(t, memcontrolprotocol.Rsp{
				Command: memcontrolprotocol.CmdInvalidate, Error: tc.want}, rsp, tc.name)
			require.Equal(t, before, cachedSegments(comp), "%s: a rejected command must change nothing", tc.name)
			require.Equal(t, st, comp.State.CurrentState)
		}
	}
}
