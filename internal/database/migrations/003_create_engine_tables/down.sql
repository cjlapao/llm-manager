-- 003_create_engine_tables/down.sql
-- Reverses migration 003: removes engine_version_slug from models, drops engine_versions and engine_types.
-- Safe for partially-upgraded databases: uses conditional checks throughout.
-- Order matters: drop child table first, then parent table.

-- Remove the column we added to models
ALTER TABLE models DROP COLUMN IF EXISTS engine_version_slug;

-- Drop engine_versions (child table, has FK to engine_types)
DROP TABLE IF EXISTS engine_versions;

-- Drop engine_types (parent table)
DROP TABLE IF EXISTS engine_types;
