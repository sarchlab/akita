package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(tb testing.TB) string {
	tb.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("failed to determine caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "./akitav5"}, args...)...)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(out), exitErr.ExitCode()
	}
	t.Fatalf("unexpected error running CLI: %v", err)
	return "", 1
}

func TestComponentLintSamples(t *testing.T) {
	t.Helper()

	tests := []struct {
		name        string
		args        []string
		wantExit    int
		mustContain []string
	}{
		{
			name:        "clean component passes",
			args:        []string{"component-lint", "akitav5/tests/rule1_1_multi_marker"},
			wantExit:    0,
			mustContain: []string{"\tOK"},
		},
		{
			name:        "directory without marker is skipped",
			args:        []string{"component-lint", "akitav5/tests/rule1_2_missing_marker"},
			wantExit:    0,
			mustContain: []string{"-- not a component"},
		},
		{
			name:        "violations reported",
			args:        []string{"component-lint", "akitav5/tests/rule1_3_missing_comp"},
			wantExit:    1,
			mustContain: []string{"Rule 1.3"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			out, code := runCLI(t, tt.args...)
			if code != tt.wantExit {
				t.Fatalf("expected exit %d, got %d, output: %s", tt.wantExit, code, out)
			}
			for _, needle := range tt.mustContain {
				if !strings.Contains(out, needle) {
					t.Fatalf("expected output to contain %q, got: %s", needle, out)
				}
			}
		})
	}
}

func TestComponentLintRecursive(t *testing.T) {
	out, code := runCLI(t, "component-lint", "-r", "akitav5/tests")
	if code == 0 {
		t.Fatalf("expected recursive lint to fail due to violations, output: %s", out)
	}
	if !strings.Contains(out, "akitav5/tests/rule1_3_missing_comp") {
		t.Fatalf("expected recursive output to include rule1_3_missing_comp, got: %s", out)
	}
}

func TestComponentCreateGeneratesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "samplecomp")

	out, code := runCLI(t, "component-create", target)
	if code != 0 {
		t.Fatalf("component-create failed with code %d, output: %s", code, out)
	}

	builder := filepath.Join(target, "builder.go")
	comp := filepath.Join(target, "comp.go")
	if _, err := os.Stat(builder); err != nil {
		t.Fatalf("expected builder.go to exist: %v", err)
	}
	if _, err := os.Stat(comp); err != nil {
		t.Fatalf("expected comp.go to exist: %v", err)
	}

	data, err := os.ReadFile(comp)
	if err != nil {
		t.Fatalf("failed to read comp.go: %v", err)
	}
	if !strings.Contains(string(data), "package samplecomp") {
		t.Fatalf("comp.go package not rewritten, content: %s", string(data))
	}
}
