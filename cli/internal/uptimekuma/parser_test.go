package uptimekuma_test

import (
	"testing"

	"github.com/memetics19/pulse/cli/internal/uptimekuma"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleJSON = `{
  "monitorList": [
    {
      "id": 1,
      "name": "My API",
      "url": "https://api.example.com",
      "type": "http",
      "interval": 60,
      "maxretries": 3,
      "keyword": "ok",
      "tags": []
    },
    {
      "id": 2,
      "name": "Push Monitor",
      "url": "",
      "type": "push",
      "interval": 30,
      "maxretries": 1,
      "keyword": "",
      "tags": []
    },
    {
      "id": 3,
      "name": "TCP Check",
      "url": "tcp://db.internal:5432",
      "type": "tcp",
      "interval": 120,
      "maxretries": 2,
      "keyword": "",
      "tags": []
    },
    {
      "id": 4,
      "name": "Unknown Type",
      "url": "https://whatever.example.com",
      "type": "grpc",
      "interval": 60,
      "maxretries": 1,
      "keyword": "",
      "tags": []
    },
    {
      "id": 5,
      "name": "DNS Check",
      "url": "example.com",
      "type": "dns",
      "interval": 300,
      "maxretries": 1,
      "keyword": "",
      "tags": []
    },
    {
      "id": 6,
      "name": "Ping Check",
      "url": "10.0.0.1",
      "type": "ping",
      "interval": 60,
      "maxretries": 1,
      "keyword": "",
      "tags": []
    },
    {
      "id": 7,
      "name": "SSL Monitor",
      "url": "https://secure.example.com",
      "type": "ssl",
      "interval": 86400,
      "maxretries": 1,
      "keyword": "",
      "tags": []
    }
  ],
  "notificationList": []
}`

func TestParse_ReturnsImportedAndSkipped(t *testing.T) {
	monitors, skipped, err := uptimekuma.Parse([]byte(sampleJSON))
	require.NoError(t, err)

	// id=4 (grpc) must be skipped; everything else imported
	assert.Equal(t, 6, len(monitors), "expected 6 imported monitors")
	assert.Equal(t, 1, len(skipped), "expected 1 skipped monitor")
	assert.Equal(t, "Unknown Type", skipped[0], "skipped name should be the grpc monitor")
}

func TestParse_HTTPMonitor(t *testing.T) {
	monitors, _, err := uptimekuma.Parse([]byte(sampleJSON))
	require.NoError(t, err)

	// id=1 maps directly
	m := findByName(monitors, "My API")
	require.NotNil(t, m)
	assert.Equal(t, "My API", m.Name)
	assert.Equal(t, "https://api.example.com", m.URL)
	assert.Equal(t, "http", m.Type)
	assert.Equal(t, int64(60), m.IntervalSeconds)
	assert.Equal(t, "ok", m.KeywordCheck)
}

func TestParse_PushMapsToHTTP(t *testing.T) {
	monitors, _, err := uptimekuma.Parse([]byte(sampleJSON))
	require.NoError(t, err)

	m := findByName(monitors, "Push Monitor")
	require.NotNil(t, m)
	assert.Equal(t, "http", m.Type, "push must be remapped to http")
	assert.Equal(t, int64(30), m.IntervalSeconds)
	assert.Equal(t, "", m.KeywordCheck)
}

func TestParse_TCPMonitor(t *testing.T) {
	monitors, _, err := uptimekuma.Parse([]byte(sampleJSON))
	require.NoError(t, err)

	m := findByName(monitors, "TCP Check")
	require.NotNil(t, m)
	assert.Equal(t, "tcp", m.Type)
	assert.Equal(t, "tcp://db.internal:5432", m.URL)
	assert.Equal(t, int64(120), m.IntervalSeconds)
}

func TestParse_DNSMonitor(t *testing.T) {
	monitors, _, err := uptimekuma.Parse([]byte(sampleJSON))
	require.NoError(t, err)

	m := findByName(monitors, "DNS Check")
	require.NotNil(t, m)
	assert.Equal(t, "dns", m.Type)
}

func TestParse_PingMonitor(t *testing.T) {
	monitors, _, err := uptimekuma.Parse([]byte(sampleJSON))
	require.NoError(t, err)

	m := findByName(monitors, "Ping Check")
	require.NotNil(t, m)
	assert.Equal(t, "ping", m.Type)
}

func TestParse_SSLMonitor(t *testing.T) {
	monitors, _, err := uptimekuma.Parse([]byte(sampleJSON))
	require.NoError(t, err)

	m := findByName(monitors, "SSL Monitor")
	require.NotNil(t, m)
	assert.Equal(t, "ssl", m.Type)
	assert.Equal(t, int64(86400), m.IntervalSeconds)
}

func TestParse_InvalidJSON(t *testing.T) {
	_, _, err := uptimekuma.Parse([]byte(`not json`))
	assert.Error(t, err)
}

func TestParse_EmptyMonitorList(t *testing.T) {
	monitors, skipped, err := uptimekuma.Parse([]byte(`{"monitorList":[],"notificationList":[]}`))
	require.NoError(t, err)
	assert.Empty(t, monitors)
	assert.Empty(t, skipped)
}

// findByName is a test helper — not exported.
func findByName(monitors []uptimekuma.PulseMonitor, name string) *uptimekuma.PulseMonitor {
	for i := range monitors {
		if monitors[i].Name == name {
			return &monitors[i]
		}
	}
	return nil
}
