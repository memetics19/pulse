package alerter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A push monitor's msg reaches the alert unchanged: whoever holds the push URL writes it.
var hostile = Alert{
	Title:    `Shop <b>"API"</b>` + "\r\nBcc: victim@example.com",
	Status:   "detected",
	Severity: "critical",
	Message:  `<a href="https://evil.example/login">Click to restore service</a> & <!channel> <img src=x onerror=alert(1)>`,
}

func TestRenderEmailEscapesEveryField(t *testing.T) {
	m, err := renderEmail(hostile)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<a ", "<b>", "<img", "<!channel>", `href="https://evil`} {
		if strings.Contains(m.html, bad) {
			t.Errorf("html carries %q unescaped:\n%s", bad, m.html)
		}
	}
	for _, want := range []string{"&lt;a href=&#34;https://evil.example/login&#34;&gt;", "&amp; &lt;!channel&gt;", "Shop &lt;b&gt;&#34;API&#34;&lt;/b&gt;"} {
		if !strings.Contains(m.html, want) {
			t.Errorf("html is missing the escaped text %q:\n%s", want, m.html)
		}
	}
	// The layout itself is still HTML.
	if !strings.HasPrefix(m.html, "<h2>") || !strings.Contains(m.html, "<strong>Status:</strong> detected") {
		t.Errorf("html lost its markup:\n%s", m.html)
	}
}

func TestRenderEmailSubjectIsOneLine(t *testing.T) {
	m, err := renderEmail(hostile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(m.subject, "\r\n") {
		t.Errorf("subject has a line break: %q", m.subject)
	}
	if !strings.HasPrefix(m.subject, "[Pulse] DETECTED — Shop") {
		t.Errorf("subject = %q", m.subject)
	}
}

func TestRenderEmailHasAPlainTextPart(t *testing.T) {
	m, err := renderEmail(Alert{Title: "API", Status: "resolved", Severity: "minor", Message: "back up"})
	if err != nil {
		t.Fatal(err)
	}
	want := "API\n\nStatus: resolved\nSeverity: minor\n\nback up\n"
	if m.text != want {
		t.Errorf("text = %q, want %q", m.text, want)
	}
}

func TestSlackEscapesLinksMentionsAndEntities(t *testing.T) {
	var got string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var p map[string]string
		_ = json.Unmarshal(b, &p)
		got = p["text"]
	}))
	defer srv.Close()

	s := NewSlackSender("", true, WithHTTPClient(srv.Client()))
	if err := s.Send(context.Background(), map[string]string{"webhook_url": srv.URL}, hostile); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<a ", "<!channel>", "<https", "<img"} {
		if strings.Contains(got, bad) {
			t.Errorf("slack text carries %q unescaped: %q", bad, got)
		}
	}
	if !strings.Contains(got, "&lt;!channel&gt;") || !strings.Contains(got, " &amp; ") {
		t.Errorf("slack text is not escaped as Slack expects: %q", got)
	}
	if !strings.HasPrefix(got, ":red_circle: *[detected]* Shop") {
		t.Errorf("slack text lost its layout: %q", got)
	}
}
