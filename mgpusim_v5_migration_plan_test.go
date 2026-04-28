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
	citationPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9_./-])((?:[A-Za-z0-9_-]+/)*[A-Za-z0-9_.-]+\.[A-Za-z0-9_.-]+):([1-9][0-9]*)(?:-([1-9][0-9]*))?`)

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
