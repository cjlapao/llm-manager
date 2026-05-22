# Release v0.1.0

## Release Manifest

| Field | Value |
|---|---|
| **Version** | v0.1.0 |
| **Date** | 2026-05-19 |
| **Branch** | `feature/rag-gpu-memory` |
| **Target** | `main` |
| **Commits** | 18 |
| **Files changed** | 10+ |
| **Tests** | All passing (80+) |

## Changelog Highlights
- RAG GPU memory checks and memory estimates
- Profile-level runtime tuning fields (max_num_seqs, max_num_batched_tokens, speculative_decoding, num_speculative_tokens)
- Speculative decoding support with NVFP4 guard
- Versioned migration system with 5 migrations
- Compose flag formatting fix
- Migration idempotency

## Release Artifacts
- `CHANGELOG.md` — full release notes
- Git tag: `v0.1.0`
- Target commit: `ac0ed3f` (HEAD of feature/rag-gpu-memory)

## Deployment Checklist
- [x] All tests passing
- [x] Binary rebuilt with latest changes
- [x] CHANGELOG.md created
- [ ] Git tag v0.1.0 pushed
- [ ] Feature branch merged to main
- [ ] Main branch pushed to origin
