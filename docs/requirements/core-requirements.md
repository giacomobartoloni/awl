# AgentWorklog CLI — Core Requirements (Proposed Revision)

**Status:** draft; review required.  
**Date:** 2026-10-23.  
**Product:** AgentWorklog CLI (awl), distinct from the existing AgentWorklog Markdown conventions.  
**Provenance:** Consolidates the working vault draft at AgentWorklog/awl/Specs/2026-10-23-core-requirements-spec.md and the follow-up product discussion. This is a proposed replacement baseline, not an approved specification and not an implemented feature.

## 1. Purpose

Provide a project-oriented, offline-first interface for reading and editing Markdown documents and their generic YAML-frontmatter metadata. AgentWorklog clients use awl's controlled API rather than directly editing the managed local replica. Changes can be made while offline and synchronized in both directions with one configured remote backend for each project.

## 2. Agreed product boundaries

- A project is identified by a slug. The slug need not equal a repository or folder name.
- Each project has exactly **one remote backend** at a time. Git and Google Drive are initial backend candidates, not yet implemented. Separate projects may use separate providers.
- A durable, locally managed document replica supports offline reads and changes; remote synchronization is asynchronous with respect to a local change.
- Clients access documents through awl. Direct filesystem mutation of its managed local state is not a supported editing interface. This is an API and audit boundary, not a claim of OS-level tamper resistance.
- Markdown files remain portable, with YAML frontmatter as the metadata serialization. New AgentWorklog conventions target Open Knowledge Format (OKF) v0.2; legacy files are user-owned and must not be silently migrated.
- OKF lifecycle status (draft, stable, deprecated) is distinct from the AgentWorklog extension workflow-status. No implicit mapping.
- Standard Markdown links are preferred for new links. Existing Obsidian links require a separately authorized migration.
- Document types and custom metadata keys have no fixed domain semantics in the CLI core. No Activity, Spec, Plan, or ProductPlan lifecycle commands are required.
- Single requests and composites may include multiple document and metadata operations. Composites need ordered, individually reported results, not global atomicity or rollback.
- Every applied change is associated with an auditable request/operation identity. Shared or tamper-resistant audit remains an open requirement.
- No metadata query engine, shared _research/_guides handling, mandatory server, or synchronization daemon is assumed for the initial slice.

## 3. Terminology

- **Project:** Slug-addressed context with a configured remote backend and a local managed replica.
- **Document:** A Markdown resource addressed by project slug and project-relative path.
- **Document metadata:** Arbitrary YAML-frontmatter values, independent of backend metadata.
- **Local revision:** An identifier for the locally committed document state; representation TBD.
- **Remote revision:** The backend's own relevant state indicator, if any.
- **Operation:** An individual document or metadata action.
- **Composite:** An ordered collection of operations in one request, possibly spanning multiple documents.
- **Audit record:** A record of request intent, actor information supplied or authenticated, per-operation results, and resulting revisions.
- **Sync state:** Observable status distinguishing locally applied, pending, synchronized, and blocked changes. Exact state names TBD.

## 4. Functional requirements

Priority is indicative: Core means candidate first increment, Later means explicitly deferred. No release plan is implied.

| ID | Priority | Requirement |
| --- | --- | --- |
| FR-01 | Core | Discover locally configured accessible projects and resolve a project by its slug; reject ambiguous resolution. |
| FR-02 | Core | Associate a project with exactly one configurable remote backend, while allowing different projects to use different backends. |
| FR-03 | Core | Retain a locally durable, managed replica for a project and support reads and writes to already available local documents when disconnected. |
| FR-04 | Core | Offer project-scoped listing, full document reads, creation, and modification through awl without requiring direct access to managed replica files. |
| FR-05 | Core | Allow generic frontmatter reads and typed metadata set, remove, and grouped changes; preserve unknown keys and unaffected content. |
| FR-06 | Core | Support content editing operations usable by LLM agents, including exact targeted replacement with explicit ambiguity/failure results. The final patch protocol is not yet selected. |
| FR-07 | Core | Accept several ordered edits within a single document-modification request and provide a clear document-level result. |
| FR-08 | Core | Support a composite containing multiple document and metadata operations, with per-operation outcomes and a request-level correlation identifier. |
| FR-09 | Core | Preserve the outcome of locally accepted modifications durably across application restarts; do not silently discard pending remote changes. |
| FR-10 | Core | Record an auditable local history of attempted and applied operations and their outcomes, including actor information as available, without treating ordinary Git history as a complete audit by itself. |
| FR-11 | Core | Support bidirectional synchronization of the managed local replica with the configured remote provider; distinguish local success from remote synchronization. |
| FR-12 | Core | Expose sync state, pending work, and actionable errors; no invisible last-writer-wins policy or silent loss of conflicting changes. |
| FR-13 | Core | Retain existing file contents and links unless an explicit modification succeeds. Never silently normalize, rewrite, move, overwrite, or delete user-owned Markdown. |
| FR-14 | Core | Produce explicit per-operation error outcomes for nonexistent or ambiguous targets, malformed YAML, unavailable permissions, unsupported actions, and modifications that cannot be reconciled safely. |
| FR-15 | Core | Constrain access to configured project scope and provider authorization; do not expose provider credentials or unrestricted local storage access. |
| FR-16 | Core | Support nonsemantic operations on arbitrary document types and frontmatter fields; neither OKF nor worklog-specific fields are interpreted as mandatory workflow rules. |
| FR-17 | Core | Make failure/restart recovery observable; global cross-document atomicity, distributed rollback, and ACID transactions are not required. |
| FR-18 | Later | Offer a remote-agent/MCP interface using the same application operations, without defining the wire protocol now. |
| FR-19 | Later | Optional OKF or project-defined validation, indexing, metadata query, or structural Markdown editing; no commitment to implement them initially. |

## 5. Non-functional requirements

| ID | Requirement |
| --- | --- |
| NFR-01 — Data integrity | A confirmed local change must be durable and recoverable; unexpected partial results, ambiguous states, and failed synchronizations must be explicit. |
| NFR-02 — Portability | Content remains standard UTF-8 Markdown and YAML frontmatter without mandatory Obsidian plugins or proprietary document formats. |
| NFR-03 — Offline usability | Already synchronized material is available and editable offline without hidden online preconditions. |
| NFR-04 — Auditability | Correlate caller intent, operations, outcomes, revisions and eventual sync results; describe limitations of claimed actor identity or tamper resistance. |
| NFR-05 — Extensibility | Document metadata and backend differences are isolated from common application operations, with provider-specific capabilities surfaced rather than fabricated. |
| NFR-06 — Security | Treat paths, document content, config, and provider input as untrusted; avoid path escapes, credential exposure and unauthorized operations. |
| NFR-07 — Testability | Verify operations, offline recovery, audit and synchronization against representative Markdown samples and backend capabilities. |
| NFR-08 — Usability | CLI outputs should be predictable for humans and agents; composite responses should be machine-readable. |
| NFR-09 — Performance | Avoid redundant downloads and unnecessary content transfer; set quantitative targets after a representative prototype. |

## 6. Acceptance scenarios to refine

| ID | Scenario | Expected observable behavior |
| --- | --- | --- |
| AC-01 | Configure two slugged projects with different remote backend types. | Both are discoverable and addressable using the same awl operations. |
| AC-02 | Disconnect after initial synchronization and read a known document. | Its local version remains readable; freshness limitations are exposed. |
| AC-03 | Modify a known document while offline; restart awl before reconnecting. | Change and its audit record survive; synchronization remains pending. |
| AC-04 | Send multiple edits for one document. | The returned result identifies which requested edits were applied and what revision was produced; the document is never silently corrupted. |
| AC-05 | Run a composite across several documents and cause one operation to fail. | Per-operation results identify applied, failed, and unexecuted operations; no global rollback is promised. |
| AC-06 | Sync after offline changes and new remote changes. | Reconciliation is attempted or a visible conflict is returned; no silent overwrite. Detailed rules remain open. |
| AC-07 | Edit arbitrary nested YAML metadata without changing document body. | Metadata and unrelated content are retained; unsupported or ambiguous YAML is reported. |
| AC-08 | Read legacy AgentWorklog Markdown with wikilinks and old status conventions. | Reads do not trigger implicit migration or normalization. |
| AC-09 | Access audit and sync status for a composite. | Caller can determine local application outcome versus remote propagation status. |
| AC-10 | Attempt direct modification outside project scope. | Operation is rejected, and authorization boundaries are enforced where technically possible. |

## 7. Architectural evaluation and open requirements

See [Local-first architecture options](../architecture/2026-10-23-local-first-storage-evaluation.md).

**Selected product direction, not selected implementation:**
- Single remote backend per project.
- Offline-first managed replica.
- Controlled API, generic metadata, auditable operations and composites.
- Candidate remote providers: Git and Google Drive.

**Open, explicitly deferred:**
- Local persistence and version mechanism: Git, an embedded revision/journal store, CRDT, or other.
- Synchronization protocols and scheduling, including first checkout, partial replicas, and consistency/freshness semantics.
- Conflict detection, merge policy, stale edits, external writer behavior, simultaneous creates, uncertain remote writes.
- Whether audit history must be synchronized and independently verifiable across users.
- Composite failure policy and precise document-level edit atomicity.
- Base-revision/read identity and patch semantics; whether context, three-way merge or line/hash anchors are used.
- Slug grammar/global namespace; project configuration distribution; remote root/branch/folder mapping.
- Credentials, security model, actor identity, sharing, and key lifecycle.
- Minimum provider capability set and limitations.
- Whether the CLI itself is the initial only interface; format/conformance strictness.
- Local snapshot lifecycle, retention, garbage collection, portability and backup.

## 8. Compatibility and migration

Current AgentWorklog documents remain user-owned. The existing vault convention and structure are compatibility inputs rather than an already approved CLI schema. Migration of legacy frontmatter, wikilinks or directory layouts must be separately scoped and authorized. No backend migration or file rewriting is implied by this proposal.

## 9. Evidence and status

- Current implementation status: no awl CLI implementation; the GitHub default branch contained LICENSE only at inspection time (2026-10-23).
- Existing working requirements draft: AgentWorklog/awl/Specs/2026-10-23-core-requirements-spec.md in the connected Google Drive vault.
- Format reference: https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/main/SPEC.md (OKF v0.2).
- Repository-local durable requirements are **proposed in this draft PR**, not merged or approved.

## 10. Next review gate

Review the requirements delta introduced by offline-first local persistence and composites. Compare architectural alternatives; then scope focused experiments for backend synchronization, audit persistence and editing safety before picking implementation language or libraries.
