package modelingtest

import (
	"testing"

	"github.com/sarchlab/akita/v5/inspect/schema"
	"github.com/sarchlab/akita/v5/inspect/testdata/fixtures/containers"
	"github.com/sarchlab/akita/v5/inspect/testdata/fixtures/zerodefaults"
)

func TestCheckDefinitionSupportedDefaults(t *testing.T) {
	CheckDefinition(t, containers.Definition, "github.com/sarchlab/akita/v5/inspect/testdata/fixtures/containers")
	CheckDefinition(t, zerodefaults.Definition, "github.com/sarchlab/akita/v5/inspect/testdata/fixtures/zerodefaults")
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
