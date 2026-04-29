-- 002_add_default_column/down.sql
-- Removes the 'default' boolean column from the models table.

ALTER TABLE models DROP COLUMN IF EXISTS "default";
