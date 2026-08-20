package installer

import (
	"encoding/json"
	"testing"
)

func TestPayloadManifestUsesCamelCaseJSON(t *testing.T) {
	raw, err := json.Marshal(PayloadManifest{
		Name:     "edge-os",
		EFILabel: "TelemetryOS",
		Partitions: []PayloadPartition{{
			Name:       "root",
			MountPoint: "/",
		}},
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if body["efiLabel"] != "TelemetryOS" {
		t.Fatalf("efiLabel = %#v, want TelemetryOS", body["efiLabel"])
	}
	if _, ok := body["efi_label"]; ok {
		t.Fatalf("retired efi_label leaked into %#v", body)
	}
	partitions := body["partitions"].([]any)
	partition := partitions[0].(map[string]any)
	if partition["mountPoint"] != "/" {
		t.Fatalf("mountPoint = %#v, want /", partition["mountPoint"])
	}
	if _, ok := partition["mount_point"]; ok {
		t.Fatalf("retired mount_point leaked into %#v", partition)
	}
}

func TestPayloadManifestIgnoresRetiredSnakeCaseJSON(t *testing.T) {
	var manifest PayloadManifest
	if err := json.Unmarshal([]byte(`{
		"name":"edge-os",
		"efi_label":"retired",
		"partitions":[{"name":"root","mount_point":"/retired"}]
	}`), &manifest); err != nil {
		t.Fatalf("decode retired manifest: %v", err)
	}
	if manifest.EFILabel != "" {
		t.Fatalf("retired efi_label populated EFILabel: %q", manifest.EFILabel)
	}
	if got := manifest.Partitions[0].MountPoint; got != "" {
		t.Fatalf("retired mount_point populated MountPoint: %q", got)
	}
}
