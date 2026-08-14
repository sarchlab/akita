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
	{"non-constant default", "fixtures/nonconst",
		"not statically analyzable"},
	{"misnamed definition var", "fixtures/wrongname",
		`must be named "Definition"`},
	{"unkeyed literal", "fixtures/unkeyed", "must be keyed"},
	{"count field without spec field", "fixtures/badcount",
		`CountField "missing" does not match`},
}

// TestInspect loads all test subjects in one Inspect call (loading carries
// the whole dependency graph, so one load shared by all subtests keeps the
// test fast) and checks definitions and errors per package.
func TestInspect(t *testing.T) {
	defs, errs := inspect.Inspect(inspect.Options{},
		"github.com/sarchlab/akita/v5/mem/rob",
		"github.com/sarchlab/akita/v5/inspect/fixtures/...",
		"github.com/sarchlab/akita/v5/timing",
	)

	byPkg := map[string]schema.Definition{}
	for _, d := range defs {
		byPkg[d.Package] = d
	}

	t.Run("rob matches golden", func(t *testing.T) {
		checkGolden(t, byPkg, "github.com/sarchlab/akita/v5/mem/rob",
			"rob.golden.json")
	})

	t.Run("fullcomp matches golden", func(t *testing.T) {
		checkGolden(t, byPkg,
			"github.com/sarchlab/akita/v5/inspect/fixtures/fullcomp",
			"fullcomp.golden.json")
	})

	t.Run("explicit type instantiation", func(t *testing.T) {
		checkInstantiated(t, byPkg)
	})

	t.Run("same-package protocol and bare role identifiers",
		func(t *testing.T) {
			checkLocalProto(t, byPkg)
		})

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

func checkInstantiated(
	t *testing.T, byPkg map[string]schema.Definition,
) {
	t.Helper()

	def, ok := byPkg["github.com/sarchlab/akita/v5/inspect/fixtures/instantiated"]
	if !ok {
		t.Fatalf("no definition extracted for the instantiated fixture")
	}

	if def.Name != "Instantiated" {
		t.Errorf("Name = %q, want %q", def.Name, "Instantiated")
	}

	if len(def.Spec) != 1 || def.Spec[0].JSONName != "depth" ||
		def.Spec[0].Default != int64(16) {
		t.Errorf("Spec = %+v, want depth with default 16", def.Spec)
	}
}

func checkLocalProto(t *testing.T, byPkg map[string]schema.Definition) {
	t.Helper()

	def, ok := byPkg["github.com/sarchlab/akita/v5/inspect/fixtures/localproto"]
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
