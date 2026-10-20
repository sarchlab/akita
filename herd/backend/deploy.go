package backend

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"sync"

	"github.com/sarchlab/akita/v5/herd/deploy"
	ws "github.com/sarchlab/akita/v5/herd/workspace"
)

// DefaultAkitaDir is where akita herd puts its akita on a host.
const DefaultAkitaDir = "~/.cache/akita-herd"

// missingAkita is the exit status of a script whose deployed akita is
// not there yet.
const missingAkita = 97

// Deployer puts this akita on hosts whose config does not name one, so
// that nothing has to be installed there by hand. Key names the copies
// (<akita_dir>/<Key()>/akita; called only when a host needs it), Binary returns a local akita for a platform,
// and Notify, when set, is told of each upload.
type Deployer struct {
	Key    func() string
	Binary func(ctx context.Context, goos, goarch string) (string, error)
	Notify func(host, path string)
}

var (
	deployerMu sync.Mutex
	deployer   *Deployer
)

// SetDeployer sets how backends put akita on hosts (nil: they run the
// akita on the host's PATH).
func SetDeployer(d *Deployer) {
	deployerMu.Lock()
	defer deployerMu.Unlock()

	deployer = d
}

func currentDeployer() *Deployer {
	deployerMu.Lock()
	defer deployerMu.Unlock()

	return deployer
}

// deployState remembers, per backend, that its akita is in place.
type deployState struct {
	mu   sync.Mutex
	done bool
}

// managedAkita is the path of the deployed akita on the host, or "" when
// the host's akita is configured, the host is this machine, or nothing
// deploys.
func (b *baseBackend) managedAkita() string {
	d := currentDeployer()
	if _, local := b.runner.(localRunner); local || b.host.Akita != "" || d == nil {
		return ""
	}

	dir := b.host.AkitaDir
	if dir == "" {
		dir = DefaultAkitaDir
	}

	return path.Join(strings.TrimRight(dir, "/"), d.Key(), "akita")
}

// akitaScript runs akita with args: the deployed one, which must exist
// (else the script exits with missingAkita), or the configured one.
func (b *baseBackend) akitaScript(args []string) string {
	m := b.managedAkita()
	if m == "" {
		return pathPrelude + "exec " + ws.ShellPath(b.akitaPath()) + " " + ws.ShellJoin(args)
	}

	return pathPrelude + "p=" + ws.ShellPath(m) + "\n" +
		"if [ -x \"$p\" ]; then exec \"$p\" " + ws.ShellJoin(args) + "; fi\n" +
		fmt.Sprintf("exit %d\n", missingAkita)
}

// runAkita runs a script made by akitaScript; when the deployed akita is
// missing, it deploys it and runs the script again, or, when deploying
// fails, runs the akita on the host's PATH instead.
func (b *baseBackend) runAkita(ctx context.Context, args []string) ([]byte, error) {
	out, err := b.output(ctx, b.akitaScript(args))
	if !exitedWith(err, missingAkita) {
		return out, err
	}

	if derr := b.deployAkita(ctx); derr != nil {
		out, err = b.output(ctx, pathPrelude+"command -v akita >/dev/null 2>&1 || exit "+
			fmt.Sprint(missingAkita)+"\nexec akita "+ws.ShellJoin(args))
		if exitedWith(err, missingAkita) {
			return nil, fmt.Errorf("%s: no akita there, and putting one there failed: %w", b.host.Name, derr)
		}

		return out, err
	}

	return b.output(ctx, b.akitaScript(args))
}

// ensureAkita returns the akita to start on the host, deploying it when
// needed (the configured or PATH akita when it is not deployed).
func (b *baseBackend) ensureAkita(ctx context.Context) (string, error) {
	m := b.managedAkita()
	if m == "" {
		return b.akitaPath(), nil
	}

	_, err := b.output(ctx, "[ -x "+ws.ShellPath(m)+" ] || exit "+fmt.Sprint(missingAkita))
	switch {
	case err == nil:
		return m, nil
	case !exitedWith(err, missingAkita):
		return "", err
	}

	if derr := b.deployAkita(ctx); derr != nil {
		if _, err := b.output(ctx, pathPrelude+"command -v akita >/dev/null 2>&1"); err == nil {
			return "akita", nil
		}

		return "", fmt.Errorf("%s: no akita there, and putting one there failed: %w", b.host.Name, derr)
	}

	return m, nil
}

// deployAkita puts this akita on the host, built for its platform.
func (b *baseBackend) deployAkita(ctx context.Context) error {
	b.deployed.mu.Lock()
	defer b.deployed.mu.Unlock()

	if b.deployed.done {
		return nil
	}

	d, m := currentDeployer(), b.managedAkita()
	if d == nil || m == "" {
		return errors.New("nothing deploys akita")
	}

	out, err := b.output(ctx, "uname -s; uname -m")
	if err != nil {
		return err
	}

	f := strings.Fields(string(out))
	if len(f) < 2 {
		return fmt.Errorf("uname printed %q", out)
	}

	goos, goarch, err := deploy.Platform(f[0], f[1])
	if err != nil {
		return err
	}

	local, err := d.Binary(ctx, goos, goarch)
	if err != nil {
		return err
	}

	if err := b.Upload(ctx, local, m); err != nil {
		return err
	}

	if d.Notify != nil {
		d.Notify(b.host.Name, m)
	}

	b.deployed.done = true

	return nil
}

// exitedWith tells whether a script failed with an exit status.
func exitedWith(err error, code int) bool {
	var se *scriptError
	if !errors.As(err, &se) {
		return false
	}

	var ee *exec.ExitError

	return errors.As(se.Err, &ee) && ee.ExitCode() == code
}
