//go:build unix

package hostsys

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// lockPoll is how often a blocked Flock retries.
const lockPoll = 50 * time.Millisecond

// Flock takes an exclusive flock on path (created if missing), waiting at
// most timeout; a zero timeout tries once. The file is opened close-on-exec,
// so processes started while it is held do not inherit the lock.
func Flock(path string, timeout time.Duration) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)

	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}

		if !errors.Is(err, syscall.EWOULDBLOCK) || !time.Now().Before(deadline) {
			f.Close()
			return nil, ErrLocked
		}

		time.Sleep(lockPoll)
	}
}

// StartGroup starts argv in dir as the leader of a new session (so its
// process group id is its pid), with stdin and output on /dev/null. The
// returned channel delivers its exit once it ends.
func StartGroup(dir string, argv, env []string) (int, <-chan Exit, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir, cmd.Env = dir, env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return 0, nil, err
	}

	done := make(chan Exit, 1)

	go func() {
		_ = cmd.Wait()

		ex := Exit{Code: -1}
		if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
			switch {
			case ws.Exited():
				ex.Code = ws.ExitStatus()
			case ws.Signaled():
				ex.Signal = unix.SignalName(ws.Signal())
			}
		}

		done <- ex
	}()

	return cmd.Process.Pid, done, nil
}

// GroupAlive tells whether a process group has a process.
func GroupAlive(pgid int) bool {
	if pgid <= 1 {
		return false
	}

	err := syscall.Kill(-pgid, 0)

	return err == nil || errors.Is(err, syscall.EPERM)
}

// PauseGroup stops every process of a process group (SIGSTOP).
func PauseGroup(pgid int) error { return signalGroup(pgid, syscall.SIGSTOP) }

// ResumeGroup continues every process of a process group (SIGCONT).
func ResumeGroup(pgid int) error { return signalGroup(pgid, syscall.SIGCONT) }

// Terminate sends SIGTERM to one process.
func Terminate(pid int) error {
	if pid <= 1 {
		return errors.New("bad pid")
	}

	return syscall.Kill(pid, syscall.SIGTERM)
}

func signalGroup(pgid int, sig syscall.Signal) error {
	if pgid <= 1 {
		return errors.New("bad process group")
	}

	return syscall.Kill(-pgid, sig)
}

// SignalName is the name of a signal number of this machine ("SIGBUS"),
// or "".
func SignalName(n int) string { return unix.SignalName(syscall.Signal(n)) }
