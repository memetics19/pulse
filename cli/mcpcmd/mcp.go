// Package mcpcmd serves the Pulse REST API as Model Context Protocol tools, so
// an AI assistant can read and manage a Pulse instance with a scoped API key.
package mcpcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// maxResponse caps how much of one API response is passed to the model.
const maxResponse = 4 << 20

// NewMCPCmd returns the "mcp" command, an MCP server on stdin and stdout.
func NewMCPCmd() *cobra.Command {
	var server, tokenFile string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve the Pulse API as MCP tools on stdin and stdout",
		Long: `Serve the Pulse API as Model Context Protocol tools on stdin and stdout.

The server URL comes from --server or PULSE_URL. The API key comes from
PULSE_API_KEY or --token-file, never from a flag, so it stays out of process
listings. The key's scopes decide which tools work.

  claude mcp add pulse --env PULSE_URL=https://status.example.com \
    --env PULSE_API_KEY=pulse_live_... -- pulse-cli mcp`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, token, err := settings(server, tokenFile, os.Getenv)
			if err != nil {
				return err
			}
			return NewServer(base, token, nil).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
	cmd.Flags().StringVarP(&server, "server", "s", "", "Pulse base URL, e.g. https://status.example.com (default $PULSE_URL)")
	cmd.Flags().StringVar(&tokenFile, "token-file", "", "File holding the API key, read when PULSE_API_KEY is unset")
	return cmd
}

// settings resolves the Pulse base URL and API key from flags and environment.
func settings(server, tokenFile string, getenv func(string) string) (string, string, error) {
	if server == "" {
		server = getenv("PULSE_URL")
	}
	if server == "" {
		return "", "", errors.New("set the Pulse URL with --server or PULSE_URL")
	}
	u, err := url.Parse(server)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", fmt.Errorf("pulse URL %q must be an http or https URL", server)
	}
	token := getenv("PULSE_API_KEY")
	if token == "" && tokenFile != "" {
		b, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", "", fmt.Errorf("read token file: %w", err)
		}
		token = strings.TrimSpace(string(b))
	}
	if token == "" {
		return "", "", errors.New("set the API key with PULSE_API_KEY or --token-file")
	}
	return strings.TrimRight(server, "/"), token, nil
}

// NewServer returns an MCP server whose tools call the Pulse API at baseURL with
// token. hc is the HTTP client to use, or nil for a default one.
func NewServer(baseURL, token string, hc *http.Client) *mcp.Server {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	c := client{base: baseURL, token: token, hc: hc}
	version := "devel"
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		version = bi.Main.Version
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "pulse", Title: "Pulse status page", Version: version}, nil)
	for _, t := range tools {
		s.AddTool(t.mcpTool(), c.handler(t))
	}
	return s
}

func (t tool) mcpTool() *mcp.Tool {
	props := map[string]any{}
	for _, name := range t.pathArgs {
		props[name] = integer("ID, from the matching list tool.")
	}
	for name, p := range t.query {
		props[name] = p
	}
	for name, p := range t.body {
		props[name] = p
	}
	schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if required := slices.Concat(t.pathArgs, t.required); len(required) > 0 {
		schema["required"] = required
	}
	destructive := t.destructive
	return &mcp.Tool{
		Name:        t.name,
		Description: t.description,
		InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    t.readOnly,
			DestructiveHint: &destructive,
			IdempotentHint:  t.readOnly || t.method == http.MethodPut || t.method == http.MethodDelete,
		},
	}
}

type client struct {
	base, token string
	hc          *http.Client
}

// toolError reports a failure to the model as a tool result, not a protocol
// error, so the model can read it and correct its call.
func toolError(format string, a ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, a...)}}}
}

func (c client) handler(t tool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := map[string]json.RawMessage{}
		if raw := req.Params.Arguments; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &args); err != nil {
				return toolError("arguments must be a JSON object"), nil
			}
		}
		for name := range args {
			if !slices.Contains(t.pathArgs, name) && t.query[name] == nil && t.body[name] == nil {
				return toolError("unknown argument %q", name), nil
			}
		}

		path := t.path
		for _, name := range t.pathArgs {
			id, ok := positiveInt(args[name])
			if !ok {
				return toolError("%s must be a positive integer", name), nil
			}
			path = strings.Replace(path, "{"+name+"}", strconv.FormatInt(id, 10), 1)
		}
		query := url.Values{}
		for name := range t.query {
			if raw, ok := args[name]; ok {
				n, ok := positiveInt(raw)
				if !ok {
					return toolError("%s must be a positive integer", name), nil
				}
				query.Set(name, strconv.FormatInt(n, 10))
			}
		}
		if len(query) > 0 {
			path += "?" + query.Encode()
		}

		var body io.Reader
		if t.body != nil {
			payload := map[string]json.RawMessage{}
			for name := range t.body {
				if raw, ok := args[name]; ok {
					payload[name] = raw
				}
			}
			for _, name := range t.required {
				if _, ok := payload[name]; !ok {
					return toolError("%s is required", name), nil
				}
			}
			b, err := json.Marshal(payload)
			if err != nil {
				return nil, err
			}
			body = bytes.NewReader(b)
		}

		status, text, err := c.do(ctx, t.method, path, body)
		if err != nil {
			return toolError("%s %s: %v", t.method, t.path, err), nil
		}
		if status >= 400 {
			return toolError("HTTP %d from %s %s: %s", status, t.method, t.path, text), nil
		}
		if text == "" {
			text = fmt.Sprintf("HTTP %d, done.", status)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
	}
}

func positiveInt(raw json.RawMessage) (int64, bool) {
	var n int64
	if len(raw) == 0 || json.Unmarshal(raw, &n) != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// do sends one API request and returns the status and the trimmed body.
func (c client) do(ctx context.Context, method, path string, body io.Reader) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, "", err
	}
	// The body is only read, so a failed close loses nothing.
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return 0, "", err
	}
	text := strings.TrimSpace(string(b))
	if len(b) > maxResponse {
		text = string(b[:maxResponse]) + "\n[truncated: the response is larger than 4 MiB]"
	}
	return resp.StatusCode, text, nil
}
