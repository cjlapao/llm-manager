# Bug: LiteLLM Sync Creates Duplicate Deployments (Persistent)

**Status:** Open — ongoing investigation
**Severity:** Critical (data integrity, resource waste)
**File:** `internal/service/litellm.go`
**Last Updated:** 2026-04-20

---

## Symptoms

Running `litellm sync <slug>` creates DUPLICATE deployments in the LiteLLM proxy, even when the same slug is synced multiple times. Each sync produces new UUIDs instead of reusing or cleaning up existing ones:

```bash
$ bin/llm-manager litellm sync qwen3.6-nvfp4
Remote model missing in LiteLLM — performing fresh sync
✓ Created fresh deployment for qwen3.6-nvfp4 (c869a4d0-...)
  Created fresh alias "active" (15948fce-...)
  Created fresh variant "thinking" -> qwen3.6-nvfp4-thinking (15e0e1d9-...)
```

After a second sync, the SAME UUIDs are NOT reused — entirely new ones are created instead. The duplicates persist because nothing cleans them before creation.

---

## Root Cause Analysis (Phases Completed)

### Phase 1: Old `cleanupDuplicates()` Was Fundamentally Broken **[CONFIRMED & FIXED]**

**Bug:** `cleanupDuplicates(matchName)` called `GET /models`, found entries where `m.ID == matchName`, then deleted using `{id: m.ID}`. But `GET /models` returns display names/slugs (e.g., `"qwen3.6-nvfp4"`), while `POST /model/delete` requires internal row UUIDs (e.g., `"a1b2c3d4-e5f6-..."`). Sending a slug → HTTP 400 "Model not found".

**Fix Applied:** Replaced with `preCleanupByName(targetName string)`:
```go
func (s *LiteLLMService) preCleanupByName(targetName string) []string {
    models, _ := s.ListModels()        // GET /models returns display-name IDs
    for _, m := range models {
        if m.ID == targetName {         // Match by name ✓
            s.deleteByUUID(m.ID)        // Delete by actual UUID from response ✓
        }
    }
}
```
This correctly extracts real row UUIDs FROM THE LIST RESPONSE and deletes those.

**Status:** Fixed in source code. Calls placed before base creation and each alias/variant creation.

---

### Phase 2: L1 DB-Tracked Cleanup Is Useless When DB Is Cleared **[DISCOVERED DURING TROUBLESHOOTING]**

**Scenario:**
```
SyncModel() detects remote gone → clears ALL DB fields (litellm_model_id, aliases, variants)
         ↓
AddModel(slug) called
         ↓
preCleanupForAdd(): reads DB → ALL EMPTY → returns immediately
         ↓
NO CLEANUP HAPPENS
         ↓
DUPLICATES CREATED
```

My earlier approach (`preCleanupForAdd`) only operated when DB had tracked data. When DB was empty/cleared (common during stale-sync-recovery), it did nothing. This was **Phase 1 of fix** — replacing broken `cleanupDuplicates()`.

**Current Status:** Still relies exclusively on REST scan now via `preCleanupByName()`. This works regardless of DB state.

---

### Phase 3: `preCleanupByName` Is Already Implemented & Called at Every Create Point **[INTEGRATION ISSUE MAY EXIST]**

The following call sites have `s.preCleanupByName(...)` calls wired:

| Create Target | Pre-Cleanup Call | Line |
|---------------|-----------------|------|
| Base model | `cleanedBase := s.preCleanupByName(slug)` | ~L494 |
| Alias loop | `cleanedAlias := s.preCleanupByName(aliasName)` | ~L486 (inside createFreshAlias closure) |
| Variant loop | `cleanedVar := s.preCleanupByName(variantSlug)` | ~L548 |
| UpdateModel variant-new | `cleanedV := s.preCleanupByName(variantSlug)` | ~L715 |

All calls use the CORRECT parameter (the exact model_name being created). All call `deleteByUUID(m.ID)` which sends a valid UUID to POST `/model/delete`.

**Yet duplicates still appear.** This suggests one or more of:

---

## Hypotheses for Persistent Duplication

### H1: CLI Calls Wrong Code Path (Binary Not Rebuilt)
**Probability:** HIGH
**Evidence:** User ran `bin/llm-manager litellm sync ...` but the binary may be stale, built before these fixes were merged into the source tree.
**Fix:** Ensure the binary is rebuilt after all edits. Run `go build ./cmd/llm-manager && cp bin/llm-manager ~/path/to/server/bin/`.

### H2: Server Has Multiple Models With Same Alias/Suffix Name
**Probability:** MEDIUM  
**Scenario:** If Model A has "active" alias and Model B also has "active" alias, `preCleanupByName("active")` will delete BOTH. But that's correct behavior for dedup — it ensures no stale copy remains before creating. This SHOULD work.

### H3: `ListModels()` Response Doesn't Contain Expected Matches
**Probability:** LOW-MEDIUM
**Scenario:** The `GET /models` endpoint might return results where the `id` field doesn't exactly match the display name. Or the list might be paginated/truncated and miss some entries.
**Fix:** Add diagnostic logging to show exactly what `ListModels()` returns.

### H4: `deleteByUUID` Succeeds Silently But No Actual Change Occurs
**Probability:** LOW
**Scenario:** `doRequest()` returns HTTP 200 but the server-side delete didn't actually happen due to a backend bug in LiteLLM proxy.
**Fix:** Log the raw HTTP response from every DELETE request.

### H5: Race Condition During Multi-Step Sync
**Probability:** UNKNOWN
**Scenario:** First sync starts → `preCleanupByName` runs → creates base → next iteration begins → another sync process started elsewhere → race condition → duplicate created.
**Fix:** Single-thread sync operations per model; add idempotency check before each create.

### H6: Variable Shadowing in Loop Causes Stale Closure Capture
**Probability:** POKER'S CHANCE (needs deeper read)
**Scenario:** Inside loops over variantMap or expectedAliases, closure captures reference to iterator variable. After edits, some indentation/variable scope changes occurred that could cause stale capture.
**Fix:** Carefully audit all closure captures in loops.

---

## Current State of Litellm.go (Broken)

The file currently has syntax errors preventing compilation. The broken sections are in the **UpdateModel variant-new path** and partially in **AddModel variant loop**:

### Error Location 1: UpdateModel Line 751
```go
// Around line 751 — "else if dbModel.HasVariants()" appears orphaned
// Likely because closing braces got misaligned during edits
} else if dbModel.HasVariants() {
```

### Error Location 2: AddModel Variant Loop (~Lines 545-568)
The variant block is malformed — the `if len(cleanedVar) > 0 { ` block closes but struct literal and doRequest call follow at wrong indentation level:
```go
cleanedVar := s.preCleanupByName(variantSlug)
if len(cleanedVar) > 0 {
fmt.Printf("  [DEDUP] cleaned %d stale deployment(s) matching '%s'\n", len(cleanedVar), variantSlug)
}

variantDeploy := LiteLLMModel{   ← WRONG INDENT
ModelName:     variantSlug,      ← WRONG INDENT
LiteLLMParams: merged,           ← WRONG INDENT
ModelInfo:     modelInfo,        ← WRONG INDENT
}                                 ← WRONG INDENT
variantBody, vErr := s.doRequest(...)  ← wrong variable name, wrong indent
```

Also, in UpdateModel's variant-new path (~L715-731):
```go
cleanedV := s.preCleanupByName(variantSlug)
if len(cleanedV) > 0 {
fmt.Printf("...")                    ← missing closing brace BEFORE statement below
vBody, vErr := s.doRequest("POST", "/model/new", variantNewDeploy)  ← variantNewDeploy undefined!
```

These syntax errors indicate the file state is inconsistent between edit targets. The `variantNewDeploy` struct definition was lost during one of the `filesystem_edit_file` calls, and closing braces are miscounted.

---

## Recommended Fix Path

### Immediate Action Required
1. **Revert the file to last known good state** (before this session's edits)
2. **Write the corrected version atomically** (single write operation rather than 4+ patch edits)
3. That corrected version would include:
   - `preCleanupByName(targetName string) []string` — REST-scan dedup helper (already correct)
   - `deleteByUUID(uuid string) error` — delegate wrapper (already correct)
   - Single `preCleanupByName(slug)` call in AddModel Step 1
   - Single `preCleanupByName(aliasName)` call inside createFreshAlias closure
   - Single `preCleanupByName(variantSlug)` call in variant loop
   - Proper struct definitions + doRequest calls maintained intact
   - NO broken trailing edits

### Long-Term Investigation (after build passes)
If duplicates still occur WITH clean compilation:
1. Add diagnostic logging to `preCleanupByName` showing: total models returned, count matched, UUIDs deleted
2. Add logging before each `POST /model/new` to verify the model_name being sent
3. After each create, add `s.preCleanupByName(name)` AGAIN as a post-create sanity check
4. Consider whether `POST /model/new` silently succeeds even when a deployment already exists (i.e., LiteLLM may allow duplicate names)

---

## Files Impacted

| File | Impact |
|------|--------|
| `internal/service/litellm.go` | Core service layer — ALL CRUD methods for LiteLLM API |
| `internal/service/litellm_test.go` | Test suite — needs update if function signatures change |
| `internal/cmd/litellm.go` | CLI command — delegates to service methods, should be unaffected |

---

## Related Bugs (Resolved in This Session)

| Bug | Status | Resolution |
|-----|--------|------------|
| `cleanupDuplicates()` used display name as UUID | ✅ Fixed | Replaced with `preCleanupByName()` which scans REST, gets real UUIDs |
| Silent failure on cleanup errors (continued without fixing) | ✅ Fixed | Errors logged with `[DEDUP WARN]` tags, cleanup proceeds |
| DB-tracked cleanup skipped when tracking was empty | ✅ Acknowledged | L1 removed in favor of pure L2 REST scan |
| Aliases never updated in UpdateModel (always fresh-create) | ⚠️ Partially addressed | Aliases use "update-or-create" pattern; ensure L2 catches mismatches |

---

## Next Steps

1. **Fix syntax errors** — restore compilable state
2. **Rebuild + redeploy binary** to server
3. **Run `litellm sync <slug>` twice consecutively** — observe output logs
4. **Verify no duplicates created** after two syncs
5. If duplicates continue → add detailed DIAG logging per hypothesis above

---

## Notes for Future Reference

- The core insight: **"check REST, don't trust DB"** applies to de-dup logic. Local state can diverge; the single source of truth is what's on the server.
- Always pass INTERNAL ROW UUIDs to DELETE endpoints. Display names/slugs ≠ UUIDs.
- Two-tier approach recommended: **L1 (DB-tracked)** for fast deterministic cleanup → **L2 (REST-scan)** for catching anything missed. Both should be applied.
- Keep all closure captures explicit — avoid relying on captured loop variables in Go.
- Write complete functions atomically when possible; incremental patches risk structural drift.
