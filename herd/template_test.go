package herd

import (
	"slices"
	"strings"
	"testing"
)

func TestExpandTemplate(t *testing.T) {
	vars := map[string]string{"name": "1019_fir_base", "w": "fir"}

	got, err := ExpandTemplate(`./run.sh {name} {bin:sim} -w {w} "${w}" {other}`, vars)
	if err != nil {
		t.Fatal(err)
	}

	if want := `./run.sh 1019_fir_base {bin:sim} -w fir "${w}" {other}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}

	if _, err := ExpandTemplate("echo {name}", map[string]string{"name": "a b"}); err == nil ||
		!strings.Contains(err.Error(), "HERD_NAME") {
		t.Errorf("unsafe value: %v", err)
	}

	// An unsafe value that is not used is fine.
	if got, err := ExpandTemplate("echo hi", map[string]string{"name": "a;b"}); err != nil || got != "echo hi" {
		t.Errorf("unused unsafe value: %q %v", got, err)
	}
}

func TestPlaceholders(t *testing.T) {
	if got := Placeholders("{a}_{b}_{a} ${c} {bin:x}"); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("got %v", got)
	}
}

func TestParseEachAndCombinations(t *testing.T) {
	w, err := ParseEach("w=fir,mm")
	if err != nil {
		t.Fatal(err)
	}

	c, err := ParseEach("c=base,fast,slow")
	if err != nil {
		t.Fatal(err)
	}

	combos := Combinations([]Each{w, c})
	if len(combos) != 6 || combos[0]["w"] != "fir" || combos[0]["c"] != "base" ||
		combos[1]["c"] != "fast" || combos[5]["w"] != "mm" || combos[5]["c"] != "slow" {
		t.Errorf("combos: %v", combos)
	}

	for _, bad := range []string{"w", "w=", "w=a,,b", "1w=a", "name=a"} {
		if _, err := ParseEach(bad); err == nil {
			t.Errorf("ParseEach(%q) succeeded", bad)
		}
	}

	if got := Combinations(nil); len(got) != 1 || len(got[0]) != 0 {
		t.Errorf("no dims: %v", got)
	}
}
