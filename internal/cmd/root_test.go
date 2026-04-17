package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestNewRootCommand(t *testing.T) {
	cmd := NewRootCommand()
	if cmd == nil {
		t.Fatal("NewRootCommand() returned nil")
	}
}

func captureOutput(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestRunNoArgs(t *testing.T) {
	cmd := NewRootCommand()
	output := captureOutput(func() {
		cmd.Run([]string{})
	})

	if !strings.Contains(output, "llm-manager") {
		t.Errorf("Output does not contain 'llm-manager': %s", output)
	}
	if !strings.Contains(output, "COMMANDS") {
		t.Errorf("Output does not contain 'COMMANDS': %s", output)
	}
}

func TestRunHelp(t *testing.T) {
	cmd := NewRootCommand()
	output := captureOutput(func() {
		cmd.Run([]string{"help"})
	})

	if !strings.Contains(output, "llm-manager") {
		t.Errorf("Output does not contain 'llm-manager': %s", output)
	}
}

func TestRunVersion(t *testing.T) {
	cmd := NewRootCommand()
	output := captureOutput(func() {
		cmd.Run([]string{"version"})
	})

	if !strings.Contains(output, "llm-manager version") {
		t.Errorf("Output does not contain 'llm-manager version': %s", output)
	}
}

func TestRunVersionFlag(t *testing.T) {
	cmd := NewRootCommand()
	output := captureOutput(func() {
		cmd.Run([]string{"--version"})
	})

	if !strings.Contains(output, "llm-manager version") {
		t.Errorf("Output does not contain 'llm-manager version': %s", output)
	}
}

func TestRunConfig(t *testing.T) {
	cmd := NewRootCommand()
	output := captureOutput(func() {
		cmd.Run([]string{"config"})
	})

	if !strings.Contains(output, "llm-manager config") {
		t.Errorf("Output does not contain 'llm-manager config': %s", output)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	cmd := NewRootCommand()
	exitCode := cmd.Run([]string{"unknown"})

	if exitCode != 1 {
		t.Errorf("Exit code = %d, want 1", exitCode)
	}
}

func TestPrintShortVersion(t *testing.T) {
	output := captureOutput(func() {
		PrintShortVersion()
	})

	if !strings.Contains(output, "v") {
		t.Errorf("Output does not contain 'v': %s", output)
	}
}

func TestPlatformInfo(t *testing.T) {
	info := PlatformInfo()
	if info == "" {
		t.Error("PlatformInfo() returned empty string")
	}

	parts := strings.Split(info, "/")
	if len(parts) != 2 {
		t.Errorf("PlatformInfo() = %q, want format 'os/arch'", info)
	}
}
