package backend

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sarchlab/akita/v5/herd"
)

// Scan runs `akita herd _local scan` on a host (putting akita there when
// needed) and returns what it found; the host's root is looked at too.
func Scan(ctx context.Context, b Backend) (*herd.HostScan, error) {
	args := []string{"herd", "_local", "scan"}
	if root := b.Host().Root; root != "" {
		args = append(args, "--root", root)
	}

	data, err := b.Akita(ctx, args...)
	if err != nil {
		return nil, err
	}

	scan := &herd.HostScan{}
	if err := json.Unmarshal(data, scan); err != nil {
		return nil, fmt.Errorf("%s: bad scan: %w", b.Host().Name, err)
	}

	return scan, nil
}
