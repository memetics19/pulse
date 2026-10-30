package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{in: -5 * time.Second, want: "0s"},
		{in: 45 * time.Second, want: "45s"},
		{in: 7*time.Minute + 20*time.Second, want: "7m"},
		{in: time.Hour, want: "1h 0m"},
		{in: time.Hour + 19*time.Minute, want: "1h 19m"},
		{in: 26*time.Hour + 5*time.Minute, want: "26h 5m"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, humanDuration(tt.in), tt.in.String())
	}
}
