package mem_test

import (
	"encoding/json"
	"testing"

	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/cache/writethrough"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/dram"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/simplebankedmemory"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/gmmu"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/mem/vm/mmuCache"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func TestConfigurationJSON(t *testing.T) {
	for _, tc := range []struct {
		name   string
		value  any
		fields map[string]any
	}{
		{"dram", dram.DDR4Spec(), map[string]any{"protocol": "ddr4", "page_policy": "close"}},
		{"writethrough", writethrough.DefaultSpec(), map[string]any{
			"write_policy": "write-around", "max_in_flight_requests": float64(16),
		}},
		{"writeback", writeback.DefaultSpec(), map[string]any{
			"max_in_flight_fetches": float64(128), "max_in_flight_evictions": float64(128),
		}},
		{"mmu", mmu.DefaultSpec(), map[string]any{"max_in_flight_translations": float64(16)}},
		{"ideal", idealmemcontroller.DefaultSpec(), map[string]any{"num_req_per_cycle": float64(1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.value)
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(data, &got))
			for key, value := range tc.fields {
				require.Equal(t, value, got[key], key)
			}
			t.Logf("%s: %v", tc.name, tc.fields)
		})
	}
	var spec dram.Spec
	require.NoError(t, json.Unmarshal([]byte(`{"protocol":"hbm3e","page_policy":"open"}`), &spec))
	require.Equal(t, dram.ProtocolHBM3E, spec.Protocol)
	require.Equal(t, dram.PagePolicyOpen, spec.PagePolicy)
}

func TestRenamedProtocolCheckpointPayloads(t *testing.T) {
	for _, payload := range []any{
		addrtransprotocol.Req{VAddr: 4096, PID: 1}, addrtransprotocol.Rsp{},
		datamoverprotocol.Req{
			SrcSide: datamoverprotocol.SideInside, DstSide: datamoverprotocol.SideOutside, ByteSize: 64,
		},
		datamoverprotocol.Rsp{},
	} {
		original := messaging.Msg{ID: 17, Payload: payload}
		data, err := json.Marshal(original)
		require.NoError(t, err)
		var restored messaging.Msg
		require.NoError(t, json.Unmarshal(data, &restored))
		require.Equal(t, original, restored)
		t.Logf("%T round-trips through the registered message codec", payload)
	}
}

func TestNegativeSpecLatenciesRejected(t *testing.T) {
	cases := map[string]func(){
		"ideal": func() {
			c := &idealmemcontroller.Comp{}
			c.Spec = idealmemcontroller.DefaultSpec()
			c.Spec.Latency = -1
			c.Middlewares.Initialize(c)
		},
		"mmu": func() { c := &mmu.Comp{}; c.Spec = mmu.DefaultSpec(); c.Spec.Latency = -1; c.Middlewares.Initialize(c) },
		"gmmu": func() {
			c := &gmmu.Comp{}
			c.Spec = gmmu.DefaultSpec()
			c.Spec.Latency = -1
			c.Middlewares.Initialize(c)
		},
		"mmuCache": func() {
			c := &mmuCache.Comp{}
			c.Spec = mmuCache.DefaultSpec()
			c.Spec.LatencyPerLevel = -1
			c.State.Initialize(c)
		},
		"tlb": func() { c := &tlb.Comp{}; c.Spec = tlb.DefaultSpec(); c.Spec.Latency = -1; c.State.Initialize(c) },
		"banked": func() {
			c := &simplebankedmemory.Comp{}
			c.Spec = simplebankedmemory.DefaultSpec()
			c.Spec.StageLatency = -1
			c.State.Initialize(c)
		},
		"dram": func() { c := &dram.Comp{}; c.Spec = dram.DefaultSpec(); c.Spec.TCL = -1; c.Middlewares.Initialize(c) },
	}
	for name, initialize := range cases {
		t.Run(name, func(t *testing.T) {
			if name == "tlb" {
				require.PanicsWithError(t, ": TLB latency must be nonnegative", initialize)
				return
			}
			require.PanicsWithValue(t, map[string]any{
				"ideal":    "idealmemcontroller: Latency must be nonnegative",
				"mmu":      "vm/mmu: Latency must be nonnegative",
				"gmmu":     "vm/gmmu: Latency must be nonnegative",
				"mmuCache": "vm/mmuCache: LatencyPerLevel must be nonnegative",
				"banked":   "simplebankedmemory: StageLatency must be nonnegative",
				"dram":     "dram: timing parameters must be nonnegative",
			}[name], initialize)
		})
	}
}
