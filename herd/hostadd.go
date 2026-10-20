package herd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// hostNameUnsafe are the characters a derived host name drops.
var hostNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// HostNameFor derives a host name from an ssh address: the host part
// without the user and the domain ("me@server1.example.edu" ->
// "server1").
func HostNameFor(address string) string {
	h := address
	if i := strings.LastIndex(h, "@"); i >= 0 {
		h = h[i+1:]
	}

	if i := strings.Index(h, ":"); i >= 0 {
		h = h[:i]
	}

	if net := strings.Trim(h, "[]"); strings.Count(net, ".") != 3 { // keep IPv4 addresses whole
		h, _, _ = strings.Cut(h, ".")
	}

	return strings.Trim(hostNameUnsafe.ReplaceAllString(h, "-"), "-")
}

// addressRe is what an ssh address may look like: a host, an
// ~/.ssh/config alias, or user@host, optionally with a port.
var addressRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._@:-]*$`)

// ValidateAddress rejects addresses that are not a host, an alias, or
// user@host (in particular, ones that ssh would read as options).
func ValidateAddress(address string) error {
	if !addressRe.MatchString(address) {
		return fmt.Errorf("%q is not an ssh address (a host, an alias in ~/.ssh/config, or user@host)", address)
	}

	return nil
}

// Suggested queue settings: half the CPUs, and a tenth of the memory (at
// least 4 GB) kept free.
const (
	suggestReserveShare = 0.1
	suggestReserveMinGB = 4
)

// SuggestHost is the host entry that host add proposes for a scanned
// machine: ssh at address, with root (omitted when it is the default)
// and a queue sized to the machine.
func SuggestHost(name, address, root string, scan *HostScan) HostConfig {
	h := HostConfig{Name: name, Type: HostSSH, Address: address}
	if root != DefaultRoot {
		h.Root = root
	}

	if scan != nil && scan.CPUs > 0 {
		h.Queue = &QueueConfig{
			Slots:        max(1, scan.CPUs/2),
			MemReserveGB: math.Max(suggestReserveMinGB, math.Round(scan.MemTotalGB*suggestReserveShare)),
		}
	}

	return h
}

// PickRoot is the root host add proposes for a scanned machine: ~/exp
// when it holds workspaces or nothing does, else the root with the most
// sessions and results.
func PickRoot(s *HostScan) string {
	best, most := DefaultRoot, -1

	for _, r := range s.Roots {
		if r.Root == DefaultRoot {
			return r.Root
		}

		n := 0
		for _, p := range r.Projects {
			n += p.Sessions + p.Results
		}

		if n > most {
			best, most = r.Root, n
		}
	}

	return best
}

// AddHosts appends hosts to the user config file at path, keeping its
// other entries and keys; a missing file is created with the local host
// first (the hosts it stood for). The previous file is kept as
// path + ".bak".
func AddHosts(path string, hosts []HostConfig) error {
	doc := map[string]json.RawMessage{}

	data, err := os.ReadFile(path)

	switch {
	case errors.Is(err, fs.ErrNotExist):
		local, err := json.Marshal([]HostConfig{{Name: HostLocal, Type: HostLocal}})
		if err != nil {
			return err
		}

		doc["hosts"] = local
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}

		if err := os.WriteFile(path+".bak", data, 0o644); err != nil {
			return err
		}
	}

	var list []json.RawMessage
	if raw, ok := doc["hosts"]; ok {
		if err := json.Unmarshal(raw, &list); err != nil {
			return fmt.Errorf("%s: hosts: %w", path, err)
		}
	}

	for _, h := range hosts {
		raw, err := json.Marshal(h)
		if err != nil {
			return err
		}

		list = append(list, raw)
	}

	if doc["hosts"], err = json.Marshal(list); err != nil {
		return err
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}
