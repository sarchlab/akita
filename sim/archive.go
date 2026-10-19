package sim

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// A checkpoint archive is a tar.gz holding one payload per registered entity.
// The payload files are the inventory, so there is no manifest. Each entity owns the bytes of its own payload.
const (
	entityPrefix = "entities/"
)

// Checkpointable is implemented by runtime entities that save and load their own
// payload bytes. The entity owns its encoding (JSON, binary, ...); the archive
// only shuttles bytes. It is satisfied structurally, so an entity package never
// imports the simulation package to be checkpointable.
type Checkpointable interface {
	SaveCheckpoint(w io.Writer) error
	LoadCheckpoint(r io.Reader) error
}

// archiveEntry is one entity payload to write into a checkpoint archive.
type archiveEntry struct {
	name string
	data []byte
}

// entityPath returns the archive path for an entity's payload. The name is
// escaped so it can never become a real filesystem path.
func entityPath(name string) string {
	return entityPrefix + url.PathEscape(name)
}

// entityName reverses entityPath, returning false for non-entity archive paths.
func entityName(archivePath string) (string, bool) {
	if !strings.HasPrefix(archivePath, entityPrefix) {
		return "", false
	}

	name, err := url.PathUnescape(strings.TrimPrefix(archivePath, entityPrefix))
	if err != nil {
		return "", false
	}
	return name, true
}

// writeArchive atomically writes entity payloads sorted by name.
func writeArchive(path string, entries []archiveEntry) error {
	tmpPath := path + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	err = writeArchiveStream(file, entries)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	cleanup = false
	return nil
}

func writeArchiveStream(w io.Writer, entries []archiveEntry) error {
	gz := gzip.NewWriter(w)
	gz.ModTime = time.Unix(0, 0)

	tw := tar.NewWriter(gz)

	sorted := append([]archiveEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].name < sorted[j].name
	})

	seen := map[string]struct{}{}
	for _, entry := range sorted {
		if _, dup := seen[entry.name]; dup {
			return fmt.Errorf("checkpoint: duplicate entity %q", entry.name)
		}
		seen[entry.name] = struct{}{}

		if err := writeTarEntry(tw, entityPath(entry.name), entry.data); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeTarEntry(tw *tar.Writer, path string, data []byte) error {
	header := &tar.Header{
		Name:    path,
		Mode:    0o600,
		Size:    int64(len(data)),
		ModTime: time.Unix(0, 0),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err := tw.Write(data)
	return err
}

// readArchive returns entity payloads keyed by entity name. Unknown archive entries are rejected.
func readArchive(path string) (map[string][]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return readArchiveStream(file)
}

func readArchiveStream(r io.Reader) (map[string][]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	payloads := make(map[string][]byte)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
			return nil,
				fmt.Errorf("checkpoint: unsupported archive entry %q", header.Name)
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}

		name, ok := entityName(header.Name)
		if !ok {
			return nil,
				fmt.Errorf("checkpoint: unexpected archive entry %q", header.Name)
		}
		if _, dup := payloads[name]; dup {
			return nil, fmt.Errorf("checkpoint: duplicate entity %q", name)
		}
		payloads[name] = data
	}

	return payloads, nil
}
