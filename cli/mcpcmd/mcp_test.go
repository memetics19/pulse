package mcpcmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apiCall is one request the fake Pulse API received.
type apiCall struct {
	method, path, query, auth string
	body                      map[string]any
}

// fakePulse records requests and answers each with reply.
type fakePulse struct {
	mu    sync.Mutex
	calls []apiCall
	reply func(w http.ResponseWriter, r *http.Request)
}

func (f *fakePulse) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := apiCall{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, auth: r.Header.Get("Authorization")}
	if b, _ := io.ReadAll(r.Body); len(b) > 0 {
		_ = json.Unmarshal(b, &c.body)
	}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	if f.reply != nil {
		f.reply(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (f *fakePulse) all() []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// connect starts the MCP server against api and returns a client session.
func connect(t *testing.T, api http.Handler) *mcp.ClientSession {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	return connectTo(t, srv.URL)
}

func connectTo(t *testing.T, baseURL string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := NewServer(baseURL, "pulse_live_test", nil).Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.Len(t, res.Content, 1)
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

func TestListToolsCoversTheTable(t *testing.T) {
	cs := connect(t, &fakePulse{})
	res, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, res.Tools, len(tools))

	byName := map[string]*mcp.Tool{}
	for _, tl := range res.Tools {
		byName[tl.Name] = tl
	}
	del := byName["delete_monitor"]
	require.NotNil(t, del)
	require.NotNil(t, del.Annotations.DestructiveHint)
	assert.True(t, *del.Annotations.DestructiveHint)
	assert.True(t, byName["list_monitors"].Annotations.ReadOnlyHint)

	schema := byName["update_incident_status"].InputSchema.(map[string]any)
	assert.Equal(t, "object", schema["type"])
	assert.ElementsMatch(t, []any{"id", "status"}, schema["required"])
}

// Every {arg} in a path is a declared path argument and the other way round,
// required arguments exist, and names are unique.
func TestToolTableIsConsistent(t *testing.T) {
	placeholder := regexp.MustCompile(`\{(\w+)\}`)
	seen := map[string]bool{}
	for _, tl := range tools {
		assert.False(t, seen[tl.name], "duplicate tool %s", tl.name)
		seen[tl.name] = true
		var inPath []string
		for _, m := range placeholder.FindAllStringSubmatch(tl.path, -1) {
			inPath = append(inPath, m[1])
		}
		assert.ElementsMatch(t, inPath, tl.pathArgs, tl.name)
		for _, r := range tl.required {
			assert.NotNil(t, tl.body[r], "%s requires %s, which is not a body field", tl.name, r)
		}
		assert.NotEmpty(t, tl.description, tl.name)
		if tl.readOnly {
			assert.Equal(t, http.MethodGet, tl.method, tl.name)
		}
	}
}

func TestReadToolSendsKeyAndReturnsBody(t *testing.T) {
	api := &fakePulse{reply: func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"name":"API"}]`))
	}}
	cs := connect(t, api)

	text, isErr := call(t, cs, "list_monitors", nil)
	assert.False(t, isErr)
	assert.Equal(t, `[{"id":1,"name":"API"}]`, text)
	calls := api.all()
	require.Len(t, calls, 1)
	assert.Equal(t, apiCall{method: "GET", path: "/api/monitors", auth: "Bearer pulse_live_test"}, calls[0])
}

func TestPathAndQueryArguments(t *testing.T) {
	api := &fakePulse{}
	cs := connect(t, api)

	_, isErr := call(t, cs, "list_check_results", map[string]any{"monitor_id": 12, "days": 7})
	assert.False(t, isErr)
	calls := api.all()
	require.Len(t, calls, 1)
	assert.Equal(t, "/api/monitors/12/checks", calls[0].path)
	assert.Equal(t, "days=7", calls[0].query)
}

func TestWriteToolSendsOnlyGivenFields(t *testing.T) {
	api := &fakePulse{}
	cs := connect(t, api)

	_, isErr := call(t, cs, "create_incident", map[string]any{
		"title": "API down", "severity": "major", "affected_monitor_ids": []int{3, 4},
	})
	assert.False(t, isErr)
	_, isErr = call(t, cs, "update_incident_status", map[string]any{"id": 7, "status": "resolved", "rca": "bad deploy"})
	assert.False(t, isErr)

	calls := api.all()
	require.Len(t, calls, 2)
	assert.Equal(t, "POST", calls[0].method)
	assert.Equal(t, map[string]any{"title": "API down", "severity": "major", "affected_monitor_ids": []any{3.0, 4.0}}, calls[0].body)
	assert.Equal(t, "PUT", calls[1].method)
	assert.Equal(t, "/api/incidents/7/status", calls[1].path)
	assert.Equal(t, map[string]any{"status": "resolved", "rca": "bad deploy"}, calls[1].body)
}

func TestBadArgumentsNeverReachTheAPI(t *testing.T) {
	api := &fakePulse{}
	cs := connect(t, api)

	cases := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"create_incident", map[string]any{"title": "x"}, "severity is required"},
		{"get_monitor", map[string]any{}, "id must be a positive integer"},
		{"get_monitor", map[string]any{"id": 0}, "id must be a positive integer"},
		{"get_monitor", map[string]any{"id": "abc"}, "id must be a positive integer"},
		{"list_check_results", map[string]any{"monitor_id": 1, "days": -3}, "days must be a positive integer"},
		{"list_monitors", map[string]any{"verbose": true}, `unknown argument "verbose"`},
	}
	for _, c := range cases {
		text, isErr := call(t, cs, c.tool, c.args)
		assert.True(t, isErr, c.tool)
		assert.Equal(t, c.want, text, c.tool)
	}
	assert.Empty(t, api.all())
}

func TestAPIErrorsAreToolErrors(t *testing.T) {
	api := &fakePulse{reply: func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `an agent named "web" already exists`, http.StatusConflict)
	}}
	cs := connect(t, api)

	text, isErr := call(t, cs, "create_agent", map[string]any{"name": "web"})
	assert.True(t, isErr)
	assert.Equal(t, `HTTP 409 from POST /api/agents: an agent named "web" already exists`, text)
}

func TestEmptySuccessSaysDone(t *testing.T) {
	api := &fakePulse{reply: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }}
	cs := connect(t, api)

	text, isErr := call(t, cs, "delete_monitor", map[string]any{"id": 5})
	assert.False(t, isErr)
	assert.Equal(t, "HTTP 204, done.", text)
	assert.Equal(t, "DELETE", api.all()[0].method)
	assert.Equal(t, "/api/monitors/5", api.all()[0].path)
}

func TestUnreachableServerIsToolError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()
	cs := connectTo(t, base)

	text, isErr := call(t, cs, "get_overview", nil)
	assert.True(t, isErr)
	assert.True(t, strings.HasPrefix(text, "GET /api/overview: "), text)
	assert.NotContains(t, text, "pulse_live_test")
}

func TestLargeResponseIsTruncated(t *testing.T) {
	big := strings.Repeat("a", maxResponse+10)
	api := &fakePulse{reply: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(big)) }}
	cs := connect(t, api)

	text, isErr := call(t, cs, "list_incidents", nil)
	assert.False(t, isErr)
	assert.True(t, strings.HasSuffix(text, "[truncated: the response is larger than 4 MiB]"))
	assert.Len(t, text, maxResponse+len("\n[truncated: the response is larger than 4 MiB]"))
}

func TestSettings(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	keyFile := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(keyFile, []byte("pulse_live_file\n"), 0o600))

	base, token, err := settings("", "", env(map[string]string{"PULSE_URL": "https://status.example.com/", "PULSE_API_KEY": "k"}))
	require.NoError(t, err)
	assert.Equal(t, "https://status.example.com", base)
	assert.Equal(t, "k", token)

	base, token, err = settings("http://localhost:8080", keyFile, env(nil))
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080", base)
	assert.Equal(t, "pulse_live_file", token, "the file is read when PULSE_API_KEY is unset")

	_, token, err = settings("http://localhost:8080", keyFile, env(map[string]string{"PULSE_API_KEY": "env"}))
	require.NoError(t, err)
	assert.Equal(t, "env", token, "PULSE_API_KEY wins over the file")

	for _, c := range []struct {
		server, file string
		env          map[string]string
		want         string
	}{
		{"", "", nil, "set the Pulse URL"},
		{"ftp://x", "", map[string]string{"PULSE_API_KEY": "k"}, "must be an http or https URL"},
		{"https://", "", map[string]string{"PULSE_API_KEY": "k"}, "must be an http or https URL"},
		{"https://x", "", nil, "set the API key"},
		{"https://x", filepath.Join(t.TempDir(), "missing"), nil, "read token file"},
	} {
		_, _, err := settings(c.server, c.file, env(c.env))
		require.Error(t, err)
		assert.Contains(t, err.Error(), c.want)
	}
}

func TestCommandNeedsSettings(t *testing.T) {
	t.Setenv("PULSE_URL", "")
	t.Setenv("PULSE_API_KEY", "")
	cmd := NewMCPCmd()
	cmd.SetArgs(nil)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "set the Pulse URL")
}
