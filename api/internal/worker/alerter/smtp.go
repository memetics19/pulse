package alerter

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/netguard"
)

// SMTP TLS modes (PULSE_SMTP_TLS).
const (
	smtpStartTLS = "starttls"
	smtpTLS      = "tls"
	smtpNone     = "none"
)

// smtpDefaultPorts is the port each TLS mode uses when PULSE_SMTP_PORT is unset.
var smtpDefaultPorts = map[string]int{smtpStartTLS: 587, smtpTLS: 465, smtpNone: 25}

// smtpTimeout bounds one whole send: connect, TLS, AUTH and the message. A relay
// that hangs must not hold an alert forever.
const smtpTimeout = 30 * time.Second

type smtpSender struct {
	host         string
	addr         string // host:port
	mode         string
	user         string
	password     string
	from         *mail.Address
	allowPrivate bool
	timeout      time.Duration
	// rootCAs verifies the relay's certificate. nil means the system roots. Tests
	// set it to trust their own certificate.
	rootCAs *x509.CertPool
}

// NewSMTPSender returns an email sender that delivers through the relay in cfg.
// from (PULSE_EMAIL_FROM) is the sender. allowPrivate is PULSE_ALLOW_PRIVATE_MONITORS:
// without it the relay cannot be on a private or loopback address, the same rule
// webhooks follow. A setting that cannot work makes every send fail with the reason.
func NewSMTPSender(cfg config.SMTP, from string, allowPrivate bool) Sender {
	s, err := newSMTPSender(cfg, from, allowPrivate)
	if err != nil {
		return &misconfiguredSender{err: err}
	}
	return s
}

func newSMTPSender(cfg config.SMTP, from string, allowPrivate bool) (*smtpSender, error) {
	mode := cfg.TLS
	if mode == "" {
		mode = smtpStartTLS
	}
	port, ok := smtpDefaultPorts[mode]
	if !ok {
		return nil, fmt.Errorf("smtp: PULSE_SMTP_TLS must be starttls, tls or none, got %q", cfg.TLS)
	}
	if cfg.Port != "" {
		p, err := strconv.Atoi(cfg.Port)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("smtp: PULSE_SMTP_PORT %q is not a port number", cfg.Port)
		}
		port = p
	}
	if mode == smtpNone && !isLocalhost(cfg.Host) {
		return nil, fmt.Errorf("smtp: PULSE_SMTP_TLS=none is only allowed for a relay on localhost, " +
			"because the password and the alerts would cross the network in clear text")
	}
	if cfg.User != "" && cfg.Password == "" {
		return nil, fmt.Errorf("smtp: PULSE_SMTP_USER is set but PULSE_SMTP_PASSWORD and PULSE_SMTP_PASSWORD_FILE are empty")
	}
	if err := CheckEmailFrom(from); err != nil {
		return nil, err
	}
	addr, _ := mail.ParseAddress(from) // checked just above
	return &smtpSender{
		host:         cfg.Host,
		addr:         net.JoinHostPort(cfg.Host, strconv.Itoa(port)),
		mode:         mode,
		user:         cfg.User,
		password:     cfg.Password,
		from:         addr,
		allowPrivate: allowPrivate,
		timeout:      smtpTimeout,
	}, nil
}

// isLocalhost reports whether host names this machine: localhost or a loopback IP.
func isLocalhost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *smtpSender) Send(ctx context.Context, cfg map[string]string, a Alert) error {
	to := cfg["to"]
	if to == "" {
		return nil
	}
	rcpt, err := mail.ParseAddress(to)
	if err != nil || strings.ContainsAny(to, "\r\n") {
		return fmt.Errorf("smtp: %q is not an email address", to)
	}
	msg, err := renderEmail(a)
	if err != nil {
		return err
	}
	body, err := buildMessage(s.from, rcpt, msg, time.Now())
	if err != nil {
		return fmt.Errorf("smtp: %w", err)
	}
	if err := s.deliver(ctx, rcpt.Address, body); err != nil {
		return fmt.Errorf("smtp: %s: %w", s.addr, err)
	}
	return nil
}

func (s *smtpSender) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: s.host, RootCAs: s.rootCAs, MinVersion: tls.VersionTLS12}
}

// deliver runs one SMTP session. The connection goes through the netguard dial
// guard, and the whole session ends when ctx is done or the timeout passes.
// Errors quote the relay's reply but never the password.
func (s *smtpSender) deliver(ctx context.Context, rcpt string, body []byte) (err error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	d := &net.Dialer{Control: netguard.DialControl(s.allowPrivate)}
	conn, err := d.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
		if err != nil && ctx.Err() != nil {
			err = fmt.Errorf("%w: %v", ctx.Err(), err)
		}
	}()
	// Closing the connection unblocks any read or write when ctx ends early.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	var c *smtp.Client
	if s.mode == smtpTLS {
		tc := tls.Client(conn, s.tlsConfig())
		if err := tc.HandshakeContext(ctx); err != nil {
			return err
		}
		c, err = smtp.NewClient(tc, s.host)
	} else {
		c, err = smtp.NewClient(conn, s.host)
	}
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()

	if s.mode == smtpStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("the relay does not offer STARTTLS (use PULSE_SMTP_TLS=tls for a relay that expects TLS from the start)")
		}
		if err := c.StartTLS(s.tlsConfig()); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	if s.user != "" {
		if err := c.Auth(smtp.PlainAuth("", s.user, s.password, s.host)); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := c.Rcpt(rcpt); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("data: %w", err)
	}
	return c.Quit()
}

// buildMessage writes the email as multipart/alternative: the text part, then the
// HTML part, both quoted-printable. The subject is one line already (renderEmail)
// and is encoded as an RFC 2047 word, so nothing in it can start a new header.
func buildMessage(from, to *mail.Address, m email, now time.Time) ([]byte, error) {
	var parts bytes.Buffer
	mw := multipart.NewWriter(&parts)
	for _, p := range []struct{ typ, body string }{
		{"text/plain; charset=utf-8", m.text},
		{"text/html; charset=utf-8", m.html},
	} {
		w, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {p.typ},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(w)
		if _, err := qp.Write([]byte(p.body)); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	var b bytes.Buffer
	for _, h := range [][2]string{
		{"From", from.String()},
		{"To", to.String()},
		{"Subject", mime.QEncoding.Encode("utf-8", m.subject)},
		{"Date", now.Format(time.RFC1123Z)},
		{"Message-ID", messageID(from.Address)},
		{"MIME-Version", "1.0"},
		{"Content-Type", `multipart/alternative; boundary="` + mw.Boundary() + `"`},
	} {
		b.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	b.WriteString("\r\n")
	b.Write(parts.Bytes())
	return b.Bytes(), nil
}

// messageID returns a unique Message-ID on the sender's domain.
func messageID(from string) string {
	domain := "pulse.invalid"
	if i := strings.LastIndexByte(from, '@'); i >= 0 && i < len(from)-1 {
		domain = from[i+1:]
	}
	var r [16]byte
	_, _ = rand.Read(r[:])
	return "<" + hex.EncodeToString(r[:]) + "@" + domain + ">"
}
