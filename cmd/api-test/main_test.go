package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ── truncate ──

func TestTruncate_Short(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestTruncate_Exact(t *testing.T) {
	if got := truncate("hello", 5); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestTruncate_Long(t *testing.T) {
	if got := truncate("hello world", 8); got != "hello..." {
		t.Errorf("got %q, want %q", got, "hello...")
	}
}

func TestTruncate_MinMax(t *testing.T) {
	if got := truncate("abcdefghij", 4); got != "a..." {
		t.Errorf("got %q, want %q", got, "a...")
	}
}

// ── countStr ──

func TestCountStr_MapSlice(t *testing.T) {
	items := []map[string]any{{"a": 1}, {"b": 2}}
	if got := countStr(items); got != "2 items" {
		t.Errorf("got %q, want %q", got, "2 items")
	}
}

func TestCountStr_MetricsResult(t *testing.T) {
	m := &connector.MetricsResult{Metrics: []connector.MetricSeries{{Name: "cpu"}}}
	if got := countStr(m); got != "1 series" {
		t.Errorf("got %q, want %q", got, "1 series")
	}
}

func TestCountStr_LogResult(t *testing.T) {
	l := &connector.LogResult{Logs: []map[string]any{{"msg": "a"}}, TotalCount: 100}
	if got := countStr(l); got != "1 logs (total=100)" {
		t.Errorf("got %q, want %q", got, "1 logs (total=100)")
	}
}

func TestCountStr_TopologyLiveResult(t *testing.T) {
	topo := &connector.TopologyLiveResult{
		Nodes: []map[string]any{{"n": 1}, {"n": 2}},
		Links: []map[string]any{{"l": 1}},
	}
	if got := countStr(topo); got != "2 nodes, 1 links" {
		t.Errorf("got %q, want %q", got, "2 nodes, 1 links")
	}
}

func TestCountStr_Map(t *testing.T) {
	m := map[string]any{"a": 1, "b": 2, "c": 3}
	if got := countStr(m); got != "3 fields" {
		t.Errorf("got %q, want %q", got, "3 fields")
	}
}

func TestCountStr_String(t *testing.T) {
	if got := countStr("hello"); got != "5 chars" {
		t.Errorf("got %q, want %q", got, "5 chars")
	}
}

func TestCountStr_Default(t *testing.T) {
	if got := countStr(42); got != "OK" {
		t.Errorf("got %q, want %q", got, "OK")
	}
}

// ── formatValue ──

func TestFormatValue_String(t *testing.T) {
	if got := formatValue("hello"); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestFormatValue_Float64Int(t *testing.T) {
	if got := formatValue(float64(42)); got != "42" {
		t.Errorf("got %q, want %q", got, "42")
	}
}

func TestFormatValue_Float64Decimal(t *testing.T) {
	if got := formatValue(3.14); got != "3.14" {
		t.Errorf("got %q, want %q", got, "3.14")
	}
}

func TestFormatValue_Bool(t *testing.T) {
	if got := formatValue(true); got != "true" {
		t.Errorf("got %q, want %q", got, "true")
	}
}

func TestFormatValue_Nil(t *testing.T) {
	if got := formatValue(nil); got != "<nil>" {
		t.Errorf("got %q, want %q", got, "<nil>")
	}
}

func TestFormatValue_Map(t *testing.T) {
	m := map[string]any{"key": "val"}
	got := formatValue(m)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("formatValue returned non-JSON: %v", err)
	}
	if parsed["key"] != "val" {
		t.Errorf("unexpected parsed value: %v", parsed["key"])
	}
}

func TestFormatValue_Slice(t *testing.T) {
	s := []int{1, 2, 3}
	got := formatValue(s)
	if got != "[1,2,3]" {
		t.Errorf("got %q, want %q", got, "[1,2,3]")
	}
}

// ── printProps (output test) ──

func TestPrintProps_Empty(t *testing.T) {
	// 空 map 不应 panic
	printProps(map[string]any{})
}

func TestPrintProps_WithValues(t *testing.T) {
	printProps(map[string]any{
		"name":   "router-01",
		"status": "active",
		"count":  float64(42),
		"flag":   true,
		"empty":  nil,
	})
}

// ── testRunner ──

func TestNewTestRunner(t *testing.T) {
	r := newTestRunner()
	if r == nil {
		t.Fatal("newTestRunner returned nil")
	}
	if len(r.results) != 0 {
		t.Errorf("expected 0 results, got %d", len(r.results))
	}
}

func TestTestRunner_Run_Pass(t *testing.T) {
	r := newTestRunner()
	r.run("test-pass", func() (string, error) {
		return "ok", nil
	})
	if len(r.results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(r.results))
	}
	if r.results[0].status != "PASS" {
		t.Errorf("status = %q, want PASS", r.results[0].status)
	}
	if r.results[0].detail != "ok" {
		t.Errorf("detail = %q, want ok", r.results[0].detail)
	}
	if r.results[0].name != "test-pass" {
		t.Errorf("name = %q, want test-pass", r.results[0].name)
	}
}

func TestTestRunner_Run_Fail(t *testing.T) {
	r := newTestRunner()
	r.run("test-fail", func() (string, error) {
		return "", fmt.Errorf("connection refused")
	})
	if r.results[0].status != "FAIL" {
		t.Errorf("status = %q, want FAIL", r.results[0].status)
	}
	if r.results[0].detail != "connection refused" {
		t.Errorf("detail = %q", r.results[0].detail)
	}
}

func TestTestRunner_Run_NotImplemented(t *testing.T) {
	r := newTestRunner()
	r.run("test-skip", func() (string, error) {
		return "", fmt.Errorf("not implemented in V1")
	})
	if r.results[0].status != "SKIP" {
		t.Errorf("status = %q, want SKIP", r.results[0].status)
	}
}

func TestTestRunner_Section(t *testing.T) {
	r := newTestRunner()
	r.section("Test Section") // should not panic
}

func TestTestRunner_Summary_AllPass(t *testing.T) {
	r := newTestRunner()
	r.run("a", func() (string, error) { return "ok", nil })
	r.run("b", func() (string, error) { return "ok", nil })
	r.summary() // should not panic
}

func TestTestRunner_Summary_WithFailures(t *testing.T) {
	r := newTestRunner()
	r.run("pass1", func() (string, error) { return "ok", nil })
	r.run("fail1", func() (string, error) { return "", fmt.Errorf("oops") })
	r.run("skip1", func() (string, error) { return "", fmt.Errorf("not implemented") })
	r.summary() // should not panic
}

func TestTestRunner_MultipleRuns(t *testing.T) {
	r := newTestRunner()
	for i := 0; i < 5; i++ {
		r.run(fmt.Sprintf("test-%d", i), func() (string, error) {
			return "done", nil
		})
	}
	if len(r.results) != 5 {
		t.Errorf("expected 5 results, got %d", len(r.results))
	}
}
