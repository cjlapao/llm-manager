## Project summary

This project is a cli tool to manage a ai server LLMs lifecycle, including deployment, monitoring, and maintenance. It provides a user-friendly cli tool to import a standard yaml to storage information about a LLM like name, repo, variables and so on. we can then sync, download, deploy, etc. The project is designed to be modular and extensible, allowing for easy integration with different LLM providers and deployment platforms. We will also include a UI at a later stage to visualize and manage the LLMs.

## Agent stack

This project uses the multi-agent stack in `.claude/agents/` plus the skills
in `.claude/skills/`. The entry point for any development request is the
`task-coordinator` subagent.

**Routing rules for the primary Claude Code session:**

- **New feature, bug, refactor, docs, or infra work** → delegate to
  `task-coordinator` via the `Task` tool. It triages into the right mode
  (Pipeline for multi-task features, Direct for single-domain work,
  Continuation for in-flight threads, Question for read-only answers).
- **Specific specialist explicitly requested** (e.g. "use `debugger`") →
  delegate directly via `Task` naming that agent.
- **Pure conversation / clarification / code-reading** → answer inline; no
  subagent needed.
- **Git write operations** (branch, commit, push, PR, merge) → route through
  `github-steward`. Never run these from the primary session.

## Workflow state directory

Workflow state lives in `.opencode/` at the project root:

- `.opencode/feature-{name}.md` — feature descriptions (owned by
  `product-manager`)
- `.opencode/feature-{name}-task-{n}.md` — per-task implementation files
  (owned by `project-manager`, worked by specialists)
- `.opencode/feature-{name}-task-{n}-review.md` — review records (owned by
  `code-reviewer`)
- `.opencode/hotfix-{slug}.md` — hotfix tickets (hotfix-protocol skill)
- `.opencode/qa-bug-{slug}.md` — QA-owned bug reports (qa-audit-protocol
  skill)
- `.opencode/rfc-{topic}.md` — architecture RFCs (architecture-rfc-protocol
  skill)
- `.opencode/debug-investigations/{kind}-{slug}.md` — debugger investigation
  reports
- `.opencode/legal/`, `.opencode/compliance/`, `.opencode/business/`,
  `.opencode/containers/`, `.opencode/kubernetes/`, `.opencode/writing-audits/`
  — advisory reports per domain

Do not edit workflow files directly from the primary session — let the
owning agent write them.

## Canonical operational rules

The canonical Operational Rules block lives in
`.claude/skills/development-map/SKILL.md`. Every specialist inherits it.
Highlights:

- **Visible checklist.** Agents publish a TodoWrite checklist before
  starting any work.
- **Local verification.** Build, test, lint must run before declaring done.
- **FINAL REPORT terminator.** Every agent response ends with a single-line
  parseable report (`FINAL REPORT: status=... files_changed=N
  verification=... notes="..."`). The coordinator reads this to detect
  completion reliably.
- **Never run destructive git ops** outside `github-steward`.
- **Never self-advance dev-map Status** outside your declared role.
- **Redact secrets** before pasting anything into artefacts.

## Two invocation modes

The coordinator picks one at triage:

- **Pipeline mode** — multi-task feature work with file-based handoffs.
  10 steps: triage → feature description → task breakdown → implementation
  → review → revision → approval → merge. Each specialist reads a workflow
  file, transitions a Status field, fills Implementation Notes, and stops.
- **Direct mode** — single-domain ad-hoc work (bug fixes, quick follow-ups,
  targeted questions). The coordinator builds an inline Brief (Context /
  Ask / Known state / What has been tried / Non-goals / Domain /
  Verification / Reporting back) and delegates. No workflow file is
  created.

## Escalation paths

- **Production bug?** → `hotfix-protocol` skill (compressed Pipeline, one PR
  to the release branch, bypasses product/project-manager).
- **Unclear root cause?** → coordinator calls `debugger` first, gets the
  investigation report, then routes the fix to the right specialist with
  the report as context.
- **Cross-cutting architectural decision?** → `architecture-rfc-protocol`
  skill; the coordinator routes to `architect-reviewer` to draft the RFC.
- **Subagent returns without a FINAL REPORT line?** → treat as failed;
  retry with a narrower ask, then escalate to the user with a discrete
  labelled-option question.

## Code conventions

{Fill in per-project preferences: style guide, testing discipline, commit
message format, PR template, review checklists.}

## Known caveats

- Claude Code has no dedicated UI-prompt tool; agents ask clarifying
  questions inline. Expect labelled-option prose replies.
- Per-agent temperature from the opencode originals is not preserved; the
  session temperature governs.
- `model: inherit` is the default on every agent — override per-agent if
  you want to pin cheap models (`haiku`) for rewrite-style tasks or strong
  models (`opus`) for hard design reviews.
