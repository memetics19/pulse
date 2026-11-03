package web

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGroupSummary(t *testing.T) {
	m := func(states ...string) []monitorVM {
		out := make([]monitorVM, len(states))
		for i, s := range states {
			out[i] = monitorVM{StatusClass: s}
		}
		return out
	}
	for _, tc := range []struct {
		in          []monitorVM
		text, class string
	}{
		{nil, "", ""},
		{m("up", "up"), "All operational", "up"},
		{m("up", "degraded", "degraded"), "2 degraded", "degraded"},
		{m("degraded", "down"), "1 down", "down"},
		{m("nodata"), "No data yet", "nodata"},
		{m("up", "nodata"), "All operational", "up"},
	} {
		text, class := groupSummary(tc.in)
		assert.Equal(t, tc.text, text)
		assert.Equal(t, tc.class, class)
	}
}

func TestHumanDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		20 * time.Second:              "under a minute",
		35 * time.Minute:              "35 min",
		2 * time.Hour:                 "2 h",
		2*time.Hour + 5*time.Minute:   "2 h 5 min",
		48 * time.Hour:                "2 d",
		50*time.Hour + 10*time.Minute: "2 d 2 h",
	} {
		assert.Equal(t, want, humanDuration(d), d.String())
	}
}

func TestCapitalize(t *testing.T) {
	assert.Equal(t, "Investigating", capitalize("investigating"))
	assert.Empty(t, capitalize(""))
}
