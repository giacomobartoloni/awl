# AgentWorklog CLI — Core Requirements (Proposed Revision)

**Status:** draft; review required; owner-selected Git hub-and-spoke topology.  
**Date:** 2026-10-23.  
**Product:** AgentWorklog CLI (awl), distinct from the existing AgentWorklog Markdown conventions.  
**Provenance:** Consolidates the working vault draft at AgentWorklog/awl/Specs/2026-10-23-core-requirements-spec.md and the follow-up product discussion. This is a proposed replacement baseline, not an approved specification and not an implemented feature.

## 1. Purpose

Provide a project-oriented, offline-first interface for reading, editing, indexing, and searching Markdown documents, while preserving generic YAML-frontmatter document operations. AgentWorklog clients use awl's controlled API rather than directly editing the managed local replica. Changes can be made while offline and published to one authoritative Git remote per project. Optional satellite integrations (starting with Google Drive as a candidate) connect to that remote Git hub. Satellite synchronization direction and mechanics remain open.

## 2. Agreed product boundaries

- A project is identified by a slug. The slug need not equal a repository or folder name.
- Each project uses managed local Git and one authoritative **remote Git repository** as its shared history. One or more optional secondary integrations, including Google Drive as an initial candidate, connect to the remote Git hub. These are not co-equal authoritative backends.
- A durable, locally managed **Git-based document replica** supports offline reads and changes; Git hub publication is **logically deferred** from local request acceptance. **Owner-confirmed 2026-11-06:** initial remote synchronization and derived index refresh are **explicit, recoverable invocations**, not automatic background work. Optional satellite projections can lag independently.
- Clients access documents through awl. Direct filesystem mutation of its managed local state is not a supported editing interface. This is an API and audit boundary, not a claim of OS-level tamper resistance.
- Markdown files remain portable, with YAML frontmatter as the metadata serialization. New AgentWorklog conventions target Open Knowledge Format (OKF) v0.2; legacy files are user-owned and must not be silently migrated.
- OKF lifecycle status (draft, stable, deprecated) is distinct from the AgentWorklog extension workflow-status. No implicit mapping.
- Standard Markdown links are preferred for new links. Existing Obsidian links require a separately authorized migration.
- Document types and custom metadata keys have no fixed domain semantics in the CLI core. No Activity, Spec, Plan, or ProductPlan lifecycle commands are required.
- Single requests and composites may include multiple document and metadata operations. Composites run operations in order, **continue after member errors**, and report every outcome. **One local Git content commit per request with successful modifications** records the resulting changes; no multi-request/distributed rollback requirement is implied.
- Every applied change is associated with an auditable request/operation identity. Git commits may provide version history but do not by themselves capture attempted/failed operations or prove caller identity. Shared or tamper-resistant audit remains open.
- Project-scoped document indexing and lexical/full-text, semantic, and hybrid retrieval are **core capabilities**. Structured YAML metadata indexing/query/filtering, shared _research/_guides handling, a mandatory server, and a required synchronization daemon are not in the initial scope. No search engine or dependency has been selected.

### Source repository and awl configuration (owner-confirmed 2026-11-04)

- A source-code repository is an optional context for an awl project; the source repository and awl's managed worklog Git repository are distinct. Nothing here requires all awl projects to be Git source repositories.
- When a source repository is used, `.awl/project.yaml` is its shared, version-controllable awl project configuration; `.awl/local.yaml` is its personal, Git-ignored configuration entry point.
- All linked Git worktrees of the same source repository must resolve one shared effective **local awl configuration** and one managed local awl Git repository on the same machine. No per-worktree awl repository or file-replication service is required.
- The authoritative personal configuration is stored once under the **source Git common directory**. `.awl/local.yaml` may expose that single file through an optional symlink; awl must work without the symlink. A separate ordinary local.yaml in a worktree must not silently override the common file.
- Managed local worklog Git repositories live by default under the user's application-data directory, with an explicit personal storage-path override. A project slug alone must not silently conflate distinct project histories.
- Git's effective user identity is the default when operating in a source repository; awl may explicitly override it in local configuration. awl must not modify Git user configuration. Git commit attribution, identity provenance, and the effect of worktree-specific Git identity settings still need a precise contract.
- The `.awl/local.yaml` convention must be excluded from source Git tracking. Project configuration must not carry machine-specific paths, personal identities, or secrets by default. No background configuration synchronization, watcher, account system, or extra registry is required by this decision.
- Differences between branch-specific `.awl/project.yaml` versions must not silently redirect an operation to an incompatible awl project or managed repository.

These are owner-confirmed product/configuration boundaries, not claims of implemented behavior. Exact YAML schema, config CLI, platform paths, initialization, link handling, and conflict/error representations remain design work.

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
| FR-04 | Core | Offer project-scoped listing, full and bounded document reads, creation and modification through awl without direct access to managed replica files. **Approved read behavior:** 1-based absolute physical-file line coordinates including frontmatter, optional pinned local Git revision across pages, and explicit truncation/continuation. Quantitative limits and public syntax remain open. |
| FR-05 | Core | Support generic, typed YAML-frontmatter reads and mutations using **RFC 6901 JSON Pointer** for single-node addressing. Distinguish replacement, insertion at a list index, append, and removal; preserve unknown keys and unaffected content and reject unverifiable stale positional list edits. |
| FR-06 | Core | Support LLM-agent-friendly **body-only** Markdown editing, including exact targeted replacement with explicit ambiguity/failure results; awl resolves current revisions internally, and callers need not pass any file revision. The final patch syntax is not yet selected. |
| FR-07 | Core | Accept several ordered edits within a single document-modification request and provide a clear document-level result. |
| FR-08 | Core | Support ordered composites that **continue after individual operation failures**, reporting every outcome and a request-level correlation identifier. Commit successfully applied changes once per request when content changes. |
| FR-09 | Core | Preserve locally accepted modifications in managed Git durably across restarts; do not silently discard changes awaiting publication to the authoritative Git remote. |
| FR-10 | Core | Keep a durable, shared, operation-level audit in **Git commit messages** for locally completed mutating requests, including ordered member failures, request identity, result and actor provenance (not authenticated identity). A content-changing completed request produces **one content commit with audit**; a completed nonconcurrency all-failed or no-op mutating request produces **one audit-only commit**, with unchanged document tree. **Owner-confirmed exception (2026-11-06):** a request rejected before publication for an unsafe concurrent local conflict produces **no commit, including no audit-only commit**, and no durable shared-Git audit event; report the rejection to the caller. Do not infer audit intent/outcomes from diffs alone. Exact audit schema, scope of pre-execution invalid requests and retention remain review gates. |
| FR-11 | Core | Support exchange of local Git commits and shared history with the project's authoritative Git remote, distinguishing locally committed changes from remotely accepted changes. Satellite synchronization behavior is separately scoped. |
| FR-12 | Core | Expose local commit, Git hub publication, and per-satellite propagation status where applicable; errors, unresolved changes and conflicts must be visible without silent overwrites. **Initial execution policy (owner-confirmed 2026-11-06):** local accepted mutations never wait on remote Git synchronization or search indexing; these are **explicitly invoked, recoverable, idempotent-or-safe-to-retry operations** that can be run by a later awl process. No automatic background worker, required daemon or periodic scheduling is promised. |
| FR-13 | Core | Retain existing file contents and links unless an explicit modification succeeds. Never silently normalize, rewrite, move, overwrite, or delete user-owned Markdown. |
| FR-14 | Core | Produce explicit per-operation error outcomes for nonexistent or ambiguous targets, malformed YAML, unavailable permissions, unsupported actions, and modifications that cannot be reconciled safely. |
| FR-15 | Core | Constrain access to configured project scope and provider authorization; do not expose provider credentials or unrestricted local storage access. |
| FR-16 | Core | Support nonsemantic operations on arbitrary document types and frontmatter fields; neither OKF nor worklog-specific fields are interpreted as mandatory workflow rules. |
| FR-17 | Core | Make failure/restart recovery observable; global cross-document atomicity, distributed rollback, and ACID transactions are not required. |
| FR-18 | Later | Offer a remote-agent/MCP interface using the same application operations, without defining the wire protocol now. |
| FR-19 | Core | Build and maintain a searchable, project-scoped index of managed Markdown documents as **derived state**, without mutating original content or replacing Git as document authority. Support recovery/rebuild of derived index state. |
| FR-20 | Later | Support additional backend integrations as satellites of the authoritative remote Git hub, beginning with a Google Drive projection candidate; whether they support external-to-Git ingestion remains open. |
| FR-21 | Core | Expose project-scoped **lexical/full-text, semantic, and hybrid document retrieval** with grounded document identities and useful excerpts. Mode-specific model provisioning, ranking, limits and public syntax require design and validation. |
| FR-22 | Core | Make index availability and freshness relative to managed local document revisions observable, including pending, stale, failed, or rebuild-required states; never misrepresent old index hits as current Git content. Initial index refresh is **explicitly invoked** and recoverable; do not require index refresh to complete before acknowledging a local Git commit. Avoid redundant content reindexing on audit-only commits whose document tree is unchanged. |
| FR-23 | Later | Structured frontmatter metadata indexing, filtering, discovery, and queries; optional OKF/project validation and structural Markdown editing. Generic YAML metadata **document operations** remain in scope via FR-05. |
| FR-24 | Core | In a source Git repository, support version-controllable `.awl/project.yaml` and a Git-ignored `.awl/local.yaml` entry point, resolving one authoritative personal configuration via the source repository's Git common directory for all linked worktrees. No replication or mandatory symlink. |
| FR-25 | Core | All worktrees of a source repository resolve one managed local awl Git repository in user-level application data unless explicitly overridden. Reject ambiguous project identity or unsafe storage redirection rather than silently combining histories. |
| FR-26 | Core | Use the effective Git user identity by default, with explicit awl-local overrides and no mutation of Git configuration. Preserve actor provenance without treating Git author fields as authenticated identity. |
| FR-27 | Core | Document creation/editing, generic YAML metadata mutations, and composite request members must not require or accept caller-supplied file revision references, commit SHA, `base_revision` or `expected_revision`. awl resolves Git revisions, validates content intent, and handles safe internal concurrency retries; unsafe semantic conflicts remain explicit. |
| FR-28 | Core | Deliver awl first as an independent shell-invocable CLI usable by humans and coding agents across different harnesses. Support non-interactive execution, predictable human and machine-readable outcomes, actionable domain errors and reliable process exit statuses without requiring a daemon, MCP server, SDK, harness-specific tool wrapper, or direct Git commands from the caller. Concrete command names, flags, request serialization and exit-code numbers remain unapproved. |
| FR-29 | Core | **Owner-confirmed 2026-11-06:** If concurrent modifications make final local publication unsafe, reject the **whole** submitted request without publishing **any** of its staged document changes **or any commit for that rejected request**, including audit-only commits; report a machine-readable conflict so the coding agent can reread and resubmit updated intent. Internal revision checks and safe retries belong to awl; the caller need not send a document revision. Ordinary composite member failures still continue under FR-08. A later Git remote push conflict does not undo a request already locally acknowledged. |

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
| NFR-08 — Usability | CLI-first and harness-agnostic: commands can run non-interactively through ordinary shell tools; humans see concise output, automation can request stable machine-readable outcomes, and stdout/stderr plus exit codes distinguish full success, partial success and failure without exposing internal Git mechanics. |
| NFR-09 — Performance | Avoid redundant downloads and unnecessary content transfer; set quantitative targets after a representative prototype. |
| NFR-10 — Search safety | Indexes are disposable derived data; indexing and querying must not rewrite Markdown or leak files outside authorized project scope. |
| NFR-11 — Search resilience | Offline lexical retrieval must be possible on initialized local state; semantic/hybrid model setup, offline behavior, degradation and resource budgets need explicit contracts and measurement. |

## 6. Acceptance scenarios to refine

| ID | Scenario | Expected observable behavior |
| --- | --- | --- |
| AC-01 | Configure two slugged projects with different authoritative remote Git repositories. | Each project resolves to its own local managed Git state and corresponding Git hub without conflating histories. |
| AC-02 | Disconnect after initial synchronization and read a known document. | Its local Git version remains readable; Git hub publication/freshness limits are exposed. |
| AC-03 | Modify a known document while offline; restart awl before reconnecting. | Change and its audit record survive; synchronization remains pending. |
| AC-04 | Submit multiple body-only edits without any file revision argument, even after a stale prior read. | awl evaluates against current managed content, preserves frontmatter, and safely applies valid targets without a caller-controlled revision; ambiguous/unsafe targets yield explicit document errors. |
| AC-05 | Run a composite across several documents and cause its middle operation to fail. | Later operations still execute; all operation results are reported; one local Git commit contains the successful changes, if any. |
| AC-06 | Exchange commits after offline changes and new authoritative Git commits. | The shared Git history is reconciled or an explicit conflict is returned; no silent overwrite. Detailed rules remain open. |
| AC-07 | Edit nested YAML metadata through JSON Pointer, including objects and nested lists, without changing the body. | Pointer escaping and zero-based indexes resolve precisely; replacement never inserts; array insert/append/remove and unsafe stale-position conflicts are explicitly handled without rewriting unrelated content. |
| AC-08 | Read legacy AgentWorklog Markdown with wikilinks and old status conventions. | Reads do not trigger implicit migration or normalization. |
| AC-09 | Access audit and sync status for a composite. | Caller can distinguish local Git commit, authoritative Git publication, and optional satellite propagation. |
| AC-10 | Attempt direct modification outside project scope. | Operation is rejected, and authorization boundaries are enforced where technically possible. |
| AC-11 | Configure a Google Drive satellite for a Git-backed project and project an accepted Git revision. | The integration reports its own projection progress and failures without changing the authoritative Git revision; reverse ingestion is not presumed. |

Owner-confirmed partial-read acceptance direction: AC-12 — 1-based full-file lines include frontmatter; AC-13 — an optional pinned local revision stays stable across pages; AC-14 — explicit incomplete/truncated result with continuation information on a size limit. Exact limits, public fields and long-line fallback remain to be designed.

New search acceptance candidates (2026-11-04; **not executed**):

| ID | Scenario | Expected observable behavior |
| --- | --- | --- |
| AC-15 | Search documents in a locally initialized project with an exact phrase and a conceptual paraphrase. | Lexical and semantic/hybrid modes are addressable and return grounded project-relative document references. |
| AC-16 | Modify a document through awl while an index already exists. | Results and index status distinguish current content from pending/stale indexing and support repair/rebuild. |
| AC-17 | Search offline after initial local index/model setup. | Supported modes work without unannounced remote access; unavailable semantic models are reported explicitly. |
| AC-18 | Attempt a query that filters by arbitrary YAML-frontmatter fields. | Structured metadata filtering is clearly unsupported in the initial search contract, never silently ignored. |
| AC-19 | Index legacy Markdown with YAML frontmatter and existing Obsidian links. | Source documents are not rewritten, normalized, or migrated by indexing. |

Configuration acceptance candidates (2026-11-04; **not implemented or tested in awl**):

| ID | Scenario | Expected observable behavior |
| --- | --- | --- |
| AC-20 | Invoke awl from a main worktree and a linked worktree of one source repository. | Both resolve the same authoritative personal awl config and managed local worklog Git repository, independently of their working-tree paths. |
| AC-21 | Use `.awl/local.yaml` with a symlink in one worktree and without a symlink in another. | Both use the common-directory configuration; no copy or background synchronization is required. |
| AC-22 | Git provides a user name/email, then awl-local identity overrides them. | Git identity is used by default; configured overrides take precedence without changing Git config; missing identity is reported when required for a commit. |
| AC-23 | A branch's `.awl/project.yaml` changes the awl project identity or repository target incompatibly. | awl signals the mismatch before writing; it does not silently redirect or merge unrelated histories. |
| AC-24 | Two concurrent awl processes use different worktrees of the same source repository. | Managed local repository operations preserve integrity and report incompatible concurrent changes; no duplicate per-worktree awl repository is created. |
| AC-25 | A client edits a document or YAML metadata without reading it first and without providing a file revision. | awl obtains the current revision internally, validates explicit operation intent and either applies safely or reports a content-level error; no revision-token prompt. |
| AC-26 | Two agents concurrently edit disjoint documents without revision references. | awl handles recoverable local revision races transparently through safe revalidation and bounded internal attempts, without surfacing Git CAS details. |
| AC-27 | A coding agent invokes awl through an authorized shell from a source worktree, without an MCP server or custom integration. | awl resolves the intended source/project context and returns a deterministic CLI result without interactive prompts or manual Git coordination. |
| AC-28 | A coding agent submits a multi-line or composite document change non-interactively. | The CLI has a safe machine-oriented input mechanism and produces ordered, machine-readable member results, with no mandatory file revision arguments. Exact input transport and flag spelling remain proposals. |
| AC-29 | A CLI operation fails or partially succeeds. | An agent can distinguish the outcome using documented process exit status and structured diagnostics, without scraping Git stderr; local persistence and remote publication are reported independently. |

Request-level concurrency acceptance candidates (2026-11-06; owner-confirmed behavior, **not implemented or tested**):

| ID | Scenario | Expected observable behavior |
| --- | --- | --- |
| AC-30 | A composite stages changes to A and C, but another process changes its target in B incompatibly before local publication. | Reject the entire request; no staged document changes to A, B or C become visible; the managed Git head does **not** advance due to that request, including for audit; return conflict context for agent reread/resubmit. |
| AC-31 | A composite contains an ordinary invalid member, with other valid members and no concurrent publication conflict. | Continue in order; record all member outcomes and commit the successful changes once. |
| AC-32 | Two requests started on the same head change unrelated documents; one publishes first. | The other may safely revalidate/rebuild and publish, or return a truthful retryable conflict; never silently clobber an acknowledged change. |
| AC-33 | A request is already committed locally, but synchronization encounters remote Git divergence. | Retain the local commit and audit; report remote pending/conflicted separately, without claiming the local request was aborted. |

**Owner-confirmed 2026-11-06:** a locally completed mutating request with no document changes (ordinary all-failed or valid no-op) still creates exactly **one Git audit-only commit**; the user document tree remains unchanged. Requests completed with changes use one content+audit commit. A fatal concurrent publication conflict remains the explicit **no-commit** exception. Read-only calls, rejected pre-execution malformed requests and ordinary network retries do not create audit commits under the initial proposed scope; acceptance boundaries and exact serialization require review. See [request-level conflict design proposal](../architecture/2026-11-06-request-conflict-rejection-proposal.md).

### Owner-selected implementation baseline (2026-11-06)

For the **first implementation increment**, the owner selected **Go** as the CLI language/runtime and the **external Git CLI** as the Git integration mechanism (not an embedded Git engine). Go is required for building, but not as an installed runtime for users of an appropriately built binary; Git must be installed in supported execution environments. Invoke Git with structured argument vectors, never through shell interpolation. Git version compatibility, distribution targets, platform-specific lock primitives, durability/fsync profile, process environment controls, concrete CLI grammar and dependency choices remain to be validated; no implementation is present.

These are explicit owner selections, **not implemented or demonstrated by the prior Python-only Git fixtures**. They do not override the local-first managed-Git/remote-Git-hub direction or choose a Git library, daemon, provider-neutral storage engine or micro-worktree mandate. See [request publication proposal](../architecture/2026-11-06-request-conflict-rejection-proposal.md).

### Deferred execution contract (owner-selected 2026-11-06)

- **Local acceptance:** finish validation, final audited Git commit (including audit-only commits for locally completed no-op/all-failed mutating requests) and guarded ref publication before replying. Return local success independently of downstream Git/QMD status. Fatal unsafe concurrent request conflicts still return **no commit**.
- **Remote Git sync:** a separately invoked operation performs fetch, conservative reconciliation, push and remote confirmation. Never assume `git pull` or network success; a failed attempt leaves locally acknowledged history intact for an explicit later retry.
- **Search index refresh:** a separately invoked operation updates a read-only controlled projection of the **published** managed Git document tree and any derived index (QMD is still an evaluation candidate, not approved). Expose freshness/checkpoints; index failures cannot undo local changes. Embedding refresh may have a separate availability state.
- **Recovery and execution:** an invocation may block its **own caller** while running the operation; "deferred" is relative to the local mutation request, **not a promise that it will execute in the background after the CLI exits**. A subsequent explicit invocation resumes/retries from Git history and derived state. A durable per-request job queue, required worker, cron, watcher and daemon are **not selected** for the first increment. Exact commands, policies and automatic future scheduling remain open.

Acceptance additions (proposed; no implementation tests):

| ID | Scenario | Expected observable behavior |
| --- | --- | --- |
| ASYNC-AC-01 | Complete a local changing or audit-only request while network is unavailable and QMD is not installed | Local request is confirmed if local Git publication succeeds; remote/index remain pending/unavailable, without blocking or rollback |
| ASYNC-AC-02 | Explicitly invoke sync later after local commits have accumulated | Safe attempt advances or reports conflict/pending, with idempotent retry; no new mutating-request audit is created by a network retry |
| ASYNC-AC-03 | Explicitly refresh index after multiple changing commits | Only committed document content is projected; resulting index revision is marked current only when fully verified |
| ASYNC-AC-04 | Locally complete an audit-only commit with the same document tree | Remote sync may remain pending; content reindexing need not recur solely because the commit SHA changed |
| ASYNC-AC-05 | End the CLI process without invoking sync or index refresh | Neither operation is falsely reported as running or completed; later invocation can determine needed work |
| ASYNC-AC-06 | Two agents invoke the same sync/refresh concurrently | Operations are safely serialized or coalesced per project; no corruption and no false completed status |

## 7. Architectural evaluation and open requirements

See [Local-first Git hub-and-spoke architecture](../architecture/2026-10-23-local-first-storage-evaluation.md), the [draft document and operation contract](document-operation-contract.md), [QMD search/indexing assessment](../research/2026-11-04-qmd-search-and-indexing-assessment.md), [YAML path research](../research/2026-10-23-yaml-paths-and-partial-reads.md), and [partial-read best-practices research](../research/2026-10-23-partial-read-agent-conventions.md).

**Owner-selected architectural direction, not an implemented or merged design:**
- One authoritative Git remote per project, with optional secondary integrations connected in a hub-and-spoke topology.
- Offline-first managed local Git replica.
- Controlled API, generic metadata document operations, auditable operations and composites.
- Indexing, lexical/full-text retrieval, semantic retrieval, and hybrid retrieval as core document capabilities; QMD is an **evaluation candidate**, not a selected implementation.
- Google Drive is a candidate secondary satellite integration, not an alternative authoritative remote.

**Open, explicitly deferred:**
- Direction of satellite synchronization: initial export-only projection versus optional import of externally authored changes into Git; execution location for satellite workers.
- Local Git persistence implementation: command-line Git versus library, private object/worktree model, commit granularity, retention, and possible additional audit journal.
- Git remote conflict/reconciliation policy, initial clone/bootstrap, partial history, separate satellite projection semantics and freshness. **Initial remote publication and indexing trigger policy is no longer open:** explicit recoverable invocation without automatic worker; later scheduling remains deferred.
- Exact safe-retry criteria and Git ref-conflict implementation; **owner-selected** fatal request-level concurrency conflict behavior is now recorded in FR-29 and AC-30–AC-33. Remote merge policy, external writer behavior, simultaneous creates and uncertain remote writes remain open.
- Whether full application audit history must be synchronized through Git and independently verifiable across users.
- Precise fatal request-level error and interruption behavior, individual document edit atomicity and request idempotency. **Durable Git audit-only commits for completed failed/no-op mutating requests are now owner-confirmed**, including the no-commit concurrent-rejection exception; exact schema, pre-execution rejection scope and retention still need review.
- Detailed stale body-edit safety criteria; content/element-based conditions for indexed YAML writes; whether context, three-way merge or line/hash anchors are used. Caller-supplied file revision preconditions for mutations are **not allowed**; optional revision-pinned reads remain supported. Quantitative read limits and error edge cases are still open.
- Slug grammar/global namespace; project configuration distribution; remote Git repository/ref mapping and optional satellite destination mapping.
- Credentials, security model, actor identity, sharing, and key lifecycle.
- Git remote provider requirements and separate capabilities, deployment needs, and limitations of satellite integrations.
- Public names/schema for JSON Pointer-based metadata operations, indexed-list preconditions and YAML formatting/alias preservation; read response serialization and quantitative bounds (the read coordinate model is approved); whether the CLI is initially the only interface.
- Local snapshot lifecycle, retention, garbage collection, portability and backup.
- Search-result contract, text scope (body only versus frontmatter as plain text), index revision/freshness representation, refresh triggers, ranking, model bootstrap, offline failures, and cross-project scope.
- Search engine choice and integration boundary. No QMD, model runtime, database schema or mandatory daemon is approved; structured metadata search stays deferred.

## 8. Compatibility and migration

Current AgentWorklog documents remain user-owned. The existing vault convention and structure are compatibility inputs rather than an already approved CLI schema. Migration of legacy frontmatter, wikilinks or directory layouts must be separately scoped and authorized. No backend migration or file rewriting is implied by this proposal.

## 9. Evidence and status

- Current implementation status: no awl CLI implementation; the GitHub default branch contained LICENSE only at inspection time (2026-10-23).
- Existing working requirements draft: AgentWorklog/awl/Specs/2026-10-23-core-requirements-spec.md in the connected Google Drive vault.
- Format reference: https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/main/SPEC.md (OKF v0.2).
- Repository-local durable requirements are **proposed in this draft PR**, not merged or implemented.
- **Owner-confirmed scope (2026-11-04):** indexing and lexical/semantic/hybrid search are core; structured metadata search and filtering are deferred; QMD is being assessed, not selected.

## 10. Next review gate

Review this revised contract for managed local Git, one authoritative remote Git hub per project, and optional satellite integrations. Validate a minimal search/indexing contract and evaluate a pinned QMD build on representative Markdown without metadata filters; then scope focused experiments for Git publication, audit persistence, satellite projections and editing safety before selecting implementation libraries or deployment tooling.
