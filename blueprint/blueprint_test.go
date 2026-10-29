package blueprint_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/wakeup"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type testSpec struct {
	Peer   messaging.RemotePort
	Label  string
	Values []int
}

type testPorts struct {
	Top   messaging.Port
	Links []messaging.Port
}

type testComp = wakeup.Component[testSpec, modeling.None, modeling.None, testPorts, modeling.None]
type resourceComp = wakeup.Component[testSpec, modeling.None, resourceUses, testPorts, modeling.None]
type resourceUses struct{ Storage *mem.Storage }

func portSpec() twowaybuffered.Spec {
	return twowaybuffered.Spec{IncomingCapacity: 3, OutgoingCapacity: 7}
}

func comp(spec testSpec) *blueprint.ComponentNode {
	return blueprint.Comp[*testComp](spec).Port("Top", portSpec())
}

type domainSpec struct {
	Peer  messaging.RemotePort
	Count int
}

type domainPorts struct {
	Top   messaging.Port
	Links []messaging.Port
}

func leaf(spec domainSpec) *blueprint.DomainNode {
	return blueprint.NewDomain[domainSpec, domainPorts](spec, func(s domainSpec, bp *blueprint.Blueprint) {
		bp.Add("Inner", comp(testSpec{Peer: s.Peer}))
		bp.Expose("Top", "Inner.Top")
		for i := s.Count - 1; i >= 0; i-- {
			name := fmt.Sprintf("Child[%d]", i)
			bp.Add(name, comp(testSpec{}))
			bp.Expose(fmt.Sprintf("Links[%d]", i), name+".Top")
		}
	})
}

func TestCyclicDomainsAndIndependentCopies(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			s := sim.MakeBuilder().Build()
			defer s.Terminate()
			root := blueprint.NewDomain[modeling.None, modeling.None](modeling.None{}, func(_ modeling.None,
				bp *blueprint.Blueprint) {
				a, b := leaf(domainSpec{Peer: bp.PortRef("B.Top"), Count: 17}),
					leaf(domainSpec{Peer: bp.PortRef("A.Top"), Count: 17})
				if reverse {
					bp.Add("B", b)
					bp.Add("A", a)
				} else {
					bp.Add("A", a)
					bp.Add("B", b)
				}
				bp.Connect("Wire", timing.GHz, bp.PortRef("A.Top"), bp.PortRef("B.Top"))
			})
			s.Add("", root)
			require.NoError(t, s.Initialize())
			a := s.Lookup("A.Inner").(*testComp)
			b := s.Lookup("B.Inner").(*testComp)
			require.Equal(t, b.Ports.Top.AsRemote(), a.Spec.Peer)
			require.Equal(t, a.Ports.Top.AsRemote(), b.Spec.Peer)
			ports := s.Lookup("A").(domainPorts)
			require.Same(t, a.Ports.Top, ports.Top)
			require.Len(t, ports.Links, 17)
			for i, p := range ports.Links {
				require.Equal(t, fmt.Sprintf("A.Child[%d].Top", i), p.Name())
			}
			require.Equal(t, 3, ports.Top.IncomingBuf().Capacity())
			require.Equal(t, 7, ports.Top.OutgoingBuf().Capacity())
		})
	}
}

func TestPortRefDomainRootAndAbsoluteSpecPassThrough(t *testing.T) {
	for _, rootName := range []string{"", "Platform"} {
		t.Run(rootName, func(t *testing.T) {
			s := sim.MakeBuilder().Build()
			defer s.Terminate()
			prefix := ""
			if rootName != "" {
				prefix = rootName + "."
			}
			s.Add(rootName, blueprint.NewDomain[modeling.None, modeling.None](modeling.None{},
				func(_ modeling.None, bp *blueprint.Blueprint) {
					external := bp.PortRef("External.Top")
					require.Equal(t, messaging.RemotePort(prefix+"External.Top"), external)
					bp.Add("D", blueprint.NewDomain[domainSpec, domainPorts](domainSpec{Peer: external},
						func(spec domainSpec, child *blueprint.Blueprint) {
							// D inside D is a child name, never an already absolute prefix.
							local := child.PortRef("D.Top")
							require.Equal(t, messaging.RemotePort(prefix+"D.D.Top"), local)
							path := child.Add("D", comp(testSpec{Peer: spec.Peer}))
							require.Equal(t, prefix+"D.D", path)
							child.Add("Client", comp(testSpec{Peer: local}))
							child.Expose("Top", "D.Top")
						}))
					bp.Add("External", comp(testSpec{Peer: bp.PortRef("D.Top")}))
				}))
			require.NoError(t, s.Initialize())
			inner := s.Lookup(prefix + "D.D").(*testComp)
			external := s.Lookup(prefix + "External").(*testComp)
			client := s.Lookup(prefix + "D.Client").(*testComp)
			require.Equal(t, external.Ports.Top.AsRemote(), inner.Spec.Peer)
			require.Equal(t, inner.Ports.Top.AsRemote(), external.Spec.Peer)
			require.Equal(t, inner.Ports.Top.AsRemote(), client.Spec.Peer)
		})
	}
}

func TestExpandPureSnapshotsAndNestedAliases(t *testing.T) {
	spec := testSpec{Values: []int{1, 2}, Label: "not a port"}
	n := comp(spec)
	var bp blueprint.Blueprint
	bp.Add("A", n)
	spec.Values[0] = 9
	n.Spec = testSpec{Values: []int{10}}
	x, err := blueprint.Expand(&bp)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, x.Nodes()[0].(*blueprint.ComponentNode).Spec.(testSpec).Values)
	x.Nodes()[0].(*blueprint.ComponentNode).Spec.(testSpec).Values[0] = 8
	y, err := blueprint.Expand(&bp)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, y.Nodes()[0].(*blueprint.ComponentNode).Spec.(testSpec).Values)
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	s.Add("Outer", blueprint.NewDomain[modeling.None, domainPorts](modeling.None{}, func(_ modeling.None,
		bp *blueprint.Blueprint) {
		bp.Add("Inner", leaf(domainSpec{}))
		bp.Expose("Top", "Inner.Top")
	}))
	require.NoError(t, s.Initialize())
	require.Equal(t, "Outer.Inner.Inner.Top", s.Lookup("Outer").(domainPorts).Top.Name())
}

func TestSharedResourceAndDomainIsolation(t *testing.T) {
	type empty struct{}
	node := blueprint.NewDomain[empty, modeling.None](empty{}, func(_ empty, bp *blueprint.Blueprint) {
		storage := bp.Add("Memory", blueprint.Res(mem.StorageSpec{Capacity: 1024, UnitSize: 64}))
		for _, name := range []string{"A", "B"} {
			bp.Add(name, blueprint.Comp[*resourceComp](testSpec{}).Port("Top", portSpec()).Use("Storage", storage))
		}
	})
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	s.Add("X", node)
	s.Add("Y", node)
	require.NoError(t, s.Initialize())
	xa := s.Lookup("X.A").(*resourceComp)
	xb := s.Lookup("X.B").(*resourceComp)
	ya := s.Lookup("Y.A").(*resourceComp)
	require.Same(t, xa.Resources.Storage, xb.Resources.Storage)
	require.NotSame(t, xa.Resources.Storage, ya.Resources.Storage)
	xa.Resources.Storage.Write(0, []byte{42})
	require.Equal(t, []byte{42}, xb.Resources.Storage.Read(0, 1))
	require.Equal(t, []byte{0}, ya.Resources.Storage.Read(0, 1))
}

func TestExpandDiagnostics(t *testing.T) {
	tests := []struct {
		name, want string
		declare    func(*blueprint.Blueprint)
	}{
		{"missing ref", "A.Spec.Peer", func(bp *blueprint.Blueprint) {
			bp.Add("A", comp(testSpec{Peer: bp.PortRef("Missing.Top")}))
		}},
		{"missing port", "A.Ports.Top", func(bp *blueprint.Blueprint) {
			bp.Add("A",
				blueprint.Comp[*testComp](testSpec{}))
		}},
		{"unknown port", "A.Wrong", func(bp *blueprint.Blueprint) {
			bp.Add("A", comp(testSpec{}).Port("Wrong",
				portSpec()))
		}},
		{"duplicate port", "A.Top", func(bp *blueprint.Blueprint) {
			bp.Add("A", comp(testSpec{}).Port("Top",
				portSpec()))
		}},
		{"capacity", "A.Top", func(bp *blueprint.Blueprint) {
			bp.Add("A", blueprint.Comp[*testComp](testSpec{}).Port("Top", twowaybuffered.Spec{IncomingCapacity: 1}))
		}},
		{"duplicate name", "A.name", func(bp *blueprint.Blueprint) {
			bp.Add("A", comp(testSpec{}))
			bp.Add("A", comp(testSpec{}))
		}},
		{"missing resource", "A.Uses.Storage", func(bp *blueprint.Blueprint) {
			bp.Add("A", blueprint.Comp[*resourceComp](testSpec{}).Port("Top", portSpec()).Use("Storage", "Absent"))
		}},
		{"unknown resource field", "A.Uses.Other", func(bp *blueprint.Blueprint) {
			bp.Add("A", comp(testSpec{}).Use("Other", "Absent"))
		}},
		{"unexposed boundary", "A.Spec.Peer", func(bp *blueprint.Blueprint) {
			bp.Add("D", leaf(domainSpec{}))
			bp.Add("A", comp(testSpec{Peer: bp.PortRef("D.Inner.Top")}))
		}},
		{"missing expose", "D.Ports.Top", func(bp *blueprint.Blueprint) {
			bp.Add("D", blueprint.NewDomain[modeling.None, domainPorts](modeling.None{}, func(_ modeling.None,
				_ *blueprint.Blueprint) {
			}))
		}},
		{"duplicate expose", "D.Top", func(bp *blueprint.Blueprint) {
			bp.Add("D", blueprint.NewDomain[modeling.None, domainPorts](modeling.None{}, func(_ modeling.None,
				b *blueprint.Blueprint) {
				b.Expose("Top", "A.Top")
				b.Expose("Top", "A.Top")
			}))
		}},
		{"group gap", "D.Links", func(bp *blueprint.Blueprint) {
			bp.Add("D", blueprint.NewDomain[modeling.None, domainPorts](modeling.None{}, func(_ modeling.None,
				b *blueprint.Blueprint) {
				b.Expose("Top", "A.Top")
				b.Expose("Links[1]", "A.Top")
			}))
		}},
		{"alias loop", "alias cycle", func(bp *blueprint.Blueprint) {
			bp.Add("D", blueprint.NewDomain[modeling.None, domainPorts](modeling.None{}, func(_ modeling.None,
				b *blueprint.Blueprint) {
				b.Expose("Top", "Top")
			}))
		}},
		{"duplicate binding", "already attached", func(bp *blueprint.Blueprint) {
			bp.Add("A", comp(testSpec{}))
			bp.Connect("Wire", timing.GHz, bp.PortRef("A.Top"), bp.PortRef("A.Top"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bp blueprint.Blueprint
			tt.declare(&bp)
			_, err := blueprint.Expand(&bp)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestInitializationPanicAndUnconnected(t *testing.T) {
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	s.Add("A", comp(testSpec{Peer: messaging.Unconnected}))
	require.NoError(t, s.Initialize())
	a := s.Lookup("A").(*testComp)
	require.PanicsWithValue(t, `twowaybuffered: sender "A.Top" cannot send to "<unconnected>"`,
		func() { a.Ports.Top.Send(messaging.Msg{Dst: messaging.Unconnected}) })
	bad := sim.MakeBuilder().Build()
	defer bad.Terminate()
	bad.Add("Broken", comp(testSpec{Peer: "Missing.Top"}))
	require.Panics(t, func() { _ = bad.Initialize() })
	require.Panics(t, func() { _ = bad.Initialize() })
	require.Panics(t, func() { _ = s.Lookup("A").(int) })
}

func TestBoundaryChecksOriginalAliases(t *testing.T) {
	var bp blueprint.Blueprint
	bp.Add("D", blueprint.NewDomain[domainSpec, domainPorts](domainSpec{Peer: bp.PortRef("External.Top")},
		func(s domainSpec, b *blueprint.Blueprint) {
			b.Add("A", comp(testSpec{Peer: s.Peer}))
			b.Expose("Top", "A.Top")
		}))
	bp.Add("External", comp(testSpec{Peer: bp.PortRef("D.Top")}))
	x, err := blueprint.Expand(&bp)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(x.Nodes()[0].(*blueprint.ComponentNode).Spec.(testSpec).
		Peer), "External"))
}

type linkedSpec struct{ Tag string }

type linkedResource struct {
	name   string
	parent *linkedResource
}

type linkedUses struct{ Parent *linkedResource }

func (r *linkedResource) Name() string { return r.name }

func (s linkedSpec) BuildResource(simulation modeling.Simulation, name string, uses linkedUses) *linkedResource {
	r := &linkedResource{name, uses.Parent}
	simulation.RegisterResource(r)
	return r
}

type wrappedSpec struct{ Tag string }

type wrappedResource struct {
	name    string
	storage *mem.Storage
}

type wrappedUses struct{ Storage *mem.Storage }

func (r *wrappedResource) Name() string { return r.name }

func (s wrappedSpec) BuildResource(simulation modeling.Simulation, name string, uses wrappedUses) *wrappedResource {
	r := &wrappedResource{name, uses.Storage}
	simulation.RegisterResource(r)
	return r
}

func TestResourceDependenciesAndTypeDiagnostics(t *testing.T) {
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	s.Add("Wrapper", blueprint.Res(wrappedSpec{Tag: "outer"}).Use("Storage", "Memory"))
	s.Add("Memory", blueprint.Res(mem.StorageSpec{Capacity: 64, UnitSize: 16}))
	require.NoError(t, s.Initialize())
	require.Same(t, s.Lookup("Memory").(*mem.Storage), s.Lookup("Wrapper").(*wrappedResource).storage)
	var mismatch blueprint.Blueprint
	mismatch.Add("Wrong", blueprint.Res(wrappedSpec{}).Use("Storage", "Other"))
	mismatch.Add("Other", blueprint.Res(wrappedSpec{}).Use("Storage", "Other"))
	_, err := blueprint.Expand(&mismatch)
	require.ErrorContains(t, err, "Wrong.Uses.Storage")
	require.ErrorContains(t, err, "need *mem.Storage")
	var cycle blueprint.Blueprint
	cycle.Add("A", blueprint.Res(linkedSpec{}).Use("Parent", "B"))
	cycle.Add("B", blueprint.Res(linkedSpec{}).Use("Parent", "A"))
	_, err = blueprint.Expand(&cycle)
	require.ErrorContains(t, err, "resource dependency cycle")
}

func TestSamePrefixComponentAndDomainAlias(t *testing.T) {
	type public struct{ CommandProcessor messaging.Port }
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	s.Add("GPU", blueprint.NewDomain[modeling.None, public](modeling.None{}, func(_ modeling.None,
		bp *blueprint.Blueprint) {
		bp.Add("CommandProcessor", comp(testSpec{}))
		bp.Expose("CommandProcessor", "CommandProcessor.Top")
	}))
	s.Add("Driver", comp(testSpec{Peer: "GPU.CommandProcessor"}))
	require.NoError(t, s.Initialize())
	require.Equal(t, messaging.RemotePort("GPU.CommandProcessor.Top"), s.Lookup("Driver").(*testComp).Spec.Peer)
}

func TestPortGroupConfigurationOrder(t *testing.T) {
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	n := comp(testSpec{})
	for i := 16; i >= 0; i-- {
		n.Port(fmt.Sprintf("Links[%d]", i), portSpec())
	}
	s.Add("Group", n)
	require.NoError(t, s.Initialize())
	ports := s.Lookup("Group").(*testComp).Ports.Links
	require.Len(t, ports, 17)
	for i, p := range ports {
		require.Equal(t, fmt.Sprintf("Group.Links[%d]", i), p.Name())
	}
}

func TestAddRejectsDottedNames(t *testing.T) {
	var bp blueprint.Blueprint
	require.PanicsWithError(t, "blueprint: A.Top.name: node name must be a single path segment", func() {
		bp.Add("A.Top", comp(testSpec{}))
	})
	require.PanicsWithError(t, "blueprint: A.Wire.name: node name must be a single path segment", func() {
		bp.Connect("A.Wire", timing.GHz)
	})
}

func TestChangedSpecTypesFailDuringExpand(t *testing.T) {
	component := comp(testSpec{})
	component.Spec = modeling.None{}
	resource := blueprint.Res(mem.StorageSpec{Capacity: 64, UnitSize: 16})
	resource.Spec = modeling.None{}
	domain := leaf(domainSpec{})
	domain.Spec = modeling.None{}
	for _, node := range []blueprint.Node{component, resource, domain} {
		var bp blueprint.Blueprint
		bp.Add("Changed", node)
		_, err := blueprint.Expand(&bp)
		require.ErrorContains(t, err, "Changed.Spec: expected")
		require.NotContains(t, err.Error(), "interface conversion")
	}
}

func TestMalformedConstructionDescriptors(t *testing.T) {
	var bp blueprint.Blueprint
	bp.Add("A", blueprint.Comp[*resourceComp](testSpec{}).Port("Top", portSpec()).
		Use("Storage", "Raw"))
	bp.Add("Raw", &blueprint.ResourceNode{})
	_, err := blueprint.Expand(&bp)
	require.ErrorContains(t, err, "Raw.Kind: use Res")
}

func TestAdditionalDeclarationDiagnostics(t *testing.T) {
	cases := []struct {
		name, want string
		declare    func(*blueprint.Blueprint)
	}{
		{"invalid group", "Links[01]", func(bp *blueprint.Blueprint) {
			bp.Add("A", comp(testSpec{}).Port("Links[01]", portSpec()))
		}},
		{"missing use", "Uses.Storage", func(bp *blueprint.Blueprint) {
			bp.Add("A", blueprint.Comp[*resourceComp](testSpec{}).Port("Top", portSpec()))
		}},
		{"component used as resource", "unknown resource", func(bp *blueprint.Blueprint) {
			bp.Add("A", comp(testSpec{}))
			bp.Add("B", blueprint.Comp[*resourceComp](testSpec{}).Port("Top", portSpec()).Use("Storage", "A"))
		}},
		{"unconnected member", "Members[0]", func(bp *blueprint.Blueprint) {
			bp.Connect("Wire", timing.GHz, messaging.Unconnected)
		}},
		{"unknown connection member", "Wire.Members[0]", func(bp *blueprint.Blueprint) {
			bp.Connect("Wire", timing.GHz, bp.PortRef("Missing.Top"))
		}},
		{"zero frequency", "Wire.Freq", func(bp *blueprint.Blueprint) { bp.Connect("Wire", 0) }},
		{"unknown exposed slot", "D.Unknown", func(bp *blueprint.Blueprint) {
			bp.Add("D",
				blueprint.NewDomain[modeling.None, modeling.None](modeling.None{}, func(_ modeling.None,
					b *blueprint.Blueprint) {
					b.Expose("Unknown", "A.Top")
				}))
		}},
		{"unknown exposed target", "D.Top", func(bp *blueprint.Blueprint) {
			bp.Add("D", blueprint.NewDomain[modeling.None,
				domainPorts](modeling.None{}, func(_ modeling.None, b *blueprint.Blueprint) { b.Expose("Top", "Missing.Top") }))
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var bp blueprint.Blueprint
			tt.declare(&bp)
			_, err := blueprint.Expand(&bp)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestNodeNamesBelongToIndependentSnapshots(t *testing.T) {
	original := comp(testSpec{})
	var localName string
	domain := blueprint.NewDomain[modeling.None, modeling.None](modeling.None{}, func(_ modeling.None,
		bp *blueprint.Blueprint) {
		absolute := bp.Add("Child", original)
		require.Contains(t, absolute, ".Child")
		localName = bp.Nodes()[0].Name()
	})
	var bp blueprint.Blueprint
	bp.Add("A", domain)
	bp.Add("B", domain)
	require.Empty(t, original.Name())
	require.Empty(t, domain.Name())
	require.Equal(t, "A", bp.Nodes()[0].Name())
	expanded, err := blueprint.Expand(&bp)
	require.NoError(t, err)
	require.Equal(t, "Child", localName)
	require.Equal(t, "A.Child", expanded.Nodes()[0].Name())
	require.Equal(t, "B.Child", expanded.Nodes()[1].Name())
	require.Empty(t, original.Name())
	require.Equal(t, "A", bp.Nodes()[0].Name())
}

func TestSimulationLookupUsesRuntimeInventoryInBothAssemblyModes(t *testing.T) {
	for _, declarative := range []bool{false, true} {
		s := sim.MakeBuilder().Build()
		defer s.Terminate()
		require.Panics(t, func() { s.Lookup("Agent") })
		if declarative {
			s.Add("Agent", comp(testSpec{}))
			s.Add("Memory", blueprint.Res(mem.StorageSpec{Capacity: 64, UnitSize: 16}))
		} else {
			c := modeling.MakeBuilder[*testComp](testSpec{}).WithSimulation(s).Build("Agent")
			c.BindPort("Top", twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(portSpec()).Build())
			mem.MakeStorageBuilder().WithSimulation(s).WithCapacity(64).WithUnitSize(16).Build("Memory")
		}
		require.NoError(t, s.Initialize())
		c := s.Lookup("Agent").(*testComp)
		require.Same(t, c.Ports.Top, s.Lookup("Agent.Top"))
		require.IsType(t, &mem.Storage{}, s.Lookup("Memory"))
		require.Same(t, s.Engine(), s.Lookup(s.Engine().(*timing.SerialEngine).Name()))
		require.Panics(t, func() { s.Lookup("Absent") })
	}
}

func TestDomainPortsCannotShadowRuntimeInventory(t *testing.T) {
	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	s.Add(s.Engine().(*timing.SerialEngine).Name(), blueprint.NewDomain[modeling.None, modeling.None](modeling.None{},
		func(_ modeling.None, _ *blueprint.Blueprint) {}))
	require.Panics(t, func() { _ = s.Initialize() })
}

func TestExposeRequiresDomainButAllowsEmptyRootDomain(t *testing.T) {
	var bare blueprint.Blueprint
	bare.Add("Inner", comp(testSpec{}))
	bare.Expose("Top", "Inner.Top")
	_, err := blueprint.Expand(&bare)
	require.EqualError(t, err, "blueprint: <root>.Expose: requires a domain declaration")

	s := sim.MakeBuilder().Build()
	defer s.Terminate()
	s.Add("", leaf(domainSpec{}))
	require.NoError(t, s.Initialize())
	ports := s.Lookup("").(domainPorts)
	require.Same(t, s.Lookup("Inner.Top"), ports.Top)
}
