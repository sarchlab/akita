package hostsys

import (
	"context"
	"errors"

	"github.com/shirou/gopsutil/v4/mem"
)

// ErrLocked is returned by Flock when another process holds the lock.
var ErrLocked = errors.New("locked by another process")

// Exit is how a process ended: its exit code, or the signal that killed
// it (Code -1).
type Exit struct {
	Code   int
	Signal string
}

// MemAvailable is the memory available to new processes (MemAvailable of
// /proc/meminfo on Linux), in bytes.
func MemAvailable(ctx context.Context) (uint64, error) {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return 0, err
	}

	return vm.Available, nil
}
