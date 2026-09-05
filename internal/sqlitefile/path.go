// Package sqlitefile encodes literal filesystem paths for SQLite connections.
package sqlitefile

import (
	"net/url"
	"path/filepath"
)

// DSN prevents SQLite from interpreting characters in a filename as connection
// options and silently opening a different file. Callers may append URI options
// to the returned string.
func DSN(filename string) (string, error) {
	abs, err := filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String(), nil
}
