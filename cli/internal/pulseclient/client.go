package pulseclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/memetics19/pulse/cli/internal/uptimekuma"
)

// Client sends authenticated requests to the Pulse REST API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// New returns a Client targeting baseURL (no trailing slash) with the given token.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{},
	}
}

// createGroupRequest is the POST /api/groups request body.
type createGroupRequest struct {
	Name         string `json:"name"`
	DisplayOrder int64  `json:"display_order"`
	Description  string `json:"description"`
}

// createGroupResponse captures only the id from the 201 response.
type createGroupResponse struct {
	ID int64 `json:"id"`
}

// CreateGroup POSTs to /api/groups and returns the new group's ID.
func (c *Client) CreateGroup(name string) (int64, error) {
	body, err := json.Marshal(createGroupRequest{Name: name, DisplayOrder: 0, Description: ""})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/groups", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("unexpected status %d from POST /api/groups", resp.StatusCode)
	}

	var gr createGroupResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return 0, err
	}
	return gr.ID, nil
}

// createMonitorRequest is the POST /api/monitors request body.
type createMonitorRequest struct {
	Name                string `json:"name"`
	URL                 string `json:"url"`
	Type                string `json:"type"`
	IntervalSeconds     int64  `json:"interval_seconds"`
	TimeoutSeconds      int64  `json:"timeout_seconds"`
	ExpectedStatus      *int64 `json:"expected_status"`
	KeywordCheck        string `json:"keyword_check"`
	DegradedThresholdMs int64  `json:"degraded_threshold_ms"`
	DownThresholdMs     int64  `json:"down_threshold_ms"`
	IsActive            int64  `json:"is_active"`
	GroupID             int64  `json:"group_id"`
	Source              string `json:"source"`
	ExternalID          string `json:"external_id"`
}

// CreateMonitor POSTs a single monitor to /api/monitors under the given groupID.
func (c *Client) CreateMonitor(m uptimekuma.PulseMonitor, groupID int64) error {
	payload := createMonitorRequest{
		Name:                m.Name,
		URL:                 m.URL,
		Type:                m.Type,
		IntervalSeconds:     m.IntervalSeconds,
		TimeoutSeconds:      30,
		ExpectedStatus:      nil,
		KeywordCheck:        m.KeywordCheck,
		DegradedThresholdMs: 1000,
		DownThresholdMs:     3000,
		IsActive:            1,
		GroupID:             groupID,
		Source:              "uptime-kuma",
		ExternalID:          "",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/monitors", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status %d from POST /api/monitors (monitor: %s)", resp.StatusCode, m.Name)
	}
	return nil
}
