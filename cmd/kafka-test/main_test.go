package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/events"
)

// ── uniqueTopic ──

func TestUniqueTopic_Prefix(t *testing.T) {
	topic := uniqueTopic("test")
	if !strings.HasPrefix(topic, "test-") {
		t.Errorf("topic %q should have prefix 'test-'", topic)
	}
}

func TestUniqueTopic_Different(t *testing.T) {
	t1 := uniqueTopic("x")
	time.Sleep(time.Microsecond) // ensure different UnixNano
	t2 := uniqueTopic("x")
	if t1 == t2 {
		t.Errorf("two calls should produce different topics, got %q both", t1)
	}
}

// ── extractHost ──

func TestExtractHost_LookupError(t *testing.T) {
	msg := "dial tcp: lookup kafka: no such host"
	if got := extractHost(msg); got != "kafka" {
		t.Errorf("got %q, want %q", got, "kafka")
	}
}

func TestExtractHost_LookupWithPort(t *testing.T) {
	msg := "dial tcp: lookup broker1:9092 no such host"
	if got := extractHost(msg); got != "broker1" {
		t.Errorf("got %q, want %q", got, "broker1")
	}
}

func TestExtractHost_NoLookup(t *testing.T) {
	msg := "connection refused"
	if got := extractHost(msg); got != "unknown" {
		t.Errorf("got %q, want %q", got, "unknown")
	}
}

func TestExtractHost_EmptyString(t *testing.T) {
	if got := extractHost(""); got != "unknown" {
		t.Errorf("got %q, want %q", got, "unknown")
	}
}

// ── safeClose ──

type mockCloser struct {
	closed  bool
	panicOn bool
}

func (m *mockCloser) Close() error {
	m.closed = true
	if m.panicOn {
		panic("sarama bug: send on closed channel")
	}
	return nil
}

func TestSafeClose_Normal(t *testing.T) {
	c := &mockCloser{}
	safeClose(c)
	if !c.closed {
		t.Error("expected Close() to be called")
	}
}

func TestSafeClose_Panic(t *testing.T) {
	c := &mockCloser{panicOn: true}
	// should not panic
	safeClose(c)
	if !c.closed {
		t.Error("expected Close() to be called even if it panics")
	}
}

// ── makeTestEvents ──

func TestMakeTestEvents_Zero(t *testing.T) {
	evts := makeTestEvents(0)
	if len(evts) != 0 {
		t.Errorf("expected 0 events, got %d", len(evts))
	}
}

func TestMakeTestEvents_Count(t *testing.T) {
	evts := makeTestEvents(3)
	if len(evts) != 3 {
		t.Errorf("expected 3 events, got %d", len(evts))
	}
}

func TestMakeTestEvents_Fields(t *testing.T) {
	evts := makeTestEvents(1)
	e := evts[0]
	if e.Action != "update" {
		t.Errorf("action = %q, want %q", e.Action, "update")
	}
	if e.EntityType != "Device" {
		t.Errorf("entity_type = %q, want %q", e.EntityType, "Device")
	}
	if e.Connector != "netbox" {
		t.Errorf("connector = %q, want %q", e.Connector, "netbox")
	}
	if len(e.Data) != 2 {
		t.Errorf("data len = %d, want 2", len(e.Data))
	}
}

func TestMakeTestEvents_DataFormat(t *testing.T) {
	evts := makeTestEvents(2)
	// First event
	d0 := evts[0].Data[0]
	if d0["name"] != "PE-Router-01" {
		t.Errorf("first device name = %v, want PE-Router-01", d0["name"])
	}
	if d0["role"] != "edge" {
		t.Errorf("role = %v, want edge", d0["role"])
	}
	// Second event
	d1 := evts[1].Data[1]
	if d1["name"] != "PE-Switch-02" {
		t.Errorf("second device name = %v, want PE-Switch-02", d1["name"])
	}
}

// suppress unused import
var _ events.SyncEvent

// ── testRunner ──

func TestKafkaTestRunner_New(t *testing.T) {
	r := newTestRunner()
	if r == nil {
		t.Fatal("newTestRunner returned nil")
	}
	if len(r.results) != 0 {
		t.Errorf("expected 0 results, got %d", len(r.results))
	}
}

func TestKafkaTestRunner_Run_Pass(t *testing.T) {
	r := newTestRunner()
	r.run("kafka-pass", func() (string, func(), error) {
		return "ok", nil, nil
	})
	if len(r.results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(r.results))
	}
	if r.results[0].Status != "PASS" {
		t.Errorf("status = %q, want PASS", r.results[0].Status)
	}
	if r.results[0].Name != "kafka-pass" {
		t.Errorf("name = %q, want kafka-pass", r.results[0].Name)
	}
}

func TestKafkaTestRunner_Run_Fail(t *testing.T) {
	r := newTestRunner()
	r.run("kafka-fail", func() (string, func(), error) {
		return "", nil, fmt.Errorf("broker unreachable")
	})
	if r.results[0].Status != "FAIL" {
		t.Errorf("status = %q, want FAIL", r.results[0].Status)
	}
	if r.results[0].Detail != "broker unreachable" {
		t.Errorf("detail = %q", r.results[0].Detail)
	}
}

func TestKafkaTestRunner_Run_WithCleanup(t *testing.T) {
	r := newTestRunner()
	cleaned := false
	r.run("kafka-cleanup", func() (string, func(), error) {
		cleanup := func() { cleaned = true }
		return "ok", cleanup, nil
	})
	if !cleaned {
		t.Error("cleanup function should have been called")
	}
}

func TestKafkaTestRunner_Section(t *testing.T) {
	r := newTestRunner()
	r.section("Section 1") // should not panic
}

func TestKafkaTestRunner_Summary(t *testing.T) {
	r := newTestRunner()
	r.run("pass", func() (string, func(), error) { return "ok", nil, nil })
	r.run("fail", func() (string, func(), error) { return "", nil, fmt.Errorf("oops") })
	r.summary() // should not panic
}

func TestKafkaTestRunner_FlushResults_NoFile(t *testing.T) {
	r := newTestRunner()
	r.results = []testResult{{Name: "test", Status: "PASS"}}
	// resultsFile is empty by default, should return early
	r.flushResults() // should not panic
}

func TestKafkaTestRunner_FlushResults_WithFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "results.json")
	oldResultsFile := resultsFile
	resultsFile = tmpFile
	defer func() { resultsFile = oldResultsFile }()

	r := newTestRunner()
	r.results = []testResult{
		{Name: "test1", Status: "PASS", Detail: "ok", Elapsed: time.Millisecond},
	}
	r.flushResults()

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read results file: %v", err)
	}
	if !strings.Contains(string(data), "test1") {
		t.Error("results file should contain test1")
	}
}

func TestPrintSummaryFromResults_Empty(t *testing.T) {
	printSummaryFromResults(nil, time.Now()) // should not panic
}

func TestPrintSummaryFromResults_WithResults(t *testing.T) {
	results := []testResult{
		{Name: "a", Status: "PASS"},
		{Name: "b", Status: "FAIL", Detail: "error"},
	}
	printSummaryFromResults(results, time.Now().Add(-time.Second)) // should not panic
}

func TestFindProjectRoot(t *testing.T) {
	root := findProjectRoot()
	// Should find go.mod in project root
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Errorf("findProjectRoot returned %q which has no go.mod: %v", root, err)
	}
}
