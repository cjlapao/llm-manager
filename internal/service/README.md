# Service Package

Business logic services that wrap the database layer.

## Target Language

**Go 1.26.2** — Before making any decisions about types, concurrency, or stdlib usage, check Context7 MCP for the latest Go language features and best practices.

## Quick Reference

| File       | Responsibility                                               | Lines  | Priority              |
|------------|--------------------------------------------------------------|-------:|-----------------------|
| service.go | Main orchestrator: models, lite.llm, OpenCode config, port collision, compose gen        | ~1962  | 🔴 Refactor target    |
| litellm.go | LiteLLM proxy integration: HTTP transport, model sync, deployment status           | ~1326  | 🟡 Refactor target    |
| engine.go  | Engine type/version CRUD + hardcoded defaults + import overrides                         | ~973   | 🟡 Refactor target    |
| mem.go     | VRAM estimation from HuggingFace metadata                                                  | ~766   | 🟢 Monitor            |
| model.go   | Model import/export, LiteLLM param building, port resolution                               | ~715   | Part of Step 4 & 5     |
| mem_calculator.go | GPU memory math engine following vLLM specs                                      | ~509   | ✅ Stable             |

### Smaller Files

| File               | Responsibility                                                                 |
|--------------------|-------------------------------------------------------------------------------|
| `config.go`        | `ConfigService` — persistent key-value store with AES-256-GCM encryption (HF_TOKEN etc.) |
| `compose.go`       | `ComposeGenerator` — Docker Compose YAML templating engine                     |
| `types.go`         | Shared types: `StartOverrides`, `ModelProfile`, helper structs                  |
| `validation.go`    | Multi-model coexistence validation (`CanFitDynamic`) + `/proc/meminfo` reader  |
| `profile_discovery.go` | Auto-discover model profiles from HuggingFace API/config.json                |
| `command_generator.go` | `GeneratedFlags`, `ParseExistingFlags`, `MergeFlags` for vLLM CLI args     |

## Dependencies

```
service/ → database/, crypto/, config/          (inner layers)
cmd/api/      → service/                        (outer layers call inward)
```

Dependency rule: **Never reverse direction.** The service layer must never import `cmd/` or `api/`.

## Testing

```bash
# Run all service tests
go test ./internal/service/... -count=1

# Run specific sub-package
go test ./internal/service/... -run TestServiceRunList -v
```

Most tests use real SQLite in-memory databases. After refactoring introduces interfaces for mocking, generated mocks will live in `internal/mocks/`.
