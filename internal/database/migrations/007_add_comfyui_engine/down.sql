-- 007_add_comfyui_engine/down.sql
-- Removes the seeded ComfyUI engine type and engine version.
-- Order matters: delete child (engine_versions) before parent (engine_types).

DELETE FROM engine_versions WHERE engine_type_slug = 'comfyui' AND slug = 'latest';
DELETE FROM engine_types WHERE slug = 'comfyui';
