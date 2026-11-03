package monitorvalidation_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/monitorvalidation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCAPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test CA"},
		NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestValidateHTTP(t *testing.T) {
	manyHeaders := map[string]string{}
	for i := 0; i <= monitorvalidation.MaxHeaders; i++ {
		manyHeaders[fmt.Sprintf("X-H%d", i)] = "v"
	}
	manyStatuses := make([]string, monitorvalidation.MaxStatusEntries+1)
	for i := range manyStatuses {
		manyStatuses[i] = "200"
	}

	tests := []struct {
		name string
		in   monitorvalidation.HTTPOptions
		want string
	}{
		{name: "zero value is valid"},
		{name: "HEAD", in: monitorvalidation.HTTPOptions{Method: "HEAD"}},
		{name: "POST with body", in: monitorvalidation.HTTPOptions{Method: "POST", Body: "x"}},
		{name: "PUT", in: monitorvalidation.HTTPOptions{Method: "PUT"}, want: "method must be GET, HEAD or POST"},
		{name: "body with GET", in: monitorvalidation.HTTPOptions{Method: "GET", Body: "x"}, want: "body is only sent with POST"},
		{
			name: "body too large",
			in:   monitorvalidation.HTTPOptions{Method: "POST", Body: strings.Repeat("a", monitorvalidation.MaxBodyBytes+1)},
			want: "body must be at most",
		},
		{name: "custom header", in: monitorvalidation.HTTPOptions{Headers: map[string]string{"X-Api-Key": "k"}}},
		{name: "too many headers", in: monitorvalidation.HTTPOptions{Headers: manyHeaders}, want: "at most 32 entries"},
		{name: "bad header name", in: monitorvalidation.HTTPOptions{Headers: map[string]string{"X Bad": "v"}}, want: "is not valid"},
		{name: "empty header name", in: monitorvalidation.HTTPOptions{Headers: map[string]string{"": "v"}}, want: "is not valid"},
		{name: "Host header", in: monitorvalidation.HTTPOptions{Headers: map[string]string{"host": "internal"}}, want: `header "Host" cannot be set`},
		{name: "Content-Length header", in: monitorvalidation.HTTPOptions{Headers: map[string]string{"Content-Length": "1"}}, want: "cannot be set"},
		{name: "Transfer-Encoding header", in: monitorvalidation.HTTPOptions{Headers: map[string]string{"Transfer-Encoding": "chunked"}}, want: "cannot be set"},
		{name: "Authorization header", in: monitorvalidation.HTTPOptions{Headers: map[string]string{"authorization": "Bearer x"}}, want: "with auth, not headers"},
		{
			name: "duplicate header",
			in:   monitorvalidation.HTTPOptions{Headers: map[string]string{"x-a": "1", "X-A": "2"}},
			want: "is set twice",
		},
		{name: "CRLF in header value", in: monitorvalidation.HTTPOptions{Headers: map[string]string{"X-A": "a\r\nHost: x"}}, want: "control character"},
		{
			name: "header value too long",
			in:   monitorvalidation.HTTPOptions{Headers: map[string]string{"X-A": strings.Repeat("a", monitorvalidation.MaxHeaderValue+1)}},
			want: "value must be at most",
		},
		{name: "basic auth", in: monitorvalidation.HTTPOptions{AuthType: "basic", AuthUsername: "u", AuthSecret: "p"}},
		{name: "basic auth without user", in: monitorvalidation.HTTPOptions{AuthType: "basic"}, want: "needs a username"},
		{name: "basic user with colon", in: monitorvalidation.HTTPOptions{AuthType: "basic", AuthUsername: "a:b"}, want: "cannot contain a colon"},
		{name: "bearer auth", in: monitorvalidation.HTTPOptions{AuthType: "bearer", AuthSecret: "t"}},
		{name: "bearer without token", in: monitorvalidation.HTTPOptions{AuthType: "bearer"}, want: "needs a token"},
		{name: "unknown auth", in: monitorvalidation.HTTPOptions{AuthType: "ntlm"}, want: "must be basic or bearer"},
		{name: "secret with newline", in: monitorvalidation.HTTPOptions{AuthType: "bearer", AuthSecret: "t\n"}, want: "control characters"},
		{
			name: "secret too long",
			in:   monitorvalidation.HTTPOptions{AuthType: "bearer", AuthSecret: strings.Repeat("t", monitorvalidation.MaxAuthField+1)},
			want: "auth fields must be at most",
		},
		{name: "ca_pem", in: monitorvalidation.HTTPOptions{CAPEM: testCAPEM(t)}},
		{name: "ca_pem garbage", in: monitorvalidation.HTTPOptions{CAPEM: "nope"}, want: "no valid PEM certificate"},
		{
			name: "ca_pem too large",
			in:   monitorvalidation.HTTPOptions{CAPEM: strings.Repeat("a", monitorvalidation.MaxCAPEMBytes+1)},
			want: "ca_pem must be at most",
		},
		{name: "accepted statuses", in: monitorvalidation.HTTPOptions{AcceptedStatuses: []string{"200-299", "301"}}},
		{name: "bad accepted status", in: monitorvalidation.HTTPOptions{AcceptedStatuses: []string{"abc"}}, want: "100 to 599"},
		{name: "too many accepted statuses", in: monitorvalidation.HTTPOptions{AcceptedStatuses: manyStatuses}, want: "at most 32 entries"},
		{name: "contains", in: monitorvalidation.HTTPOptions{KeywordMode: "contains", Keyword: "ok"}},
		{name: "not_contains", in: monitorvalidation.HTTPOptions{KeywordMode: "not_contains", Keyword: "error"}},
		{name: "not_contains without keyword", in: monitorvalidation.HTTPOptions{KeywordMode: "not_contains"}, want: "needs keyword_check"},
		{name: "unknown keyword mode", in: monitorvalidation.HTTPOptions{KeywordMode: "regex"}, want: "contains or not_contains"},
		{name: "keyword with HEAD", in: monitorvalidation.HTTPOptions{Method: "HEAD", Keyword: "ok"}, want: "cannot be used with HEAD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitorvalidation.ValidateHTTP(tt.in)
			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			assert.Contains(t, got, tt.want)
		})
	}
}

func TestParseStatusRanges(t *testing.T) {
	got, err := monitorvalidation.ParseStatusRanges([]string{"200-299", " 301 ", "404 - 410"})
	require.NoError(t, err)
	require.Equal(t, []monitorvalidation.StatusRange{{Lo: 200, Hi: 299}, {Lo: 301, Hi: 301}, {Lo: 404, Hi: 410}}, got)
	assert.Equal(t, "200-299", got[0].String())
	assert.Equal(t, "301", got[1].String())
	assert.True(t, got[0].Contains(250))
	assert.False(t, got[0].Contains(300))

	none, err := monitorvalidation.ParseStatusRanges(nil)
	require.NoError(t, err)
	assert.Empty(t, none)

	for _, bad := range []string{"", "99", "600", "2xx", "300-200", "200-abc"} {
		_, err := monitorvalidation.ParseStatusRanges([]string{bad})
		assert.Error(t, err, bad)
	}
}
