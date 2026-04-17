package version

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	t.Run("default version is dev", func(t *testing.T) {
		if got := Version(); got != "dev" {
			t.Errorf("Version() = %q, want %q", got, "dev")
		}
	})

	t.Run("short version includes dev", func(t *testing.T) {
		got := ShortVersion()
		if !strings.Contains(got, "dev") {
			t.Errorf("ShortVersion() = %q, want to contain %q", got, "dev")
		}
	})

	t.Run("is dev", func(t *testing.T) {
		if !IsDev() {
			t.Error("IsDev() = false, want true for default build")
		}
	})
}

func TestShortCommit(t *testing.T) {
	tests := []struct {
		name    string
		commit  string
		wantLen int
	}{
		{"short commit", "abc123", 6},
		{"exact 7 chars", "abcdef1", 7},
		{"long commit", "abcdef1234567890", 7},
		{"empty commit", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := commit
			commit = tt.commit
			defer func() { commit = original }()

			got := shortCommit()
			if len(got) != tt.wantLen {
				t.Errorf("shortCommit() length = %d, want %d (got %q)", len(got), tt.wantLen, got)
			}
		})
	}
}

func TestInfo(t *testing.T) {
	info := Info()
	if !strings.Contains(info, "llm-manager version") {
		t.Errorf("Info() does not contain expected prefix, got: %s", info)
	}
	if !strings.Contains(info, "go version:") {
		t.Errorf("Info() does not contain go version, got: %s", info)
	}
	if !strings.Contains(info, "architecture:") {
		t.Errorf("Info() does not contain architecture, got: %s", info)
	}
}
