package writethrough

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func TestDirectGeometryAndCalculatedQueues(t *testing.T) {
	for _, requests := range []int{1, 7} {
		spec := DefaultSpec()
		spec.Bottom.Destinations = []messaging.RemotePort{"Memory.Top"}
		spec.NumSets, spec.NumWays, spec.Log2BlockSize = 13, 3, 6
		spec.NumBanks = 2
		spec.NumReqPerCycle = requests
		spec.DirectoryWidth, spec.BankWidth = 3, 2
		spec.DirLatency = 1
		c := &Comp{}
		c.Spec = spec
		c.State.Initialize(c)
		require.Len(t, c.State.DirectoryState.Sets, 13)
		require.Equal(t, uint64(13*3*64), spec.ByteSize())
		require.Equal(t, requests, c.State.DirBuf.Capacity())
		require.Equal(t, 3, c.State.DirPostBuf.Capacity())
		for i := range spec.NumBanks {
			require.Equal(t, 2, c.State.BankPostBufs[i].Capacity())
		}

		for i := range spec.NumBanks {
			require.Equal(t, requests, c.State.BankBufs[i].Capacity())
		}

		// Fill the calculated output queue, then block a second batch behind it.
		for i := range 3 {
			c.State.DirPipeline.Accept(i)
		}
		require.False(t, c.State.DirPipeline.CanAccept())
		c.State.DirPipeline.Tick(&c.State.DirPostBuf)
		for i := range 3 {
			c.State.DirPipeline.Accept(i + 3)
		}
		for range 5 {
			c.State.DirPipeline.Tick(&c.State.DirPostBuf)
		}
		require.Equal(t, 3, c.State.DirPostBuf.Size())
		require.Len(t, c.State.DirPipeline.Stages(), 3)
		got := make([]int, 0, 6)
		for range 6 {
			value, ok := c.State.DirPostBuf.Pop()
			require.True(t, ok)
			got = append(got, value)
			c.State.DirPipeline.Tick(&c.State.DirPostBuf)
		}
		require.ElementsMatch(t, []int{0, 1, 2, 3, 4, 5}, got)
		require.Zero(t, c.State.DirPostBuf.Size())
		require.Empty(t, c.State.DirPipeline.Stages())
		t.Logf("dispatch %d, directory 3, bank 2: calculated queues stall and drain six entries without loss", requests)
	}
}
func TestInvalidDirectGeometryAndWidths(t *testing.T) {
	for _, modify := range []func(*Spec){
		func(s *Spec) { s.NumSets = 0 }, func(s *Spec) { s.NumWays = 0 },
		func(s *Spec) { s.Log2BlockSize = 63 }, func(s *Spec) { s.NumSets = 8; s.Log2BlockSize = 62 },
		func(s *Spec) { s.NumBanks = 0 }, func(s *Spec) { s.DirectoryWidth = 0 },
		func(s *Spec) { s.BankWidth = 0 }, func(s *Spec) { s.NumReqPerCycle = 0 },
	} {
		spec := DefaultSpec()
		modify(&spec)
		require.Error(t, validateSpec(spec))
	}
}
