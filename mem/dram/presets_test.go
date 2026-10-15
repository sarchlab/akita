package dram_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/dram"
	"github.com/stretchr/testify/require"
)

func TestPresetsReturnIndependentValues(t *testing.T) {
	cases := []struct {
		name   string
		preset func() dram.Spec
		tcl    int
	}{
		{"DDR4", dram.DDR4Spec, 16},
		{"DDR5", dram.DDR5Spec, 40},
		{"HBM2", dram.HBM2Spec, 14},
		{"HBM3", dram.HBM3Spec, 36},
		{"GDDR6", dram.GDDR6Spec, 20},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, second := tc.preset(), tc.preset()
			first.TCL = 99
			fresh := tc.preset()

			require.Equal(t, tc.tcl, second.TCL)
			require.Equal(t, tc.tcl, fresh.TCL)
			require.Equal(t, second, fresh)
			t.Logf("%s: modified TCL=%d, independent TCL=%d, fresh TCL=%d",
				tc.name, first.TCL, second.TCL, fresh.TCL)
		})
	}
}
