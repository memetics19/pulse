package alerter

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/config"
)

// fakeSMTP is an in-process SMTP server that speaks just enough of the protocol for
// net/smtp: EHLO, STARTTLS, AUTH PLAIN, MAIL, RCPT, DATA and QUIT.
type fakeSMTP struct {
	t        *testing.T
	ln       net.Listener
	cert     tls.Certificate
	starttls bool   // offer STARTTLS
	implicit bool   // the listener speaks TLS from the start
	user     string // when set, offer AUTH PLAIN and require these credentials
	pass     string
	mailErr  string // when set, the reply to MAIL FROM
	silent   bool   // accept connections but never greet

	mu   sync.Mutex
	from string
	rcpt string
	data string
	auth bool
}

func newFakeSMTP(t *testing.T, f *fakeSMTP) (*fakeSMTP, *x509.CertPool) {
	t.Helper()
	cert, pool := testCert(t)
	f.t, f.cert = t, cert
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if f.implicit {
		ln = tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
	}
	f.ln = ln
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f, pool
}

func (f *fakeSMTP) port() string {
	_, p, _ := net.SplitHostPort(f.ln.Addr().String())
	return p
}

func (f *fakeSMTP) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	if f.silent {
		_, _ = io.Copy(io.Discard, c)
		return
	}
	_, isTLS := c.(*tls.Conn)
	tp := textproto.NewConn(c)
	reply := func(s string) { _ = tp.PrintfLine("%s", s) }
	reply("220 fake ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO", "HELO":
			lines := []string{"250-fake"}
			if f.starttls && !isTLS {
				lines = append(lines, "250-STARTTLS")
			}
			if f.user != "" {
				lines = append(lines, "250-AUTH PLAIN")
			}
			lines = append(lines, "250 8BITMIME")
			for _, l := range lines {
				reply(l)
			}
		case "STARTTLS":
			reply("220 go ahead")
			tc := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{f.cert}})
			if err := tc.Handshake(); err != nil {
				return
			}
			c, isTLS = tc, true
			tp = textproto.NewConn(c)
		case "AUTH":
			_, b64, _ := strings.Cut(arg, " ")
			raw, _ := base64.StdEncoding.DecodeString(b64)
			if string(raw) != "\x00"+f.user+"\x00"+f.pass {
				reply("535 5.7.8 authentication failed")
				continue
			}
			f.mu.Lock()
			f.auth = true
			f.mu.Unlock()
			reply("235 2.7.0 ok")
		case "MAIL":
			if f.mailErr != "" {
				reply(f.mailErr)
				continue
			}
			f.mu.Lock()
			f.from = arg
			f.mu.Unlock()
			reply("250 ok")
		case "RCPT":
			f.mu.Lock()
			f.rcpt = arg
			f.mu.Unlock()
			reply("250 ok")
		case "DATA":
			reply("354 go on")
			b, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.data = string(b)
			f.mu.Unlock()
			reply("250 queued")
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("250 ok")
		}
	}
}

func (f *fakeSMTP) got() (from, rcpt, data string, auth bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.from, f.rcpt, f.data, f.auth
}

// testCert makes a self-signed certificate for 127.0.0.1 and the pool that trusts it.
func testCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "fake smtp"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

// smtpTo builds a sender for the fake server. It trusts the fake's certificate.
func smtpTo(t *testing.T, f *fakeSMTP, pool *x509.CertPool, mode, user, pass string) *smtpSender {
	t.Helper()
	s, err := newSMTPSender(config.SMTP{Host: "127.0.0.1", Port: f.port(), User: user, Password: pass, TLS: mode},
		"Pulse <alerts@example.com>", true)
	if err != nil {
		t.Fatal(err)
	}
	s.rootCAs = pool
	return s
}

var testAlert = Alert{Title: "API\r\nBcc: x@example.com", Status: "detected", Severity: "major", Message: `<a href="https://example.com">click</a>`}

// parseSent reads the delivered message and returns its subject and the text and HTML parts.
func parseSent(t *testing.T, data string) (subject, text, html string) {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(data))
	if err != nil {
		t.Fatalf("read message: %v\n%s", err, data)
	}
	if msg.Header.Get("Bcc") != "" {
		t.Errorf("the title added a Bcc header")
	}
	subject, err = new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p)
		switch {
		case strings.HasPrefix(p.Header.Get("Content-Type"), "text/plain"):
			text = string(b)
		case strings.HasPrefix(p.Header.Get("Content-Type"), "text/html"):
			html = string(b)
		}
	}
	return subject, text, html
}

func TestSMTPStartTLSWithAuth(t *testing.T) {
	f, pool := newFakeSMTP(t, &fakeSMTP{starttls: true, user: "pulse", pass: "s3cret"})
	s := smtpTo(t, f, pool, "", "pulse", "s3cret") // empty mode means starttls
	if err := s.Send(context.Background(), map[string]string{"to": "ops@example.com"}, testAlert); err != nil {
		t.Fatal(err)
	}
	from, rcpt, data, auth := f.got()
	if !auth {
		t.Error("the sender did not authenticate")
	}
	if !strings.HasPrefix(from, "FROM:<alerts@example.com>") || !strings.HasPrefix(rcpt, "TO:<ops@example.com>") {
		t.Errorf("envelope = %q, %q", from, rcpt)
	}
	subject, text, html := parseSent(t, data)
	if subject != "[Pulse] DETECTED — API Bcc: x@example.com" {
		t.Errorf("subject = %q, want one line", subject)
	}
	if !strings.Contains(html, "&lt;a href=&#34;https://example.com&#34;&gt;click&lt;/a&gt;") || strings.Contains(html, "<a ") {
		t.Errorf("html = %q, want the message escaped", html)
	}
	if !strings.Contains(text, `<a href="https://example.com">click</a>`) || !strings.Contains(text, "Severity: major") {
		t.Errorf("text = %q, want the plain text part", text)
	}
}

func TestSMTPImplicitTLS(t *testing.T) {
	f, pool := newFakeSMTP(t, &fakeSMTP{implicit: true, user: "pulse", pass: "s3cret"})
	s := smtpTo(t, f, pool, "tls", "pulse", "s3cret")
	if err := s.Send(context.Background(), map[string]string{"to": "ops@example.com"}, testAlert); err != nil {
		t.Fatal(err)
	}
	if _, _, data, auth := f.got(); !auth || data == "" {
		t.Errorf("auth = %v, data = %q, want a delivered, authenticated message", auth, data)
	}
}

// A relay on localhost may run without TLS. net/smtp still sends AUTH PLAIN to it.
func TestSMTPNoneOnLocalhost(t *testing.T) {
	f, pool := newFakeSMTP(t, &fakeSMTP{user: "pulse", pass: "s3cret"})
	s := smtpTo(t, f, pool, "none", "pulse", "s3cret")
	if err := s.Send(context.Background(), map[string]string{"to": "ops@example.com"}, testAlert); err != nil {
		t.Fatal(err)
	}
	if _, _, data, auth := f.got(); !auth || data == "" {
		t.Errorf("auth = %v, data = %q", auth, data)
	}
	// Without a user there is no AUTH at all.
	f2, pool2 := newFakeSMTP(t, &fakeSMTP{})
	if err := smtpTo(t, f2, pool2, "none", "", "").Send(context.Background(), map[string]string{"to": "ops@example.com"}, testAlert); err != nil {
		t.Fatal(err)
	}
}

func TestSMTPErrors(t *testing.T) {
	ctx := context.Background()
	to := map[string]string{"to": "ops@example.com"}

	t.Run("wrong password", func(t *testing.T) {
		f, pool := newFakeSMTP(t, &fakeSMTP{starttls: true, user: "pulse", pass: "s3cret"})
		err := smtpTo(t, f, pool, "starttls", "pulse", "wrong-pw").Send(ctx, to, testAlert)
		if err == nil || !strings.Contains(err.Error(), "535") {
			t.Fatalf("err = %v, want the relay's 535", err)
		}
		if strings.Contains(err.Error(), "wrong-pw") {
			t.Fatalf("err = %v quotes the password", err)
		}
	})
	t.Run("server refuses the sender", func(t *testing.T) {
		f, pool := newFakeSMTP(t, &fakeSMTP{starttls: true, mailErr: "550 5.7.1 relay denied"})
		err := smtpTo(t, f, pool, "starttls", "", "").Send(ctx, to, testAlert)
		if err == nil || !strings.Contains(err.Error(), "550") || !strings.Contains(err.Error(), "relay denied") {
			t.Fatalf("err = %v, want the relay's reply", err)
		}
	})
	t.Run("no STARTTLS offered", func(t *testing.T) {
		f, pool := newFakeSMTP(t, &fakeSMTP{})
		err := smtpTo(t, f, pool, "starttls", "", "").Send(ctx, to, testAlert)
		if err == nil || !strings.Contains(err.Error(), "does not offer STARTTLS") {
			t.Fatalf("err = %v, want a refusal to go on without TLS", err)
		}
		if _, _, data, _ := f.got(); data != "" {
			t.Fatal("the message was sent without TLS")
		}
	})
	t.Run("untrusted certificate", func(t *testing.T) {
		for _, mode := range []string{"starttls", "tls"} {
			f, _ := newFakeSMTP(t, &fakeSMTP{starttls: true, implicit: mode == "tls"})
			err := smtpTo(t, f, nil, mode, "", "").Send(ctx, to, testAlert)
			if err == nil || !strings.Contains(err.Error(), "certificate") {
				t.Fatalf("%s: err = %v, want a certificate error", mode, err)
			}
		}
	})
	t.Run("private relay without PULSE_ALLOW_PRIVATE_MONITORS", func(t *testing.T) {
		f, pool := newFakeSMTP(t, &fakeSMTP{starttls: true})
		s := smtpTo(t, f, pool, "starttls", "", "")
		s.allowPrivate = false
		err := s.Send(ctx, to, testAlert)
		if err == nil || !strings.Contains(err.Error(), "PULSE_ALLOW_PRIVATE_MONITORS") {
			t.Fatalf("err = %v, want the netguard refusal", err)
		}
	})
	t.Run("relay that never answers", func(t *testing.T) {
		f, pool := newFakeSMTP(t, &fakeSMTP{silent: true})
		s := smtpTo(t, f, pool, "starttls", "", "")
		s.timeout = 200 * time.Millisecond
		err := s.Send(ctx, to, testAlert)
		if err == nil || !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "deadline exceeded") {
			t.Fatalf("err = %v, want a timeout", err)
		}
	})
	t.Run("nothing listening", func(t *testing.T) {
		ln, _ := net.Listen("tcp", "127.0.0.1:0")
		_, port, _ := net.SplitHostPort(ln.Addr().String())
		_ = ln.Close()
		s, _ := newSMTPSender(config.SMTP{Host: "127.0.0.1", Port: port}, "alerts@example.com", true)
		if err := s.Send(ctx, to, testAlert); err == nil {
			t.Fatal("send to a closed port succeeded")
		}
	})
	t.Run("bad recipient", func(t *testing.T) {
		s, _ := newSMTPSender(config.SMTP{Host: "127.0.0.1"}, "alerts@example.com", true)
		if err := s.Send(ctx, map[string]string{"to": "not an address"}, testAlert); err == nil {
			t.Fatal("send to a bad address succeeded")
		}
		if err := s.Send(ctx, map[string]string{}, testAlert); err != nil {
			t.Fatalf("no recipient = %v, want nil", err)
		}
	})
}

func TestSMTPSettings(t *testing.T) {
	ok := func(c config.SMTP) *smtpSender {
		t.Helper()
		s, err := newSMTPSender(c, "alerts@example.com", false)
		if err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		return s
	}
	for mode, addr := range map[string]string{"": "smtp.example.com:587", "starttls": "smtp.example.com:587", "tls": "smtp.example.com:465"} {
		if s := ok(config.SMTP{Host: "smtp.example.com", TLS: mode}); s.addr != addr {
			t.Errorf("TLS %q: addr = %s, want %s", mode, s.addr, addr)
		}
	}
	if s := ok(config.SMTP{Host: "localhost", TLS: "none"}); s.addr != "localhost:25" {
		t.Errorf("none: addr = %s", s.addr)
	}
	if s := ok(config.SMTP{Host: "smtp.example.com", Port: "2525"}); s.addr != "smtp.example.com:2525" {
		t.Errorf("port: addr = %s", s.addr)
	}

	for _, tc := range []struct {
		cfg  config.SMTP
		from string
		want string
	}{
		{config.SMTP{Host: "smtp.example.com", TLS: "ssl"}, "alerts@example.com", "PULSE_SMTP_TLS must be"},
		{config.SMTP{Host: "smtp.example.com", Port: "smtp"}, "alerts@example.com", "PULSE_SMTP_PORT"},
		{config.SMTP{Host: "smtp.example.com", Port: "70000"}, "alerts@example.com", "PULSE_SMTP_PORT"},
		{config.SMTP{Host: "smtp.example.com", TLS: "none"}, "alerts@example.com", "only allowed for a relay on localhost"},
		{config.SMTP{Host: "192.0.2.10", TLS: "none"}, "alerts@example.com", "only allowed for a relay on localhost"},
		{config.SMTP{Host: "smtp.example.com", User: "pulse"}, "alerts@example.com", "PULSE_SMTP_PASSWORD"},
		{config.SMTP{Host: "smtp.example.com"}, "", "PULSE_EMAIL_FROM"},
	} {
		s := NewSMTPSender(tc.cfg, tc.from, false)
		err := s.Send(context.Background(), map[string]string{"to": "ops@example.com"}, Alert{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v from %q: send = %v, want %q", tc.cfg, tc.from, err, tc.want)
		}
	}
	if !isLocalhost("::1") || !isLocalhost("LOCALHOST") || isLocalhost("smtp.example.com") {
		t.Error("isLocalhost is wrong")
	}
}

// SMTP wins when both transports are set up. Without either, email is off.
func TestNewEmailPicksTheTransport(t *testing.T) {
	both := config.Config{ResendAPIKey: "re_fake_key", EmailFrom: "alerts@example.com", SMTP: config.SMTP{Host: "smtp.example.com"}}
	if _, ok := NewEmail(both).(*smtpSender); !ok {
		t.Error("with SMTP and Resend set, want the SMTP sender")
	}
	if _, ok := NewEmail(config.Config{ResendAPIKey: "re_fake_key", EmailFrom: "alerts@example.com"}).(*emailSender); !ok {
		t.Error("with only Resend set, want the Resend sender")
	}
	if _, ok := NewEmail(config.Config{}).(*noopSender); !ok {
		t.Error("with nothing set, want no email")
	}
}

func TestCheckEmail(t *testing.T) {
	for _, tc := range []struct {
		cfg  config.Config
		want string // empty means no error
	}{
		{config.Config{}, ""},
		{config.Config{ResendAPIKey: "re_fake_key", EmailFrom: "alerts@example.com"}, ""},
		{config.Config{ResendAPIKey: "re_fake_key"}, "PULSE_EMAIL_FROM"},
		{config.Config{EmailFrom: "alerts@example.com", SMTP: config.SMTP{Host: "smtp.example.com", TLS: "bad"}}, "PULSE_SMTP_TLS"},
		{config.Config{EmailFrom: "alerts@example.com", SMTP: config.SMTP{Host: "127.0.0.1"}}, "PULSE_ALLOW_PRIVATE_MONITORS"},
		{config.Config{EmailFrom: "alerts@example.com", SMTP: config.SMTP{Host: "127.0.0.1"}, AllowPrivateMonitors: true}, ""},
	} {
		err := CheckEmail(tc.cfg)
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("CheckEmail(%+v) = %v, want %q", tc.cfg, err, tc.want)
		}
	}
}

// The message is CRLF-terminated MIME that a strict reader accepts.
func TestBuildMessage(t *testing.T) {
	from, _ := mail.ParseAddress("Pulse <alerts@example.com>")
	to, _ := mail.ParseAddress("ops@example.com")
	m, _ := renderEmail(testAlert)
	b, err := buildMessage(from, to, m, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(strings.NewReader(string(b)))
	hdr, err := textproto.NewReader(r).ReadMIMEHeader()
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Get("From") != `"Pulse" <alerts@example.com>` || hdr.Get("To") != "<ops@example.com>" {
		t.Errorf("From = %q, To = %q", hdr.Get("From"), hdr.Get("To"))
	}
	if id := hdr.Get("Message-Id"); !strings.HasSuffix(id, "@example.com>") {
		t.Errorf("Message-ID = %q", id)
	}
	if messageID("nodomain") == "" || !strings.HasSuffix(messageID("x@"), "@pulse.invalid>") {
		t.Error("messageID without a domain")
	}
}
