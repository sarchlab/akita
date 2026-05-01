package akita_test

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

const runBeforeMergeScriptPath = "run_before_merge.sh"

func TestRunBeforeMergeScriptSyntax(t *testing.T) {
	cmd := exec.Command("bash", "-n", runBeforeMergeScriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash -n %s failed: %v\n%s", runBeforeMergeScriptPath, err, output)
	}
}

func TestRunBeforeMergeFailsClosedBeforeValidationCommands(t *testing.T) {
	script := readTextFile(t, runBeforeMergeScriptPath)

	strictMode := "set -Eeuo pipefail"
	strictModeIndex := strings.Index(script, strictMode)
	if strictModeIndex < 0 {
		t.Fatalf("%s should enable strict shell mode with %q", runBeforeMergeScriptPath, strictMode)
	}

	firstValidationCommandIndex := firstRunBeforeMergeValidationCommandIndex(t, script)
	if strictModeIndex > firstValidationCommandIndex {
		t.Fatalf("%s should enable strict mode before validation commands", runBeforeMergeScriptPath)
	}

	for _, required := range []string{
		"verify_go_mod_sum_clean \"startup\"",
		"verify_tracked_clean \"startup\"",
		"verify_go_mod_sum_clean \"validation\"",
		"verify_tracked_clean \"validation\"",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("%s should contain %q", runBeforeMergeScriptPath, required)
		}
	}
}

func TestRunBeforeMergeRemovedMutatingDependencyCommands(t *testing.T) {
	script := readTextFile(t, runBeforeMergeScriptPath)

	for _, forbiddenPattern := range []string{
		`(?m)^\s*go\s+get\b`,
		`(?m)^\s*run\s+go\s+get\b`,
		`(?m)^\s*go\s+mod\s+tidy\s*$`,
		`(?m)^\s*run\s+go\s+mod\s+tidy\s*$`,
		`curl\b`,
		`\|\s*(ba)?sh\b`,
		`install\.sh`,
		`@latest\b`,
	} {
		if regexp.MustCompile(forbiddenPattern).MatchString(script) {
			t.Errorf("%s should not match mutating/remote-installer pattern %q", runBeforeMergeScriptPath, forbiddenPattern)
		}
	}
}

func TestRunBeforeMergeUsesReadOnlyDependencyAndTidyChecks(t *testing.T) {
	script := readTextFile(t, runBeforeMergeScriptPath)

	required := []string{
		"go list -mod=readonly -m all",
		"go mod tidy -diff",
		"go generate -mod=readonly ./...",
		"go build -mod=readonly ./...",
		"--modules-download-mode=readonly",
		"ginkgo\" -r --mod=readonly",
		"-mod=readonly",
	}
	for _, text := range required {
		if !strings.Contains(script, text) {
			t.Errorf("%s should contain read-only validation text %q", runBeforeMergeScriptPath, text)
		}
	}
}

func TestRunBeforeMergeIsolatesWritableGoCaches(t *testing.T) {
	script := readTextFile(t, runBeforeMergeScriptPath)

	required := []string{
		"temp_parent_dir=\"${AKITA_RUN_BEFORE_MERGE_TMPDIR:-${TMPDIR:-/tmp}}\"",
		"go_path_dir=\"${temp_dir}/gopath\"",
		"go_mod_cache_dir=\"${go_path_dir}/pkg/mod\"",
		"go_build_cache_dir=\"${temp_dir}/go-build-cache\"",
		"GOPATH=${go_path_dir}",
		"GOMODCACHE=${go_mod_cache_dir}",
		"GOCACHE=${go_build_cache_dir}",
		"chmod -R u+w -- \"${temp_dir}\"",
		"run env \"${go_env[@]}\" go list -mod=readonly -m all",
		"run env \"${go_env[@]}\" go mod tidy -diff",
	}
	for _, text := range required {
		if !strings.Contains(script, text) {
			t.Errorf("%s should isolate writable Go state with %q", runBeforeMergeScriptPath, text)
		}
	}
}

func TestRunBeforeMergeScopeIsDocumentedAsLocalGoGate(t *testing.T) {
	doc := readTextFile(t, "TOOLCHAIN_VERSIONS.md")

	required := []string{
		"local Akita Go build/lint/test gate",
		"not a full merge-equivalent CI run",
		"does not run frontend Node jobs",
		"NOC/MEM Python acceptance tests",
		"dependency-security validation (the separate\n`./run_dependency_security_validation.sh` path)",
		"downstream `mgpusim`/\n`mgpusim-dev` compile/smoke/benchmark validation",
		"go list -mod=readonly -m all",
		"go mod tidy -diff",
		"go.uber.org/mock/mockgen@v0.6.0",
		"github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0",
		"github.com/onsi/ginkgo/v2/ginkgo@v2.25.1",
		"golangci-lint run --modules-download-mode=readonly ./...",
		"ginkgo -r --mod=readonly",
		"does not mutate a developer's shared Go\n   dependency or build caches",
	}
	for _, text := range required {
		if !strings.Contains(doc, text) {
			t.Errorf("TOOLCHAIN_VERSIONS.md should document local gate scope text %q", text)
		}
	}
}

func TestRunBeforeMergeToolPinsMatchRepositoryDocs(t *testing.T) {
	script := readTextFile(t, runBeforeMergeScriptPath)
	doc := readTextFile(t, "TOOLCHAIN_VERSIONS.md")

	testCases := []struct {
		name          string
		variable      string
		version       string
		installTarget string
	}{
		{
			name:          "mockgen",
			variable:      "MOCKGEN_VERSION",
			version:       "v0.6.0",
			installTarget: "go.uber.org/mock/mockgen@${MOCKGEN_VERSION}",
		},
		{
			name:          "ginkgo",
			variable:      "GINKGO_VERSION",
			version:       "v2.25.1",
			installTarget: "github.com/onsi/ginkgo/v2/ginkgo@${GINKGO_VERSION}",
		},
		{
			name:          "golangci-lint",
			variable:      "GOLANGCI_LINT_VERSION",
			version:       "v2.9.0",
			installTarget: "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pinLine := tc.variable + "=\"" + tc.version + "\""
			if !strings.Contains(script, pinLine) {
				t.Fatalf("%s should pin %s with %q", runBeforeMergeScriptPath, tc.name, pinLine)
			}
			if !strings.Contains(script, tc.installTarget) {
				t.Fatalf("%s should install %s with %q", runBeforeMergeScriptPath, tc.name, tc.installTarget)
			}
			if !strings.Contains(doc, "**"+tc.name+"**: "+tc.version) {
				t.Fatalf("TOOLCHAIN_VERSIONS.md should document %s pin %s", tc.name, tc.version)
			}
		})
	}
}

func TestRunBeforeMergeScriptIsExecutable(t *testing.T) {
	info, err := os.Stat(runBeforeMergeScriptPath)
	if err != nil {
		t.Fatalf("stat %s: %v", runBeforeMergeScriptPath, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("%s should be executable by maintainers", runBeforeMergeScriptPath)
	}
}

func firstRunBeforeMergeValidationCommandIndex(t *testing.T, script string) int {
	t.Helper()

	candidates := []string{
		"go get",
		"go mod",
		"go list",
		"go install",
		"go generate",
		"go build",
		"golangci-lint",
		"ginkgo",
		"curl",
	}

	first := len(script)
	for _, candidate := range candidates {
		if index := strings.Index(script, candidate); index >= 0 && index < first {
			first = index
		}
	}
	if first == len(script) {
		t.Fatalf("%s should contain at least one validation command", runBeforeMergeScriptPath)
	}

	return first
}
