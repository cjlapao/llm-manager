# Discovery — speculative-config-refactor

## Context

The project is a Go application (`/home/cjlapao/code/llm-manager`) managing LLM model deployments. It uses:
- **GORM/SQLite** for the database
- **gorilla/mux** for HTTP API
- **gopkg.in/yaml.v3** for YAML parsing
- **text/template** for docker-compose generation

## Current State

Speculative decoding is managed via 3 flat fields in the YAML profile and corresponding DB columns:

```yaml
profile:
  speculative_decoding: dflash
  speculative_model: z-lab/Qwen3.6-27B-DFlash
  num_speculative_tokens: 2
```

These map to:
- `pkg/yamlparser/parser.go` — `ModelProfile.SpeculativeDecoding`, `NumSpeculativeTokens`, `SpeculativeModel`
- `internal/database/models/model.go` — GORM columns `speculative_decoding`, `num_speculative_tokens`, `speculative_model`
- `internal/api/model_handler.go` — `ModelInfoResponse` JSON fields
- `internal/api/odata_fields.go` — OData white-list columns
- `internal/service/types.go` — `StartOverrides` CLI fields
- `internal/service/compose.go` — `--speculative-config` JSON generation
- `internal/service/model_import.go` — YAML → DB field mapping
- `internal/service/model_export.go` — DB → YAML field mapping
- `internal/cmd/llm_lifecycle.go` — CLI flag parsing (`--speculative-decoding`, `--speculative-tokens`, `--speculative-model`)

Validation currently restricts `speculative_decoding` to `"mtp"` or `"dflash"` only.

`moe_backend` (`speculative_moe_backend` in YAML) exists in some YAML files but is silently ignored — not parsed by any Go code.

## Target State

Replace the 3 flat fields with a nested `speculative_config` map:

```yaml
profile:
  speculative_config:
    decoding: dflash
    model: z-lab/Qwen3.6-27B-DFlash
    num_tokens: 2
    moe_backend: triton
```

Changes needed:
1. Add `SpeculativeConfig` struct with `Decoding`, `Model`, `NumTokens`, `MoeBackend` fields
2. Replace flat fields in `ModelProfile` with `*SpeculativeConfig`
3. Store as JSON column `speculative_config` in DB (alongside `moe_backend` or inside the JSON)
4. Remove validation on `decoding` — any string accepted
5. Remove validation on `num_tokens` — no positive-only constraint
6. Generation JSON includes `moe_backend` when set, omits unset fields
7. Update CLI flags, OData white-list, API response, import/export, tests, migrations, YAML files

## Generation Output

Only fields that are set appear in the `--speculative-config` JSON:
- `{"method":"mtp","num_speculative_tokens":3}` (decoding + tokens only)
- `{"method":"dflash","model":"...","num_speculative_tokens":2}` (full)
- `{"method":"mtp","model":"...","num_speculative_tokens":2,"moe_backend":"triton"}` (with moe)
- If no config set, no `--speculative-config` flag added

## Files to Modify

| File | Change |
|------|--------|
| `pkg/yamlparser/parser.go` | Add `SpeculativeConfig` struct, replace flat fields, remove validation |
| `internal/database/models/model.go` | Replace 3 flat columns with `SpeculativeConfig` JSON + `MoeBackend` or merge into JSON |
| `internal/api/model_handler.go` | Update `ModelInfoResponse` to nested `speculative_config` |
| `internal/api/odata_fields.go` | Update OData white-list |
| `internal/service/types.go` | Update `StartOverrides` |
| `internal/service/compose.go` | Read nested config, add `moe_backend`, omit unset fields |
| `internal/service/model_import.go` | Update field mapping |
| `internal/service/model_export.go` | Update field mapping |
| `internal/cmd/llm_lifecycle.go` | Update CLI flag parsing, add `--moe-backend` |
| `pkg/yamlparser/parser_test.go` | Update tests, remove validation tests, add new tests |
| `internal/api/odata_fields_test.go` | Update column list |
| `internal/database/migrations/` | Create migration 013 |
| YAML model files in `models/` and `import_data/models/` | Refactor to nested format |