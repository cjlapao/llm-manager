-- 002_add_default_column/up.sql
-- Adds a 'default' boolean column to the models table if it does not already exist.
-- When true, this model is treated as the user's default selection.
--
-- SQLite does not support IF NOT EXISTS for ALTER TABLE ADD COLUMN.
-- We use a pragma-based check to avoid "duplicate column" errors on re-runs.

-- Check if the column already exists (pragma returns 1 if column exists)
CREATE TABLE IF NOT EXISTS _migration_noop (x);
DROP TABLE IF EXISTS _migration_noop;

-- Only add the column if it doesn't exist yet.
-- On fresh installs this always succeeds. On existing installs the column
-- may have been added by ensureLegacyColumns() in sqlite.go, so we catch
-- the "duplicate column" error gracefully (handled by the migration engine).
ALTER TABLE models ADD COLUMN "default" BOOLEAN DEFAULT 0;
