### Task

Create the `stt` command with `start`, `stop`, and `info` subcommands matching the RAG embed pattern.

### Assigned Specialist

golang-engineer

### Parent Feature

feature-speech-models (Issue #27)

### Depends on

Task 001: Investigation (findings should inform any speech-specific compose behavior)

### Acceptance Criteria

- [ ] New file `internal/cmd/stt.go` created following the exact pattern of `internal/cmd/embed.go`
- [ ] Command registers via `RegisterCommand("stt", factory)` in `init()`
- [ ] `stt start [--default] [<slug>]` resolves slug via `resolveSlug()` and calls `c.svc.StartModelBySlug(slug)`
- [ ] `stt stop [--default] [<slug>]` resolves slug via `resolveSlug()` and calls `c.svc.StopModelBySlug(slug)`
- [ ] `stt info` lists all models with `Type=speech, SubType=stt` from DB with structured output (name, slug, container, port, status)
- [ ] `resolveSlug()` queries `c.cfg.db.ListModelsByTypeSubType("speech", "stt")` — NOT "rag"/"embedding"
- [ ] `--default` flag selects the model with `Default=true` from the speech/stt subset
- [ ] No args selects the first model from DB (preferring default)
- [ ] Help text matches the style of `embed.go`'s `PrintHelp()`
- [ ] Error messages reference "speech/stt" not "rag/embedding"
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
- SubType filter: `"embedding"` → `"stt"`
- Type filter: `"rag"` → `"speech"`
- Error messages: "embed" → "STT", "rag/embedding" → "speech/stt"
- Help text examples
