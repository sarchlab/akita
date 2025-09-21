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

type componentLintTestCase struct {
	name        string
	args        []string
	wantExit    int
	mustContain []string
}

func getBasicComponentLintTestCases() []componentLintTestCase {
	return []componentLintTestCase{
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
			mustContain: []string{"not a component"},
		},
		{
			name:        "violations reported",
			args:        []string{"component-lint", "akitav5/tests/rule1_3_missing_comp"},
			wantExit:    1,
			mustContain: []string{"Rule 1.3"},
		},
	}
}

func getRule2TestCases() []componentLintTestCase {
	return []componentLintTestCase{
		{
			name:        "rule 2.1 pointer violation",
			args:        []string{"component-lint", "akitav5/tests/rule2_1_pointer"},
			wantExit:    1,
			mustContain: []string{"Rule 2.1", "pointer"},
		},
		{
			name:        "rule 2.1 map allowed",
			args:        []string{"component-lint", "akitav5/tests/rule2_1_map"},
			wantExit:    0,
			mustContain: []string{"\tOK"},
		},
		{
			name:        "rule 2.1 nested pure data",
			args:        []string{"component-lint", "akitav5/tests/rule2_1_nested"},
			wantExit:    0,
			mustContain: []string{"\tOK"},
		},
		{
			name:        "rule 2.1 channel violation",
			args:        []string{"component-lint", "akitav5/tests/rule2_1_channel"},
			wantExit:    1,
			mustContain: []string{"Rule 2.1", "channel"},
		},
	}
}

func getRule3TestCases() []componentLintTestCase {
	return []componentLintTestCase{
		{
			name:        "rule 3.1 missing spec",
			args:        []string{"component-lint", "akitav5/tests/rule3_1_missing_spec"},
			wantExit:    1,
			mustContain: []string{"Rule 3.1"},
		},
		{
			name:        "rule 3.3 defaults missing",
			args:        []string{"component-lint", "akitav5/tests/rule3_3_defaults_missing"},
			wantExit:    1,
			mustContain: []string{"Rule 3.3"},
		},
		{
			name:        "rule 3.4 validate missing",
			args:        []string{"component-lint", "akitav5/tests/rule3_4_validate_missing"},
			wantExit:    1,
			mustContain: []string{"Rule 3.4"},
		},
		{
			name:        "rule 3.2 nested pointer violation",
			args:        []string{"component-lint", "akitav5/tests/rule3_2_bad_nested"},
			wantExit:    1,
			mustContain: []string{"Rule 3.2"},
		},
		{
			name:        "rule 3.2 nested pure data passes",
			args:        []string{"component-lint", "akitav5/tests/rule3_2_good_nested"},
			wantExit:    0,
			mustContain: []string{"\tOK"},
		},
	}
}

func getRule4TestCases() []componentLintTestCase {
	return []componentLintTestCase{
		{
			name:        "rule 4.2 missing simulation setter",
			args:        []string{"component-lint", "akitav5/tests/rule4_2_missing_sim"},
			wantExit:    1,
			mustContain: []string{"Rule 4.2"},
		},
		{
			name:        "rule 4.3 missing spec setter",
			args:        []string{"component-lint", "akitav5/tests/rule4_3_missing_spec"},
			wantExit:    1,
			mustContain: []string{"Rule 4.3"},
		},
		{
			name:        "rule 4.6 missing validate call",
			args:        []string{"component-lint", "akitav5/tests/rule4_6_missing_validate"},
			wantExit:    1,
			mustContain: []string{"Rule 4.6"},
		},
	}
}

func getComponentLintTestCases() []componentLintTestCase {
	var cases []componentLintTestCase
	cases = append(cases, getBasicComponentLintTestCases()...)
	cases = append(cases, getRule2TestCases()...)
	cases = append(cases, getRule3TestCases()...)
	cases = append(cases, getRule4TestCases()...)
	return cases
}

func runComponentLintTest(t *testing.T, tc componentLintTestCase) {
	t.Helper()
	out, code := runCLI(t, tc.args...)
	if code != tc.wantExit {
		t.Fatalf("expected exit %d, got %d, output: %s", tc.wantExit, code, out)
	}
	for _, needle := range tc.mustContain {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected output to contain %q, got: %s", needle, out)
		}
	}
}

func TestComponentLintSamples(t *testing.T) {
	t.Helper()

	tests := getComponentLintTestCases()
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			runComponentLintTest(t, tt)
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
