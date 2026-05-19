### Task

Create the `omni` command with `start`, `stop`, and `info` subcommands matching the RAG pattern for multimodal speech models.

### Assigned Specialist

golang-engineer

### Parent Feature

feature-speech-models (Issue #27)

### Depends on

Task 001: Investigation (findings should inform any speech-specific compose behavior)

### Acceptance Criteria

- [ ] New file `internal/cmd/omni.go` created following the exact pattern of `internal/cmd/embed.go` (single-argument subcommands)
- [ ] Command registers via `RegisterCommand("omni", factory)` in `init()`
- [ ] `omni start [--default] [<slug>]` resolves slug via `resolveSlug()` and calls `c.svc.StartModelBySlug(slug)`
- [ ] `omni stop [--default] [<slug>]` resolves slug via `resolveSlug()` and calls `c.svc.StopModelBySlug(slug)`
- [ ] `omni info` lists all models with `Type=speech, SubType=omni` from DB with structured output (name, slug, container, port, status)
- [ ] `resolveSlug()` queries `c.cfg.db.ListModelsByTypeSubType("speech", "omni")` — NOT "rag"/"embedding"
- [ ] `--default` flag selects the model with `Default=true` from the speech/omni subset
- [ ] No args selects the first model from DB (preferring default)
- [ ] Help text matches the style of `embed.go`'s `PrintHelp()`
- [ ] Error messages reference "Omni" not "embed"
- [ ] Build succeeds: `go build ./...` passes
- [ ] Unit tests written for `resolveSlug()` and argument parsing

### Definition of Done

- [ ] Code implemented following best practices.
- [ ] Unit tests written and passing.
- [ ] Reviewed and approved.

### Status

pending

### Implementation Notes

Reference implementation: `internal/cmd/embed.go` — copy the structure and change:
- SubType filter: `"embedding"` → `"omni"`
- Type filter: `"rag"` → `"speech"`
- Error messages: "embed" → "Omni", "rag/embedding" → "speech/omni"
- Help text examples

Omni models are multimodal speech models (can do both STT and TTS, e.g., Whisper-large-v3, OpenAI Whisper API). They follow the same single-slug pattern as embed (not the dual-slug pattern of rag).
