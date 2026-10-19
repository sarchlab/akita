package workspace

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
)

// SetGroup puts sessions of a workspace into a group; group "-" removes
// them from their group. It returns how many sessions it changed.
func SetGroup(workspace, group string, names []string, out io.Writer) (int, error) {
	logs := LogsPath(herd.ExpandHome(workspace))
	if !hostsys.IsDir(logs) {
		return 0, fmt.Errorf("no workspace %s on this host", workspace)
	}

	changed := 0

	for _, n := range names {
		if ValidateSessionName(n) != nil || !hostsys.Exists(SessionFile(logs, n, LogExt)) {
			fmt.Fprintf(out, "no session %s\n", n)
			continue
		}

		p := SessionFile(logs, n, GroupExt)

		var err error
		if group == "-" {
			err = os.Remove(p)
			if errors.Is(err, fs.ErrNotExist) {
				err = nil
			}
		} else {
			err = os.WriteFile(p, []byte(group+"\n"), 0o644)
		}

		if err != nil {
			return changed, err
		}

		changed++
	}

	return changed, nil
}
