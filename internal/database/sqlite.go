// Package database provides SQLite-backed persistence for llm-manager entities.
package database

import (
	"fmt"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/user/llm-manager/internal/database/migrations"
	"github.com/user/llm-manager/internal/version"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)


// sqliteManager implements DatabaseManager using SQLite.
type sqliteManager struct {
	dsn string
	db  *gorm.DB
	mg  *migrations.Engine
}

// NewDatabaseManager creates a new SQLite-backed DatabaseManager.
func NewDatabaseManager(dsn string) (DatabaseManager, error) {
	return &sqliteManager{dsn: dsn}, nil
}

// Open opens the database connection.
func (m *sqliteManager) Open() error {
	db, err := gorm.Open(sqlite.Open(m.dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	m.db = db

	// Create the migrations engine
	m.mg, err = migrations.NewEngine(db)
	if err != nil {
		return fmt.Errorf("failed to create migration engine: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (m *sqliteManager) Close() error {
	if m.db == nil {
		return nil
	}
	sqlDB, err := m.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}
	return sqlDB.Close()
}

// SchemaVersion returns the current schema version from applied migrations.
func (m *sqliteManager) SchemaVersion() (int, error) {
	if m.mg == nil {
		return 0, fmt.Errorf("migration engine not initialized")
	}
	return m.mg.CurrentVersion()
}

// LatestVersion returns the latest migration version known in code.
func (m *sqliteManager) LatestVersion() (int, error) {
	if m.mg == nil {
		return 0, fmt.Errorf("migration engine not initialized")
	}
	return m.mg.LatestVersion(), nil
}

// ApplyPendingMigrations runs all pending up-migrations.
// When verbose is true, prints informational messages about migration status.
func (m *sqliteManager) ApplyPendingMigrations(verbose bool) error {
	if m.mg == nil {
		return fmt.Errorf("migration engine not initialized")
	}
	if verbose {
		fmt.Println("Checking for pending migrations...")
		fmt.Printf("  Version: %s (built: %s)\n", version.Version(), version.Date())
	}
	if err := m.ensureLegacyColumns(); err != nil {
		return fmt.Errorf("legacy column check failed: %w", err)
	}
	if err := m.mg.ApplyUp(); err != nil {
		return fmt.Errorf("pending migrations failed: %w", err)
	}
	return nil
}

// ensureLegacyColumns adds any columns that may be missing from pre-migration
// databases. Old databases created the models table directly without these columns.
// This runs BEFORE the migration engine so that the engine can safely assume
// the table schema is complete after migration 001 creates it.
func (m *sqliteManager) ensureLegacyColumns() error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	// Skip if the models table doesn't exist yet (fresh database - migrations will create it)
	var exists int
	if err := m.db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='models'").Scan(&exists).Error; err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	columns := []struct {
		table string
		col   string
		def   string
	}{
		{"models", "sub_type", "TEXT"},
		{"models", "engine_type", "TEXT DEFAULT 'vllm'"},
		{"models", "env_vars", "TEXT"},
		{"models", "command_args", "TEXT"},
		{"models", "input_token_cost", "REAL DEFAULT 0"},
		{"models", "output_token_cost", "REAL DEFAULT 0"},
		{"models", "cache_creation_input_token_cost", "REAL DEFAULT 0"},
		{"models", "cache_read_input_token_cost", "REAL DEFAULT 0"},
		{"models", "capabilities", "TEXT"},
		{"models", "lite_llm_params", "TEXT"},
		{"models", "model_info", "TEXT"},
		{"models", "litellm_model_id", "TEXT"},
		{"models", "litellm_active_aliases", "TEXT"},
		{"models", "litellm_variant_ids", "TEXT"},
		{"models", "base_image_id", "TEXT DEFAULT ''"},
		{"models", "default", "BOOLEAN DEFAULT 0"},
		{"models", "max_num_seqs", "INTEGER"},
		{"models", "max_num_batched_tokens", "INTEGER"},
		{"models", "speculative_decoding", "TEXT"},
		{"models", "num_speculative_tokens", "INTEGER"},
		{"models", "speculative_model", "TEXT"},
	}
	for _, c := range columns {
		colRef := c.col
		if c.col == "default" {
			colRef = "`default`"
		}
		if err := m.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", c.table, colRef, c.def)).Error; err != nil {
			// Ignore "duplicate column" errors - column already exists
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("add column %s.%s: %w", c.table, c.col, err)
			}
		}
	}
	return nil
}

// MigrateTo migrates to a specific target schema version.
func (m *sqliteManager) MigrateTo(targetVersion int) error {
	if m.mg == nil {
		return fmt.Errorf("migration engine not initialized")
	}

	currentVersion, err := m.SchemaVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	maxVersion := m.mg.LatestVersion()
	if targetVersion > maxVersion {
		return fmt.Errorf("target version %d exceeds latest known migration %d", targetVersion, maxVersion)
	}

	// Get migrations we need to apply or roll back
	allMigrations := m.mg.Migrations()

	if targetVersion > currentVersion {
		// Up migration
		count := 0
		for _, mig := range allMigrations {
			if mig.Version > currentVersion && mig.Version <= targetVersion {
				if err := m.mg.RunUp(mig); err != nil {
					return fmt.Errorf("migration up to %d failed on %d: %w", targetVersion, mig.Version, err)
				}
				count++
			}
		}
		fmt.Printf("Applied %d up migrations to reach version %d\n", count, targetVersion)
	} else if targetVersion < currentVersion {
		// Down migration - reverse order, applied last first
		count := 0
		for i := len(allMigrations) - 1; i >= 0; i-- {
			mig := allMigrations[i]
			if mig.Version > targetVersion && mig.Version <= currentVersion {
				if err := m.mg.RunDown(mig); err != nil {
					return fmt.Errorf("down migration to %d failed on %d: %w", targetVersion, mig.Version, err)
				}
				count++
			}
		}
		fmt.Printf("Rolled back %d migrations to reach version %d\n", count, targetVersion)
	} else {
		fmt.Println("Already at target version")
	}

	return nil
}

// DB returns the underlying GORM database instance.
func (m *sqliteManager) DB() *gorm.DB {
	return m.db
}

// AutoMigrate runs the migration engine to ensure schema is up to date.
// Kept for backward compatibility with existing tests and code.
func (m *sqliteManager) AutoMigrate() error {
	if m.mg == nil {
		return fmt.Errorf("migration engine not initialized")
	}
	fmt.Println("Running migration engine (backward compatible)...")
	return m.ApplyPendingMigrations(true) // Always verbose for backward-compat AutoMigrate
}

// SuppressRecordNotFound wraps a GORM query to suppress "record not found" log noise.
// Use this for existence checks where "not found" is expected behavior.
func SuppressRecordNotFound(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

// LogDB returns a GORM DB instance with verbose logging for debugging.
func LogDB(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Info),
	})
}
