package cmd

import "testing"

func TestRegistry_Basic(t *testing.T) {
	// Test that registered commands can be retrieved
	names := RegisteredCommandNames()
	if len(names) == 0 {
		t.Error("no commands registered")
	}

	// Check known commands are registered
	expected := []string{"model", "container", "service", "hotspot", "logs", "mem", "update", "comfyui", "embed", "rerank", "rag", "speech", "import", "export", "compose", "swap", "litellm", "install"}
	if len(expected) != len(names) {
		t.Errorf("expected %d registered commands, got %d", len(expected), len(names))
	}
	for _, name := range expected {
		found := false
		for _, n := range names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("command %q not found in registry", name)
		}
	}
}

func TestRegistry_GetUnknown(t *testing.T) {
	_, ok := getCommand("nonexistent", nil)
	if ok {
		t.Error("getCommand should return false for unknown command")
	}
}

func TestRegistry_RegisteredCommandNamesSorted(t *testing.T) {
	names := RegisteredCommandNames()
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("names not sorted: %q at %d > %q at %d", names[i], i, names[i-1], i-1)
		}
	}
}

func TestCommandDispatcher_Dispatch(t *testing.T) {
	// Verify the dispatcher can be created without panicking
	d := NewCommandDispatcher(nil, nil)
	if d == nil {
		t.Error("NewCommandDispatcher returned nil")
	}
}
