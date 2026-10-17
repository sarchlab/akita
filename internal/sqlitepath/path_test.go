package sqlitepath

import "testing"

func TestFileURIDriveLetterAndLiteralCharacters(t *testing.T) {
	for _, tc := range []struct{ path, mode, want string }{
		{"C:/traces/run #?%.sqlite3", "rw", "file:///C:/traces/run%20%23%3F%25.sqlite3?mode=rw"},
		{"/tmp/run #?%.sqlite3", "ro", "file:///tmp/run%20%23%3F%25.sqlite3?mode=ro"},
		{"//server/share/run.sqlite3", "rwc", "file:////server/share/run.sqlite3?mode=rwc"},
	} {
		if got := fileURI(tc.path, tc.mode); got != tc.want {
			t.Errorf("fileURI(%q, %q) = %q; want %q", tc.path, tc.mode, got, tc.want)
		}
	}
}
