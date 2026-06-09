package messaging

import (
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/internal/codec"
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
	p := DefineProtocol("test.protocol",
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
		DefineProtocol("")
	})

	mustPanic(t, "at least one role", func() {
		DefineProtocol("test.noroles")
	})

	DefineProtocol("test.duplicate",
		RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})
	mustPanic(t, "already defined", func() {
		DefineProtocol("test.duplicate",
			RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})
	})

	mustPanic(t, `role "dup" is already defined`, func() {
		DefineProtocol("test.duprole",
			RoleDef{Name: "dup", Sends: []Msg{protoTestReq{}}},
			RoleDef{Name: "dup", Sends: []Msg{protoTestRsp{}}})
	})

	mustPanic(t, "exactly one role", func() {
		DefineProtocol("test.twosenders",
			RoleDef{Name: "a", Sends: []Msg{protoTestReq{}}},
			RoleDef{Name: "b", Sends: []Msg{protoTestReq{}}})
	})

	mustPanic(t, "envelope", func() {
		DefineProtocol("test.baremeta",
			RoleDef{Name: "only", Sends: []Msg{MsgMeta{}}})
	})

	p := DefineProtocol("test.unknownrole",
		RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})
	mustPanic(t, "does not define role", func() {
		p.Role("nonexistent")
	})
}

func TestMsgTypeMayBelongToTwoProtocols(t *testing.T) {
	DefineProtocol("test.shared.a",
		RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})
	DefineProtocol("test.shared.b",
		RoleDef{Name: "only", Sends: []Msg{protoTestReq{}}})
}

func TestBareMsgMetaIsRejected(t *testing.T) {
	mustPanic(t, "envelope", func() {
		RegisterMsg(MsgMeta{})
	})

	mustPanic(t, "envelope", func() {
		RegisterMsg(&MsgMeta{})
	})

	port := NewPort(nil, 1, 1, "BanTestPort")

	mustPanic(t, "envelope", func() {
		port.Send(MsgMeta{Src: "BanTestPort", Dst: "Elsewhere"})
	})

	mustPanic(t, "envelope", func() {
		port.Deliver(MsgMeta{Src: "Elsewhere", Dst: "BanTestPort"})
	})
}

func TestMsgMetaIsNotRegistered(t *testing.T) {
	bareTag := codec.Tag(MsgMeta{})
	for _, tag := range msgCodec.Tags() {
		if tag == bareTag {
			t.Fatalf("bare MsgMeta is registered with the codec")
		}
	}
}

func TestPortRoles(t *testing.T) {
	p := DefineProtocol("test.portroles",
		RoleDef{Name: "requester", Sends: []Msg{protoTestReq{}}},
		RoleDef{Name: "responder", Sends: []Msg{protoTestRsp{}}},
	)

	po := NewPortOwnerBase()
	po.DeclarePort("Top", p.Role("responder"))
	po.DeclarePort("Legacy")
	po.DeclarePortGroup("Link", p.Role("requester"))

	topRoles := po.PortRoles("Top")
	if len(topRoles) != 1 || topRoles[0] != p.Role("responder") {
		t.Errorf("PortRoles(Top) = %v", topRoles)
	}

	if roles := po.PortRoles("Legacy"); roles != nil {
		t.Errorf("PortRoles(Legacy) = %v, want nil", roles)
	}

	linkRoles := po.PortRoles("Link")
	if len(linkRoles) != 1 || linkRoles[0] != p.Role("requester") {
		t.Errorf("PortRoles(Link) = %v", linkRoles)
	}

	mustPanic(t, "not declared", func() {
		po.PortRoles("Nonexistent")
	})
}
