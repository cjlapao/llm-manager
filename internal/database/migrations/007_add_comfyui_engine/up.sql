-- 007_add_comfyui_engine/up.sql
-- Seeds a default ComfyUI engine type and engine version into engine_types and engine_versions.

-- Seed the ComfyUI engine type (idempotent: INSERT OR IGNORE)
INSERT OR IGNORE INTO engine_types (id, slug, name, description, created_at, updated_at)
VALUES (
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(6))),
    'comfyui',
    'ComfyUI',
    'ComfyUI - A modular, node-based GUI for Stable Diffusion',
    datetime('now'),
    datetime('now')
);

-- Seed the default ComfyUI engine version (idempotent: INSERT OR IGNORE)
INSERT OR IGNORE INTO engine_versions (
    id, slug, engine_type_slug, version, container_name, image, entrypoint,
    is_default, is_latest, environment_json, volumes_json,
    enable_logging, syslog_address, syslog_facility,
    deploy_enable_nvidia, deploy_gpu_count, command_args,
    created_at, updated_at
)
VALUES (
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(6))),
    'latest',
    'comfyui',
    'latest',
    'comfyui-flux',
    'comfyanonymous/ComfyUI',
    '',
    1,
    1,
    '{"CLI_ARGS":"--listen 0.0.0.0"}',
    '{"~/.comfyui/models":"/home/runner/ComfyUI/models"}',
    0,
    '',
    'local3',
    1,
    'all',
    '[]',
    datetime('now'),
    datetime('now')
);
