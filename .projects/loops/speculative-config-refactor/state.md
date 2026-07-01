# State — speculative-config-refactor
<!-- project root (from pwd): /home/cjlapao/code/llm-manager -->
<!-- canonical loop dir: /home/cjlapao/code/llm-manager/.projects/loops/speculative-config-refactor/ -->
<!-- INVARIANTS: absolute paths only, root from pwd; never `..`; never bare-relative; never type /home from memory; never continue if a state write fails -->

goal: Refactor speculative decoding from 3 flat YAML/DB fields into a nested `speculative_config` map with `decoding`, `model`, `num_tokens`, and new `moe_backend` field. Remove speculative decoding validation (any string accepted). Omit unset properties from generation output.

discovery_done: true
plan_done: true
phase: build

todo:
  - "[ ] T1: Refactor parser, model, API, service, CLI, export, tests, migrations, YAML files"
  - "[ ] T2: Verify build + tests pass"

state:
  T1:
    status: building
    retries: 0
    security_rounds: 0
    review_rounds: 0
    issue: ""
    pr: ""
    agent: 5b0a5153-5790-4ee0-b956-b7a04f5fe1a0
  T2:
    status: pending
    retries: 0
    issue: ""
    pr: ""