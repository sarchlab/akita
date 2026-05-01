package akita_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const migrationPlanPath = "mgpusim_v5_migration_plan.md"

var (
	citationPattern = regexp.MustCompile(
		`(?:^|[^A-Za-z0-9_./-])` +
			`((?:[A-Za-z0-9_-]+/)*[A-Za-z0-9_.-]+\.[A-Za-z0-9_.-]+):` +
			`([1-9][0-9]*)(?:-([1-9][0-9]*))?`,
	)

	packageDiscoveryTargets = []string{
		"./sim",
		"./mem",
		"./noc/directconnection",
		"./queueing",
		"./monitoring",
		"./daisen",
		"./simulation",
		"./tracing",
		"./modeling",
	}
)

func TestMigrationPlanLocalCitationsResolve(t *testing.T) {
	plan := readMigrationPlan(t)
	citations := findLocalCitations(plan)
	if len(citations) == 0 {
		t.Fatalf("%s has no local file:line citations to validate", migrationPlanPath)
	}

	for _, citation := range citations {
		citation.validate(t)
	}
}

func TestMigrationPlanPackageDiscoveryCommandResolves(t *testing.T) {
	plan := string(readMigrationPlan(t))
	commandText := "go list " + strings.Join(packageDiscoveryTargets, " ")
	if !strings.Contains(plan, commandText) {
		t.Fatalf("%s does not name package-discovery command %q", migrationPlanPath, commandText)
	}

	args := append([]string{"list"}, packageDiscoveryTargets...)
	cmd := exec.Command("go", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migration plan package-discovery command %q failed: %v\n%s",
			commandText, err, output)
	}
}

func TestMigrationPlanValidationGateScopeIsLocalAkitaGoOnly(t *testing.T) {
	plan := string(readMigrationPlan(t))

	requiredClaims := []string{
		"M25 validation-command scope",
		"local Akita Go build/lint/test gate",
		"not a full merge-equivalent CI or downstream validation gate",
		"go list -mod=readonly -m all",
		"go mod tidy -diff",
		"mockgen` v0.6.0",
		"golangci-lint` v2.9.0",
		"ginkgo` v2.25.1",
		"golangci-lint run --modules-download-mode=readonly ./...",
		"ginkgo -r --mod=readonly",
		"does not run Daisen frontend Node jobs",
		"NOC/MEM Python acceptance tests",
		"dependency-security validation",
		"downstream `mgpusim`/`mgpusim-dev` compile/smoke/benchmark validation",
		"CI/frontend/acceptance coverage (separate from the local gate)",
	}
	for _, claim := range requiredClaims {
		if !strings.Contains(plan, claim) {
			t.Errorf("%s should document validation-gate scope claim %q", migrationPlanPath, claim)
		}
	}

	staleOrOverstatedClaims := []string{
		"Akita merge-equivalent check",
		"repo merge script",
	}
	for _, claim := range staleOrOverstatedClaims {
		if strings.Contains(plan, claim) {
			t.Errorf("%s should not overstate local validation scope with %q", migrationPlanPath, claim)
		}
	}
}

func TestMigrationPlanRunBeforeMergeCitationsTrackCurrentScript(t *testing.T) {
	plan := string(readMigrationPlan(t))
	script := readTextFile(t, runBeforeMergeScriptPath)

	commandStart := lineNumberContaining(t, script, "Running local Akita Go build/lint/test gate.")
	commandEnd := lineNumberContaining(t, script, "\"${bin_dir}/ginkgo\" -r --mod=readonly")
	fullGateEnd := lineNumberContaining(t, script, "Local Akita Go build/lint/test gate completed successfully.")

	requiredCitations := []string{
		fmt.Sprintf("%s:%d-%d", runBeforeMergeScriptPath,
			lineNumberContaining(t, script, "MOCKGEN_VERSION=\""),
			lineNumberContaining(t, script, "GOLANGCI_LINT_VERSION=\"")),
		fmt.Sprintf("%s:%d", runBeforeMergeScriptPath,
			lineNumberContaining(t, script, "go.uber.org/mock/mockgen@${MOCKGEN_VERSION}")),
		fmt.Sprintf("%s:%d", runBeforeMergeScriptPath,
			lineNumberContaining(t, script, "go generate -mod=readonly ./...")),
		fmt.Sprintf("%s:%d-%d", runBeforeMergeScriptPath, commandStart, commandEnd),
		fmt.Sprintf("%s:%d-%d", runBeforeMergeScriptPath,
			lineNumberContaining(t, script, "verify_go_mod_sum_clean() {"),
			lineNumberContaining(t, script, "verify_tracked_clean \"startup\"")),
		fmt.Sprintf("%s:%d-%d", runBeforeMergeScriptPath,
			lineNumberContaining(t, script, "verify_go_mod_sum_clean \"validation\""),
			lineNumberContaining(t, script, "verify_tracked_clean \"validation\"")),
		fmt.Sprintf("%s:%d-%d", runBeforeMergeScriptPath, commandStart, fullGateEnd),
	}
	for _, citation := range requiredCitations {
		if !strings.Contains(plan, citation) {
			t.Errorf("%s should cite current %s line span %q", migrationPlanPath, runBeforeMergeScriptPath, citation)
		}
	}
}

func TestMigrationPlanStatusRefreshDoesNotCarryStaleBaselineClaims(t *testing.T) {
	plan := string(readMigrationPlan(t))

	requiredClaims := []string{
		"Historical M2 audit finding (report-only)",
		"current Akita checkout passes `go test ./...`",
		"Akita baseline-health gate (currently green)",
		"Downstream mgpusim validation gate (future)",
	}
	for _, claim := range requiredClaims {
		if !strings.Contains(plan, claim) {
			t.Errorf("%s should include refreshed status claim %q", migrationPlanPath, claim)
		}
	}

	staleClaims := []string{
		"Report-only test-status conflict",
		"origin/main does not contain that repair",
		"making `go test ./...` pass here would require",
		"current `go test ./...` is blocked",
		"this M3 branch carries the generated mock repair",
	}
	for _, claim := range staleClaims {
		if strings.Contains(plan, claim) {
			t.Errorf("%s still contains stale status claim %q", migrationPlanPath, claim)
		}
	}
}

type localCitation struct {
	planLine int
	text     string
	path     string
	start    int
	end      int
}

func readMigrationPlan(t *testing.T) []byte {
	t.Helper()

	content, err := os.ReadFile(migrationPlanPath)
	if err != nil {
		t.Fatalf("read %s: %v", migrationPlanPath, err)
	}

	return content
}

func lineNumberContaining(t *testing.T, text, needle string) int {
	t.Helper()

	for index, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return index + 1
		}
	}

	t.Fatalf("text did not contain line with %q", needle)
	return 0
}

func findLocalCitations(plan []byte) []localCitation {
	matches := citationPattern.FindAllSubmatchIndex(plan, -1)
	citations := make([]localCitation, 0, len(matches))

	for _, match := range matches {
		path := string(plan[match[2]:match[3]])
		if !isLocalCitationPath(path) {
			continue
		}

		start, _ := strconv.Atoi(string(plan[match[4]:match[5]]))
		end := start
		if match[6] >= 0 {
			end, _ = strconv.Atoi(string(plan[match[6]:match[7]]))
		}

		citationText := fmt.Sprintf("%s:%d", path, start)
		if end != start {
			citationText = fmt.Sprintf("%s-%d", citationText, end)
		}

		citations = append(citations, localCitation{
			planLine: bytes.Count(plan[:match[2]], []byte("\n")) + 1,
			text:     citationText,
			path:     path,
			start:    start,
			end:      end,
		})
	}

	return citations
}

func isLocalCitationPath(path string) bool {
	return path != "" && !filepath.IsAbs(path) && !strings.Contains(path, "..")
}

func (citation localCitation) validate(t *testing.T) {
	t.Helper()

	content, err := os.ReadFile(citation.path)
	if err != nil {
		t.Errorf("%s:%d cites %q, but the file is not readable: %v",
			migrationPlanPath, citation.planLine, citation.text, err)
		return
	}

	lineCount := countLines(content)
	if citation.start > citation.end {
		t.Errorf("%s:%d cites invalid descending range %q",
			migrationPlanPath, citation.planLine, citation.text)
		return
	}
	if citation.end > lineCount {
		t.Errorf("%s:%d cites %q, but %s has %d lines",
			migrationPlanPath, citation.planLine, citation.text, citation.path, lineCount)
	}
}

func countLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}

	lines := bytes.Count(content, []byte("\n"))
	if !bytes.HasSuffix(content, []byte("\n")) {
		lines++
	}

	return lines
}
