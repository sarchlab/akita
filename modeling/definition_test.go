package modeling_test

import (
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
	NumLanes int         `json:"num_lanes"`
}

type defTestResources struct{}

func makeDefTestDef() modeling.ComponentDef[defTestSpec] {
	return modeling.ComponentDef[defTestSpec]{
		Name: "DefTest",
		DefaultSpec: defTestSpec{
			Freq:     1 * timing.GHz,
			NumLanes: 4,
		},
		Ports: []modeling.PortDef{
			{Name: "Top", Roles: []*messaging.Role{defTestResponder}},
			{Name: "Bottom", Roles: []*messaging.Role{defTestRequester}},
			{Name: "Out", Roles: []*messaging.Role{defTestRequester}, Group: true},
		},
	}
}

// --- Tests ---

func buildWithDef(
	def modeling.ComponentDef[defTestSpec], name string,
) *modeling.Component[defTestSpec, TestState, defTestResources] {
	engine := timing.NewSerialEngine()

	return modeling.NewBuilder[defTestSpec, TestState, defTestResources]().
		WithSimulation(modeling.NewStandaloneSimulation(engine)).
		WithFreq(1 * timing.GHz).
		WithSpec(def.DefaultSpec).
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
		WithSpec(makeDefTestDef().DefaultSpec).
		WithDefinition(makeDefTestDef()).
		Build("EDComp")

	checkDefTestRoles(t, comp)
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
