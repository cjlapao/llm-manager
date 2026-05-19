### Task

Investigate whether speech models (Whisper, Kokoro, etc.) fit the same compose-based pattern as RAG models, determine GPU memory calculation approach, and document findings to inform implementation.

### Assigned Specialist

debugger

### Parent Feature

feature-speech-models (Issue #27)

### Depends on

none

### Acceptance Criteria

- [ ] **Compose pattern analysis**: Review `internal/cmd/speech.go` and `internal/service/service.go` to document the hardcoded `speech start/stop` approach (whisper-stt, kokoro-tts via `docker compose --profile speech` from `docker-compose.yml` in InstallDir)
- [ ] **Engine research**: Identify what inference engines speech models typically run on (Whisper: faster-whisper/whisper.cpp, Kokoro: custom Python server, etc.) and whether the existing compose generator in `internal/compose/` can handle them or needs extension. Specifically check if `EngineService.BuildComposeConfig()` can resolve speech engines or if new engine types need to be registered
- [ ] **RAG pattern comparison**: Compare the RAG compose flow — specifically how `ensureCompose`, slug resolution, DB-driven YAML generation, and `StopAllBySubType` work — and document what would need to change for speech. Key reference files: `internal/cmd/embed.go`, `internal/cmd/rerank.go`, `internal/cmd/rag.go`
- [ ] **GPU memory decision**: Determine one of three approaches:
  - (A) Speech models need the same GPU memory pre-flight check as LLMs
  - (B) Speech models need a simplified calculation (params × quant_bytes_per_param)
  - (C) Speech models should be exempt from GPU memory checks entirely (like RAG currently is — `checkGPUMemory` only runs for `llm` and `auto-complete` types)
  - Document the rationale with specific speech model examples (e.g., Whisper on GPU vs CPU, Kokoro CPU-only)
- [ ] **Backward compatibility assessment**: Document how existing `speech start/stop` behavior (profile-based compose from `docker-compose.yml` in InstallDir) should be preserved — as a fallback path when no DB records exist, or fully replaced
- [ ] **Output**: Create a findings document (comment on this issue) with:
  - Recommended compose pattern for speech models
  - GPU memory approach decision with rationale
  - Recommended backward compatibility strategy
  - Any new data model fields needed (e.g., `inference_engine`, `container_image` per speech subtype, or whether existing `EngineType` field suffices)

### Definition of Done

- [ ] Findings documented with clear recommendations
- [ ] Investigation results shared as a comment on this issue (linked from parent #27)
- [ ] Recommendations are actionable for the implementation tasks

### Status

pending

### Implementation Notes
