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
