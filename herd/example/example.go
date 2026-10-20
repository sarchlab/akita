// Package example holds the project of the Herd walkthrough, which
// `akita herd init --example` writes: a fake simulator, the scripts that
// run it, and a herd.json that uses every key.
package example

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarchlab/akita/v5/herd"
)

//go:embed files
var files embed.FS

// File is a file of the example project.
type File struct {
	// Path is relative to the project, slash-separated.
	Path string
	// Role says what the file stands for in a real project.
	Role string
}

// Files lists the files of the example project in the order Write
// writes them.
var Files = []File{
	{"herd.json", "project config: naming, simulators, campaigns, plots, build (edit for your project)"},
	{"sim/main.go", "the fake simulator (your simulator)"},
	{"run.sh", "runs one simulation (how you start yours)"},
	{"sweep.sh", "runs a campaign of 6 simulations (your sweep script, if any)"},
	{"legacy.py", "maps old result names (only if you have old names)"},
	{"flaky.sh", "dies of SIGBUS on server1 (demo only)"},
	{"go.mod", "the Go module of sim (demo only)"},
	{".gitignore", "ignores remote/, where pull and collect put results"},
}

// generated holds the files that cannot be embedded: a go.mod would make
// files/ a module of its own, and a .gitignore would apply to this
// repository.
var generated = map[string]string{
	"go.mod":     "module demo\n\ngo 1.22\n",
	".gitignore": "remote/\n",
}

// Write writes the example project into dir. It writes nothing when one
// of the files exists.
func Write(dir string) error {
	for _, f := range Files {
		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("%s exists; write the example into an empty directory", p)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}

	for _, f := range Files {
		data, err := content(f.Path)
		if err != nil {
			return err
		}

		mode := os.FileMode(0o644)
		if strings.HasSuffix(f.Path, ".sh") {
			mode = 0o755
		}

		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}

		if err := os.WriteFile(p, data, mode); err != nil {
			return err
		}
	}

	return nil
}

func content(path string) ([]byte, error) {
	if s, ok := generated[path]; ok {
		return []byte(s), nil
	}

	return files.ReadFile("files/" + path)
}

// Hosts returns the user config of the example: hosts server1 and
// server2 are the directories server1/ and server2/ next to the project
// directory, with queues. server1 has one slot and avoids SIGBUS.
func Hosts(projectDir string) *herd.UserConfig {
	parent := filepath.Dir(projectDir)

	return &herd.UserConfig{Hosts: []herd.HostConfig{
		{Name: "server1", Type: herd.HostLocal, Root: filepath.Join(parent, "server1"),
			Queue: &herd.QueueConfig{Slots: 1, MemReserveGB: 0.5, MemPauseGB: 0.2,
				MemResumeGB: 0.4, AvoidSignals: []string{"SIGBUS"}}},
		{Name: "server2", Type: herd.HostLocal, Root: filepath.Join(parent, "server2"),
			Queue: &herd.QueueConfig{Slots: 4, MemReserveGB: 1, MemPauseGB: 0.5, MemResumeGB: 2}},
	}}
}
