package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/telemetryos/starforge/installer"
	"github.com/telemetryos/starforge/installer/diskutil"
)

func TestNewClient_Timeout(t *testing.T) {
	c := NewClient("http://localhost:8100")
	if c.http.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", c.http.Timeout)
	}
}

func TestGetSystem_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system" {
			t.Errorf("expected /system, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SystemInfo{Hostname: "edge-01", Arch: "amd64", Memory: 8 << 30})
	}))
	defer srv.Close()

	info, err := NewClient(srv.URL).GetSystem()
	if err != nil {
		t.Fatalf("GetSystem error: %v", err)
	}
	if info.Hostname != "edge-01" {
		t.Errorf("Hostname = %q, want %q", info.Hostname, "edge-01")
	}
	if info.Arch != "amd64" {
		t.Errorf("Arch = %q, want %q", info.Arch, "amd64")
	}
}

func TestGetSystem_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"internal error"}`)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).GetSystem()
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestListPayloads_Success(t *testing.T) {
	payloads := []installer.PayloadManifest{{Name: "edge-os"}, {Name: "recovery"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(payloads)
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL).ListPayloads()
	if err != nil {
		t.Fatalf("ListPayloads error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 payloads, got %d", len(got))
	}
	if got[0].Name != "edge-os" {
		t.Errorf("first payload = %q, want %q", got[0].Name, "edge-os")
	}
}

func TestListPayloads_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "[]")
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL).ListPayloads()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 payloads, got %d", len(got))
	}
}

func TestListDisks_Success(t *testing.T) {
	disks := []diskutil.Disk{{Name: "sda", Path: "/dev/sda", Size: 500 << 30}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(disks)
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL).ListDisks()
	if err != nil {
		t.Fatalf("ListDisks error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "sda" {
		t.Errorf("unexpected disks: %v", got)
	}
}

func TestStartInstallation_Created(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		if req["payload"] != "edge-os" || req["disk"] != "sda" {
			t.Errorf("unexpected body: %v", req)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Installation{ID: "1", Status: "pending"})
	}))
	defer srv.Close()

	inst, err := NewClient(srv.URL).StartInstallation("edge-os", "sda")
	if err != nil {
		t.Fatalf("StartInstallation error: %v", err)
	}
	if inst.ID != "1" || inst.Status != "pending" {
		t.Errorf("unexpected installation: %+v", inst)
	}
}

func TestStartInstallation_ConflictError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"error":"disk already has an active installation"}`)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).StartInstallation("edge-os", "sda")
	if err == nil {
		t.Fatal("expected error for 409 conflict")
	}
}

func TestGetLog_ReturnsLinesAndOffset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "5" {
			t.Errorf("expected offset=5, got %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(LogResponse{Lines: []string{"line1", "line2"}, Offset: 7})
	}))
	defer srv.Close()

	lines, offset, err := NewClient(srv.URL).GetLog("inst-1", 5)
	if err != nil {
		t.Fatalf("GetLog error: %v", err)
	}
	if len(lines) != 2 || lines[0] != "line1" {
		t.Errorf("unexpected lines: %v", lines)
	}
	if offset != 7 {
		t.Errorf("offset = %d, want 7", offset)
	}
}

func TestReboot_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/system/reboot" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).Reboot(); err != nil {
		t.Fatalf("Reboot error: %v", err)
	}
}

func TestReboot_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "unavailable")
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).Reboot(); err == nil {
		t.Fatal("expected error for 503 response")
	}
}

func TestReadError_JSONFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"payload not found"}`)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).GetSystem()
	if err == nil || err.Error() != "payload not found" {
		t.Errorf("expected 'payload not found' error, got %v", err)
	}
}

func TestReadError_PlainTextFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "bad gateway")
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).GetSystem()
	if err == nil {
		t.Fatal("expected error")
	}
}
