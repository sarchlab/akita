// Package sourcefs locates and serializes Akita's own Go source so a simulation
// can record the exact library source that produced a trace. DaisenBot (in
// daisen2) reads that recorded source to interpret a trace's Kinds, milestones,
// and component behavior instead of guessing.
//
// The source is read from disk at record time — from the Go module cache, a
// `replace` path, or the working tree when Akita is the main module — so it
// always matches the binary that ran, with no generated or embedded artifact to
// keep in sync. When the source is not on disk (e.g. a `-trimpath` build, or a
// binary moved off the build machine), AkitaSourceDir reports false and the
// caller records nothing for Akita rather than guessing.
package sourcefs

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ModulePath is Akita's Go module path — the natural root label when recording
// its source into a trace.
const ModulePath = "github.com/sarchlab/akita/v5"

// skipDirs are directory names pruned anywhere in a source tree: VCS metadata,
// Claude worktrees (full nested repo copies), vendored or generated trees, and
// test fixtures.
var skipDirs = map[string]bool{
	".git":         true,
	".claude":      true,
	"vendor":       true,
	"node_modules": true,
	"testdata":     true,
	"dist":         true,
}

// isSourceFile reports whether a file should be recorded: non-test .go source,
// plus go.mod for module/dependency context.
func isSourceFile(name string) bool {
	if name == "go.mod" {
		return true
	}
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

// AkitaSourceDir returns the on-disk directory of Akita's module root and true
// if found. It locates itself from the compiled-in path of this source file and
// walks up to the enclosing go.mod, so it resolves Akita whether it is the main
// module, a module-cache dependency, or a `replace` target. It returns false
// when the path is unavailable (e.g. a -trimpath build) or the source is not
// present on the machine.
func AkitaSourceDir() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(file) {
		return "", false
	}

	root, ok := moduleRootOf(filepath.Dir(file), ModulePath)
	if !ok {
		return "", false
	}
	if _, err := os.Stat(root); err != nil {
		return "", false
	}
	return root, true
}

// moduleRootOf walks up from dir to the directory whose go.mod declares
// modulePath, returning it and true if found.
func moduleRootOf(dir, modulePath string) (string, bool) {
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if moduleLineIs(data, modulePath) {
				return dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func moduleLineIs(gomod []byte, modulePath string) bool {
	scanner := bufio.NewScanner(bytes.NewReader(gomod))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "module" {
			return fields[1] == modulePath
		}
	}
	return false
}

// ArchiveDir reads the recordable source files (non-test .go plus go.mod) under
// dir and returns them as a gzip-compressed tar, keyed by slash paths relative
// to dir.
func ArchiveDir(dir string) ([]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSourceFile(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = content
		return nil
	})
	if err != nil {
		return nil, err
	}
	return packArchive(files)
}

// ArchiveFS reads every regular file from fsys and returns a gzip-compressed
// tar. The caller is responsible for filtering — fsys should contain only what
// should be recorded (e.g. a //go:embed of the wanted .go files).
func ArchiveFS(fsys fs.FS) ([]byte, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		content, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		files[p] = content
		return nil
	})
	if err != nil {
		return nil, err
	}
	return packArchive(files)
}

func packArchive(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteArchive(&buf, files); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteArchive writes files (slash-separated path -> content) as a
// gzip-compressed tar to w. Entries are emitted in sorted path order with a
// zero modtime, so identical inputs produce byte-identical output.
func WriteArchive(w io.Writer, files map[string][]byte) error {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	for _, p := range paths {
		content := files[p]
		hdr := &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     p,
			Mode:     0o644,
			Size:     int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(content); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// ReadArchive decompresses a gzip-compressed tar (as produced by WriteArchive)
// into a slash-separated path -> content map. Non-regular entries are skipped.
func ReadArchive(gztar []byte) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(gztar))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		files[hdr.Name] = content
	}
	return files, nil
}
