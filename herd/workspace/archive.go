package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
)

// Log reading limits of the archive: LogHeadBytes for the start marker,
// markerTailBytes for the exit and stop markers, hintBytes at both ends of
// a log for the result directories it mentions; maxCmdBytes of a command.
const (
	LogHeadBytes    = 4096
	markerTailBytes = 64 << 10
	hintBytes       = 2 << 20
	maxCmdBytes     = 1 << 20
)

// ArchiveRequest is an archive or unarchive operation on a workspace of
// this machine. See `akita herd archive --help`.
type ArchiveRequest struct {
	Workspace string    // workspace path ("~/" is expanded)
	Project   string    // project name, for tmux session names
	Tokens    []string  // group or session names
	Results   []string  // result directories named with --results
	DryRun    bool      // only print what would be done
	Out       io.Writer // messages
}

// ParseArchiveArgs splits "[-n] <token>... [--results <run>...]".
func ParseArchiveArgs(args []string) (tokens, results []string, dryRun bool) {
	inResults := false

	for _, a := range args {
		switch {
		case a == "-n" || a == "--dry-run":
			dryRun = true
		case a == "--results":
			inResults = true
		case inResults:
			results = append(results, filepath.Base(strings.TrimRight(a, "/")))
		default:
			tokens = append(tokens, a)
		}
	}

	return tokens, results, dryRun
}

// archiveCtx is the state of one archive operation.
type archiveCtx struct {
	ArchiveRequest

	root    string
	logs    string
	arch    string
	results string
	db      *archiveDB
	now     string
}

func newArchiveCtx(req ArchiveRequest) (*archiveCtx, error) {
	root := herd.ExpandHome(req.Workspace)
	if req.Project == "" {
		req.Project = filepath.Base(root)
	}

	if !hostsys.IsDir(root) {
		return nil, fmt.Errorf("no workspace %s on this host (run akita herd from the project root)",
			req.Workspace)
	}

	if req.Out == nil {
		req.Out = io.Discard
	}

	a := &archiveCtx{
		ArchiveRequest: req, root: root,
		logs:    LogsPath(root),
		arch:    ArchivedPath(root),
		results: ResultsPath(root),
		now:     time.Now().Format(recordTime),
	}

	db, err := loadArchiveDB(filepath.Join(root, ArchiveFile))
	if err != nil {
		return nil, err
	}

	a.db = db

	return a, nil
}

func (a *archiveCtx) printf(format string, args ...any) {
	fmt.Fprintf(a.Out, format, args...)
}

// Archive archives finished sessions (by session or group name): their
// files move to logs/archived/ and they, their groups, and their result
// directories are recorded in .herd_archive.json. Result directories stay
// in place. Without tokens and results it lists what is archived.
func Archive(ctx context.Context, req ArchiveRequest) error {
	a, err := newArchiveCtx(req)
	if err != nil {
		return err
	}

	if len(a.Tokens) == 0 && len(a.Results) == 0 {
		a.list()
		return nil
	}

	return a.archive(ctx)
}

// sessionFiles is what archive needs to know about a session.
type sessionFiles struct {
	group string
	cmd   string
}

// readSessions maps the name of each <name>.log in dir to its files.
func readSessions(dir string) map[string]sessionFiles {
	out := map[string]sessionFiles{}

	for _, e := range ListEntries(dir, false) {
		out[e.Name] = sessionFiles{
			group: ReadGroup(dir, e.Name),
			cmd:   hostsys.ReadText(SessionFile(dir, e.Name, CmdExt), maxCmdBytes),
		}
	}

	return out
}

// hasFiles tells whether any file of session name is in dir.
func hasFiles(dir, name string) bool {
	for _, e := range SessionExts {
		if _, err := os.Lstat(SessionFile(dir, name, e)); err == nil {
			return true
		}
	}

	return false
}

// archivePlan is what archive will do.
type archivePlan struct {
	targets map[string]sessionFiles // session name -> files
	groups  map[string]bool         // group tokens
	names   map[string]bool         // session-name tokens
	oldRefs []string                // refs of already archived tokens
	notes   []string
	rename  map[string]string   // session -> archived name
	auto    map[string][]string // session -> result dirs its command names
}

func (a *archiveCtx) archive(ctx context.Context) error {
	p, err := a.planArchive(ctx)
	if err != nil {
		return err
	}

	for _, n := range p.notes {
		a.printf("%s\n", n)
	}

	if len(p.targets) == 0 && len(a.Results) == 0 {
		return nil
	}

	a.renameArchived(p)
	a.printArchivePlan(p)
	a.printHints(p)

	before := len(a.db.section(secResults))
	err = a.applyArchive(p)

	if !a.DryRun {
		if serr := a.db.save(); err == nil {
			err = serr
		}
	}

	if err != nil {
		return err
	}

	a.printf("%s %d result directories as archived (hidden in the web UI by default; "+
		"the directories are not moved).%s\n",
		pick(a.DryRun, "Would mark", "Marked"), len(a.db.section(secResults))-before,
		pick(a.DryRun, dryRunNote, ""))

	return nil
}

func (a *archiveCtx) planArchive(ctx context.Context) (*archivePlan, error) {
	active, archived := readSessions(a.logs), readSessions(a.arch)
	p := &archivePlan{
		targets: map[string]sessionFiles{}, groups: map[string]bool{},
		names: map[string]bool{},
	}

	var errs []string

	for _, t := range a.Tokens {
		if e := a.planToken(p, t, active, archived); e != "" {
			errs = append(errs, e)
		}
	}

	rdirs := resultNames(a.results)
	for _, r := range a.Results {
		if !slices.Contains(rdirs, r) {
			errs = append(errs, "no result directory results/"+r)
		}
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "\n") + nothingChanged)
	}

	if err := a.refuseBusy(ctx, slices.Sorted(maps.Keys(p.targets))); err != nil {
		return nil, err
	}

	p.auto = map[string][]string{}
	for n, s := range p.targets {
		p.auto[n] = namedResults(rdirs, s.cmd)
	}

	return p, nil
}

// planToken adds the sessions a token names; it returns an error message.
func (a *archiveCtx) planToken(p *archivePlan, t string, active, archived map[string]sessionFiles) string {
	var byGroup []string

	for n, s := range active {
		if s.group == t {
			byGroup = append(byGroup, n)
		}
	}

	if len(byGroup) > 0 {
		p.groups[t] = true
	}

	if _, ok := active[t]; ok {
		p.names[t] = true
		byGroup = append(byGroup, t)
	}

	for _, n := range byGroup {
		p.targets[n] = active[n]
	}

	if len(byGroup) > 0 {
		return ""
	}

	if a.db.has(secGroups, t) || anyGroup(archived, t) {
		p.notes = append(p.notes, "group "+t+" is already archived")
		p.oldRefs = append(p.oldRefs, "group:"+t)

		return ""
	}

	var old []string

	for k := range archived {
		if k == t || a.db.sessionName(k) == t {
			old = append(old, k)
		}
	}

	if len(old) == 0 {
		return "no session or group named " + t
	}

	sort.Strings(old)
	p.notes = append(p.notes, "session "+t+" is already archived")

	for _, k := range old {
		p.oldRefs = append(p.oldRefs, "session:"+k)
	}

	return ""
}

func anyGroup(sessions map[string]sessionFiles, g string) bool {
	for _, s := range sessions {
		if s.group == g {
			return true
		}
	}

	return false
}

// namedResults are the result directories a command names.
func namedResults(rdirs []string, cmd string) []string {
	out := []string{}

	for _, r := range rdirs {
		if strings.Contains(cmd, r) {
			out = append(out, r)
		}
	}

	return out
}

// refuseBusy fails if any of the sessions is alive (see Liveness).
func (a *archiveCtx) refuseBusy(ctx context.Context, names []string) error {
	panes, err := hostsys.TmuxPanes(ctx)
	if err != nil {
		return fmt.Errorf("cannot list tmux sessions (%w)%s", err, nothingChanged)
	}

	var busy []string

	for _, n := range names {
		finished := false
		if l, err := ReadLog(SessionFile(a.logs, n, LogExt), LogHeadBytes, markerTailBytes); err == nil {
			finished = l.Markers().Finished()
		}

		_, inTmux := TmuxPane(panes, a.Project, n)

		live := Liveness(ctx, HostSystem{}, LiveInput{
			Finished: finished, Tmux: inTmux, Job: ReadJob(a.logs, n),
		})
		if live.State != "" {
			busy = append(busy, n)
		}
	}

	if len(busy) > 0 {
		return fmt.Errorf("these sessions are still running and cannot be archived: %s%s",
			strings.Join(busy, ", "), nothingChanged)
	}

	return nil
}

// renameArchived keeps a session's name in logs/archived/ unless an older
// archived copy has it; then it becomes name~2, name~3, ...
func (a *archiveCtx) renameArchived(p *archivePlan) {
	p.rename = map[string]string{}
	taken := map[string]bool{}

	for k := range readSessions(a.arch) {
		taken[k] = true
	}

	for _, n := range slices.Sorted(maps.Keys(p.targets)) {
		k := n
		for i := 2; taken[k] || hasFiles(a.arch, k); i++ {
			k = fmt.Sprintf("%s~%d", n, i)
		}

		taken[k] = true
		p.rename[n] = k
	}
}

var resultsMention = regexp.MustCompile("results/([^/\\s'\"`;,)]+)")

// hints are the result directories that the logs of the archived sessions
// mention but that are not archived with them (campaigns launched by
// scripts).
func (a *archiveCtx) hints(p *archivePlan) []string {
	covered := map[string]bool{}
	for _, r := range a.Results {
		covered[r] = true
	}

	for r := range a.db.section(secResults) {
		covered[r] = true
	}

	for _, rs := range p.auto {
		for _, r := range rs {
			covered[r] = true
		}
	}

	rset := map[string]bool{}
	for _, r := range resultNames(a.results) {
		rset[r] = true
	}

	hints := map[string]bool{}

	for n := range p.targets {
		for _, m := range resultsMention.FindAllStringSubmatch(logEnds(SessionFile(a.logs, n, LogExt)), -1) {
			if rset[m[1]] && !covered[m[1]] {
				hints[m[1]] = true
			}
		}
	}

	return slices.Sorted(maps.Keys(hints))
}

// logEnds is the first and the last hintBytes of a log.
func logEnds(path string) string {
	l, err := ReadLog(path, hintBytes, hintBytes)
	switch {
	case err != nil:
		return ""
	case l.Size <= hintBytes:
		return l.Head
	default:
		return l.Head + l.Tail
	}
}

func (a *archiveCtx) applyArchive(p *archivePlan) error {
	if !a.DryRun {
		if err := os.MkdirAll(a.arch, 0o755); err != nil {
			return err
		}
	}

	for _, n := range slices.Sorted(maps.Keys(p.targets)) {
		if err := a.archiveSession(p, n); err != nil {
			return err
		}
	}

	var refs []string
	for _, g := range slices.Sorted(maps.Keys(p.groups)) {
		refs = append(refs, "group:"+g)
	}

	for _, n := range slices.Sorted(maps.Keys(p.names)) {
		refs = append(refs, "session:"+p.rename[n])
	}

	refs = append(refs, p.oldRefs...)
	if len(refs) == 0 {
		refs = []string{"manual"}
	}

	for _, r := range a.Results {
		for _, ref := range refs {
			a.db.addRef(r, ref, a.now)
		}
	}

	return nil
}

func (a *archiveCtx) archiveSession(p *archivePlan, n string) error {
	k, g := p.rename[n], p.targets[n].group
	if !a.DryRun {
		if err := moveSession(a.logs, n, a.arch, k); err != nil {
			return err
		}
	}

	var group any
	if g != "" {
		group = g
	}

	a.db.section(secSessions)[k] = map[string]any{
		"name": n, "group": group, "archived_at": a.now, "results": toAny(p.auto[n]),
	}

	for _, r := range p.auto[n] {
		a.db.addRef(r, "session:"+k, a.now)
	}

	if p.groups[g] {
		ge := a.db.entry(secGroups, g, map[string]any{"archived_at": a.now, "sessions": []any{}})
		if !slices.Contains(strList(ge["sessions"]), k) {
			ge["sessions"] = append(toAny(strList(ge["sessions"])), k)
		}
	}

	return nil
}

// moveSession renames the files of session from in dir src to to in dst.
func moveSession(src, from, dst, to string) error {
	for _, e := range SessionExts {
		s := SessionFile(src, from, e)
		if _, err := os.Lstat(s); err != nil {
			continue
		}

		if err := os.Rename(s, SessionFile(dst, to, e)); err != nil {
			return err
		}
	}

	return nil
}
