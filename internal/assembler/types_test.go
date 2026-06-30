package assembler

import "testing"

func TestNodeFields(t *testing.T) {
	tests := []struct {
		name   string
		node   Node
		labels []string
		uri    string
		props  int
	}{
		{
			name: "Device node with properties",
			node: Node{
				Labels: []string{"Device"},
				URI:    "device:SN001",
				Props: map[string]any{
					"hostname": "router-01",
					"vendor":   "Huawei",
					"status":   "Up",
				},
			},
			labels: []string{"Device"},
			uri:    "device:SN001",
			props:  3,
		},
		{
			name:   "empty node",
			node:   Node{},
			labels: nil,
			uri:    "",
			props:  0,
		},
		{
			name: "node with nil props",
			node: Node{
				Labels: []string{"Interface"},
				URI:    "iface:SN001_eth0",
				Props:  nil,
			},
			labels: []string{"Interface"},
			uri:    "iface:SN001_eth0",
			props:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.node.Labels) != len(tt.labels) {
				t.Errorf("Labels = %v, want %v", tt.node.Labels, tt.labels)
			} else {
				for i := range tt.labels {
					if tt.node.Labels[i] != tt.labels[i] {
						t.Errorf("Labels[%d] = %q, want %q", i, tt.node.Labels[i], tt.labels[i])
					}
				}
			}
			if tt.node.URI != tt.uri {
				t.Errorf("URI = %q, want %q", tt.node.URI, tt.uri)
			}
			if len(tt.node.Props) != tt.props {
				t.Errorf("Props count = %d, want %d", len(tt.node.Props), tt.props)
			}
		})
	}
}

func TestRelationFields(t *testing.T) {
	tests := []struct {
		name     string
		relation Relation
		relType  string
		from     string
		to       string
		props    int
	}{
		{
			name: "HAS_INTERFACE relation",
			relation: Relation{
				Type:  "HAS_INTERFACE",
				From:  "device:SN001",
				To:    "iface:SN001_eth0",
				Props: nil,
			},
			relType: "HAS_INTERFACE",
			from:    "device:SN001",
			to:      "iface:SN001_eth0",
			props:   0,
		},
		{
			name: "RUNS_ON relation with props",
			relation: Relation{
				Type: "RUNS_ON",
				From: "isis:100",
				To:   "iface:SN001_eth0",
				Props: map[string]any{
					"cost": 10,
				},
			},
			relType: "RUNS_ON",
			from:    "isis:100",
			to:      "iface:SN001_eth0",
			props:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.relation.Type != tt.relType {
				t.Errorf("Type = %q, want %q", tt.relation.Type, tt.relType)
			}
			if tt.relation.From != tt.from {
				t.Errorf("From = %q, want %q", tt.relation.From, tt.from)
			}
			if tt.relation.To != tt.to {
				t.Errorf("To = %q, want %q", tt.relation.To, tt.to)
			}
			if len(tt.relation.Props) != tt.props {
				t.Errorf("Props count = %d, want %d", len(tt.relation.Props), tt.props)
			}
		})
	}
}

func TestGraphModelFields(t *testing.T) {
	gm := GraphModel{
		Nodes: []Node{
			{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "router-01"}},
			{Labels: []string{"Interface"}, URI: "iface:SN001_eth0"},
		},
		Relations: []Relation{
			{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
		},
	}

	if len(gm.Nodes) != 2 {
		t.Errorf("Nodes count = %d, want 2", len(gm.Nodes))
	}
	if len(gm.Relations) != 1 {
		t.Errorf("Relations count = %d, want 1", len(gm.Relations))
	}
	if gm.Nodes[0].MostSpecificLabel() != "Device" {
		t.Errorf("Nodes[0].MostSpecificLabel() = %q, want %q", gm.Nodes[0].MostSpecificLabel(), "Device")
	}
	if gm.Relations[0].Type != "HAS_INTERFACE" {
		t.Errorf("Relations[0].Type = %q, want %q", gm.Relations[0].Type, "HAS_INTERFACE")
	}
}

func TestGraphModelEmpty(t *testing.T) {
	gm := GraphModel{}
	if gm.Nodes != nil {
		t.Errorf("expected nil Nodes for zero-value, got %v", gm.Nodes)
	}
	if gm.Relations != nil {
		t.Errorf("expected nil Relations for zero-value, got %v", gm.Relations)
	}
}

func TestValidationWarningFields(t *testing.T) {
	w := ValidationWarning{
		Type:   "orphan_edge",
		Detail: "HAS_INTERFACE: device:SN12345 → iface:SN12345_GE1/0/2",
	}
	if w.Type != "orphan_edge" {
		t.Errorf("Type = %q, want %q", w.Type, "orphan_edge")
	}
	if w.Detail != "HAS_INTERFACE: device:SN12345 → iface:SN12345_GE1/0/2" {
		t.Errorf("Detail = %q, want full orphan edge detail", w.Detail)
	}
}

// TestNodePropsMapIsReference verifies that Props is a map (reference type),
// so assigning the same map to two Nodes shares the underlying data.
func TestNodePropsMapIsReference(t *testing.T) {
	shared := map[string]any{"status": "Up"}
	n1 := Node{Labels: []string{"Device"}, URI: "device:A", Props: shared}
	n2 := Node{Labels: []string{"Device"}, URI: "device:B", Props: shared}

	// Modify through n1
	n1.Props["status"] = "Down"

	// n2 should see the change (shared map)
	if n2.Props["status"] != "Down" {
		t.Error("expected Props map to be shared reference, but n2 was unaffected")
	}
}

func TestNewNode(t *testing.T) {
	n := NewNode("Device", "device:SN001", map[string]any{"hostname": "router-01"})

	if len(n.Labels) != 1 || n.Labels[0] != "Device" {
		t.Errorf("Labels = %v, want [\"Device\"]", n.Labels)
	}
	if n.URI != "device:SN001" {
		t.Errorf("URI = %q, want %q", n.URI, "device:SN001")
	}
	if n.Props["hostname"] != "router-01" {
		t.Errorf("Props[hostname] = %v, want %q", n.Props["hostname"], "router-01")
	}
}

func TestMostSpecificLabel(t *testing.T) {
	tests := []struct {
		name   string
		node   Node
		expect string
	}{
		{
			name:   "empty labels",
			node:   Node{},
			expect: "",
		},
		{
			name:   "single label",
			node:   Node{Labels: []string{"Device"}},
			expect: "Device",
		},
		{
			name:   "multi labels with inheritance",
			node:   Node{Labels: []string{"Resource", "Device"}},
			expect: "Device",
		},
		{
			name:   "three level inheritance",
			node:   Node{Labels: []string{"Base", "Resource", "Device"}},
			expect: "Device",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.node.MostSpecificLabel()
			if got != tt.expect {
				t.Errorf("MostSpecificLabel() = %q, want %q", got, tt.expect)
			}
		})
	}
}
