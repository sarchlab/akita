// Package workspace owns the on-disk format of a workspace, which the
// original exp tool also reads and writes:
//
//	<workspace>/
//	  logs/<name>.{log,cmd,group,heartbeat,job}   a session's files
//	  logs/archived/                               archived sessions
//	  results/<run>/                               result directories
//	  results/RENAMES.tsv                          applied renames
//	  .herd_archive.json                           archive records
//	                                               (.exp_archive.json before)
//
// It defines the layout, the log markers and the script that writes them,
// the .job and heartbeat files, the rule that tells whether a session is
// alive, and the operations that change a workspace: archive, unarchive,
// group, and renaming result directories.
package workspace

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
)

// Directories and files of a workspace.
const (
	LogsDir     = "logs"
	ArchivedDir = "archived" // under LogsDir
	ResultsDir  = "results"
	ArchiveFile = ".herd_archive.json"
	// LegacyArchiveFile is ArchiveFile's former name (akita exp and the
	// original exp tool), read while ArchiveFile does not exist.
	LegacyArchiveFile = ".exp_archive.json"
	renamesFile       = "RENAMES.tsv" // under ResultsDir
)

// The files of a session <name> in logs/ (or logs/archived/).
const (
	LogExt       = ".log"       // output with the markers
	CmdExt       = ".cmd"       // the command
	GroupExt     = ".group"     // optional group name
	HeartbeatExt = ".heartbeat" // optional heartbeat JSON
	JobExt       = ".job"       // SLURM job id or pid (see parseJob)
)

// SessionExts are the files of a session; ".log" last, because sessions
// are listed by it.
var SessionExts = []string{CmdExt, GroupExt, HeartbeatExt, JobExt, LogExt}

// recordTime is the time format of the archive and rename records (the
// original exp tool's, local time).
const recordTime = "2006-01-02 15:04:05"

// nothingChanged ends the messages of refused operations.
const nothingChanged = "\nNothing was changed."

// maxGroupBytes bounds the .group file read.
const maxGroupBytes = 1000

// LogsPath is logs/ of a workspace.
func LogsPath(ws string) string { return filepath.Join(ws, LogsDir) }

// ArchivedPath is logs/archived/ of a workspace.
func ArchivedPath(ws string) string { return filepath.Join(ws, LogsDir, ArchivedDir) }

// ResultsPath is results/ of a workspace.
func ResultsPath(ws string) string { return filepath.Join(ws, ResultsDir) }

// SessionFile is the file of session name with extension ext in a logs
// directory (logs/ or logs/archived/).
func SessionFile(dir, name, ext string) string { return filepath.Join(dir, name+ext) }

// ScriptFile is the file of session name relative to the workspace, as
// scripts run in the workspace name it.
func ScriptFile(name, ext string) string { return LogsDir + "/" + name + ext }

// ReadGroup is the group of a session ("" when it has none).
func ReadGroup(dir, name string) string {
	return strings.TrimSpace(hostsys.ReadText(SessionFile(dir, name, GroupExt), maxGroupBytes))
}

// Entry is a session found in a logs directory: a <name>.log, or a
// <name>.job without a log yet (a queued batch job).
type Entry struct {
	Name   string
	HasLog bool
}

// ListEntries lists the sessions of a logs directory, sorted by name;
// with jobs, also the .job files without a log.
func ListEntries(dir string, jobs bool) []Entry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	logs, jobFiles := map[string]bool{}, map[string]bool{}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		if n, ok := strings.CutSuffix(e.Name(), LogExt); ok {
			logs[n] = true
		} else if n, ok := strings.CutSuffix(e.Name(), JobExt); ok {
			jobFiles[n] = true
		}
	}

	out := make([]Entry, 0, len(logs))

	for n := range logs {
		out = append(out, Entry{Name: n, HasLog: true})
	}

	for n := range jobFiles {
		if jobs && !logs[n] {
			out = append(out, Entry{Name: n})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// ResultDir is a directory of results/.
type ResultDir struct {
	Name  string
	MTime time.Time
}

// ResultDirs lists the directories of a results directory (following
// symbolic links), sorted by name.
func ResultDirs(results string) []ResultDir {
	entries, err := os.ReadDir(results)
	if err != nil {
		return nil
	}

	var out []ResultDir

	for _, e := range entries {
		st, err := os.Stat(filepath.Join(results, e.Name()))
		if err == nil && st.IsDir() {
			out = append(out, ResultDir{Name: e.Name(), MTime: st.ModTime()})
		}
	}

	return out
}

// resultNames are the names of ResultDirs.
func resultNames(results string) []string {
	dirs := ResultDirs(results)
	names := make([]string, len(dirs))

	for i, d := range dirs {
		names[i] = d.Name
	}

	return names
}
