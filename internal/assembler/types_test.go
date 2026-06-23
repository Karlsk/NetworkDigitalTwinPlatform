package assembler

import "testing"

func TestNodeFields(t *testing.T) {
	tests := []struct {
		name  string
		node  Node
		label string
		uri   string
		props int
	}{
		{
			name: "Device node with properties",
			node: Node{
				Label: "Device",
				URI:   "device:SN001",
				Props: map[string]any{
					"hostname": "router-01",
					"vendor":   "Huawei",
					"status":   "Up",
				},
			},
			label: "Device",
			uri:   "device:SN001",
			props: 3,
		},
		{
			name:  "empty node",
			node:  Node{},
			label: "",
			uri:   "",
			props: 0,
		},
		{
			name: "node with nil props",
			node: Node{
				Label: "Interface",
				URI:   "iface:SN001_eth0",
				Props: nil,
			},
			label: "Interface",
			uri:   "iface:SN001_eth0",
			props: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.node.Label != tt.label {
				t.Errorf("Label = %q, want %q", tt.node.Label, tt.label)
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

// TestNodePropsMapIsReference verifies that Props is a map (reference type),
// so assigning the same map to two Nodes shares the underlying data.
func TestNodePropsMapIsReference(t *testing.T) {
	shared := map[string]any{"status": "Up"}
	n1 := Node{Label: "Device", URI: "device:A", Props: shared}
	n2 := Node{Label: "Device", URI: "device:B", Props: shared}

	// Modify through n1
	n1.Props["status"] = "Down"

	// n2 should see the change (shared map)
	if n2.Props["status"] != "Down" {
		t.Error("expected Props map to be shared reference, but n2 was unaffected")
	}
}
