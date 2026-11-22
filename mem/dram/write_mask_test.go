package dram

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

const maskTestBackground = byte(0xAA)

func (h *p0Harness) maskedWrite(addr uint64, data []byte, mask []bool) messaging.Msg {
	msg := h.write(addr, data)
	msg.Payload = memprotocol.WriteReq{Address: addr, Data: data, DirtyMask: mask}
	return msg
}

// readBack reads size bytes through the component's normal request path.
func (h *p0Harness) readBack(t *testing.T, addr, size uint64) []byte {
	t.Helper()

	msg := h.read(addr)
	msg.Payload = memprotocol.ReadReq{Address: addr, AccessByteSize: size}
	h.src.Send(msg)
	require.NoError(t, h.engine.Run())

	reads, _ := h.collect()
	require.Len(t, reads, 1)
	return reads[0].Payload.(memprotocol.DataReadyRsp).Data
}

func expectedMerge(background byte, data []byte, mask []bool) []byte {
	merged := bytes.Repeat([]byte{background}, len(data))
	for i := range data {
		if mask == nil || mask[i] {
			merged[i] = data[i]
		}
	}
	return merged
}

func TestWriteHonorsDirtyMask(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	sparse := []bool{true, false, false, true, false, true, false, false}
	allTrue := []bool{true, true, true, true, true, true, true, true}
	allFalse := make([]bool, len(data))

	cases := []struct {
		name string
		addr uint64
		mask []bool
	}{
		{name: "nil mask writes every byte", addr: 0x40, mask: nil},
		{name: "all-true mask writes every byte", addr: 0x40, mask: allTrue},
		{name: "all-false mask writes nothing", addr: 0x40, mask: allFalse},
		{name: "sparse mask keeps unmasked bytes", addr: 0x40, mask: sparse},
		{name: "unaligned write inside an access unit", addr: 0x45, mask: sparse},
		{name: "write spanning two access units", addr: 0x7C, mask: sparse},
		{name: "write spanning two storage units", addr: 0xFFC, mask: sparse},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newP0Harness(t, DefaultSpec())

			// The window includes guard bytes on both sides of the write.
			const guard = 4
			windowAddr := tc.addr - guard
			windowSize := uint64(len(data)) + 2*guard
			h.dram.Resources.Storage.Write(windowAddr,
				bytes.Repeat([]byte{maskTestBackground}, int(windowSize)))

			h.src.Send(h.maskedWrite(tc.addr, data, tc.mask))
			require.NoError(t, h.engine.Run())
			_, writes := h.collect()
			require.Len(t, writes, 1)

			want := bytes.Repeat([]byte{maskTestBackground}, int(windowSize))
			copy(want[guard:], expectedMerge(maskTestBackground, data, tc.mask))
			require.Equal(t, want, h.readBack(t, windowAddr, windowSize))
		})
	}
}

// A write whose response is blocked retries finalization. Overlapping masked
// writes must still all land while the response port is under backpressure.
func TestMaskedWritesSurviveResponseBackpressure(t *testing.T) {
	sim := akitasim.MakeBuilder().WithTaskTracking().Build()
	engine := sim.Engine().(*timing.SerialEngine)
	dramComp := buildDRAM(sim, DefaultSpec(), "MaskDRAM", 1)
	src := newDriverPort(sim, "MaskSrc.Top", 1)

	conn := direct.NewConnection("MaskConn", sim, timing.GHz)
	conn.BindPort(dramComp.Ports.Top)
	conn.BindPort(src)
	require.NoError(t, sim.Initialize())

	h := &p0Harness{engine: engine, dram: dramComp, src: src, top: dramComp.Ports.Top}
	const addr = uint64(0x40)
	dramComp.Resources.Storage.Write(addr, bytes.Repeat([]byte{maskTestBackground}, 4))

	// Nothing retrieves responses yet, so the later writes back up.
	writes := []struct {
		data []byte
		mask []bool
	}{
		{data: []byte{1, 1, 1, 1}, mask: []bool{true, false, false, false}},
		{data: []byte{2, 2, 2, 2}, mask: []bool{false, true, false, false}},
		{data: []byte{3, 3, 3, 3}, mask: []bool{false, false, true, false}},
	}
	for _, w := range writes {
		h.src.Send(h.maskedWrite(addr, w.data, w.mask))
		require.NoError(t, h.engine.Run())
	}

	completed := 0
	for completed < len(writes) {
		_, done := h.collect()
		require.NotEmpty(t, done, "blocked write responses should drain")
		completed += len(done)
		require.NoError(t, h.engine.Run())
	}

	require.Equal(t, []byte{1, 2, 3, maskTestBackground},
		dramComp.Resources.Storage.Read(addr, 4))
}

// The mask is functional only: it must not change DRAM timing or statistics.
func TestDirtyMaskDoesNotChangeTiming(t *testing.T) {
	run := func(t *testing.T, mask []bool) state {
		t.Helper()

		h := newP0Harness(t, DefaultSpec())
		data := make([]byte, 8)
		for _, addr := range []uint64{0x40, 0x7C, 0x20040} {
			h.src.Send(h.maskedWrite(addr, data, mask))
		}
		require.NoError(t, h.engine.Run())
		_, writes := h.collect()
		require.Len(t, writes, 3)
		return h.dram.State
	}

	unmasked := run(t, nil)
	t.Logf("unmasked: ticks=%d writeLatency=%d bytes=%d",
		unmasked.TickCount, unmasked.TotalWriteLatencyCycles, unmasked.BytesWritten)

	for _, mask := range [][]bool{
		{true, false, false, true, false, true, false, false},
		make([]bool, 8),
	} {
		masked := run(t, mask)
		require.Equal(t, unmasked.TickCount, masked.TickCount)
		require.Equal(t, unmasked.TotalWriteLatencyCycles, masked.TotalWriteLatencyCycles)
		require.Equal(t, unmasked.BytesWritten, masked.BytesWritten)
		require.Equal(t, unmasked.CompletedWrites, masked.CompletedWrites)
	}
}
