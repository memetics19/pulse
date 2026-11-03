package uptimekuma_test

import (
	"testing"

	"github.com/memetics19/pulse/cli/internal/uptimekuma"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const httpOptionsJSON = `{
  "monitorList": [
    {
      "id": 1, "name": "Full", "url": "https://api.example.com/health", "type": "http",
      "interval": 60, "method": "post", "body": "{\"ping\":true}",
      "headers": "{\"X-Api-Key\":\"k1\",\"Authorization\":\"Bearer tok\"}",
      "ignoreTls": true, "maxredirects": 0,
      "accepted_statuscodes": ["200-299", "301"]
    },
    {
      "id": 2, "name": "Basic", "url": "https://intranet.example.com", "type": "keyword",
      "interval": 60, "method": "GET", "body": "ignored on GET",
      "authMethod": "basic", "basic_auth_user": "admin", "basic_auth_pass": "pw",
      "keyword": "error", "invertKeyword": 1, "maxredirects": 10, "headers": null
    },
    {
      "id": 3, "name": "Legacy basic", "url": "https://old.example.com", "type": "http",
      "interval": 60, "authMethod": null, "basic_auth_user": "u", "basic_auth_pass": "p",
      "headers": "", "ignoreTls": false, "invertKeyword": false
    },
    {
      "id": 4, "name": "Object headers", "url": "https://h.example.com", "type": "http",
      "interval": 60, "method": "HEAD", "headers": {"X-A": "1"}
    },
    {"id": 5, "name": "PUT", "url": "https://p.example.com", "type": "http", "interval": 60, "method": "PUT"},
    {"id": 6, "name": "NTLM", "url": "https://n.example.com", "type": "http", "interval": 60, "authMethod": "ntlm"},
    {"id": 7, "name": "Token scheme", "url": "https://t.example.com", "type": "http", "interval": 60,
     "headers": "{\"Authorization\":\"Token abc\"}"},
    {"id": 8, "name": "Bad headers", "url": "https://b.example.com", "type": "http", "interval": 60,
     "headers": "{not json"},
    {"id": 9, "name": "Two auths", "url": "https://two.example.com", "type": "http", "interval": 60,
     "headers": "{\"Authorization\":\"Bearer x\"}", "basic_auth_user": "u"}
  ]
}`

func TestParse_HTTPOptions(t *testing.T) {
	monitors, skipped, err := uptimekuma.Parse([]byte(httpOptionsJSON))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"PUT", "NTLM", "Token scheme", "Bad headers", "Two auths"}, skipped)

	full := findByName(monitors, "Full")
	require.NotNil(t, full)
	assert.Equal(t, "POST", full.Method)
	assert.Equal(t, `{"ping":true}`, full.Body)
	assert.Equal(t, map[string]string{"X-Api-Key": "k1"}, full.Headers)
	assert.Equal(t, &uptimekuma.PulseAuth{Type: "bearer", Token: "tok"}, full.Auth)
	require.NotNil(t, full.TLSVerify)
	assert.False(t, *full.TLSVerify)
	require.NotNil(t, full.FollowRedirects)
	assert.False(t, *full.FollowRedirects)
	assert.Equal(t, []string{"200-299", "301"}, full.AcceptedStatuses)

	basic := findByName(monitors, "Basic")
	require.NotNil(t, basic)
	assert.Equal(t, "http", basic.Type, "keyword monitors map to http")
	assert.Empty(t, basic.Method, "GET is the default and is left out")
	assert.Empty(t, basic.Body, "a body is only sent with POST")
	assert.Equal(t, &uptimekuma.PulseAuth{Type: "basic", Username: "admin", Password: "pw"}, basic.Auth)
	assert.Equal(t, "not_contains", basic.KeywordMode)
	assert.Nil(t, basic.FollowRedirects)
	assert.Nil(t, basic.Headers)

	legacy := findByName(monitors, "Legacy basic")
	require.NotNil(t, legacy)
	assert.Equal(t, &uptimekuma.PulseAuth{Type: "basic", Username: "u", Password: "p"}, legacy.Auth)
	assert.Nil(t, legacy.TLSVerify)
	assert.Empty(t, legacy.KeywordMode)

	head := findByName(monitors, "Object headers")
	require.NotNil(t, head)
	assert.Equal(t, "HEAD", head.Method)
	assert.Equal(t, map[string]string{"X-A": "1"}, head.Headers)
}
