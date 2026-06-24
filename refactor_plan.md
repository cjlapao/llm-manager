# Refactoring Plan — llm-manager Architecture Restructure

> **Created:** 2026-06-22  
> **Target branch:** `refactor/architecture`  
> **Current step:** All 11 refactoring steps completed. Plan finalized 2026-06-23. 
> **Architecture target:** Package-per-responsibility (split by domain concern, no god-files)
> **Target language:** Go 1.26.2

---

## Quick-Start for Agents

When reading/editing this project, start here:

| Goal | Go to |
|------|-------|
| How does the CLI work? | `internal/cmd/` — each subcommand is one struct + `Run()` method |
| Add a new CLI command | Look at `cmd/config.go` or `cmd/export.go` as minimal templates |
| Main entry point | `main.go` at project root → calls `cmd.NewRootCommand().Execute()` |
| Where's the database setup? | `internal/database/sqlite.go` — SQLiteManager, GORM connection, auto-migrate |
| What models exist? | `internal/models/` — only 4 files: model, engine, hotspot, baseimage |
| Business logic / service layer | `internal/service/` — everything heavy lives here |
| HTTP API endpoints | `internal/api/server.go` (+ handlers for rag, odata, status, hotspots, version) |
| Configuration loading | `internal/config/config.go` — YAML + env overrides |
| Docker Compose generation | `internal/service/compose.go` (generator logic) + templates in service/ |
| Yaml parsing utilities | `pkg/yamlparser/` — standalone, used by config and import/export |

### Before you edit any large file

**Check line counts first.** If a file is approaching ~800 lines, split it before making more changes. The hard rule is **1000 lines per file**.

---

## Architecture Overview

### Current State

```
┌─────────────────────────────────────────────────────────┐
│  cmd/              interfaces/presentation  (CLIs)       │
│  api/              interfaces/presentation  (HTTP)       │
├──────────────────────┬──────────────────────────────────┤
│  service/            application layer   (**OVERLOADED**)│
│  config/             application layer                  │
├──────────────────────┼──────────────────────────────────┤
│  database/           infrastructure (DAO, migrations)    │
│  yamlparser/         infrastructure (utility package)    │
├──────────────────────┴──────────────────────────────────┤
│  models/               domain layer (entity structs)     │
└─────────────────────────────────────────────────────────┘
```

**Dependency direction:** inner layers never import outer layers. Only `service/` touches `database/`. Only `cmd/` and `api/` touch `services/`. This is correct — just needs structural cleanup in the overloaded `service/` package.

### Target Package-per-Responsibility Layout

```
internal/
├── cmd/                          ← CLI commands (keep here, split big ones)
│   ├── root.go                   (RootCommand — shared, small)
│   ├── main.go                   (root subcommands wiring)
│   ├── llm.go                    ← LLM container lifecycle
│   ├── model.go                  ← Model management commands
│   ├── speech.go                 ← Speech commands (STT→_stt.go, TTS→_tts.go, Omni→_omni.go)
│   ├── engine.go                 ← Engine/type/version commands
│   ├── install.go                ← Install command
│   ├── import.go                 ← Import command
│   ├── export.go                 ← Export command
│   ├── compose.go                ← Compose management
│   ├── config.go                 ← Config subcommand
│   └── uninstall.go              ← Uninstall command
│
├── service/                      ← Application logic (**RESTRUCTURE HERE**)
│   ├── README.md                 ← Agent-friendly doc for this package
│   ├── model_service.go          ← Pure model CRUD + port collision resolution
│   ├── litellm_client.go         ← LiteLLM HTTP transport & types (~300 lines)
│   ├── litellm_sync.go           ← LiteLLM deploy sync logic (~400 lines)
│   ├── litellm_merge.go          ← DeepMerge helper (extracted from litellm.go)
│   ├── engine_types.go           ← EngineType CRUD + defaults map
│   ├── engine_versions.go        ← EngineVersion CRUD
│   ├── engine_import.go          ← Import overrides logic
│   ├── mem_estimator.go          ← VRAM estimation + HuggingFace metadata fetch
│   ├── mem_calculator.go         ← GPU memory math engine (keep as-is)
│   ├── model_import.go           ← ImportModel logic extracted from service.go/model.go
│   ├── model_export.go           ← ExportModel logic extracted from service.go
│   ├── opencode_export.go        ← OpenCode config generation
│   ├── compose_provider.go       ← Provider-specific compose templates
│   └── compose_generator.go      ← Core compose generator logic
│
├── api/                          ← HTTP API handlers
│   ├── server.go                 ← Router, middleware, health-check init
│   ├── rag_handler.go            ← RAG model operations
│   ├── odata_filter.go           ← OData filter parsing
│   ├── odata_query.go            ← Query builder
│   ├── status_handler.go         ← Status endpoint
│   └── ...                       ← other handlers
│
├── database/                     ← Infrastructure / DAO
│   ├── sqlite.go                 ← Connection + schema (split: conn/schema vs queries)
│   ├── migration_engine.go       ← Migration orchestration
│   └── models/                   ← Entity structs (GORM tags)
│       ├── model.go
│       ├── engine.go
│       ├── hotspot.go
│       └── baseimage.go
│
├── config/                       ← Configuration loading
│   └── config.go                 ← YAML load + env override merge
│
└── models/                       ← Domain entities (raw GORM structs, no business logic)
    ├── model.go
    ├── engine.go
    ├── hotspot.go
    └── baseimage.go
```

### Key Refactoring Rules

1. **No file exceeds 1000 lines.** Split before hitting 800.
2. **Split by responsibility, not arbitrarily.** Each sub-file should have a single clear purpose.
3. **Extract interfaces where mocking matters.** Use counterfeiter via `go generate` to produce mocks.
4. **Keep existing imports working.** Refactor incrementally — tests must pass after every split.
5. **Do not change public APIs of existing types** unless necessary. Minimize breaking changes.

---

## Large Files Inventory

### Critical (>1000 lines) — Must Fix Immediately

| # | File | Lines | Package | Onion Layer | Priority |
|---|------|------:|---------|-------------|----------|
| 1 | `internal/service/service.go` | 1962 | service | Application | **P0** — God object |
| 2 | `internal/service/litellm.go` | 1326 | service | Application | **P0** — Heavy HTTP client + merge utils mixed in |
| 3 | `internal/cmd/speech.go` | 1026 | cmd | Interfaces/Presentation | **P1** — 3 distinct CLI dispatchers |

### High (>500 lines) — Should Fix Early

| # | File | Lines | Package | Onion Layer |
|---|------|------:|---------|-------------|
| 4 | `internal/service/engine.go` | 973 | service | Application |
| 5 | `internal/database/sqlite.go` | 840 | database | Infrastructure |
| 6 | `internal/service/mem.go` | 766 | service | Application |
| 7 | `internal/cmd/model.go` | 726 | cmd | Interfaces/Presentation |
| 8 | `internal/service/model.go` | 715 | service | Application |
| 9 | `internal/cmd/llm.go` | 623 | cmd | Interfaces/Presentation |
| 10 | `internal/service/mem_calculator.go` | 509 | service | Application |

### Warning Zone (300–500 lines) — Monitor During Refactoring

- `internal/cmd/engine.go` (488) – already large for a CLI handler
- `internal/config/config.go` (470) – functional complexity could grow
- `internal/cmd/install.go` (467) – multi-engine installation paths
- `internal/service/compose.go` (419) – closely related to service.go's compose section

---

## Refactoring Steps

Each step below is a self-contained task. Steps marked `[seq]` are sequential. Steps marked `[par]` can go out in parallel.

### Phase 1: Foundation (non-breaking, safe)

#### Step 1 — Create package README docs [seq]
**Owner:** technical-writer or frontend-developer  
**Scope:** Create `README.md` files inside `internal/service/` and `internal/cmd/` that explain what each file does, why it exists, and how to extend it.  
**Output:** Two README agent-facing docs.

#### Step 2 — Extract deep-merge helpers from litellm.go [par]
**Owner:** golang-engineer  
**Scope:** Extract `DeepMerge`, `deepObjectMerge`, `castToMap`, `stripMetadata` from `litellm.go` into `internal/service/litellm_merge.go`.  
**Test impact:** All existing litellm_test.go tests must still pass.

#### Step 3 — Split sqlite.go into connection/schema + per-domain queries [par]
**Owner:** sql-engineer  
**Scope:** Move individual query methods into separate files like `sqlite_model.go`, `sqlite_engine.go`. Keep `sqlite.go` with only DB connection + auto-migrate.  
**Test impact:** All database tests must pass.

### Phase 2: Core Restructure (focused changes)

#### Step 4 — Split service.go (1962 lines!) [seq — blocks most others]
**Owner:** golang-engineer  
**Scope:** Split service.go (~1962 lines) into:
- `model_service.go` — pure model CRUD + port collision
- `opencode_export.go` — OpenCode config generation
- `port_resolver.go` — port collision resolution algorithm
- `model_import.go` — ImportModel + BuildLiteLLMParams + buildModelInfo
- `model_export.go` — ExportModel + buildExportDir + writeComposeFile

#### Step 5 — Split litellm.go (1326 lines) [seq]
**Owner:** golang-engineer  
**Scope:** 
- `litellm_client.go` — HTTP request/response types, URL building, DoQuery
- `litellm_sync.go` — DeployModelSync, UpdateDeployedModelStatus, ListDeployedModels
- (merge helpers already covered by Step 2)

#### Step 6 — Split engine.go (973 lines) [seq]
**Owner:** golang-engineer  
**Scope:**
- `engine_types.go` — EngineType CRUD + hardcoded DefaultEngineVersions map
- `engine_import.go` — ImportOverrides and custom engine processing
- `engine_versions.go` — EngineVersion CRUD

### Phase 3: CLI Cleanup & Monitoring

#### Step 7 — Split speech.go (1026 lines)
**Owner:** golang-engineer  
**Scope:** Split STT/TTS/Omni dispatch into `speech_stt.go`, `speech_tts.go`, `speech_omni.go`. Shared stubs in `speech_shared.go`.

#### Step 8 — Split llm.go (623 lines)
**Owner:** golang-engineer  
**Scope:** Split lifecycle into `llm_lifecycle.go` and status/logs into `llm_operational.go`.

#### Step 9 — Clean up warning-zone files
**Owner:** various specialists  
**Scope:** Revisit files in the 300–500 range; split if refactoring creates new natural boundaries.

### Phase 4: Test Mocking Infrastructure

#### Step 10 — Set up interface contracts + mockgen tools
**Owner:** golang-engineer  
**Scope:** Define interfaces in `_interface.go` files in each package. Configure counterfeiter via `go generate`. Generate initial mocks for DatabaseManager.

#### Step 11 — Convert critical path tests to use mocks
**Owner:** test-automator  
**Scope:** Pick top 3 integration-heavy test suites and convert to use generated mocks.

---

## Testing Strategy

### Mocking Approach: Counterfeiter

Use `go:generate` comments with counterfeiter to auto-generate mock implementations.

**Setup:**
```bash
cd /home/cjlapao/code/llm-manager
go install github.com/maxbrunecker/counterfeiter/v6@latest
```

**Example — mock the DatabaseManager:**
```go
//go:generate counterfeiter -generate -o ../mocks/mock_database.go . DatabaseManager

type DatabaseManager interface {
    Create(...any) error
    First(...any) error
    FindAll(...any, ...any) error
}
```

**Run mock generation:**
```bash
cd internal/database && go generate ./...
```

### Interface Placement

Every interface lives in the same package as the implementation, suffixed with `_interface.go`.

### Test Execution During Refactoring

After every split, run:
```bash
go test ./... -count=1
```

For fast iteration during a specific package:
```bash
go test ./internal/service/... -count=1
```

If circular imports appear in mocks, use an external `internal/mocks/` directory instead.

---

## Progress Log

Track completed steps here so agents can skip already-done work.

| Date | Step | Status | Notes |
|------|------|--------|-------|
| 2026-06-22 | Planning + scan complete | Done | 10 critical files >500 lines identified |
| 2026-06-22 | Branch `refactor/architecture` created | Done | Pushed to origin |
| 2026-06-22 | **Step 1: Package README docs** | Done | service/README.md + cmd/README.md pushed |
| 2026-06-22 | **Step 2: Extract deep-merge helpers** | Done | litellm_merge.go (86 lines); litellm.go 1326→1241; vet/build/test all pass + full module build |
| 2026-06-22 | **Step 3: Split sqlite.go** | Done | 8 files from 1; sqlite.go 840→228 lines; build clean |
| 2026-06-22 | **Step 4a: Split service.go** | Done | model_service.go (429), orchestrator stubs, cleaned up ~1960-line god-object |
| 2026-06-22 | **Step 4b+5: Split container_services** | Done | container_service.go 839L → lifecycle(552) + operations(247); further split into comfyui/helper_funcs/model_ops (~156 each); commit e8ca9d0 |
| 2026-06-22 | **Step 5b: Split litellm.go** | Done | 1241L → client(382) + deployments(343) + model_ops(532); verified clean. Note: plan had step numbering off vs actual; what was Step 5 in original became Step 6 in practice |
| 2026-06-22 | **Step 6: Split engine.go** | Done | 973L → management(251) + config_builder(459) + yaml_imports(284); commit f472f41 |
| 2026-06-22 | **Step 7: Split speech.go** | Done | 1026L → command_shared(586) + stt/tts/omni (155–156 lines each) |
| 2026-06-22 | **Step 8a: Split mem.go** | Done | 766L → estimator(654) + hf_cache(123); build verified |
| 2026-06-22 | **Step 8b: Split model.go (service)** | Done | 715L → crud_helpers(161) + import(429) + export(148); build verified |
| 2026-06-22 | **Step 8c: Split model.go (CLI)** | Done | 726L → read/write/inspection/stubs (~150–232 lines each); commit 177b0c9 |
| 2026-06-22 | **Step 8d: Split llm.go (CLI)** | Done | 623L → lifecycle(179) + ops(226) + logs-resolvers(183) + stub(60); commit d019728 |
| 2026-06-23 | Step 9: Warning-zone status | Ongoing | Largest non-test Go files now range 459–654 lines (down from 1962/1326 max). No file exceeds 700L except mocks/generated code (mock_database_manager.go 3574L is expected). Step 9 is advisory — split further only when new natural boundaries emerge during feature work. |
| 2026-06-23 | **Step 10: Interface contracts + counterfeiter** | Done | DatabaseManager interface formalized; `go generate` + counterfeiter configured; mock generated at `internal/mocks/mock_database_manager.go`. Commit `e89a0f0`. |
| 2026-06-23 | **Step 11: Convert tests to fake/mock patterns** | Done | Converted critical path tests to use FakeDatabaseManager instead of real DB. Wired new focused service methods into internal command package. Stats: 7 files changed, +469/-2500 lines. Commit `eb07eb2`. |
