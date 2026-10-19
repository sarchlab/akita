package hostsys

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadLimited reads at most limit bytes of a file.
func ReadLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(io.LimitReader(f, limit))
}

// ReadText reads at most limit bytes of a file as text (invalid UTF-8
// replaced); "" when the file cannot be read.
func ReadText(path string, limit int64) string {
	data, err := ReadLimited(path, limit)
	if err != nil {
		return ""
	}

	return strings.ToValidUTF8(string(data), "�")
}

// ReadJSONMap reads a JSON object of at most limit bytes; nil when the
// file is missing, too long, or not an object.
func ReadJSONMap(path string, limit int64) map[string]any {
	data, err := ReadLimited(path, limit)
	if err != nil || len(data) == 0 {
		return nil
	}

	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return nil
	}

	return m
}

// WriteFileAtomic replaces a file by writing a unique temporary file in
// its directory and renaming it, so that readers never see a partial file
// and concurrent writers do not share a temporary file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}

	tmp := f.Name()

	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}

	if err == nil {
		err = os.Chmod(tmp, perm)
	}

	if err == nil {
		err = os.Rename(tmp, path)
	}

	if err != nil {
		_ = os.Remove(tmp)
	}

	return err
}

// Exists tells whether a path exists (following symbolic links).
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir tells whether a path is a directory (following symbolic links).
func IsDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
