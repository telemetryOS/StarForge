package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/telemetryos/starforge/installer"
	"github.com/telemetryos/starforge/installer/diskutil"
)

// Client is an HTTP client for the installer daemon REST API.
type Client struct {
	BaseURL string
	http    *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Installation mirrors the server's Installation type for JSON decoding.
type Installation struct {
	ID        string    `json:"id"`
	Payload   string    `json:"payload"`
	Disk      string    `json:"disk"`
	Status    string    `json:"status"`
	Progress  float64   `json:"progress"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// LogResponse is the response from the log polling endpoint.
type LogResponse struct {
	Lines  []string `json:"lines"`
	Offset int      `json:"offset"`
}

// SystemInfo is the response from GET /system.
type SystemInfo struct {
	Hostname string `json:"hostname"`
	Arch     string `json:"arch"`
	Memory   uint64 `json:"memory"`
}

// ListPayloads returns all available payloads.
func (c *Client) ListPayloads() ([]installer.PayloadManifest, error) {
	var result []installer.PayloadManifest
	if err := c.getJSON("/payloads", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetPayload returns details for a specific payload.
func (c *Client) GetPayload(name string) (*installer.PayloadManifest, error) {
	var result installer.PayloadManifest
	if err := c.getJSON(fmt.Sprintf("/payloads/%s", name), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListDisks returns all available target disks.
func (c *Client) ListDisks() ([]diskutil.Disk, error) {
	var result []diskutil.Disk
	if err := c.getJSON("/disks", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// StartInstallation begins an installation.
func (c *Client) StartInstallation(payload, disk string) (*Installation, error) {
	body := map[string]string{"payload": payload, "disk": disk}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Post(c.BaseURL+"/installations", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("starting installation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, c.readError(resp)
	}

	var result Installation
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetInstallation returns the current status of an installation.
func (c *Client) GetInstallation(id string) (*Installation, error) {
	var result Installation
	if err := c.getJSON(fmt.Sprintf("/installations/%s", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetLog returns log lines starting from the given offset.
func (c *Client) GetLog(id string, offset int) ([]string, int, error) {
	var result LogResponse
	if err := c.getJSON(fmt.Sprintf("/installations/%s/log?offset=%d", id, offset), &result); err != nil {
		return nil, 0, err
	}
	return result.Lines, result.Offset, nil
}

// Reboot sends a reboot request to the server.
func (c *Client) Reboot() error {
	resp, err := c.http.Post(c.BaseURL+"/system/reboot", "application/json", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// GetSystem returns system information.
func (c *Client) GetSystem() (*SystemInfo, error) {
	var result SystemInfo
	if err := c.getJSON("/system", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) getJSON(path string, target any) error {
	resp, err := c.http.Get(c.BaseURL + path)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func (c *Client) readError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to extract "error" field from JSON response body.
	var parsed struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error != "" {
		return fmt.Errorf("%s", parsed.Error)
	}

	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
}
