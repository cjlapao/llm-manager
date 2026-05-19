# Feature: GPU Memory Check and Docker Compose Flag Generation for RAG Models

## Status

**Phase**: Pipeline Ready — Feature Branch Created  
**Feature Issue**: [#29](https://github.com/cjlapao/llm-manager/issues/29)  
**Feature Branch**: `feature/rag-gpu-memory` (from `main`)  
**Parent Issue**: #8 — Model Architecture Profiles & Auto GPU Memory Calculation  
**Created**: 2026-05-18

## Pipeline State

| Step | Action | Status |
|------|--------|--------|
| 1. Intake | User request received | Done |
| 2. Feature Issue | Created [#29](https://github.com/cjlapao/llm-manager/issues/29) | Done |
| 3. Task Breakdown | Tasks #30, #31, #32 created and linked | Done |
| 4. Feature Branch | `feature/rag-gpu-memory` created from `main` | Done |
| 5. Implement | Tasks ready for assignment | Pending |
| 6. Review | Pending task completion | Pending |
| 7. Merge to feature | Pending task PRs | Pending |
| 8. User verification | Pending feature complete | Pending |
| 9. Merge to main | Pending user sign-off | Pending |

## Task Issues

| # | Task | Specialist | Depends on | Status |
|---|------|-----------|-----------|--------|
| [#30](https://github.com/cjlapao/llm-manager/issues/30) | Extend `checkGPUMemory()` to include RAG models | `backend-developer` | none | pending |
| [#31](https://github.com/cjlapao/llm-manager/issues/31) | Add memory estimates to `rag info` output | `backend-developer` | #30 | pending |
| [#32](https://github.com/cjlapao/llm-manager/issues/32) | Add unit tests for RAG encoder memory calculation | `test-automator` | none | pending |

## Dependency Graph

```
#30 (backend-developer) ──┐
                           ├──▶ #31 (backend-developer)
#32 (test-automator)  ─────┘
```

- **#30 and #32** can be executed **in parallel** — neither depends on the other.
- **#31 depends on #30** — the `rag info` memory display needs the RAG type gate enabled first.

## Description

LLM models (chat, auto-complete) already have GPU memory pre-flight checks and on-the-fly docker-compose flag generation via the `checkGPUMemory()` function and `mergeProfileFlagsWithOptions()` compose integration. This system calculates how much GPU memory a model needs, checks if it fits in current free memory, and injects the correct `--gpu-memory-utilization`, `--max-model-len`, `--max-num-batched-tokens`, and `--max-num-seqs` flags into the generated docker-compose YAML.

RAG models (embeddings and rerankers) currently bypass this entire system. The `checkGPUMemory()` function explicitly skips models where `model.Type != "llm" && model.Type != "auto-complete"`. As a result, RAG models start without any GPU memory validation, and their compose files are generated without the dynamically calculated vLLM flags.

This feature extends the GPU memory check and docker-compose flag generation to RAG models (`type: rag`), enabling the same safety guarantees that LLMs enjoy.

## Current State Analysis

### What LLMs Do (Working)

1. **`checkGPUMemory()`** in `service.go:717` — runs pre-flight check via `CanFitDynamic()`
2. **`mergeProfileFlagsWithOptions()`** in `compose.go:16` — builds profile, calculates memory, generates flags, merges into compose
3. **`CalculateMemory()`** in `mem_calculator.go:75` — computes all 8 memory components
4. **`CanFitDynamic()`** in `validation.go:92` — checks against `/proc/meminfo` free memory
5. **`GenerateFlags()`** in `command_generator.go:18` — produces `--gpu-memory-utilization`, `--max-model-len`, `--max-num-batched-tokens`, `--max-num-seqs`
6. **Docker compose** includes dynamically calculated flags in the `command:` block

### What RAGs Do (Current Gap)

1. **`checkGPUMemory()`** skips RAG models — line 722 in `service.go`:
   ```go
   if model.Type != "llm" && model.Type != "auto-complete" {
       return nil // only check for LLM models
   }
   ```
2. **`mergeProfileFlagsWithOptions()`** — this function has NO type gate! It works generically for any model with `total_params_b` and `quant_bytes_per_param`. However, it's only called from `ensureComposeWithOptions()`, which IS called for RAG models via `StartModelBySlug` → `ensureCompose`. So compose flag generation might actually already work for RAGs — the main gap is the pre-flight check.
3. **No pre-flight check** — RAG models can be started even if GPU is full.
4. **No memory estimates shown** in `rag info`.

### Important Discovery

The `mergeProfileFlagsWithOptions()` function in `compose.go` does NOT have a type gate. It checks for `model.TotalParamsB == nil || model.QuantBytesPerParam == nil` (both are set on RAG models from YAML). So **compose flag generation may already work for RAG models** — we just need to verify.

The main gap is:
1. The `checkGPUMemory()` type gate (the pre-flight check)
2. `rag info` showing memory estimates

## Memory Calculation for RAG Models

RAG models are encoder models (0 attention layers). The `CalculateMemory()` already handles this path correctly:

| Component | Formula | 0.6B Embed (BF16) |
|-----------|---------|-------------------|
| Weights | `0.6 × 2.0 × 1024` | 1,228 MB |
| KV Cache | 0 (no attention layers) | 0 MB |
| GDN State | 0 (no GDN layers) | 0 MB |
| Prefix Cache | standard | 1,024 MB |
| MTP | 0 (no MTP support) | 0 MB |
| CUDA Context | encoder path (1500 MB) | 1,500 MB |
| Off-Budget | encoder path (500 MB) | 500 MB |
| Vision | 0 | 0 MB |
| **Total** | | **4,252 MB** |

- `gpu_memory_utilization` = 4252 / 121856 = 0.0349 → rounded to **0.04**
- `docker_limit_gb` = ceil(4252 × 115 / 102400) = ceil(4.77) = **5 GB**

## Acceptance Criteria

### AC1: GPU Memory Check for RAG Models
- `checkGPUMemory()` in `service.go` is extended to include `type: rag` in its type gate (currently only `llm` and `auto-complete`).
- The check uses the same `CanFitDynamic()` call path as LLMs with the model's profile data from the DB.
- RAG models with profile data (`total_params_b` and `quant_bytes_per_param` set) trigger the check.
- RAG models without profile data skip the check gracefully (non-fatal, same as LLMs without profile data).

### AC2: Correct Memory Calculation for Encoder Models
- The `CalculateMemory()` function already handles encoder models correctly (0 KV cache, 1500 MB CUDA context, 500 MB off-budget when `attention_layers == 0`).
- Verify that the existing encoder-path formulas produce accurate estimates for typical RAG models.
- No changes to `CalculateMemory()` logic needed — the encoder path is already correct.

### AC3: Compose Flag Generation for RAG Models
- Verify that `mergeProfileFlagsWithOptions()` already generates flags for RAG models (no type gate exists).
- When a RAG model's compose is generated, it should include:
  - `--gpu-memory-utilization` calculated from `totalRealistic / TotalGPUMB` (should be very low, ~0.04 for 0.6B models)
  - `--max-model-len` from the model's `default_context` or `max_context`
  - `--max-num-batched-tokens` derived from off-budget (should be 8192 for encoder models)
  - `--max-num-seqs` defaults to 1

### AC4: Error Messages and Warnings Match LLM Format
- When a RAG model doesn't fit, the error message format matches the LLM format.
- Tight memory warnings also use the same format.

### AC5: `rag info` Shows Memory Estimates
- The `rag info` command displays the calculated memory estimate alongside model status.
- Shows: total estimated VRAM, breakdown (weights, CUDA context, off-budget), and whether the model currently fits on the GPU.

### AC6: Multi-Model Validation
- Starting a RAG model when an LLM is already running validates that combined memory fits.
- The existing `CanFitDynamic()` already accounts for current free memory, so this works automatically once `checkGPUMemory()` is enabled for RAG.

## Implementation Plan

The investigation revealed that the gap is smaller than initially assumed:

### File 1: `internal/service/service.go` — The primary change
**Line ~722**: Expand the type gate in `checkGPUMemory()`:
```go
// BEFORE:
if model.Type != "llm" && model.Type != "auto-complete" {
    return nil // only check for LLM models
}

// AFTER:
if model.Type != "llm" && model.Type != "auto-complete" && model.Type != "rag" {
    return nil // only check for LLM and RAG models
}
```
That's it. This single line change enables the pre-flight GPU memory check for all RAG models.

### File 2: `internal/cmd/rag.go` — Enhanced `runInfo()`
Add memory estimate display to the `rag info` output for each model:
- Call a helper function to calculate memory from the model's profile data
- Display total estimated VRAM, weights, CUDA context, off-budget
- Optionally show whether the model currently fits

### File 3: (possibly) Tests
Add unit tests in `mem_calculator_test.go` for the encoder path with RAG-like profiles (0 attention layers, 0.6B params).

## Files Changed (Expected)

| File | Change |
|------|--------|
| `internal/service/service.go` | Extend `checkGPUMemory()` type gate to include `rag` |
| `internal/cmd/rag.go` | Add memory estimates to `rag info` output |
| `internal/service/mem_calculator_test.go` | Add encoder-path test cases for RAG models |

## Non-Goals

- Real-time GPU memory monitoring during RAG inference
- Automatic context-length reduction for RAG models when memory is tight
- Support for non-vLLM RAG engines
- Changes to the RAG CLI interface (no new subcommands or flags)

## Related Issues

- **#8** — Model Architecture Profiles & Auto GPU Memory Calculation (parent feature)
- **#27** — Feature: Manage speech models (STT/TTS/Omni) like RAG models (references RAG pattern)
