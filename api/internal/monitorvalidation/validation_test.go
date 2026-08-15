package monitorvalidation_test

import (
	"testing"

	"github.com/memetics19/pulse/api/internal/monitorvalidation"
	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name         string
		input        monitorvalidation.Input
		allowPrivate bool
		want         string
	}{
		{
			name:  "push does not require a URL",
			input: monitorvalidation.Input{Type: "push", IntervalSeconds: 60},
		},
		{
			name:  "http requires a URL",
			input: monitorvalidation.Input{Type: "http", IntervalSeconds: 60},
			want:  "url is required",
		},
		{
			name:  "tcp rejects URL syntax",
			input: monitorvalidation.Input{URL: "tcp://db:5432", Type: "tcp", IntervalSeconds: 60},
			want:  "tcp target must be host:port",
		},
		{
			name:  "tcp accepts host and port",
			input: monitorvalidation.Input{URL: "db:5432", Type: "tcp", IntervalSeconds: 60},
		},
		{
			name:  "tcp accepts IPv6 host and port",
			input: monitorvalidation.Input{URL: "[2001:db8::1]:443", Type: "tcp", IntervalSeconds: 60},
		},
		{
			name:  "tcp rejects missing port",
			input: monitorvalidation.Input{URL: "db", Type: "tcp", IntervalSeconds: 60},
			want:  "tcp target must be host:port",
		},
		{
			name:  "interval must be positive",
			input: monitorvalidation.Input{Type: "push", IntervalSeconds: 0},
			want:  "interval_seconds must be at least 1",
		},
		{
			name:  "type must be supported",
			input: monitorvalidation.Input{URL: "example.com", Type: "smtp", IntervalSeconds: 60},
			want:  "invalid type",
		},
		{
			name:         "https accepts a valid public URL",
			input:        monitorvalidation.Input{URL: "https://example.com/health", Type: "https", IntervalSeconds: 60},
			allowPrivate: false,
		},
		{
			name:         "https rejects private URL",
			input:        monitorvalidation.Input{URL: "https://127.0.0.1/health", Type: "https", IntervalSeconds: 60},
			allowPrivate: false,
			want:         "127.0.0.1 is a private or internal address (set PULSE_ALLOW_PRIVATE_MONITORS=true to allow)",
		},
		{
			name:         "https permits private URL when configured",
			input:        monitorvalidation.Input{URL: "https://127.0.0.1/health", Type: "https", IntervalSeconds: 60},
			allowPrivate: true,
		},
		{
			name:         "https rejects a non-HTTP scheme",
			input:        monitorvalidation.Input{URL: "ftp://example.com/health", Type: "https", IntervalSeconds: 60},
			allowPrivate: true,
			want:         `url scheme must be http or https, got "ftp"`,
		},
		{
			name:         "tcp rejects a private target",
			input:        monitorvalidation.Input{URL: "127.0.0.1:5432", Type: "tcp", IntervalSeconds: 60},
			allowPrivate: false,
			want:         "127.0.0.1 is a private or internal address (set PULSE_ALLOW_PRIVATE_MONITORS=true to allow)",
		},
		{
			name:         "tcp permits a private target when configured",
			input:        monitorvalidation.Input{URL: "127.0.0.1:5432", Type: "tcp", IntervalSeconds: 60},
			allowPrivate: true,
		},
		{
			name:         "ssl rejects a private target",
			input:        monitorvalidation.Input{URL: "127.0.0.1:443", Type: "ssl", IntervalSeconds: 60},
			allowPrivate: false,
			want:         "127.0.0.1 is a private or internal address (set PULSE_ALLOW_PRIVATE_MONITORS=true to allow)",
		},
		{
			name:         "ssl permits a private target when configured",
			input:        monitorvalidation.Input{URL: "127.0.0.1:443", Type: "ssl", IntervalSeconds: 60},
			allowPrivate: true,
		},
		{
			name:         "ping rejects a private target",
			input:        monitorvalidation.Input{URL: "127.0.0.1", Type: "ping", IntervalSeconds: 60},
			allowPrivate: false,
			want:         "127.0.0.1 is a private or internal address (set PULSE_ALLOW_PRIVATE_MONITORS=true to allow)",
		},
		{
			name:         "ping permits a private target when configured",
			input:        monitorvalidation.Input{URL: "127.0.0.1", Type: "ping", IntervalSeconds: 60},
			allowPrivate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, monitorvalidation.Validate(tt.input, tt.allowPrivate))
		})
	}
}
