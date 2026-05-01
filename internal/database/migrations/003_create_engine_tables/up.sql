-- 003_create_engine_tables/up.sql
-- Creates engine_types and engine_versions tables, adds engine_version_slug to models.
-- Safe for re-execution: all CREATE/ADD use IF NOT EXISTS or idempotent patterns.

-- engine_types: identity records for inference engine types (vllm, sglang, etc.)
CREATE TABLE IF NOT EXISTS engine_types (
    id TEXT PRIMARY KEY NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    name TEXT,
    description TEXT DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- engine_versions: full Docker recipe per engine type version
CREATE TABLE IF NOT EXISTS engine_versions (
    id TEXT PRIMARY KEY NOT NULL,
    slug TEXT NOT NULL,
    engine_type_slug TEXT NOT NULL REFERENCES engine_types(slug),
    version TEXT NOT NULL,
    container_name TEXT,
    image TEXT NOT NULL,
    entrypoint TEXT DEFAULT '',
    is_default BOOLEAN DEFAULT 0,
    is_latest BOOLEAN DEFAULT 1,
    environment_json TEXT,
    volumes_json TEXT,
    enable_logging BOOLEAN DEFAULT 0,
    syslog_address TEXT DEFAULT '',
    syslog_facility TEXT DEFAULT 'local3',
    deploy_enable_nvidia BOOLEAN DEFAULT 0,
    deploy_gpu_count TEXT DEFAULT '',
    command_args TEXT DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(engine_type_slug, slug),
    UNIQUE(engine_type_slug, version)
);

-- Add engine_version_slug column to models table
ALTER TABLE models ADD COLUMN engine_version_slug TEXT DEFAULT '';
