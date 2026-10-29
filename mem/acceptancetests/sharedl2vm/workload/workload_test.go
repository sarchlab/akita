package workload

import (
	"math"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/stretchr/testify/require"
)

func TestInvalidAddressArithmeticFailsBeforeMapping(t *testing.T) {
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	vm.MakePageTableBuilder().WithSimulation(s).WithLog2PageSize(12).Build("PageTable")
	require.NoError(t, s.Initialize())
	for _, tc := range []struct {
		size, offset uint64
		message      string
	}{
		{0, 0, "invalid aligned address range"},
		{math.MaxUint64, 0, "invalid aligned address range"},
		{1 << 62, 0, "address range overflows"},
		{4096, math.MaxUint64 - 3, "address range overflows"},
		{4096, math.MaxUint64 - 0x10000 - 3, "physical address range overflows"},
	} {
		spec := DefaultSpec()
		spec.AddressRange, spec.AddressOffset = tc.size, tc.offset
		require.PanicsWithValue(t, "sharedl2vm workload: "+tc.message, func() { New(spec).Execute(s) })
	}
}
