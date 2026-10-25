package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/config"
)

func TestCheckEmailLogs(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	checkEmail(config.Config{})
	if buf.Len() != 0 {
		t.Fatalf("email off logged %q", buf.String())
	}

	checkEmail(config.Config{ResendAPIKey: "re_fake_key", SMTP: config.SMTP{Host: "smtp.example.com", TLS: "bad"}})
	out := buf.String()
	if !strings.Contains(out, "email alerts go through SMTP and RESEND_API_KEY is ignored") {
		t.Errorf("log = %q, want which transport wins", out)
	}
	if !strings.Contains(out, "PULSE_SMTP_TLS") {
		t.Errorf("log = %q, want the SMTP setting that cannot work", out)
	}
}
