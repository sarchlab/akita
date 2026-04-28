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

const (
	dependencySecurityDocPath    = "DEPENDENCY_SECURITY_VALIDATION.md"
	dependencySecurityGoModPath  = "go.mod"
	dependencySecurityScriptPath = "run_dependency_security_validation.sh"
)

var dependencySecurityCitationPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9_./-])((?:[A-Za-z0-9_.-]+/)*[A-Za-z0-9_.-]+\.[A-Za-z0-9_.-]+):([1-9][0-9]*)(?:-([1-9][0-9]*))?`)

const fakeDependencySecurityGoScript = `#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  version)
    echo "go version go1.26.2 fake"
    ;;
  env)
    echo "GOVERSION=go1.26.2"
    echo "GOTOOLCHAIN=go1.26.2"
    echo "GOMOD=/fake/go.mod"
    ;;
  list)
    echo "github.com/sarchlab/akita/v5"
    echo "golang.org/x/vuln v1.3.0"
    ;;
  mod)
    case "${2:-}" in
      graph)
        echo "github.com/sarchlab/akita/v5 golang.org/x/vuln@v1.3.0"
        ;;
      tidy)
        if [[ "${3:-}" != "-diff" ]]; then
          echo "unexpected go mod tidy arguments: $*" >&2
          exit 2
        fi
        ;;
      *)
        echo "unexpected go mod command: $*" >&2
        exit 2
        ;;
    esac
    ;;
  test)
    if [[ "${AKITA_FAKE_GO_FAIL_TEST:-}" == "1" ]]; then
      echo "fake go test failure" >&2
      exit 42
    fi
    echo "ok github.com/sarchlab/akita/v5 0.001s"
    ;;
  install)
    if [[ "${AKITA_FAKE_GO_FAIL_INSTALL:-}" == "1" ]]; then
      echo "fake govulncheck install failure" >&2
      exit 43
    fi
    if [[ -z "${GOBIN:-}" ]]; then
      echo "GOBIN is required" >&2
      exit 2
    fi
    mkdir -p "${GOBIN}"
    cat >"${GOBIN}/govulncheck" <<'GOVULNCHECK'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  -version)
    if [[ "${AKITA_FAKE_GOVULNCHECK_FAIL_VERSION:-}" == "1" ]]; then
      echo "fake govulncheck version failure" >&2
      exit 44
    fi
    echo "govulncheck v1.3.0 fake"
    ;;
  -test)
    if [[ "${AKITA_FAKE_GOVULNCHECK_FAIL_TEST:-}" == "1" ]]; then
      echo "fake govulncheck test failure" >&2
      exit 45
    fi
    echo "No vulnerabilities found."
    ;;
  *)
    echo "unexpected govulncheck arguments: $*" >&2
    exit 2
    ;;
esac
GOVULNCHECK
    chmod +x "${GOBIN}/govulncheck"
    echo "GOBIN=${GOBIN}"
    ;;
  *)
    echo "unexpected go command: $*" >&2
    exit 2
    ;;
esac
`

const fakeDependencySecurityGitScript = `#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "diff" && "${2:-}" == "--check" ]]; then
  echo "git diff clean"
  exit 0
fi

echo "unexpected git command: $*" >&2
exit 2
`

const fakeDependencySecurityNpmScript = `#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "audit" && "${2:-}" == "--audit-level=high" && "${3:-}" == "--omit=optional" && "$#" -eq 3 ]]; then
  if [[ "${AKITA_FAKE_NPM_FAIL_AUDIT:-}" == "1" ]]; then
    echo "fake npm audit failure in ${PWD}" >&2
    exit 46
  fi
  echo "found 0 vulnerabilities in ${PWD}"
  exit 0
fi

echo "unexpected npm command: $*" >&2
exit 2
`

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
		"npm audit --audit-level=high --omit=optional",
		"daisen/static",
		"daisen2/static",
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

func TestDependencySecurityDocumentsRetainedModuleExcludes(t *testing.T) {
	goMod := readTextFile(t, dependencySecurityGoModPath)
	doc := readTextFile(t, dependencySecurityDocPath)

	testCases := []struct {
		module      string
		version     string
		goModReason string
		docEvidence string
	}{
		{
			module:      "golang.org/x/crypto",
			version:     "v0.44.0",
			goModReason: "golang.org/x/net@v0.47.0",
			docEvidence: "go list -mod=readonly -m all",
		},
		{
			module:      "gopkg.in/yaml.v2",
			version:     "v2.2.2",
			goModReason: "go mod tidy -diff",
			docEvidence: "gopkg.in/yaml.v2 v2.2.2/go.mod",
		},
	}

	for _, tc := range testCases {
		directive := fmt.Sprintf("exclude %s %s", tc.module, tc.version)
		for name, text := range map[string]string{
			dependencySecurityGoModPath: goMod,
			dependencySecurityDocPath:   doc,
		} {
			if !strings.Contains(text, directive) {
				t.Errorf("%s should retain and document %q", name, directive)
			}
		}
		if !strings.Contains(goMod, tc.goModReason) {
			t.Errorf("%s should explain why %q is retained", dependencySecurityGoModPath, directive)
		}
		if !strings.Contains(doc, tc.docEvidence) {
			t.Errorf("%s should include regression evidence for %q", dependencySecurityDocPath, directive)
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
		"run_required_in daisen_static_npm_audit daisen/static",
		"npm audit --audit-level=high --omit=optional",
		"run_required_in daisen2_static_npm_audit daisen2/static",
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

func TestDependencySecurityScriptStopsOnRequiredCommandFailure(t *testing.T) {
	toolsDir := writeDependencySecurityFakeTools(t)
	reportDir := filepath.Join(t.TempDir(), "report")

	output, err := runDependencySecurityScript(t, toolsDir, reportDir,
		"AKITA_FAKE_GO_FAIL_TEST=1")
	if err == nil {
		t.Fatalf("%s should fail when a required command fails\n%s",
			dependencySecurityScriptPath, output)
	}
	if !strings.Contains(output, "Dependency security validation failed during go_test") {
		t.Fatalf("failure output should name the failing check; got:\n%s", output)
	}
	if strings.Contains(output, "Dependency security validation completed successfully") {
		t.Fatalf("failure output must not include the final success message:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(reportDir, "logs", "govulncheck_test.log")); !os.IsNotExist(err) {
		t.Fatalf("script should stop before govulncheck_test after go_test failure; stat err=%v", err)
	}
}

func TestDependencySecurityScriptPropagatesGovulncheckFailures(t *testing.T) {
	testCases := []struct {
		name       string
		env        string
		check      string
		message    string
		exitCode   int
		absentLogs []string
	}{
		{
			name:       "install",
			env:        "AKITA_FAKE_GO_FAIL_INSTALL=1",
			check:      "govulncheck_install",
			message:    "fake govulncheck install failure",
			exitCode:   43,
			absentLogs: []string{"govulncheck_version.log", "govulncheck_test.log"},
		},
		{
			name:       "version",
			env:        "AKITA_FAKE_GOVULNCHECK_FAIL_VERSION=1",
			check:      "govulncheck_version",
			message:    "fake govulncheck version failure",
			exitCode:   44,
			absentLogs: []string{"govulncheck_test.log"},
		},
		{
			name:     "test",
			env:      "AKITA_FAKE_GOVULNCHECK_FAIL_TEST=1",
			check:    "govulncheck_test",
			message:  "fake govulncheck test failure",
			exitCode: 45,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			toolsDir := writeDependencySecurityFakeTools(t)
			reportDir := filepath.Join(t.TempDir(), "report")

			output, err := runDependencySecurityScript(t, toolsDir, reportDir, tc.env)
			if err == nil {
				t.Fatalf("%s should fail when %s fails\n%s",
					dependencySecurityScriptPath, tc.check, output)
			}
			if code := dependencySecurityExitCode(err); code != tc.exitCode {
				t.Fatalf("%s should exit with %d from %s; got %d (%v)\n%s",
					dependencySecurityScriptPath, tc.exitCode, tc.check, code, err, output)
			}

			failureLine := "Dependency security validation failed during " + tc.check
			if !strings.Contains(output, failureLine) {
				t.Fatalf("failure output should name %s; got:\n%s", tc.check, output)
			}
			if !strings.Contains(output, tc.message) {
				t.Fatalf("failure output should include fake tool failure %q; got:\n%s",
					tc.message, output)
			}
			if strings.Contains(output, "Dependency security validation completed successfully") {
				t.Fatalf("failure output must not include the final success message:\n%s", output)
			}

			failingLog := readTextFile(t, filepath.Join(reportDir, "logs", tc.check+".log"))
			if !strings.Contains(failingLog, tc.message) {
				t.Fatalf("%s log should include fake tool failure %q; got:\n%s",
					tc.check, tc.message, failingLog)
			}
			for _, logName := range tc.absentLogs {
				if _, err := os.Stat(filepath.Join(reportDir, "logs", logName)); !os.IsNotExist(err) {
					t.Fatalf("script should stop before %s after %s failure; stat err=%v",
						logName, tc.check, err)
				}
			}
		})
	}
}

func TestDependencySecurityScriptPropagatesFrontendAuditFailures(t *testing.T) {
	toolsDir := writeDependencySecurityFakeTools(t)
	reportDir := filepath.Join(t.TempDir(), "report")

	output, err := runDependencySecurityScript(t, toolsDir, reportDir,
		"AKITA_FAKE_NPM_FAIL_AUDIT=1")
	if err == nil {
		t.Fatalf("%s should fail when frontend npm audit fails\n%s",
			dependencySecurityScriptPath, output)
	}
	if code := dependencySecurityExitCode(err); code != 46 {
		t.Fatalf("%s should exit with fake npm audit status 46; got %d (%v)\n%s",
			dependencySecurityScriptPath, code, err, output)
	}
	if !strings.Contains(output, "Dependency security validation failed during daisen_static_npm_audit") {
		t.Fatalf("failure output should name daisen_static_npm_audit; got:\n%s", output)
	}
	if !strings.Contains(output, "fake npm audit failure") {
		t.Fatalf("failure output should include npm audit failure detail; got:\n%s", output)
	}
	if strings.Contains(output, "Dependency security validation completed successfully") {
		t.Fatalf("failure output must not include the final success message:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(reportDir, "logs", "daisen2_static_npm_audit.log")); !os.IsNotExist(err) {
		t.Fatalf("script should stop before daisen2 audit after daisen audit failure; stat err=%v", err)
	}
}

func TestDependencySecurityScriptCanonicalizesReportDirForGOBIN(t *testing.T) {
	toolsDir := writeDependencySecurityFakeTools(t)
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "target")
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target report dir: %v", err)
	}

	linkDir := filepath.Join(tempDir, "report-link")
	if err := os.Symlink(targetDir, linkDir); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	output, err := runDependencySecurityScript(t, toolsDir, linkDir)
	if err != nil {
		t.Fatalf("%s should succeed with fake tools: %v\n%s",
			dependencySecurityScriptPath, err, output)
	}

	canonicalReportDir, err := filepath.EvalSymlinks(linkDir)
	if err != nil {
		t.Fatalf("resolve report symlink: %v", err)
	}
	if !strings.Contains(output, "Dependency security validation report: "+canonicalReportDir) {
		t.Fatalf("output should use canonical report directory %q; got:\n%s",
			canonicalReportDir, output)
	}

	installLog := readTextFile(t, filepath.Join(canonicalReportDir, "logs", "govulncheck_install.log"))
	canonicalGOBIN := filepath.Join(canonicalReportDir, "bin")
	if !strings.Contains(installLog, "GOBIN="+canonicalGOBIN) {
		t.Fatalf("govulncheck install should receive canonical GOBIN %q; log:\n%s",
			canonicalGOBIN, installLog)
	}
	if _, err := os.Stat(filepath.Join(canonicalGOBIN, "govulncheck")); err != nil {
		t.Fatalf("govulncheck should be installed under canonical GOBIN: %v", err)
	}
}

type dependencySecurityCitation struct {
	docLine int
	text    string
	path    string
	start   int
	end     int
}

func runDependencySecurityScript(t *testing.T, toolsDir, reportDir string, extraEnv ...string) (string, error) {
	t.Helper()

	cmd := exec.Command("bash", dependencySecurityScriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DEPENDENCY_SECURITY_REPORT_DIR="+reportDir,
	)
	cmd.Env = append(cmd.Env, extraEnv...)

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func dependencySecurityExitCode(err error) int {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}

	return -1
}

func writeDependencySecurityFakeTools(t *testing.T) string {
	t.Helper()

	toolsDir := t.TempDir()
	writeDependencySecurityFakeTool(t, toolsDir, "go", fakeDependencySecurityGoScript)
	writeDependencySecurityFakeTool(t, toolsDir, "git", fakeDependencySecurityGitScript)
	writeDependencySecurityFakeTool(t, toolsDir, "npm", fakeDependencySecurityNpmScript)
	return toolsDir
}

func writeDependencySecurityFakeTool(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
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
