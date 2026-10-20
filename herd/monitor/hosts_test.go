package monitor

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// post sends a JSON body as the UI does, with extra headers.
func post(t *testing.T, url, body string, headers map[string]string) (int, string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")

	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(data)
}

func TestHostRequestsComeFromTheUI(t *testing.T) {
	srv, _ := testServer(t)
	body := `{"address": "server9"}`
	ui := map[string]string{requestHeader: "1"}

	code, _ := get(t, srv.URL+"/api/v1/hosts/scan")
	assert.Equal(t, http.StatusMethodNotAllowed, code)

	code, _ = post(t, srv.URL+"/api/v1/hosts/scan", body, nil)
	assert.Equal(t, http.StatusForbidden, code, "without the header")

	code, _ = post(t, srv.URL+"/api/v1/hosts/scan", body,
		map[string]string{requestHeader: "1", "Origin": "http://evil.example"})
	assert.Equal(t, http.StatusForbidden, code, "from another site")

	code, out := post(t, srv.URL+"/api/v1/hosts/scan", `{"address": "-oProxyCommand=x"}`, ui)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, out, "not an ssh address")
}

func TestAddHostFromTheUI(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "herd.json")
	t.Setenv("AKITA_HERD_CONFIG", cfg)

	srv, _ := testServer(t)
	ui := map[string]string{requestHeader: "1"}
	add := func(h string) (int, string) {
		return post(t, srv.URL+"/api/v1/hosts", `{"host": `+h+`}`, ui)
	}

	code, out := add(`{"name": "a", "type": "ssh", "address": "a"}`)
	assert.Equal(t, http.StatusConflict, code, out)

	code, _ = add(`{"name": "x.y", "type": "ssh", "address": "x"}`)
	assert.Equal(t, http.StatusBadRequest, code)

	code, out = add(`{"name": "s9", "type": "ssh", "address": "me@s9.example", "queue": {"slots": 4}}`)
	require.Equal(t, http.StatusOK, code, out)

	data, err := os.ReadFile(cfg)
	require.NoError(t, err)

	var written herd.UserConfig
	require.NoError(t, json.Unmarshal(data, &written))
	require.Len(t, written.Hosts, 2) // local, as a new config keeps it, and s9
	assert.Equal(t, "s9", written.Hosts[1].Name)
	assert.Equal(t, 4, written.Hosts[1].Queue.Slots)

	_, status := get(t, srv.URL+"/api/v1/status")

	var st herd.Status
	require.NoError(t, json.Unmarshal([]byte(status), &st))

	names := make([]string, 0, len(st.Hosts))
	for _, h := range st.Hosts {
		names = append(names, h.Name)
	}

	assert.Contains(t, names, "s9", "the monitor polls the new host")

	code, _ = add(`{"name": "s9", "type": "ssh", "address": "s9"}`)
	assert.Equal(t, http.StatusConflict, code, "added twice")
}
