# PR #7 Merge-Readiness Review — Experimental Go Local CLI

**Date:** 2026-11-06  
**Review status:** completed static and cross-PR review; **no merge approval**  
**Target:** [PR #7 — Go local init/read/write and audited Git publication](https://github.com/giacomobartoloni/awl/pull/7)  
**Reviewed code head:** `73f357a086f2f01f250ed9377ba3648b5e1a0666` (prior to this review-only document)  
**Target branch:** `main`  
**Product:** AgentWorklog **CLI**, distinct from the existing AgentWorklog Markdown conventions.

## 1. Decision summary

**Not ready to merge as production-capable local persistence.** The prototype is reviewable and has substantial Linux CI evidence, but critical integrity and release assumptions are not established. **Experimental-only merge:** a separately reviewable decision, potentially acceptable after defining the explicit experimental scope, fixing or accepting its bounded risks, reconciling the authority documents, and verifying that it will not be presented as a supported release. This review does **not** approve such a merge or change PR #7's draft status.

A GitHub `mergeable=true` result describes a technical mergeability check, not product/architecture/security approval. Green CI must not be treated as proof of crash durability, cross-platform support, remote synchronization or full requirements coverage.

## 2. Evidence and limits

| Evidence | Verified observation | What it does not establish |
| --- | --- | --- |
| GitHub [PR #7](https://github.com/giacomobartoloni/awl/pull/7) | Open draft, 41 commits and 16 added source/docs/workflow files relative to `main` at inspected head; branch ahead 41 / behind 0 | Production or design approval |
| [CI run 42292946676](https://github.com/giacomobartoloni/awl/actions/runs/42292946676) | Success: `gofmt`, `go vet`, full `go test -race`, black-box `TestCLIContract*`, repeated incompatible-concurrent-request and SIGKILL tests | Power loss, storage faults, real hosted remote, macOS/Windows, complete CLI contract |
| `internal/awl/cli_contract_test.go`, `cli_list_contract_test.go`, `cli_consolidation_contract_test.go` | Actual `awl` subprocess, JSON/exit/path checks, nested-context byte preservation, `ls` and `read` revisions, replay and partial composite outcomes | Every malformed input, broader Markdown/YAML compatibility, full user UX |
| `internal/awl/concurrency_test.go` | Deterministic two-process incompatible same-head conflict test and no **published/reachable** rejected-request commit | All scheduling interleavings, zero *unreachable* Git objects, physical power-cut safety |
| `internal/awl/crash_test.go` | Linux test-only Git wrapper kills real `awl` process before/after guarded `update-ref`; same-ID retry is verified | Filesystem fsync durability, real power outage or arbitrary process/child interleavings |
| Source review of `awl.go`, `config.go`, `gitstore.go`, `documents.go` | Concrete CLI, config, mutation, and Git plumbing boundaries; no speculative provider hierarchy | Formal security audit or proof of all failure handling |

**Verification performed for this review:** GitHub metadata, source text, PR diffs and CI result/step summaries were inspected; no source code changed, no local Go suite or crash test rerun, and no GitHub PR was merged. A local Git clone attempt was blocked by DNS resolution of `github.com`; GitHub-hosted CI is the actual execution evidence.

## 3. Cross-PR reconciliation

| Contract/decision | Authority and state | PR #7 alignment | Required disposition |
| --- | --- | --- | --- |
| Command-first CLI with implicit source-worktree project context | [PR #6](https://github.com/giacomobartoloni/awl/pull/6): owner-selected shape; exact parser/JSON/exit grammar still proposed, PR draft | **Aligned in implemented subset:** `awl <command>`; optional leading `--project` confirms already resolved slug; no `document` prefix, no external lookup | Explicitly confirm that production grammar remains open; do not interpret experimental `--json` or exit numbers as ratified |
| Shared project binding, no per-worktree duplicate | [PR #2](https://github.com/giacomobartoloni/awl/pull/2): draft project/worktree config design | Source Git common-dir binding, distinct bare managed Git repo, linked-worktree tests | Reconcile compatibility and partial-config recovery against the proposed design; inspect canonical config path safety |
| Request-level conflict abort vs ordinary member failures; completed no-change audit-only commit | [PR #4](https://github.com/giacomobartoloni/awl/pull/4): owner-confirmed behavior in unmerged requirements proposal; PR open, not draft | Conditional `update-ref`, ordered composites, audit-only commits, idempotent request IDs; deterministic supporting tests | Preserve the distinction between **no published commit** for an aborted concurrent request and permitted speculative unreachable Git objects. Resolve remaining uncertainty/durability definitions |
| Modular monolith | [ADR 0001 in PR #8](https://github.com/giacomobartoloni/awl/blob/docs/sync-modes-review-2026-11-06/docs/adr/0001-modular-monolith.md): **owner accepted**, but ADR not merged to `main`; broader transport design remains proposed | **Consistent in principle:** one Go CLI, cohesive files inside `internal/awl`, concrete functions, no plugin registry or mandatory interfaces | Integrate/reference the accepted ADR separately; keep code responsibilities cohesive, but do not demand an unapproved package layout |
| Explicit Git sync and independent search index refresh | PR #6 and [PR #8](https://github.com/giacomobartoloni/awl/pull/8); provider/transport details proposed | **Not implemented**; local commits and `remote add` do not initiate network or indexing | Do not present unimplemented commands as a regression against the bounded *experiment*. Do not call PR #7 a complete product |
| Requirements document integration | PR #2 and PR #4 both modify `docs/requirements/core-requirements.md` and `docs/requirements/document-operation-contract.md` | PR #7 adds code/docs only; none of those requirement changes are integrated in `main` | **Reconcile overlapping document edits and approved IDs/behavior in review order**; avoid overwriting or silently combining draft requirements |

**Not a blocker to this bounded experiment:** deferred out-of-source-repository project lookup, QMD, rsync/rclone providers and generic storage interfaces. They are open product scope, not implied first-slice obligations.

## 4. Findings and gates (evidence levels explicit)

### G1 — P0: Acknowledge/power-loss contract (production blocker)

**Observed:** `gitstore.go` stages immutable Git objects, writes a commit and calls guarded `update-ref`. It distinguishes an unchanged head (`GIT_PUBLICATION_FAILED`) and an unobservable head (`PUBLICATION_OUTCOME_UNKNOWN`) after an update error. Linux SIGKILL tests cover two deterministic boundaries.

**Not established:** durable acknowledgement across machine power failure, filesystem/storage errors, commit/ref fsync ordering, or a complete unknown-outcome recovery contract on a selected Linux/Git/filesystem support profile. The current tests must not be reinterpreted as power-cut tests.

**Acceptance before production:** choose a supported environment and persistence guarantee; document Git object/ref flush and recovery assumptions; inject storage/ref failures and ambiguous responses against actual code; test stable request-ID recovery without duplicate accepted effects; declare any unsupported failure modes explicitly.

### G2 — P0/P1: Configuration binding and path-integrity hardening (targeted tests required)

**Source-inspected gap, not an exploit claim:** `resolve` uses `parseLocal(<git-common-dir>/awl/local.yaml)` which calls `os.ReadFile` without the same `Lstat` regular-file guard used by `addRemote`. `initialize` uses `os.Stat` when reusing the existing canonical binding and its `localRepo` target. Both may follow symlinks before resolving the intended canonical boundary. `resolve` and `remote add` do check that the final managed repository directory itself is not a symlink; `validateOptionalLocalLink` separately validates an **optional source-worktree** symlink. These safeguards do **not** automatically verify every canonical-file or ancestor component.

**Acceptance:** adversarial tests for canonical `local.yaml` symlink and type, managed repo path/parent substitution, concurrent config edits and one-sided setup; fail closed without redirecting reads/writes to a different store or overwriting user files. Review parent-directory trust and TOCTOU boundaries, and preserve existing supported optional symlink conventions.

### G3 — P0/P1: Two-file setup/remote recovery (production blocker until bounded)

**Observed:** `remote add` updates source `.awl/project.yaml` and canonical Git common-dir `local.yaml` as independent per-file replacements. It supports explicit same-URL retry after one expected half-update. `init` creates the managed repo, source config and binding in separate steps.

**Not established:** one atomic cross-file transaction, power-loss behavior at every step, or isolation from out-of-band writers. Incomplete initialization can leave orphaned objects/configuration needing a recovery policy.

**Acceptance:** enumerate and test interrupted creation/remote-add states, expected recovery and safe cleanup; avoid silently repairing unrelated data. The intentionally nontransactional experimental scope can be disclosed, but cannot be called atomic.

### G4 — P1: Audit admission, scale and untrusted inputs

**Observed:** `findAudit` scans commit messages and fails closed when history exceeds roughly 5,000 entries. `strictJSON` rejects unknown fields/trailing JSON but **does not reject duplicate object keys**. `git` bounds subprocess output (32 MiB stdout) and lifetime per call; `ls` enumerates the complete managed tree. These are explicit conservative experiment limits.

**Acceptance:** define supported corpus/history/request sizes, profile worst-case latency and failure responses, choose duplicate-key semantics, version/migration strategy for audit and request schemas; add matching CLI-facing and failure tests. Do not replace the bounded scan with a speculative index prematurely.

### G5 — P1: Release scope, portability and CLI grammar

**Observed:** Linux/POSIX `syscall.Flock`, hard-link publication, Go 1.23 and external Git CLI; hosted CI covers Linux. CLI uses proposed `--json`, `--input`, leading `--project`, and experimental numeric exits. `emitError` currently does not propagate a JSON encoder failure's distinct exit code, unlike `emit`; this is a small static code-consistency finding, not a demonstrated production incident.

**Acceptance:** select a support matrix (or explicitly Linux-only experiment), minimum Git version and packaging/upgrade expectation; finalize or explicitly defer parser/JSON/exit contract; add test coverage for output write errors and selected non-TTY failure cases. Windows/macOS and remote tests are not needed to *describe* a Linux prototype, but are needed before claiming those capabilities.

### G6 — P1: Documentation and review integration

**Observed:** PR #7 is a 41-commit experimental addition; PR #2 and PR #4 touch overlapping requirements documents; PR #6 is a draft CLI direction; owner-accepted ADR 0001 is still in draft PR #8. Prior PR body sections retain historical “unverified” statements that later increments partially supersede; their provenance is valid but the current gate summary must be unambiguous.

**Acceptance:** agree on the incremental integration order, reconcile requirements without silently treating proposals as approvals, preserve historical evidence, add a current limitations/decision summary to PR #7 and identify a responsible human reviewer. No green status or draft PR automatically approves those changes.

## 5. Readiness checklist

| Check | State at review | Required next action |
| --- | --- | --- |
| Current branch and CI | **Passed** (inspected head `73f357a`, run `42292946676`) | Recheck on any new head |
| Observable bounded CLI slice | **Evidence present** | Keep black-box contract testing primary |
| Source Git vs managed document store separation | **Evidence present** | Preserve and test under configuration faults |
| Accepted modular-monolith style | **Owner-approved, ADR unmerged** | Integrate ADR documentation without speculative architecture |
| CLI and core requirements cross-PR baseline | **Not integrated** | Reconcile PR #2/#4 overlap, align PR #6 and accepted ADR |
| Crash durability / unknown outcome | **Open production blocker** | Fault tests, support profile and explicit guarantees |
| Config setup/canonical path integrity | **Open targeted security/recovery gate** | Adversarial tests and code review |
| Request audit history and duplicate input | **Open** | Bounded product contract and negative tests |
| Remote sync, QMD, optional satellite transports | **Intentionally out of experimental scope** | Separate feature slices; do not claim present |
| Final public parser/JSON/exit/platform guarantee | **Unselected** | Owner decision before shipping as product |
| Human PR review and merge decision | **Not approved** | Keep PR #7 draft and unmerged |

## 6. Recommended sequence (no implementation authorized by this review)

1. **Document-only reconciliation:** agree on the accepted behavior/requirement IDs from PR #2, #4 and #6, and integrate accepted ADR 0001 from PR #8. Do not merge competing requirement edits blindly. Keep open command/transport questions explicitly open.
2. **Targeted canonical-config/partial-init safety increment:** write black-box/adversarial cases first; fix demonstrated path/type/recovery defects within existing `config.go` boundaries. No new provider interfaces.
3. **Durability and ambiguous publication increment:** select the Linux/Git/filesystem support contract, test injected storage/ref faults, and close or narrow P0 claims before production merge.
4. **Bounded audit/input/CLI hardening:** validate duplicate keys and acceptable history size, expose truthful resource-limit/error outcomes; avoid freezing speculative parser/schema before owner review.
5. **Re-run full CI, peer review, and decide separately** whether to land a clearly marked experiment on `main` or hold it until production gates close.

**Explicit non-action:** This review does **not** merge PR #7, mark it ready for review, alter `main`, implement `sync`, change provider authority, change the accepted ADR, or assert any unexecuted runtime verification.
