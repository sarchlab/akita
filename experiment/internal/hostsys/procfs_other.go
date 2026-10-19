//go:build !linux

package hostsys

// procfsTable is nil: only Linux has /proc.
func procfsTable() *ProcTable { return nil }

// NewestWritten returns 0: open files are read from /proc on Linux only.
func NewestWritten(int) float64 { return 0 }
