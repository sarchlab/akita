package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/experiment/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLogMarkers(t *testing.T) {
	head := "[start] Fri Oct 16 01:56:33 AM UTC 2026  server2  abc1234\nhello\n"

	st := parseLogMarkers(head, head+"[exit 3] Fri Oct 16 01:57:33 AM UTC 2026\n")
	if st.State != "failed" || st.ExitCode == nil || *st.ExitCode != 3 || st.Start == nil || st.End == nil {
		t.Fatalf("got %+v", st)
	}

	if *st.End-*st.Start != 60 {
		t.Errorf("duration %v", *st.End-*st.Start)
	}

	if st := parseLogMarkers(head, "x\n[exit 0] Thu Oct 15 21:56:13 EDT 2026\n"); st.State != "ok" {
		t.Errorf("got %+v", st)
	}

	if st := parseLogMarkers(head, "[stopped] Fri Oct 16 01:56:36 AM UTC 2026  akita exp stop\n"); st.State != "stopped" ||
		st.End == nil {
		t.Errorf("got %+v", st)
	}

	if st := parseLogMarkers(head, "hello\n"); st.State != "died" {
		t.Errorf("got %+v", st)
	}
}

func TestParseLogTime(t *testing.T) {
	v := parseLogTime("Mon Oct  5 01:02:03 AM UTC 2026")
	require.NotNil(t, v)
	assert.Equal(t, float64(time.Date(2026, 10, 5, 1, 2, 3, 0, time.UTC).Unix()), *v)

	v = parseLogTime("Thu Oct 15 14:59:54 UTC 2026")
	require.NotNil(t, v)
	assert.Nil(t, parseLogTime("not a date"))
}

// englishDate is the date of the log markers in every locale.
var englishDate = regexp.MustCompile(`^[A-Z][a-z]{2} [A-Z][a-z]{2} [ 0-9]\d \d\d:\d\d:\d\d [AP]M \S+ \d{4}$`)

func TestWrapperScriptRuns(t *testing.T) {
	ws := t.TempDir()
	testutil.MustMkdir(t, filepath.Join(ws, "logs"))

	script := WrapperScript("h1", ws, "w", `echo "it's $EXP_HEARTBEAT"; exit 4`)

	// The marker dates do not depend on the locale.
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(), "LC_ALL=de_DE.UTF-8", "LANG=de_DE.UTF-8", "LC_TIME=de_DE.UTF-8")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	log := readFile(t, filepath.Join(ws, "logs", "w.log"))
	lines := strings.Split(strings.TrimSpace(log), "\n")

	if len(lines) != 3 || !strings.HasPrefix(lines[0], "[start] ") || !strings.Contains(lines[0], "  h1  ") {
		t.Fatalf("log %q", log)
	}

	if lines[1] != "it's "+filepath.Join(ws, "logs", "w.heartbeat") {
		t.Errorf("command output %q", lines[1])
	}

	if !strings.HasPrefix(lines[2], "[exit 4] ") {
		t.Errorf("exit line %q", lines[2])
	}

	start, _, _ := strings.Cut(strings.TrimPrefix(lines[0], "[start] "), "  ")
	for _, d := range []string{start, strings.TrimPrefix(lines[2], "[exit 4] ")} {
		if !englishDate.MatchString(d) || parseLogTime(d) == nil {
			t.Errorf("marker date %q is not the English date format", d)
		}
	}
}
