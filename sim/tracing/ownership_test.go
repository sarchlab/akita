package tracing

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Protect the architectural boundary: mutable correlation belongs to an
// observer instance, never package-level simulation state.
func TestNoPackageLevelTracingMaps(t *testing.T) {
	files, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range files {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			vars, ok := decl.(*ast.GenDecl)
			if !ok || vars.Tok != token.VAR {
				continue
			}
			ast.Inspect(vars, func(node ast.Node) bool {
				if _, ok := node.(*ast.MapType); ok {
					t.Errorf("package-level tracing map in %s", entry.Name())
				}
				return true
			})
		}
	}
}
