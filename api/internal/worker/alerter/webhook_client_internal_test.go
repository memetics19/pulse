package alerter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The production webhook client does not follow redirects: a redirect could
// point anywhere, including where the URL check would have refused to go.
// Only the TLS trust is swapped, so the test server's certificate is accepted.
func TestWebhookClientDoesNotFollowRedirects(t *testing.T) {
	var followed bool
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed = true
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	client := newWebhookClient(true)
	client.Transport.(*http.Transport).TLSClientConfig = redirect.Client().Transport.(*http.Transport).TLSClientConfig

	s := NewSlackSender("", true, WithHTTPClient(client))
	err := s.Send(context.Background(), map[string]string{"webhook_url": redirect.URL}, Alert{Title: "t", Status: "detected"})
	assert.Error(t, err, "a 3xx is a failed send")
	assert.False(t, followed, "the redirect must not be followed")
}
