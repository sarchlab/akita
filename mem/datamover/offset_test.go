package datamover

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/dram"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/simplebankedmemory"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

const offsetTestRegion = 8192

type offsetCopy struct {
	src, dst, size uint64
}

type offsetRig struct {
	engine  timing.Engine
	sim     *testsim.Simulation
	mover   *Comp
	agent   messaging.Port
	storage map[datamoverprotocol.Side]*mem.Storage
}

// offsetMemBuilder builds a memory backed by storage and returns its Top port.
type offsetMemBuilder func(sim *testsim.Simulation, storage *mem.Storage, name string) messaging.Port

func buildOffsetIdealMem(sim *testsim.Simulation, storage *mem.Storage, name string) messaging.Port {
	spec := idealmemcontroller.DefaultSpec()
	spec.Latency = 10
	return buildIdealMem(sim, spec, storage, name).Ports.Top
}

func buildOffsetBankedMem(sim *testsim.Simulation, storage *mem.Storage, name string) messaging.Port {
	comp := modeling.MakeBuilder[*simplebankedmemory.Comp](simplebankedmemory.DefaultSpec()).
		WithSimulation(sim).
		WithResources(simplebankedmemory.Resources{Storage: storage}).
		Build(name)
	for _, slot := range []string{"Top", "Control"} {
		comp.BindPort(slot, twowaybuffered.MakeBuilder().
			WithSimulation(sim).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
			Build())
	}
	return comp.Ports.Top
}

func buildOffsetDRAM(sim *testsim.Simulation, storage *mem.Storage, name string) messaging.Port {
	comp := modeling.MakeBuilder[*dram.Comp](dram.DefaultSpec()).
		WithSimulation(sim).
		WithResources(dram.Resources{Storage: storage}).
		Build(name)
	for _, slot := range []string{"Top", "Control"} {
		comp.BindPort(slot, twowaybuffered.MakeBuilder().
			WithSimulation(sim).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
			Build())
	}
	return comp.Ports.Top
}

// forEachOffsetMemory runs body with each memory model as source and destination.
func forEachOffsetMemory(t *testing.T, body func(t *testing.T, buildMem offsetMemBuilder)) {
	t.Helper()
	t.Run("idealmemcontroller", func(t *testing.T) { body(t, buildOffsetIdealMem) })
	t.Run("simplebankedmemory", func(t *testing.T) { body(t, buildOffsetBankedMem) })
}

func newOffsetRig(
	t *testing.T, buildMem offsetMemBuilder, insideWidth, outsideWidth, window uint64, limit int,
) *offsetRig {
	t.Helper()
	return newOffsetRigWithCapacity(t, buildMem, mem.MB, insideWidth, outsideWidth, window, limit)
}

func newOffsetRigWithCapacity(
	t *testing.T, buildMem offsetMemBuilder, capacity, insideWidth, outsideWidth, window uint64, limit int,
) *offsetRig {
	t.Helper()
	engine := timing.NewSerialEngine()
	sim := testsim.New(engine, false)
	agent := newDriverPort(sim, "Agent.Top", 16)

	storage := mem.MakeStorageBuilder().WithSimulation(sim).WithCapacity(capacity)
	insideStorage := storage.Build("InsideStorage")
	outsideStorage := storage.Build("OutsideStorage")
	insideTop := buildMem(sim, insideStorage, "InsideMem")
	outsideTop := buildMem(sim, outsideStorage, "OutsideMem")

	spec := DefaultSpec()
	spec.MaxTransactions = limit
	spec.BufferSize = window
	spec.InsideByteGranularity = insideWidth
	spec.OutsideByteGranularity = outsideWidth
	spec.Inside.Destinations = []messaging.RemotePort{insideTop.AsRemote()}
	spec.Outside.Destinations = []messaging.RemotePort{outsideTop.AsRemote()}
	mover := modeling.MakeBuilder[*Comp](spec).WithSimulation(sim).Build("DataMover")
	ports := makePorts(sim, "DataMover", 16, 64, 64, 16)
	mover.BindPort("Top", ports.Top)
	mover.BindPort("Inside", ports.Inside)
	mover.BindPort("Outside", ports.Outside)
	mover.BindPort("Control", ports.Control)

	conn := direct.NewConnection("Conn", sim, timing.GHz)
	for _, p := range []messaging.Port{
		agent, mover.Ports.Top, mover.Ports.Inside, mover.Ports.Outside,
		insideTop, outsideTop,
	} {
		conn.BindPort(p)
	}
	require.NoError(t, sim.Initialize())

	return &offsetRig{
		engine: engine, sim: sim, mover: mover, agent: agent,
		storage: map[datamoverprotocol.Side]*mem.Storage{
			datamoverprotocol.SideInside:  insideStorage,
			datamoverprotocol.SideOutside: outsideStorage,
		},
	}
}

func offsetPattern(salt byte) []byte {
	data := make([]byte, offsetTestRegion)
	for i := range data {
		data[i] = byte(i*7) ^ salt
	}
	return data
}

func (r *offsetRig) deliver(srcSide, dstSide datamoverprotocol.Side, c offsetCopy) uint64 {
	id := r.sim.NewID()
	r.mover.Ports.Top.Deliver(messaging.Msg{
		ID: id, Src: r.agent.AsRemote(), Dst: r.mover.Ports.Top.AsRemote(),
		Payload: datamoverprotocol.Req{
			SrcAddress: c.src, DstAddress: c.dst, ByteSize: c.size,
			SrcSide: srcSide, DstSide: dstSide,
		},
	})
	return id
}

// runCopies performs copies and checks every destination byte: requested bytes
// hold source data and all others keep their original value.
func (r *offsetRig) runCopies(t *testing.T, srcSide, dstSide datamoverprotocol.Side, copies []offsetCopy) {
	t.Helper()
	src := offsetPattern(0x5a)
	want := offsetPattern(0xa5)
	if srcSide == dstSide {
		want = append([]byte(nil), src...)
	}
	r.storage[dstSide].Write(0, want)
	r.storage[srcSide].Write(0, src)

	ids := make([]uint64, len(copies))
	for i, c := range copies {
		ids[i] = r.deliver(srcSide, dstSide, c)
		copy(want[c.dst:c.dst+c.size], src[c.src:c.src+c.size])
	}
	require.NoError(t, r.engine.Run())

	require.Equal(t, want, r.storage[dstSide].Read(0, offsetTestRegion))
	if srcSide != dstSide {
		require.Equal(t, src, r.storage[srcSide].Read(0, offsetTestRegion))
	}
	for _, id := range ids {
		rsp, ok := r.agent.RetrieveIncoming()
		require.True(t, ok)
		require.Equal(t, id, rsp.RspTo)
	}
	require.Empty(t, r.mover.State.Transactions)
}

func TestUnalignedCopiesPreserveData(t *testing.T) {
	ranges := []struct {
		name string
		copy offsetCopy
	}{
		{"different offsets", offsetCopy{64 + 5, 1024 + 39, 300}},
		{"head only partial", offsetCopy{64 + 20, 1024 + 20, 108}},
		{"tail only partial", offsetCopy{64, 1024, 100}},
		{"head only on destination", offsetCopy{64, 1024 + 9, 119}},
		{"tail only on source", offsetCopy{64 + 9, 1024, 119}},
		{"inside one unit", offsetCopy{64 + 10, 1024 + 30, 12}},
		{"single byte", offsetCopy{64 + 63, 1024, 1}},
		{"crosses one boundary", offsetCopy{64 + 60, 1024 + 62, 8}},
		{"crosses many units", offsetCopy{4 * 25, 2048 + 4*7, 4 * 900}},
		{"adjacent to next unit", offsetCopy{64, 1024 + 63, 2}},
	}
	widths := [][2]uint64{{64, 64}, {64, 256}, {256, 64}, {16, 64}, {48, 64}}
	directions := [][2]datamoverprotocol.Side{
		{datamoverprotocol.SideOutside, datamoverprotocol.SideInside},
		{datamoverprotocol.SideInside, datamoverprotocol.SideOutside},
	}
	forEachOffsetMemory(t, func(t *testing.T, buildMem offsetMemBuilder) {
		t.Helper()
		for _, rg := range ranges {
			for _, w := range widths {
				for _, d := range directions {
					name := fmt.Sprintf("%s/in%d_out%d/%s_to_%s", rg.name, w[0], w[1], d[0], d[1])
					t.Run(name, func(t *testing.T) {
						r := newOffsetRig(t, buildMem, w[0], w[1], 2048, 1)
						r.runCopies(t, d[0], d[1], []offsetCopy{rg.copy})
					})
				}
			}
		}
	})
}

func TestUnalignedSameSideCopy(t *testing.T) {
	forEachOffsetMemory(t, func(t *testing.T, buildMem offsetMemBuilder) {
		t.Helper()
		for _, side := range []datamoverprotocol.Side{datamoverprotocol.SideInside, datamoverprotocol.SideOutside} {
			r := newOffsetRig(t, buildMem, 64, 64, 2048, 1)
			r.runCopies(t, side, side, []offsetCopy{{64 + 5, 4096 + 39, 300}})
		}
	})
}

func TestConcurrentUnalignedCopies(t *testing.T) {
	copies := []offsetCopy{
		{4, 4096 + 8, 200},
		{1024 + 3, 4096 + 512 + 61, 130},
		{2048 + 63, 4096 + 1024 + 1, 1},
		// Shares a destination unit with the first copy's tail.
		{3072 + 7, 4096 + 208, 40},
	}
	forEachOffsetMemory(t, func(t *testing.T, buildMem offsetMemBuilder) {
		t.Helper()
		for _, limit := range []int{1, 2, 4} {
			for _, window := range []uint64{1, 64, 128, 4096} {
				t.Run(fmt.Sprintf("limit%d_window%d", limit, window), func(t *testing.T) {
					r := newOffsetRig(t, buildMem, 64, 64, window, limit)
					r.runCopies(t, datamoverprotocol.SideOutside, datamoverprotocol.SideInside, copies)
				})
			}
		}
	})
}

func TestZeroSizeUnalignedCopyCompletes(t *testing.T) {
	forEachOffsetMemory(t, func(t *testing.T, buildMem offsetMemBuilder) {
		t.Helper()
		r := newOffsetRig(t, buildMem, 64, 64, 2048, 1)
		r.runCopies(t, datamoverprotocol.SideOutside, datamoverprotocol.SideInside,
			[]offsetCopy{{13, 1024 + 27, 0}})
	})
}

// offsetHandRig drives the mover one tick at a time with unconnected ports.
func offsetHandRig(t *testing.T, window uint64, limit int, copies ...offsetCopy) *Comp {
	t.Helper()
	sim := testsim.New(timing.NewSerialEngine(), false)
	spec := DefaultSpec()
	spec.BufferSize = window
	spec.MaxTransactions = limit
	spec.InsideByteGranularity = 64
	spec.OutsideByteGranularity = 64
	spec.Inside.Destinations = []messaging.RemotePort{"InsideMem"}
	spec.Outside.Destinations = []messaging.RemotePort{"OutsideMem"}
	mover := modeling.MakeBuilder[*Comp](spec).WithSimulation(sim).Build("DataMover")
	ports := makePorts(sim, "DataMover", 16, 64, 64, 16)
	mover.BindPort("Top", ports.Top)
	mover.BindPort("Inside", ports.Inside)
	mover.BindPort("Outside", ports.Outside)
	mover.BindPort("Control", ports.Control)
	for _, p := range allPorts(mover) {
		(&ccNoopConn{}).BindPort(p)
	}
	require.NoError(t, sim.Initialize())
	for _, c := range copies {
		mover.Ports.Top.Deliver(messaging.Msg{
			ID: sim.NewID(), Src: "Agent", Dst: mover.Ports.Top.AsRemote(),
			Payload: datamoverprotocol.Req{
				SrcAddress: c.src, DstAddress: c.dst, ByteSize: c.size,
				SrcSide: datamoverprotocol.SideOutside, DstSide: datamoverprotocol.SideInside,
			},
		})
	}
	for range 2*len(copies) + 4 {
		modelingtest.Tick(mover)
	}
	return mover
}

func drainReads(t *testing.T, port messaging.Port) []messaging.Msg {
	t.Helper()
	var reads []messaging.Msg
	for {
		msg, ok := port.RetrieveOutgoing()
		if !ok {
			return reads
		}
		require.IsType(t, memprotocol.ReadReq{}, msg.Payload)
		reads = append(reads, msg)
	}
}

func TestRequestsAreFullAlignedUnits(t *testing.T) {
	mover := offsetHandRig(t, 4096, 1, offsetCopy{64 + 60, 1024 + 62, 8})
	reads := drainReads(t, mover.Ports.Outside)
	require.Len(t, reads, 2)
	for i, msg := range reads {
		read := msg.Payload.(memprotocol.ReadReq)
		require.Equal(t, uint64(64+64*i), read.Address)
		require.Equal(t, uint64(64), read.AccessByteSize)
		require.Equal(t, 12, msg.TrafficBytes)
		data := make([]byte, 64)
		for j := range data {
			data[j] = byte(read.Address) + byte(j)
		}
		mover.Ports.Outside.Deliver(messaging.Msg{
			ID: uint64(1000 + i), Dst: mover.Ports.Outside.AsRemote(), RspTo: msg.ID,
			Payload: memprotocol.DataReadyRsp{Data: data},
		})
	}

	var writes []messaging.Msg
	for range 8 {
		modelingtest.Tick(mover)
		if msg, ok := mover.Ports.Inside.RetrieveOutgoing(); ok {
			writes = append(writes, msg)
		}
	}
	require.Len(t, writes, 2)
	for i, wantBytes := range []int{2, 6} {
		write := writes[i].Payload.(memprotocol.WriteReq)
		require.Equal(t, uint64(1024+64*i), write.Address)
		require.Len(t, write.Data, 64)
		require.Len(t, write.DirtyMask, 64)
		require.Equal(t, 64+12, writes[i].TrafficBytes)
		dirty := 0
		for j, set := range write.DirtyMask {
			if set {
				dirty++
				require.Equal(t, i == 0, j >= 62)
			}
		}
		require.Equal(t, wantBytes, dirty)
	}
}

func TestFullyCoveredUnitHasNoMask(t *testing.T) {
	write := fullUnitWrite(128, 64, make([]byte, 64))
	require.Equal(t, uint64(128), write.Address)
	require.Nil(t, write.DirtyMask)
}

func TestWindowCountsOverFetchedUnits(t *testing.T) {
	// Eight requested bytes occupy two source units, filling a 128-byte window
	// so the second copy cannot start its read.
	mover := offsetHandRig(t, 128, 2,
		offsetCopy{64 + 60, 1024, 8}, offsetCopy{4096, 2048, 64})
	require.Len(t, mover.State.Transactions, 2)
	require.Len(t, drainReads(t, mover.Ports.Outside), 2)
}

func TestSmallWindowFetchesOnlyNextWriteSpan(t *testing.T) {
	// The first destination unit needs two source units, so a 64-byte window
	// may be exceeded by one unit but no further.
	mover := offsetHandRig(t, 64, 1, offsetCopy{64 + 32, 1024, 256})
	require.Len(t, drainReads(t, mover.Ports.Outside), 2)
}

func TestUnalignedCopiesThroughDRAM(t *testing.T) {
	for _, c := range []offsetCopy{
		{64, 1024, 100},           // short tail
		{64 + 20, 1024 + 20, 108}, // unaligned head
		{64 + 5, 1024 + 39, 300},  // both, with different offsets
		{64 + 10, 1024 + 30, 12},  // inside one unit
	} {
		for _, d := range [][2]datamoverprotocol.Side{
			{datamoverprotocol.SideOutside, datamoverprotocol.SideInside},
			{datamoverprotocol.SideInside, datamoverprotocol.SideOutside},
		} {
			t.Run(fmt.Sprintf("%d_%d_%d_%s", c.src, c.dst, c.size, d[0]), func(t *testing.T) {
				r := newOffsetRig(t, buildOffsetDRAM, 64, 64, 2048, 1)
				r.runCopies(t, d[0], d[1], []offsetCopy{c})
			})
		}
	}
}

func TestZeroWindowWriteSpansManySourceUnits(t *testing.T) {
	// Each 64-byte destination write needs five 16-byte source units.
	forEachOffsetMemory(t, func(t *testing.T, buildMem offsetMemBuilder) {
		t.Helper()
		r := newOffsetRig(t, buildMem, 64, 16, 0, 1)
		r.runCopies(t, datamoverprotocol.SideOutside, datamoverprotocol.SideInside,
			[]offsetCopy{{16 + 5, 1024, 128}})
	})
}

func TestFullUnitsMustFitInStorage(t *testing.T) {
	const beyondCapacity = "accessing physical address beyond the storage capacity"
	out, in := datamoverprotocol.SideOutside, datamoverprotocol.SideInside
	forEachOffsetMemory(t, func(t *testing.T, buildMem offsetMemBuilder) {
		t.Helper()
		// Bytes 940-999 fit in a 1000-byte storage, but unit 960-1023 does not.
		for name, c := range map[string]offsetCopy{
			"source":      {940, 128, 60},
			"destination": {128, 940, 60},
		} {
			t.Run(name, func(t *testing.T) {
				r := newOffsetRigWithCapacity(t, buildMem, 1000, 64, 64, 2048, 1)
				r.deliver(out, in, c)
				require.ErrorContains(t, r.engine.Run(), beyondCapacity)
			})
		}
		t.Run("units ending at capacity", func(t *testing.T) {
			r := newOffsetRigWithCapacity(t, buildMem, 1024, 64, 64, 2048, 1)
			src := offsetPattern(0x5a)[:1024]
			r.storage[out].Write(0, src)
			r.deliver(out, in, offsetCopy{940, 950, 60})
			require.NoError(t, r.engine.Run())
			require.Equal(t, src[940:1000], r.storage[in].Read(950, 60))
			require.Equal(t, make([]byte, 14), r.storage[in].Read(1010, 14))
		})
	})
}

func TestRoutingBoundariesMustAlign(t *testing.T) {
	two := []messaging.RemotePort{"A", "B"}
	one := []messaging.RemotePort{"A"}
	cases := []struct {
		name   string
		mapper mem.AddressMapper
		valid  bool
	}{
		{"interleaving multiple of unit", mem.AddressMapper{Destinations: two, InterleavingSize: 128}, true},
		{"interleaving equals unit", mem.AddressMapper{Destinations: two, InterleavingSize: 64}, true},
		{"GPU memory interleaving", mem.AddressMapper{Destinations: two, InterleavingSize: 4 * mem.GB}, true},
		{"aligned range", mem.AddressMapper{Destinations: one, RangeLow: 64, RangeHigh: 128, Fallback: "F"}, true},
		{"unused interleaving", mem.AddressMapper{Destinations: one, InterleavingSize: 100}, true},
		{"interleaving inside unit", mem.AddressMapper{Destinations: two, InterleavingSize: 32}, false},
		{"interleaving not multiple", mem.AddressMapper{Destinations: two, InterleavingSize: 96}, false},
		{"range low inside unit", mem.AddressMapper{Destinations: one, RangeLow: 80, RangeHigh: 128, Fallback: "F"}, false},
		{"range high inside unit", mem.AddressMapper{Destinations: one, RangeLow: 64, RangeHigh: 96, Fallback: "F"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			valid := mem.AddressMapper{Destinations: one}
			for _, spec := range []Spec{
				{MaxTransactions: 1, InsideByteGranularity: 64, OutsideByteGranularity: 16,
					Inside: c.mapper, Outside: valid},
				{MaxTransactions: 1, InsideByteGranularity: 16, OutsideByteGranularity: 64,
					Inside: valid, Outside: c.mapper},
			} {
				err := validateSpec(spec)
				if c.valid {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "routing boundaries must align")
				}
			}
		})
	}
}
