package inspect_test

import (
	"encoding/json"
	"flag"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/inspect"
	"github.com/sarchlab/akita/v5/inspect/schema"
)

var update = flag.Bool("update", false, "rewrite golden files")

const modulePath = "github.com/sarchlab/akita/v5"

// expectedErrors lists the fixture packages that must fail extraction and
// the message each failure must carry.
var expectedErrors = []struct {
	name      string
	pkgSubstr string
	msgSubstr string
}{
	{"duplicatejson", "fixtures/duplicatejson", "duplicate JSON"},
	{"pointerspec", "fixtures/pointerspec", "disallowed Spec"},
	{"pointertype", "fixtures/pointertype", "Spec must be a struct"},
	{"nonnumeric", "fixtures/nonnumeric", "only to numeric"},
	{"nonfinite", "fixtures/nonfinite", "non-finite"},
	{"non-constant default", "fixtures/nonconst",
		"not statically analyzable"},
	{"misnamed definition var", "fixtures/wrongname",
		`must be named "Definition"`},
	{"unkeyed literal", "fixtures/unkeyed", "must be keyed"},
	{"container field", "fixtures/containerfield",
		"Spec fields must be scalars"},
	{"computed definition", "fixtures/computed",
		"must be initialized with a composite literal"},
	{"pointer definition", "fixtures/pointerdef", "not a pointer"},
	{"container behind json dash", "fixtures/dashcontainer",
		"Spec fields must be scalars"},
	{"unexported-only spec", "fixtures/unexportedonly", "serializes as {}"},
	{"non-struct resources", "fixtures/badresources", "must be a struct"},
	{"omitzero-only spec", "fixtures/omitzeroonly", "serializes as {}"},
	{"container behind MarshalJSON", "fixtures/marshalercontainer",
		"Spec fields must be scalars"},
	{"ports field that is not a port", "fixtures/badport",
		"must be an exported messaging.Port"},
	{"undefined protocol role", "fixtures/unknownrole",
		`no protocol "github.com/sarchlab/akita/v5/mem/memprotocol" with role "owner"`},
	{"malformed role tag", "fixtures/badroletag",
		"want role=<protocol>.<role>"},
	{"missing NewMiddlewares", "fixtures/nomiddlewares",
		"must set NewMiddlewares"},
	{"NewState held in a var", "fixtures/funcvar",
		"NewState must name a function, not newState"},
	{"NewMiddlewares as a literal", "fixtures/funclit",
		"NewMiddlewares must name a function"},
	{"middleware without Handle", "fixtures/badmiddleware",
		"implement modeling.Middleware"},
}

// TestInspect loads all test subjects in one Inspect call (loading carries
// the whole dependency graph, so one load shared by all subtests keeps the
// test fast) and checks definitions and errors per package.
func TestInspect(t *testing.T) {
	defs, errs := inspect.Inspect(inspect.Options{}, fixturePatterns(t)...)

	byPkg := map[string]schema.Definition{}
	for _, d := range defs {
		byPkg[d.Package] = d
	}

	t.Run("escaped string choices", func(t *testing.T) {
		def := byPkg[fixturePath("scalars")]
		found := false
		for _, f := range def.Spec {
			if f.Name != "Choice" {
				continue
			}
			found = true
			if len(f.Choices) != 1 || f.Choices[0] != f.Default {
				t.Errorf("choices %q disagree with Go default %q", f.Choices, f.Default)
			}
		}
		if !found {
			t.Fatal("no Choice field extracted")
		}
	})

	t.Run("slices of scalars", func(t *testing.T) {
		checkSliceFields(t, byPkg)
	})

	t.Run("rob matches golden", func(t *testing.T) {
		checkGolden(t, byPkg, modulePath+"/mem/rob", "rob.golden.json")
	})

	t.Run("fullcomp matches golden", func(t *testing.T) {
		checkGolden(t, byPkg, fixturePath("fullcomp"), "fullcomp.golden.json")
	})

	t.Run("protocol declared in the component's package", func(t *testing.T) {
		checkLocalProto(t, byPkg)
	})

	checkDeclarationForms(t, byPkg)

	t.Run("package without definition is skipped", func(t *testing.T) {
		if _, ok := byPkg[modulePath+"/sim/timing"]; ok {
			t.Errorf("timing has no definition but one was extracted")
		}
	})

	for _, c := range expectedErrors {
		t.Run(c.name+" is an error", func(t *testing.T) {
			checkError(t, errs, c.pkgSubstr, c.msgSubstr)
		})
	}

	t.Run("no unexpected errors", func(t *testing.T) {
		checkNoUnexpectedErrors(t, errs)
	})
}

// checkDeclarationForms covers definitions written in less common but valid
// forms: through type aliases, with untyped ports, and with no ports.
func checkDeclarationForms(t *testing.T, byPkg map[string]schema.Definition) {
	t.Helper()

	t.Run("definitions declared through aliases", func(t *testing.T) {
		for pkg, depth := range map[string]int64{
			"aliasdef":        8,
			"genericaliasdef": 4,
		} {
			def, ok := byPkg[fixturePath(pkg)]
			if !ok || def.Name != pkg || def.Model != schema.ModelTicking ||
				len(def.Spec) != 1 || def.Spec[0].Default != depth {
				t.Errorf("%s: got %+v, want a ticking definition with Depth %d",
					pkg, def, depth)
			}
		}
	})

	t.Run("untagged ports are untyped", func(t *testing.T) {
		want := []schema.Port{
			{Name: "Untyped"},
			{Name: "UntypedGroup", Group: true},
		}

		got := byPkg[fixturePath("scalars")].Ports
		if len(got) != len(want) {
			t.Fatalf("Ports = %+v, want %+v", got, want)
		}

		for i := range want {
			if got[i].Name != want[i].Name || got[i].Group != want[i].Group ||
				got[i].Roles != nil {
				t.Errorf("port %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("no ports, resources, or middlewares", func(t *testing.T) {
		def, ok := byPkg[fixturePath("zerodefaults")]
		if !ok || len(def.Ports) != 0 || len(def.Resources) != 0 ||
			len(def.Middlewares) != 0 {
			t.Errorf("zerodefaults: got %+v, want a definition without "+
				"ports, resources, or middlewares", def)
		}
	})

	t.Run("invalid tag name falls back to the field name", func(t *testing.T) {
		for _, f := range byPkg[fixturePath("scalars")].Spec {
			if f.Name == "Quoted" && f.JSONName != "Quoted" {
				t.Errorf("Quoted JSON name = %q, want Quoted", f.JSONName)
			}
		}
	})
}

func fixturePath(name string) string {
	return modulePath + "/inspect/testdata/fixtures/" + name
}

// checkSliceFields checks the scalars fixture's slice fields: one with a
// default slice literal, and one the DefaultSpec literal leaves out.
func checkSliceFields(t *testing.T, byPkg map[string]schema.Definition) {
	t.Helper()

	want := map[string]schema.Field{
		"Lanes":   {Type: "[]int", Default: []any{int64(1), int64(2)}},
		"Targets": {Type: "[]github.com/sarchlab/akita/v5/sim/messaging.RemotePort", Default: []any{}},
	}

	for _, f := range byPkg[fixturePath("scalars")].Spec {
		w, ok := want[f.Name]
		if !ok {
			continue
		}

		delete(want, f.Name)

		if f.Type != w.Type || !reflect.DeepEqual(f.Default, w.Default) {
			t.Errorf("%s: type %q default %#v, want type %q default %#v",
				f.Name, f.Type, f.Default, w.Type, w.Default)
		}
	}

	if len(want) != 0 {
		t.Errorf("fields not extracted: %v", want)
	}
}

func checkLocalProto(t *testing.T, byPkg map[string]schema.Definition) {
	t.Helper()

	def, ok := byPkg[fixturePath("localproto")]
	if !ok {
		t.Fatalf("no definition extracted for the localproto fixture")
	}

	if len(def.Ports) != 3 {
		t.Fatalf("Ports = %+v, want 3 ports", def.Ports)
	}

	want := map[string]schema.Role{
		"In":   {Protocol: fixturePath("localproto"), Role: "consumer"},
		"Feed": {Protocol: fixturePath("localproto"), Role: "producer"},
		"Sink": {Protocol: modulePath + "/sim/messaging", Role: "any"},
	}

	for _, port := range def.Ports {
		if len(port.Roles) != 1 || port.Roles[0] != want[port.Name] {
			t.Errorf("port %s roles = %+v, want %+v",
				port.Name, port.Roles, want[port.Name])
		}
	}
}

func checkNoUnexpectedErrors(t *testing.T, errs []error) {
	t.Helper()

	for _, err := range errs {
		expected := false
		for _, c := range expectedErrors {
			if strings.Contains(err.Error(), c.pkgSubstr) {
				expected = true
				break
			}
		}
		if !expected {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func checkGolden(
	t *testing.T, byPkg map[string]schema.Definition,
	pkgPath, goldenName string,
) {
	t.Helper()

	def, ok := byPkg[pkgPath]
	if !ok {
		t.Fatalf("no definition extracted for %s", pkgPath)
	}

	got, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		t.Fatalf("marshaling definition: %v", err)
	}
	got = append(got, '\n')

	goldenPath := filepath.Join("testdata", goldenName)

	if *update {
		if err := os.WriteFile(goldenPath, got, 0600); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden file (run with -update to create): %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("schema differs from %s\ngot:\n%s\nwant:\n%s",
			goldenPath, got, want)
	}
}

func checkError(t *testing.T, errs []error, pkgSubstr, msgSubstr string) {
	t.Helper()

	for _, err := range errs {
		msg := err.Error()
		if strings.Contains(msg, pkgSubstr) &&
			strings.Contains(msg, msgSubstr) {
			return
		}
	}

	t.Errorf("no error for %s containing %q; errors: %v",
		pkgSubstr, msgSubstr, errs)
}

// TestModuleDiscovery inspects the whole module. Instead of pinning the
// number of definitions, which changes with every new component, it checks
// that the inspector extracts a definition from exactly the packages whose
// source declares a package-level Definition var, so a definition it skips
// or cannot read still fails the test.
func TestModuleDiscovery(t *testing.T) {
	defs, errs := inspect.Inspect(inspect.Options{Dir: ".."}, "./...")
	if len(errs) != 0 {
		t.Fatalf("module inspection failed: %v", errs)
	}

	models := map[string]bool{
		schema.ModelTicking: true, schema.ModelWakeup: true, schema.ModelEvent: true,
	}

	got := make([]string, 0, len(defs))
	for _, def := range defs {
		if def.Kind != schema.KindComponent || !models[def.Model] {
			t.Errorf("%s: kind %q, model %q; want a component model",
				def.Package, def.Kind, def.Model)
		}
		got = append(got, strings.TrimPrefix(def.Package, modulePath+"/"))
	}

	want := definitionDirs(t, "..")
	if !slices.Equal(got, want) {
		t.Errorf("discovered definitions differ from the declared ones\n"+
			"got:  %v\nwant: %v", got, want)
	}

	if !slices.Contains(got, "mem/rob") {
		t.Errorf("mem/rob, the golden-tested component, was not discovered")
	}

	t.Logf("module inspection: %d definitions, zero errors", len(defs))
}

// definitionDirs lists, sorted and relative to root, the package directories
// in which a non-test Go file declares a package-level var named Definition.
// It skips what "./..." skips: testdata, directories starting with "." or
// "_", nested modules, and files excluded by build constraints.
func definitionDirs(t *testing.T, root string) []string {
	t.Helper()

	ctx := build.Default
	ctx.CgoEnabled = false

	found := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path != root && skipDir(path, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		dir := filepath.Dir(path)
		if match, err := ctx.MatchFile(dir, name); err != nil || !match {
			return err
		}

		if declaresDefinition(t, path) {
			rel, err := filepath.Rel(root, dir)
			if err != nil {
				return err
			}
			found[filepath.ToSlash(rel)] = true
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}

	return slices.Sorted(maps.Keys(found))
}

func skipDir(path, name string) bool {
	if name == "testdata" || strings.HasPrefix(name, ".") ||
		strings.HasPrefix(name, "_") {
		return true
	}

	_, err := os.Stat(filepath.Join(path, "go.mod"))

	return err == nil
}

// declaresDefinition reports whether the Go file declares a package-level var
// named Definition.
func declaresDefinition(t *testing.T, path string) bool {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil,
		parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}

		for _, spec := range gen.Specs {
			for _, name := range spec.(*ast.ValueSpec).Names {
				if name.Name == "Definition" {
					return true
				}
			}
		}
	}

	return false
}

func fixturePatterns(t *testing.T) []string {
	t.Helper()

	patterns := []string{modulePath + "/mem/rob", modulePath + "/sim/timing"}
	fixtures, err := os.ReadDir("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		if fixture.IsDir() {
			patterns = append(patterns, "./testdata/fixtures/"+fixture.Name())
		}
	}
	return patterns
}
