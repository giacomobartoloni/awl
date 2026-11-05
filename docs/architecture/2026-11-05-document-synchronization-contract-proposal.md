# Document Synchronization Contract — Provider-Neutral Architecture Proposal

**Status:** exploratory design proposal for review; **not approved, implemented, or an ADR**.  
**Date:** 2026-11-05.  
**Product:** AgentWorklog CLI (awl), not the existing AgentWorklog Markdown worklog system.  
**Scope:** Provider-neutral local-first synchronization of project-scoped Markdown documents and YAML frontmatter.  
**Existing baseline:** [Core Requirements](../requirements/core-requirements.md), [Document and Operation Contract](../requirements/document-operation-contract.md), [Git Hub-and-Spoke Evaluation](2026-10-23-local-first-storage-evaluation.md).  
**Active parallel proposal:** [PR #2 — Project configuration and worktree resolution](https://github.com/giacomobartoloni/awl/pull/2).  
**Research provenance:** AgentWorklog/awl/Specs/2026-11-05-markdown-synchronization-research.md (exploratory literature review, 2026-11-05). This document is self-contained; access to the private worklog is not required to review the proposal.

> **Approval boundary:** This document compares a potential replacement for Git-centered persistence and synchronization. The Git-centered design merged via PR #1 remains the recorded repository baseline. Do not treat the proposal, its examples, or acceptance criteria as an approved architecture, a public CLI schema, or authorization to implement. In particular, do not silently supersede owner-confirmed Git-specific requirements or alter PR #2.

## 1. Motivation and requirements boundary

The owner raised three reasons to reconsider Git as the mandatory synchronization engine: (1) Git ref and merge orchestration under concurrent requests is complex; (2) independent remote providers could be strategically useful; (3) per-document publication may reduce coordination between unrelated files. awl will maintain a complete managed local copy of available project Markdown documents and support offline writes.

These are **hypotheses to evaluate**, not established performance or simplicity results. File-level synchronization reduces the scope of unrelated edit conflicts but moves complexity into local history, identity, checkpointing, deletion tracking, remote races, audit, and recovery.

### Confirmed product behavior to retain

- Project identified by slug, with project-relative document paths and arbitrary, user-owned Markdown/YAML frontmatter.
- CLI-first and harness-agnostic; no mandatory MCP, daemon, SDK, or external agent editing of managed local files.
- Offline local reads/writes; controlled body-only text editing, structured metadata operations, and safely applicable stale edits.
- Mutating callers **do not supply file revision identifiers**. awl performs internal validation and safe retries, returning useful domain conflicts when necessary. Optional revision-pinned reads remain valid.
- Ordered composite requests continue after member errors, retain every outcome, and have request-level audit correlation; no global distributed rollback or strict multi-document ACID guarantee.
- Operation-level audit is distinct from provider revision history and includes attempted/failed operations where supported by the established contract.
- Compatibility with existing file bytes, YAML, links and Obsidian conventions; no silent normalization or migration.
- Lexical, semantic and hybrid search remain derived capabilities with separately observable index freshness.
- Shared source-repository worktree project configuration remains a requirement; the storage binding and Git-only details must be reconsidered explicitly rather than dropped.

### Explicitly contested by this proposal

The merged baseline requires a **managed local Git repository, one authoritative remote Git hub, and one local Git content commit per request with successful edits**. A provider-neutral managed store would require a product decision on whether equivalent request-scoped durability/history is sufficient. Do not claim this decision has already been made. The current Git-based model remains a complete alternative to this proposal.

## 2. Proposed logical architecture

~~~text
Human or coding-agent shell
            |
         awl CLI
            |
Application operations (project, read, edit, metadata, composite, audit, search)
            |
Managed local document state + durable operation journal
            |
Document synchronization coordinator
  - discover remote changes and establish causality
  - reconcile individual document states
  - validate content and document safety
  - stage imports and guarded publications
  - track per-document checkpoints and conflicts
            |
Remote backend adapter (one selected authoritative remote per project)
       /             |                  \
   Git remote    Google Drive       Filesystem/SSH candidate
~~~

The diagram shows **logical responsibilities**, not required processes, module boundaries, language choices, storage format, or a dynamic plugin framework. A remote adapter may itself need project-wide operations. In particular, Git remote publication still updates commit history and refs; the phrase “per-document” must not imply per-file atomic Git pushes.

Candidate simplicity constraint: one authoritative remote backend per project, selected through shared configuration; many local clients may write to that remote, but multiple co-equal remote authorities and satellite export/ingestion are outside this initial proposal.

### Boundaries

**Application core** owns validation of user-initiated operations, document and metadata semantics, composite ordering and outcomes, and audit provenance. It must not expose remote-specific revision tokens in mutation inputs.

**Local state** owns durable document bytes, local identities/revisions, pending changes, verified merge bases, request/audit records, tombstones, and per-provider synchronization checkpoints. The actual implementation (Git, file snapshots plus journal, database, etc.) is **undecided**. Avoid inventing a custom engine without testing the cost of replacing Git history.

**Synchronization coordinator** owns a conservative reconciliation policy and truthful status reporting; remote content must be inspected/staged before it can modify managed local state.

**Remote adapter** owns backend-specific identity, listing/change detection, reads, guarded writes/deletes where feasible, acknowledgement/retry mechanics, credentials and provider error mapping. An adapter must declare unsupported capabilities instead of pretending to offer atomic writes.

## 3. Document identity and synchronization state (candidate model)

Separate at least these concepts:

| Concept | Responsibility |
| --- | --- |
| Project identity | The configured awl project; a slug alone must not accidentally join unrelated histories |
| Document identity | Stable **local** identity to correlate revisions across path changes; exact representation unresolved |
| Project-relative path | User-visible Markdown path, which may change and can collide on some providers |
| Managed content | Original bytes of a local document revision, preserving encoding and formatting unless explicitly edited |
| Local revision | Durable local state after an accepted change; internal version identifier and history format TBD |
| Common base B | **Retained and verified** content/version last known to be shared by local and remote; do not fabricate one |
| Local head L | Current committed local content, including pending offline edits |
| Observed remote head R | Version actually read/verified from the provider, with its native identity/revision token if available |
| Sync checkpoint | B plus provider identity/token and acknowledgement position, updated only after confirmation |
| Tombstone | Durable evidence of deletion, rather than treating an absent/unavailable file as an authorized delete |
| Conflict record | Retained B, L and R (or references), affected path/metadata, and reason; never silently discard inputs |
| Audit request | Request identity, actor information actually known, member outcomes, local revisions and remote effects |

**Critical distinction:** equality of file bytes is useful for no-op detection but is not by itself proof of causal ancestry. A provider version token is useful but may not supply history, and its exact guarantees differ by backend. A safe three-way merge needs the **actual preserved common base**, not a guessed snapshot. If initial import or history loss leaves no trustworthy base, prefer explicit reconciliation or a bounded manual decision rather than synthesizing ancestry.

Do not assume every backend retains historic revisions indefinitely, preserves stable IDs through rename, or supports strong conditional updates.

## 4. Candidate per-document reconciliation algorithm

For each document, use B (verified shared base), L (local head) and R (observed remote head), with existence/tombstone and identity handled alongside the content.

| Verified observation | Candidate action |
| --- | --- |
| L and R have identical content and compatible identity | No content transfer; reconcile/checkpoint confirmed state |
| L = B; R differs from B | Stage and validate the remote change for local import |
| R = B; L differs from B | Attempt guarded publication of L |
| L differs from B; R differs from B; L differs from R | Attempt conservative three-way reconciliation from B |
| B not available or remote discovery incomplete | No unsafe merge/deletion inference; explicit bootstrap or recovery |
| One side deletes while the other edits | Preserve tombstone and modified content; explicit conflict |
| Both sides rename/move or rename conflicts with another writer | Resolve identity/path conflict explicitly; do not silently rewrite links |

A candidate sync attempt proceeds as follows:

1. **Capture:** take a coherent snapshot of the relevant local document revisions, outstanding request changes and provider checkpoints. Do not hold a long-lived lock across network calls.
2. **Discover:** list or incrementally discover remote changes; handle pagination, change-token expiration, permission loss, provider eventual visibility and partial scans. Treat an incomplete scan as uncertainty, **not** mass deletion.
3. **Observe:** fetch R and any needed identity/version metadata into staging. Compare against retained B and L; detect remote-originated edits without assuming they passed through awl.
4. **Reconcile:** no-op / one-sided fast-forward / genuine three-way merge. If no reliable base, incompatible same-key metadata, ambiguous body intent, missing document identity, or unsupported safety precondition, create a conflict record and retain every input.
5. **Validate:** check the result without destructive reserialization: byte/encoding and frontmatter integrity, YAML parsing when relevant, accepted mutation intent and known Markdown/link constraints. Parsing success alone is not a proof of semantic correctness.
6. **Apply locally:** accept safe remote imports/merges through the managed store and audit path. Persist both content and associated state before reporting a durable local result.
7. **Publish:** write changed content to the authoritative backend using the strongest supported precondition. If a concurrent remote update invalidates the precondition, fetch the new remote state and re-evaluate under a bounded retry policy. Never force-overwrite or delete remote content solely to overcome a conflict.
8. **Acknowledge:** update the per-document remote checkpoint only after the exact published state is confirmed. A timeout after provider acceptance is **indeterminate** until reinspection or an idempotent acknowledgement check; do not blindly repeat side effects.
9. **Report:** distinguish locally applied, remotely acknowledged, pending, conflict, and recovery-needed results. All request/member-level audit remains traceable.

These are behavior candidates, not an implemented algorithm. A successful textual merge might still alter Markdown meaning; automated merging must be **conservative**, with explicit review for ambiguity. The first bounded prototype can deliberately treat all two-sided same-file divergence as a conflict before adding automated merge.

### Merge safety examples

- Independent new paragraphs may be mergeable when placement and surrounding headings are unambiguous.
- Concurrent incompatible writes to the **same YAML key** must not be resolved merely because a textual merge yields syntactically valid YAML.
- YAML comments, key order, quotes, anchors, BOM, LF/CRLF, whitespace and unchanged Markdown must not be silently normalized.
- Moving a heading, renaming a linked document, or inserting rows into a Markdown table can create semantic errors even without line-level conflicts.
- Conflict markers must not be installed into the authoritative Markdown content by default. Retain conflict versions separately and surface useful excerpts to the caller.

## 5. Local mutations, composites, audit, and concurrency

**Local editing is not remote synchronization.** A successful local edit is durable and readable offline even when publication later fails.

- A single awl request may target several documents. Member operations execute in order, continue after individual failures, and report final persisted outcomes rather than provisional/rejected retry results.
- Request-level audit links every successful/failed member to its local revisions and subsequent per-document publication statuses.
- Per-file remote publication may be partial across a composite; **do not imply distributed multi-document atomicity** or that all remote observers see a coherent request snapshot.
- Multi-document operations affecting paths/links might require an explicit integrity barrier or manual publication mode. That policy is open and must not be smuggled into automatic file-level synchronization.
- Concurrent local requests should read coherent local snapshots and publish updates without lost writes. An implementation may use per-document guards, a short local transaction, or compare-and-swap internally; the concurrency primitive is not selected.
- The current owner-selected requirement of **one Git content commit per locally changing request** is incompatible with silently replacing the local Git store. This proposal calls for an explicit review of equivalent local durability and request-level history semantics before superseding it.
- Retry must not replay external side effects or duplicate audit records. A request ID and recoverable request journal should distinguish accepted changes from tentative attempts and crash-ambiguous results.
- Remote-originated modifications need an explicit “external import” provenance category; unavailable verified actor identity must not be fabricated.
- Search indexing remains downstream of durable local content, with its own freshness state; it must not mutate synchronized documents.

## 6. Remote backend capability contract (questions before interfaces)

The following is a **minimum evaluation checklist**, not a frozen public adapter API:

| Capability | Why it matters | Candidate fallback / safety rule |
| --- | --- | --- |
| Enumerate documents or discover deltas | Detect remote changes | Bounded full scan; never infer deletions from partial results |
| Stable object identity and name/path mapping | Distinguish rename from delete/create | Explicit ambiguity when identity cannot be proven |
| Read bytes plus version identity | Preserve exact content and detect change | Hash plus conservative checkpoint and reread |
| Conditional create/update/delete or equivalent | Prevent lost remote writes | If unavailable, evaluate single-writer restriction or declare multi-writer unsafe |
| Recover acknowledgement after timeout | Avoid duplicate or false publication | Read back exact remote version and preserve pending state |
| Historical version access | Three-way merge/recovery | Maintain verified B and local snapshots regardless of remote history |
| Deletion/tombstone semantics | Prevent resurrection and data loss | Retain deletions; explicit edit-versus-delete conflict |
| Permissions and credential handling | Scope and security | Personal local config/auth; no secrets in shared YAML |
| Limits and batch/ordering semantics | Honest composite publication | Publish per supported unit; expose partial propagation |
| Fidelity and portability | Preserve user-owned Markdown | Verify round-trip byte content; no silent format conversion |

**Important:** Retrying a read-then-overwrite update is not equivalent to an atomic remote compare-and-swap. If an adapter cannot prevent another writer from changing a file between validation and publication, it cannot claim safe, general multi-writer synchronization without additional coordination. This is a gating provider capability, not a generic REST assumption.

### Backend-specific considerations

- **Git:** a Git remote can remain a fully supported backend candidate, with publication of commit/ref history and internal guarded ref updates. Per-document logical reconciliation does **not** turn Git into an independent per-file remote transaction system; ref-level races remain adapter concerns. The Git-centered architecture remains a valid alternative.
- **Google Drive:** discover changes through provider mechanisms where available; map file IDs to project paths, manage revisions/tokens, pagination, rename/deletion and permissions. Atomic conditional write capabilities require **provider-specific verification**, not inference from HTTP terminology.
- **rsync:** useful for efficient delta transfer to filesystem/SSH destinations, but is not itself a two-writer content reconciliation engine.
- **rclone / bisync:** useful transport or a delegated filesystem-sync experiment. Bidirectional conflict copies, snapshot state and potentially destructive resync do not replace awl's controlled imports, audit or conflict policy. Never run an external synchronizer against awl's writable managed store in a way that bypasses document operations.
- **Automerge and other text CRDTs:** not a preferred dependency for the initial file-based contract. External Markdown editors produce state snapshots without CRDT operation history; bridging those snapshots still requires a separate reconciliation policy. Revisit only with demonstrated frequent same-document collaborative editing.

A single backend per project is the current simplifying **proposal** for this alternative. Provider changes would be explicit migrations, with pending changes and provenance handled; changing one YAML key is not automatically a safe migration.

## 7. Project configuration interaction

Retain the existing owner-confirmed distinction between a version-controllable shared **.awl/project.yaml** and private **.awl/local.yaml**. A selected synchronization backend belongs conceptually in project configuration; credentials, machine-specific paths and authorization stay local.

Illustrative, **not approved YAML**:

~~~yaml
project:
  slug: example
sync:
  type: google-drive
  remote: "<configured-project-folder>"
~~~

The current merged Git baseline and PR #2 presume a Git remote and Git-backed local store. A backend-neutral design must explicitly revisit remote binding, local-store identity, worktree-specific resolution and source Git identity fallback; it must **not** silently remove shared-worktree guarantees or pretend existing YAML examples are an approved schema.

## 8. Acceptance criteria and falsification scenarios

The earlier AgentWorklog research proposed tests T01–T24; prioritize these candidate cases for a decision-quality comparison against the existing Git-centered baseline:

| Scenario | Required observation / rejection rule |
| --- | --- |
| T01, repeat sync/no changes | No duplicate publication or audit side effect |
| T02, two agents edit separate files | Both changes survive; no unrelated content conflict |
| T03/T04, same-file disjoint/overlapping edits | Safe proven merge or explicit preserved conflict; no silent loss |
| T06/T07, concurrent YAML metadata keys | Preserve independent typed fields; reject incompatible same-key edits |
| T08/T09/T10, delete/edit, rename/edit, link changes | No resurrection, loss of modified content, or silent link rewriting |
| T11/T12, encoding and malformed frontmatter | Byte fidelity; reject unsafe structured merge |
| T14, many small Markdown documents | Measure discovery cost, rate limiting and bounded rescans |
| T15/T16, crash and ambiguous remote timeout | Durable local result; recoverable pending/indeterminate status |
| T17, composite partial outcomes | All local member outcomes recorded; per-file remote progress truthful |
| T18, concurrent local processes | No lost writes; bounded retries; coherent reads |
| T19, remote editor outside awl | Controlled external import or explicit conflict and retained inputs |
| T20/T21/T22, path and provider uncertainty | Reject path escapes, stale scans, false deletes and online-only gaps |
| T23/T24, migration and search lag | Preserve history/audit; report separately stale search/index state |

Also consider T05 (same-offset insertions), T13 (long offline divergence) and mixed file counts. **No tests in this table have been executed.** Suggested measurements: data loss/corruption (hard stop), conflict false negatives/positives, byte fidelity, provider API calls, retry counts, crash recovery, maintenance complexity and operational burden.

A small prototype comparison should use identical fixture files and deliberately injected concurrency/failure cases for a Git-backed implementation and one verified file-oriented backend. Do not infer the chosen storage engine, provider contracts or language from hypothetical results.

## 9. Alternatives, open questions and decision gates

| Alternative | Why retain or defer |
| --- | --- |
| Existing managed Git + authoritative Git remote | Baseline with established history/ancestry and provider ecosystem; potentially simpler overall than implementing a new journal |
| Provider-neutral local store + per-file synchronization | Candidate to lower cross-file coupling and support direct Drive-style providers, but substantial history/recovery responsibilities |
| Delegated rclone bisync with isolated staging | Potential bounded option for low-concurrency workflows; insufficient alone for audited controlled edits |
| CRDT-backed local state | Defer until continuous or frequent same-document collaboration justifies additional protocol/state and external-edit bridging |

**Review gates requiring explicit owner decisions:**

1. Is replacing mandatory managed local Git and the single local Git commit per changing request acceptable if the alternate store preserves equivalent durability and request-correlated revision history?
2. Is **one authoritative remote per project** the desired initial topology? Are export-only secondary destinations in or out of initial scope?
3. Is a file-oriented remote lacking atomic conditional writes allowed only in a documented single-writer mode, or excluded?
4. What is the minimum automatic same-file merge policy for the first increment: one-sided propagation only, or guarded three-way merge?
5. What remote guarantees are required for multi-document link/rename operations, given intentionally loose transactionality?
6. What explicit migration behavior is acceptable for an existing Git-centered project and its pending local work?
7. Which provider should be used in the first bounded comparison? Avoid assuming Google Drive provider semantics before testing.

**Recommendation for review:** Evaluate a conservative file-granular model with retained base snapshots, controlled three-way merge, durable per-document checkpoints, staged imports, explicit conflicts and guarded remote publication against the existing Git baseline. Keep the local engine, exact YAML schema, adapter types, language, dependencies and release plan unselected pending evidence.

## 10. Sources and provenance

- [Git merge-file](https://git-scm.com/docs/git-merge-file) and [Git merge](https://git-scm.com/docs/git-merge): established three-way merge and repository-level behavior.
- [Pierce and Vouillon, What's in Unison?](https://www.cis.upenn.edu/~bcpierce/papers/unisonspec.pdf): bidirectional file synchronization and failure models.
- [Saito and Shapiro, Optimistic Replication](https://www.microsoft.com/en-us/research/publication/optimistic-replication/): distributed update and reconciliation theory.
- [Fraser, Differential Synchronization](https://research.google/pubs/differential-synchronization/): shadow-state and patch-based synchronization.
- [Joplin sync design](https://joplinapp.org/help/dev/spec/sync/): item-oriented offline synchronization and sync metadata.
- [Google Drive change tracking](https://developers.google.com/workspace/drive/api/guides/manage-changes) and [revision history](https://developers.google.com/workspace/drive/api/guides/manage-revisions): remote discovery and version limits.
- [rclone bisync documentation](https://rclone.org/bisync/), [rsync algorithm](https://rsync.samba.org/tech_report/).
- Internal exploratory research: AgentWorklog/awl/Specs/2026-11-05-markdown-synchronization-research.md; its literature review and T01–T24 scenarios informed this proposal. It remains **exploratory**, not a product decision.

**Verification status:** Documentation proposal only. No awl implementation, prototype, test, provider authentication, synchronization benchmark, or migration has been performed or validated.
