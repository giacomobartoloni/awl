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

**Decided 2026-11-06:** rejected request-level concurrency conflicts produce **no commit and no durable shared-Git audit record**; the response alone carries rejection details. Additionally the owner selected **Go with external Git CLI** for the first implementation increment, and **one audit-only commit** for completed mutating requests whose operations all fail ordinarily or make no changes. **Unresolved:** exact public error encoding and audit schema, request admission rules, bounded retry count, local durability profile, request-id policy, source-Git/worktree bootstrap details and remote merge guarantees. No awl implementation, benchmark or power-loss proof exists.

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

**Audit:** The owner selected compact versioned **operation-level audit in the Git commit message** without tracked audit files per request. A **locally completed mutating request** results in **one audit-bearing Git commit**: content+audit when documents change; **audit-only with an unchanged document tree** when all operations fail ordinarily or the completed request is a no-op. Successfully published composites retain every ordered member's outcome. **Exception:** an unresolved request-level concurrent publication conflict produces **NO Git commit**, including no audit-only commit, and its response is not durable shared-Git audit. Read-only operations, pre-execution malformed requests and transient network retries do not create commits under the initial proposed boundary. The exact schema, retention, request admission and crash recovery are still open.

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
- **Owner-confirmed:** no commit of any kind for concurrently rejected requests; only the response conveys that rejected attempt. **Also confirmed:** one audit-only commit for completed mutating all-failed/no-op requests, and Go with external Git CLI for the first implementation increment. **Open:** exact identity/response/audit schema; validation/admission boundary; CAS versus short lock policy; platform durability; proof of safe same-document retries; remote branch policy.
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

## 11. Follow-up: deterministic process interruption and remote divergence (2026-11-06)

**Evidence status:** Separate, bounded **Git CLI fixture**, extending the earlier baseline in §10; this is not an `awl` implementation, durability proof, scale benchmark or remote-provider acceptance test. The fixture source and machine-readable results were retained as a downloadable conversation artifact rather than inserted into this documentation PR.

**Environment/fixture:** Git 2.47.3, Python 3.13.5, POSIX host. All cases create independent temporary **bare Git repositories**; two local bare replicas plus a third bare repository simulate the remote. `SIGKILL` terminates a child process at deterministic checkpoints. Request audit is illustrative and stored in commit messages; a linear request-ID/payload-digest history lookup is used **only for the small test fixture**. No real Git host, network transport or coding-agent API was exercised.

| Probe | Observed result |
| --- | --- |
| P4 — Kill before creating candidate commit | No new reachable commit or audit; managed head unchanged |
| P5 — Kill after `commit-tree` but before `update-ref` | Managed head and reachable history unchanged; the candidate **commit object exists but is unreachable** |
| P6 — Kill after guarded `update-ref` but before response | Commit and audit reachable; retry with same request ID/payload digest identifies prior publication without another commit; same ID with different payload is rejected |
| P7 — Stale expected-old-ref update from another writer | CAS rejects the stale candidate; head remains at winning commit; the rejected candidate may remain an unreachable object |
| P8 — Two independently accepted, **disjoint-file** replica commits; second push rejects | Fetch followed by conservative disjoint integration using `git merge-tree --write-tree`, two-parent commit and fast-forward push succeeds; both original audit commits remain ancestors of remote head; status can be confirmed from remote ancestry |
| P9 — Two independently accepted commits touching the **same** document; second push rejects | Conservative policy declines merge; both local and remote committed versions remain intact; no force push or overwrite |

**Repetition:** five consecutive full extended runs passed all six scenarios: **30/30 scenario executions**. This result covers the six new probes only. Combined with §10, **45/45 executions across nine distinct scenarios** were observed in the two bounded test suites. These counts do not imply statistical reliability at high concurrency.

**Safety interpretation:**

- The owner-selected **no-commit-on-rejected-concurrent-request** guarantee is an observable **published/reachable managed history** guarantee in this candidate. A failed ref CAS or a crash between commit-object creation and publication can leave an **unreachable candidate Git commit object**. A literal prohibition on constructing even unreachable objects requires tighter staging/locking; do not conceal this distinction.
- A crash **after** the ref advances may have published a successful request even when the caller sees no reply. Verify reachable request ID, submitted payload digest and the final outcome before returning a definitive result or attempting replay. A content digest does not authenticate a caller or guarantee durable storage after power loss.
- A remote push rejection is **not** a local request rejection. The locally accepted commit and audit remain retained for later reconciliation; conflict reporting must not set local `applied=false`.
- The remote success in P8 is **local bare-Git transport only**, with disjoint paths and a trivial merge. No general same-file Markdown/YAML merge safety or hosted-remote correctness is established.

**Remaining gates:** power-loss/fdatasync/fsync behavior on supported systems; deterministic publication uncertainty and recovery over real persistence; complete immutable audit/member failure reconstruction; nontrivial request-ID dedup under offline replica divergence; Git remote branch policy and external writers; symlink/path traversal, YAML pointer and exact Markdown byte-preservation cases; high-contention throughput and cleanup/GC after orphans; production implementation tests. Do not select micro-worktrees versus private staging solely on this evidence.

**Verification boundary:** Both suites passed as standalone isolated Git experiments; **no awl product tests or benchmarks** were run, and the PR remains a design draft.


## 12. Markdown byte fidelity, YAML guards and 24-process contention (2026-11-06)

**Evidence status:** Third standalone, **non-product** Git CLI fixture using temporary bare Git repositories; the source and JSON outputs are retained in a downloadable conversation artifact. This verifies illustrative mechanism behavior, **not** an awl implementation, an approved Git-storage algorithm, an end-to-end document editor, a durability profile or performance targets. The fixture uses Git 2.47.3, Python 3.13.5 and a **test-only** `ruamel.yaml` 0.18.17 parser for an indexed YAML validation guard. It does **not** make that dependency part of the CLI architecture.

| Probe | Observed result and limit |
| --- | --- |
| S1 — Four Markdown byte fixtures, including CRLF/LF, Unicode, YAML comments and an alias | Exact body-only replacement kept the entire frontmatter **byte-for-byte**; a duplicate body target was rejected without a published commit. **Not** a general Markdown-aware editor or YAML rewrite proof |
| S2 — Concurrent YAML list insertion changes a positional target | An expected element at `/items/0` no longer matched after another insertion; the stale positional request was rejected without a commit. A duplicate mapping key was rejected by the test parser. **Only validation, not YAML mutation**, was exercised |
| S3 — Invalid paths and Git symlink mode | A narrow fixture guard rejected five unsafe illustrative paths, including traversal and a link target; no rejected commit. **Not** a complete cross-platform path/security analysis |
| S4 — 24 competing local processes against the same managed ref | Eight processes targeted one document; sixteen targeted distinct documents. Each of **nine** repetitions preserved **17 accepted commits** and rejected **7** conflicting requests without published audit/commit for the rejected IDs. All distinct-file edits survived |
| S5 — Remote push response not observed by caller | A successful local-bare Git push's response was ignored; the published commit was verified by reading remote ancestry. **No actual network failure or hosted remote** was tested |
| S6 — Missing reachable Git object | After intentionally deleting a loose blob **inside a disposable test repo**, `git fsck` returned an error and reading its content failed. This shows detection, **not** repair or resilience to actual power loss |

**Repetition accounting:** The third fixture contains six distinct probes. It passed **3 full runs × 6 probes = 18/18 scenario executions**. Within those runs, S4 was itself repeated three times per full run: **9 × 24 = 216 individual concurrent requests**, of which **153 were published and 63 were rejected** as expected. These process requests are nested test operations, not 216 additional acceptance scenarios. The previous first fixture was also rerun 5× (3 × 5 = 15 scenario executions), and the second fixture 5× (6 × 5 = 30). Across all three standalone fixtures, **15 distinct synthetic scenarios passed 63/63 scenario executions** in this verification session. No product test or cross-platform/remote-hosted benchmark is implied.

**Test-fixture correction (negative evidence):** The first S4 run failed an **assertion**, not an observed Git safety property: the test used substring matching for request IDs, so `stress-1-1` could appear inside `stress-1-12`. The assertion was corrected to compare **complete audit trailer lines**, and the full test set was rerun successfully. Preserve this caveat when interpreting the result; a successful fixture is not a substitute for independent system testing.

**Conclusions bounded by evidence:**

- On a cooperating single POSIX host, a private Git index plus short `fcntl.flock` serialization of revalidation/commit publication, guarded by `git update-ref`, can maintain a single authoritative ref without a materialized per-request worktree **for these limited operations**. It is not proof that this mechanism is the best or fastest option for all of awl.
- Rejected concurrently stale requests were screened **before** creating a candidate commit in S4 and did not advance the managed head. As in §11, a separate CAS failure after speculative commit creation may leave an unreachable candidate object; the observable contract concerns **published/reachable** history.
- Byte-preserving exact body replacement is feasible without reserializing frontmatter, but the test YAML parser only **checked** a precondition. A full generic YAML edit engine must still prove fidelity, aliases, pointers, list preconditions and failure isolation.
- The repository's `core.fsync`/filesystem/platform guarantees were **not** tested under sudden power loss. Synthetic missing-object detection and deterministic process SIGKILL must never be described as proof of power-loss durability.

**Remaining verification gates before implementation approval:** full YAML/Markdown and link-preservation operations on representative real documents; repeated remote pushes with competing writers, hosted-provider branch policies and timeouts; crash/recovery with complete member audit and idempotent response reconstruction; power-loss/fsync behavior on supported operating systems; path case-folding/symlink safety; larger and less cooperative local concurrency; retention/GC for unreachable speculative objects; quantitative comparison with micro-worktrees if a maintenance tradeoff remains. **No awl source code was modified or executed by these experiments.**

## 13. Owner selections: initial runtime and completed no-change audit (2026-11-06)

**Decision status: owner-approved product direction, not implemented.**

### Initial CLI technology

- **Language/runtime:** Go for the first CLI implementation increment. A properly built binary does not require a Go runtime installation on the target machine.
- **Git integration:** use the installed **external Git CLI**, not an embedded Git library. Invoke it with explicit argument arrays and controlled environment/process execution (no shell interpolation). In the first slice, prefer the existing Git object/tree/ref APIs already exercised by the standalone fixtures.
- **No new mandatory abstraction:** do not implement a generalized interchangeable Git engine, micro-worktree manager, provider plugin registry or daemon just to preserve unchosen alternatives.
- **Still unchosen:** exact Go module/dependencies, Git version floor, target OS/architecture and packaging matrix, cross-platform locking primitives, real durability/fsync policy, public CLI names/input formats and remote synchronization scheduling. POSIX `fcntl.flock` from a Python fixture is **not** directly portable Go runtime behavior.

### Completed request and audit publication

| Request completion class | Document tree | Git publication | Audit record |
| --- | --- | --- | --- |
| Completed changing mutating request | Changed | One content commit | Complete versioned request/member outcomes in its commit message |
| Completed no-op mutating request | Identical | **One audit-only commit** | Complete request/member outcomes in the commit message |
| Completed mutating request with all ordinary members failed | Identical | **One audit-only commit** | All ordered failures and request outcome in the commit message |
| Whole request rejected for unsafe local concurrent publication conflict | Identical from that request | **No commit of any kind** | Error returned to caller; no durable shared Git audit entry |
| Invalid input rejected before request admission/execution (initial proposal) | Identical | No commit | Error returned to caller, no published audit |
| Read-only action or transient network synchronization retry | Identical | No request-audit commit | Existing sync-status semantics remain separate |
| Already locally published request, later remote push fails | Existing accepted tree retained | **No new request commit** due only to push failure | Existing local commit remains; remote pending/conflict status separately observable |

A no-change audit-only commit **advances the authoritative local ref and affects ref contention**, even though its user document tree equals its parent tree. It therefore requires the **same guarded ref publication**, complete final outcomes and lost-response/idempotency recovery as a changing request. A duplicate request ID/payload digest must not produce a second commit; a different payload under an existing ID must fail closed, subject to the final request identity contract. A failed speculative publication must never be reported as a completed request, even if earlier members provisionally succeeded.

**Boundary:** “no commit” for a rejected concurrent request means none reachable/published in the authoritative history. Private staging that prepares an unreachable candidate object can exist under some Git protocols; whether *even constructing* such objects is forbidden is still a test/design gate. No audit-only attempt can be used to circumvent the no-commit decision.

### Focused acceptance criteria (not executed against awl)

- **RD-01:** Accepted body edit -> exactly one content+audit commit with all member outcomes; original user-owned bytes retained except requested edit.
- **RD-02:** Completed valid no-op -> one audit-only commit, unchanged document tree, full outcome queryable by request ID.
- **RD-03:** Completed composite with all ordinary members failed -> one audit-only commit; every member failure retained in order; unchanged document tree.
- **RD-04:** Unsafe concurrent whole-request conflict (even after staged successes) -> no published user content and **no request commit**; caller receives reread/resubmit diagnostic.
- **RD-05:** Lost response after audit-only ref publication -> same request ID and identical payload returns recorded result without a duplicate commit; changed payload with existing ID is rejected.
- **RD-06:** Two concurrent audit-only requests -> both finalized outcomes survive if safely publishable; no silent head loss; bounded retries and explicit uncertain outcome on interruption.
- **RD-07:** A subsequent remote push rejection cannot erase locally acknowledged content or audit-only commits.
- **RD-08:** `awl` Go binary invokes Git without a shell; Git missing/unsupported, malformed stdout, nonzero exit, path attack and process interruption produce controlled diagnostics rather than silent mutation.

These are **new acceptance proposals**, not new observations from the previous **Python** fixture. The owner selected the high-level behavior and technology, but not a fixed CLI schema, Git version, OS support guarantee, exact message format, dependency or implementation timeline.

**Coordination risk:** Draft PR #2 introduces `FR-27` and `AC-25`–`AC-29` identifiers also used for different requirements/scenarios in this PR #4. Reconcile those IDs and their semantics before either PR is merged; do not silently renumber another PR's work or treat both proposed documents as already integrated.
