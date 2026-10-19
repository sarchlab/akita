package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ws "github.com/sarchlab/akita/v5/herd/workspace"
)

// pathPrelude makes binaries installed with `go install` or in ~/.local/bin
// visible to non-login shells.
const pathPrelude = `PATH="$PATH:$HOME/go/bin:$HOME/.local/bin"
`

// stopWho is who the "[stopped]" marker names.
const stopWho = "akita herd stop"

// aliveFunc defines, for scripts run in a workspace:
//
//   - herd_tmux <tmux session>: true while a tmux session has exactly that
//     name.
//   - herd_owner <name> <tmux session>: true unless the tmux session was
//     started for another session, i.e. its @herd_log option (@exp_log
//     before the rename) names another log (sessions started by the
//     original exp tool have none). Different projects or names can share
//     a tmux session name (TmuxSessionName).
//   - herd_alive <name> <tmux session> [<legacy tmux session>]: true while
//     the session's own tmux session (under its current or legacy name),
//     process, or SLURM job lives.
const aliveFunc = `herd_tmux() {
  command -v tmux >/dev/null 2>&1 && tmux has-session -t "=$1" 2>/dev/null
}
herd_owner() {
  herd_owner_log=$(tmux show-options -qv -t "=$2:" @herd_log 2>/dev/null)
  [ -n "$herd_owner_log" ] || herd_owner_log=$(tmux show-options -qv -t "=$2:" @exp_log 2>/dev/null)
  [ -z "$herd_owner_log" ] || [ "$herd_owner_log" = "$(pwd -P)/logs/$1.log" ]
}
herd_alive() {
  if herd_tmux "$2" && herd_owner "$1" "$2"; then return 0; fi
  if [ -n "$3" ] && herd_tmux "$3" && herd_owner "$1" "$3"; then return 0; fi
  [ -f "logs/$1.job" ] || return 1
  { read -r id; read -r kind; } < "logs/$1.job"
  [ -n "$id" ] || return 1
  case "$kind" in
    slurm) [ -n "$(squeue -h -j "$id" -o %T 2>/dev/null)" ] ;;
    *) kill -0 "$id" 2>/dev/null ;;
  esac
}
`

// Launch starts a session in tmux, or under nohup setsid without tmux.
func (b *baseBackend) Launch(ctx context.Context, spec LaunchSpec) (string, error) {
	if err := ws.ValidateSessionName(spec.Name); err != nil {
		return "", err
	}

	// tmux fails with "server exited unexpectedly" when its server is
	// exiting after its last session ended; retry then.
	useTmux := "command -v tmux >/dev/null 2>&1"
	if b.host.Tmux != nil && !*b.host.Tmux {
		useTmux = "false"
	}

	// The tmux session records the log it writes (@herd_log), so that a
	// session whose tmux name is the same is told apart (see herd_owner).
	sess := ws.TmuxSessionName(spec.Project, spec.Name)
	start := "if " + useTmux + "; then\n" +
		"  for i in 1 2 3 4 5; do\n" +
		"    tmux new-session -d -s " + ws.ShellQuote(sess) +
		" " + ws.ShellQuote("bash -lc "+ws.ShellQuote(b.wrapper(spec))) +
		" \\; set-option -t " + ws.ShellQuote("="+sess+":") + " @herd_log \"$(pwd -P)\"/" +
		ws.ShellQuote(ws.ScriptFile(spec.Name, ws.LogExt)) + " && break\n" +
		"    [ \"$i\" = 5 ] && exit 1; sleep 0.2\n" +
		"  done\n" +
		"else\n" + b.nohupStart(spec) + "fi\n"
	_, err := b.output(ctx, b.launchPrelude(spec)+start)

	return "", err
}

func (b *baseBackend) nohupStart(spec LaunchSpec) string {
	cmd := "nohup bash -lc " + ws.ShellQuote(b.wrapper(spec)) + " </dev/null >/dev/null 2>&1 &"
	job := ws.ShellQuote(ws.ScriptFile(spec.Name, ws.JobExt))

	return "  if command -v setsid >/dev/null 2>&1; then setsid " + cmd + "\n" +
		"  else set -m; " + cmd + "\n  fi\n" +
		"  printf '%s\\n" + ws.JobPID + "\\n' \"$!\" > " + job + "\n"
}

// launchPrelude prepares the workspace and the session's files, failing if
// a live session has the name or another session its tmux session name.
func (b *baseBackend) launchPrelude(spec LaunchSpec) string {
	n := spec.Name
	file := func(ext string) string { return ws.ShellQuote(ws.ScriptFile(n, ext)) }

	var s strings.Builder

	s.WriteString(pathPrelude + aliveFunc)
	wsp := ws.ShellPath(b.Workspace(spec.Project))
	s.WriteString("mkdir -p " + wsp + "/" + ws.LogsDir + " " + wsp + "/" + ws.ResultsDir +
		" && cd " + wsp + " || exit 1\n")

	sess := ws.TmuxSessionName(spec.Project, n)
	s.WriteString("if herd_tmux " + ws.ShellQuote(sess) + " && ! herd_owner " + ws.ShellQuote(n) + " " +
		ws.ShellQuote(sess) + "; then\n" +
		"  printf '%s%s%s\\n' " + ws.ShellQuote("session "+n+": tmux session "+sess+
		" is in use by another project or session (") + " \"$herd_owner_log\" " +
		ws.ShellQuote("); tmux session names replace '.' and ':' with '_' and must be unique, "+
			"so choose another name") + " >&2; exit 3\nfi\n")
	s.WriteString("if herd_alive " + ws.ShellQuote(n) + " " + ws.ShellQuote(sess) + " " +
		ws.ShellQuote(ws.LegacyTmuxSessionName(spec.Project, n)) + "; then\n" +
		"  echo " + ws.ShellQuote("session "+n+" is already running") + " >&2; exit 3\nfi\n")
	s.WriteString("rm -f " + file(ws.HeartbeatExt) + " " + file(ws.JobExt) + "\n")
	s.WriteString("printf '%s\\n' " + ws.ShellQuote(spec.Command) + " > " + file(ws.CmdExt) + "\n")

	if spec.Group != "" {
		s.WriteString("printf '%s\\n' " + ws.ShellQuote(spec.Group) + " > " + file(ws.GroupExt) + "\n")
	}

	s.WriteString(": > " + file(ws.LogExt) + "\n")

	return s.String()
}

// wrapper is the bash script of a session (see workspace.WrapperScript).
func (b *baseBackend) wrapper(spec LaunchSpec) string {
	return ws.WrapperScript(b.host.Name, b.Workspace(spec.Project), spec.Name, spec.Command)
}

// Stop stops a live session after appending the "[stopped]" marker.
func (b *baseBackend) Stop(ctx context.Context, project, name string) error {
	if err := ws.ValidateSessionName(name); err != nil {
		return err
	}

	_, err := b.output(ctx, b.stopScript(project, name))

	var se *scriptError
	if errors.As(err, &se) && strings.Contains(se.Stderr, "is not running") {
		return fmt.Errorf("%s: %s: %w", b.host.Name, name, errNotRunning)
	}

	return err
}

func (b *baseBackend) stopScript(project, name string) string {
	sess := ws.ShellQuote(ws.TmuxSessionName(project, name))
	legacy := ws.ShellQuote(ws.LegacyTmuxSessionName(project, name))
	job := ws.ShellQuote(ws.ScriptFile(name, ws.JobExt))

	var kill strings.Builder

	for _, s := range ws.TmuxSessionNames(project, name) {
		kill.WriteString("if herd_tmux " + ws.ShellQuote(s) + " && herd_owner " + ws.ShellQuote(name) + " " +
			ws.ShellQuote(s) + "; then tmux kill-session -t " + ws.ShellQuote("="+s) + "; fi\n")
	}

	return pathPrelude + aliveFunc +
		"cd " + ws.ShellPath(b.Workspace(project)) + " || exit 1\n" +
		"herd_alive " + ws.ShellQuote(name) + " " + sess + " " + legacy +
		" || { echo " + ws.ShellQuote("session "+name+" is not running") + " >&2; exit 3; }\n" +
		ws.StoppedMarkerScript(name, stopWho) +
		kill.String() +
		"if [ -f " + job + " ]; then\n" +
		"  { read -r id; read -r kind; } < " + job + "\n" +
		"  case \"$kind\" in\n" +
		"    " + ws.JobSlurm + ") scancel \"$id\" ;;\n" +
		"    *) kill -TERM -- \"-$id\" 2>/dev/null || kill -TERM \"$id\" 2>/dev/null\n" +
		"       sleep 1\n" +
		"       if kill -0 \"$id\" 2>/dev/null; then kill -KILL -- \"-$id\" 2>/dev/null || " +
		"kill -KILL \"$id\" 2>/dev/null; fi ;;\n" +
		"  esac\nfi\nexit 0\n"
}
