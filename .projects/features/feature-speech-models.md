# Feature: Manage speech models (STT/TTS/Omni) like RAG models

## Description

Speech models (STT, TTS, Omni) are currently managed with a minimal hardcoded approach — a single `speech start`/`stop` pair that drives `docker compose --profile speech` with fixed container names (`whisper-stt`, `kokoro-tts`). There is no per-model control, no DB records, no `info` command, no slug resolution, and no per-subtype isolation.

This feature brings speech model management in line with the RAG model pattern (which already has per-subtype `embed` and `rerank` commands with full slug resolution, DB-backed compose generation, and structured info output). Speech models will be managed identically to RAG models: individually by subtype (`stt`, `tts`, `omni`), combined via a `speech` subcommand, with DB-driven compose file generation, slug resolution, and per-subtype isolation.

## User Story

As a user, I want to manage speech models (STT, TTS, Omni) individually by subtype with the same commands and patterns I use for RAG models, so that I can start/stop specific speech models, see their status, and have them managed through the same DB-driven infrastructure.

## Acceptance Criteria

### AC-1: Individual STT subcommand

Given the `llm-manager` CLI is installed
When a user runs `llm-manager stt start [--default] [<slug>]`
Then the command resolves the slug (default flag, positional arg, or first-in-DB), calls `StartModelBySlug` on the ContainerService, and outputs confirmation.

Given STT models exist in the database with `Type=speech, SubType=stt`
When a user runs `llm-manager stt stop [--default] [<slug>]`
Then the command resolves the slug, calls `StopModelBySlug`, and outputs confirmation.

Given STT models exist in the database
When a user runs `llm-manager stt info`
Then the command lists all STT models with their name, slug, container, port, and status.

### AC-2: Individual TTS subcommand

Given TTS models exist in the database with `Type=speech, SubType=tts`
When a user runs `llm-manager tts start [--default] [<slug>]`
Then the command resolves the slug and starts the model via `StartModelBySlug`.

When a user runs `llm-manager tts stop [--default] [<slug>]`
Then the command resolves the slug and stops the model via `StopModelBySlug`.

When a user runs `llm-manager tts info`
Then the command lists all TTS models with structured info (name, slug, container, port, status).

### AC-3: Individual Omni subcommand

Given Omni models exist in the database with `Type=speech, SubType=omni`
When a user runs `llm-manager omni start [--default] [<slug>]`
Then the command resolves the slug and starts the model via `StartModelBySlug`.

When a user runs `llm-manager omni stop [--default] [<slug>]`
Then the command resolves the slug and stops the model via `StopModelBySlug`.

When a user runs `llm-manager omni info`
Then the command lists all Omni models with structured info (name, slug, container, port, status).

### AC-4: Combined speech subcommand

Given speech models exist in the database
When a user runs `llm-manager speech start [--allow-multiple|-m] [<stt-slug> <tts-slug> [<omni-slug>]]`
Then the command resolves up to 3 slugs (one per subtype), starts each via `StartModelBySlugWithAllow`, and rolls back on failure.

When a user runs `llm-manager speech stop [--allow-multiple|-m] [<stt-slug> <tts-slug> [<omni-slug>]]`
Then the command resolves slugs, stops each, and (without `--allow-multiple`) calls `StopAllBySubType` for each subtype.

When a user runs `llm-manager speech info`
Then the command groups and displays all speech models by subtype (STT, TTS, Omni) with status.

### AC-5: Slug resolution matching RAG pattern

For all subcommands (`stt`, `tts`, `omni`, `speech`):
- `--default` flag selects the model with `Default=true`
- Positional slug selects a specific model by its slug
- No args selects the first model from DB (preferring default)
- `--allow-multiple/-m` flag skips peer-stopping

### AC-6: Per-subtype isolation

When starting one STT model, other running STT containers are stopped first (via `StopAllBySubType("speech", "stt")`).
Same behavior for TTS (`StopAllBySubType("speech", "tts")`) and Omni (`StopAllBySubType("speech", "omni")`).

### AC-7: DB-driven compose generation

Speech models imported via `model import` create DB records with `Type=speech` and appropriate `SubType`.
The compose generator produces per-speech-model YAML files in `LLMDir` (same as RAG).
Container naming follows the pattern from the model's `Container` field.

### AC-8: GPU memory pre-flight handling

Speech models are exempt from GPU memory pre-flight checks (same as RAG models), with documented rationale in the investigation output.

### AC-9: Backward compatibility

Existing `speech start`/`stop` commands continue to work (profile-based compose fallback).
Users with `docker-compose.yml` profile-based speech setup see no breaking changes.

### AC-10: Help text

All subcommands have clear, consistent help text matching the style of `rag`, `embed`, and `rerank` commands.

## Non-Goals

- Adding new inference engines for speech models (the investigation may reveal new engine types, but engine registration is out of scope)
- Real-time speech model streaming or audio I/O
- Speech model benchmarking or performance tuning
- LiteLLM proxy integration for speech models

## Technical Notes

### Existing Codebase Architecture

**Command layer** (`internal/cmd/`):
- Each command registers via `RegisterCommand("name", factory)` in `init()`
- Commands implement `Run(args []string) int` with subcommand switch
- Commands use `service.NewContainerService(root.db, root.cfg)` for service access
- RAG commands: `embed.go` (individual), `rerank.go` (individual), `rag.go` (combined)
- Current speech: `speech.go` — minimal `start`/`stop` with hardcoded names

**Service layer** (`internal/service/`):
- `ContainerService` handles all Docker operations
- `StartModelBySlug(slug)` — starts via `ensureCompose` + compose up
- `StopModelBySlug(slug)` — stops by container name
- `StopAllBySubType(modelType, subType)` — stops all containers matching type+subtype
- `StartModelBySlugWithAllow(slug, allowMultiple)` — starts with optional peer-stopping
- `GetModelStatus(slug)` — returns ModelStatus with container status
- `StartSpeech()` / `StopSpeech()` — current hardcoded profile-based approach
- `ensureCompose(model)` — generates per-model YAML in LLMDir

**Database** (`internal/database/`):
- `ListModelsByTypeSubType(modelType, subType)` — filters by type+subtype
- `GetModel(slug)` — retrieves model by slug
- Model struct has `Type`, `SubType`, `Container`, `Port`, `Default` fields

**Compose generation** (`internal/service/compose.go`):
- `ComposeGenerator` renders Go templates for docker-compose YAML
- `EngineComposeConfig` carries image, entrypoint, env vars, volumes, command args
- `EngineService.BuildComposeConfig(model)` resolves engine version and builds config
- Template produces `services: {serviceName}: image, container_name, ports, environment, command`

**Model schema** (`internal/database/models/model.go`):
- `Type` field: "llm", "rag", "speech", etc.
- `SubType` field: "stt", "tts", "omni", "embedding", "reranker"
- `EngineType` field: defaults to "vllm", but speech may use different engines
- `Container` field: container name used for docker operations
- `Default` field: marks the preferred default model

## Dependencies

- Investigation task must complete first to inform GPU memory approach and compose pattern
- RAG pattern serves as the reference implementation

## Implementation Strategy

### Phase 1: Investigation
Complete the investigation to determine GPU memory approach and compose pattern for speech.

### Phase 2: Individual subcommands
Create `stt.go`, `tts.go`, `omni.go` following the embed.go/rerank.go pattern exactly.
Each registers via `RegisterCommand`, implements `Run()` with start/stop/info subcommands,
and uses `resolveSlug()` for argument parsing.

### Phase 3: Combined speech subcommand
Rewrite `speech.go` to match `rag.go` pattern:
- Parse up to 3 positional slugs (stt, tts, omni)
- Resolve each via subtype-specific DB queries
- Call `StartModelBySlugWithAllow` for each
- Group info output by subtype

### Phase 4: Service layer
Add `StartSpeechBySlugWithAllow` (or reuse `StartModelBySlugWithAllow` with proper type/subtype),
ensure `ensureCompose` works for speech models (may need engine type customization),
add `StopAllBySubType("speech", "stt")` etc.

### Phase 5: Backward compatibility
Keep existing `speech start/stop` as fallback when no DB records exist.

## Status

approved — 7 task files created

## Task Files

| # | File | Specialist | Depends On |
|---|------|-----------|------------|
| 001 | `feature-speech-models-task-001-investigation.md` | debugger | none |
| 002 | `feature-speech-models-task-002-stt-subcommand.md` | golang-engineer | 001 |
| 003 | `feature-speech-models-task-003-tts-subcommand.md` | golang-engineer | 001 |
| 004 | `feature-speech-models-task-004-omni-subcommand.md` | golang-engineer | 001 |
| 005 | `feature-speech-models-task-005-combined-speech-subcommand.md` | golang-engineer | 002, 003, 004 |
| 006 | `feature-speech-models-task-006-service-layer.md` | golang-engineer | 001 |
| 007 | `feature-speech-models-task-007-backward-compatibility.md` | golang-engineer | 005, 006 |

## History

- 2026-05-18: Created and approved — 7 task files decomposed from Issue #27
