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

	t.Run("package without definition is skipped", func(t *testing.T) {
		if _, ok := byPkg["github.com/sarchlab/akita/v5/timing"]; ok {
			t.Errorf("timing has no definition but one was extracted")
		}
	})

	t.Run("non-constant default is an error", func(t *testing.T) {
		checkError(t, errs, "fixtures/nonconst", "not statically analyzable")
	})

	t.Run("misnamed definition var is an error", func(t *testing.T) {
		checkError(t, errs, "fixtures/wrongname", `must be named "Definition"`)
	})

	t.Run("no unexpected errors", func(t *testing.T) {
		for _, err := range errs {
			msg := err.Error()
			if !strings.Contains(msg, "fixtures/nonconst") &&
				!strings.Contains(msg, "fixtures/wrongname") {
				t.Errorf("unexpected error: %v", err)
			}
		}
	})
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
		if err := os.WriteFile(goldenPath, got, 0644); err != nil {
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
