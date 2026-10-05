package messaging_test

import (
	"go/ast"
	"go/types"
	"strings"
	"testing"

	_ "github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	_ "github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	_ "github.com/sarchlab/akita/v5/mem/memprotocol"
	_ "github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	_ "github.com/sarchlab/akita/v5/noc/acceptance"
	_ "github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"golang.org/x/tools/go/packages"
)

const modulePath = "github.com/sarchlab/akita/v5"

// TestEveryPayloadTypeIsRegistered checks the concrete payloads constructed in
// library message literals against the live registry. Dynamic payloads are
// guarded by Port.Send; examples and package main cannot be imported here.
func TestEveryPayloadTypeIsRegistered(t *testing.T) {
	if testing.Short() {
		t.Skip("loads and type-checks the whole module")
	}
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedDeps | packages.NeedSyntax | packages.NeedTypesInfo,
		Dir:  "../..",
	}, "./...")
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	for _, tag := range messaging.RegisteredMsgTags() {
		registered[tag] = true
	}
	found := map[string]bool{}
	for _, pkg := range pkgs {
		if strings.Contains(pkg.PkgPath, "/doc-site/") {
			continue
		}
		for _, err := range pkg.Errors {
			t.Errorf("%s: %v", pkg.PkgPath, err)
		}
		if pkg.Types == nil || pkg.Types.Name() == "main" || strings.HasPrefix(pkg.PkgPath, modulePath+"/examples/") {
			continue
		}
		for _, tag := range constructedPayloadTags(pkg) {
			found[tag] = true
			if !registered[tag] {
				t.Errorf("payload %s is not registered; define its protocol and import it in this audit", tag)
			}
		}
	}
	if len(found) < 10 {
		t.Fatalf("audit found only %d payload types; expected at least 10", len(found))
	}
}

func constructedPayloadTags(pkg *packages.Package) []string {
	var tags []string
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			named, ok := pkg.TypesInfo.TypeOf(lit).(*types.Named)
			if !ok || named.Obj().Pkg() == nil {
				return true
			}
			if named.Obj().Pkg().Path() != modulePath+"/simulation/messaging" || named.Obj().Name() != "Msg" {
				return true
			}
			for _, field := range lit.Elts {
				if tag := payloadFieldTag(pkg, field); tag != "" {
					tags = append(tags, tag)
				}
			}
			return true
		})
	}
	return tags
}

func payloadFieldTag(pkg *packages.Package, field ast.Expr) string {
	kv, ok := field.(*ast.KeyValueExpr)
	if !ok {
		return ""
	}
	key, ok := kv.Key.(*ast.Ident)
	if !ok || key.Name != "Payload" {
		return ""
	}
	payload, ok := pkg.TypesInfo.TypeOf(kv.Value).(*types.Named)
	if !ok || payload.Obj().Pkg() == nil {
		return ""
	}
	if _, ok := payload.Underlying().(*types.Interface); ok {
		return ""
	}
	return payload.Obj().Pkg().Path() + "." + payload.Obj().Name()
}
