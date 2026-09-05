package simulation

import (
	"errors"
	"os"
	"path/filepath"
)

// Publish completion only after the marker is fully written and closed. A failed
// write must not leave an empty marker that a host could mistake for success.
func markOutputComplete(outputPath string) error {
	marker := outputPath + ".complete"
	file, err := os.CreateTemp(filepath.Dir(marker), filepath.Base(marker)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString("complete\n")
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), marker)
}
