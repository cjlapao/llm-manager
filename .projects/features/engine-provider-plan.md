# Plan: Engine Provider-Based Compose Generation (Engine-Type Level)

## Problem

1. The compose generator always adds vLLM-specific CLI flags (`--max-model-len`, `--gpu-memory-utilization`, etc.) to the `command:` section, regardless of which engine is actually being used.
2. For non-vLLM engines (e.g., custom TTS engines), these flags are meaningless and cause runtime errors because the entrypoint is empty — Docker tries to execute `--max-model-len` as a command.

## Solution

Introduce an `engine_provider` field on **engine types** (not versions). All versions of the same engine share the same provider. This controls both flag generation and compose rendering behavior.

---

## 1. Database Schema Change

### File: `internal/database/models/engine.go`

Add new field to `EngineType` struct (NOT EngineVersion):

```go
type EngineType struct {
    // ... existing fields: Slug, Name, Description ...
    Provider   string `gorm:"size:32;default:'custom';column:provider"`
}
```

Valid values: `"vllm"`, `"sglang"`, `"llama.cpp"`, `"custom"`
Default: `"custom"`

Add helper function:
```go
func IsValidProvider(p string) bool {
    return p == "vllm" || p == "sglang" || p == "llama.cpp" || p == "custom"
}
```

### File: `internal/migrate/<N>_add_engine_provider.sql`

New migration file:
```sql
ALTER TABLE engine_types ADD COLUMN provider VARCHAR(32) NOT NULL DEFAULT 'custom';

-- Backfill: set known vllm engines from image detection
UPDATE engine_types SET provider = 'vllm' WHERE id IN (
    SELECT DISTINCT ev.engine_type_slug FROM engine_versions ev
    JOIN models m ON m.engine_type = ev.slug
    WHERE m.command_args LIKE '%vllm%' OR m.engine_type LIKE '%vllm%'
);
```

All other rows remain `'custom'`.

---

## 2. YAML Import Parser Change

### File: `pkg/yamlparser/engine_parser.go` (or equivalent)

Update `yamlEngine` struct (NOT yamlVersion) to include optional `provider` field:

```go
type yamlEngine struct {
    Slug        string `yaml:"slug"`
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
    Provider    string `yaml:"provider"`  // NEW
}
```

When `ImportEngineFile()` creates the engine type record, use `yf.Engine.Provider` as the provider value. If omitted in YAML → default to `"custom"`. Validate with `IsValidProvider()`.

Example YAML usage:
```yaml
engine:
  slug: qwen3-voice
  name: Qwen3 Voice
  provider: custom  # NEW — tells generator this is NOT vLLM-based

versions:
  - slug: v1
    container_name: speech-node
    image: ghcr.io/cjlapao/qwen3-audio-gb10:latest
    command_args:
      - /app/run-tts.sh
```

For standard vLLM engines:
```yaml
engine:
  slug: qwen-voice
  provider: vllm   # <-- vLLM engine — auto-flags will be added

versions:
  - slug: v1
    ...
```

---

## 3. Service Wiring

### A. BuildComposeConfig needs provider

**File: `internal/service/engine.go`**

In `BuildComposeConfig(model *models.Model)`:
```go
func (s *EngineService) BuildComposeConfig(model *models.Model) (*EngineComposeConfig, error) {
    ev, err := s.ResolveVersionForModel(*model)
    if err != nil {
        return nil, fmt.Errorf("resolve engine version for model %s: %w", model.Slug, err)
    }

    // Get provider from ENGINE TYPE (via engine version → engine type lookup)
    et, err := s.GetEngineTypeBySlug(ev.EngineTypeSlug)
    if err != nil {
        return nil, fmt.Errorf("get engine type for model %s: %w", model.Slug, err)
    }

    cfg := &EngineComposeConfig{
        Image:       ev.Image,
        Entrypoint:  parseEntrypoint(ev.Entrypoint),
        Provider:    et.Provider,          // ← NEW: grab from engine type
        EnvVars:     ev.GetEnvironment(),
        Volumes:     ...
        CommandArgs: ev.GetCommandArgs(),
        LoggingSection: ...,
        DeploySection: ...,
        HealthCheckSection: ...,
        ModelHealthcheckJSON: model.HealthcheckJSON,
        UlimitsSection: ...,
        IPCOverride: ev.IPC,
    }
    ...rest unchanged
}
```

Also update `ShowComposition` method similarly.

### B. EngineComposeConfig struct

**File: `internal/service/compose.go`**

Add `Provider` field:
```go
type EngineComposeConfig struct {
    Image                string
    Entrypoint           []string
    Provider             string          // NEW — from engine type
    EnvVars              map[string]string
    Volumes              []string
    CommandArgs          []string
    LoggingSection       string
    DeploySection        string
    HealthCheckSection   string
    ModelHealthcheckJSON string
    UlimitsSection       string
    IPCOverride          string
}
```

---

## 4. Compose Generation Conditionality

### A. Only add profile flags when provider is `vllm`

Pass `cfg.Provider` into `mergeProfileFlagsWithOptions`:

```go
commandArgs := mergeProfileFlagsWithOptions(cfg.Provider, model, cfg.CommandArgs, overrides)
```

Behavior inside `mergeProfileFlagsWithOptions`:
- **If `provider == "vllm"`**: calculate vLLM profile params, append them after any existing `cmdArgs` (so user-defined args come first, auto-flags come after) ✓
- **If `provider != "vllm"`**: return `existingCmds` verbatim — no modifications ✓

This means:
- Custom/TTS engines get NO automatic modifications ✓ 
- Their `command_args` from YAML are passed through literally ✓
- No `[]entrypoint` or bare flags rendered if nothing defined ✓

### B. Don't render empty entrypoint

Current template renders:
```yaml
entrypoint: [{{range $i, $e := .Entrypoint}}...{{end}}]
```

Which produces `entrypoint: []` for empty arrays — this tells Docker "run nothing plus arguments" which is exactly what breaks things.

Fix: gate the block behind presence check:

```yaml
{{- if len .Entrypoint}}
    entrypoint: [{{range $i, $e := .Entrypoint}}{{if $i}}, {{end}}'{{$e}}'{{end}}]
{{- end}}
```

Only renders `entrypoint:` block if the slice has at least one element.

### C. Don't render empty command

Current template already gates this conditionally! But it's worth keeping explicit:

```yaml
{{- if len .CommandArgs}}
    command: >
{{- range .CommandArgs}}
      {{.}} {{end}}
{{- end}}
```

Already correct — only renders when args are present.

### D. Container defaults

When neither `entrypoint:` nor `command:` appears in compose YAML, Docker falls back to the ENTRYPOINT/CMD defined in the Docker image itself. This is usually exactly what custom engines want — they have their own startup scripts baked into the image.

---

## 5. Data Backward Compatibility

### Existing data:
The migration adds column with `DEFAULT 'custom'`. When importing new engine YAML without `provider:` → stored as `'custom'`. Existing records get `'custom'`.

This means:
- Pre-existing qwen-voice/v1 via old YAML → provider stays `'custom'` → gets NO auto-flags
- Standard vLLM engine with explicit `provider: vllm` in YAML → gets auto-flags ✓
- vLLM engine without explicit `provider:` → gets NO auto-flags (operator must explicitly declare)

**Rationale**: Forcing explicit intent prevents regressions. Every engine developer knows whether they're using vLLM or something else. This is cleaner than inferring from image names.

---

## 6. Template Rendering Rules (Summary)

| entrypoint | command | provider | Result in docker-compose.yml |
|------------|---------|----------|------------------------------|
| `[]` (empty) | `[]` (empty) | any | Neither section rendered → container uses IMAGE defaults |
| `["vllm"]` | `[serve, --flag]` | vllm | Both rendered + auto-profile flags appended |
| `["something.sh"]` | `[]` | custom | Only entrypoint rendered |
| `[]` | `["run-server.sh arg1"]` | custom | Only command rendered |
| `[...]` | `[...]` | non-vllm | Both rendered as-is (pass-through) |

All four scenarios work correctly with this setup.

---

## 7. Edge Cases & Testing

### Test cases:

1. **"vllm provider, no command defined"** → no entrypoint/command sections → container runs default CMD from IMAGE
2. **"vllm provider, has command defined"** → renders entry+command, appends vLLM profile flags (max-model-len, gpu-mem-util, max-num-batched-tokens, max-num-seqs)
3. **"custom provider, no command defined"** → no entrypoint/command sections generated
4. **"custom provider, has command defined"** → renders command as-is, zero modifications from profile calculation
5. **"sglang provider, no command defined"** → same as custom
6. **"llama.cpp provider, has command defined"** → renders command verbatim
7. **Backward compat**: models without engine/provider defined continue to run (container defaults only)

---

## 8. Files Changed

| File | Change |
|------|--------|
| `internal/database/models/engine.go` | Add `Provider` to `EngineType` struct + validation helpers |
| New migration `<N>_add_engine_provider.sql` | `ALTER TABLE engine_types ADD COLUMN provider...` |
| `pkg/yamlparser/engine_parser.go` | Add `provider` to `yamlEngine` struct, parse from YAML |
| `internal/service/model.go` | Pass provider when building EngineType for ImportEngineFile |
| `internal/service/engine.go` | BuildComposeConfig reads EngineType.Provider, forwards to EngineComposeConfig |
| `internal/service/compose.go` | Add `Provider` to EngineComposeConfig, gate profile flags, fix template |
| Tests | Unit tests for each provider x command/entrypoint combination |

---

## Implementation Order

1. **DB schema** — Add `Provider` column to `EngineType` + migration file
2. **YAML parser** — Accept `provider` in `yamlEngine`, validate & store
3. **Service wiring** — `BuildComposeConfig` fetches env type, reads provider
4. **Compose logic** — Gate profile flags by provider, fix template entrypoint gating
5. **Tests** — Verify all provider × config combinations
6. **Data migration note** — Document how operators re-import existing engines
