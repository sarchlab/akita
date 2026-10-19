package workspace

import (
	"fmt"
	"strings"
)

// validPathComponent rejects names that cannot be one component of a path
// in a workspace: empty, ".", "..", or with '/' or NUL.
func validPathComponent(name string) error {
	switch {
	case name == "", name == ".", name == "..":
		return fmt.Errorf("invalid name %q", name)
	case strings.ContainsAny(name, "/\x00"):
		return fmt.Errorf("invalid name %q: it must not contain '/'", name)
	}

	return nil
}

// ValidateSessionName rejects names that cannot be a file name in logs/:
// see validPathComponent; a session name also has no newline and does not
// start with '-'.
func ValidateSessionName(name string) error {
	switch {
	case strings.ContainsAny(name, "/\n\x00"):
		return fmt.Errorf("invalid session name %q: it must not contain '/' or a newline", name)
	case validPathComponent(name) != nil:
		return fmt.Errorf("invalid session name %q", name)
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("invalid session name %q: it must not start with '-'", name)
	}

	return nil
}

// ValidateRunName rejects project and result directory names that a
// request may not name: see validPathComponent, and no '\'.
func ValidateRunName(name string) error {
	if err := validPathComponent(name); err != nil {
		return err
	}

	if strings.Contains(name, `\`) {
		return fmt.Errorf("invalid name %q: it must not contain '\\'", name)
	}

	return nil
}

// Prefixes of the tmux session names: the current one, and the former one
// (akita exp and the original exp tool), still recognized for sessions
// started before the rename.
const (
	tmuxPrefix       = "herd-"
	legacyTmuxPrefix = "exp-"
)

// TmuxSessionName is the tmux session of a session: herd-<project>-<name>
// with "." and ":" replaced by "_". Two sessions can map to the same name
// (project "a" session "b-c" and project "a-b" session "c", or sessions
// "x.y" and "x_y"); every lookup uses the exact name, and a launch refuses
// a name in use by another session.
func TmuxSessionName(project, name string) string {
	return tmuxName(tmuxPrefix, project, name)
}

// LegacyTmuxSessionName is the tmux session name a session got before the
// rename: exp-<project>-<name>, otherwise like TmuxSessionName.
func LegacyTmuxSessionName(project, name string) string {
	return tmuxName(legacyTmuxPrefix, project, name)
}

// TmuxSessionNames are the tmux session names a session may have: the
// current one first, then the legacy one.
func TmuxSessionNames(project, name string) []string {
	return []string{TmuxSessionName(project, name), LegacyTmuxSessionName(project, name)}
}

// TmuxPane finds a session's tmux session among panes (tmux session name
// to pane pid), under its current or its legacy name.
func TmuxPane(panes map[string]int, project, name string) (pid int, ok bool) {
	for _, s := range TmuxSessionNames(project, name) {
		if pid, ok := panes[s]; ok {
			return pid, true
		}
	}

	return 0, false
}

func tmuxName(prefix, project, name string) string {
	return strings.NewReplacer(".", "_", ":", "_").Replace(prefix + project + "-" + name)
}
