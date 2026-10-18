package gmmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
)

func TestPageTablePageSizeMustMatchGMMU(t *testing.T) {
	for _, bits := range []uint64{12, 16} {
		s := sim.MakeBuilder().Build()
		c := modeling.MakeBuilder[*Comp](DefaultSpec()).
			WithSimulation(s).
			WithResources(Resources{PageTable: vm.NewPageTable(bits)}).
			Build("GMMU")
		ports := defaultPorts("GMMU")
		c.BindPort("Top", ports.Top)
		c.BindPort("Bottom", ports.Bottom)
		c.BindPort("Control", ports.Control)
		if bits == c.Spec.Log2PageSize {
			require.NoError(t, s.Initialize())
		} else {
			require.PanicsWithValue(t, "page table page size does not match GMMU page size", func() {
				_ = s.Initialize()
			})
		}
		s.Terminate()
	}
}
