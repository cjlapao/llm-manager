### Task

Rewrite the `speech` combined command to match the `rag` command pattern: support up to 3 positional slugs (stt, tts, omni), `--allow-multiple/-m` flag, `--default` flag, and grouped info output.

### Assigned Specialist

golang-engineer

### Parent Feature

feature-speech-models (Issue #27)

### Depends on

Task 002: STT subcommand (for reference on resolveSlug pattern)
Task 003: TTS subcommand (for reference on resolveSlug pattern)
Task 004: Omni subcommand (for reference on resolveSlug pattern)

### Acceptance Criteria

- [ ] `internal/cmd/speech.go` rewritten to follow the `internal/cmd/rag.go` pattern
- [ ] `speech start [--allow-multiple|-m] [<stt-slug> <tts-slug> <omni-slug>]`
  - Accepts 0, 1, 2, or 3 positional slugs
  - `--default` flag selects default models from each provided subtype
  - No args: starts first model from each subtype that has records
  - Calls `c.svc.StartModelBySlugWithAllow(slug, allowMultiple)` for each resolved slug
  - Rolls back started containers on failure (stop previously started ones)
- [ ] `speech stop [--allow-multiple|-m] [<stt-slug> <tts-slug> <omni-slug>]`
  - Same slug resolution as start
  - Calls `c.svc.StopModelBySlug(slug)` for each resolved slug
  - Without `--allow-multiple`: calls `c.svc.StopAllBySubType("speech", <subtype>)` for each subtype that was stopped
  - Handles the case where no containers are running (returns 0, not error)
- [ ] `speech info` groups and displays all speech models by subtype:
  - "STT Models" section with all `Type=speech, SubType=stt` models
  - "TTS Models" section with all `Type=speech, SubType=tts` models
  - "Omni Models" section with all `Type=speech, SubType=omni` models
  - Each model shows: name, slug (with "(default)" if applicable), container, port, status
- [ ] Help text matches the style of `rag.go`'s `PrintHelp()`
- [ ] Error messages reference "speech" subtypes (stt, tts, omni)
- [ ] Build succeeds: `go build ./...` passes
- [ ] Unit tests written for slug resolution with multiple slugs and `--default` flag

### Definition of Done

- [ ] Code implemented following best practices.
- [ ] Unit tests written and passing.
- [ ] Reviewed and approved.

### Status

pending

### Implementation Notes

Reference implementation: `internal/cmd/rag.go` — the key differences:
- Rag has 2 subtypes (embed=embedding, rerank=reranker); speech has 3 (stt, tts, omni)
- Rag uses `StartModelBySlugWithAllow` for start; speech should do the same
- Rag stop without args finds running containers by checking status; speech should do the same
- The `resolveSlug` helper method pattern from embed.go/rerank.go can be reused
- Info output groups by subtype (3 sections instead of 2)
