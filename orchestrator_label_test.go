package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// newTestOrchestrator builds a minimal Orchestrator suitable for unit tests.
// It uses a temp directory for runDir so setStatus can write status.json.
func newTestOrchestrator(t *testing.T, nodes []NodeConfig) *Orchestrator {
	t.Helper()
	runDir := t.TempDir()

	nm := make(map[string]NodeConfig)
	for _, n := range nodes {
		nm[n.ID] = n
	}

	return &Orchestrator{
		config: PipelineConfig{
			Nodes: nodes,
		},
		runID:    "test-run",
		runDir:   runDir,
		logsDir:  filepath.Join(runDir, "logs"),
		projects: make(map[string]Project),
		nodesMap: nm,
		statuses: make(map[string]NodeStatus),
		answerChs: make(map[string]chan []Question),
		logStreams: make(map[string]*LogStream),
	}
}

// TestSetStatus_PopulatesLabelFromNodeConfig verifies that setStatus copies the
// node's Label into NodeStatus.Label on the first call.
func TestSetStatus_PopulatesLabelFromNodeConfig(t *testing.T) {
	nodes := []NodeConfig{
		{ID: "agent_1", Type: "agent", Label: "Coder"},
	}
	o := newTestOrchestrator(t, nodes)

	o.setStatus("agent_1", "running", "Working...")

	ns := o.statuses["agent_1"]
	if ns.Label != "Coder" {
		t.Errorf("expected Label=%q, got %q", "Coder", ns.Label)
	}
}

// TestSetStatus_LabelNotOverwrittenOnSubsequentCalls verifies that once Label is
// set it is not replaced by a later setStatus call (label comes from nodesMap,
// but the guard `if ns.Label == ""` must hold).
func TestSetStatus_LabelNotOverwrittenOnSubsequentCalls(t *testing.T) {
	nodes := []NodeConfig{
		{ID: "agent_1", Type: "agent", Label: "Coder"},
	}
	o := newTestOrchestrator(t, nodes)

	o.setStatus("agent_1", "running", "Working...")
	// Simulate a node whose Label in nodesMap changed (shouldn't happen in
	// practice, but verifies the guard).
	o.nodesMap["agent_1"] = NodeConfig{ID: "agent_1", Label: "SomethingElse"}
	o.setStatus("agent_1", "done", "Complete")

	ns := o.statuses["agent_1"]
	if ns.Label != "Coder" {
		t.Errorf("Label should remain %q, got %q", "Coder", ns.Label)
	}
}

// TestSetStatus_EmptyLabelWhenNodeHasNoLabel verifies that nodes without a label
// don't get a spurious label in NodeStatus.
func TestSetStatus_EmptyLabelWhenNodeHasNoLabel(t *testing.T) {
	nodes := []NodeConfig{
		{ID: "task_1", Type: "task", Label: ""},
	}
	o := newTestOrchestrator(t, nodes)

	o.setStatus("task_1", "done", "Task provided")

	ns := o.statuses["task_1"]
	if ns.Label != "" {
		t.Errorf("expected empty Label, got %q", ns.Label)
	}
}

// TestSetStatus_LabelForMultipleNodes verifies that each node gets its own label.
func TestSetStatus_LabelForMultipleNodes(t *testing.T) {
	nodes := []NodeConfig{
		{ID: "agent_1", Type: "agent", Label: "Coder"},
		{ID: "agent_2", Type: "agent", Label: "Tester"},
		{ID: "agent_3", Type: "agent", Label: "Reviewer"},
	}
	o := newTestOrchestrator(t, nodes)

	for _, n := range nodes {
		o.setStatus(n.ID, "running", "Working...")
	}

	want := map[string]string{
		"agent_1": "Coder",
		"agent_2": "Tester",
		"agent_3": "Reviewer",
	}
	for id, wantLabel := range want {
		if got := o.statuses[id].Label; got != wantLabel {
			t.Errorf("node %s: expected Label=%q, got %q", id, wantLabel, got)
		}
	}
}

// TestNodeStatus_LabelSerializesInJSON verifies the Label field appears in the
// JSON output of NodeStatus (and is omitted when empty).
func TestNodeStatus_LabelSerializesInJSON(t *testing.T) {
	ns := NodeStatus{
		Status:  "running",
		Label:   "Coder",
		Message: "Working...",
	}
	data, err := json.Marshal(ns)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]interface{}
	json.Unmarshal(data, &m)

	if got, ok := m["label"]; !ok || got != "Coder" {
		t.Errorf("expected json label=%q, got %v (present=%v)", "Coder", got, ok)
	}
}

// TestNodeStatus_LabelOmittedWhenEmpty verifies omitempty behaviour.
func TestNodeStatus_LabelOmittedWhenEmpty(t *testing.T) {
	ns := NodeStatus{
		Status:  "done",
		Message: "Task provided",
	}
	data, _ := json.Marshal(ns)

	var m map[string]interface{}
	json.Unmarshal(data, &m)

	if _, ok := m["label"]; ok {
		t.Error("label key should be absent from JSON when empty, but it was present")
	}
}

// TestSetStatus_WritesLabelToStatusJSON verifies that the status.json file
// written by setStatus contains the label field.
func TestSetStatus_WritesLabelToStatusJSON(t *testing.T) {
	nodes := []NodeConfig{
		{ID: "agent_1", Type: "agent", Label: "Tester"},
	}
	o := newTestOrchestrator(t, nodes)

	o.setStatus("agent_1", "running", "Running tests...")

	statusPath := filepath.Join(o.runDir, "status.json")
	raw, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("status.json not written: %v", err)
	}

	var statuses map[string]NodeStatus
	if err := json.Unmarshal(raw, &statuses); err != nil {
		t.Fatalf("cannot parse status.json: %v", err)
	}

	ns, ok := statuses["agent_1"]
	if !ok {
		t.Fatal("agent_1 not found in status.json")
	}
	if ns.Label != "Tester" {
		t.Errorf("status.json label: expected %q, got %q", "Tester", ns.Label)
	}
}

// TestSetStatus_UnknownNodeID verifies setStatus is safe for unknown node IDs
// (e.g., no label is set but also no panic).
func TestSetStatus_UnknownNodeID(t *testing.T) {
	o := newTestOrchestrator(t, nil)
	// Should not panic
	o.setStatus("ghost_node", "error", "not found")
	ns := o.statuses["ghost_node"]
	if ns.Label != "" {
		t.Errorf("unknown node should have empty label, got %q", ns.Label)
	}
}

// TestConcurrentSetStatus verifies that concurrent setStatus calls don't race
// or corrupt the label (checked with -race).
func TestConcurrentSetStatus(t *testing.T) {
	nodes := []NodeConfig{
		{ID: "agent_1", Type: "agent", Label: "Coder"},
		{ID: "agent_2", Type: "agent", Label: "Tester"},
	}
	o := newTestOrchestrator(t, nodes)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			o.setStatus("agent_1", "running", "Working...")
		}()
		go func() {
			defer wg.Done()
			o.setStatus("agent_2", "running", "Testing...")
		}()
	}
	wg.Wait()

	if o.statuses["agent_1"].Label != "Coder" {
		t.Errorf("agent_1 label corrupted: %q", o.statuses["agent_1"].Label)
	}
	if o.statuses["agent_2"].Label != "Tester" {
		t.Errorf("agent_2 label corrupted: %q", o.statuses["agent_2"].Label)
	}
}
