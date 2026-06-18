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
	paths := sortedFilePaths(fsys)

	var body strings.Builder
	matches := 0
	truncated := false

	for _, p := range paths {
		if filter != "" && !strings.Contains(p, filter) {
			continue
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if !re.MatchString(line) {
				continue
			}
			if matches >= codeSearchMaxMatches {
				truncated = true
				break
			}
			entry := fmt.Sprintf("%s:%d: %s\n", p, lineNo, clipLine(strings.TrimSpace(line)))
			if body.Len()+len(entry) > codeSearchMaxBytes {
				truncated = true
				break
			}
			body.WriteString(entry)
			matches++
		}
		if truncated {
			break
		}
	}

	if matches == 0 {
		scope := ""
		if filter != "" {
			scope = fmt.Sprintf(" in paths containing %q", filter)
		}
		return fmt.Sprintf("No matches for %q%s.", query, scope), nil
	}

	header := fmt.Sprintf("%d match(es):\n", matches)
	footer := ""
	if truncated {
		footer = "[truncated — refine the query or pass path_contains]\n"
	}
	return header + body.String() + footer, nil
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
	total := len(lines)
	ranged := start > 0 || end > 0

	from, to := 1, total
	if start > 0 {
		from = start
	}
	if end > 0 {
		to = end
	} else if !ranged && total > codeReadDefaultLines {
		to = codeReadDefaultLines
	}

	if total == 0 {
		return fmt.Sprintf("%s is empty (0 lines).", clean), nil
	}
	from = max(from, 1)
	to = min(to, total)
	if from > total {
		return fmt.Sprintf("%s has %d lines; start_line %d is past the end.", clean, total, start), nil
	}
	if to < from {
		to = from
	}
	if to-from+1 > codeReadMaxLines {
		to = min(from+codeReadMaxLines-1, total)
	}

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
	return b.String(), nil
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
