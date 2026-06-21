package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v5/sourcefs"
)

// Code tools (DaisenBot Phase 3, Workstreams C/D): code_search and code_read let
// the agent read the simulator source recorded in the trace (Workstream B's
// codeSource) so it can interpret Kinds, milestones, types, and components
// instead of guessing. Both are guarded with caps so a search or read can never
// flood the model context, mirroring data_query.

const (
	codeSearchMaxMatches   = 60
	codeSearchMaxBytes     = 16 << 10
	codeSearchMaxLineBytes = 300

	codeReadDefaultLines = 200
	codeReadMaxLines     = 400
	codeReadMaxBytes     = 24 << 10

	codeLsMaxEntries = 300
	codeLsMaxBytes   = 16 << 10
)

const noSourceRecorded = "No simulator source is recorded in this trace, so code " +
	"cannot be searched or read. Answer from general knowledge and say you could " +
	"not consult the source for this trace."

func codeSearchTool(src *sourcefs.Source) agentTool {
	return agentTool{
		name: "code_search",
		description: "Search the simulator source recorded in this trace (Go RE2 regular " +
			"expression, matched per line) to find where a Kind, milestone, type (e.g. " +
			"*mem.ReadReq), or component is defined or used. Returns matching file:line " +
			"locations; read around a match with code_read. Use this to ground what a trace " +
			"label means before explaining it.",
		parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "One sentence: what you are looking for and why. Shown to the user.",
				},
				"query": map[string]interface{}{
					"type":        "string",
					"description": "A Go (RE2) regular expression to match against source lines.",
				},
				"path_contains": map[string]interface{}{
					"type": "string",
					"description": "Optional: only search files whose path contains this substring " +
						"(e.g. \"mem/cache\" or \"dram\").",
				},
			},
			"required": []string{"reason", "query"},
		},
		run: func(_ context.Context, args map[string]interface{}) (toolResult, error) {
			query, _ := args["query"].(string)
			filter, _ := args["path_contains"].(string)
			out, err := runCodeSearch(src, query, filter)
			return toolResult{text: out}, err
		},
	}
}

func runCodeSearch(src *sourcefs.Source, query, filter string) (string, error) {
	if src.IsEmpty() {
		return noSourceRecorded, nil
	}
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("a query is required")
	}
	re, err := regexp.Compile(query)
	if err != nil {
		return "", fmt.Errorf("invalid regular expression: %w", err)
	}

	fsys := src.FS()
	var body strings.Builder
	matches := 0
	truncated := false
	for _, p := range sortedFilePaths(fsys) {
		if filter != "" && !strings.Contains(p, filter) {
			continue
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			continue
		}
		if appendFileMatches(re, p, data, &body, &matches) {
			truncated = true
			break
		}
	}
	return formatSearchResult(query, filter, body.String(), matches, truncated), nil
}

// appendFileMatches appends "path:line: snippet" for each regex match in data, up
// to the match and byte caps. It returns true if a cap was hit (so the caller
// stops searching further files).
func appendFileMatches(
	re *regexp.Regexp, p string, data []byte, body *strings.Builder, matches *int,
) bool {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if !re.MatchString(line) {
			continue
		}
		if *matches >= codeSearchMaxMatches {
			return true
		}
		entry := fmt.Sprintf("%s:%d: %s\n", p, lineNo, clipLine(strings.TrimSpace(line)))
		if body.Len()+len(entry) > codeSearchMaxBytes {
			return true
		}
		body.WriteString(entry)
		*matches++
	}
	return false
}

func formatSearchResult(query, filter, body string, matches int, truncated bool) string {
	if matches == 0 {
		scope := ""
		if filter != "" {
			scope = fmt.Sprintf(" in paths containing %q", filter)
		}
		return fmt.Sprintf("No matches for %q%s.", query, scope)
	}
	out := fmt.Sprintf("%d match(es):\n%s", matches, body)
	if truncated {
		out += "[truncated — refine the query or pass path_contains]\n"
	}
	return out
}

func codeReadTool(src *sourcefs.Source) agentTool {
	return agentTool{
		name: "code_read",
		description: "Read a file (or a line range) from the simulator source recorded in this " +
			"trace. Paths look like \"github.com/sarchlab/akita/v5/mem/mshr/mshr.go\" (as shown " +
			"by code_search). Use after code_search to study the logic around a match.",
		parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "One sentence: what you want to read and why. Shown to the user.",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "FS-relative path as shown by code_search.",
				},
				"start_line": map[string]interface{}{
					"type":        "integer",
					"description": "Optional 1-based first line to read.",
				},
				"end_line": map[string]interface{}{
					"type":        "integer",
					"description": "Optional 1-based last line to read (inclusive).",
				},
			},
			"required": []string{"reason", "path"},
		},
		run: func(_ context.Context, args map[string]interface{}) (toolResult, error) {
			p, _ := args["path"].(string)
			out, err := runCodeRead(src, p, intArg(args["start_line"]), intArg(args["end_line"]))
			return toolResult{text: out}, err
		},
	}
}

func runCodeRead(src *sourcefs.Source, p string, start, end int) (string, error) {
	if src.IsEmpty() {
		return noSourceRecorded, nil
	}
	clean, err := normalizeReadPath(p)
	if err != nil {
		return "", err
	}

	data, err := fs.ReadFile(src.FS(), clean)
	if err != nil {
		return fmt.Sprintf("File not found: %q. Use code_search to find the right path "+
			"(recorded roots: %v).", p, src.Roots), nil
	}

	lines := splitLines(data)
	if len(lines) == 0 {
		return fmt.Sprintf("%s is empty (0 lines).", clean), nil
	}
	from, to, msg := resolveReadWindow(clean, start, end, len(lines))
	if msg != "" {
		return msg, nil
	}
	return renderReadWindow(clean, lines, from, to), nil
}

// resolveReadWindow computes the 1-based [from, to] line window to show, honoring
// the requested range, the default window, and the max-lines cap. A non-empty msg
// means return it instead (e.g. start_line past the end of the file).
func resolveReadWindow(clean string, start, end, total int) (from, to int, msg string) {
	from, to = 1, total
	ranged := start > 0 || end > 0
	if start > 0 {
		from = start
	}
	if end > 0 {
		to = end
	} else if !ranged && total > codeReadDefaultLines {
		to = codeReadDefaultLines
	}

	from = max(from, 1)
	to = min(to, total)
	if from > total {
		return 0, 0, fmt.Sprintf("%s has %d lines; start_line %d is past the end.", clean, total, start)
	}
	if to < from {
		to = from
	}
	if to-from+1 > codeReadMaxLines {
		to = min(from+codeReadMaxLines-1, total)
	}
	return from, to, ""
}

// renderReadWindow formats lines[from-1:to] with line numbers, bounded by the
// per-read byte cap, with a footer noting truncation or remaining lines.
func renderReadWindow(clean string, lines []string, from, to int) string {
	total := len(lines)
	var b strings.Builder
	fmt.Fprintf(&b, "%s (lines %d-%d of %d):\n", clean, from, to, total)
	width := len(strconv.Itoa(to))
	truncated := false
	for i := from; i <= to; i++ {
		row := fmt.Sprintf("%*d\t%s\n", width, i, lines[i-1])
		if b.Len()+len(row) > codeReadMaxBytes {
			truncated = true
			break
		}
		b.WriteString(row)
	}

	switch {
	case truncated:
		b.WriteString("[output truncated — narrow the line range]\n")
	case to < total:
		fmt.Fprintf(&b, "[%d more lines; pass start_line/end_line to read further]\n", total-to)
	}
	return b.String()
}

func codeLsTool(src *sourcefs.Source) agentTool {
	return agentTool{
		name: "code_ls",
		description: "Browse the directory tree of the simulator source recorded in this trace: " +
			"list the immediate sub-directories and files under a directory. Call with an empty " +
			"path to see the recorded module roots, then pass a path (as shown by code_search or " +
			"a previous code_ls) to list one directory. Directories end with \"/\"; files show " +
			"their line and byte counts. Use this to discover what exists and learn the layout " +
			"before reading, instead of guessing paths.",
		parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "One sentence: what you are looking for and why. Shown to the user.",
				},
				"path": map[string]interface{}{
					"type": "string",
					"description": "Optional directory to list (FS-relative, as shown by code_search " +
						"or code_ls). Empty or omitted lists the recorded module roots.",
				},
			},
			"required": []string{"reason"},
		},
		run: func(_ context.Context, args map[string]interface{}) (toolResult, error) {
			p, _ := args["path"].(string)
			out, err := runCodeLs(src, p)
			return toolResult{text: out}, err
		},
	}
}

func runCodeLs(src *sourcefs.Source, p string) (string, error) {
	if src.IsEmpty() {
		return noSourceRecorded, nil
	}

	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimSuffix(p, "/")

	fsys := src.FS()
	if p == "" || p == "." {
		// The recorded FS is a deep single-child chain (github.com/sarchlab/…), so
		// listing "." literally would just show "github.com/". Surface the recorded
		// module roots directly — that is what "the top-level directories" means here.
		if roots := nonEmptyRoots(src.Roots); len(roots) > 0 {
			return formatRootListing(roots), nil
		}
		return listDir(fsys, "."), nil // roots unknown/blank: fall back to the real root
	}

	clean := path.Clean(p)
	if !fs.ValidPath(clean) {
		return "", fmt.Errorf("invalid path %q", p)
	}

	info, err := fs.Stat(fsys, clean)
	if err != nil {
		return fmt.Sprintf("Directory not found: %q. Use code_search to find a path, or call "+
			"code_ls with no path to see the recorded roots (%v).", p, src.Roots), nil
	}
	if !info.IsDir() {
		return fmt.Sprintf("%s is a file, not a directory. Use code_read to read it.", clean), nil
	}
	return listDir(fsys, clean), nil
}

// listDir renders the immediate entries under dir: directories first (trailing
// "/"), then files annotated with line and byte counts, bounded by the entry and
// byte caps so a large directory can never flood the model context.
func listDir(fsys fs.FS, dir string) string {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Sprintf("Could not list %s: %v.", dir, err)
	}
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di // directories before files
		}
		return entries[i].Name() < entries[j].Name()
	})

	totalDirs, totalFiles := 0, 0
	for _, e := range entries {
		if e.IsDir() {
			totalDirs++
		} else {
			totalFiles++
		}
	}

	var body strings.Builder
	truncated := false
	shown := 0
	for _, e := range entries {
		if shown >= codeLsMaxEntries {
			truncated = true
			break
		}
		var line string
		if e.IsDir() {
			line = e.Name() + "/\n"
		} else {
			line = fmt.Sprintf("%s\t%s\n", e.Name(), fileAnnotation(fsys, path.Join(dir, e.Name())))
		}
		if body.Len()+len(line) > codeLsMaxBytes {
			truncated = true
			break
		}
		body.WriteString(line)
		shown++
	}

	label := dir
	if label == "." {
		label = "(root)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %d dir(s), %d file(s):\n", label, totalDirs, totalFiles)
	b.WriteString(body.String())
	if truncated {
		b.WriteString("[truncated — list a sub-directory to narrow]\n")
	}
	return b.String()
}

// fileAnnotation returns "<n> lines, <size>" for a file, reading it once. A read
// error degrades to "?" rather than failing the whole listing.
func fileAnnotation(fsys fs.FS, p string) string {
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return "?"
	}
	return fmt.Sprintf("%d lines, %s", countLines(data), humanBytes(len(data)))
}

func formatRootListing(roots []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Recorded module root(s) — %d; pass one as the path to browse it:\n", len(roots))
	for _, r := range roots {
		fmt.Fprintf(&b, "%s/\n", r)
	}
	return b.String()
}

func nonEmptyRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		if strings.TrimSpace(r) != "" {
			out = append(out, r)
		}
	}
	return out
}

// countLines counts lines the same way splitLines does: a trailing newline does
// not add a phantom final line.
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		n++ // last line has no trailing newline
	}
	return n
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func sortedFilePaths(fsys fs.FS) []string {
	var paths []string
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		paths = append(paths, p)
		return nil
	})
	sort.Strings(paths)
	return paths
}

func normalizeReadPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "./")
	if p == "" {
		return "", fmt.Errorf("a path is required")
	}
	clean := path.Clean(p)
	if !fs.ValidPath(clean) {
		return "", fmt.Errorf("invalid path %q", p)
	}
	return clean, nil
}

func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1] // drop the empty element after a trailing newline
	}
	return lines
}

func clipLine(s string) string {
	if len(s) > codeSearchMaxLineBytes {
		return s[:codeSearchMaxLineBytes] + "…"
	}
	return s
}

func intArg(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
