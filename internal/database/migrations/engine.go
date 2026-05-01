// Package migrations provides a versioned migration engine for SQLite databases.
package migrations

import (
	"embed"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

//go:embed 001_create_schema/*.sql 002_add_default_column/*.sql 003_create_engine_tables/*.sql
var migrationFS embed.FS

// Direction indicates which way a migration runs.
type Direction string

const (
	DirectionUp   Direction = "up"
	DirectionDown Direction = "down"
)

// Migration represents a single versioned migration.
type Migration struct {
	Version int
	Name    string
	UpSQL   string
	DownSQL string
}

// Engine handles applying migrations up or down.
type Engine struct {
	db         *gorm.DB
	migrations []Migration
}

// NewEngine creates a new migration engine from SQL migration files.
func NewEngine(db *gorm.DB) (*Engine, error) {
	if db == nil {
		return nil, fmt.Errorf("database not open")
	}
	engine := &Engine{db: db}
	err := engine.loadMigrations()
	if err != nil {
		return nil, fmt.Errorf("failed to load migrations: %w", err)
	}
	return engine, nil
}

// loadMigrations reads embedded SQL files with known names.
// Currently: 001_create_schema, 002_add_default_column
func (e *Engine) loadMigrations() error {
	files := []struct {
		version  int
		name     string
		upName   string
		downName string
	}{
		{1, "create_schema", "001_create_schema/up.sql", "001_create_schema/down.sql"},
		{2, "add_default_column", "002_add_default_column/up.sql", "002_add_default_column/down.sql"},
		{3, "create_engine_tables", "003_create_engine_tables/up.sql", "003_create_engine_tables/down.sql"},
	}

	for _, f := range files {
		upData, err := migrationFS.ReadFile(f.upName)
		if err != nil {
			return fmt.Errorf("missing %s: %w", f.upName, err)
		}
		downData, err := migrationFS.ReadFile(f.downName)
		if err != nil {
			return fmt.Errorf("missing %s: %w", f.downName, err)
		}
		e.migrations = append(e.migrations, Migration{
			Version: f.version,
			Name:    f.name,
			UpSQL:   string(upData),
			DownSQL: string(downData),
		})
	}

	return nil
}

// CurrentVersion returns the highest applied migration version. 0 if none or table missing.
func (e *Engine) CurrentVersion() (int, error) {
	var maxVer int
	err := e.db.Raw(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE applied = 1`).Scan(&maxVer).Error
	if err != nil {
		return 0, nil // table doesn't exist yet, treat as version 0
	}
	return maxVer, nil
}

// LatestVersion returns the latest migration version known in code.
func (e *Engine) LatestVersion() int {
	if len(e.migrations) == 0 {
		return 0
	}
	return e.migrations[len(e.migrations)-1].Version
}

// Migrations returns all migrations sorted by version.
func (e *Engine) Migrations() []Migration {
	return e.migrations
}

// RunUp runs the UP part of a single migration.
func (e *Engine) RunUp(m Migration) error {
	return e.runSingleMigration(m, DirectionUp)
}

// RunDown runs the DOWN part of a single migration.
func (e *Engine) RunDown(m Migration) error {
	return e.runSingleMigration(m, DirectionDown)
}

// PendingMigrations returns migrations that haven't been applied yet.
func (e *Engine) PendingMigrations(currentVersion int) []Migration {
	var pending []Migration
	for _, m := range e.migrations {
		if m.Version > currentVersion {
			pending = append(pending, m)
		}
	}
	return pending
}

// AppliedMigrations returns migrations between two versions.
func (e *Engine) AppliedMigrations(from, to int) []Migration {
	var applied []Migration
	for _, m := range e.migrations {
		if m.Version > from && m.Version <= to {
			applied = append(applied, m)
		}
	}
	return applied
}

// Status shows the current state of migrations.
func (e *Engine) Status() {
	current, _ := e.CurrentVersion()
	latest := e.LatestVersion()
	fmt.Printf("Schema version: %d / %d\n", current, latest)
	if current >= latest {
		fmt.Println("Database schema is up to date.")
		return
	}
	pending := e.PendingMigrations(current)
	fmt.Printf("\n%d pending migration(s):\n", len(pending))
	for _, m := range pending {
		fmt.Printf("  [up] %03d: %s\n", m.Version, m.Name)
	}
}

// ApplyUp runs all pending migrations in order.
// If any migration fails, it rolls back all previously-applied migrations in this batch.
func (e *Engine) ApplyUp() error {
	currentVersion, err := e.CurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}
	pending := e.PendingMigrations(currentVersion)
	if len(pending) == 0 {
		return nil // nothing to do
	}
	successCount := 0
	for index, migration := range pending {
		if err := e.runSingleMigration(migration, DirectionUp); err != nil {
			rollbackApplied := e.AppliedMigrations(currentVersion, currentVersion+index)
			for i := len(rollbackApplied) - 1; i >= 0; i-- {
				rm := rollbackApplied[i]
				if rbErr := e.runSingleMigration(rm, DirectionDown); rbErr != nil {
					return fmt.Errorf("migration %d failed (%v) AND rollback failed (%v): database may be inconsistent", rm.Version, err, rbErr)
				}
			}
			return fmt.Errorf("migration %d (%s) failed (rolled back): %w", migration.Version, migration.Name, err)
		}
		successCount++
	}
	fmt.Printf("Applied %d migration(s): ", successCount)
	for i, m := range pending {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("%d:%s", m.Version, m.Name)
	}
	fmt.Println()
	return nil
}

// runSingleMigration executes a single migration.
// NOTE: We intentionally do NOT wrap in a GORM transaction.
// SQLite DDL statements (CREATE/ALTER/DROP TABLE) implicitly commit
// any active transaction before executing, which causes issues when
// combined with INSERT OR REPLACE in the same transaction block.
// By executing statements directly, each DDL statement auto-commits
// and the migration record insertion is guaranteed to persist.
func (e *Engine) runSingleMigration(m Migration, direction Direction) error {
	sql := ""
	if direction == DirectionUp {
		sql = m.UpSQL
	} else {
		sql = m.DownSQL
	}
	statements := splitStatements(sql)
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if err := e.db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("failed to execute statement: %w", err)
		}
	}
	recordQuery := ""
	if direction == DirectionUp {
		recordQuery = fmt.Sprintf(
			"INSERT OR REPLACE INTO schema_migrations (version, name, direction, applied_at, applied) VALUES (%d, '%s', 'up', datetime('now'), 1)",
			m.Version, escapeSQLString(m.Name),
		)
	} else {
		recordQuery = fmt.Sprintf("DELETE FROM schema_migrations WHERE version = %d AND direction = 'up'", m.Version)
	}
	if err := e.db.Exec(recordQuery).Error; err != nil {
		return fmt.Errorf("failed to %s migration record: %w", string(direction), err)
	}
	return nil
}

// escapeSQLString escapes single quotes.
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// splitStatements splits SQL by semicolons, respecting quotes and parentheses.
func splitStatements(sql string) []string {
	var statements []string
	var builder strings.Builder
	quoteChar := rune(0)
	depth := 0
	for _, r := range sql {
		switch {
		case (r == '\'' || r == '"') && quoteChar == 0:
			quoteChar = r
			builder.WriteRune(r)
		case r == quoteChar:
			quoteChar = 0
			builder.WriteRune(r)
		case quoteChar == 0:
			if r == '(' {
				depth++
			} else if r == ')' {
				depth--
			} else if r == ';' && depth == 0 {
				s := strings.TrimSpace(builder.String())
				if s != "" {
					statements = append(statements, s)
				}
				builder.Reset()
				continue
			}
			builder.WriteRune(r)
		default:
			builder.WriteRune(r)
		}
	}
	last := strings.TrimSpace(builder.String())
	if last != "" {
		statements = append(statements, last)
	}
	return statements
}

// parseVersion extracts numeric prefix from a dir like "001_FOO".
func parseVersion(dirName string) int {
	v, err := strconv.Atoi(strings.SplitN(dirName, "_", 2)[0])
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

// ColumnExists checks whether a column exists in a table using PRAGMA table_info.
func (e *Engine) ColumnExists(tableName, columnName string) (bool, error) {
	return e.columnExists(tableName, columnName)
}

// columnExists checks whether a column exists in a table using PRAGMA table_info.
func (e *Engine) columnExists(tableName, columnName string) (bool, error) {
	rows, err := e.db.Raw(fmt.Sprintf("PRAGMA table_info(%s)", tableName)).Rows()
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	return false, rows.Err()
}

// AddColumnIfNotExists adds a column to a table only if it does not already exist.
// This is a safe, idempotent operation that works around SQLite's lack of
// IF NOT EXISTS support for ALTER TABLE ADD COLUMN.
func (e *Engine) AddColumnIfNotExists(tableName, columnName, columnDef string) error {
	exists, err := e.columnExists(tableName, columnName)
	if err != nil {
		return fmt.Errorf("check column %s.%s: %w", tableName, columnName, err)
	}
	if exists {
		return nil
	}
	return e.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnDef)).Error
}
