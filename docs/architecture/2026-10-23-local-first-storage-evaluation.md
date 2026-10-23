# Local-first Git Hub and Satellite Integrations — Architecture Evaluation

**Status:** draft architectural direction selected by the project owner; details unapproved and unimplemented.  
**Date:** 2026-10-23.  
**Scope:** AgentWorklog CLI (awl), not a change to existing Markdown worklog conventions.  
**Related requirements:** [Proposed Core Requirements](../requirements/core-requirements.md).

## 1. Architectural direction and provenance

The owner selected this topology after considering two previous alternatives: (A) a local persistence engine with Git or Google Drive as interchangeable remote backends, and (B) an embedded Git engine versus a custom snapshot/journal engine.

**Current direction:**
- awl uses **managed local Git** for offline document revisions and history;
- each project has **one authoritative remote Git repository** (or a defined project scope within such a repository; layout remains open);
- synchronization between local Git and the central remote Git provides the primary exchange of work;
- **optional satellite integrations** (Google Drive first candidate; other backends later) connect to the authoritative remote Git hub, not directly to every local replica or to each other;
- clients interact with awl's controlled document, metadata and composite operation API, not with writable managed repository files.

This **supersedes** the earlier proposal of "one remote backend per project, either Git or Google Drive." Instead: one authoritative **Git** remote per project **plus zero or more secondary integrations**. The satellites do not become co-equal authorities merely by being connected.

The owner has not selected Git library/runtime, hosting provider, commit layout, branch strategy, audit-storage mechanism, synchronization scheduler, merge policy, bidirectionality of satellite integrations, or required deployment services. No implementation is claimed.

## 2. Logical architecture

~~~text
Humans / coding agents / future MCP
                |
          awl operations
          (read, edit, metadata,
           composite, audit)
                |
         Managed local Git
         (offline commits/history)
                |
          Git fetch/push
                |
        Authoritative Git remote
          /        |         \
    [adapter]  [adapter]   [adapter]
        |         |           |
   Google Drive  Other     Future systems
~~~

The final three arrows are **logical integration relationships**, not a promise of bidirectional, real-time or synchronous replication. A satellite can initially be an export-only projection; ingesting external satellite edits requires a separately defined policy.

The diagram describes logical responsibilities; a central awl service, custom Git server, daemon, monorepo, or mandatory public working tree is **not** implied.

### awl application API

- Resolve project slug and authorization context.
- Read project-relative Markdown, manage arbitrary YAML-frontmatter metadata, and support editing operations inspired by LLM-agent edit/patch tools.
- Accept multiple ordered operations in a composite; report individual success, failure and nonexecution without promising global transactions.
- Record operation intent, actor information actually known, resulting Git revisions and local/sync outcomes. Git commits alone do not capture failed requests, verified identities or all partial composite results.
- Persist managed Git state privately where possible, without allowing direct filesystem edits as part of the supported API. OS-level tamper resistance is **not** established merely by hiding files.

### Managed local Git

- Records local document versions while offline; locally acknowledged changes must survive restart.
- Can provide content-addressed trees, commits, history and established diff/merge primitives.
- May use low-level Git objects or a private worktree; the internal layout and Git library/process invocation are open.
- Maintains or links to operation audit records as necessary. Per-composite commit granularity, audit durability and failure behavior require decisions.
- Does not expose an agent-editable working directory as the supported persistence interface.

### Central remote Git

- Acts as the **authoritative shared history**, defining the baseline from which clients and satellite integrations synchronize.
- Can be hosted by a Git provider or other compatible Git remote; selection, credential model, reference/branch rules and authorization are open.
- Is not necessarily the same repository used for source code: source repository and worklog repository can remain independent.
- Cannot guarantee tamper-proof audit without appropriate remote controls; commit author data is not verified identity.
- Local offline commits and successfully shared commits are distinct states.

### Satellite integration boundary

- Consumes remote Git revisions and projects the selected Markdown tree (or subset) into the external system, subject to access controls.
- Tracks mapping to external file IDs and revisions and its own progress/checkpoint state; exactly-once remote side effects cannot be assumed.
- Does not require every external provider to support Git commit graphs.
- Needs explicit handling of the inverse direction **if** external edits are to flow back toward Git. Automatic Drive → Git import must not be assumed.
- Integration failure must not silently roll back or corrupt the authoritative Git history.

## 3. Topology consequences

| Concern | Revised direction | Still open |
| --- | --- | --- |
| Project identity | Slug addresses an awl project | Slug grammar, namespaces, project-to-repository mapping |
| Local persistence | Git-backed local state | Library, repository layout, commit policy, operation journal |
| Shared version history | One authoritative Git remote per project | Hosting, branch/ref, permissions, update rules |
| Google Drive | Optional satellite integration from Git hub | Export-only versus bidirectional; scope and mapping |
| Future providers | New hub-connected integrations | Which providers, plugin mechanism, scheduling |
| Audit | awl request/operation audit + Git history | Shared audit location, retention, actor authentication, tamper resistance |
| Conflicts | Visible and never silently overwritten | Algorithms and handling of external edits |
| Offline | Local commits continue without a network | Initial checkout, partial/offline availability, recovery |
| Composite | Grouped requests with per-operation outcomes | Failure policy and Git commit grouping |
| Markdown | Plain .md + generic YAML frontmatter | Detailed preservation/compatibility semantics |

### Consistency boundaries

There are now **three potentially different progress positions**:

1. **Locally applied:** an operation is recorded in local Git and audit state.
2. **Git published:** the relevant commit is accepted by the authoritative Git remote.
3. **Satellite projected:** a given integration has reflected the corresponding Git state, if configured.

Consequently, "synchronized" must be qualified; a user may be current with the Git hub while Google Drive is behind. Satellites must not report success before the provider confirms the corresponding operation.

### Why Git at the center?

Using a central Git remote avoids a network of arbitrary pairwise synchronizers and gives clients a shared change history independent of external provider revision models.

The tradeoff is that every non-Git provider now needs an adapter connecting it to the Git hub; an external provider is not a drop-in alternative source of truth. Google Drive can contain ordinary Markdown files, but cannot natively consume Git commits, references or operation audit logs.

## 4. Alternative history and rejected-for-now simplifications

### Previous Option B: Generic local snapshots/journal

Originally considered as an alternative local engine, with both Git and Drive treated as peer remote backends. This is **not the owner's currently preferred topology** because it would duplicate version-history functionality while a Git hub is already central. An operation journal may still supplement Git for application audit; selecting its storage is open.

### Previous Option C: CRDT / Automerge

CRDTs were considered for offline convergence. They are not currently necessary to describe the selected Git hub topology, and non-CRDT Markdown integrations lose causal CRDT information unless additional state is maintained. Deferred, not ruled out by evidence.

### Alternative: Direct filesystem editing

Previously discussed, but rejected as the supported agent write path because uncontrolled mutations bypass awl operation audit and safe multi-edit processing. Managed on-disk Git files may still exist internally.

### Alternative: Full ACID transaction manager

Not required. The system should preserve each accepted document result durably and clearly describe partial composite outcomes. Global cross-document rollback or distributed transactions are not product requirements.

## 5. Operational model (illustrative, not finalized CLI syntax)

~~~yaml
project: muse
request_id: example-123
operations:
  - id: update-status
    op: metadata.set
    path: Activities/example.md
    key: workflow-status
    value: completed
  - id: update-outcome
    op: document.edit
    path: Activities/example.md
    edits:
      - old_string: Pending verification.
        new_string: Implementation completed.
~~~

An accepted local change may become one or more Git commits. Its status can subsequently progress through central Git publication and optional independent satellite export. Audit should connect these stages by stable correlation IDs; exact storage and synchronization representation remain open.

## 6. Open questions — defer detailed edge cases

1. **Satellites:** Is Google Drive a one-way export/projection or does it also ingest Drive-originated edits? If bidirectional, who resolves the new Git commits and their provenance?
2. **Integration execution:** Where do satellite sync workers run (client, hosted job, webhook-triggered process, etc.)? GitHub/GitLab hosting alone does not automatically run a Drive connector.
3. **Git repository topology:** One repository per project versus per-project scope within a repository; main branch, operational refs, policies and protected updates.
4. **Audit:** Is locally stored audit sufficient or must attempted/failed operations be shared and authenticated? How does audit survive synchronizing across different clients?
5. **Git persistence:** API or CLI invocation; storage isolation, commit frequency, idempotency/recovery, and history management.
6. **Editing:** Exact text edit, context patch, line/hash anchors and optional merge techniques remain candidates, not chosen algorithms.
7. **Conflicts:** Divergent Git history, external Drive edits, in-flight publishes, simultaneous creation and ambiguous results. Detailed scenarios postponed at user request.
8. **Satellite scope:** Whole project export versus selected documents; filenames, YAML preservation, and provider limitations.
9. **Security:** Authoritative access controls, actor identity, credentials, secrets, failure isolation, optional encryption.
10. **Operational behavior:** Sync timing, background process need, backpressure, API quotas, onboarding and migration.
11. **Metadata compatibility:** OKF v0.2 target, old worklog status and wikilinks remain user-owned until separately migrated.

## 7. Next evaluation increment (proposal only)

A focused implementation-independent assessment should investigate:

- How to represent a project and a composite operation in managed Git without a user-writable working tree.
- How to correlate Git commits with application-level audit and partial operation results.
- How a satellite exporter derives Drive file changes from a moving Git reference and checkpoints completed work.
- Which architectural parts remain operationally necessary when no satellite backend is configured.
- Whether publishing Git commits and projecting to Drive can be cleanly decoupled with visible per-destination status.

**Do not** implement a Git/Drive bridge or choose a specific CI/daemon architecture solely on the basis of this document.

## 8. Decision boundary and documentation status

- **Owner-selected architectural direction:** Git local + authoritative Git remote + satellite integrations in a hub-and-spoke topology.
- **Not yet approved:** concrete Git layout, sync mechanisms, operational deployment, libraries, trust model and conflict semantics.
- **Repository status:** this document is in an unmerged draft PR. It is not an implementation record or ADR.

## References

- [Proposed Core Requirements](../requirements/core-requirements.md)
- [AgentWorklog CLI repository](https://github.com/giacomobartoloni/awl)
- [OKF v0.2](https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/main/SPEC.md)
- [Git reference updates](https://git-scm.com/docs/git-update-ref)
- [Google Drive changes](https://developers.google.com/workspace/drive/api/guides/manage-changes)
- [Google Drive revisions](https://developers.google.com/workspace/drive/api/guides/manage-revisions)

This file supersedes the earlier draft architecture comparison from the same PR branch while preserving its alternatives and reasons for reconsideration. No experiments, tests, provider integrations or implementation have been performed.
