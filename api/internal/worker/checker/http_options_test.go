package checker_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/monitorvalidation"
	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTLSServer starts a TLS test server with a self-signed certificate. Its
// error log is muted, since several tests fail the handshake on purpose.
func newTLSServer() *httptest.Server {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	return srv
}

// certPEM returns the PEM of a TLS test server's self-signed certificate.
func certPEM(srv *httptest.Server) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
}

func checkWith(t *testing.T, opts checker.HTTPOptions, target string) checker.Result {
	t.Helper()
	return checker.NewHTTPWithOptions(opts, true).Check(context.Background(), target, 5)
}

func TestHTTPOptions_Method(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Method
	}))
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{}, srv.URL)
	assert.Equal(t, "up", res.Status)
	assert.Equal(t, http.MethodGet, got, "the default method is GET")

	res = checkWith(t, checker.HTTPOptions{Method: http.MethodHead}, srv.URL)
	assert.Equal(t, "up", res.Status)
	assert.Equal(t, http.MethodHead, got)
}

func TestHTTPOptions_PostBody(t *testing.T) {
	var method, body, contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, contentType = r.Method, r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{
		Method:  http.MethodPost,
		Body:    `{"ping":true}`,
		Headers: map[string]string{"Content-Type": "application/json"},
	}, srv.URL)
	assert.Equal(t, "up", res.Status)
	assert.Equal(t, http.MethodPost, method)
	assert.Equal(t, `{"ping":true}`, body)
	assert.Equal(t, "application/json", contentType)
}

func TestHTTPOptions_Headers(t *testing.T) {
	var apiKey, agent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey, agent = r.Header.Get("X-Api-Key"), r.UserAgent()
	}))
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{Headers: map[string]string{"X-Api-Key": "k1"}}, srv.URL)
	assert.Equal(t, "up", res.Status)
	assert.Equal(t, "k1", apiKey)
	assert.Contains(t, agent, "Pulse/", "the default User-Agent stays")

	res = checkWith(t, checker.HTTPOptions{Headers: map[string]string{"User-Agent": "probe"}}, srv.URL)
	assert.Equal(t, "up", res.Status)
	assert.Equal(t, "probe", agent, "a monitor header overrides the User-Agent")
}

func TestHTTPOptions_BasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()

	assert.Equal(t, "down", checkWith(t, checker.HTTPOptions{}, srv.URL).Status)
	res := checkWith(t, checker.HTTPOptions{AuthType: "basic", AuthUsername: "admin", AuthSecret: "s3cret"}, srv.URL)
	assert.Equal(t, "up", res.Status)
}

func TestHTTPOptions_BearerAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{AuthType: "bearer", AuthSecret: "tok"}, srv.URL)
	assert.Equal(t, "up", res.Status)
	res = checkWith(t, checker.HTTPOptions{AuthType: "bearer", AuthSecret: "wrong"}, srv.URL)
	assert.Equal(t, "down", res.Status)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestHTTPOptions_SelfSignedFailsByDefault(t *testing.T) {
	srv := newTLSServer()
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{}, srv.URL)
	assert.Equal(t, "down", res.Status)
	assert.Contains(t, res.ErrorMessage, "certificate")
}

func TestHTTPOptions_SkipTLSVerify(t *testing.T) {
	srv := newTLSServer()
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{SkipTLSVerify: true}, srv.URL)
	assert.Equal(t, "up", res.Status, res.ErrorMessage)
}

func TestHTTPOptions_CAPEM(t *testing.T) {
	srv := newTLSServer()
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{CAPEM: certPEM(srv)}, srv.URL)
	assert.Equal(t, "up", res.Status, res.ErrorMessage)

	// A second check reuses the cached transport for the same CA.
	res = checkWith(t, checker.HTTPOptions{CAPEM: certPEM(srv)}, srv.URL)
	assert.Equal(t, "up", res.Status, res.ErrorMessage)
}

func TestHTTPOptions_CAPEMFromOtherCAFails(t *testing.T) {
	srv := newTLSServer()
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{CAPEM: newCAPEM(t)}, srv.URL)
	assert.Equal(t, "down", res.Status)
	assert.Contains(t, res.ErrorMessage, "certificate")
}

func TestHTTPOptions_InvalidCAPEM(t *testing.T) {
	res := checkWith(t, checker.HTTPOptions{CAPEM: "not a pem"}, "https://example.invalid")
	assert.Equal(t, "down", res.Status)
	assert.Contains(t, res.ErrorMessage, "ca_pem")
}

func TestHTTPOptions_CustomTLSKeepsSSRFGuard(t *testing.T) {
	srv := newTLSServer()
	defer srv.Close()

	for _, opts := range []checker.HTTPOptions{{SkipTLSVerify: true}, {CAPEM: certPEM(srv)}} {
		res := checker.NewHTTPWithOptions(opts, false).Check(context.Background(), srv.URL, 5)
		assert.Equal(t, "down", res.Status)
		assert.Contains(t, res.ErrorMessage, "netguard")
	}
}

func TestHTTPOptions_FollowRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusFound)
	})
	mux.HandleFunc("/new", func(http.ResponseWriter, *http.Request) {})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{}, srv.URL+"/old")
	assert.Equal(t, "up", res.Status)
	assert.Equal(t, http.StatusOK, res.StatusCode, "redirects are followed by default")

	res = checkWith(t, checker.HTTPOptions{NoFollowRedirects: true}, srv.URL+"/old")
	assert.Equal(t, "down", res.Status)
	assert.Equal(t, http.StatusFound, res.StatusCode)
	assert.Contains(t, res.ErrorMessage, "expected status 2xx, got 302")

	res = checkWith(t, checker.HTTPOptions{NoFollowRedirects: true, ExpectedStatus: 302}, srv.URL+"/old")
	assert.Equal(t, "up", res.Status, "the redirect itself can be the check")
}

func TestHTTPOptions_RedirectLoopStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	}))
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{}, srv.URL)
	assert.Equal(t, "down", res.Status)
	assert.Contains(t, res.ErrorMessage, "stopped after 10 redirects")
}

func TestHTTPOptions_CrossHostRedirectDropsSecrets(t *testing.T) {
	var apiKey, auth, agent string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey, auth, agent = r.Header.Get("X-Api-Key"), r.Header.Get("Authorization"), r.UserAgent()
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{
		Headers:  map[string]string{"X-Api-Key": "k1", "User-Agent": "probe"},
		AuthType: "bearer", AuthSecret: "tok",
	}, srv.URL)
	assert.Equal(t, "up", res.Status)
	assert.Empty(t, apiKey, "monitor headers stay on the monitored host")
	assert.Empty(t, auth, "auth stays on the monitored host")
	assert.Contains(t, agent, "Pulse/")
}

func TestHTTPOptions_SameHostRedirectKeepsHeaders(t *testing.T) {
	var apiKey string
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		apiKey = r.Header.Get("X-Api-Key")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res := checkWith(t, checker.HTTPOptions{Headers: map[string]string{"X-Api-Key": "k1"}}, srv.URL+"/old")
	assert.Equal(t, "up", res.Status)
	assert.Equal(t, "k1", apiKey)
}

func TestHTTPOptions_AcceptedStatuses(t *testing.T) {
	code := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
	}))
	defer srv.Close()

	ranges, err := monitorvalidation.ParseStatusRanges([]string{"200-299", "404"})
	require.NoError(t, err)
	opts := checker.HTTPOptions{AcceptedStatuses: ranges}

	for _, c := range []int{200, 204, 404} {
		code = c
		assert.Equal(t, "up", checkWith(t, opts, srv.URL).Status, "status %d", c)
	}
	code = http.StatusForbidden
	res := checkWith(t, opts, srv.URL)
	assert.Equal(t, "down", res.Status)
	assert.Contains(t, res.ErrorMessage, "expected status 200-299 or 404, got 403")

	// The expected status still counts alongside the ranges.
	opts.ExpectedStatus = http.StatusForbidden
	assert.Equal(t, "up", checkWith(t, opts, srv.URL).Status)
	code = http.StatusGone
	res = checkWith(t, opts, srv.URL)
	assert.Contains(t, res.ErrorMessage, "expected status 403 or 200-299 or 404, got 410")
}

func TestHTTPOptions_KeywordNotContains(t *testing.T) {
	body := "all good"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	opts := checker.HTTPOptions{Keyword: "error", KeywordAbsent: true}
	assert.Equal(t, "up", checkWith(t, opts, srv.URL).Status)

	body = "fatal error"
	res := checkWith(t, opts, srv.URL)
	assert.Equal(t, "down", res.Status)
	assert.Contains(t, res.ErrorMessage, `keyword "error" found`)
}

func TestHTTPOptions_BadMethodIsDown(t *testing.T) {
	res := checkWith(t, checker.HTTPOptions{Method: "BAD METHOD"}, "http://example.invalid")
	assert.Equal(t, "down", res.Status)
	assert.True(t, strings.Contains(res.ErrorMessage, "method"), res.ErrorMessage)
}

// newCAPEM returns a throwaway self-signed CA, unrelated to the httptest
// certificate, so it must not verify an httptest server.
func newCAPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
