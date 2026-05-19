### Task

Replace hardcoded `StartSpeech`/`StopSpeech` with DB-driven compose generation pattern, add per-subtype stop methods, and implement GPU memory decision from investigation.

### Assigned Specialist

golang-engineer

### Parent Feature

feature-speech-models (Issue #27)

### Depends on

Task 001: Investigation (GPU memory approach and compose pattern findings)

### Acceptance Criteria

- [ ] `ContainerService.StartModelBySlug(slug)` works correctly for speech models (Type=speech)
  - `ensureCompose(model)` generates valid compose YAML for speech models
  - Project name uses consistent pattern (e.g., `speech-{slug}` or `llm-{slug}` — match investigation findings)
  - Compose file written to `LLMDir/{slug}.yml`
- [ ] `ContainerService.StartModelBySlugWithAllow(slug, allowMultiple)` works for speech models
  - When `allowMultiple=false`: calls `StopAllBySubType("speech", model.SubType)` before starting
  - When `allowMultiple=true`: starts without stopping peers
- [ ] `ContainerService.StopModelBySlug(slug)` works for speech models
  - Stops by container name from model record
- [ ] `ContainerService.StopAllBySubType("speech", "stt")` / `("speech", "tts")` / `("speech", "omni")` works
  - Lists running containers, matches against DB records with matching type+subtype
  - Stops matching containers, reports count
- [ ] `ContainerService.GetModelStatus(slug)` works for speech models
  - Returns ModelStatus with container status (running/stopped/unknown)
- [ ] GPU memory pre-flight: speech models are handled per investigation findings
  - If exempted: verify `checkGPUMemory` already skips non-llm/non-auto-complete types (it does — line ~1100 in service.go)
  - If simplified: implement simplified memory check for speech models
  - If full: add speech models to the type check in `checkGPUMemory`
- [ ] No regression: existing LLM, RAG, and other model types still work correctly
- [ ] Build succeeds: `go build ./...` passes
- [ ] Unit tests for speech model start/stop via service layer

### Definition of Done

- [ ] Code implemented following best practices.
- [ ] Unit tests written and passing.
- [ ] Reviewed and approved.

### Status

pending

### Implementation Notes

Key service methods to verify/modify in `internal/service/service.go`:

1. **`ensureCompose(model)`** — already generic, works for any model type. Speech models need `Container`, `Port`, and `EngineType` fields populated in DB records.

2. **`ensureComposeWithOptions(model, overrides)`** — same as above, used by `StartContainer` with CLI overrides.

3. **`checkGPUMemory(slug, overrides)`** — already skips non-llm/non-auto-complete types. Speech models should be exempt (like RAG). No change needed unless investigation recommends otherwise.

4. **`StartModelBySlug(slug)`** — uses `ensureCompose` + compose up with project name `rag-{slug}`. For speech, project name should be `speech-{slug}`. May need a variant or parameter.

5. **`StartModelBySlugWithAllow(slug, allowMultiple)`** — uses `StopAllBySubType(model.Type, model.SubType)`. For speech, this becomes `StopAllBySubType("speech", "stt")` etc. This should work as-is since it uses `model.Type` and `model.SubType`.

6. **`StartSpeech()` / `StopSpeech()`** — these are the current hardcoded methods. They should be deprecated but kept for backward compatibility (see Task 007).

7. **`StopAllBySubType(modelType, subType)`** — already generic, works for any type+subtype combo. `StopAllBySubType("speech", "stt")` will work without changes.
