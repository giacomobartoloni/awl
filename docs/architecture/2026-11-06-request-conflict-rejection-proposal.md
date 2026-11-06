# Request-Level Concurrent Conflict Rejection — Behavioral Contract and Minimal Git Design Proposal

**Status:** proposed design for review, not implemented, verified, or an ADR.  
**Date:** 2026-11-06.  
**Product:** AgentWorklog CLI (`awl`), not the pre-existing AgentWorklog Markdown convention set.  
**Owner-confirmed behavior:** 2026-11-06 discussion: a concurrent conflict that `awl` cannot safely resolve must be returned to the coding agent; **no commit is made for the rejected request, including no audit-only commit**; the agent rereads and submits revised changes.  
**Baseline:** [Core requirements](../requirements/core-requirements.md), [document/operation contract](../requirements/document-operation-contract.md), [Git hub architecture](2026-10-23-local-first-storage-evaluation.md).  
**Other proposals:** [PR #2](https://github.com/giacomobartoloni/awl/pull/2) and [PR #3](https://github.com/giacomobartoloni/awl/pull/3) remain distinct draft proposals.  
**Worklog provenance:** `AgentWorklog/awl/Specs/2026-11-06-concurrent-git-audit-design-spec.md` (revised commit-message audit) and `AgentWorklog/awl/Specs/2026-11-06-audit-consistency-and-crash-recovery-research.md`.

## 1. Scope and decision status

**Confirmed observable requirements:**

1. `awl` must not silently overwrite ambiguous concurrent edits or choose the last writer when the same Markdown/YAML content is modified incompatibly.
2. If a **request-level concurrency/publication conflict** cannot be resolved safely, the **whole request fails** without publishing any staged document changes **or any Git commit for that rejected request**. The coding agent receives enough context to reread affected documents and submit an updated request. No audit-only commit is permitted on this rejection path.
3. The CLI handles internal Git revision checks; the mutation caller need not supply a document revision ID. Reads can still be explicitly revision-pinned when requested.
4. A composite **continues after ordinary member-level failures** and can publish its successful members in one content-changing Git commit. The new whole-request conflict rule is a **separate fatal boundary**, not a repeal of continue-on-member-error.
5. Local commit acceptance and later Git remote publication are distinct. A remote rejection **must not roll back a locally acknowledged request**.
6. Keep user-owned Markdown, arbitrary YAML frontmatter, links and other pre-existing files unchanged unless their modification is explicitly accepted.

**Proposed implementation direction, not owner-selected mechanism:** stage a request against one immutable Git tree, construct a candidate tree/commit without changing the shared working tree, and publish via one conditional update of the managed project ref. A short cross-process writer lock or bounded compare-and-swap retries may be used if safety tests justify them. A per-request temporary Git branch or micro-worktree remains an **alternative**, not a requirement.

**Decided 2026-11-06:** rejected request-level concurrency conflicts produce **no commit and no durable shared-Git audit record**; the response alone carries rejection details. **Unresolved:** exact public error encoding, bounded retry count, nonconcurrency failed/no-op audit policy, local durability profile, request-id policy, source-Git/worktree bootstrap details, and remote merge guarantees. No implementation, benchmark or crash proof exists.

## 2. Definitions and publication boundary

- **Request:** the complete ordered operation list submitted to `awl`, with correlation/identity.
- **Member failure:** a definite validation or action failure of one operation evaluated against the request's current coherent staged state; it does not modify that operation's document, and later members continue.
- **Concurrent conflict:** the request's starting snapshot/assumptions became invalid through another committed change and its original intent cannot be **proved** safe on the new head, including stale YAML positional edits, contradictory same-target edits, rename/delete interactions and uncertain identity.
- **Request-level abort:** **no Git commit is published or recorded in shared history for the rejected request**, including audit-only commits. Its staged document changes do not become visible. Candidate objects might exist as unreachable temporary Git objects under optimistic staging, but are **not commits in the published history**. In a lock-first publication experiment a conflict should be detected before even constructing a candidate commit object.
- **Locally acknowledged:** the request's final content and complete per-member audit are published and durable under the selected Git/OS profile; a lost CLI response does not negate publication.
- **Remote pending/conflicted:** the request may already be locally acknowledged but not yet integrated into the remote hub. A push rejection is **not** a locally aborted request.

The distinction between a member error and a request-level abort is critical: do not return a `partial_success` for a request whose final publication was rejected; no provisional member result is authoritative before head publication.

## 3. Minimal candidate local algorithm

```text
coding agent -> awl(request_id?, ordered operations)
                 |
       read one immutable managed Git head H
                 |
   evaluate sequentially in isolated staged state
       - ordinary per-member failures continue
       - preserve unchanged Markdown bytes
                 |
        candidate documents + compact commit-message audit
                 |
             verify current managed ref
              /                 \
          still H         advanced to H'
             |                 |
          guarded CAS     reread/revalidate full intent
             |              /          \
         published        safe          unsafe/ambiguous
                            |                  |
                      rebuild candidate     abort request
                      and bounded CAS       error to caller
```

1. Do not give multiple awl processes a writable shared Git index or worktree. In-memory document staging, an isolated index or direct Git tree construction are candidate techniques.
2. Each request evaluates its ordered members against one coherent snapshot; a later member sees earlier successful staged edits. Individual failures do not leak intermediate bytes.
3. Assemble the final audit **in the commit message**, not a new tracked audit file per request. An accepted changing request produces one content-changing commit containing its final member outcomes; no commit contains a fabricated result from an abandoned candidate.
4. Publish with an expected-old-ref guard. If another process moved the ref, never blindly force or apply a stale candidate: reread the new head and re-evaluate the *original* request. Use bounded retries to avoid starvation.
5. Safe retry is allowed only when applicability can be demonstrated (e.g., another agent changed an unrelated document; a targeted body replacement remains unique and semantically safe). If unsure, abort the **whole** request and report reread/retry.
6. A returned request-level conflict must identify known affected project-relative paths and why automatic applicability was rejected; avoid exposing unrestricted managed-store paths, secrets or unverified actor identity.
7. Crash recovery and idempotency must identify whether an audited commit was already published before reporting `aborted` or retrying. Lost-response recovery is not equivalent to a fresh request.

These are testable expectations, not proof that a particular Git library, API, lock strategy or filesystem flush setting meets them.

## 4. Contract of the conflict response and caller retry

Illustrative envelope only; command names, field spellings and exit codes are **not frozen**:

```json
{
  "request_id": "req-illustrative-42",
  "status": "conflict",
  "code": "CONCURRENT_REQUEST_CONFLICT",
  "applied": false,
  "document_changes_published": false,
  "commit_created": false,
  "conflicting_documents": ["Specs/api.md"],
  "retry": "reread_and_resubmit",
  "message": "Another request changed this document; the submitted edit is no longer provably safe."
}
```

- `applied: false` and `commit_created: false` mean this request **did not publish a Git commit at all**, including for audit. No persisted Git audit is promised for the rejected attempt; returned diagnostics can be kept by the caller. A crash/uncertain local ref outcome is a separate status and must never be mislabeled as a definitive no-commit conflict.
- An agent **must reread** current documents and compose a revised operation request; it must not repeatedly submit an obsolete positional patch without reconsidering its intent.
- If a request identifier is stable for idempotency, **a changed payload must not reuse the same identifier**; a distinct retry request should have a new identifier (and may cite the original in an optional correlation field).
- The CLI should not oblige a caller to choose Git merge strategies, manually manage branches, read `.git` metadata or provide revision tokens for mutation.
- A remote-only push/fetch error for a previously acknowledged local change should instead expose `local_success` plus `remote_pending`/`remote_conflict`; do not return `applied: false` or delete the local commit.

## 5. Relationship to ordered composites

| Situation | Existing/applicable behavior | Local document publication |
| --- | --- | --- |
| Member 2 has `TARGET_NOT_FOUND` during normal evaluation; members 1 and 3 are valid | Continue; record all three member outcomes | One commit contains successful changes (1 and 3) |
| Another awl process advances head while request stages; targets are provably independent | Revalidate intent and rebuild safely, within bounded retries | One request content commit if changed |
| Another process changes the same YAML position or Markdown target ambiguously before publication | Fatal **concurrency** conflict; agent rereads/retries | **None** of this request's staged edits are published |
| Local ref publication is indeterminate after crash | Inspect durable history and request identity before reporting an outcome | Do not claim `aborted` if the commit may already be published |
| Local request committed, later remote push rejected | Preserve accepted commit and audit; show pending remote integration | Local success remains valid |

No cross-request ACID rollback or multi-provider distributed transaction is proposed. This is a **single-request, single-local-publication** guarantee.

## 6. Why micro-worktrees are optional, not the contract

A `git worktree` offers isolated `HEAD`, index and checkout, but linked worktrees still share objects and common refs. A throwaway branch/worktree therefore does **not** remove contention on the authoritative managed head. A micro-worktree can be appropriate if concrete patch operations require materialized files, or an embedded Git library proves less maintainable than Git CLI workflows.

A simpler first spike should compare:

- **No-checkout private staging:** read immutable Git tree/blob bytes; apply validated operations in memory or an isolated temporary index; build a candidate commit; publish with guarded ref update.
- **Temporary worktree per request:** materialize a checkout and index, commit on an isolated temporary branch, integrate into the authoritative head, clean up safely after success/conflict/crash.

Both require head revalidation. Prefer no-checkout only if fidelity, path handling, error reporting, multi-process safety and crash tests pass; do not claim performance advantages without measurements. Avoid a merge commit for every local request if a single newly based commit can be safely constructed **before** acknowledgment; do not rebase or rewrite already acknowledged commits during remote integration.

## 7. Audit, remote synchronization and safety boundaries

**Audit:** Recent owner discussion supports a compact, versioned audit **in the Git commit message**, avoiding a tracked file for each request. This remains a *design candidate* and no exact schema is approved. **Owner-selected exclusion:** an unresolved **request-level concurrent conflict returns an error and produces NO Git commit whatsoever for that request**, including no audit-only commit. Its failed attempt is consequently **not** durable in shared Git history. This is an explicit, reviewed exception to the older blanket FR-10 all-attempts wording; successfully published composites still audit each member's success/failure. Whether completed all-failed/no-op **nonconcurrency** requests warrant audit-only commits is a separate open question.

**Remote Git:** Keep existing local-first + authoritative Git hub topology until separately revised. A locally committed request is never discarded because of a rejected remote push. Remote branch divergence can be surfaced as pending/reconciliation-required; only provably safe integration may advance the remote. Do not force-push, automatically choose a winner for same-document ambiguity, or equate local CAS success with remote acknowledgement. Multiple remote branches or a per-agent remote branch model are possible future evaluations, not selected components of this local conflict contract.

**Search/index:** Derived search state does not participate in local publication. A rejected request must not index its uncommitted document bytes as current. Any result served from an older local snapshot must disclose freshness as established elsewhere.

**Retention:** Never delete pending locally acknowledged commits, conflict evidence, or required sync bases as a side effect of cleaning temporary staging. Do not automatically rewrite user-owned Markdown, link targets or Git history.

## 8. Acceptance checks (proposed, not executed)

| ID | Scenario | Pass condition |
| --- | --- | --- |
| RC-01 | Two processes modify distinct documents from one starting head | Both changes survive, or the losing request gets a truthful retryable error; no lost locally acknowledged write |
| RC-02 | Two processes replace exactly the same original span differently | No silent last-writer-wins; one is accepted, the other receives whole-request conflict unless a separately proven safe policy applies |
| RC-03 | Composite changes doc A, then doc B, then doc C; B is normally invalid | Continue and report member failure; accepted content contains A and C only |
| RC-04 | Composite stages A and C, then another process changes B in a conflicting way | Whole-request concurrent conflict; neither A nor C is published by that request |
| RC-05 | Stale exact-target body edit remains uniquely safe after another change | May rebuild and apply once without caller revision input |
| RC-06 | Two writers race for one Git ref | Guarded publication, bounded retry, no forced update and no partial result falsely acknowledged |
| RC-07 | Concurrent YAML insertion shifts a positional target | Reject stale positional intent; do not mutate a neighboring list element |
| RC-08 | Temporary staging fails or process terminates before publication | No partial document effect visible; candidate remains unacknowledged and safely recoverable/garbage-collectable |
| RC-09 | Process terminates immediately after ref publication, before reply | On retry/status query the original audited committed outcome is discovered; no duplicate application |
| RC-10 | Remote push rejected after accepted local request | Local content/audit retained, clear `remote_pending/conflict`, no request-level `applied:false` |
| RC-11 | Agent rereads after conflict and submits a new payload/request ID | Updated intent is evaluated against current snapshot and accepted if safe |
| RC-12 | Rejected concurrent conflict, including a composite with staged successful members | No user document change **and no audit-only or content commit for the rejected request**; commit count and managed ref are unchanged by that request; caller receives conflict details |
| RC-13 | Same source repository invoked from two linked worktrees | Same managed awl repository is used, with independent request staging and safe publication |
| RC-14 | Case-only rename, delete/edit, symlink escape or backlink-sensitive rename | Fail closed on ambiguity; no wrong-document mutation or silent migration |

These scenarios are requirements/experiment proposals, **not executed tests**. Crash-kill injection is not a power-loss durability demonstration.

## 9. Decision gates and YAGNI

- **Owner-confirmed:** return a fatal request-level concurrent conflict without publishing any staged document changes; the coding agent rereads and resubmits. Retain ordinary partial composite behavior.
- **Recommended minimum spike:** one authoritative local Git ref, isolated non-checkout staging if feasible, short conditional publication, bounded safe retries, explicit conflict errors; do not build a dynamic persistence strategy interface.
- **Owner-confirmed:** no commit of any kind for concurrently rejected requests; only the response conveys that rejected attempt. **Open:** audit of completed nonconcurrency all-failed/no-op requests; exact identity/response schema; CAS versus short lock policy; platform durability; proof of safe same-document retries; remote branch policy.
- **Defer:** mandatory micro-worktree per request, general three-way semantic auto-merger, CRDT engine, daemon, database audit authority, provider-neutral local storage, remote per-agent branch fleet and automatic distributed rollback.

## 10. Preliminary isolated Git CLI probe (2026-11-06)

**Evidence status:** A standalone Python harness operated on temporary **bare Git repositories**, not on `awl` application code or user data. It is a deliberately narrow mechanism test, not implementation verification, a platform benchmark, a crash-recovery test or an accepted architecture.

**Fixture and mechanics:** Git **2.47.3**, Python **3.13.5**, local POSIX `fcntl.flock` protecting cooperating writers during final revalidation, Git commit construction and expected-old-ref publication. Two real OS processes each read the same initial Git head before racing. Each request evaluates exact-target replacement operations in memory, uses a private temporary `GIT_INDEX_FILE`, writes a tree and a commit with a compact illustrative `Awl-Request-ID` trailer only after final validation, then advances the managed ref by `git update-ref <ref> <new> <old>`. Each test used a fresh temporary Git repository and asserted actual Git history and document bytes.

| Probe | Observed result |
| --- | --- |
| P1: two concurrent requests edit different Markdown files | Both were published as separate commits; two request commits advanced the local head; both document changes survived |
| P2: one request changes an API line, another attempts a different replacement of the same original line *and* stages an unrelated plan edit | One request was published; the other returned `conflict`; **only one** request commit was added, the rejected request ID was absent from reachable Git history, the plan edit from the rejected request was absent, and the shared head did not advance during its rejection |
| P3: ordered composite with two valid replacements and an invalid middle member | One content commit published the two valid edits, preserved the original middle document, and recorded the middle member failure in the illustrative commit-message audit |

Five sequential full harness runs passed (15 scenario executions). **They do not establish concurrent performance, robustness across operating systems, recovery after a crash, remote push/reconciliation safety, file-format byte fidelity on arbitrary Markdown, YAML pointer safety or correctness of production `awl`.** The private prototype code and machine-readable results were retained as conversation artifacts, not added to this PR; permanent repo inclusion is a separate code-review decision.

**Important applicability limit:** A POSIX writer lock only coordinates cooperating local processes. External ref writers can race; `update-ref` remains necessary. This probe validated rejection **before constructing** the candidate commit while holding the lock. Optimistic commit-before-CAS may leave unreachable Git commit objects even when the rejected request creates no **reachable/published** commit; the owner's zero-commit guarantee is about the authoritative history, and an exact zero-unreachable-object requirement would need additional implementation constraints.

**Next proof gates:** deterministic fault injection before/after commit-object creation and ref publication; durable idempotency lookup after a lost response; platform flush safety; independent writers without/with lock and crash cleanup; real YAML/Markdown patch validation; remote divergence; scale under contention. No benchmarks or power-loss guarantees are claimed.

**Provenance:** Owner discussion on 2026-11-06, GitHub requirements/PRs and the isolated Git CLI probe. This PR modifies documentation, not `awl` product code or an ADR. The mechanism was tested in a standalone harness only.
