# Local-first Storage and Synchronization — Architecture Evaluation

**Status:** exploratory proposal; **no architecture or dependency approved**.  
**Date:** 2026-10-23.  
**Scope:** Architecture of the AgentWorklog CLI (awl), not the existing Markdown worklog system.  
**Related draft requirements:** [Core Requirements](../requirements/core-requirements.md).

## 1. Question

How should awl preserve and edit offline Markdown documents, maintain an auditable operation history, and synchronize changes with **one configured remote backend per project**, initially considering Git and Google Drive, while avoiding unnecessary service infrastructure and strong multi-document transactions?

Key constraints from the product discussion:

- Project identity is a slug; it need not match a repository name or folder.
- Clients read and edit through awl operations; the local replica is managed state, not a public editing interface.
- Document bodies remain Markdown and frontmatter remains arbitrary YAML, with future conventions oriented toward OKF v0.2.
- Editing operations are inspired by agent read/edit/patch tools; an operation may edit text and metadata.
- One request may contain a composite of ordered operations and can have partial outcomes; no global ACID/rollback guarantee.
- Local operation success must be distinguishable from the later synchronization result.
- Audit needs caller intent and operation outcomes as well as document diffs.
- The user explicitly deferred edge-case enumeration, concurrency algorithms and library selection.

## 2. Logical architecture

~~~text
CLI / future MCP
       |
Operation API (projects, documents, metadata, composites)
       |
Application processor -- audit records
       |
Local state interface (documents, versions, pending changes)
       |
Sync coordinator (reconciliation policy not yet specified)
       |
Remote adapter boundary
      / \
 Git adapter     Google Drive adapter
~~~

These are **logical responsibilities**, not required processes, packages or deployment units.

### Application layer

1. Resolve project slug and validate project-relative paths.
2. Convert individual CLI actions and composite requests into the same set of document/metadata operations.
3. Apply edits to a proposed document state, collect per-operation results, and publish the resulting local state according to explicit failure semantics.
4. Record request identity, caller/actor information available to the process, intended operations, failures, applied changes, and resulting local revision(s).
5. Never infer or enforce Activity, Plan or Spec lifecycle rules.

### Local persistence boundary

Potential duties: store current document bytes, revision references and historical snapshots sufficient for editing and later synchronization; store pending local changes and progress markers; preserve audit records across restart.

The local format is **not selected**. Hiding a storage directory from the API does not by itself prevent same-user processes from modifying physical files; any claim of tamper-resistant audit would require a separate threat model.

### Sync coordinator and remote adapters

The sync coordinator can own project-level scheduling, state transitions and reporting, but should not pretend all providers have Git's history or Drive's metadata.

A useful abstract boundary may expose remote inventory, content retrieval, version state, writing, and provider capabilities. Whether it should be this general or instead allow materially different sync strategies is an **architecture investigation**, not an assumed interface.

**Git remote**: a repository containing versioned trees/commits and refs; synchronization may transport commit history with Git protocols if the local engine is Git-backed. If not, an adapter must produce or consume Git revisions and map them to awl document revisions.

**Google Drive remote**: a hierarchy of Markdown files, file IDs, provider metadata/revisions and a changes feed. It does not natively understand commits or awl audit events. A Drive adapter needs a mapping between project-relative paths, file identity and sync baselines; it cannot assume Drive revision history is a permanent audit ledger.

An awl project uses **one** such remote backend. The local replica is not a second user-configurable remote.

## 3. Candidate local persistence mechanisms

| Criterion | A — Embedded Git repository | B — Managed snapshots + journal | C — CRDT (e.g., Automerge) |
| --- | --- | --- | --- |
| Offline local changes | Native commits/local repository | Explicit stored snapshots and append-only operation journal | Native local CRDT updates |
| History/diffs | Mature Git object model and diff tools | Must select/store versions and diff/merge mechanisms | CRDT change history |
| API-controlled edits | Possible without exposing a working tree, subject to implementation | Natural boundary for private snapshots | Natural if CRDT is only written through awl |
| Application audit of failed/partial requests | Separate operation log required | Natural journal candidate | Separate application-level log required |
| Git remote | Can reuse native Git transport/ref model | Requires projection to/from Git commits | Requires Git projection, including CRDT metadata problem |
| Google Drive remote | Requires document projection and revision mapping | Requires document projection and revision mapping | Requires document projection; remote Markdown alone loses causal metadata |
| Concurrent merge | Git three-way merge and conflict handling | Need diff/merge library or chosen policy | CRDT convergence among compatible replicas; not semantic correctness |
| Portability of remote Markdown | Natural | Natural | Requires careful Markdown projection and sidecar/remote CRDT state design |
| Main risk | Coupling local state design to Git history and complexity of Drive bridge | Reinventing substantial version/merge machinery | Mismatch with plain Markdown and external Drive edits |

### Option A — Git as internal local engine

Use private Git objects/trees/commits to represent local document snapshots. A request may produce one commit or another carefully specified grouping. The remote Git adapter can use native Git where possible; Google Drive requires conversion from the Git state to Drive file operations and a persisted sync mapping.

**Advantages:** mature version history, content addressing, merge implementations, native Git remote connectivity.

**Costs/risks:** Git commits do not record failed requests; commit author metadata is not an authenticated actor; import of direct edits from Drive requires synthetic commits/mapping; many small operations may produce many Git commits; branch/ref semantics, garbage collection, audit completeness and hidden working-tree exposure need careful design. No public working tree is required in principle.

### Option B — Managed snapshots and an operation journal

Store durable document snapshots or version deltas, a journal/outbox of attempted and applied operations, and remote synchronization state. An embedded storage mechanism (potentially a small database, filesystem objects or an append-only store) is an implementation candidate, **not** a decision to adopt an RDBMS or require ACID transactions.

**Advantages:** aligns persistence with awl's request/operation model and per-operation audit; the same document-level model can be projected to Git or Google Drive; composite partial outcomes are natural.

**Costs/risks:** version retention, snapshot reconstruction, integrity, compaction and diff/merge must be designed or delegated. Git remote handling would require a Git adapter creating/reading commits, not merely uploading Markdown blobs. A bespoke implementation must be bounded to avoid recreating a VCS.

### Option C — CRDT-backed local engine

Use a CRDT state model as canonical local representation and export Markdown to remote backends.

**Advantages:** offline edits and convergence among participating CRDT replicas.

**Costs/risks:** Git and Google Drive remotes store ordinary Markdown; a Markdown-only copy does not retain all CRDT causal history. External edits and round-tripping of YAML/Markdown introduce semantic challenges, and not every converged text is meaningful. This is **deferred**, not rejected on general grounds.

### Preliminary assessment

**Shortlist A and B for an evaluation spike; defer C.** There is insufficient evidence to choose Git as the local engine solely because Git is also a remote backend. B may make audit/composite semantics more direct; A may avoid building history and merge systems. Both still need a Drive synchronization implementation.

## 4. Sync adapter implications

| Concern | Git backend candidate | Google Drive backend candidate |
| --- | --- | --- |
| Remote identifier | Repository URL/ref/object ID | Drive folder/file IDs |
| Change discovery | Fetch/remote refs/commit ancestry | Incremental changes feed and provider inventory |
| Document retrieval | Git tree/blob | Drive file download |
| Publishing | Commit/ref/push semantics | File upload/create/update/move semantics |
| Remote history | Native commit graph | Provider revisions; may have retention/access limitations |
| Local-to-remote projection | Native if Git-backed locally; otherwise synthetic commits | Project-relative document paths/files |
| Audit sharing | Git commits alone are incomplete application audit | Needs independent application-defined representation/policy |
| Constraints | Refs, non-fast-forward, branch/permission rules | File IDs vs duplicate names, folder scope, API rate limits/permissions |

The provider can supply state signals, but its guarantees cannot be copied verbatim into a provider-independent promise. In particular, Drive change tokens aid incremental discovery, not arbitrary, durable two-way commits.

The first implementation slice must not conceal unsupported safety guarantees. How conflicts are reconciled, and the exact handling of remote writes by other applications, remain open pending a concrete use-case study.

## 5. Editing, composites, and audit

Proposed conceptual request shape (not a fixed CLI command schema):

~~~yaml
project: muse
request_id: example-123
operations:
  - id: status
    op: metadata.set
    path: Activities/example.md
    key: workflow-status
    value: completed
  - id: outcome
    op: document.edit
    path: Activities/example.md
    edits:
      - old_string: Pending verification.
        new_string: Implementation completed.
~~~

- A content edit may use exact text replacement; contextual patches and revision-aware merge remain candidate strategies.
- For every operation, awl must report applied, failed or not executed as appropriate. Composite-level status may be partial.
- Individual document updates should not leave truncated or inconsistent bytes; this need not imply global cross-document transactions or distributed rollback.
- Audit should distinguish request intent from final document diff, local application from remote synchronization, and asserted actor identity from authenticated identity.
- An implementation may group several modifications of one document before committing its new local version; the policy is not settled.
- Local revision and remote provider revision should not be conflated.
- When a client sees a locally committed revision, awl must not silently lose it on restart or during failed sync.

## 6. Open questions (intentionally not resolved here)

1. **Local engine:** A or B, persistence libraries, snapshot representation, revision retention and garbage collection.
2. **Sync contract:** Shared primitives versus backend-specific sync strategies, order of pull/push, and remote state baselines.
3. **Sync scheduling:** Explicit command, opportunistic sync during operations, or optional daemon; no daemon requirement currently.
4. **Conflicts:** Stale patches, out-of-band provider edits, simultaneous changes, interrupted transfers, conflict exposure and merge policy. Detailed edge cases deferred per user direction.
5. **Audit:** Which events are persisted locally, shared via remote backend, protected from rewrite, and correlated across collaborators?
6. **Provider scope:** Git branch/ref choice and Drive folder/root and file mapping. Git/GDrive are candidates, not shipped features.
7. **Local access boundary:** Threat model, credentials, and whether reading through awl is mandated for all clients.
8. **Composite semantics:** Stop-on-error or continue-on-error, dependencies, partial status, document-level grouping.
9. **Formatting:** YAML comment/order preservation, frontmatter parse failure, Markdown encoding and line endings.
10. **Compatibility:** Future OKF conformance verification, legacy convention migration, and links.
11. **Operations interface:** CLI arguments, request serialization, streaming outputs, agent tool interoperability, future MCP.
12. **Licensing/dependencies:** Choose only after technical spikes and license review; the awl repository currently uses AGPLv3.

## 7. Suggested small evaluation, not yet approved implementation

Conduct a short, controlled evaluation of A and B with equivalent scenarios:

- Bootstrap a project with several Markdown files containing unknown frontmatter and standard Markdown links.
- Read, apply several edits in one document request, and run a composite with one deliberate failure.
- Commit locally offline, restart, and inspect pending operations and audit history.
- Export changes to a Git repository and to a Google Drive folder in test environments.
- Pull an externally modified remote Markdown document; explicitly report when reconciliation is unsafe.
- Compare complexity, code size, dependency and license implications, correct audit semantics, behavior on restart, and number of network interactions.

The experiment **must not** be represented as successfully implemented, run or verified merely because it is proposed here.

## 8. Decision gates

1. Review this architectural decomposition and whether A/B are sufficient candidate local persistence models.
2. Confirm how strongly the product needs **shared** audit versus local audit.
3. Clarify minimal provider capabilities and synchronization outcomes when evaluation begins.
4. Select technology only after evidence from a small proof of concept.
5. Record any accepted significant decision as a repository ADR; this exploratory document is not an ADR.

## References

- [Proposed core requirements](../requirements/core-requirements.md).
- [AgentWorklog CLI](https://github.com/giacomobartoloni/awl).
- [OKF v0.2 specification](https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/main/SPEC.md).
- [Google Drive: retrieve changes](https://developers.google.com/workspace/drive/api/guides/manage-changes): API supports incremental change discovery.
- [Google Drive: manage file revisions](https://developers.google.com/workspace/drive/api/guides/manage-revisions): revision availability/access are provider-specific.
- [Git: update-ref](https://www.kernel.org/pub/software/scm/git/docs/git-update-ref.html): conditional ref updates.
- [Git: worktrees](https://git-scm.com/docs/git-worktree.html): distinguishes repositories and working trees.

**Repository status at evaluation:** default branch held LICENSE only on 2026-10-23. These files are proposed in an unmerged branch; no code, integration tests, or synchronization experiments were performed.
