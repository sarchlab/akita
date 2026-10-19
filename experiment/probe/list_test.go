package probe

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/experiment/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListSessionsDiedSlurmJob(t *testing.T) {
	fake := testutil.FakeSlurm(t)
	ws := t.TempDir()
	writeFiles(t, ws, map[string]string{
		"logs/t.log": "[start] Fri Oct 16 01:56:33 AM UTC 2026  hpc  \nworking\n",
		"logs/t.job": "77\nslurm\n",
	})
	writeFiles(t, fake, map[string]string{"final77": "TIMEOUT\n"})

	ss, err := ListSessions(context.Background(), ws, "p", false, 5)
	require.NoError(t, err)
	require.Len(t, ss, 1)
	assert.Equal(t, "died", ss[0].State)
	assert.Equal(t, "[slurm] TIMEOUT", ss[0].Tail[len(ss[0].Tail)-1])
	assert.Equal(t, "77", ss[0].Job)

	calls := readFile(t, filepath.Join(fake, "squeue-calls"))
	assert.Equal(t, 1, strings.Count(calls, "x"), "squeue asked once for one session")
}

func TestListSessionsIsLight(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "proj")
	writeFiles(t, ws, map[string]string{
		"logs/a.log":          logStart + "one\ntwo\nGOPATH set to GOROOT\nthree\n" + logExit0,
		"logs/a.cmd":          "./run\n",
		"logs/a.heartbeat":    `{"pid": 1}`,
		"logs/archived/b.log": logStart + logExit0,
	})

	ss, err := ListSessions(context.Background(), ws, "", false, 2)
	require.NoError(t, err)
	require.Len(t, ss, 1, "archived sessions only on request")
	assert.Equal(t, "proj", ss[0].Project)
	assert.Equal(t, "ok", ss[0].State)
	assert.Equal(t, "./run", ss[0].Cmd)
	assert.Equal(t, []string{"three", strings.TrimSpace(logExit0)}, ss[0].Tail)
	assert.Nil(t, ss[0].Heartbeat)

	ss, err = ListSessions(context.Background(), ws, "proj", true, 2)
	require.NoError(t, err)
	assert.Len(t, ss, 2)
}
