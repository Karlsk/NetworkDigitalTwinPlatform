package engine

import (
	"context"
	"errors"
	"testing"
)

// ---------------------------------------------------------------------------
// ImpactEngine
// ---------------------------------------------------------------------------

func TestImpactEngine_New(t *testing.T) {
	e := NewImpactEngine()
	if e == nil {
		t.Fatal("NewImpactEngine() returned nil")
	}
}

func TestImpactEngine_Analyze_ReturnsNotImplemented(t *testing.T) {
	e := NewImpactEngine()
	result, err := e.Analyze(context.Background(), "device:A", 2)
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Analyze() error = %v, want ErrNotImplemented", err)
	}
	if result != nil {
		t.Errorf("Analyze() result = %v, want nil", result)
	}
}

func TestImpactEngine_Analyze_ContextCancelled(t *testing.T) {
	e := NewImpactEngine()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := e.Analyze(ctx, "device:A", 1)
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Analyze() error = %v, want ErrNotImplemented", err)
	}
	if result != nil {
		t.Errorf("Analyze() result = %v, want nil", result)
	}
}

// ---------------------------------------------------------------------------
// RCAEngine
// ---------------------------------------------------------------------------

func TestRCAEngine_New(t *testing.T) {
	e := NewRCAEngine()
	if e == nil {
		t.Fatal("NewRCAEngine() returned nil")
	}
}

func TestRCAEngine_Analyze_ReturnsNotImplemented(t *testing.T) {
	e := NewRCAEngine()
	result, err := e.Analyze(context.Background(), []Alarm{
		{SourceURI: "device:A", Type: "link_down", Severity: "critical", Message: "link down"},
	})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Analyze() error = %v, want ErrNotImplemented", err)
	}
	if result != nil {
		t.Errorf("Analyze() result = %v, want nil", result)
	}
}

func TestRCAEngine_Analyze_EmptyAlarms(t *testing.T) {
	e := NewRCAEngine()
	result, err := e.Analyze(context.Background(), nil)
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Analyze() error = %v, want ErrNotImplemented", err)
	}
	if result != nil {
		t.Errorf("Analyze() result = %v, want nil", result)
	}
}

// ---------------------------------------------------------------------------
// SimulationEngine
// ---------------------------------------------------------------------------

func TestSimulationEngine_New(t *testing.T) {
	e := NewSimulationEngine()
	if e == nil {
		t.Fatal("NewSimulationEngine() returned nil")
	}
}

func TestSimulationEngine_Simulate_ReturnsNotImplemented(t *testing.T) {
	e := NewSimulationEngine()
	result, err := e.Simulate(context.Background(), OperationPlan{
		TargetURI: "device:A",
		Action:    "disable",
		Params:    map[string]string{"force": "true"},
	})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Simulate() error = %v, want ErrNotImplemented", err)
	}
	if result != nil {
		t.Errorf("Simulate() result = %v, want nil", result)
	}
}

func TestSimulationEngine_Simulate_EmptyPlan(t *testing.T) {
	e := NewSimulationEngine()
	result, err := e.Simulate(context.Background(), OperationPlan{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Simulate() error = %v, want ErrNotImplemented", err)
	}
	if result != nil {
		t.Errorf("Simulate() result = %v, want nil", result)
	}
}

// ---------------------------------------------------------------------------
// ErrNotImplemented sentinel
// ---------------------------------------------------------------------------

func TestErrNotImplemented(t *testing.T) {
	if ErrNotImplemented.Error() != "engine: not implemented" {
		t.Errorf("ErrNotImplemented.Error() = %q", ErrNotImplemented.Error())
	}
}

// ---------------------------------------------------------------------------
// Struct field coverage
// ---------------------------------------------------------------------------

func TestStructFieldCoverage(t *testing.T) {
	// ImpactResult
	ir := ImpactResult{
		SourceNode:    "device:A",
		AffectedNodes: []AffectedNode{{URI: "device:B", Label: "Device", Distance: 1}},
		AffectedRels:  []AffectedRel{{Type: "CONNECTS_TO", From: "device:A", To: "device:B"}},
		MaxDepth:      2,
	}
	if ir.SourceNode != "device:A" || ir.MaxDepth != 2 {
		t.Error("ImpactResult fields incorrect")
	}
	if len(ir.AffectedNodes) != 1 || ir.AffectedNodes[0].Distance != 1 {
		t.Error("AffectedNodes incorrect")
	}
	if len(ir.AffectedRels) != 1 || ir.AffectedRels[0].Type != "CONNECTS_TO" {
		t.Error("AffectedRels incorrect")
	}

	// RCAResult
	rr := RCAResult{
		RootCause:     "device:A",
		Confidence:    0.95,
		RelatedAlarms: []string{"alarm-1"},
	}
	if rr.RootCause != "device:A" || rr.Confidence != 0.95 {
		t.Error("RCAResult fields incorrect")
	}

	// Alarm
	a := Alarm{SourceURI: "device:A", Type: "down", Severity: "critical", Message: "down"}
	if a.SourceURI != "device:A" {
		t.Error("Alarm fields incorrect")
	}

	// SimulationResult
	sr := SimulationResult{
		ImpactedNodes:  []string{"device:B"},
		RiskScore:      0.8,
		Recommendation: "do not proceed",
	}
	if sr.RiskScore != 0.8 {
		t.Error("SimulationResult fields incorrect")
	}

	// OperationPlan
	op := OperationPlan{
		TargetURI: "device:A",
		Action:    "disable",
		Params:    map[string]string{"force": "true"},
	}
	if op.Action != "disable" {
		t.Error("OperationPlan fields incorrect")
	}
}
