package workspace

import "strings"

// shellSafe are the characters that need no quoting in a POSIX shell word.
const shellSafe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./=:,+@%"

// ShellQuote quotes s for a POSIX shell (words of safe characters stay as
// they are).
func ShellQuote(s string) string {
	if s != "" && strings.Trim(s, shellSafe) == "" {
		return s
	}

	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ShellJoin quotes and joins arguments.
func ShellJoin(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = ShellQuote(a)
	}

	return strings.Join(q, " ")
}

// ShellPath quotes a path for a shell, expanding a leading "~" there.
func ShellPath(p string) string {
	switch {
	case p == "~":
		return `"$HOME"`
	case strings.HasPrefix(p, "~/"):
		return `"$HOME"/` + ShellQuote(p[2:])
	default:
		return ShellQuote(p)
	}
}
