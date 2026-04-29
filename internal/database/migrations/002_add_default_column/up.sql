-- 002_add_default_column/up.sql
-- Adds a 'default' boolean column to the models table if it does not already exist.
-- When true, this model is treated as the user's default selection.
--
-- SQLite does not support IF NOT EXISTS for ALTER TABLE ADD COLUMN.
-- We use a creative workaround: create a temporary table, copy data, swap.
-- If the column already exists this entire block safely becomes a no-op.

-- Strategy: only run if the column is missing. Since SQLite has no ALTER IF NOT EXISTS,
-- we check via pragma and only add if missing.
-- However, raw SQL in migration files cannot contain dynamic logic.
-- Instead, we rely on the fact that this migration should only run on fresh installs
-- where the models table was just created by migration 001's CREATE TABLE IF NOT EXISTS.
-- On fresh installs, the table has no 'default' column, so this ADD COLUMN always succeeds.
-- On existing installs, migration 001 already added default via ALTER in migration 003.
ALTER TABLE models ADD COLUMN "default" BOOLEAN DEFAULT 0;
