# llm-manager

Go CLI for managing LLM containers and services.

## Decisions & Notes

- [2026-04-17] Architecture: Established service layer (`internal/service/`) + 7 subcommand packages (`model`, `container`, `service`, `hotspot`, `logs`, `update`, `mem`) extending CLI from 4 to 11 commands. Reason: separate business logic from CLI concerns, improve testability.
- [2026-04-17] Database: RootCommand owns a single `*gorm.DB` connection shared by all subcommands. Reason: prevents connection leaks, ensures consistent error handling. Departure from original pattern where each command opened its own connection.
- [2026-04-17] The `DatabaseManager` interface in `internal/database/manager.go` already had all CRUD methods defined — they were implemented in `sqlite.go` but not wired into CLI commands. The `RootCommand` struct needed a `db` field added (was only `cfg` before) to expose this to the service layer.
- [2026-06-04] Feature #100: Dynamic ComfyUI Container Provisioning — 5 tasks (#101-#105), 6 PRs (#106-#110), merged to `feature/comfyui-provisioning`. Key changes: migration 007 seeds ComfyUI engine type, dynamic Docker Compose YAML generation with x-gpu-service anchor for NVIDIA GPU passthrough, rewritten StartComfyUI/StopComfyUI with volume validation and HTTP health checks, CLI consolidation (flux/3D moved from `llm` to `comfyui` command).
- [2026-06-23] Step 10 — Refactor service layer split: decomposed monolithic `internal/service/litellm_model_ops.go` into three focused files (`litellm_add_update.go`, `litellm_delete_clean.go`, `litellm_lookup.go`) covering write, delete, and read paths respectively. Rationale: single-responsibility per CRUD boundary improves testability and concurrent ownership. Deleted stale artifacts (`mem.go`, `model.go`) that were dead/duplicate code. Build verified clean (go vet + go build, 0 errors).
- [2026-06-23] Step 11 — Converted `service_test.go` pattern to FakeDatabaseManager mocks; wired new focused service methods into internal command package. Stats: 7 files changed, +469/-2500 lines (net reduction ~2 kB). All changes committed (eb07eb2) and pushed to origin/refactor/architecture.
- [2026-06-23] Step 12 — Plan finalized: All 11 architecture refactoring steps complete (Steps 1-8 split monoliths, Step 9 ongoing monitoring, Steps 10-11 mocks/tests). Largest non-test Go files now peak at 654 lines (mem_estimator.go), down from 1962-line service.go god-object. 19 commits ahead of main on refactor/architecture.

