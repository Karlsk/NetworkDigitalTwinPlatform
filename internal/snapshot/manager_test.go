package snapshot

import (
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
)

func TestSnapshotMetaFields(t *testing.T) {
	createdAt := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	meta := SnapshotMeta{
		Name:      "snap-20240615",
		CreatedAt: createdAt,
		NodeCount: 42,
		RelCount:  18,
		FilePath:  "/data/snapshots/snap-20240615.yaml",
	}

	if meta.Name != "snap-20240615" {
		t.Errorf("Name = %q, want %q", meta.Name, "snap-20240615")
	}
	if !meta.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v", meta.CreatedAt, createdAt)
	}
	if meta.NodeCount != 42 {
		t.Errorf("NodeCount = %d, want 42", meta.NodeCount)
	}
	if meta.RelCount != 18 {
		t.Errorf("RelCount = %d, want 18", meta.RelCount)
	}
	if meta.FilePath != "/data/snapshots/snap-20240615.yaml" {
		t.Errorf("FilePath = %q, want %q", meta.FilePath, "/data/snapshots/snap-20240615.yaml")
	}
}

func TestSnapshotDiffFields(t *testing.T) {
	diff := SnapshotDiff{
		AddedNodes: []assembler.Node{
			{Label: "Device", URI: "device:NEW001", Props: map[string]any{"hostname": "new-router"}},
		},
		RemovedNodes: []assembler.Node{
			{Label: "Device", URI: "device:OLD001"},
		},
		AddedRels: []assembler.Relation{
			{Type: "HAS_INTERFACE", From: "device:NEW001", To: "iface:NEW001_eth0"},
		},
		RemovedRels: []assembler.Relation{
			{Type: "HAS_INTERFACE", From: "device:OLD001", To: "iface:OLD001_eth0"},
		},
	}

	if len(diff.AddedNodes) != 1 {
		t.Fatalf("AddedNodes count = %d, want 1", len(diff.AddedNodes))
	}
	if diff.AddedNodes[0].URI != "device:NEW001" {
		t.Errorf("AddedNodes[0].URI = %q, want %q", diff.AddedNodes[0].URI, "device:NEW001")
	}
	if len(diff.RemovedNodes) != 1 {
		t.Fatalf("RemovedNodes count = %d, want 1", len(diff.RemovedNodes))
	}
	if diff.RemovedNodes[0].URI != "device:OLD001" {
		t.Errorf("RemovedNodes[0].URI = %q, want %q", diff.RemovedNodes[0].URI, "device:OLD001")
	}
	if len(diff.AddedRels) != 1 {
		t.Fatalf("AddedRels count = %d, want 1", len(diff.AddedRels))
	}
	if diff.AddedRels[0].Type != "HAS_INTERFACE" {
		t.Errorf("AddedRels[0].Type = %q, want %q", diff.AddedRels[0].Type, "HAS_INTERFACE")
	}
	if len(diff.RemovedRels) != 1 {
		t.Fatalf("RemovedRels count = %d, want 1", len(diff.RemovedRels))
	}
	if diff.RemovedRels[0].From != "device:OLD001" {
		t.Errorf("RemovedRels[0].From = %q, want %q", diff.RemovedRels[0].From, "device:OLD001")
	}
}

func TestSnapshotDiffEmpty(t *testing.T) {
	diff := SnapshotDiff{}
	if diff.AddedNodes != nil {
		t.Errorf("expected nil AddedNodes for zero-value, got %v", diff.AddedNodes)
	}
	if diff.RemovedNodes != nil {
		t.Errorf("expected nil RemovedNodes for zero-value, got %v", diff.RemovedNodes)
	}
	if diff.AddedRels != nil {
		t.Errorf("expected nil AddedRels for zero-value, got %v", diff.AddedRels)
	}
	if diff.RemovedRels != nil {
		t.Errorf("expected nil RemovedRels for zero-value, got %v", diff.RemovedRels)
	}
}
