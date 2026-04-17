package models

import (
	"testing"

	"github.com/google/uuid"
)

func TestContainerTableName(t *testing.T) {
	c := Container{}
	if got := c.TableName(); got != "containers" {
		t.Errorf("Container.TableName() = %q, want %q", got, "containers")
	}
}

func TestContainerBeforeCreate_GeneratesUUID(t *testing.T) {
	c := &Container{
		Slug:   "test-container",
		Status: "stopped",
	}

	err := c.BeforeCreate(nil)
	if err != nil {
		t.Fatalf("BeforeCreate() returned error: %v", err)
	}

	if c.ID == uuid.Nil {
		t.Error("BeforeCreate() did not generate a UUID")
	}
}

func TestContainerBeforeCreate_PreservesExistingUUID(t *testing.T) {
	existingID := uuid.New()
	c := &Container{
		ID:     existingID,
		Slug:   "test-container",
		Status: "stopped",
	}

	err := c.BeforeCreate(nil)
	if err != nil {
		t.Fatalf("BeforeCreate() returned error: %v", err)
	}

	if c.ID != existingID {
		t.Errorf("BeforeCreate() changed existing UUID: got %v, want %v", c.ID, existingID)
	}
}

func TestContainerFields(t *testing.T) {
	c := Container{
		Slug:    "test-container",
		Name:    "test-container-name",
		Status:  "running",
		Port:    8080,
		GPUUsed: true,
	}

	if c.Slug != "test-container" {
		t.Errorf("Slug = %q, want %q", c.Slug, "test-container")
	}
	if c.Name != "test-container-name" {
		t.Errorf("Name = %q, want %q", c.Name, "test-container-name")
	}
	if c.Status != "running" {
		t.Errorf("Status = %q, want %q", c.Status, "running")
	}
	if c.Port != 8080 {
		t.Errorf("Port = %d, want %d", c.Port, 8080)
	}
	if !c.GPUUsed {
		t.Error("GPUUsed = false, want true")
	}
}

func TestContainerDefaultStatus(t *testing.T) {
	c := Container{
		Slug: "test-container",
	}

	// Go zero value for string is empty.
	// The "stopped" default is applied by the GORM migration (DB level),
	// not by Go struct initialization.
	if c.Status != "" {
		t.Errorf("Zero-value Status = %q, want empty string", c.Status)
	}
}
