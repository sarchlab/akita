package akita_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	dependencySecurityDocPath    = "DEPENDENCY_SECURITY_VALIDATION.md"
	dependencySecurityScriptPath = "run_dependency_security_validation.sh"
)

var dependencySecurityCitationPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9_./-])((?:[A-Za-z0-9_.-]+/)*[A-Za-z0-9_.-]+\.[A-Za-z0-9_.-]+):([1-9][0-9]*)(?:-([1-9][0-9]*))?`)

func TestDependencySecurityValidationDocNamesRequiredChecks(t *testing.T) {
	doc := readTextFile(t, dependencySecurityDocPath)

	required := []string{
		"./run_dependency_security_validation.sh",
		"go list -mod=readonly -m all",
		"go mod graph",
		"go mod tidy -diff",
		"go test ./...",
		"git diff --check",
		"govulncheck -test ./...",
		"GitHub/Dependabot",
		"asynchronous",
		"default-branch",
		"local/manual",
		"CI",
	}
	for _, text := range required {
		if !strings.Contains(doc, text) {
			t.Errorf("%s should document %q", dependencySecurityDocPath, text)
		}
	}
}

func TestDependencySecurityScriptRunsRequiredChecks(t *testing.T) {
	script := readTextFile(t, dependencySecurityScriptPath)

	required := []string{
		"set -euo pipefail",
		"GOVULNCHECK_VERSION=\"v1.3.0\"",
		"go list -mod=readonly -m all",
		"go mod graph",
		"go mod tidy -diff",
		"go test ./...",
		"git diff --check",
		"golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}",
		"govulncheck\" -test ./...",
	}
	for _, text := range required {
		if !strings.Contains(script, text) {
			t.Errorf("%s should contain %q", dependencySecurityScriptPath, text)
		}
	}

	info, err := os.Stat(dependencySecurityScriptPath)
	if err != nil {
		t.Fatalf("stat %s: %v", dependencySecurityScriptPath, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("%s should be executable by maintainers", dependencySecurityScriptPath)
	}
}

func TestDependencySecurityGovulncheckPinMatchesDocAndScript(t *testing.T) {
	doc := readTextFile(t, dependencySecurityDocPath)
	script := readTextFile(t, dependencySecurityScriptPath)

	scriptVersion := firstSubmatch(t, script, `GOVULNCHECK_VERSION="([^"]+)"`)
	if !strings.Contains(doc, "govulncheck@"+scriptVersion) {
		t.Fatalf("%s should document script govulncheck pin %s", dependencySecurityDocPath, scriptVersion)
	}
}

func TestDependencySecurityLocalCitationsResolve(t *testing.T) {
	docBytes := []byte(readTextFile(t, dependencySecurityDocPath))
	citations := findDependencySecurityCitations(docBytes)
	if len(citations) == 0 {
		t.Fatalf("%s has no local file:line citations to validate", dependencySecurityDocPath)
	}

	for _, citation := range citations {
		citation.validate(t)
	}
}

func TestDependencySecurityScriptSyntax(t *testing.T) {
	cmd := exec.Command("bash", "-n", dependencySecurityScriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash -n %s failed: %v\n%s", dependencySecurityScriptPath, err, output)
	}
}

type dependencySecurityCitation struct {
	docLine int
	text    string
	path    string
	start   int
	end     int
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(content)
}

func firstSubmatch(t *testing.T, text, pattern string) string {
	t.Helper()

	match := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if match == nil {
		t.Fatalf("text did not match %q", pattern)
	}

	return match[1]
}

func findDependencySecurityCitations(doc []byte) []dependencySecurityCitation {
	matches := dependencySecurityCitationPattern.FindAllSubmatchIndex(doc, -1)
	citations := make([]dependencySecurityCitation, 0, len(matches))

	for _, match := range matches {
		path := string(doc[match[2]:match[3]])
		if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
			continue
		}

		start, _ := strconv.Atoi(string(doc[match[4]:match[5]]))
		end := start
		if match[6] >= 0 {
			end, _ = strconv.Atoi(string(doc[match[6]:match[7]]))
		}

		citationText := fmt.Sprintf("%s:%d", path, start)
		if end != start {
			citationText = fmt.Sprintf("%s-%d", citationText, end)
		}

		citations = append(citations, dependencySecurityCitation{
			docLine: bytes.Count(doc[:match[2]], []byte("\n")) + 1,
			text:    citationText,
			path:    path,
			start:   start,
			end:     end,
		})
	}

	return citations
}

func (citation dependencySecurityCitation) validate(t *testing.T) {
	t.Helper()

	content, err := os.ReadFile(citation.path)
	if err != nil {
		t.Errorf("%s:%d cites %q, but the file is not readable: %v",
			dependencySecurityDocPath, citation.docLine, citation.text, err)
		return
	}

	lineCount := dependencySecurityCountLines(content)
	if citation.start > citation.end {
		t.Errorf("%s:%d cites invalid descending range %q",
			dependencySecurityDocPath, citation.docLine, citation.text)
		return
	}
	if citation.end > lineCount {
		t.Errorf("%s:%d cites %q, but %s has %d lines",
			dependencySecurityDocPath, citation.docLine, citation.text, citation.path, lineCount)
	}
}

func dependencySecurityCountLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}

	lines := bytes.Count(content, []byte("\n"))
	if !bytes.HasSuffix(content, []byte("\n")) {
		lines++
	}

	return lines
}
