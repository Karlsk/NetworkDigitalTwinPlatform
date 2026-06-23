package service

import (
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
)

func TestSyncResultFields(t *testing.T) {
	sr := SyncResult{
		NodesCreated:       10,
		RelationsCreated:   5,
		OrphanEdgesSkipped: 2,
		Warnings: []assembler.ValidationWarning{
			{Type: "orphan_edge", Detail: "HAS_INTERFACE: device:A → iface:missing"},
		},
		Duration: 3 * time.Second,
	}

	if sr.NodesCreated != 10 {
		t.Errorf("NodesCreated = %d, want 10", sr.NodesCreated)
	}
	if sr.RelationsCreated != 5 {
		t.Errorf("RelationsCreated = %d, want 5", sr.RelationsCreated)
	}
	if sr.OrphanEdgesSkipped != 2 {
		t.Errorf("OrphanEdgesSkipped = %d, want 2", sr.OrphanEdgesSkipped)
	}
	if len(sr.Warnings) != 1 {
		t.Fatalf("Warnings count = %d, want 1", len(sr.Warnings))
	}
	if sr.Warnings[0].Type != "orphan_edge" {
		t.Errorf("Warnings[0].Type = %q, want %q", sr.Warnings[0].Type, "orphan_edge")
	}
	if sr.Duration != 3*time.Second {
		t.Errorf("Duration = %v, want 3s", sr.Duration)
	}
}

func TestSyncEventActions(t *testing.T) {
	actions := []string{"update", "delete", "delete_relation"}
	for _, action := range actions {
		e := SyncEvent{Action: action}
		if e.Action != action {
			t.Errorf("Action = %q, want %q", e.Action, action)
		}
	}
}

func TestSyncEventUpdateData(t *testing.T) {
	e := SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Connector:  "mock-netbox",
		Data: []map[string]any{
			{"hostname": "router-01", "vendor": "Huawei"},
			{"hostname": "router-02", "vendor": "Cisco"},
		},
	}

	if e.EntityType != "Device" {
		t.Errorf("EntityType = %q, want %q", e.EntityType, "Device")
	}
	if e.Connector != "mock-netbox" {
		t.Errorf("Connector = %q, want %q", e.Connector, "mock-netbox")
	}
	if len(e.Data) != 2 {
		t.Fatalf("Data count = %d, want 2", len(e.Data))
	}
	if e.Data[0]["hostname"] != "router-01" {
		t.Errorf("Data[0][hostname] = %v, want %q", e.Data[0]["hostname"], "router-01")
	}
}

func TestSyncEventDeleteURIs(t *testing.T) {
	e := SyncEvent{
		Action: "delete",
		URIs:   []string{"device:SN001", "device:SN002"},
	}

	if len(e.URIs) != 2 {
		t.Fatalf("URIs count = %d, want 2", len(e.URIs))
	}
	if e.URIs[0] != "device:SN001" {
		t.Errorf("URIs[0] = %q, want %q", e.URIs[0], "device:SN001")
	}
}

func TestSyncEventDeleteRelations(t *testing.T) {
	e := SyncEvent{
		Action: "delete_relation",
		Relations: []assembler.Relation{
			{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
		},
	}

	if len(e.Relations) != 1 {
		t.Fatalf("Relations count = %d, want 1", len(e.Relations))
	}
	if e.Relations[0].Type != "HAS_INTERFACE" {
		t.Errorf("Relations[0].Type = %q, want %q", e.Relations[0].Type, "HAS_INTERFACE")
	}
}
