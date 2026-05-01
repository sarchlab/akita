package akita_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const documentedNodeBaseline = "18.20.7"

func TestFrontendNodeBaselineIsConsistent(t *testing.T) {
	toolchainDoc := readTextFile(t, "TOOLCHAIN_VERSIONS.md")
	if !strings.Contains(toolchainDoc, "**Node.js Version**: "+documentedNodeBaseline) {
		t.Fatalf("TOOLCHAIN_VERSIONS.md should document Node.js %s", documentedNodeBaseline)
	}

	nvmrc := strings.TrimSpace(readTextFile(t, "daisen/static/.nvmrc"))
	if nvmrc != documentedNodeBaseline {
		t.Fatalf("daisen/static/.nvmrc = %q, want %q", nvmrc, documentedNodeBaseline)
	}

	workflow := readTextFile(t, ".github/workflows/akita_test.yml")
	workflowNodeVersions := regexp.MustCompile(`node-version:\s*([^\s]+)`).FindAllStringSubmatch(workflow, -1)
	if len(workflowNodeVersions) != 2 {
		t.Fatalf(
			"workflow should pin both frontend jobs to Node %s; found %d node-version entries",
			documentedNodeBaseline,
			len(workflowNodeVersions),
		)
	}
	for _, match := range workflowNodeVersions {
		if match[1] != documentedNodeBaseline {
			t.Fatalf("workflow node-version = %q, want %q", match[1], documentedNodeBaseline)
		}
	}

	for _, dir := range []string{"daisen/static", "daisen2/static"} {
		pkg := readPackageJSON(t, dir+"/package.json")
		assertFrontendRootEngines(t, dir+"/package.json", pkg.Engines)

		lock := readPackageLock(t, dir+"/package-lock.json")
		root := lock.Packages[""]
		assertFrontendRootEngines(t, dir+"/package-lock.json root", root.Engines)
	}
}

func TestSelectedFrontendPackageEnginesSupportNodeBaseline(t *testing.T) {
	for _, dir := range []string{"daisen/static", "daisen2/static"} {
		lock := readPackageLock(t, dir+"/package-lock.json")
		root := lock.Packages[""]
		for _, pkgName := range directFrontendDependencyNames(root) {
			t.Run(dir+"/"+pkgName, func(t *testing.T) {
				pkg, ok := lock.Packages["node_modules/"+pkgName]
				if !ok {
					t.Fatalf("%s lockfile does not select direct dependency %s", dir, pkgName)
				}

				engine := pkg.Engines.Node
				if engine == "" {
					return
				}
				if !nodeVersionSatisfies(documentedNodeBaseline, engine) {
					t.Fatalf("%s selects %s@%s with engines.node %q, incompatible with Node %s",
						dir, pkgName, pkg.Version, engine, documentedNodeBaseline)
				}
			})
		}
	}
}

func TestDependencySecurityFrontendNodeEngineEvidenceMatchesLockfiles(t *testing.T) {
	doc := readTextFile(t, dependencySecurityDocPath)
	section := frontendNodeEngineEvidenceSection(t, doc)
	actualEvidence := parseFrontendNodeEngineEvidence(t, section)
	expectedEvidence := expectedFrontendNodeEngineEvidence(t)

	if len(actualEvidence) != len(expectedEvidence) {
		t.Errorf(
			"%s frontend engine evidence entries = %d, want %d",
			dependencySecurityDocPath,
			len(actualEvidence),
			len(expectedEvidence),
		)
	}

	for key, expected := range expectedEvidence {
		actual, ok := actualEvidence[key]
		if !ok {
			t.Errorf(
				"%s should document %s/%s@%s engines.node %q",
				dependencySecurityDocPath,
				key.dir,
				key.pkg,
				expected.version,
				expected.engine,
			)
			continue
		}

		if actual.version != expected.version {
			t.Errorf(
				"%s documents %s/%s version %s, want %s from lockfile",
				dependencySecurityDocPath,
				key.dir,
				key.pkg,
				actual.version,
				expected.version,
			)
		}
		if actual.engine != expected.engine {
			t.Errorf(
				"%s documents %s/%s engines.node %q, want %q from lockfile",
				dependencySecurityDocPath,
				key.dir,
				key.pkg,
				actual.engine,
				expected.engine,
			)
		}
		if actual.path != expected.path {
			t.Errorf(
				"%s documents %s/%s citation path %s, want %s",
				dependencySecurityDocPath,
				key.dir,
				key.pkg,
				actual.path,
				expected.path,
			)
			continue
		}

		assertFrontendNodeEngineCitationRange(t, actual)
	}

	for key, actual := range actualEvidence {
		if _, ok := expectedEvidence[key]; ok {
			continue
		}
		t.Errorf(
			"%s documents %s/%s@%s engines.node %q, but that package is not a direct dependency with engines.node",
			dependencySecurityDocPath,
			key.dir,
			key.pkg,
			actual.version,
			actual.engine,
		)
	}
}

func assertFrontendRootEngines(t *testing.T, source string, engines nodeEngines) {
	t.Helper()
	if engines.Node != documentedNodeBaseline {
		t.Fatalf("%s engines.node = %q, want %q", source, engines.Node, documentedNodeBaseline)
	}
	if engines.NPM != ">=10.0.0" {
		t.Fatalf("%s engines.npm = %q, want >=10.0.0", source, engines.NPM)
	}
}

type packageJSON struct {
	Engines nodeEngines `json:"engines"`
}

type packageLock struct {
	Packages map[string]lockedPackage `json:"packages"`
}

type lockedPackage struct {
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         nodeEngines       `json:"engines"`
}

type nodeEngines struct {
	Node string `json:"node"`
	NPM  string `json:"npm"`
}

func readPackageJSON(t *testing.T, path string) packageJSON {
	t.Helper()
	var pkg packageJSON
	readJSONFile(t, path, &pkg)
	return pkg
}

func readPackageLock(t *testing.T, path string) packageLock {
	t.Helper()
	var lock packageLock
	readJSONFile(t, path, &lock)
	return lock
}

func readJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func directFrontendDependencyNames(root lockedPackage) []string {
	names := map[string]bool{}
	for name := range maps.Keys(root.Dependencies) {
		names[name] = true
	}
	for name := range maps.Keys(root.DevDependencies) {
		names[name] = true
	}
	return slices.Sorted(maps.Keys(names))
}

type frontendNodeEngineEvidenceKey struct {
	dir string
	pkg string
}

type frontendNodeEngineEvidence struct {
	dir     string
	pkg     string
	version string
	engine  string
	path    string
	start   int
	end     int
}

func frontendNodeEngineEvidenceSection(t *testing.T, doc string) string {
	t.Helper()

	heading := "## Frontend Node engine reconciliation evidence"
	start := strings.Index(doc, heading)
	if start < 0 {
		t.Fatalf("%s should contain %q", dependencySecurityDocPath, heading)
	}

	remaining := doc[start+len(heading):]
	end := strings.Index(remaining, "\n## ")
	if end < 0 {
		return remaining
	}

	return remaining[:end]
}

func parseFrontendNodeEngineEvidence(
	t *testing.T,
	section string,
) map[frontendNodeEngineEvidenceKey]frontendNodeEngineEvidence {
	t.Helper()

	recordPattern := regexp.MustCompile(
		"`([^`]+)@([^`]+)` requires `([^`]+)` " +
			"\\(`((?:daisen|daisen2)/static/package-lock\\.json):" +
			"([1-9][0-9]*)-([1-9][0-9]*)`\\)",
	)
	matches := recordPattern.FindAllStringSubmatch(section, -1)
	if len(matches) == 0 {
		t.Fatalf("%s frontend Node engine section has no package-lock evidence records", dependencySecurityDocPath)
	}

	evidence := map[frontendNodeEngineEvidenceKey]frontendNodeEngineEvidence{}
	for _, match := range matches {
		path := match[4]
		dir := strings.TrimSuffix(path, "/package-lock.json")
		pkg := match[1]
		key := frontendNodeEngineEvidenceKey{dir: dir, pkg: pkg}
		if existing, ok := evidence[key]; ok {
			t.Fatalf(
				"%s duplicates frontend Node engine evidence for %s/%s at %s and %s",
				dependencySecurityDocPath,
				key.dir,
				key.pkg,
				existing.path,
				path,
			)
		}

		start, _ := strconv.Atoi(match[5])
		end, _ := strconv.Atoi(match[6])
		evidence[key] = frontendNodeEngineEvidence{
			dir:     dir,
			pkg:     pkg,
			version: match[2],
			engine:  match[3],
			path:    path,
			start:   start,
			end:     end,
		}
	}

	return evidence
}

func expectedFrontendNodeEngineEvidence(
	t *testing.T,
) map[frontendNodeEngineEvidenceKey]frontendNodeEngineEvidence {
	t.Helper()

	expected := map[frontendNodeEngineEvidenceKey]frontendNodeEngineEvidence{}
	for _, dir := range []string{"daisen/static", "daisen2/static"} {
		lock := readPackageLock(t, dir+"/package-lock.json")
		root := lock.Packages[""]
		for _, pkgName := range directFrontendDependencyNames(root) {
			pkg, ok := lock.Packages["node_modules/"+pkgName]
			if !ok {
				t.Fatalf("%s package-lock is missing direct dependency %s", dir, pkgName)
			}
			if pkg.Engines.Node == "" {
				continue
			}
			expected[frontendNodeEngineEvidenceKey{dir: dir, pkg: pkgName}] = frontendNodeEngineEvidence{
				dir:     dir,
				pkg:     pkgName,
				version: pkg.Version,
				engine:  pkg.Engines.Node,
				path:    dir + "/package-lock.json",
			}
		}
	}

	return expected
}

func assertFrontendNodeEngineCitationRange(t *testing.T, evidence frontendNodeEngineEvidence) {
	t.Helper()

	if evidence.start > evidence.end {
		t.Fatalf("%s cites descending package-lock range %s:%d-%d",
			dependencySecurityDocPath, evidence.path, evidence.start, evidence.end)
	}

	rangeText := readFrontendNodeEngineCitationRange(t, evidence)
	requiredFragments := map[string]string{
		"dependency name": fmt.Sprintf(`"node_modules/%s": {`, evidence.pkg),
		"version":         fmt.Sprintf(`"version": "%s"`, evidence.version),
		"engines.node":    fmt.Sprintf(`"node": "%s"`, evidence.engine),
	}
	for label, fragment := range requiredFragments {
		if !strings.Contains(rangeText, fragment) {
			t.Errorf(
				"%s cites %s:%d-%d for %s@%s, but the range does not contain the %s fragment %q",
				dependencySecurityDocPath,
				evidence.path,
				evidence.start,
				evidence.end,
				evidence.pkg,
				evidence.version,
				label,
				fragment,
			)
		}
	}
}

func readFrontendNodeEngineCitationRange(t *testing.T, evidence frontendNodeEngineEvidence) string {
	t.Helper()

	lines := strings.Split(readTextFile(t, evidence.path), "\n")
	if evidence.end > len(lines) {
		t.Fatalf("%s cites %s:%d-%d, but the file has %d lines",
			dependencySecurityDocPath, evidence.path, evidence.start, evidence.end, len(lines))
	}

	return strings.Join(lines[evidence.start-1:evidence.end], "\n")
}

func nodeVersionSatisfies(version, expression string) bool {
	for _, alternative := range strings.Split(expression, "||") {
		if nodeVersionSatisfiesAll(version, strings.Fields(strings.TrimSpace(alternative))) {
			return true
		}
	}
	return false
}

func nodeVersionSatisfiesAll(version string, comparators []string) bool {
	if len(comparators) == 0 {
		return false
	}
	for _, comparator := range comparators {
		if !nodeVersionSatisfiesComparator(version, comparator) {
			return false
		}
	}
	return true
}

func nodeVersionSatisfiesComparator(version, comparator string) bool {
	switch {
	case strings.HasPrefix(comparator, ">="):
		return compareNodeVersions(version, strings.TrimPrefix(comparator, ">=")) >= 0
	case strings.HasPrefix(comparator, "^"):
		base := strings.TrimPrefix(comparator, "^")
		versionParts := parseNodeVersion(version)
		baseParts := parseNodeVersion(base)
		return versionParts[0] == baseParts[0] && compareNodeVersions(version, base) >= 0
	default:
		return compareNodeVersions(version, comparator) == 0
	}
}

func compareNodeVersions(left, right string) int {
	leftParts := parseNodeVersion(left)
	rightParts := parseNodeVersion(right)
	for index := range leftParts {
		if leftParts[index] < rightParts[index] {
			return -1
		}
		if leftParts[index] > rightParts[index] {
			return 1
		}
	}
	return 0
}

func parseNodeVersion(version string) [3]int {
	parts := strings.Split(version, ".")
	var parsed [3]int
	for index := range parsed {
		if index >= len(parts) {
			continue
		}
		value, err := strconv.Atoi(parts[index])
		if err != nil {
			panic(fmt.Sprintf("invalid Node version %q", version))
		}
		parsed[index] = value
	}
	return parsed
}
