package version

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	got := Version()
	if got == "" {
		t.Error("Version() returned empty string")
	}
}

func TestCommit(t *testing.T) {
	got := Commit()
	if got == "" {
		t.Error("Commit() returned empty string")
	}
}

func TestInfo(t *testing.T) {
	got := Info()
	if got == "" {
		t.Error("Info() returned empty string")
	}
	if !strings.Contains(got, "llm-manager") {
		t.Errorf("Info() does not contain 'llm-manager': %s", got)
	}
}

func TestShortVersion(t *testing.T) {
	got := ShortVersion()
	if got == "" {
		t.Error("ShortVersion() returned empty string")
	}
}

func TestIsDev(t *testing.T) {
	// Default build should be dev
	if !IsDev() {
		t.Log("IsDev() = false (may be overridden by ldflags)")
	}
}
