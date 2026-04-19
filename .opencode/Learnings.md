# Learnings

## llm-manager PR Bug Fixes

### Missing Import — `strings` package (sqlite.go)
`internal/database/sqlite.go` used `strings.Contains()` without importing `"strings"`, causing a compilation failure. **Rule**: When adding new code that uses standard library functions, verify the import exists before committing.

### Bulk-existence check skipped partial migrations (sqlite.go)
`ensureModelColumns()` checked if ANY of 6 columns existed, and if so, skipped adding ALL of them. This meant partial migrations would never complete — once one column was present, the rest were silently ignored. **Rule**: For per-column migrations, check EACH column individually and add only missing ones. Use a `map[string]struct{}` for O(1) lookups of existing columns.

### `os.Args` read inside handler instead of parameter slice (model.go)
`runUpdate()` in `internal/cmd/model.go` read `os.Args` (the full process command line) instead of its parsed `args []string` parameter. Combined with passing only `args[1]` (the slug) instead of `args[1:]` (remaining flags), all key=value updates were silently lost. **Rule**: Handler functions should always use their parameter slices, never `os.Args`. Pass complete remaining args slices (`args[1:]`) to sub-handlers.

## Raw Activity Log

### SQLite + GORM Database Layer (2026-04-17)

1. **`gen_random_uuid()` unavailable in SQLite** — GORM struct tag `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"` caused "SQL logic error: near '('" during AutoMigrate. Root cause: `gen_random_uuid()` is a PostgreSQL/CockroachDB function, not SQLite. The pure-Go SQLite driver (`modernc.org/sqlite`) doesn't support it. Fix: use `gorm:"type:uuid;primaryKey"` and rely on `BeforeCreate(*gorm.DB) error` hooks for UUID generation at the application level.

2. **GORM `default:` tag is DB-level, not Go-level** — Tests expected `Container.Status == "stopped"` and `Hotspot.Active == true` on zero-value structs, but Go zero values are `""` and `false`. GORM `default:` struct tags only apply during database schema creation (AutoMigrate), not to Go struct zero values. Test Go behavior for zero-value structs separately from DB-level defaults.

3. **`NewDatabaseManager()` only creates the struct** — Tests failed with "database not open" because `Run()` called `AutoMigrate()` without calling `Open()` first. Lifecycle: `NewDatabaseManager()` → `Open()` → `AutoMigrate()` → use → `Close()`. Never skip `Open()` before operations.

4. **`runtime.Caller(0)` in tests returns test file path** — Migration test couldn't find `models.json` because it was looking in the wrong directory. Go tests run from the package test directory, not the module root. `runtime.Caller(0)` returns the test file's path (e.g., `internal/database/migration_test.go`), so navigate up 3 levels (`Dir(Dir(Dir(path)))`) to reach the project root. Use `runtime.Caller(0)` to resolve paths relative to the test file location, not the working directory.

### Tier 3–5 CLI Implementation (2026-04-18)

5. **`update` command repurposed from app updates to HF weight pulls** — Original `update` checked for apt/pip updates; the bash script used `hf download` for pulling model weights. Implementation (`internal/cmd/update.go`) takes a model slug or "all", checks `HF_TOKEN` env var, and runs `hf download` with `HF_HOME` set. DB is optional — if nil, returns error early, enabling testing without a real database. **Never Again**: Don't repurpose commands without updating help text and examples; the old help text was completely stale.

6. **Enhanced `model list` with live STATUS + CACHED columns** — `internal/cmd/model.go` `runList()` queries Docker for container status via `docker inspect -f '{{.State.Status}}'` and checks HF cache directory existence. `ContainerService.IsHFCached()` checks `${HFCacheDir}/models--${org}--${name}/snapshots/<snap>/config.json` exists. **Never Again**: Always query live Docker state for the STATUS column — cached DB status is stale.

7. **Enhanced `container status` — comprehensive overview, not per-container** — `internal/cmd/container.go` `runStatusAll()` shows `docker ps` output, active flux/3d/hotspot files. Service aliases (comfyui, embed, rerank, etc.) map to container names and are used by both `logs` and `container status`. The old `status <slug>` showed only cached DB status. New `status` (no args) shows full overview; `status <slug>` handles flux/3D/normal models.

8. **Enhanced `logs` with --follow flag + service name mapping** — `internal/cmd/logs.go` resolves service aliases (comfyui→comfyui-flux, embed→llm-embed, etc.) and model slugs to container names. `-f` flag runs `docker logs -f` for live streaming. `resolveServiceAlias()` is case-insensitive; unknown services print all known aliases. **Never Again**: Always support both model slugs AND service aliases for logs — users think in terms of services, not containers.

9. **`mem [model]` — Port Python VRAM estimator to Go** — `internal/service/mem.go` reads models.json, loads config.json from HF cache or API, computes params and KV cache sizes. Quantization detection: check slug for quant keywords, then yml for `--quantization=X` or `--dtype=X`, default bf16. Parameter formula: `L*H*(nh*hd + 2*kh*hd + nh*hd) + L*ff*H*3*ne + V*H*2 + L*H*4`. KV cache: `kv_bpt * context_length` where `kv_bpt = 2 * L * kh * hd * kv_bytes`. **Never Again**: The Python script was a dependency; Go port eliminates Python requirement for VRAM estimation.

10. **Service layer pattern for memory estimation** — `MemService` uses `ContainerService.IsHFCached()` for cache detection. `FormatVRAM()` and `FormatKV()` are package-level helpers for consistent formatting. Config loading tries local cache first, then HF API — avoids unnecessary network calls.

11. **Flux/3D model activation/deactivation — mutually exclusive via ComfyUI** — `internal/cmd/container.go` `runStart()` and `runStop()` detect flux/3D models and handle specially. Flux models (flux-schnell, flux-dev) stop all LLMs → write ACTIVE_FLUX_FILE → start ComfyUI. 3D models (hunyuan3d, trellis) stop all LLMs → remove ACTIVE_FLUX_FILE → write ACTIVE_3D_FILE → start ComfyUI. Stopping removes the respective active file. **Never Again**: Flux and 3D models share the ComfyUI container but are mutually exclusive — activating one must deactivate the other.

12. **HF Cache Detection — check config.json in snapshot, not just directory** — `internal/service/mem.go` `IsHFCached(hfRepo string)` converts repo to cache dir format and checks for config.json. `Qwen/Qwen3.6-35B-A3B` → `models--Qwen--Qwen3.6-35B-A3B/snapshots/<snap>/config.json`. **Never Again**: Always check for config.json in the snapshot directory, not just the directory existence.

13. **General CLI patterns** — (a) DB nil safety: many commands accept nil DB for testing; always check `if c.db == nil` before DB operations. (b) Service alias resolution: centralize alias→container mapping in a single function; both `logs` and `container` commands use it. (c) Flag parsing: manual flag parsing with `-f`/`--follow` patterns; check for flags before positional args. (d) Error handling: print helpful error messages with known services list when service/model is unknown. (e) Config paths: use `cfg.InstallDir`, `cfg.LLMDir`, `cfg.HFCacheDir` consistently; never hardcode paths. (f) Test patterns: tests that call Docker operations should expect non-zero exit codes; use `t.Log()` for expected failures.
