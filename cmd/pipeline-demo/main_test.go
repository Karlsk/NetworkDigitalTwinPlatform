package main

import (
	"testing"
)

// ── propsPreview ──

func TestPropsPreview_Empty(t *testing.T) {
	result := propsPreview(map[string]any{})
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestPropsPreview_LessThan5(t *testing.T) {
	props := map[string]any{"a": 1, "b": 2, "c": 3}
	result := propsPreview(props)
	if len(result) != 3 {
		t.Errorf("expected 3 keys, got %d", len(result))
	}
	if result["a"] != 1 || result["b"] != 2 || result["c"] != 3 {
		t.Errorf("unexpected values: %v", result)
	}
}

func TestPropsPreview_Exactly5(t *testing.T) {
	props := map[string]any{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	result := propsPreview(props)
	if len(result) != 5 {
		t.Errorf("expected 5 keys, got %d", len(result))
	}
	// no "..." key when exactly 5
	if _, ok := result["..."]; ok {
		t.Error("should not have '...' key when exactly 5 props")
	}
}

func TestPropsPreview_MoreThan5(t *testing.T) {
	props := map[string]any{
		"a": 1, "b": 2, "c": 3, "d": 4, "e": 5,
		"f": 6, "g": 7, "h": 8,
	}
	result := propsPreview(props)
	// should have 5 props + "..." key = 6 entries
	if len(result) != 6 {
		t.Errorf("expected 6 keys (5 + '...'), got %d", len(result))
	}
	more, ok := result["..."]
	if !ok {
		t.Fatal("expected '...' key for overflow")
	}
	if more != "(3 more)" {
		t.Errorf("'...' = %v, want '(3 more)'", more)
	}
}

// ── toInt ──

func TestToInt_Empty(t *testing.T) {
	if got := toInt(nil, "count"); got != 0 {
		t.Errorf("nil rows: got %d, want 0", got)
	}
	if got := toInt([]map[string]any{}, "count"); got != 0 {
		t.Errorf("empty rows: got %d, want 0", got)
	}
}

func TestToInt_Int64(t *testing.T) {
	rows := []map[string]any{{"count": int64(42)}}
	if got := toInt(rows, "count"); got != 42 {
		t.Errorf("got %d, want 42", got)
	}
}

func TestToInt_Int(t *testing.T) {
	rows := []map[string]any{{"count": 99}}
	if got := toInt(rows, "count"); got != 99 {
		t.Errorf("got %d, want 99", got)
	}
}

func TestToInt_Float64(t *testing.T) {
	rows := []map[string]any{{"count": float64(7)}}
	if got := toInt(rows, "count"); got != 7 {
		t.Errorf("got %d, want 7", got)
	}
}

func TestToInt_UnknownType(t *testing.T) {
	rows := []map[string]any{{"count": "not a number"}}
	if got := toInt(rows, "count"); got != 0 {
		t.Errorf("got %d, want 0 for unknown type", got)
	}
}

func TestToInt_MissingKey(t *testing.T) {
	rows := []map[string]any{{"other": 1}}
	if got := toInt(rows, "count"); got != 0 {
		t.Errorf("got %d, want 0 for missing key", got)
	}
}

func TestToInt_NilValue(t *testing.T) {
	rows := []map[string]any{{"count": nil}}
	if got := toInt(rows, "count"); got != 0 {
		t.Errorf("got %d, want 0 for nil value", got)
	}
}

// ── section / checkpoint / printJSON ──

func TestSection(t *testing.T) {
	section("Test Section Title") // should not panic
}

func TestCheckpoint_Pass(t *testing.T) {
	checkpoint("Neo4j connection", true) // should not panic
}

func TestCheckpoint_Fail(t *testing.T) {
	checkpoint("Schema validation", false) // should not panic
}

func TestPrintJSON_Map(t *testing.T) {
	printJSON("  ", map[string]any{"name": "router-01", "ip": "10.0.0.1"}) // should not panic
}

func TestPrintJSON_Slice(t *testing.T) {
	printJSON(">> ", []int{1, 2, 3}) // should not panic
}

func TestPrintJSON_Nil(t *testing.T) {
	printJSON("", nil) // should not panic
}
