-- 002_add_default_column/down.sql
-- Removes the 'default' boolean column from the models table.
-- SQLite doesn't support DROP COLUMN IF EXISTS, so we wrap in a do-block
-- (SQLite 3.30+) or just let the error be caught.

ALTER TABLE models DROP COLUMN "default";
