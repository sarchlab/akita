package workspace

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/experiment"
)

// The log markers. A session's log starts with
// "[start] <date>  <host>  <git revision>" and ends with
// "[exit <code>] <date>", or gets "[stopped] <date>  <who>" from stop.
var (
	startRe   = regexp.MustCompile(`(?m)^\[start\] (.+?)  `)
	exitRe    = regexp.MustCompile(`(?m)^\[exit (-?\d+)\] (.+?)\r?$`)
	stoppedRe = regexp.MustCompile(`(?m)^\[stopped\] (.+?)(?:  |\r?$)`)
	spacesRe  = regexp.MustCompile(`\s+`)
)

// logDate is the shell expression of the date in the markers: what `date`
// prints in an English locale ("Fri Oct 16 05:49:14 PM UTC 2026"),
// whatever the host's locale, so that parseLogTime and the original exp
// tool's parser read it.
const logDate = `$(LC_ALL=C date '+%a %b %e %I:%M:%S %p %Z %Y')`

// WrapperScript is the bash script that runs a session's command in a
// workspace and writes logs/<name>.log with the "[start] <date>  <host>
// <git revision>" and "[exit <code>] <date>" markers. The command runs in
// a subshell, so `exit N` is recorded. EXP_HEARTBEAT points to
// logs/<name>.heartbeat.
func WrapperScript(host, workspace, name, command string) string {
	logPath := ShellQuote(ScriptFile(name, LogExt))
	heartbeat := ShellQuote("/" + ScriptFile(name, HeartbeatExt))

	return "cd " + ShellPath(workspace) + " || exit 1\n" +
		"{ printf '[start] %s  %s  %s\\n' \"" + logDate + "\" " + ShellQuote(host) +
		" \"$(git rev-parse --short HEAD 2>/dev/null)\"; " +
		"export EXP_HEARTBEAT=\"$PWD\"" + heartbeat + "; " +
		"( eval " + ShellQuote(command) + " ); " +
		"echo \"[exit $?] " + logDate + "\"; } 2>&1 | tee -a " + logPath + "\n"
}

// StoppedMarkerScript is the shell command, run in the workspace, that
// appends the "[stopped] <date>  <who>" marker to a session's log.
func StoppedMarkerScript(name, who string) string {
	return "printf '[stopped] %s  %s\\n' \"" + logDate + "\" " + ShellQuote(who) +
		" >> " + ShellQuote(ScriptFile(name, LogExt)) + "\n"
}

// LogState is what a session's log markers say about it.
type LogState struct {
	State    string // "ok", "failed", "stopped", or "died" (no marker)
	ExitCode *int
	Start    *float64
	End      *float64
}

// Finished tells whether the log has an exit or stop marker.
func (s LogState) Finished() bool { return s.State != "died" }

// parseLogMarkers reads the markers of a session from the head and the
// tail of its log.
func parseLogMarkers(head, tail string) LogState {
	st := LogState{State: "died"}
	if m := startRe.FindStringSubmatch(head); m != nil {
		st.Start = parseLogTime(m[1])
	}

	if ms := exitRe.FindAllStringSubmatch(tail, -1); len(ms) > 0 {
		m := ms[len(ms)-1]
		code, _ := strconv.Atoi(m[1])
		st.ExitCode, st.End = &code, parseLogTime(m[2])
		st.State = "failed"

		if code == 0 {
			st.State = "ok"
		}

		return st
	}

	if m := stoppedRe.FindStringSubmatch(tail); m != nil {
		st.State, st.End = "stopped", parseLogTime(m[1])
	}

	return st
}

// parseLogTime parses the dates of log markers (the output of `date`,
// e.g. "Thu Oct 15 02:59:54 PM UTC 2026"). It returns nil when the text
// is not a date.
func parseLogTime(s string) *float64 {
	s = spacesRe.ReplaceAllString(strings.TrimSpace(s), " ")
	for _, layout := range []string{
		"Mon Jan 2 03:04:05 PM MST 2006",
		"Mon Jan 2 15:04:05 MST 2006",
		"Mon Jan 2 03:04:05 PM -0700 2006",
		"Mon Jan 2 15:04:05 -0700 2006",
		time.RFC3339,
	} {
		// Zone abbreviations of the probing machine's zone resolve to it.
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil { //nolint:gosmopolitan
			return new(experiment.UnixSeconds(t))
		}
	}

	return nil
}
