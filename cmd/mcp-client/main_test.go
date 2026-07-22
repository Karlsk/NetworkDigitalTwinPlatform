package main

import (
	"errors"
	"testing"
)

// ── extractStructured ──

func TestExtractStructured_MapToStruct(t *testing.T) {
	type sample struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	raw := map[string]any{"name": "Alice", "age": float64(30)}
	var dst sample
	if err := extractStructured(raw, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.Name != "Alice" {
		t.Errorf("Name = %q, want %q", dst.Name, "Alice")
	}
	if dst.Age != 30 {
		t.Errorf("Age = %d, want %d", dst.Age, 30)
	}
}

func TestExtractStructured_SliceToSlice(t *testing.T) {
	raw := []any{float64(1), float64(2), float64(3)}
	var dst []int
	if err := extractStructured(raw, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dst) != 3 || dst[0] != 1 || dst[1] != 2 || dst[2] != 3 {
		t.Errorf("got %v, want [1 2 3]", dst)
	}
}

func TestExtractStructured_InvalidType(t *testing.T) {
	// chan cannot be marshaled to JSON
	var dst map[string]any
	err := extractStructured(make(chan int), &dst)
	if err == nil {
		t.Error("expected error for unmarshalable type")
	}
}

func TestExtractStructured_NilRaw(t *testing.T) {
	var dst map[string]any
	err := extractStructured(nil, &dst)
	if err != nil {
		t.Fatalf("nil should unmarshal to nil map: %v", err)
	}
}

func TestExtractStructured_EmptyMap(t *testing.T) {
	raw := map[string]any{}
	var dst map[string]any
	if err := extractStructured(raw, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dst) != 0 {
		t.Errorf("expected empty map, got %v", dst)
	}
}

// ── assertGTE ──

func TestAssertGTE_Equal(t *testing.T) {
	if err := assertGTE("count", 5, 5); err != nil {
		t.Errorf("equal values should pass: %v", err)
	}
}

func TestAssertGTE_Greater(t *testing.T) {
	if err := assertGTE("count", 10, 5); err != nil {
		t.Errorf("greater value should pass: %v", err)
	}
}

func TestAssertGTE_Less(t *testing.T) {
	err := assertGTE("count", 3, 5)
	if err == nil {
		t.Error("less value should fail")
	}
	if err != nil && err.Error() != "count: got 3, want >= 5" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAssertGTE_Zero(t *testing.T) {
	if err := assertGTE("count", 0, 0); err != nil {
		t.Errorf("zero values should pass: %v", err)
	}
}

func TestAssertGTE_Negative(t *testing.T) {
	err := assertGTE("count", -1, 0)
	if err == nil {
		t.Error("negative value should fail when want is 0")
	}
}

// ── printResult ──

func TestPrintResult_PASS(t *testing.T) {
	printResult(1, 5, "list_tools", "PASS", "4 tools found") // should not panic
}

func TestPrintResult_FAIL(t *testing.T) {
	printResult(2, 5, "sync_data", "FAIL", "timeout") // should not panic
}

func TestPrintResult_SKIP(t *testing.T) {
	printResult(3, 5, "restore", "SKIP", "not supported") // should not panic
}

func TestPrintResult_Unknown(t *testing.T) {
	printResult(4, 5, "query", "UNKNOWN", "") // should not panic, unknown status gets no color
}

// ── skipError ──

func TestSkipError(t *testing.T) {
	e := &skipError{reason: "not available in V1"}
	if e.Error() != "not available in V1" {
		t.Errorf("got %q", e.Error())
	}
}

func TestErrSkip(t *testing.T) {
	err := errSkip("controller not running")
	var se *skipError
	if !errors.As(err, &se) {
		t.Fatal("errSkip should return *skipError")
	}
	if se.reason != "controller not running" {
		t.Errorf("reason = %q", se.reason)
	}
}

// ── buildTests (structure validation) ──

func TestBuildTests_ReturnsNonEmpty(t *testing.T) {
	tests := buildTests()
	if len(tests) == 0 {
		t.Fatal("buildTests returned empty slice")
	}
	// Check each test has name and desc
	for i, tc := range tests {
		if tc.name == "" {
			t.Errorf("test[%d].name is empty", i)
		}
		if tc.desc == "" {
			t.Errorf("test[%d].desc is empty", i)
		}
		if tc.run == nil {
			t.Errorf("test[%d].run is nil (name=%s)", i, tc.name)
		}
	}
}
