package modelingtest

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/inspect/testdata/fixtures/flatlinks"
	"github.com/sarchlab/akita/v5/inspect/testdata/fixtures/nofreq"

	"github.com/sarchlab/akita/v5/inspect/schema"
	"github.com/sarchlab/akita/v5/inspect/testdata/fixtures/aliasdef"
	"github.com/sarchlab/akita/v5/inspect/testdata/fixtures/fullcomp"
	"github.com/sarchlab/akita/v5/inspect/testdata/fixtures/genericaliasdef"
	"github.com/sarchlab/akita/v5/inspect/testdata/fixtures/scalars"
	"github.com/sarchlab/akita/v5/inspect/testdata/fixtures/zerodefaults"
)

func TestCheckTickingSupportedDeclarations(t *testing.T) {
	CheckTicking(t, fullcomp.Definition)
	CheckTicking(t, flatlinks.Definition)
	CheckTicking(t, scalars.Definition)
	CheckTicking(t, zerodefaults.Definition)
	CheckTicking(t, aliasdef.Definition)
	CheckTicking(t, genericaliasdef.Definition)
}

func TestDefaultsDetectExactIntegerDrift(t *testing.T) {
	cases := []struct {
		name            string
		static, runtime any
	}{
		{"signed", int64(9007199254740992), struct{ N int64 }{9007199254740993}},
		{"unsigned", uint64(18446744073709551614), struct{ N uint64 }{18446744073709551615}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			def := schema.Definition{Spec: []schema.Field{{Name: "N", Default: c.static}}}
			if len(defaultMismatches(def, c.runtime)) != 1 {
				t.Fatal("integer drift was missed")
			}
		})
	}
}

func TestCheckTickingRejectsMissingFreq(t *testing.T) {
	if os.Getenv("AKITA_TEST_MISSING_FREQ") == "1" {
		CheckTicking(t, nofreq.Definition)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCheckTickingRejectsMissingFreq$")
	cmd.Env = append(os.Environ(), "AKITA_TEST_MISSING_FREQ=1")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "must have a Freq timing.Freq field") {
		t.Fatalf("CheckTicking must fail for missing Freq: %v\n%s", err, out)
	}
}
