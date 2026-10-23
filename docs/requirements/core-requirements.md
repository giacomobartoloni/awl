# AgentWorklog CLI — Core Requirements (Proposed Revision)

**Status:** draft; review required; owner-selected Git hub-and-spoke topology.  
**Date:** 2026-10-23.  
**Product:** AgentWorklog CLI (awl), distinct from the existing AgentWorklog Markdown conventions.  
**Provenance:** Consolidates the working vault draft at AgentWorklog/awl/Specs/2026-10-23-core-requirements-spec.md and the follow-up product discussion. This is a proposed replacement baseline, not an approved specification and not an implemented feature.

## 1. Purpose

Provide a project-oriented, offline-first interface for reading and editing Markdown documents and their generic YAML-frontmatter metadata. AgentWorklog clients use awl's controlled API rather than directly editing the managed local replica. Changes can be made while offline and published to one authoritative Git remote per project. Optional satellite integrations (starting with Google Drive as a candidate) connect to that remote Git hub. Satellite synchronization direction and mechanics remain open.

## 2. Agreed product boundaries

- A project is identified by a slug. The slug need not equal a repository or folder name.
- Each project uses managed local Git and one authoritative **remote Git repository** as its shared history. One or more optional secondary integrations, including Google Drive as an initial candidate, connect to the remote Git hub. These are not co-equal authoritative backends.
- A durable, locally managed **Git-based document replica** supports offline reads and changes; Git publication is asynchronous with respect to a local change. Optional satellite projections can lag independently.
- Clients access documents through awl. Direct filesystem mutation of its managed local state is not a supported editing interface. This is an API and audit boundary, not a claim of OS-level tamper resistance.
- Markdown files remain portable, with YAML frontmatter as the metadata serialization. New AgentWorklog conventions target Open Knowledge Format (OKF) v0.2; legacy files are user-owned and must not be silently migrated.
- OKF lifecycle status (draft, stable, deprecated) is distinct from the AgentWorklog extension workflow-status. No implicit mapping.
- Standard Markdown links are preferred for new links. Existing Obsidian links require a separately authorized migration.
- Document types and custom metadata keys have no fixed domain semantics in the CLI core. No Activity, Spec, Plan, or ProductPlan lifecycle commands are required.
- Single requests and composites may include multiple document and metadata operations. Composites run operations in order, **continue after member errors**, and report every outcome. **One local Git content commit per request with successful modifications** records the resulting changes; no multi-request/distributed rollback requirement is implied.
- Every applied change is associated with an auditable request/operation identity. Git commits may provide version history but do not by themselves capture attempted/failed operations or prove caller identity. Shared or tamper-resistant audit remains open.
- No metadata query engine, shared _research/_guides handling, mandatory server, or synchronization daemon is assumed for the initial slice.

## 3. Terminology

- **Project:** Slug-addressed context with a private local Git replica, one authoritative remote Git repository and optional satellite integrations.
- **Document:** A Markdown resource addressed by project slug and project-relative path.
- **Document metadata:** Arbitrary YAML-frontmatter values, independent of backend metadata.
- **Local revision:** A reference to the locally committed Git state associated with a document; exact reference/branch conventions remain TBD.
- **Authoritative Git revision:** A Git commit/ref state accepted by the project's central Git remote, distinct from locally committed history.
- **Operation:** An individual document or metadata action.
- **Composite:** An ordered collection of operations in one request, possibly spanning multiple documents.
- **Audit record:** A record of request intent, actor information supplied or authenticated, per-operation results, and resulting revisions.
- **Sync state:** Observable status distinguishing local application, publication to the Git hub, and propagation to each configured satellite; exact state names TBD.

## 4. Functional requirements

Priority is indicative: Core means candidate first increment, Later means explicitly deferred. No release plan is implied.

| ID | Priority | Requirement |
| --- | --- | --- |
| FR-01 | Core | Discover locally configured accessible projects and resolve a project by its slug; reject ambiguous resolution. |
| FR-02 | Core | Associate each project with a managed local Git replica and one authoritative Git remote; allow optional additional satellite integrations connected to the Git remote, not direct peer-to-peer remote replicas. |
| FR-03 | Core | Retain a locally durable, managed Git replica for a project and support reads and writes to already available local documents when disconnected. |
| FR-04 | Core | Offer project-scoped listing, full document reads, creation, and modification through awl without direct access to managed replica files. Partial physical-file line reads with snapshot pinning are under proposal review. |
| FR-05 | Core | Support generic, typed YAML-frontmatter reads and mutations using **RFC 6901 JSON Pointer** for single-node addressing. Distinguish replacement, insertion at a list index, append, and removal; preserve unknown keys and unaffected content and reject unverifiable stale positional list edits. |
| FR-06 | Core | Support LLM-agent-friendly **body-only** Markdown editing, including exact targeted replacement with explicit ambiguity/failure results; an obsolete read revision need not force a new read if the target remains uniquely and safely applicable. The final patch syntax is not yet selected. |
| FR-07 | Core | Accept several ordered edits within a single document-modification request and provide a clear document-level result. |
| FR-08 | Core | Support ordered composites that **continue after individual operation failures**, reporting every outcome and a request-level correlation identifier. Commit successfully applied changes once per request when content changes. |
| FR-09 | Core | Preserve locally accepted modifications in managed Git durably across restarts; do not silently discard changes awaiting publication to the authoritative Git remote. |
| FR-10 | Core | Record an auditable history of attempted and applied operations and their outcomes, including actor information as available; do not treat ordinary Git commits as a complete audit of failures, partial requests, or authenticated identity. |
| FR-11 | Core | Support exchange of local Git commits and shared history with the project's authoritative Git remote, distinguishing locally committed changes from remotely accepted changes. Satellite synchronization behavior is separately scoped. |
| FR-12 | Core | Expose local commit, Git hub publication, and per-satellite propagation status where applicable; errors, unresolved changes and conflicts must be visible without silent overwrites. |
| FR-13 | Core | Retain existing file contents and links unless an explicit modification succeeds. Never silently normalize, rewrite, move, overwrite, or delete user-owned Markdown. |
| FR-14 | Core | Produce explicit per-operation error outcomes for nonexistent or ambiguous targets, malformed YAML, unavailable permissions, unsupported actions, and modifications that cannot be reconciled safely. |
| FR-15 | Core | Constrain access to configured project scope and provider authorization; do not expose provider credentials or unrestricted local storage access. |
| FR-16 | Core | Support nonsemantic operations on arbitrary document types and frontmatter fields; neither OKF nor worklog-specific fields are interpreted as mandatory workflow rules. |
| FR-17 | Core | Make failure/restart recovery observable; global cross-document atomicity, distributed rollback, and ACID transactions are not required. |
| FR-18 | Later | Offer a remote-agent/MCP interface using the same application operations, without defining the wire protocol now. |
| FR-19 | Later | Optional OKF or project-defined validation, indexing, metadata query, or structural Markdown editing; no commitment to implement them initially. |
| FR-20 | Later | Support additional backend integrations as satellites of the authoritative remote Git hub, beginning with a Google Drive projection candidate; whether they support external-to-Git ingestion remains open. |

## 5. Non-functional requirements

| ID | Requirement |
| --- | --- |
| NFR-01 — Data integrity | A confirmed local change must be durable and recoverable; unexpected partial results, ambiguous states, and failed synchronizations must be explicit. |
| NFR-02 — Portability | Content remains standard UTF-8 Markdown and YAML frontmatter without mandatory Obsidian plugins or proprietary document formats. |
| NFR-03 — Offline usability | Already synchronized material is available and editable offline without hidden online preconditions. |
| NFR-04 — Auditability | Correlate caller intent, operations, outcomes, revisions and eventual sync results; describe limitations of claimed actor identity or tamper resistance. |
| NFR-05 — Extensibility | Document metadata operations are independent of storage. The authoritative Git transport and optional satellite connector capabilities are separated and never advertised as interchangeable. |
| NFR-06 — Security | Treat paths, document content, config, and provider input as untrusted; avoid path escapes, credential exposure and unauthorized operations. |
| NFR-07 — Testability | Verify operations, offline recovery, audit and synchronization against representative Markdown samples and backend capabilities. |
| NFR-08 — Usability | CLI outputs should be predictable for humans and agents; composite responses should be machine-readable. |
| NFR-09 — Performance | Avoid redundant downloads and unnecessary content transfer; set quantitative targets after a representative prototype. |

## 6. Acceptance scenarios to refine

| ID | Scenario | Expected observable behavior |
| --- | --- | --- |
| AC-01 | Configure two slugged projects with different authoritative remote Git repositories. | Each project resolves to its own local managed Git state and corresponding Git hub without conflating histories. |
| AC-02 | Disconnect after initial synchronization and read a known document. | Its local Git version remains readable; Git hub publication/freshness limits are exposed. |
| AC-03 | Modify a known document while offline; restart awl before reconnecting. | Change and its audit record survive; synchronization remains pending. |
| AC-04 | Send multiple body-only edits for one document, including a stale-base edit with a safe exact unique match. | The resulting document retains frontmatter unchanged; safe edits can apply without forced reread, and ambiguous/unsafe ones return explicit errors. |
| AC-05 | Run a composite across several documents and cause its middle operation to fail. | Later operations still execute; all operation results are reported; one local Git commit contains the successful changes, if any. |
| AC-06 | Exchange commits after offline changes and new authoritative Git commits. | The shared Git history is reconciled or an explicit conflict is returned; no silent overwrite. Detailed rules remain open. |
| AC-07 | Edit nested YAML metadata through JSON Pointer, including objects and nested lists, without changing the body. | Pointer escaping and zero-based indexes resolve precisely; replacement never inserts; array insert/append/remove and unsafe stale-position conflicts are explicitly handled without rewriting unrelated content. |
| AC-08 | Read legacy AgentWorklog Markdown with wikilinks and old status conventions. | Reads do not trigger implicit migration or normalization. |
| AC-09 | Access audit and sync status for a composite. | Caller can distinguish local Git commit, authoritative Git publication, and optional satellite propagation. |
| AC-10 | Attempt direct modification outside project scope. | Operation is rejected, and authorization boundaries are enforced where technically possible. |
| AC-11 | Configure a Google Drive satellite for a Git-backed project and project an accepted Git revision. | The integration reports its own projection progress and failures without changing the authoritative Git revision; reverse ingestion is not presumed. |

## 7. Architectural evaluation and open requirements

See [Local-first Git hub-and-spoke architecture](../architecture/2026-10-23-local-first-storage-evaluation.md), the [draft document and operation contract](document-operation-contract.md), and [YAML path research](../research/2026-10-23-yaml-paths-and-partial-reads.md), plus [partial-read best-practices research](../research/2026-10-23-partial-read-agent-conventions.md).

**Owner-selected architectural direction, not an implemented or merged design:**
- One authoritative Git remote per project, with optional secondary integrations connected in a hub-and-spoke topology.
- Offline-first managed local Git replica.
- Controlled API, generic metadata, auditable operations and composites.
- Google Drive is a candidate secondary satellite integration, not an alternative authoritative remote.

**Open, explicitly deferred:**
- Direction of satellite synchronization: initial export-only projection versus optional import of externally authored changes into Git; execution location for satellite workers.
- Local Git persistence implementation: command-line Git versus library, private object/worktree model, commit granularity, retention, and possible additional audit journal.
- Git remote publication and scheduling, initial clone/bootstrap, partial history, and separate satellite projection semantics and freshness.
- Conflict detection, merge policy, stale edits, external writer behavior, simultaneous creates, uncertain remote writes.
- Whether full application audit history must be synchronized through Git and independently verifiable across users.
- Precise fatal request-level error and interruption behavior, individual document edit atomicity, request idempotency and durable audit for failed/no-op requests.
- Detailed stale body-edit safety criteria, stronger preconditions for stale indexed YAML writes, strict optional preconditions, read range convention, and whether context, three-way merge or line/hash anchors are used.
- Slug grammar/global namespace; project configuration distribution; remote Git repository/ref mapping and optional satellite destination mapping.
- Credentials, security model, actor identity, sharing, and key lifecycle.
- Git remote provider requirements and separate capabilities, deployment needs, and limitations of satellite integrations.
- Public names/schema for JSON Pointer-based metadata operations, indexed-list preconditions, YAML formatting/alias preservation; read-range coordinate conventions and whether the CLI itself is initially the only interface.
- Local snapshot lifecycle, retention, garbage collection, portability and backup.

## 8. Compatibility and migration

Current AgentWorklog documents remain user-owned. The existing vault convention and structure are compatibility inputs rather than an already approved CLI schema. Migration of legacy frontmatter, wikilinks or directory layouts must be separately scoped and authorized. No backend migration or file rewriting is implied by this proposal.

## 9. Evidence and status

- Current implementation status: no awl CLI implementation; the GitHub default branch contained LICENSE only at inspection time (2026-10-23).
- Existing working requirements draft: AgentWorklog/awl/Specs/2026-10-23-core-requirements-spec.md in the connected Google Drive vault.
- Format reference: https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/main/SPEC.md (OKF v0.2).
- Repository-local durable requirements are **proposed in this draft PR**, not merged or approved.

## 10. Next review gate

Review this revised contract for managed local Git, one authoritative remote Git hub per project, and optional satellite integrations. Then scope focused experiments for Git publication, audit persistence, satellite projections and editing safety before selecting libraries or deployment tooling.
