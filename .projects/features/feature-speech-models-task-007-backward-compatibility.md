### Task

Ensure existing `speech start`/`stop` behavior still works as a backward-compatible fallback when no DB records exist for speech models.

### Assigned Specialist

golang-engineer

### Parent Feature

feature-speech-models (Issue #27)

### Depends on

Task 005: Combined speech subcommand (the rewritten speech command needs to fall back)
Task 006: Service layer (service methods must coexist with the old approach)

### Acceptance Criteria

- [ ] Existing `speech start` with no DB records falls back to profile-based compose:
  - Reads `docker-compose.yml` from `InstallDir`
  - Runs `docker compose -f <InstallDir>/docker-compose.yml --profile speech up -d whisper-stt kokoro-tts`
  - Outputs a deprecation notice: "Profile-based speech start is deprecated. Import speech models with 'llm-manager models import' for per-model management."
- [ ] Existing `speech stop` with no DB records falls back to stopping hardcoded containers:
  - Stops `whisper-stt` and `kokoro-tts` containers if running (via `docker stop`)
  - Outputs a deprecation notice matching start
- [ ] When DB records exist for speech models, the new DB-driven path is used (no fallback)
- [ ] The `--default` and `--allow-multiple` flags are ignored when falling back to profile-based compose (with a warning)
- [ ] The deprecation notice appears only once (or is configurable), not on every invocation
- [ ] Build succeeds: `go build ./...` passes
- [ ] Manual verification:
  - With no speech DB records: `speech start` uses profile-based compose and works
  - With speech DB records: `speech start` uses DB-driven compose
  - Existing `docker-compose.yml` with `--profile speech` is not modified or removed

### Definition of Done

- [ ] Code implemented following best practices.
- [ ] Unit tests written for fallback detection (no DB records → profile path)
- [ ] Manual verification completed
- [ ] Reviewed and approved.

### Status

pending

### Implementation Notes

The fallback logic goes in the rewritten `speech.go` command (Task 005):

```
// In runStart:
speechModels, err := cfg.db.ListModelsByTypeSubType("speech", "stt")
if err != nil || len(speechModels) == 0 {
    // No DB records — fall back to profile-based compose
    return c.runStartProfile()
}
// Otherwise use DB-driven path
```

The existing `StartSpeech()` and `StopSpeech()` methods in `service.go` can be kept as-is for the fallback path. They should be marked as deprecated in comments.

Deprecation notice format:
```
Note: Profile-based speech management is deprecated. 
Import speech models with 'llm-manager models import' for per-model management.
```
