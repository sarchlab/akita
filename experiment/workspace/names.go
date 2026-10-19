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

// TmuxSessionName is the tmux session of a session: exp-<project>-<name>
// with "." and ":" replaced by "_" (the original exp tool's names). Two
// sessions can map to the same name (project "a" session "b-c" and project
// "a-b" session "c", or sessions "x.y" and "x_y"); every lookup uses the
// exact name, and a launch refuses a name in use by another session.
func TmuxSessionName(project, name string) string {
	return strings.NewReplacer(".", "_", ":", "_").Replace("exp-" + project + "-" + name)
}
