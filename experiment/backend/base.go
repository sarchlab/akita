package backend

import (
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sarchlab/akita/v5/experiment"
	ws "github.com/sarchlab/akita/v5/experiment/workspace"
)

// runner runs bash scripts on a host.
type runner interface {
	// run runs a bash script; the script must not read stdin.
	run(ctx context.Context, script string, stdout, stderr io.Writer) error
	// rsyncPath is how rsync names a path on the host.
	rsyncPath(p string) string
}

// wrapStdin makes bash read a whole script before running it, so that
// commands of the script cannot consume the rest of it from stdin.
func wrapStdin(script string) string {
	return "{\n" + script + "\n} </dev/null\n"
}

// baseBackend holds what the three backends share.
type baseBackend struct {
	host      experiment.HostConfig
	runner    runner
	workspace func(project string) string // nil: <root>/<project>
}

// Host returns the host's configuration.
func (b *baseBackend) Host() experiment.HostConfig { return b.host }

// root is the workspace root, "~/" kept for the host to expand.
func (b *baseBackend) root() string {
	if b.host.Root != "" {
		return strings.TrimRight(b.host.Root, "/")
	}

	return experiment.DefaultRoot
}

// Workspace returns <root>/<project>, or the local host's override.
func (b *baseBackend) Workspace(project string) string {
	if b.workspace != nil {
		return b.workspace(project)
	}

	return path.Join(b.root(), project)
}

// output runs a script and returns its stdout.
func (b *baseBackend) output(ctx context.Context, script string) ([]byte, error) {
	var stdout, stderr bytes.Buffer

	err := b.runner.run(ctx, script, &stdout, &stderr)
	if err != nil {
		return stdout.Bytes(), &scriptError{Host: b.host.Name, Stderr: stderr.String(), Err: err}
	}

	return stdout.Bytes(), nil
}

// akitaPath is the akita binary on the host: the host config's, this
// executable for hosts reached without ssh, or "akita" on PATH.
func (b *baseBackend) akitaPath() string {
	if b.host.Akita != "" {
		return b.host.Akita
	}

	if _, local := b.runner.(localRunner); local {
		if exe, err := os.Executable(); err == nil {
			return exe
		}
	}

	return "akita"
}

// Akita runs `akita <args...>` on the host.
func (b *baseBackend) Akita(ctx context.Context, args ...string) ([]byte, error) {
	script := pathPrelude + "exec " + ws.ShellPath(b.akitaPath()) + " " + ws.ShellJoin(args)
	return b.output(ctx, script)
}

// Shell runs a command in a project's workspace.
func (b *baseBackend) Shell(ctx context.Context, project, command string) ([]byte, error) {
	return b.output(ctx, b.inWorkspace(project, command))
}

// Stream runs a command in a project's workspace, streaming its output.
func (b *baseBackend) Stream(ctx context.Context, project, command string,
	stdout, stderr io.Writer,
) error {
	return b.runner.run(ctx, b.inWorkspace(project, command), stdout, stderr)
}

func (b *baseBackend) inWorkspace(project, command string) string {
	return pathPrelude + "cd " + ws.ShellPath(b.Workspace(project)) + " || exit 1\n" + command
}

// Pull copies the workspace's logs/ and results/ to localDir.
func (b *baseBackend) Pull(ctx context.Context, project, localDir string) error {
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}

	wsp := b.Workspace(project)

	out, err := b.output(ctx, "cd "+ws.ShellPath(wsp)+" || exit 0\n"+
		"for d in "+ws.LogsDir+" "+ws.ResultsDir+"; do [ -d \"$d\" ] && echo \"$d\"; done; exit 0\n")
	if err != nil {
		return err
	}

	for _, d := range strings.Fields(string(out)) {
		src := b.runner.rsyncPath(path.Join(wsp, d))
		if err := runRsync(ctx, []string{"-az", src, localDir + "/"}); err != nil {
			return err
		}
	}

	return nil
}

// Upload copies a file to the host unless the destination exists; rsync
// writes a temporary file and renames it, so a partial upload is never
// seen under the destination's name.
func (b *baseBackend) Upload(ctx context.Context, localPath, hostPath string) error {
	dst := ws.ShellPath(hostPath)

	out, err := b.output(ctx, "if [ -e "+dst+" ]; then echo have; else mkdir -p "+
		ws.ShellPath(path.Dir(hostPath))+" || exit 1; fi\n")
	if err != nil || strings.TrimSpace(string(out)) == "have" {
		return err
	}

	return runRsync(ctx, []string{"-a", localPath, b.runner.rsyncPath(hostPath)})
}

// StartAkita starts akita in a tmux session, or under nohup setsid without
// tmux (or when the host config disables it).
func (b *baseBackend) StartAkita(ctx context.Context, session string, args ...string) error {
	run := strings.TrimSpace(pathPrelude) + " exec " + ws.ShellPath(b.akitaPath()) + " " + ws.ShellJoin(args)

	useTmux := "command -v tmux >/dev/null 2>&1"
	if b.host.Tmux != nil && !*b.host.Tmux {
		useTmux = "false"
	}

	sess := ws.ShellQuote(session)
	_, err := b.output(ctx, "if "+useTmux+"; then\n"+
		"  if tmux has-session -t "+ws.ShellQuote("="+session)+" 2>/dev/null; then\n"+
		"    echo "+ws.ShellQuote("tmux session "+session+" exists")+" >&2; exit 3\n  fi\n"+
		"  tmux new-session -d -s "+sess+" "+ws.ShellQuote(run)+"\n"+
		"else\n"+
		"  (setsid nohup bash -c "+ws.ShellQuote(run)+" </dev/null >/dev/null 2>&1 &)\n"+
		"fi\n")

	return err
}

// PullRun copies results/<name>/ and logs/<name>.* of a workspace to
// localDir/results/ and localDir/logs/.
func (b *baseBackend) PullRun(ctx context.Context, project, name, localDir string, exclude []string) error {
	if err := ws.ValidateSessionName(name); err != nil {
		return err
	}

	wsp := b.Workspace(project)
	logs, results := filepath.Join(localDir, ws.LogsDir), filepath.Join(localDir, ws.ResultsDir)

	for _, d := range []string{logs, results} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	out, err := b.output(ctx, "cd "+ws.ShellPath(wsp)+" || exit 0\n[ -d "+
		ws.ShellQuote(ws.ResultsDir+"/"+name)+" ] && echo results; exit 0\n")
	if err != nil {
		return err
	}

	if strings.TrimSpace(string(out)) == "results" {
		args := []string{"-a"}
		for _, e := range exclude {
			args = append(args, "--exclude", e)
		}

		src := b.runner.rsyncPath(path.Join(wsp, ws.ResultsDir, name))
		if err := runRsync(ctx, append(args, src, results+"/")); err != nil {
			return err
		}
	}

	args := []string{"-a"}
	for _, ext := range ws.SessionExts {
		args = append(args, "--include=/"+rsyncEscape(name+ext))
	}

	src := b.runner.rsyncPath(path.Join(wsp, ws.LogsDir)) + "/"

	return runRsync(ctx, append(args, "--exclude=*", src, logs+"/"))
}

// rsyncEscape escapes the wildcard characters of an rsync pattern.
func rsyncEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`).Replace(s)
}
