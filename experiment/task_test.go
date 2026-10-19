package experiment

import (
	"strings"
	"testing"
)

func TestTaskBinaryProperties(t *testing.T) {
	task := &Task{Binaries: map[string]Binary{
		"b": {SHA: strings.Repeat("b", 64), Commit: "a41f7e2", Dirty: true, Diff: "1234abcd"},
		"a": {SHA: strings.Repeat("a", 64), Commit: "a41f7e2"},
	}}

	got := task.BinaryProperties()
	want := []Property{
		{Property: "Binary", Value: "a aaaaaaaaaaaa, b bbbbbbbbbbbb"},
		{Property: "Git Revision", Value: "a41f7e2, a41f7e2-dirty-1234abcd"},
	}

	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("properties = %+v, want %+v", got, want)
	}

	if (&Task{}).BinaryProperties() != nil {
		t.Fatal("a task without binaries has properties")
	}
}

func TestParseProjectConfigBuild(t *testing.T) {
	for _, doc := range []string{
		`{"build": {"binaries": [{"name": "a/b", "command": ["go"]}]}}`,
		`{"build": {"binaries": [{"name": "a", "command": []}]}}`,
		`{"build": {"binaries": [{"name": "a", "command": ["x"]}, {"name": "a", "command": ["y"]}]}}`,
	} {
		if _, err := parseProjectConfig([]byte(doc)); err == nil {
			t.Errorf("accepted %s", doc)
		}
	}

	cfg, err := parseProjectConfig([]byte(`{"build": {"binaries": [{"name": "sim", "dir": "s", "command": ["go"]}]}}`))
	if err != nil {
		t.Fatal(err)
	}

	if b, ok := cfg.Binary("sim"); !ok || b.Dir != "s" {
		t.Fatalf("Binary(sim) = %+v, %v", b, ok)
	}
}
