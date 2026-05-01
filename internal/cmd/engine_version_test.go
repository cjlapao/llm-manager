package cmd

import (
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
)

func TestCmdVersionCreate_Parsing(t *testing.T) {
	db, _ := database.NewDatabaseManager(":memory:")
	db.Open()
	root := &RootCommand{cfg: &config.Config{LLMDir: "/tmp/test"}, db: db}
	c := NewEngineCommand(root)

	// Test: missing file path
	args := []string{}
	exit := c.cmdImport(args)
	if exit != 1 {
		t.Error("expected error for missing file path")
	}

	// Test: non-existent file
	args = []string{"/nonexistent/path.yml"}
	exit = c.cmdImport(args)
	if exit != 1 {
		t.Error("expected error for non-existent file")
	}

	// Test: unknown flag
	args = []string{"--unknown"}
	exit = c.cmdImport(args)
	if exit != 1 {
		t.Error("expected error for unknown flag")
	}
}

func TestCmdVersion_List(t *testing.T) {
	db, _ := database.NewDatabaseManager(":memory:")
	db.Open()
	db.ApplyPendingMigrations()
	root := &RootCommand{cfg: &config.Config{LLMDir: "/tmp/test"}, db: db}
	c := NewEngineCommand(root)

	// Empty database → should succeed
	exit := c.cmdVersionList(nil)
	if exit != 0 {
		t.Errorf("expected success for empty list, got exit %d", exit)
	}
}

func TestCmdVersion_Get(t *testing.T) {
	db, _ := database.NewDatabaseManager(":memory:")
	db.Open()
	root := &RootCommand{cfg: &config.Config{LLMDir: "/tmp/test"}, db: db}
	c := NewEngineCommand(root)

	// Missing arg
	exit := c.cmdVersionGet(nil)
	if exit != 1 {
		t.Error("expected error for missing type/slug")
	}

	// Invalid format
	exit = c.cmdVersionGet([]string{"invalid"})
	if exit != 1 {
		t.Error("expected error for invalid format")
	}
}

func TestCmdVersion_Delete(t *testing.T) {
	db, _ := database.NewDatabaseManager(":memory:")
	db.Open()
	root := &RootCommand{cfg: &config.Config{LLMDir: "/tmp/test"}, db: db}
	c := NewEngineCommand(root)

	// Missing arg
	exit := c.cmdVersionDelete(nil)
	if exit != 1 {
		t.Error("expected error for missing type/slug")
	}
}

func TestCmdVersion_ShowComposition(t *testing.T) {
	db, _ := database.NewDatabaseManager(":memory:")
	db.Open()
	root := &RootCommand{cfg: &config.Config{LLMDir: "/tmp/test"}, db: db}
	c := NewEngineCommand(root)

	// Missing arg
	exit := c.cmdVersionShowComposition(nil)
	if exit != 1 {
		t.Error("expected error for missing type/slug")
	}
}

func TestEngineList(t *testing.T) {
	db, _ := database.NewDatabaseManager(":memory:")
	db.Open()
	db.ApplyPendingMigrations()
	root := &RootCommand{cfg: &config.Config{LLMDir: "/tmp/test"}, db: db}
	c := NewEngineCommand(root)

	// Empty → success
	exit := c.cmdList(nil)
	if exit != 0 {
		t.Errorf("expected success for empty list, got exit %d", exit)
	}
}

func TestEngineGet(t *testing.T) {
	db, _ := database.NewDatabaseManager(":memory:")
	db.Open()
	root := &RootCommand{cfg: &config.Config{LLMDir: "/tmp/test"}, db: db}
	c := NewEngineCommand(root)

	// Missing arg
	exit := c.cmdGet(nil)
	if exit != 1 {
		t.Error("expected error for missing slug")
	}
}

func TestEngineDelete(t *testing.T) {
	db, _ := database.NewDatabaseManager(":memory:")
	db.Open()
	root := &RootCommand{cfg: &config.Config{LLMDir: "/tmp/test"}, db: db}
	c := NewEngineCommand(root)

	// Missing arg
	exit := c.cmdDelete(nil)
	if exit != 1 {
		t.Error("expected error for missing slug")
	}
}
