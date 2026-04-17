package models

import (
	"testing"

	"github.com/google/uuid"
)

func TestHotspotTableName(t *testing.T) {
	h := Hotspot{}
	if got := h.TableName(); got != "hotspots" {
		t.Errorf("Hotspot.TableName() = %q, want %q", got, "hotspots")
	}
}

func TestHotspotBeforeCreate_GeneratesUUID(t *testing.T) {
	h := &Hotspot{
		ModelSlug: "test-model",
	}

	err := h.BeforeCreate(nil)
	if err != nil {
		t.Fatalf("BeforeCreate() returned error: %v", err)
	}

	if h.ID == uuid.Nil {
		t.Error("BeforeCreate() did not generate a UUID")
	}
}

func TestHotspotBeforeCreate_PreservesExistingUUID(t *testing.T) {
	existingID := uuid.New()
	h := &Hotspot{
		ID:        existingID,
		ModelSlug: "test-model",
	}

	err := h.BeforeCreate(nil)
	if err != nil {
		t.Fatalf("BeforeCreate() returned error: %v", err)
	}

	if h.ID != existingID {
		t.Errorf("BeforeCreate() changed existing UUID: got %v, want %v", h.ID, existingID)
	}
}

func TestHotspotFields(t *testing.T) {
	tests := []struct {
		name      string
		modelSlug string
		active    bool
	}{
		{
			name:      "active hotspot",
			modelSlug: "qwen3_6",
			active:    true,
		},
		{
			name:      "inactive hotspot",
			modelSlug: "qwen3_5",
			active:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := Hotspot{
				ModelSlug: tt.modelSlug,
				Active:    tt.active,
			}

			if h.ModelSlug != tt.modelSlug {
				t.Errorf("ModelSlug = %q, want %q", h.ModelSlug, tt.modelSlug)
			}
			if h.Active != tt.active {
				t.Errorf("Active = %v, want %v", h.Active, tt.active)
			}
		})
	}
}

func TestHotspotDefaultActive(t *testing.T) {
	h := Hotspot{
		ModelSlug: "test-model",
	}

	// Go zero value for bool is false.
	// The "true" default is applied by the GORM migration (DB level),
	// not by Go struct initialization.
	if h.Active {
		t.Error("Zero-value Active = true, want false")
	}
}
