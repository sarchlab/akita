package modeling_test

import (
	"fmt"
	"reflect"
	"strings"
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

func makeDefTestDef() modeling.ComponentDef[defTestSpec, defTestResources] {
	return modeling.ComponentDef[defTestSpec, defTestResources]{
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
		},
		PortGroups: []modeling.PortGroupDef{
			{
				Name:       "Out",
				Roles:      []*messaging.Role{defTestRequester},
				MinCount:   1,
				CountField: "num_out",
			},
		},
	}
}

func mustPanicWith(t *testing.T, substr string, f func()) {
	t.Helper()

	defer func() {
		t.Helper()

		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q, got none", substr)
		}
		if !strings.Contains(fmt.Sprint(r), substr) {
			t.Fatalf("panic %q does not contain %q", fmt.Sprint(r), substr)
		}
	}()

	f()
}

// --- Tests ---

func TestDefineComponent(t *testing.T) {
	def := modeling.DefineComponent(makeDefTestDef())

	if def.Name() != "DefTest" {
		t.Errorf("Name() = %q, want %q", def.Name(), "DefTest")
	}

	got, want := def.DefaultSpec(), makeDefTestDef().DefaultSpec
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DefaultSpec() = %+v, want %+v", got, want)
	}

	ports := def.Ports()
	if len(ports) != 2 || ports[0].Name != "Top" || ports[1].Name != "Bottom" {
		t.Errorf("Ports() = %+v", ports)
	}
	if len(ports[0].Roles) != 1 || ports[0].Roles[0] != defTestResponder {
		t.Errorf("Ports()[0].Roles = %+v, want [responder]", ports[0].Roles)
	}

	groups := def.PortGroups()
	if len(groups) != 1 || groups[0].Name != "Out" ||
		groups[0].MinCount != 1 || groups[0].MaxCount != 0 ||
		groups[0].CountField != "num_out" {
		t.Errorf("PortGroups() = %+v", groups)
	}
}

func TestDefinitionDefaultSpecIsACopy(t *testing.T) {
	def := modeling.DefineComponent(makeDefTestDef())

	spec := def.DefaultSpec()
	spec.Values[0] = 99
	spec.Weights["a"] = 99

	fresh := def.DefaultSpec()
	if fresh.Values[0] != 1 {
		t.Errorf("mutating a returned spec's slice changed the default")
	}
	if fresh.Weights["a"] != 1.5 {
		t.Errorf("mutating a returned spec's map changed the default")
	}
}

func TestDefinitionIsIndependentOfInput(t *testing.T) {
	input := makeDefTestDef()
	def := modeling.DefineComponent(input)

	input.Ports[0].Name = "Mutated"
	input.DefaultSpec.Values[0] = 99

	if def.Ports()[0].Name != "Top" {
		t.Errorf("mutating the input def changed the definition's ports")
	}
	if def.DefaultSpec().Values[0] != 1 {
		t.Errorf("mutating the input def changed the definition's default spec")
	}
}

func TestDefinitionAccessorsReturnCopies(t *testing.T) {
	def := modeling.DefineComponent(makeDefTestDef())

	def.Ports()[0].Name = "Mutated"
	def.PortGroups()[0].Name = "Mutated"

	if def.Ports()[0].Name != "Top" || def.PortGroups()[0].Name != "Out" {
		t.Errorf("mutating an accessor's result changed the definition")
	}
}

func TestDefinitionDeclarePorts(t *testing.T) {
	def := modeling.DefineComponent(makeDefTestDef())

	po := messaging.NewPortOwnerBase()
	def.DeclarePorts(po)

	if roles := po.PortRoles("Top"); len(roles) != 1 ||
		roles[0] != defTestResponder {
		t.Errorf("PortRoles(Top) = %+v, want [responder]", roles)
	}

	if roles := po.PortRoles("Out"); len(roles) != 1 ||
		roles[0] != defTestRequester {
		t.Errorf("PortRoles(Out) = %+v, want [requester]", roles)
	}

	if n := po.NumPortsInGroup("Out"); n != 0 {
		t.Errorf("NumPortsInGroup(Out) = %d, want 0", n)
	}
}

// plainSpec is a minimal valid Spec for the panic tests below.
type plainSpec struct {
	N int `json:"n"`
}

// defineWith returns a DefineComponent thunk over plainSpec for panic tests.
func defineWith(name string, ports []modeling.PortDef,
	groups []modeling.PortGroupDef) func() {
	return func() {
		modeling.DefineComponent(
			modeling.ComponentDef[plainSpec, modeling.None]{
				Name:       name,
				Ports:      ports,
				PortGroups: groups,
			})
	}
}

func TestDefineComponentPanicsOnBadSpec(t *testing.T) {
	type pointerSpec struct {
		P *int `json:"p"`
	}
	type badTagSpec struct {
		N int `json:"n" akita:"bogus"`
	}
	type minOnStringSpec struct {
		S string `json:"s" akita:"min=1"`
	}
	// N is untagged, so its JSON name is the field name "N", colliding
	// with B's explicit tag.
	type dupJSONSpec struct {
		N int
		B int `json:"N"`
	}

	t.Run("empty name", func(t *testing.T) {
		mustPanicWith(t, "must have a name", defineWith("", nil, nil))
	})

	t.Run("invalid spec", func(t *testing.T) {
		mustPanicWith(t, "invalid default Spec", func() {
			modeling.DefineComponent(
				modeling.ComponentDef[pointerSpec, modeling.None]{Name: "C"})
		})
	})

	t.Run("bad akita tag", func(t *testing.T) {
		mustPanicWith(t, `unknown directive "bogus"`, func() {
			modeling.DefineComponent(
				modeling.ComponentDef[badTagSpec, modeling.None]{Name: "C"})
		})
	})

	t.Run("min on non-numeric field", func(t *testing.T) {
		mustPanicWith(t, "min/max apply only to numeric fields", func() {
			modeling.DefineComponent(
				modeling.ComponentDef[minOnStringSpec, modeling.None]{
					Name: "C"})
		})
	})

	t.Run("duplicate json name", func(t *testing.T) {
		mustPanicWith(t, `duplicate JSON name "N"`, func() {
			modeling.DefineComponent(
				modeling.ComponentDef[dupJSONSpec, modeling.None]{Name: "C"})
		})
	})
}

func TestDefineComponentPanicsOnBadPorts(t *testing.T) {
	t.Run("empty port name", func(t *testing.T) {
		mustPanicWith(t, "empty name",
			defineWith("C", []modeling.PortDef{{Name: ""}}, nil))
	})

	t.Run("duplicate port name", func(t *testing.T) {
		mustPanicWith(t, `port "Top" more than once`,
			defineWith("C",
				[]modeling.PortDef{{Name: "Top"}, {Name: "Top"}}, nil))
	})

	t.Run("port and group name collide", func(t *testing.T) {
		mustPanicWith(t, `port "Top" more than once`,
			defineWith("C",
				[]modeling.PortDef{{Name: "Top"}},
				[]modeling.PortGroupDef{{Name: "Top"}}))
	})

	t.Run("nil role", func(t *testing.T) {
		mustPanicWith(t, "nil role",
			defineWith("C",
				[]modeling.PortDef{
					{Name: "Top", Roles: []*messaging.Role{nil}}},
				nil))
	})
}

func TestDefineComponentPanicsOnBadPortGroups(t *testing.T) {
	type notDerivedCountSpec struct {
		NumOut int `json:"num_out"`
	}
	type nonIntCountSpec struct {
		NumOut float64 `json:"num_out" akita:"derived"`
	}

	t.Run("negative count bound", func(t *testing.T) {
		mustPanicWith(t, "negative count bound",
			defineWith("C", nil,
				[]modeling.PortGroupDef{{Name: "Out", MinCount: -1}}))
	})

	t.Run("max below min", func(t *testing.T) {
		mustPanicWith(t, "MaxCount 1 < MinCount 2",
			defineWith("C", nil,
				[]modeling.PortGroupDef{
					{Name: "Out", MinCount: 2, MaxCount: 1}}))
	})

	t.Run("count field missing", func(t *testing.T) {
		mustPanicWith(t, `CountField "num_out" does not match`,
			defineWith("C", nil,
				[]modeling.PortGroupDef{
					{Name: "Out", CountField: "num_out"}}))
	})

	t.Run("count field not integer", func(t *testing.T) {
		mustPanicWith(t, "must be an integer Spec field", func() {
			modeling.DefineComponent(
				modeling.ComponentDef[nonIntCountSpec, modeling.None]{
					Name: "C",
					PortGroups: []modeling.PortGroupDef{
						{Name: "Out", CountField: "num_out"}},
				})
		})
	})

	t.Run("count field not derived", func(t *testing.T) {
		mustPanicWith(t, "must be tagged", func() {
			modeling.DefineComponent(
				modeling.ComponentDef[notDerivedCountSpec, modeling.None]{
					Name: "C",
					PortGroups: []modeling.PortGroupDef{
						{Name: "Out", CountField: "num_out"}},
				})
		})
	})
}

// TestDefinitionWorksWithComponent exercises the intended builder usage:
// DeclarePorts on a built component, then assigning ports normally.
func TestDefinitionWorksWithComponent(t *testing.T) {
	def := modeling.DefineComponent(makeDefTestDef())

	engine := timing.NewSerialEngine()
	comp := modeling.NewBuilder[defTestSpec, TestState, defTestResources]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(def.DefaultSpec()).
		Build("Comp")

	def.DeclarePorts(comp)

	if roles := comp.PortRoles("Top"); len(roles) != 1 ||
		roles[0] != defTestResponder {
		t.Errorf("PortRoles(Top) = %+v, want [responder]", roles)
	}

	if roles := comp.PortRoles("Out"); len(roles) != 1 ||
		roles[0] != defTestRequester {
		t.Errorf("PortRoles(Out) = %+v, want [requester]", roles)
	}
}
