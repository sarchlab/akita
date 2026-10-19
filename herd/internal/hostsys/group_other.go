//go:build !unix

package hostsys

import (
	"errors"
	"os"
	"time"
)

var errUnsupported = errors.New("not supported on this system")

// Flock is not supported here.
func Flock(string, time.Duration) (*os.File, error) { return nil, errUnsupported }

// StartGroup is not supported here.
func StartGroup(string, []string, []string) (int, <-chan Exit, error) { return 0, nil, errUnsupported }

// GroupAlive is false here.
func GroupAlive(int) bool { return false }

// PauseGroup is not supported here.
func PauseGroup(int) error { return errUnsupported }

// ResumeGroup is not supported here.
func ResumeGroup(int) error { return errUnsupported }

// Terminate is not supported here.
func Terminate(int) error { return errUnsupported }

// SignalName is "" here.
func SignalName(int) string { return "" }
