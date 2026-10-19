package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestHelpDescribesUnavailableCommands(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--help"}, {"help"}, {"component", "--help"}, {"check", "--help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var output bytes.Buffer
			root := newRootCommand()
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"unavailable", "v5.1", "523"} {
				if !strings.Contains(output.String(), want) {
					t.Errorf("help missing %q: %s", want, output.String())
				}
			}
		})
	}
}

func TestLegacyCommandsFailWithoutChangingFiles(t *testing.T) {
	for _, args := range [][]string{
		{"component", "--create", "Example"},
		{"component", "--create=Example"},
		{"component"},
		{"check", "."},
		{"check", "missing"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Chdir(t.TempDir())
			const sentinel = "existing component source\n"
			if err := os.WriteFile("comp.go", []byte(sentinel), 0o600); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			root := newRootCommand()
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs(args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), "unavailable in Akita v5.0") {
				t.Fatalf("expected unavailable error, got %v", err)
			}
			if output.Len() != 0 {
				t.Fatalf("unexpected command output: %s", output.String())
			}
			entries, err := os.ReadDir(".")
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "comp.go" {
				t.Fatalf("command changed directory contents: %v", entries)
			}
			data, err := os.ReadFile("comp.go")
			if err != nil || string(data) != sentinel {
				t.Fatalf("command changed existing source: %q, %v", data, err)
			}
		})
	}
}
