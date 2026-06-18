package simulation

import (
	"encoding/base64"
	"fmt"
	"io/fs"
	"sort"

	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/sourcefs"
)

// sourceTableName is the trace table holding recorded source archives.
const sourceTableName = "source"

// sourceArchiveFormat documents how the Content column is encoded: base64 text
// wrapping a gzip-compressed tar. The data recorder stores scalar struct fields
// only (no BLOB), so the binary archive is base64-encoded into a TEXT column.
const sourceArchiveFormat = "tar.gz;base64"

// sourceArchiveEntry is one recorded source root. Content is the base64 of a
// gzip-compressed tar of slash-separated path -> file bytes, decodable with
// sourcefs.ReadArchive.
type sourceArchiveEntry struct {
	Root    string
	Format  string
	Content string
}

// recordSourceArchives writes the simulator source into the trace's source
// table, making the trace self-describing for DaisenBot. Akita's own source is
// read from disk (so it always matches the build); any author-provided roots
// (WithSourceFS) are recorded alongside it. Source is static, so this records
// once and flushes. The table is always created so a reader can distinguish
// "recorded, empty" from "no table".
func recordSourceArchives(
	recorder datarecording.DataRecorder,
	explicitFSes map[string]fs.FS,
) error {
	recorder.CreateTable(sourceTableName, sourceArchiveEntry{})

	insert := func(root string, gztar []byte) {
		recorder.InsertData(sourceTableName, sourceArchiveEntry{
			Root:    root,
			Format:  sourceArchiveFormat,
			Content: base64.StdEncoding.EncodeToString(gztar),
		})
	}

	// Akita's own source, read from disk so it matches what ran. When the
	// source is not on disk (e.g. a -trimpath build), record nothing for Akita
	// rather than guessing — DaisenBot then reports it has no source.
	if dir, ok := sourcefs.AkitaSourceDir(); ok {
		gztar, err := sourcefs.ArchiveDir(dir)
		if err != nil {
			return fmt.Errorf("archiving akita source at %q: %w", dir, err)
		}
		insert(sourcefs.ModulePath, gztar)
	}

	// Author-provided source roots (opt-in via WithSourceFS), in a stable order.
	roots := make([]string, 0, len(explicitFSes))
	for root := range explicitFSes {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	for _, root := range roots {
		gztar, err := sourcefs.ArchiveFS(explicitFSes[root])
		if err != nil {
			return fmt.Errorf("archiving source root %q: %w", root, err)
		}
		insert(root, gztar)
	}

	recorder.Flush()
	return nil
}
