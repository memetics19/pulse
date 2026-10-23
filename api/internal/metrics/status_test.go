package metrics

import "testing"

func TestStatusValue(t *testing.T) {
	for status, want := range map[string]float64{"up": 1, "degraded": 0.5, "down": 0} {
		if got, ok := statusValue(status); !ok || got != want {
			t.Errorf("statusValue(%q) = %v, %v; want %v, true", status, got, ok, want)
		}
	}
	if _, ok := statusValue("unknown"); ok {
		t.Error("statusValue(unknown) reported ok")
	}
}
