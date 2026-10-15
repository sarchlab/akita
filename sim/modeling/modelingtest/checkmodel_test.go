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
	CheckTicking(t, (*fullcomp.Comp)(nil), fullcomp.DefaultSpec())
	CheckTicking(t, (*flatlinks.Comp)(nil), flatlinks.DefaultSpec())
	CheckTicking(t, (*scalars.Comp)(nil), scalars.DefaultSpec())
	CheckTicking(t, (*zerodefaults.Comp)(nil), zerodefaults.DefaultSpec())
	CheckTicking(t, (*aliasdef.Comp)(nil), aliasdef.DefaultSpec())
	CheckTicking(t, (*genericaliasdef.Comp)(nil), genericaliasdef.DefaultSpec())
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
		CheckTicking(t, (*nofreq.Comp)(nil), nofreq.DefaultSpec())
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCheckTickingRejectsMissingFreq$")
	cmd.Env = append(os.Environ(), "AKITA_TEST_MISSING_FREQ=1")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "must have a Freq timing.Freq field") {
		t.Fatalf("CheckTicking must fail for missing Freq: %v\n%s", err, out)
	}
}

func TestDefaultSpecReturnsIndependentCollections(t *testing.T) {
	a, b := flatlinks.DefaultSpec(), flatlinks.DefaultSpec()
	a.Bottom.Destinations[0] = "Changed"
	a.Mappings[0].Destinations[0] = "Changed"
	a.Links[0].Remote = "Changed"
	if b.Bottom.Destinations[0] != "A" || b.Mappings[0].Destinations[0] != "C" || b.Links[0].Remote != "Peer.Top" {
		t.Fatalf("DefaultSpec calls share collections: %+v", b)
	}
	t.Log("Separate DefaultSpec calls retain independent flat records and nested scalar slices")
}
