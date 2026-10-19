package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testServer(t *testing.T) (*httptest.Server, *fakeBackend) {
	t.Helper()

	b := &fakeBackend{host: experiment.HostConfig{Name: "a", Type: "ssh"}, akita: func(args []string) ([]byte, error) {
		switch {
		case len(args) == 5:
			return probeJSON(t, runningProbe(1000, 10, nil)), nil
		case args[2] == "run" && args[6] == "broken":
			return nil, errors.New("exit status 1: no such run")
		case args[2] == "run":
			return json.Marshal(&experiment.RunDetail{Project: args[5], Name: args[6]})
		default:
			return []byte(`{"cells":[],"total":0}`), nil
		}
	}}

	m := newWithBackends(testConfig(), []backend.Backend{b})
	m.pollOnce(context.Background())

	assets := http.FS(fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	})

	srv := httptest.NewServer(m.handler(assets))
	t.Cleanup(srv.Close)

	return srv, b
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()

	resp, err := http.Get(url)
	require.NoError(t, err)

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(body)
}

func TestServerRoutes(t *testing.T) {
	srv, _ := testServer(t)

	code, body := get(t, srv.URL+"/api/v1/status")
	require.Equal(t, http.StatusOK, code)

	st := &experiment.Status{}
	require.NoError(t, json.Unmarshal([]byte(body), st))
	assert.Equal(t, "a/proj/s", st.Sessions[0].ID)

	code, body = get(t, srv.URL+"/api/v1/run?host=a&project=proj&name=r1")
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, `"host":"a"`)

	code, body = get(t, srv.URL+"/api/v1/campaign?host=a&project=proj&name=c&refresh=1")
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, `"fetched_at"`)
	assert.Contains(t, body, `"schema":"exp-cells/1"`)

	code, _ = get(t, srv.URL+"/api/v1/run?host=a&project=proj&name=..")
	assert.Equal(t, http.StatusBadRequest, code)

	code, _ = get(t, srv.URL+"/api/v1/run?host=a&project=proj")
	assert.Equal(t, http.StatusBadRequest, code)

	code, body = get(t, srv.URL+"/api/v1/run?host=zz&project=proj&name=r")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Contains(t, body, "unknown host")

	code, body = get(t, srv.URL+"/api/v1/run?host=a&project=proj&name=broken")
	assert.Equal(t, http.StatusBadGateway, code)
	assert.Contains(t, body, "no such run")

	code, _ = get(t, srv.URL+"/api/v2/nothing")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestServerStaticAndSPAFallback(t *testing.T) {
	srv, _ := testServer(t)

	code, body := get(t, srv.URL+"/")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "<html>app</html>", body)

	code, body = get(t, srv.URL+"/assets/app.js")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "console.log(1)", body)

	code, body = get(t, srv.URL+"/results/some/client/route")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "<html>app</html>", body)
}

func TestServeListenerStopsWithContext(t *testing.T) {
	b := &fakeBackend{host: experiment.HostConfig{Name: "a", Type: "ssh"}, akita: func([]string) ([]byte, error) {
		return probeJSON(t, runningProbe(1, 1, nil)), nil
	}}
	m := newWithBackends(testConfig(), []backend.Backend{b})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- m.ServeListener(ctx, ln) }()

	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + ln.Addr().String() + "/api/v1/status")
		if err != nil {
			return false
		}

		resp.Body.Close()

		return resp.StatusCode == http.StatusOK && b.callCount("probe") > 0
	}, 5*time.Second, 20*time.Millisecond)

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not stop")
	}
}
