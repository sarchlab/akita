package modeling_test

import (
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// --- Test protocol and roles ---

type defTestReq struct{ messaging.MsgMeta }
type defTestRsp struct{ messaging.MsgMeta }

var (
	defTestProtocol = messaging.DefineProtocol("modeling.definitiontest",
		messaging.RoleDef{
			Name: "requester", Sends: []messaging.Msg{defTestReq{}}},
		messaging.RoleDef{
			Name: "responder", Sends: []messaging.Msg{defTestRsp{}}},
	)
	defTestRequester = defTestProtocol.Role("requester")
	defTestResponder = defTestProtocol.Role("responder")
)

// --- Test Spec and Resources types ---

type defTestSpec struct {
	Freq     timing.Freq `json:"freq"`
	NumLanes int         `json:"num_lanes" akita:"min=1"`
	NumOut   int         `json:"num_out" akita:"derived"`
	Values   []int       `json:"values"`
	Weights  map[string]float64
}

type defTestResources struct{}

func makeDefTestDef() modeling.ComponentDef[defTestSpec] {
	return modeling.ComponentDef[defTestSpec]{
		Name: "DefTest",
		DefaultSpec: defTestSpec{
			Freq:     1 * timing.GHz,
			NumLanes: 4,
			Values:   []int{1, 2},
			Weights:  map[string]float64{"a": 1.5},
		},
		Ports: []modeling.PortDef{
			{Name: "Top", Roles: []*messaging.Role{defTestResponder}},
			{Name: "Bottom", Roles: []*messaging.Role{defTestRequester}},
			{Name: "Out", Roles: []*messaging.Role{defTestRequester}, Group: true},
		},
	}
}

// --- Tests ---

func TestComponentDef(t *testing.T) {
	def := makeDefTestDef()

	if def.Name != "DefTest" {
		t.Errorf("Name = %q, want %q", def.Name, "DefTest")
	}

	got, want := def.NewSpec(), makeDefTestDef().DefaultSpec
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NewSpec() = %+v, want %+v", got, want)
	}

}

func TestComponentDefNewSpecIsACopy(t *testing.T) {
	def := makeDefTestDef()

	spec := def.NewSpec()
	spec.Values[0] = 99
	spec.Weights["a"] = 99

	fresh := def.NewSpec()
	if fresh.Values[0] != 1 {
		t.Errorf("mutating a returned spec's slice changed the default")
	}
	if fresh.Weights["a"] != 1.5 {
		t.Errorf("mutating a returned spec's map changed the default")
	}
}

func buildWithDef(
	def modeling.ComponentDef[defTestSpec], name string,
) *modeling.Component[defTestSpec, TestState, defTestResources] {
	engine := timing.NewSerialEngine()

	return modeling.NewBuilder[defTestSpec, TestState, defTestResources]().
		WithSimulation(modeling.NewStandaloneSimulation(engine)).
		WithFreq(1 * timing.GHz).
		WithSpec(def.NewSpec()).
		WithDefinition(def).
		Build(name)
}

func checkDefTestRoles(t *testing.T, po interface {
	PortRoles(name string) []*messaging.Role
}) {
	t.Helper()

	if roles := po.PortRoles("Top"); len(roles) != 1 ||
		roles[0] != defTestResponder {
		t.Errorf("PortRoles(Top) = %+v, want [responder]", roles)
	}

	if roles := po.PortRoles("Out"); len(roles) != 1 ||
		roles[0] != defTestRequester {
		t.Errorf("PortRoles(Out) = %+v, want [requester]", roles)
	}
}

func TestBuilderWithDefinitionDeclaresPorts(t *testing.T) {
	comp := buildWithDef(makeDefTestDef(), "Comp")

	checkDefTestRoles(t, comp)

	if n := comp.NumPortsInGroup("Out"); n != 0 {
		t.Errorf("NumPortsInGroup(Out) = %d, want 0", n)
	}
}

func TestEventDrivenBuilderWithDefinitionDeclaresPorts(t *testing.T) {
	engine := timing.NewSerialEngine()

	comp := modeling.NewEventDrivenBuilder[defTestSpec, TestState, defTestResources]().
		WithSimulation(modeling.NewStandaloneSimulation(engine)).
		WithSpec(makeDefTestDef().NewSpec()).
		WithDefinition(makeDefTestDef()).
		Build("EDComp")

	checkDefTestRoles(t, comp)
}

func TestComponentDefNewSpecCopiesNestedContainers(t *testing.T) {
	type spec struct {
		Slices [][]int
		Maps   map[string][]int
		Arrays [1][]int
		Nested []map[int][1][]int
	}
	input := spec{
		Slices: [][]int{{1}}, Maps: map[string][]int{"a": {2}},
		Arrays: [1][]int{{3}}, Nested: []map[int][1][]int{{4: {{5}}}},
	}
	def := modeling.ComponentDef[spec]{Name: "Nested", DefaultSpec: input}
	mutate := func(s spec) {
		s.Slices[0][0] = 99
		s.Maps["a"][0] = 99
		s.Arrays[0][0] = 99
		s.Nested[0][4][0][0] = 99
	}
	mutate(def.NewSpec())
	got := def.NewSpec()
	if got.Slices[0][0] != 1 || got.Maps["a"][0] != 2 || got.Arrays[0][0] != 3 || got.Nested[0][4][0][0] != 5 {
		t.Fatalf("nested defaults were mutated: %+v", got)
	}
}

func TestComponentDefDeclaredRolesAreIndependent(t *testing.T) {
	def := makeDefTestDef()
	first, second := buildWithDef(def, "First"), buildWithDef(def, "Second")
	first.PortRoles("Top")[0] = nil
	first.PortRoles("Out")[0] = nil
	if second.PortRoles("Top")[0] != defTestResponder || second.PortRoles("Out")[0] != defTestRequester {
		t.Fatal("mutating one component's roles affected another component")
	}
	if def.Ports[0].Roles[0] != defTestResponder || def.Ports[2].Roles[0] != defTestRequester {
		t.Fatal("mutating a component's roles affected the definition")
	}
}
