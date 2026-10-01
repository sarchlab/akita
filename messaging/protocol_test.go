package messaging

import (
	"strings"
	"testing"
)

type protoTestReq struct {
	MsgMeta
}

type protoTestRsp struct {
	MsgMeta
}

func mustPanic(t *testing.T, substr string, f func()) {
	t.Helper()

	defer func() {
		t.Helper()

		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q, got none", substr)
		}

		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}

		if !strings.Contains(msg, substr) {
			t.Fatalf("panic %q does not contain %q", msg, substr)
		}
	}()

	f()
}

func TestDefineProtocol(t *testing.T) {
	p := defineProtocol("test.protocol",
		RoleDef{Name: "requester", Sends: []Msg{protoTestReq{}}},
		RoleDef{Name: "responder", Sends: []Msg{protoTestRsp{}}},
	)

	if p.Name() != "test.protocol" {
		t.Errorf("Name() = %q", p.Name())
	}

	requester := p.Role("requester")
	if requester.Protocol() != p || requester.Name() != "requester" {
		t.Errorf("requester role not resolved correctly")
	}

	if len(requester.Sends()) != 1 {
		t.Errorf("requester.Sends() = %v", requester.Sends())
	}

	if len(p.Messages()) != 2 {
		t.Errorf("Messages() = %v", p.Messages())
	}

	// Defining the protocol must have registered its messages with the codec.
	if err := msgCodec.CheckRoundTrip(protoTestReq{}); err != nil {
		t.Errorf("protoTestReq not registered: %v", err)
	}

	if err := msgCodec.CheckRoundTrip(protoTestRsp{}); err != nil {
		t.Errorf("protoTestRsp not registered: %v", err)
	}
}

func TestDefineProtocolPanics(t *testing.T) {
	mustPanic(t, "must not be empty", func() {
		defineProtocol("")
	})

	mustPanic(t, "at least one role", func() {
		defineProtocol("test.noroles")
	})

	defineProtocol("test.duplicate",
		RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})
	mustPanic(t, "already defined", func() {
		defineProtocol("test.duplicate",
			RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})
	})

	mustPanic(t, `role "dup" is already defined`, func() {
		defineProtocol("test.duprole",
			RoleDef{Name: "dup", Sends: []Msg{protoTestReq{}}},
			RoleDef{Name: "dup", Sends: []Msg{protoTestRsp{}}})
	})

	mustPanic(t, "exactly one role", func() {
		defineProtocol("test.twosenders",
			RoleDef{Name: "a", Sends: []Msg{protoTestReq{}}},
			RoleDef{Name: "b", Sends: []Msg{protoTestReq{}}})
	})

	p := defineProtocol("test.unknownrole",
		RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})
	mustPanic(t, "does not define role", func() {
		p.Role("nonexistent")
	})
}

func TestMsgTypeMayBelongToTwoProtocols(t *testing.T) {
	defineProtocol("test.shared.a",
		RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})
	defineProtocol("test.shared.b",
		RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})
}

func TestDefineProtocolNamesItAfterItsPackage(t *testing.T) {
	p := DefineProtocol(RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})

	if p.Name() != "github.com/sarchlab/akita/v5/messaging" {
		t.Errorf("Name() = %q, want the defining package's import path", p.Name())
	}

	mustPanic(t, "at most one protocol", func() {
		DefineProtocol(RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})
	})
}

func TestRoleNameMustFitInATag(t *testing.T) {
	for _, name := range []string{"", "a.b", "a,b", "a=b", "a/b", "a b"} {
		mustPanic(t, "role name", func() {
			defineProtocol("test.badrole."+name,
				RoleDef{Name: name, Sends: []Msg{protoTestReq{}}})
		})
	}
}

func TestPackageOfFunc(t *testing.T) {
	for funcName, want := range map[string]string{
		"github.com/sarchlab/akita/v5/mem/memprotocol.init":  "github.com/sarchlab/akita/v5/mem/memprotocol",
		"github.com/sarchlab/akita/v5/messaging.TestX.func1": "github.com/sarchlab/akita/v5/messaging",
		"gopkg.in/yaml%2ev3.init":                            "gopkg.in/yaml.v3",
		"main.init":                                          "main",
	} {
		if got := packageOfFunc(funcName); got != want {
			t.Errorf("packageOfFunc(%q) = %q, want %q", funcName, got, want)
		}
	}
}
