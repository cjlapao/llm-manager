-- 001_create_schema up.sql
-- Creates all tables and columns needed for llm-manager database.
-- Uses IF NOT EXISTS / safe column additions for upgrading existing DBs.

-- Schema migrations tracking table (must be created first)
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    direction TEXT NOT NULL DEFAULT 'up',
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    applied BOOLEAN DEFAULT 0 NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_schema_migrations_version_applied ON schema_migrations(version, applied);

-- Config table
CREATE TABLE IF NOT EXISTS config (
    id INTEGER NOT NULL DEFAULT 1 CHECK(id = 1),
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS models (
    id TEXT PRIMARY KEY NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    type TEXT NOT NULL,
    sub_type TEXT,
    name TEXT NOT NULL,
    hf_repo TEXT,
    yml TEXT,
    container TEXT,
    port INTEGER NOT NULL,
    engine_type TEXT DEFAULT 'vllm',
    env_vars TEXT,
    command_args TEXT,
    input_token_cost REAL DEFAULT 0,
    output_token_cost REAL DEFAULT 0,
    capabilities TEXT,
    lite_llm_params TEXT,
    model_info TEXT,
    litellm_model_id TEXT,
    litellm_active_aliases TEXT,
    litellm_variant_ids TEXT,
    base_image_id TEXT DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(slug)
);

CREATE TABLE IF NOT EXISTS containers (
    id TEXT PRIMARY KEY NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    name TEXT,
    status TEXT NOT NULL DEFAULT 'stopped',
    port INTEGER DEFAULT 0,
    gpu_used BOOLEAN DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(slug)
);

CREATE TABLE IF NOT EXISTS hotspots (
    id TEXT PRIMARY KEY NOT NULL,
    model_slug TEXT UNIQUE NOT NULL,
    active BOOLEAN NOT NULL DEFAULT 1,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(model_slug)
);

CREATE TABLE IF NOT EXISTS base_images (
    id TEXT PRIMARY KEY NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    name TEXT,
    engine_type TEXT,
    docker_image TEXT,
    entrypoint TEXT,
    environment_json TEXT,
    volumes_json TEXT,
    composed_yml_file TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(slug)
);
