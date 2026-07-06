package tui

import (
	"testing"
)

func TestInitialModel(t *testing.T) {
	labels := []string{"Build", "Test", "Deploy"}
	m := InitialModel(labels)

	if len(m.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(m.Steps))
	}
	if m.Steps[0].Label != "Build" {
		t.Errorf("expected Build, got %s", m.Steps[0].Label)
	}
	if m.Steps[0].Status != StepPending {
		t.Errorf("expected StepPending, got %v", m.Steps[0].Status)
	}
}

func TestModelUpdate(t *testing.T) {
	labels := []string{"Step 1"}
	m := InitialModel(labels)

	// Test StepUpdateMsg
	newModel, _ := m.Update(StepUpdateMsg{StepID: 1, Status: StepRunning, Info: "Starting"})
	m = newModel.(Model)
	if m.Steps[0].Status != StepRunning {
		t.Errorf("expected StepRunning, got %v", m.Steps[0].Status)
	}
	if m.Steps[0].Info != "Starting" {
		t.Errorf("expected Starting, got %s", m.Steps[0].Info)
	}

	// Test LogMsg
	newModel, _ = m.Update(LogMsg("test log"))
	m = newModel.(Model)
	if len(m.logs) != 1 || m.logs[0] != "test log" {
		t.Errorf("expected test log, got %v", m.logs)
	}

	// Test ConfigMsg
	newModel, _ = m.Update(ConfigMsg{Cloud: "my-cloud", Host: "my-host"})
	m = newModel.(Model)
	if m.Cloud != "my-cloud" || m.Host != "my-host" {
		t.Errorf("unexpected cloud/host: %s/%s", m.Cloud, m.Host)
	}

	// Test completion
	newModel, _ = m.Update(StepUpdateMsg{StepID: 1, Status: StepCompleted})
	m = newModel.(Model)
	if !m.quitting {
		t.Error("expected model to be quitting after all steps completed")
	}
}

func TestModelView(t *testing.T) {
	labels := []string{"Step 1"}
	m := InitialModel(labels)
	m.Cloud = "c1"
	m.Host = "h1"
	m.Image = "i1"

	view := m.View()
	if !contains(view, "c1") || !contains(view, "h1") || !contains(view, "i1") {
		t.Errorf("view missing header info: %s", view)
	}
	if !contains(view, "Step 1") {
		t.Errorf("view missing step label: %s", view)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(substr) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr))))
}
