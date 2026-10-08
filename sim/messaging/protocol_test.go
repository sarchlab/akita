package messaging

import (
	"strings"
	"testing"
)

type protoTestReq struct {
}

type protoTestRsp struct {
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
		RoleDef{Name: "requester", Sends: []any{protoTestReq{}}},
		RoleDef{Name: "responder", Sends: []any{protoTestRsp{}}},
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
		RoleDef{Name: "only", Sends: []any{protoTestReq{}}})
	mustPanic(t, "already defined", func() {
		defineProtocol("test.duplicate",
			RoleDef{Name: "only", Sends: []any{protoTestReq{}}})
	})

	mustPanic(t, `role "dup" is already defined`, func() {
		defineProtocol("test.duprole",
			RoleDef{Name: "dup", Sends: []any{protoTestReq{}}},
			RoleDef{Name: "dup", Sends: []any{protoTestRsp{}}})
	})

	p := defineProtocol("test.unknownrole",
		RoleDef{Name: "only", Sends: []any{protoTestReq{}}})
	mustPanic(t, "does not define role", func() {
		p.Role("nonexistent")
	})
}

func TestMsgTypeMayBelongToTwoProtocols(t *testing.T) {
	defineProtocol("test.shared.a",
		RoleDef{Name: "only", Sends: []any{protoTestReq{}}})
	defineProtocol("test.shared.b",
		RoleDef{Name: "only", Sends: []any{protoTestReq{}}})
}

func TestMsgTypeMayBeSentByTwoRoles(t *testing.T) {
	p := defineProtocol("test.twosenders",
		RoleDef{Name: "a", Sends: []any{protoTestReq{}}},
		RoleDef{Name: "b", Sends: []any{protoTestReq{}, protoTestRsp{}}})

	if len(p.Role("a").Sends()) != 1 || len(p.Role("b").Sends()) != 2 {
		t.Errorf("roles send %v and %v", p.Role("a").Sends(), p.Role("b").Sends())
	}

	if len(p.Messages()) != 2 {
		t.Errorf("Messages() = %v, want each message type once", p.Messages())
	}
}

// AnyProtocol is messaging's own protocol, so it also shows that a protocol
// is named after the package that defines it, and that a package defines at
// most one.
func TestAnyProtocolIsNamedAfterMessaging(t *testing.T) {
	if AnyProtocol.Name() != "github.com/sarchlab/akita/v5/sim/messaging" {
		t.Errorf("Name() = %q, want the defining package's import path",
			AnyProtocol.Name())
	}

	if AnyRole.Name() != "any" || AnyRole.Protocol() != AnyProtocol ||
		len(AnyRole.Sends()) != 0 || len(AnyProtocol.Messages()) != 0 {
		t.Errorf("AnyRole = %q sending %v, want a role named any that lists no messages",
			AnyRole.Name(), AnyRole.Sends())
	}
}

func TestRoleNameMustFitInATag(t *testing.T) {
	for _, name := range []string{"", "a.b", "a,b", "a=b", "a/b", "a b"} {
		mustPanic(t, "role name", func() {
			defineProtocol("test.badrole."+name,
				RoleDef{Name: name, Sends: []any{protoTestReq{}}})
		})
	}
}

func TestPackageOfFunc(t *testing.T) {
	for funcName, want := range map[string]string{
		"github.com/sarchlab/akita/v5/mem/memprotocol.init":      "github.com/sarchlab/akita/v5/mem/memprotocol",
		"github.com/sarchlab/akita/v5/sim/messaging.TestX.func1": "github.com/sarchlab/akita/v5/sim/messaging",
		"gopkg.in/yaml%2ev3.init":                                "gopkg.in/yaml.v3",
		"main.init":                                              "main",
	} {
		if got := packageOfFunc(funcName); got != want {
			t.Errorf("packageOfFunc(%q) = %q, want %q", funcName, got, want)
		}
	}
}
