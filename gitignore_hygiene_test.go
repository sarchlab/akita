package akita_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestGitignoreDoesNotHideNestedSourcePaths(t *testing.T) {
	paths := []string{
		"cmd/main.go",
		"debug/test.go",
		"virtualmem/test.go",
		"writebackcache/test.go",
		"akita/cmd/new.go",
		"daisen/cmd/new.go",
		"mem/example/debug/test.go",
		"mem/example/virtualmem/test.go",
		"mem/example/writebackcache/test.go",
		"mem/acceptancetests/virtualmem/test.go",
		"mem/acceptancetests/writebackcache/test.go",
	}

	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			stdout, ignored := gitCheckIgnore(t, path)
			if ignored {
				t.Fatalf("%s is unexpectedly ignored:\n%s", path, stdout)
			}
		})
	}
}

func TestGitignoreStillCoversExpectedBuildOutputs(t *testing.T) {
	testCases := []struct {
		path      string
		wantMatch string
	}{
		{path: "cmd", wantMatch: ".gitignore"},
		{path: "debug", wantMatch: ".gitignore"},
		{path: "virtualmem", wantMatch: ".gitignore"},
		{path: "writebackcache", wantMatch: ".gitignore"},
		{
			path:      "mem/acceptancetests/virtualmem/virtualmem",
			wantMatch: "mem/acceptancetests/virtualmem/.gitignore",
		},
		{
			path:      "mem/acceptancetests/virtualmem/virtualmem.exe",
			wantMatch: "mem/acceptancetests/virtualmem/.gitignore",
		},
		{
			path:      "mem/acceptancetests/writebackcache/writebackcache",
			wantMatch: "mem/acceptancetests/writebackcache/.gitignore",
		},
		{
			path:      "mem/acceptancetests/writebackcache/writebackcache.exe",
			wantMatch: "mem/acceptancetests/writebackcache/.gitignore",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			stdout, ignored := gitCheckIgnore(t, tc.path)
			if !ignored {
				t.Fatalf("%s is not ignored; expected generated output to stay ignored", tc.path)
			}
			if !strings.Contains(stdout, tc.wantMatch) {
				t.Fatalf("%s was ignored by unexpected rule; got %q, want match containing %q", tc.path, stdout, tc.wantMatch)
			}
		})
	}
}

func gitCheckIgnore(t *testing.T, path string) (stdout string, ignored bool) {
	t.Helper()

	quietCmd := exec.Command("git", "check-ignore", "--no-index", "--quiet", path)
	quietOut, err := quietCmd.CombinedOutput()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("git check-ignore --quiet failed for %s: %v\n%s", path, err, quietOut)
		}
		if exitErr.ExitCode() == 1 {
			return "", false
		}
		t.Fatalf("git check-ignore --quiet failed for %s with exit code %d:\n%s", path, exitErr.ExitCode(), quietOut)
	}

	verboseCmd := exec.Command("git", "check-ignore", "--no-index", "-v", path)
	verboseOut, err := verboseCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git check-ignore -v failed for ignored path %s: %v\n%s", path, err, verboseOut)
	}

	return string(verboseOut), true
}
