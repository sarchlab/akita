package workspace

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
)

// Unarchive moves archived sessions back to logs/ and drops the archive
// marks they (or their groups, or --results) put on result directories.
func Unarchive(ctx context.Context, req ArchiveRequest) error {
	a, err := newArchiveCtx(req)
	if err != nil {
		return err
	}

	if len(a.Tokens) == 0 && len(a.Results) == 0 {
		return errors.New("usage: akita herd unarchive [-n] <host> <group|name>... [--results <run>...]")
	}

	return a.unarchive(ctx)
}

// restore is an archived session and the name it gets back.
type restore struct {
	from, to string
}

func (a *archiveCtx) unarchive(ctx context.Context) error {
	archived := readSessions(a.arch)

	targets, groups, err := a.planUnarchive(ctx, archived)
	if err != nil {
		return err
	}

	if len(targets) > 0 {
		a.printf("%s %d sessions (logs move back to logs/):\n",
			pick(a.DryRun, "Would unarchive", "Unarchiving"), len(targets))
	}

	for _, t := range targets {
		a.printf("  %s%s%s\n", t.from, arrow(t.to, t.from != t.to), groupNote(archived[t.from].group))
	}

	before := slices.Collect(maps.Keys(a.db.section(secResults)))
	err = a.applyUnarchive(targets, groups, archived)

	if !a.DryRun {
		if serr := a.db.save(); err == nil {
			err = serr
		}
	}

	if err != nil {
		return err
	}

	a.printUnmarked(before)

	return nil
}

// planUnarchive returns the sessions to restore and the group tokens.
func (a *archiveCtx) planUnarchive(ctx context.Context, archived map[string]sessionFiles,
) ([]restore, map[string]bool, error) {
	var (
		errs   []string
		keys   []string
		groups = map[string]bool{}
	)

	for _, t := range a.Tokens {
		hit := 0

		for _, k := range slices.Sorted(maps.Keys(archived)) {
			if k == t || archived[k].group == t || a.db.sessionName(k) == t {
				hit++

				if !slices.Contains(keys, k) {
					keys = append(keys, k)
				}
			}
		}

		if a.db.has(secGroups, t) || anyGroup(archived, t) {
			groups[t] = true
		}

		if hit == 0 && !a.db.has(secGroups, t) {
			errs = append(errs, "no archived session or group named "+t)
		}
	}

	for _, r := range a.Results {
		if !a.db.has(secResults, r) {
			errs = append(errs, "result directory "+r+" is not archived")
		}
	}

	targets, perrs := a.restoreNames(ctx, keys)
	errs = append(errs, perrs...)

	if len(errs) > 0 {
		return nil, nil, errors.New(strings.Join(errs, "\n") + nothingChanged)
	}

	return targets, groups, nil
}

// restoreNames picks the name each archived session gets back: its
// original name unless a session in logs/ or a live tmux session has it.
func (a *archiveCtx) restoreNames(ctx context.Context, keys []string) ([]restore, []string) {
	panes, err := hostsys.TmuxPanes(ctx)
	if err != nil {
		return nil, []string{fmt.Sprintf("cannot list tmux sessions (%v)", err)}
	}

	var (
		out   []restore
		errs  []string
		taken = map[string]bool{}
	)

	for _, k := range keys {
		orig := a.db.sessionName(k)
		if orig == "" {
			orig = k
		}

		cands := []string{k}
		if orig != k {
			cands = []string{orig, k}
		}

		i := slices.IndexFunc(cands, func(c string) bool {
			_, inTmux := TmuxPane(panes, a.Project, c)
			return !taken[c] && !inTmux && !hasFiles(a.logs, c)
		})
		if i < 0 {
			errs = append(errs, fmt.Sprintf("logs/%s.log already exists (a newer session has the "+
				"name); cannot move archived/%s back", orig, k))

			continue
		}

		taken[cands[i]] = true
		out = append(out, restore{from: k, to: cands[i]})
	}

	return out, errs
}

func (a *archiveCtx) applyUnarchive(targets []restore, groups map[string]bool,
	archived map[string]sessionFiles,
) error {
	restored := map[string]bool{}

	for _, t := range targets {
		k := t.from
		restored[k] = true

		if !a.DryRun {
			if err := moveSession(a.arch, k, a.logs, t.to); err != nil {
				return err
			}
		}

		delete(a.db.section(secSessions), k)
		a.db.dropRef("session:" + k)

		for _, ge := range a.db.section(secGroups) {
			if m, ok := ge.(map[string]any); ok {
				m["sessions"] = toAny(slices.DeleteFunc(strList(m["sessions"]),
					func(s string) bool { return s == k }))
			}
		}
	}

	remaining := map[string]bool{}

	for k, s := range archived {
		if !restored[k] {
			remaining[s.group] = true
		}
	}

	for _, g := range slices.Sorted(maps.Keys(a.db.section(secGroups))) {
		if groups[g] || !remaining[g] {
			delete(a.db.section(secGroups), g)
			a.db.dropRef("group:" + g)
		}
	}

	for _, r := range a.Results {
		delete(a.db.section(secResults), r)
	}

	return nil
}
