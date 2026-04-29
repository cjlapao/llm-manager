-- 001_create_schema down.sql
-- Drops all tables created by migration 001

DROP TABLE IF EXISTS schema_migrations;
DROP TABLE IF EXISTS base_images;
DROP TABLE IF EXISTS containers;
DROP TABLE IF EXISTS hotspots;
DROP TABLE IF EXISTS models;
DROP TABLE IF EXISTS config;
