package messaging

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"unicode"

	"github.com/sarchlab/akita/v5/internal/valuecheck"
)

// A Protocol is an immutable set of payload types that travel over a port,
// organized into roles. It is named after the package that defines it. Defining a protocol with DefineProtocol
// registers every message type it carries with the checkpoint codec, so a
// message type that belongs to a protocol can always be decoded when a
// checkpoint captures it in a port buffer.
type Protocol struct {
	name  string
	roles []*Role
}

// Name returns the protocol's name: the import path of the package that
// defined it, such as "github.com/sarchlab/akita/v5/mem/memprotocol".
func (p *Protocol) Name() string {
	return p.name
}

// Role returns the role with the given name. It panics if the protocol does
// not define the role.
func (p *Protocol) Role(name string) *Role {
	for _, r := range p.roles {
		if r.name == name {
			return r
		}
	}

	panic(fmt.Sprintf(
		"protocol %q does not define role %q", p.name, name))
}

// Messages returns payload prototypes from the union of all roles' sends. Each
// type appears once, although more than one role may send it.
func (p *Protocol) Messages() []any {
	seen := map[reflect.Type]bool{}
	msgs := make([]any, 0, len(p.roles)*2)

	for _, r := range p.roles {
		for _, msg := range r.sends {
			t := reflect.TypeOf(msg)
			if !seen[t] {
				seen[t] = true
				msgs = append(msgs, msg)
			}
		}
	}

	return msgs
}

// A Role is one endpoint's view of a protocol: the messages it sends. What a
// role receives is whatever the protocol's other roles send. A component tags
// each port with the role(s) it speaks, `akita:"role=<protocol>.<role>"`,
// where <protocol> is the protocol's name, for example
// `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`.
type Role struct {
	protocol *Protocol
	name     string
	sends    []any
}

// Protocol returns the protocol this role belongs to.
func (r *Role) Protocol() *Protocol {
	return r.protocol
}

// Name returns the role's name.
func (r *Role) Name() string {
	return r.name
}

// Sends returns prototypes of the payload types this role sends.
func (r *Role) Sends() []any {
	sends := make([]any, len(r.sends))
	copy(sends, r.sends)

	return sends
}

// RoleDef declares a role and the messages it sends, as input to
// DefineProtocol.
type RoleDef struct {
	Name  string
	Sends []any
}

// protocolNames tracks every defined protocol name so a duplicate definition
// fails loudly at init time.
var (
	protocolNamesMu sync.Mutex
	protocolNames   = map[string]bool{}
)

// DefineProtocol creates a protocol and registers every message type across
// all roles with the checkpoint codec. The protocol is named after the package
// that calls DefineProtocol: its import path. Call it directly, as a
// package-level var in the package that defines the message types:
//
//	var (
//	    Protocol  = messaging.DefineProtocol(
//	        messaging.RoleDef{Name: "requester",
//	            Sends: []any{ReadReq{}, WriteReq{}}},
//	        messaging.RoleDef{Name: "responder",
//	            Sends: []any{DataReadyRsp{}, WriteDoneRsp{}}},
//	    )
//	    Requester = Protocol.Role("requester")
//	    Responder = Protocol.Role("responder")
//	)
//
// A package defines at most one protocol. A role name is made of letters,
// digits, '_', and '-'. DefineProtocol panics on a second protocol in the same
// package or an invalid or duplicate role name. A message type may be sent by
// more than one role, and may belong to more than one protocol;
// re-registration with the codec is harmless. Payloads must be value types
// without pointers, interfaces, channels, functions, or unsafe pointers at any
// depth. Nested Msg values are allowed. Calling DefineProtocol after package
// initialization panics: the payload registry is immutable during simulation.
func DefineProtocol(roles ...RoleDef) *Protocol {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	initializing := false
	for {
		frame, more := frames.Next()
		if frame.Function == "runtime.doInit1" {
			initializing = true
			break
		}
		if !more {
			break
		}
	}
	if !initializing {
		panic("messaging: DefineProtocol must run during package initialization")
	}
	return defineProtocol(callerPackage(), roles...)
}

// callerPackage returns the import path of the package whose code called the
// function that calls callerPackage.
func callerPackage() string {
	pc := make([]uintptr, 1)
	if runtime.Callers(3, pc) == 0 {
		panic("messaging: cannot find the package that defines the protocol")
	}

	frame, _ := runtime.CallersFrames(pc).Next()

	return packageOfFunc(frame.Function)
}

// packageOfFunc returns the import path in a fully qualified function name,
// such as "github.com/x/y.init" or "gopkg.in/yaml%2ev3.init". A dot in the
// last path element is escaped as %2e in function names.
func packageOfFunc(funcName string) string {
	lastSlash := strings.LastIndex(funcName, "/")
	dot := strings.Index(funcName[lastSlash+1:], ".")
	if dot < 0 {
		return strings.ReplaceAll(funcName, "%2e", ".")
	}

	return strings.ReplaceAll(funcName[:lastSlash+1+dot], "%2e", ".")
}

// defineProtocol creates the protocol with the given name. DefineProtocol
// names it after the calling package; tests name it directly.
func defineProtocol(name string, roles ...RoleDef) *Protocol {
	if name == "" {
		panic("protocol name must not be empty")
	}

	if len(roles) == 0 {
		panic(fmt.Sprintf("protocol %q must define at least one role", name))
	}

	protocolNamesMu.Lock()
	if protocolNames[name] {
		protocolNamesMu.Unlock()
		panic(fmt.Sprintf("protocol %q is already defined; "+
			"a package defines at most one protocol", name))
	}
	protocolNames[name] = true
	protocolNamesMu.Unlock()

	p := &Protocol{name: name}
	seenRoles := map[string]bool{}

	for _, def := range roles {
		if !validRoleName(def.Name) {
			panic(fmt.Sprintf(
				"protocol %q: role name %q must be non-empty and use only "+
					"letters, digits, '_', and '-'", name, def.Name))
		}

		if seenRoles[def.Name] {
			panic(fmt.Sprintf(
				"protocol %q: role %q is already defined", name, def.Name))
		}
		seenRoles[def.Name] = true

		for _, msg := range def.Sends {
			if err := valuecheck.Payload(reflect.TypeOf(msg), reflect.TypeOf(Msg{})); err != nil {
				panic(fmt.Sprintf("protocol %q: payload %T: %v", name, msg, err))
			}
			msgCodec.Register(msg)
		}

		sends := make([]any, len(def.Sends))
		copy(sends, def.Sends)

		p.roles = append(p.roles, &Role{
			protocol: p,
			name:     def.Name,
			sends:    sends,
		})
	}

	return p
}

// validRoleName reports whether name can be a role name. A port tag names a
// role as "<protocol>.<role>" in a comma-separated list, so a role name must
// not contain '.', ',', or '='.
func validRoleName(name string) bool {
	if name == "" {
		return false
	}

	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return false
		}
	}

	return true
}
