package inspect_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/inspect"
	"github.com/sarchlab/akita/v5/inspect/schema"
)

var update = flag.Bool("update", false, "rewrite golden files")

// expectedErrors lists the fixture packages that must fail extraction and
// the message each failure must carry.
var expectedErrors = []struct {
	name      string
	pkgSubstr string
	msgSubstr string
}{
	{"emptyname", "fixtures/emptyname", "must have a name"},
	{"emptyport", "fixtures/emptyport", "empty name"},
	{"duplicateport", "fixtures/duplicateport", "more than once"},
	{"duplicateboundary", "fixtures/duplicateboundary", "more than once"},
	{"emptygroup", "fixtures/emptygroup", "empty name"},
	{"duplicatejson", "fixtures/duplicatejson", "duplicate JSON"},
	{"pointerspec", "fixtures/pointerspec", "disallowed Spec"},
	{"pointertype", "fixtures/pointertype", "must be a struct"},
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
	{"non-struct resources", "fixtures/badresources",
		"must take a Resources struct"},
	{"omitzero-only spec", "fixtures/omitzeroonly", "serializes as {}"},
	{"container behind MarshalJSON", "fixtures/marshalercontainer",
		"Spec fields must be scalars"},
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
		def := byPkg["github.com/sarchlab/akita/v5/inspect/testdata/fixtures/scalars"]
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

	t.Run("rob matches golden", func(t *testing.T) {
		checkGolden(t, byPkg, "github.com/sarchlab/akita/v5/mem/rob",
			"rob.golden.json")
	})

	t.Run("fullcomp matches golden", func(t *testing.T) {
		checkGolden(t, byPkg,
			"github.com/sarchlab/akita/v5/inspect/testdata/fixtures/fullcomp",
			"fullcomp.golden.json")
	})

	t.Run("same-package protocol and bare role identifiers",
		func(t *testing.T) {
			checkLocalProto(t, byPkg)
		})

	checkDeclarationForms(t, byPkg)

	t.Run("package without definition is skipped", func(t *testing.T) {
		if _, ok := byPkg["github.com/sarchlab/akita/v5/timing"]; ok {
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

// checkDeclarationForms covers definitions and resources written in less
// common but valid forms: through type aliases, with nil roles, and behind
// a pointer WithResources parameter.
func checkDeclarationForms(t *testing.T, byPkg map[string]schema.Definition) {
	t.Helper()

	t.Run("definitions declared through aliases", func(t *testing.T) {
		for pkg, name := range map[string]string{
			"aliasdef":        "AliasDef",
			"genericaliasdef": "GenericAliasDef",
		} {
			def, ok := byPkg[fixturePath(pkg)]
			if !ok || def.Name != name {
				t.Errorf("%s: got %+v, want a definition named %s", pkg, def, name)
			}
		}
	})

	t.Run("nil roles declare an untyped port", func(t *testing.T) {
		def := byPkg[fixturePath("nilroles")]
		if len(def.Ports) != 1 || def.Ports[0].Name != "Untyped" ||
			def.Ports[0].Roles != nil {
			t.Errorf("Ports = %+v, want one untyped port", def.Ports)
		}
	})

	t.Run("resources behind pointer and variadic parameters", func(t *testing.T) {
		for _, pkg := range []string{"pointerresources", "variadicresources"} {
			def := byPkg[fixturePath(pkg)]
			if len(def.Resources) != 1 || def.Resources[0].Name != "Storage" {
				t.Errorf("%s: Resources = %+v, want Storage", pkg, def.Resources)
			}
		}
	})

	t.Run("nil ports", func(t *testing.T) {
		def, ok := byPkg[fixturePath("zerodefaults")]
		if !ok || def.Ports != nil {
			t.Errorf("zerodefaults: got %+v, want a definition without ports", def)
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
	return "github.com/sarchlab/akita/v5/inspect/testdata/fixtures/" + name
}

func checkLocalProto(t *testing.T, byPkg map[string]schema.Definition) {
	t.Helper()

	def, ok := byPkg["github.com/sarchlab/akita/v5/inspect/testdata/fixtures/localproto"]
	if !ok {
		t.Fatalf("no definition extracted for the localproto fixture")
	}

	if len(def.Ports) != 2 {
		t.Fatalf("Ports = %+v, want 2 ports", def.Ports)
	}

	want := map[string]schema.Role{
		"In":   {Protocol: "inspect.localproto", Role: "consumer"},
		"Feed": {Protocol: "inspect.localproto", Role: "producer"},
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

func TestModuleDiscovery(t *testing.T) {
	defs, errs := inspect.Inspect(inspect.Options{Dir: ".."}, "./...")
	if len(errs) != 0 {
		t.Fatalf("module inspection failed: %v", errs)
	}
	if len(defs) != 15 {
		t.Fatalf("got %d definitions, want 15", len(defs))
	}
	resources := map[string][]string{
		"mem/datamover":                      {"InsideMapper", "OutsideMapper"},
		"mem/acceptancetests/memaccessagent": {"LowModule"},
		"noc/networking/switching/endpoint":  {"DevicePorts"},
		"noc/networking/switching/switches":  {"RoutingTable"},
	}
	for _, def := range defs {
		if strings.Contains(def.Package, "/fixtures/") {
			t.Errorf("discovered fixture: %s", def.Package)
		}
		suffix := strings.TrimPrefix(def.Package, "github.com/sarchlab/akita/v5/")
		if want, ok := resources[suffix]; ok {
			got := make([]string, len(def.Resources))
			for i, field := range def.Resources {
				got[i] = field.Name
			}
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("%s resources: got %v, want %v", suffix, got, want)
			}
			delete(resources, suffix)
		}
	}
	if len(resources) != 0 {
		t.Errorf("missing resource definitions: %v", resources)
	}
	t.Logf("module inspection: %d production definitions, zero errors, all required builder resources present", len(defs))
}

func fixturePatterns(t *testing.T) []string {
	t.Helper()

	patterns := []string{"github.com/sarchlab/akita/v5/mem/rob", "github.com/sarchlab/akita/v5/timing"}
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
