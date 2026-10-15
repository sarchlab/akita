package base_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

func TestBuildCopiesRecordSlices(t *testing.T) {
	type spec struct {
		Freq     timing.Freq         `json:"freq"`
		Bottom   mem.AddressMapper   `json:"bottom"`
		Mappings []mem.AddressMapper `json:"mappings"`
	}
	def := ticking.Definition[spec, modeling.None, modeling.None, modeling.None, modeling.None]{
		DefaultSpec: spec{
			Freq:     timing.GHz,
			Bottom:   mem.AddressMapper{Destinations: []messaging.RemotePort{"A"}},
			Mappings: []mem.AddressMapper{{Destinations: []messaging.RemotePort{"B"}}},
		},
	}
	s := akitasim.MakeBuilder().Build()
	a := def.Builder().WithSimulation(s).Build("A")
	b := def.Builder().WithSimulation(s).Build("B")
	def.DefaultSpec.Bottom.Destinations[0] = "Changed"
	def.DefaultSpec.Mappings[0].Destinations[0] = "Changed"
	require.Equal(t, messaging.RemotePort("A"), a.Spec.Bottom.Find(0))
	require.Equal(t, messaging.RemotePort("B"), a.Spec.Mappings[0].Find(0))
	a.Spec.Mappings[0].Destinations[0] = "ChangedInstance"
	require.Equal(t, messaging.RemotePort("B"), b.Spec.Mappings[0].Find(0))
}
