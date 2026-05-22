# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-05-19

### Added
- **RAG model GPU memory checks** — pre-flight GPU memory validation for embedding and reranker models, matching LLM behavior
- **RAG memory estimates in `rag info`** — VRAM estimates displayed alongside model metadata
- **Profile-level runtime tuning fields** — `max_num_seqs`, `max_num_batched_tokens`, `speculative_decoding`, `num_speculative_tokens` added to YAML parser, DB model, and compose generation
- **Speculative decoding support** — auto-injects `--speculative-config` when `supports_mtp` is true, with NVFP4 guard and collision prevention
- **Versioned migration system** — SQL migration engine with 5 migrations covering schema, default column, engine tables, model profiles, and runtime tuning columns
- **Legacy column migration** — `ensureLegacyColumns` auto-adds missing columns from pre-migration databases
- **Migration idempotency** — duplicate column errors treated as success for safe re-runs

### Fixed
- **Compose flag formatting** — flags and values now rendered on the same line in docker-compose YAML (`--flag value` instead of separate lines)
- **Encoder model validation** — `num_kv_heads` and `head_dim` now accept `0` (required for embedding/reranker models)
- **RAG container cleanup** — aggressive `docker stop` + `docker rm -f` before compose up eliminates stale container name conflicts
- **Migration 002** — actually adds the `default` column to the models table
- **Migration 005** — properly handles pre-existing columns from `ensureLegacyColumns`

### Changed
- `MergeFlags` output format: flags now combined as `"--flag value"` strings for cleaner compose YAML
- `combineFlagPairs()` helper added as a safety net for uncombined flag pairs from existing command args

### Technical Details
- 18 commits across the feature lifecycle
- 10+ files modified across yamlparser, database, service, and compose layers
- All tests passing (yamlparser 52 tests, command_generator 15 tests, database 12 tests)
