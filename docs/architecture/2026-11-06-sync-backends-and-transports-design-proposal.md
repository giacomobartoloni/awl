# Sync Engines, Backends, and Transports — Capability-Based Design Proposal

**Status:** architecture proposal for review; **not approved, implemented, tested, or an ADR**.  
**Date:** 2026-11-06.  
**Product:** AgentWorklog CLI (`awl`), not the existing AgentWorklog Markdown conventions.  
**Goal:** accommodate Git, rsync, rclone and potential provider-backed synchronization without prematurely turning the Go CLI into an interchangeable storage-engine/plugin framework.

## 1. Authority, history, and decision boundary

**Confirmed current product baseline:** the merged [Git hub-and-spoke evaluation](../architecture/2026-10-23-local-first-storage-evaluation.md) and [core requirements](../requirements/core-requirements.md) use **managed local Git** for documents/audit and **one authoritative remote Git hub per project**. Other systems, including Google Drive, are optional **satellites downstream of the hub**, not automatically co-equal authoritative backends. The owner has also approved Go with external Git CLI; command-first implicit-project selection; and **explicit, recoverable sync/index invocations without a mandatory daemon** (unmerged proposals [#2](https://github.com/giacomobartoloni/awl/pull/2), [#4](https://github.com/giacomobartoloni/awl/pull/4), [#6](https://github.com/giacomobartoloni/awl/pull/6)). Local writes and their complete Git audit commits must not depend on network transfer success.

**Exploratory alternative, not an approved revision:** [PR #3](https://github.com/giacomobartoloni/awl/pull/3) and the prior [sync-strategy worklog](https://drive.google.com/file/d/1G5EmfT98MJtpoVkKZvmrb0FrUqYYoBIo/view) investigate **provider-neutral authoritative synchronization**, per-file Base/Local/Remote (B/L/R) checkpoints, rsync, rclone, Google Drive and Automerge. They do not supersede the Git-hub decision. Different project backends and different transport executables are **not the same dimension**.

**Owner-approved local-only initialization (2026-11-06):** a project can be created with no remote; later `awl remote add <url>` assigns a Git remote **as configuration only**, without contacting it. Replacement/migration remains a separate decision. The command spelling is currently an experimental CLI example, not an approved permanent API. A project without a remote has local history; its remote publication status is `not_configured`, not `published`.

The aim here is to **prepare clean extension seams** while implementing only those modes with demonstrated semantics. Do not introduce a public Go plugin API, runtime registry, dependency injection container, interface per provider, or YAML strategy key just to satisfy a speculative extension requirement.

## 2. Distinct responsibilities and two pipelines

```text
Human / coding agent
       |
    awl CLI ── controlled Markdown operations, request audit
       |
Managed LOCAL Git (one private store shared among source worktrees)
       |
       +-- explicit awl sync: Git coordinator  ── fetch / reconcile / push / verify
       |                                          |
       |                                  Authoritative remote GIT hub
       |                                          |
       |                                          +-- optional satellite export
       |                                                |-- rsync/SSH projection
       |                                                |-- rclone copy projection
       |                                                |-- provider-native projection (e.g. Drive)
       |
       +-- explicit awl index refresh ── derived local Markdown projection ── QMD candidate
```

- **Local document engine:** owns bytes, local Git history, private staging, request-level audit, optimistic publication and deterministic read/modify behavior. **It does not know rsync/rclone flags or satellite URLs.** The same local engine serves a project initialized without a remote.
- **Git-hub synchronization:** owns Git ref ancestry, fetch, conservative reconciliation, push and **verified acknowledgement** against the single authoritative Git remote. It is not `git pull`, an opaque shell script, or a file-copy primitive.
- **Satellite projection coordinator:** consumes a **known hub-accepted** content snapshot, materializes only allowlisted Markdown files to an isolated temporary tree, applies a selected file transfer and tracks per-target status. It is not allowed to write directly into managed local Git. The hub must not be bypassed accidentally by exporting unacknowledged local-only changes.
- **Provider/transfer adapter:** owns transport-specific invocation, remote identity and observation/confirmation *only to the extent the provider supports it*. It does **not** decide a Markdown conflict, invent a common ancestor, or redefine the authority of the project.
- **Search index:** disposable derived state from **committed local documents**; unrelated to remote authority or file transfer.

These are **logical responsibilities**, not five mandatory packages/processes. The first executable Git sync can be a single small Go implementation; split a package or define an interface only when a second real consumer requires it. Optional satellite execution might occur on another host/process with hub access; there is no implicit always-on service or scheduled background task.

## 3. Capability and operating-mode matrix

| Mode / tool | Appropriate role | Authority and consistency | First implementation? |
| --- | --- | --- | --- |
| **Git fetch/reconcile/push** | Bidirectional exchange of retained Git histories | Central Git ref remains authority; commit ancestry and fast-forward/guarded publication; explicit divergence | **First sync target (proposal)** |
| **rsync over SSH** | One-way Markdown projection or efficient file transfer to a controlled host | A transfer is not conditional per-file publication, a shared Git history, or semantic merge; destructive `--delete` must be explicitly gated | Later satellite experiment |
| **rclone copy** | One-way projection to cloud/object storage/WebDAV/etc. | Does not delete destination-only files; does not prove that another writer has not changed an existing file | Later satellite experiment |
| **rclone sync** | Exact one-way mirror when the target is designated disposable/owned | Can delete destination-only files; require explicit scope, preview, deletion limits, recoverable checkpoints and approval | Later, opt-in destructive mode |
| **rclone bisync** | Two independently modified file trees **only after separate policy approval** | Maintains its own snapshots and conflict behavior; cannot substitute for awl audit, Git-local CAS or Markdown-safe imports | **Not selected** |
| **Google Drive API** | Candidate satellite export; potentially richer per-file revisions/IDs | Requires provider-specific mapping/version/permission/checkpoint code; import not assumed | Later experiment |
| **Automerge** | A possible same-document CRDT reconciliation mechanism | Not a file transport or remote provider; needs an entirely different change-history contract | Deferred |
| **Non-Git authoritative remote** | Alternative provider-neutral architecture from PR #3 | Would need explicit new product approval, retained B/L/R, safe conditional publishing and cross-document limitation statement | **Not approved** |

**Source safety:** [Git push documentation](https://git-scm.com/docs/git-push) defines fast-forward and non-fast-forward refusal; [rsync manpage](https://download.samba.org/pub/rsync/rsync.1) documents `--delete`, `--dry-run` and `--delay-updates`; [rclone sync](https://rclone.org/commands/rclone_sync/) documents one-way mirroring, destination deletion and dry-run; [rclone bisync](https://rclone.org/bisync/) describes its two-way snapshot/conflict requirements. None of these tools alone provides an `awl` request-level audit or a universal guarded remote update.

## 4. Minimal internal state and contracts (no speculative public interfaces)

The local writer already knows a **committed Git snapshot**: its commit OID and **document tree OID**. An audit-only commit advances the commit but leaves the document tree unchanged. Represent these as distinct values rather than stringly-typed `revision` claims. A synchronization target also has an independent **verified acknowledgement**, not merely a last-attempt timestamp.

Candidate data for an explicit sync attempt (not a storage schema):

- `projectID`, `targetID`, `targetRole` (`git-authority` or `satellite-export`) and **configured transport**, with exactly one authority at a time;
- `snapshot`: local commit and document tree for Git sync; **verified hub commit and document tree** for satellite export;
- `observedRemote`: provider-specific ref/version/hash/token, nullable when remote state is unknown;
- `lastVerifiedCheckpoint`: reference/version and content identity actually verified at the target, plus attempt/acknowledgement time and provenance;
- `status`: `not_configured | pending | up_to_date | conflict | failed | unknown` (illustrative; exact JSON/exit contract TBD);
- `capabilities`: evidence for incremental discovery, conditional update, deletion control, version identity, content verification, atomic ref update and safe import; do not reduce all providers to `SupportsSync bool`.

**Rule:** Git-to-Git can derive ancestry from its commits/refs. Do not persist redundant per-file B/L/R tables simply to mirror Git. A future non-Git authoritative mode, if authorized, must maintain **verified common base B, local L, observed remote R, tombstone/rename evidence and per-file checkpoints**; equality of bytes alone is not causal ancestry. A future satellite export might track a target-wide snapshot plus per-file receipts only if partial publication/recovery actually requires them.

**Concrete-first Go sketch (explanatory; not an implemented interface):**

```go
// First implementation can be concrete, with no public SyncBackend interface.
// A later transport interface belongs to the package that needs to call it.
func (s *GitSync) Run(ctx context.Context, project Project, remote GitRemote) (SyncResult, error)
func (p *Projection) Export(ctx context.Context, snapshot HubSnapshot, target ExportTarget) (ExportResult, error)
```

A narrow `exec.CommandContext` wrapper should accept explicit argument slices, a controlled Git/tool environment, process deadlines and stderr capture. Never build a command string passed through a shell, process arbitrary source-controlled update hooks or let a generic `SyncBackend` abstraction erase transport-specific safety guarantees. Use descriptive structured errors and wrap I/O errors with `%w`, plus `errors.Is/As` at call sites where appropriate. No framework is needed just because `GitSync` and `Projection` are different concrete types.

## 5. Git authority: explicit recoverable synchronization algorithm

1. Fail with `REMOTE_NOT_CONFIGURED` (or equivalent) if local-only initialization has not yet been followed by explicit configuration. `awl remote add <url>` alone has no network side effect, no new document commit and no proof of remote existence.
2. Acquire **only the synchronization-target coordination** needed to prevent duplicate sync on one target. Capture local committed head; do **not** hold the document-writer lock across network calls.
3. Fetch remote ref(s) into controlled names; validate remote origin, branch/ref policy, complete ancestry and target identity. A fetch is **observation**, not acknowledgement or a change to accepted local documents.
4. If the remote is strictly ahead or histories diverged, reconcile according to a separately reviewed conservative Git policy. Stage any remote-originated document changes and revalidate the authoritative local head under the managed publisher, preserving audit/correlation and rejecting unsafe changes. **Do not casually `git pull --rebase` or force-push.**
5. Push the chosen local head with a non-forcing ref update. On non-fast-forward rejection, fetch and retry only with bounded safe revalidation; never reset already accepted local commits. On timeout or lost response, report `unknown` until a subsequent remote ref read establishes whether publication occurred.
6. Verify that the intended Git commit/ref is **actually present on the authority** before acknowledging `up_to_date`; preserve independent local and remote status. A remote failure is not a failed local request, and network retries create no new request-audit commit.
7. The operation is **explicit and foreground for its own caller**, though it is deferred relative to local writes. No automatic queued job, daemon or background scheduler.

The exact Git branching, merge-commit audit, authentication, remote ref creation and history-rewrite policy are separate open review gates. This document does not assert that the experimental PR #7 already provides Git sync.

## 6. Satellite export: explicit, scoped, and conservative

1. Select a specific **verified authoritative Git-hub commit** and its document tree. If local commits remain unpublished, the exported snapshot deliberately excludes them; make that state visible instead of silently treating local as shared authority.
2. Materialize **only approved Markdown paths** into an awl-owned isolated staging directory. Never expose `.git`, local config, credentials, uncommitted speculative Git trees, or the private managed repository to rsync/rclone. Validate symlinks/path escape and destination scope.
3. Compare destination inventory/checkpoint to the source projection. In the first export mode, use **rsync without `--delete`** or **`rclone copy`** with a dry-run/preview option when useful. Write an explicit manifest/receipt when required to verify which files actually arrived; filesystem mtime alone is not universal evidence.
4. Require a separately approved **mirror/delete policy** before `rsync --delete` or `rclone sync`; use an explicitly owned target root, preview/diff, destination drift handling, maximum deletion budget, and safe recovery. Do not interpret an incomplete listing as mass deletion.
5. Verify as strongly as the chosen transport/provider can support. Mark the target as `up_to_date` only if the specified snapshot is confirmed; otherwise retain `pending`, `failed` or `unknown` with partial state as appropriate. A single multi-file transfer is **not an atomic composite publication** to external readers.
6. Do not import edits from a satellite automatically. `rclone bisync` or a provider-created change must be staged as **untrusted remote input** and reconciled via a verified B/L/R path only after bidirectional mode and identity/deletion/audit policies are explicitly selected.

A one-way satellite that is an archival mirror should not be confused with a writable authoritative project backend. File-oriented data and audit have different representations: exporting Markdown alone does **not** automatically export the operation-level audit stored in Git commit messages; preserve the history centrally and make that omission explicit.

## 7. Configuration, authorization and local-only bootstrap

- **Current Git-only experiment:** `.awl/project.yaml` can contain `project.slug` without `storage.remote`; canonical common-dir local binding records the same absent remote; 
  `awl remote add <git-url>` sets the Git authority address later, without push, fetch, checkout, user document edit, or new awl audit commit. Same URL is idempotent; a different existing URL must fail closed. Two-file config update is *not an ACID transaction*; retry/recovery must be tested.
- **Do not overload** `awl remote add` with arbitrary rclone remote names or SSH file projections in the first slice. A later satellite target may need its own distinct command/configuration and separate status; its syntax and auth-location contract are still open.
- Keep shareable project identity and genuinely shared endpoint declarations versioned when adopted, with machine-specific paths and secret credentials confined to personal/local credential stores. A change of authoritative backend is a **migration**, not a harmless field flip: preserve pending local commits, audit lineage, remote checkpoints and provenance.
- Project selection remains implicit from source Git worktree; out-of-repository `--project <slug>` lookup was explicitly deferred by the owner. This design does not reopen it.
- Stand-alone non-source-Git projects and a provider-neutral authoritative backend are **separate product decisions**, not hidden prerequisites for local-only `awl init` within a Git source checkout.

## 8. Proposed verification matrix (no sync adapters executed)

| ID | Scenario | Required result |
| --- | --- | --- |
| SYNC-D-01 | Local-only project initialized with no remote, then multiple accepted document commits | Local reads/writes/audit work; remote `not_configured`; explicit sync does not guess a target |
| SYNC-D-02 | Add Git URL later; retry same URL and attempt conflicting URL | Add is idempotent, changes configuration only; conflicting replacement rejected; no local content loss |
| SYNC-D-03 | Git push rejected after unrelated author publishes | Local history retained; conflict/pending shown; bounded reconcile, never force-overwrite |
| SYNC-D-04 | Remote accepts push but response lost | Subsequent explicit sync verifies remote ref before claiming failure/success; no duplicate mutation audit |
| SYNC-D-05 | Audit-only local commit | Remote Git may need publication; satellite Markdown projection and QMD reindex need not rerun if content tree unchanged |
| SYNC-D-06 | rsync export interrupted after some files | Target remains partial/pending; source Git unchanged; later retry repairs without claiming whole snapshot atomicity |
| SYNC-D-07 | rclone copy to storage with extra external files | Extra files are not implicitly deleted; no silent mirror semantics |
| SYNC-D-08 | Requested exact mirror would delete externally added file | Explicit controlled deletion policy / preview required; no default destructive operation |
| SYNC-D-09 | rclone bisync detects two writers editing same Markdown | No direct overwrite of managed local Git; conflict remains staged until approved import policy exists |
| SYNC-D-10 | Hub commit differs from local unpublished commit | Satellite export uses **verified hub** snapshot, reports its revision, not local speculative content |
| SYNC-D-11 | Two CLI instances sync the same target while local writes continue | Target sync coordinated, local writes remain available; checkpoint never falsely advances |
| SYNC-D-12 | Adapter missing, unsupported precondition/rename/deletion semantics | Actionable unsupported/tool-unavailable state; no implicit provider fallback or remote destructive mutation |

**Status/evidence:** No Git, rsync, rclone, Drive or bisync adapter tests were executed for this design. Test adapters against real CLI tools, controlled remotes, interruptions and adversarial workloads before claiming support. Local Go tests in PR #7 validate the **local** document engine only.

## 9. Deliberately postponed decisions

- Whether non-Git backends can be **authoritative** instead of satellites, and which capability thresholds would justify replacing the hub; PR #3 remains an unapproved alternative.
- Whether two-way satellite ingress is supported, and if so what B/L/R conflict policy, audit provenance and tombstone/rename semantics are required.
- Exact configuration keys, CLI commands for satellites, credential management, provider native SDK choice, package structure, version requirements, platform matrix, checkpoint encoding, destination atomicity and archive retention.
- Which provider transports can meet a multiwriter guarantee. If no guarded write/ack is possible, **declare single-writer/export-only constraints** instead of pretending full interchangeable sync.

**Recommended YAGNI development order:** (1) finish local-only `init` / Git binding configuration and local writer safety; (2) implement **one concrete Git-authority sync** end to end with verified remote state; (3) prototype **one** bounded satellite export with rsync or rclone on a disposable target and evaluate actual capabilities; (4) introduce only the interfaces, data records and configuration fields the second implementation has shown to be necessary; (5) revisit provider-neutral authority with an ADR and explicit owner approval if demanded by real projects. No simultaneous Git/rsync/rclone engine implementation is proposed.


## 10. Extensibility boundary: backend authority vs transport (refinement)

**Status: design proposal, not an approved replacement of the Git hub or an implemented Go abstraction.** The owner wants future choices of backend and sync mechanism, including Git, rsync and rclone. Preserving the possibility requires a **clear architectural seam** today, not implementing all modes at once.

### Orthogonal decisions

| Responsibility | Current choice | Future options | Contract that cannot be waived |
| --- | --- | --- | --- |
| Local document/audit store | Managed local Git | Alternative store only under separate decision | Durable complete request outcome, stable snapshot, safe concurrency and recovery |
| Authoritative remote | Git hub | A provider with verified conditional updates and retained B/L/R checkpoints | One authority, observed remote version, guarded publication and verified acknowledgement |
| Transfer/export | Native Git protocol | rsync, rclone copy, rclone sync (restricted), provider API | Scoped content snapshot, explicit direction, deletion policy, independently verified transfer |
| Content reconciliation | Git history and controlled request replay | File-granular Base/Local/Remote; CRDT only if justified | Preserve edits/provenance, never silently last-writer-wins |
| Search | Derived index (QMD candidate) | Other derived engines | Never determines local/remote publication or source authority |

**Git, rsync and rclone are not three implementations of the same operation.** Git exchanges a revision graph. Rsync/rclone primarily move file bytes; their copy/mirror semantics cannot claim remote conditional-write, causal ancestry, Markdown-safe merge or request audit. In particular, rsync with delete flags and rclone sync may delete remote-only files; rclone bisync has separate state and recovery hazards.

### Minimal extension points to preserve

1. **Local writer (existing, concrete Go code):** accepts document intents and returns an acknowledged immutable local Git commit plus document tree; no network calls or rsync/rclone dependency. No new storage interface is needed now.
2. **Coordinator (first concrete Git sync):** explicit invocation owns remote observation, conflict/reconcile policy, bounded attempts, cancellation and independent verification of acknowledgement; does not hold the local publication lock across network calls.
3. **Git authority implementation:** fetch, validate ref/ancestry, reconcile, non-force push and verify. Git commit/refs are private implementation identifiers, not a universal backend revision type.
4. **Snapshot export implementation (later):** project a **verified authoritative hub** document tree into an isolated allowlisted directory, run a selected transport and verify destination receipt/manifest. No direct modification of managed Git. Content export is not Git history/audit export. No implied atomic multi-file visibility.
5. **Alternative authority (separately approved research path, PR #3):** preserve an actual Base/Local/Remote checkpoint plus provider IDs/versions, tombstones, rename/deletion evidence and controlled import. Require verified conditional writes or explicitly restrict that provider to single-writer mode. A best-effort file copy is not a multiwriter authority.
6. **Go design:** do not add an exported SyncBackend/Transport/StorageBackend interface or plugin registry yet. When the *second real implementation* is built, extract the smallest interface **in the calling package**, with capabilities and errors that cannot hide differences between Git authority and snapshot export. Favor two concrete types/functions over an artificial shared Sync method.

### Capability-based admission

| Capability | Git authority | rsync/rclone satellite | Future file/object authority |
| --- | --- | --- | --- |
| Version and acknowledgement | Git ref + ancestry + confirmed observation | Manifest/checkpoint per export; exact guarantee requires testing | Provider token/object identity and verified receipt |
| Guarded multiwriter publish | Non-force ref update + recheck | **Not provided by file transfer alone** | Must demonstrate CAS/conditional update or restrict to one writer |
| Delete and rename | Git history, explicit reconcile | Default no-delete; opt-in mirror with preview and deletion budget | Tombstones, full listing, guarded changes and origin tracing |
| Multi-file publication | One Git ref publishes a tree | **Partial remote visibility possible** | May be per-file partial unless provider guarantees atomicity |
| Interrupted attempt | Read remote ref before retry | Reverify files/manifest and repair staging | Reobserve provider state; never blind side-effect replay |
| External edits/import | Reviewed Git reconciliation | Not enabled in first one-way export | Staged B/L/R merge and auditable import required |

**Fail-closed rule:** unsupported modes report an explicit capability error rather than downgrading to overwrite/force-push/deletion. In the first implementation, the configured Git URL (including experimental local-only init followed by remote add) is not an arbitrary rsync/rclone target, and the current minimal YAML schema stays unchanged. Switching an existing authoritative backend is a **migration**, not a configuration flip.

### Incremental verification before adding code

- **EXT-01:** Git fetch/push remote divergence, lost response, retry and unchanged local acceptance.
- **EXT-02:** rsync one-way export of a verified hub snapshot to a disposable target, interruption/retry, no deletion by default.
- **EXT-03:** rclone copy on a disposable target with externally created files, interruption and checkpoint verification; test mirror/delete only after explicit authorization.
- **EXT-04:** one concrete non-Git authority's real version-token/conditional-write/deletion behavior and preserved Base/Local/Remote, before creating a provider-neutral interface.
- **EXT-05:** across enabled modes, preserve local request audit, project-relative identity, bytes, index freshness and truthful partial/unknown states.

This amendment does **not** approve a multiple-authority topology, provider plug-in API, adapter registry, rclone bisync, new project schema or production sync execution.
