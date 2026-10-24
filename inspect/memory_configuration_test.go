package inspect_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/inspect"
	"github.com/sarchlab/akita/v5/inspect/schema"
	"github.com/stretchr/testify/require"
)

func TestMemoryConfigurationSchema(t *testing.T) {
	definitions, errs := inspect.Inspect(inspect.Options{Dir: ".."},
		"./mem/dram", "./mem/cache/writethrough", "./mem/simplebankedmemory", "./mem/vm/tlb")
	require.Empty(t, errs)
	require.Len(t, definitions, 4)
	fields := map[string]map[string]schema.Field{}
	for _, definition := range definitions {
		fields[definition.Name] = map[string]schema.Field{}
		for _, field := range definition.Spec {
			fields[definition.Name][field.Name] = field
		}
		for _, port := range definition.Ports {
			if definition.Name == "tlb" && port.Name == "Top" {
				require.Equal(t, "github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol", port.Roles[0].Protocol)
			}
		}
	}
	protocol := fields["dram"]["Protocol"]
	require.Equal(t, "ddr3", protocol.Default)
	require.Contains(t, protocol.Choices, "ddr4")
	require.Contains(t, protocol.Choices, "hbm3e")
	require.ElementsMatch(t, []string{"close", "open"}, fields["dram"]["PagePolicy"].Choices)
	require.ElementsMatch(t, []string{"write-around", "write-evict", "write-through"},
		fields["writethrough"]["WritePolicy"].Choices)
	conversion := fields["simplebankedmemory"]["BankAddressConversion"]
	require.Equal(t, "bank_address_conversion", conversion.JSONName)
	require.Len(t, conversion.Fields, 5)
	require.Equal(t, "kind", conversion.Fields[0].JSONName)
	require.Equal(t, []string{"interleaving"}, conversion.Fields[0].Choices)
	t.Log("inspector exposes DRAM/write-policy choices, five flat conversion fields, and addrtransprotocol roles")
}

func TestMMUCachePeerAddressesInSpec(t *testing.T) {
	definitions, errs := inspect.Inspect(inspect.Options{Dir: ".."}, "./mem/vm/mmuCache")
	require.Empty(t, errs)
	require.Len(t, definitions, 1)
	definition := definitions[0]
	require.Empty(t, definition.Resources)
	fields := map[string]schema.Field{}
	for _, field := range definition.Spec {
		fields[field.Name] = field
	}
	require.Equal(t, "low_module", fields["LowModule"].JSONName)
	require.Equal(t, "up_module", fields["UpModule"].JSONName)
	require.NotContains(t, fields, "LowModulePort")
	require.NotContains(t, fields, "UpModulePort")
	t.Log("inspector exposes low_module and up_module in Spec with no Resources fields")
}
