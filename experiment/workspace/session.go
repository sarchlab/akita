package workspace

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/sarchlab/akita/v5/experiment/internal/hostsys"
)

// PrepareSession prepares the files of a session that is about to start
// in a workspace on this machine, as a launch does: it creates logs/ and
// results/, refuses a name whose session is alive (its process or its
// tmux session), removes the old heartbeat and .job, writes the command
// and the group, and empties the log.
func PrepareSession(ctx context.Context, ws, project, name, command, group string) error {
	if err := ValidateSessionName(name); err != nil {
		return err
	}

	logs := LogsPath(ws)
	for _, d := range []string{logs, ResultsPath(ws)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	if j := ReadJob(logs, name); j != nil && j.Kind == JobPID {
		if pid, _ := strconv.Atoi(j.ID); hostsys.PIDAlive(pid) {
			return fmt.Errorf("session %s is already running (pid %d)", name, pid)
		}
	}

	if panes, _ := hostsys.TmuxPanes(ctx); panes != nil {
		if _, ok := panes[TmuxSessionName(project, name)]; ok {
			return fmt.Errorf("tmux session %s exists", TmuxSessionName(project, name))
		}
	}

	for _, ext := range []string{HeartbeatExt, JobExt} {
		if err := os.Remove(SessionFile(logs, name, ext)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	files := map[string]string{CmdExt: command + "\n", LogExt: ""}
	if group != "" {
		files[GroupExt] = group + "\n"
	}

	for ext, content := range files {
		if err := os.WriteFile(SessionFile(logs, name, ext), []byte(content), 0o644); err != nil {
			return err
		}
	}

	return nil
}

// WriteJob writes the .job file of a session started without tmux.
func WriteJob(ws, name string, pid int) error {
	return hostsys.WriteFileAtomic(SessionFile(LogsPath(ws), name, JobExt),
		[]byte(strconv.Itoa(pid)+"\n"+JobPID+"\n"), 0o644)
}
