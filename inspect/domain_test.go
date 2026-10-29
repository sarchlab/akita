package inspect

import (
	"github.com/sarchlab/akita/v5/inspect/schema"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInspectBlueprintDomain(t *testing.T) {
	defs, errs := Inspect(Options{}, "./testdata/fixtures/blueprintdomain")
	require.Empty(t, errs)
	require.Len(t, defs, 1)
	d := defs[0]
	require.Equal(t, schema.KindDomain, d.Kind)
	require.Empty(t, d.Model)
	require.Len(t, d.Spec, 2)
	require.Len(t, d.Ports, 2)
	require.Equal(t, "Top", d.Ports[0].Name)
	require.False(t, d.Ports[0].Group)
	require.True(t, d.Ports[1].Group)
}
func TestInvalidDomainDiagnostics(t *testing.T) {
	for _, fixture := range []string{"baddomainports", "domainnodefault"} {
		t.Run(fixture, func(t *testing.T) {
			defs, errs := Inspect(Options{}, "./testdata/fixtures/"+fixture)
			require.Empty(t, defs)
			require.NotEmpty(t, errs)
		})
	}
}
func TestInspectRealAgentChain(t *testing.T) {
	defs, errs := Inspect(Options{}, "../mem/acceptancetests/sharedl2vm/agentchain")
	require.Empty(t, errs)
	require.Len(t, defs, 1)
	require.Equal(t, schema.KindDomain, defs[0].Kind)
	require.Len(t, defs[0].Ports, 2)
}
