package workspace

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

// dryRunNote ends the summary of a dry run.
const dryRunNote = " (dry run, nothing was changed)"

// Listing limits: names shown per group, characters of a date shown.
const (
	maxNamesListed = 8
	shortDateChars = 16 // "2006-01-02 15:04"
)

func (a *archiveCtx) printArchivePlan(p *archivePlan) {
	if len(p.targets) > 0 {
		a.printf("%s %d sessions (logs move to logs/archived/):\n",
			pick(a.DryRun, "Would archive", "Archiving"), len(p.targets))
	}

	for _, n := range slices.Sorted(maps.Keys(p.targets)) {
		a.printf("  %s%s%s\n", n, arrow("archived/"+p.rename[n], p.rename[n] != n),
			groupNote(p.targets[n].group))

		for _, r := range p.auto[n] {
			a.printf("      result directory %s\n", r)
		}
	}

	if len(a.Results) > 0 {
		a.printf("  Result directories given with --results:\n")

		for _, r := range a.Results {
			a.printf("      result directory %s\n", r)
		}
	}
}

// printHints lists the result directories of hints.
func (a *archiveCtx) printHints(p *archivePlan) {
	names := a.hints(p)
	if len(names) == 0 {
		return
	}

	a.printf("  Hint: the logs also mention these result directories, which are not archived " +
		"with them (common for campaigns launched by scripts):\n")

	for _, r := range names {
		a.printf("      %s\n", r)
	}

	a.printf("    To archive them too, add: --results %s\n", strings.Join(names, " "))
}

func (a *archiveCtx) printUnmarked(before []string) {
	var back []string

	for _, r := range before {
		if !a.db.has(secResults, r) {
			back = append(back, r)
		}
	}

	sort.Strings(back)

	list := "."
	if len(back) > 0 {
		list = ": " + shortList(back, maxNamesListed)
	}

	a.printf("%s %d result directories as archived%s%s\n",
		pick(a.DryRun, "Would unmark", "Unmarked"), len(back), list, pick(a.DryRun, dryRunNote, ""))
}

// list prints what is archived.
func (a *archiveCtx) list() {
	archived := readSessions(a.arch)
	results := a.db.section(secResults)

	if len(archived) == 0 && len(results) == 0 {
		a.printf("(no archived sessions or result directories)\n")
		return
	}

	byGroup := map[string][]string{}
	for k, s := range archived {
		byGroup[s.group] = append(byGroup[s.group], k)
	}

	if len(archived) > 0 {
		a.printf("Archived sessions (logs/archived/, %d in total):\n", len(archived))
	}

	for _, g := range slices.Sorted(maps.Keys(byGroup)) {
		names := byGroup[g]
		sort.Strings(names)

		label, at := "(no group)", ""
		if g != "" {
			label = "group " + g

			if ge, ok := a.db.section(secGroups)[g].(map[string]any); ok {
				if s, _ := ge["archived_at"].(string); s != "" {
					at = ", archived " + s[:min(shortDateChars, len(s))]
				}
			}
		}

		a.printf("  %s  %d%s: %s\n", label, len(names), at, shortList(names, maxNamesListed))
	}

	if len(results) > 0 {
		a.printf("Archived result directories (recorded in %s; the directories are not moved, "+
			"%d in total):\n", ArchiveFile, len(results))

		for _, r := range slices.Sorted(maps.Keys(results)) {
			a.printf("  %s\n", r)
		}
	}
}

func arrow(to string, show bool) string {
	if !show {
		return ""
	}

	return " → " + to
}

func groupNote(g string) string {
	if g == "" {
		return ""
	}

	return "  [group " + g + "]"
}

func pick(cond bool, yes, no string) string {
	if cond {
		return yes
	}

	return no
}

func shortList(names []string, n int) string {
	if len(names) <= n {
		return strings.Join(names, ", ")
	}

	return fmt.Sprintf("%s … (%d in total)", strings.Join(names[:n], ", "), len(names))
}
