package messaging_test

import (
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/sarchlab/akita/v5/messaging"

	// The audit verifies registration against the live codec registry, so the
	// packages that define message types must have had their protocol
	// definitions run. The list is self-enforcing: a message type defined in a
	// package that is not imported here fails the registry check below with a
	// message saying to add the import.
	_ "github.com/sarchlab/akita/v5/examples/ping"
	_ "github.com/sarchlab/akita/v5/examples/tickingping"
	_ "github.com/sarchlab/akita/v5/mem/control"
	_ "github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	_ "github.com/sarchlab/akita/v5/mem/memprotocol"
	_ "github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	_ "github.com/sarchlab/akita/v5/noc/acceptance"
	_ "github.com/sarchlab/akita/v5/noc/packetization"
)

const modulePath = "github.com/sarchlab/akita/v5"

// todoUnregistered lists message types known to be missing a protocol, keyed
// by wire tag. Entries here keep the audit green while migration is in
// progress and must be burned down to zero. Do not add new entries for new
// code — define a protocol instead.
var todoUnregistered = map[string]string{}

// intentionallyUnregistered lists message types that can never appear in a
// checkpointed port buffer, keyed by wire tag, with the reason. Unlike
// todoUnregistered, these entries are permanent and each needs a strong
// justification.
var intentionallyUnregistered = map[string]string{
	modulePath + "/messaging.MsgMeta": "the message envelope every message " +
		"embeds; it belongs to no protocol and is not itself wire traffic",
	modulePath + "/noc/networking/switching/switches.routedFlit": "internal " +
		"switch pipeline state held in typed buffers and serialized " +
		"concretely inside State; never sent through a Port, so the codec " +
		"never sees it (it implements Msg only by embedding Flit)",
}

// TestEveryMsgTypeIsRegistered is the registration-coverage audit: every
// concrete type in the module that implements messaging.Msg must be
// registered with the checkpoint codec (normally by belonging to a protocol
// defined with DefineProtocol). A type that is not registered would make
// LoadCheckpoint fail with "unknown message type" whenever a checkpoint
// happens to capture it in a port buffer — a latent bug this test turns into
// a CI failure.
//
// Scope: non-test files of importable packages. Test-file types live only
// inside one test binary. Package main (the examples) cannot be imported by
// this test, so its types are reported as skipped; examples still define
// protocols by convention.
func TestEveryMsgTypeIsRegistered(t *testing.T) {
	if testing.Short() {
		t.Skip("loads and type-checks the whole module")
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedDeps,
		Dir:  "..",
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("loading module packages: %v", err)
	}

	if n := packages.PrintErrors(pkgs); n > 0 {
		t.Fatalf("%d packages failed to load", n)
	}

	msgIface := lookupMsgInterface(t, pkgs)

	registered := map[string]bool{}
	for _, tag := range messaging.RegisteredMsgTags() {
		registered[tag] = true
	}

	foundMsgTypes := 0

	for _, pkg := range pkgs {
		if pkg.Types.Name() == "main" {
			continue
		}

		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			named, ok := namedConcreteType(scope.Lookup(name))
			if !ok {
				continue
			}

			if !types.Implements(named, msgIface) &&
				!types.Implements(types.NewPointer(named), msgIface) {
				continue
			}

			foundMsgTypes++
			auditMsgType(t, pkg.PkgPath+"."+name, registered)
		}
	}

	// Guard against the audit passing vacuously because the loader or the
	// interface lookup silently found nothing.
	if foundMsgTypes < 10 {
		t.Fatalf("audit found only %d message types in the module; "+
			"the package loader or interface lookup is broken", foundMsgTypes)
	}
}

// auditMsgType checks one concrete message type's registration state. The tag
// may be registered in value or pointer form.
func auditMsgType(t *testing.T, tag string, registered map[string]bool) {
	t.Helper()

	if registered[tag] || registered["*"+tag] {
		return
	}

	if reason, ok := todoUnregistered[tag]; ok {
		t.Logf("TODO: %s is not registered (%s)", tag, reason)
		return
	}

	if _, ok := intentionallyUnregistered[tag]; ok {
		return
	}

	t.Errorf("message type %s is not registered with the checkpoint codec: "+
		"add it to a protocol (messaging.DefineProtocol) in its package, and "+
		"make sure that package is blank-imported by this audit", tag)
}

// lookupMsgInterface finds the messaging.Msg interface in the loaded packages.
func lookupMsgInterface(
	t *testing.T,
	pkgs []*packages.Package,
) *types.Interface {
	t.Helper()

	for _, pkg := range pkgs {
		if pkg.PkgPath != modulePath+"/messaging" {
			continue
		}

		obj := pkg.Types.Scope().Lookup("Msg")
		if obj == nil {
			break
		}

		iface, ok := obj.Type().Underlying().(*types.Interface)
		if !ok {
			break
		}

		return iface
	}

	t.Fatal("could not find the messaging.Msg interface in the loaded module")
	return nil
}

// namedConcreteType returns the named type for an object that defines a
// concrete (non-interface, non-alias) named type.
func namedConcreteType(obj types.Object) (*types.Named, bool) {
	tn, ok := obj.(*types.TypeName)
	if !ok || tn.IsAlias() {
		return nil, false
	}

	named, ok := tn.Type().(*types.Named)
	if !ok {
		return nil, false
	}

	if _, isIface := named.Underlying().(*types.Interface); isIface {
		return nil, false
	}

	return named, true
}

// TestMainPackagesAreOutsideAuditScope documents which package-main message
// types the runtime audit cannot cover (a test cannot import package main).
// Examples define protocols by convention; this test lists them so the gap is
// visible rather than silent.
func TestMainPackagesAreOutsideAuditScope(t *testing.T) {
	if testing.Short() {
		t.Skip("loads and type-checks the whole module")
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedDeps,
		Dir:  "..",
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("loading module packages: %v", err)
	}

	msgIface := lookupMsgInterface(t, pkgs)

	for _, pkg := range pkgs {
		if pkg.Types == nil || pkg.Types.Name() != "main" {
			continue
		}

		var msgTypes []string

		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			named, ok := namedConcreteType(scope.Lookup(name))
			if !ok {
				continue
			}

			if types.Implements(named, msgIface) ||
				types.Implements(types.NewPointer(named), msgIface) {
				msgTypes = append(msgTypes, name)
			}
		}

		if len(msgTypes) > 0 {
			t.Logf("package main %s defines message types outside audit "+
				"scope: %s", pkg.PkgPath, strings.Join(msgTypes, ", "))
		}
	}
}
