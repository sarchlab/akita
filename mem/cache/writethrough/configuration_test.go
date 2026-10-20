package writethrough

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func TestDirectGeometryAndIndependentQueues(t *testing.T) {
	for _, depth := range []int{1, 7} {
		spec := DefaultSpec()
		spec.Bottom.Destinations = []messaging.RemotePort{"Memory.Top"}
		spec.NumSets = 13
		spec.WayAssociativity = 3
		spec.Log2BlockSize = 6
		spec.NumReqPerCycle = 1
		spec.DirectoryWidth = 3
		spec.BankWidth = 2
		spec.DirBufferCapacity = depth
		spec.DirPostBufferCapacity = depth
		spec.BankPostBufferCapacity = depth
		spec.DirLatency = 1
		c := &Comp{}
		c.Spec = spec
		c.State.Initialize(c)
		require.Len(t, c.State.DirectoryState.Sets, 13)
		require.Equal(t, uint64(13*3*64), spec.ByteSize())
		require.Equal(t, depth, c.State.DirBuf.Capacity())
		require.Equal(t, depth, c.State.BankPostBufs[0].Capacity())
		// Queue depth changes do not change the three admissions allowed at the
		// directory pipeline's entry, even when the request dispatch width is one.
		for i := range 3 {
			require.True(t, c.State.DirPipeline.CanAccept())
			c.State.DirPipeline.Accept(i)
		}
		require.False(t, c.State.DirPipeline.CanAccept())
		c.State.DirPipeline.Tick(&c.State.DirPostBuf)
		require.Equal(t, min(depth, 3), c.State.DirPostBuf.Size())
		// A shallow sink stalls remaining items; draining resumes without loss.
		got := []int{}
		for c.State.DirPostBuf.Size() > 0 || len(c.State.DirPipeline.Stages()) > 0 {
			for c.State.DirPostBuf.Size() > 0 {
				v, _ := c.State.DirPostBuf.Pop()
				got = append(got, v)
			}
			c.State.DirPipeline.Tick(&c.State.DirPostBuf)
		}
		require.ElementsMatch(t, []int{0, 1, 2}, got)
	}
}
func TestInvalidDirectGeometryAndWidths(t *testing.T) {
	for _, modify := range []func(*Spec){
		func(s *Spec) { s.NumSets = 0 }, func(s *Spec) { s.WayAssociativity = 0 },
		func(s *Spec) { s.Log2BlockSize = 63 }, func(s *Spec) { s.NumSets = 8; s.Log2BlockSize = 62 },
		func(s *Spec) { s.NumBanks = 0 }, func(s *Spec) { s.DirectoryWidth = 0 },
		func(s *Spec) { s.BankWidth = 0 }, func(s *Spec) { s.DirBufferCapacity = 0 },
	} {
		spec := DefaultSpec()
		modify(&spec)
		require.Error(t, validateSpec(spec))
	}
}
