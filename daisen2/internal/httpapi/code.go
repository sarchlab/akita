package httpapi

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"path"
	"sort"
)

// maxCodeReadBytes caps how much of a recorded source file /api/code/read will
// load, so a trace carrying a huge or generated source blob cannot exhaust
// server memory. Real source files are far smaller.
const maxCodeReadBytes = 4 << 20 // 4 MiB

// CodeEntry is one directory entry of the recorded source tree.
type CodeEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

// CodeLsResponse lists the entries under a directory of the recorded source.
// When Path is empty the listing is the recorded module roots.
type CodeLsResponse struct {
	Path    string      `json:"path"`
	Roots   []string    `json:"roots"`
	Entries []CodeEntry `json:"entries"`
}

// CodeReadResponse holds the contents of a recorded source file.
type CodeReadResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Lines   int    `json:"lines"`
}

// httpCodeLs lists a directory of the source recorded in the trace. With no
// (or root) path it returns the recorded module roots, since the filesystem
// root is a deep single-child chain. Missing/empty source yields an empty
// listing rather than an error.
func (s *Server) httpCodeLs(w http.ResponseWriter, r *http.Request) {
	roots := s.sourceRoots()
	resp := CodeLsResponse{
		Path:    r.FormValue("path"),
		Roots:   roots,
		Entries: []CodeEntry{},
	}

	if s.codeSource == nil || s.codeSource.IsEmpty() {
		writeJSON(w, resp)
		return
	}

	if resp.Path == "" || resp.Path == "." {
		// Surface the module roots as the top-level directories.
		for _, root := range roots {
			resp.Entries = append(resp.Entries, CodeEntry{Name: root, IsDir: true})
		}
		writeJSON(w, resp)
		return
	}

	clean := path.Clean(resp.Path)
	if !fs.ValidPath(clean) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	entries, err := fs.ReadDir(s.codeSource.FS(), clean)
	if err != nil {
		http.Error(w, "not a directory", http.StatusNotFound)
		return
	}

	for _, e := range entries {
		entry := CodeEntry{Name: e.Name(), IsDir: e.IsDir()}
		if !e.IsDir() {
			if info, err := e.Info(); err == nil {
				entry.Size = info.Size()
			}
		}
		resp.Entries = append(resp.Entries, entry)
	}
	// Directories first, then files, each alphabetical.
	sort.SliceStable(resp.Entries, func(i, j int) bool {
		a, b := resp.Entries[i], resp.Entries[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return a.Name < b.Name
	})

	writeJSON(w, resp)
}

// httpCodeRead returns the contents of a recorded source file.
func (s *Server) httpCodeRead(w http.ResponseWriter, r *http.Request) {
	p := r.FormValue("path")
	clean := path.Clean(p)
	if p == "" || !fs.ValidPath(clean) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	if s.codeSource == nil || s.codeSource.IsEmpty() {
		http.Error(w, "no source recorded", http.StatusNotFound)
		return
	}

	f, err := s.codeSource.FS().Open(clean)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	// Read at most maxCodeReadBytes+1 so an oversized file is detected without
	// pulling the whole thing into memory.
	data, err := io.ReadAll(io.LimitReader(f, maxCodeReadBytes+1))
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	if len(data) > maxCodeReadBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}

	writeJSON(w, CodeReadResponse{
		Path:    clean,
		Content: string(data),
		Lines:   countLines(data),
	})
}

// sourceRoots returns the recorded module roots, or an empty slice.
func (s *Server) sourceRoots() []string {
	if s.codeSource == nil {
		return []string{}
	}
	roots := s.codeSource.Roots
	if roots == nil {
		return []string{}
	}
	return roots
}

func writeJSON(w http.ResponseWriter, v any) {
	rsp, err := json.Marshal(v)
	dieOnErr(err)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(rsp)
	dieOnErr(err)
}
