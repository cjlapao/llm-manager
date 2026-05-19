### Task

Create the `tts` command with `start`, `stop`, and `info` subcommands matching the RAG rerank pattern.

### Assigned Specialist

golang-engineer

### Parent Feature

feature-speech-models (Issue #27)

### Depends on

Task 001: Investigation (findings should inform any speech-specific compose behavior)

### Acceptance Criteria

- [ ] New file `internal/cmd/tts.go` created following the exact pattern of `internal/cmd/rerank.go`
- [ ] Command registers via `RegisterCommand("tts", factory)` in `init()`
- [ ] `tts start [--default] [<slug>]` resolves slug via `resolveSlug()` and calls `c.svc.StartModelBySlug(slug)`
- [ ] `tts stop [--default] [<slug>]` resolves slug via `resolveSlug()` and calls `c.svc.StopModelBySlug(slug)`
- [ ] `tts info` lists all models with `Type=speech, SubType=tts` from DB with structured output (name, slug, container, port, status)
- [ ] `resolveSlug()` queries `c.cfg.db.ListModelsByTypeSubType("speech", "tts")` — NOT "rag"/"reranker"
- [ ] `--default` flag selects the model with `Default=true` from the speech/tts subset
- [ ] No args selects the first model from DB (preferring default)
- [ ] Help text matches the style of `rerank.go`'s `PrintHelp()`
- [ ] Error messages reference "TTS" not "rerank"
- [ ] Build succeeds: `go build ./...` passes
- [ ] Unit tests written for `resolveSlug()` and argument parsing

### Definition of Done

- [ ] Code implemented following best practices.
- [ ] Unit tests written and passing.
- [ ] Reviewed and approved.

### Status

pending

### Implementation Notes

Reference implementation: `internal/cmd/rerank.go` — copy the structure and change:
- SubType filter: `"reranker"` → `"tts"`
- Type filter: `"rag"` → `"speech"`
- Error messages: "rerank" → "TTS", "rag/reranker" → "speech/tts"
- Help text examples
