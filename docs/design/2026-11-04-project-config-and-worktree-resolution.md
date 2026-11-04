# Project Configuration and Shared Git Worktree Resolution — Design Proposal

**Status:** proposed design for review; not implemented or accepted as a complete technical specification.  
**Date:** 2026-11-04.  
**Product:** awl CLI, distinct from the existing AgentWorklog Markdown conventions.  
**Baseline:** [Core Requirements](../requirements/core-requirements.md) (FR-24–FR-26, AC-20–AC-24) · [Git hub-and-spoke architecture](../architecture/2026-10-23-local-first-storage-evaluation.md).  
**Scope:** Config resolution for source repositories with linked Git worktrees. Projects without an associated source Git repository remain supported in principle, but their discovery/setup mechanism is a separate design topic.

## 1. Requirements and status

### Owner-confirmed constraints

- `.awl/project.yaml`: shareable, version-controllable source-project configuration.
- `.awl/local.yaml`: private, Git-ignored configuration entry point.
- All worktrees associated with one source Git repository share **one canonical personal awl configuration** and **one managed local AgentWorklog Git repository** on a given machine.
- The canonical personal config resides under the source repository's **Git common directory**. A worktree-local `.awl/local.yaml` symlink is optional; awl must not depend on it.
- Managed AgentWorklog Git repositories are held by default under the user's application-data directory, with an explicit local path override.
- Use the user's existing Git identity by default; allow explicit awl-local override without modifying Git settings.
- No replication, watchers, mandatory daemon, speculative configuration registry, or source-worktree-local managed Git duplicates.
- Source Git repositories are a convenient project context, not a requirement for every awl project. Never silently rewrite user-owned Markdown or Git configuration.

### Design proposals, not yet approved

The precise resolution algorithm, sample YAML keys, initialized project binding, errors, and setup/relocation behavior below are **candidates**. Command names, OS-specific paths, Git internals, and storage layout are not frozen.

## 2. Files and ownership

| Location | Purpose | Owner and visibility |
| --- | --- | --- |
| `<worktree>/.awl/project.yaml` | Project slug, authoritative Git remote reference, and any genuinely shared settings | Source Git; versioned; may differ on branches |
| `<git-common-dir>/awl/local.yaml` | Canonical personal settings, project binding, resolved managed-repository location | awl/user; local to the source Git repository; never committed |
| `<worktree>/.awl/local.yaml` | Optional symlink to canonical personal file, not a second independent file | Git-ignored convenience only |
| `<user-data-dir>/awl/repos/<project-instance>/` | One awl-managed local Git repository containing worklog Markdown | awl; shared by this repository's worktrees; separate from source Git |

`<project-instance>` is intentionally schematic. Setup must prevent two unrelated local projects with the same slug from silently sharing a managed repository. Do not put secrets, local absolute paths or a personal identity into the versioned `project.yaml` by default.

The source repository should ignore `.awl/local.yaml` explicitly, e.g.:

```gitignore
.awl/local.yaml
```

For existing repositories, the ignore rule is a proposed setup responsibility; awl must not silently modify an existing `.gitignore`. A tracked `.awl/local.yaml` is an actionable error, not something to untrack automatically.

## 3. Minimal resolution flow (proposal)

1. **Find the source context.** From the invocation directory, ask Git for `--show-toplevel` and `--path-format=absolute --git-common-dir`. Validate that the command is running inside a usable source worktree. The common directory, not the working-tree path or per-worktree `--git-dir`, determines shared personal configuration.
2. **Read project configuration.** Read `<worktree-root>/.awl/project.yaml` as data. Keep it read-only during ordinary operations. If it is missing, return a clear not-configured result instead of inventing a project or silently selecting a different one.
3. **Resolve the canonical local file.** Look directly for `<git-common-dir>/awl/local.yaml`. Missing personal configuration means no personal override; first-use setup and managed-repository provisioning are separate explicit operations, not hidden side effects of a read.
4. **Validate the optional entry point.** If `<worktree-root>/.awl/local.yaml` exists, require it to resolve to the expected canonical file. A separate ordinary file, a dangling/unexpected symlink, or a tracked local file is an explicit configuration problem; do not merge or overwrite it.
5. **Check the project binding.** Compare the current versioned project's identity (at minimum its slug and remote Git target, using an explicitly defined comparison policy) against the initialized association in canonical local state. A branch that would redirect worklog operations to a different project must fail before mutation; non-identity project settings may legitimately differ by branch.
6. **Apply configuration.** Resolve shared project defaults, canonical local settings, and explicitly supplied operation overrides. Use narrowly defined overrides; do not silently accept unknown or incompatible settings. No extra precedence layer or global awl config is required for this increment.
7. **Resolve managed storage.** Use the canonical configured managed-repository path, or a unique per-project path assigned once at setup under the user's application-data directory. Verify the resolved storage identity before writing; never create a new per-worktree repository based on the current worktree name.
8. **Perform the operation.** Access managed local Git through awl's controlled document API. Do not treat a successful local commit as remote Git publication, and do not let configuration resolution write to source or managed Git implicitly.

**Read-only resolution** must not create files, follow unrelated links, change Git config or initialize a repository. Explicit setup/update paths should be designed and tested separately.

### Logical resolution pseudocode

```text
source = discover_git_source(cwd)
project = read_project_yaml(source.worktree_root)
local_path = source.common_dir / "awl/local.yaml"
assert_no_conflicting_worktree_local_entry(source.worktree_root, local_path)
local = read_optional_local_yaml(local_path)
binding = validate_existing_project_binding(project, local)
effective = resolve_overrides(project, local, explicit_options)
managed_repo = resolve_managed_repo(binding, effective)
return ResolvedProject(project.slug, managed_repo, effective, source)
```

This illustrates separate responsibilities; it does not mandate classes, a public API shape, or a particular implementation language.

## 4. Configuration shape (illustrative only)

`.awl/project.yaml`:

```yaml
project:
  slug: my-project
storage:
  remote: git@github.com:team/my-project-worklog.git
```

Canonical personal configuration at `<git-common-dir>/awl/local.yaml`:

```yaml
# Illustrative awl-local overrides; not the approved schema.
user:
  name: Local Name
  email: local@example.com
storage:
  localRepo: /home/example/.local/share/awl/repos/my-project-instance
```

The stored project binding, if adopted, needs its own minimal representation; do not infer a project identity from a potentially colliding slug or a per-worktree absolute path. No credentials belong in these examples.

## 5. Git identity and attribution

- awl reads the effective `user.name` and `user.email` in the **source Git context** when making an operation that needs attribution. Never accidentally inherit the identity of the managed AgentWorklog Git repository.
- A personal awl override can replace Git identity; it does not call `git config --global` or modify `.git/config`.
- Git can support worktree-specific configuration and conditional includes. Consequently, the **shared awl configuration file** is identical across worktrees, while the **effective inherited Git identity may legitimately vary**. The desired behavior when that happens remains an explicit review point: enforce a common identity or use the invoking worktree's Git identity with provenance. A canonical awl-local override can guarantee the same configured identity.
- Keep operation actor/agent provenance distinct from Git author/committer fields. Neither Git name/email nor a commit proves an authenticated human identity.
- Fail explicitly when an operation requires an identity and the effective value is incomplete. Whether name/email overrides must be supplied together is open.

## 6. Data integrity, failure behavior and YAGNI

- **Missing project config:** report an unconfigured source project; do not create `.awl/project.yaml` on ordinary reads.
- **Missing canonical local file:** use project defaults where possible; if managed-repository association is needed but absent, require explicit setup rather than silently creating one.
- **Branch disagreement:** compare identity against the previously initialized binding; report incompatibility and require an intentional rebind operation if the source project has genuinely changed.
- **Unexpected `.awl/local.yaml` file or symlink:** fail with its path and next-step guidance. Do not synchronize, merge, replace or delete user files.
- **Repository move / path change:** preserve the stored managed-repository association when available; relocation/migration is explicit. Do not silently start a fresh worklog history.
- **Concurrent use by two worktrees:** protect the shared managed Git state from clobbering. The refined proposal in §9.4 favors isolated request staging and a conditional Git ref update, without an awl-wide document-write lock.
- **Unsupported filesystems / symlinks:** no link is required. The canonical file is read directly.
- **Non-Git projects:** do not force a fake source repo merely to reuse this mechanism; provide a later explicit-project resolution design.

Keep configuration discovery, config validation and managed-document operations conceptually separate, but avoid factories, registries, background services, generalized provider interfaces, or a YAML merge engine without demonstrated need.

## 7. Acceptance checks for the first implementation slice (not executed)

| ID | Scenario | Expected result |
| --- | --- | --- |
| C-01 | Main and linked worktree, invoked from nested subdirectories | Same common-dir local config and managed worklog repo |
| C-02 | No symlink in either worktree | Both still read canonical local config |
| C-03 | Correct optional symlink in only one worktree | Equivalent resolution in both worktrees |
| C-04 | A separate untracked `.awl/local.yaml` in a linked worktree | Explicit ambiguity error; no overwrite |
| C-05 | Branches differ only in non-identity project settings | Each uses its branch settings, but the same local managed repo |
| C-06 | Branch changes project slug or authoritative Git remote incompatibly | Explicit binding conflict before a write |
| C-07 | Two source repositories use the same slug | No accidental managed-repository collision |
| C-08 | Git identity with and without awl-local overrides | Correct attribution provenance; no mutation of Git settings |
| C-09 | Two concurrent requests from different worktrees | At most one request publishes a commit from a shared expected parent; a losing CAS reports a request-level conflict without declaring member successes durable |
| C-10 | Source repository relocated on disk | Existing binding retained or explicit relocation error, never silent data fork |
| C-11 | A symlink points outside the canonical path, or `.awl/local.yaml` is tracked | Error without following unrelated data or silently changing Git tracking |
| C-12 | Ordinary read before initialization | No newly created state or hidden side effects |

## 8. Open decisions, intentionally limited

1. Exact YAML schema (including persisted local project binding), validation rules and treatment of unknown keys.
2. Initial setup/update commands and safe source-repository ignore configuration.
3. Cross-platform application-data locations, canonical path handling, source repo moves, and Git version minimum.
4. Effective Git identity when worktree-specific configuration differs, and author vs. committer attribution policy.
5. Validate CAS-based concurrent commit publication, isolated staging and safe atomic first-time initialization before selecting any additional lock.
6. Separate explicit-project configuration resolution when no source Git repository exists.

No new CLI command names, runtime, Git abstraction library, registry, background sync, database, authentication model, or migration tool is selected by this proposal.

## References

- [Git rev-parse: `--git-common-dir`](https://git-scm.com/docs/git-rev-parse)
- [Git worktree: common versus worktree-specific config](https://git-scm.com/docs/git-worktree)
- [Git config: includes and worktree settings](https://git-scm.com/docs/git-config)
- [Owner-confirmed configuration requirements](../requirements/core-requirements.md)


## 9. Focused design refinement (2026-11-04; pending owner review)

This section refines the schema, identity policy, initialization, and concurrency boundaries. **Owner confirmed on 2026-11-04**: (a) use the invoking source worktree's effective Git identity, subject to explicit canonical-local override, and (b) proceed with the minimal two-file YAML schema and persisted binding shown below as the design baseline. The initialization and concurrency mechanisms remain **proposals for review**. No CLI implementation, public commands, or runtime technology is approved.

### 9.1 Minimal YAML v1 — owner-accepted design baseline

`.awl/project.yaml` — version-controlled and shared:

```yaml
schema: 1
project:
  slug: my-project
storage:
  remote: git@github.com:team/my-project-worklog.git
```

`<git-common-dir>/awl/local.yaml` — canonical personal file; the optional worktree `.awl/local.yaml` is a link to this file:

```yaml
schema: 1
binding:
  slug: my-project
  remote: git@github.com:team/my-project-worklog.git
storage:
  localRepo: /home/user/.local/share/awl/repos/my-project-c8f2a0
# Optional identity override:
# user:
#   name: Example User
#   email: example@example.com
```

**Recommended field ownership:**

| Field | Source | Why |
| --- | --- | --- |
| `schema` | Both files | Explicit, inexpensive compatibility check; reject unsupported major format rather than guessing |
| `project.slug` | `project.yaml` only | Public project address, not a filesystem-path identity |
| `storage.remote` | `project.yaml` only | Authoritative shared Git remote; never override it per-worktree |
| `binding.slug`, `binding.remote` | Canonical local file; generated during setup | Persist the identity accepted when the local repo was bound; this is a safety check, **not** another authority for project settings |
| `storage.localRepo` | Canonical local file; assigned once at setup | Stable managed-repository path shared by worktrees, including after moving the source checkout |
| `user.name`, `user.email` | Optional canonical local overrides | The only awl-owned human identity overrides; Git remains the default |

Use explicit YAML mappings and strings, reject duplicate/unsupported schema keys and unsafe YAML tags, and report malformed files without rewriting them. No arbitrary YAML include, variable interpolation, environment templating, deep-merge framework, multi-profile selection, or secret store is proposed. The minimal `binding` repetition detects incompatible branch changes without a separate registry, state database or implicit remote matching. Remote comparison can start as strict literal comparison; treating different SSH/HTTPS spellings or repository renames as equivalent needs an intentional rebind operation rather than a speculative normalization algorithm. A remote URL embedding a credential must not be written to the shareable file.

The `storage.localRepo` default should be assigned once, using a collision-resistant per-initialization suffix rather than the slug alone. Persist the resulting path in the canonical local file. Cross-platform application-data roots and exact suffix encoding remain implementation details. A manual localRepo path change is a requested relocation, not permission to silently initialize an empty replacement.

**Precedence by setting, not by wholesale YAML merge:**

- Shared remote and slug: only `project.yaml`; the local `binding` must match, not override them.
- Managed repository path: canonical local file, or an explicitly persisted first-time default.
- Commit identity: explicit invocation override (if such an option is eventually approved) > canonical `user` override > Git identity resolved from the invoking source worktree. No awl-global identity copy or auto-creation of a personal profile is required.
- Missing values required for an operation must produce an explicit error; never silently use an identity from the managed worklog Git repository.

### 9.2 Identity in different source worktrees — owner-approved policy

Git `user.name` and `user.email` can differ between worktrees through `extensions.worktreeConfig`, `includeIf \"worktree:...\"`, or branch-conditioned configuration. **One canonical awl local file does not automatically imply one inherited Git identity.** The smallest Git-aligned policy is:

1. Read the effective Git name/email in the **invoking source worktree** for each attribution-requiring request, including Git's applicable conditional/worktree configuration. Do not cache them at first setup.
2. If `user` is specified in the canonical awl-local file, it overrides those Git values for every worktree; prefer an all-or-nothing name+email pair to avoid ambiguous mixed provenance.
3. For a normal awl-authored content commit, use the resulting identity explicitly for Git author **and** committer, unless a separately approved operation requires distinct parties. The executing agent/harness is tracked separately as application provenance, not impersonated as the Git user.
4. The audit should record the identity source (`awl-local` versus `git-source-worktree`) and invoking worktree/revision when available, without claiming the Git email is authenticated.

**Alternatives considered:** pin a Git identity at initialization (becomes stale and stops inheriting Git updates), or ignore worktree-specific Git identity and force Git's shared common config (does not match how Git operates for the caller and would require bespoke behavior). Both are less attractive than native Git resolution with an optional shared override.

**Implementation validation still required:** Git's environment-provided author/committer overrides versus Git config values, default fallback to OS username/hostname, and safe explicit author/committer injection into managed Git. Do not rely on unexpected Git fallback identities or confuse Git author attribution with authenticated actors.

### 9.3 Explicit first-use initialization (candidate workflow)

**Ordinary document reads and config resolution are side-effect-free.** Use an explicit initialization action; the public command name is not selected.

1. Discover the source Git worktree root and the shared Git common directory. Reject unsupported/bare/non-worktree contexts with a clear diagnostic; non-source-Git projects require a separate project resolution path later.
2. Read an existing `.awl/project.yaml` if present or prepare the smallest new file with requested slug and remote. Do not infer the worklog Git remote from the source repository's `origin`, overwrite a file, stage changes or create a source Git commit.
3. Check that `.awl/local.yaml` is not tracked, that the source Git ignore rule applies (a root-relative `/.awl/local.yaml` entry is a candidate), and that an existing local entry is either absent or points at the canonical file. Print a safe corrective action when the ignore rule is missing; changes to an existing `.gitignore` must be explicit.
4. Acquire a short-lived initialization lock associated with the **common directory**, then re-check canonical state. If another worktree already initialized the project, validate and reuse the existing binding and repository rather than creating another.
5. For a new project association, allocate a unique managed-repository path under user application data, create the managed Git repository safely, and atomically install the canonical local configuration containing its persisted path and accepted slug/remote binding. Treat partial failure as an observable incomplete setup; do not silently delete, overwrite or switch a repository.
6. Symlink creation is optional and separate from correctness. Do not copy personal configuration into other worktrees. Do not mutate source Git config.
7. Verify both the persisted local binding and the managed repository identity before reporting success. Offline initialization can establish local state even when remote Git connectivity is unavailable; remote publication is a separate status.

Do not initialize automatically on document read or when a new linked worktree appears. Changes to the shared project's identity or authoritative remote need explicit rebind/relocation with validation and no implicit destructive migration.

### 9.4 Concurrent operations from different worktrees — revised YAGNI proposal

**Avoid an application-wide write lock for ordinary managed Git document requests.** Git already provides conditional reference updates; use a **compare-and-swap (CAS) on the managed branch ref** as the commit publication boundary. This is the simplest candidate consistent with safe concurrency, provided each request avoids shared mutable staging state.

1. Read the current managed Git revision `R` at the start of the request and use it as a consistent base. A composite performs its ordered operations on the candidate tree derived from `R`, continuing after member-level errors according to the approved operation contract.
2. Build the candidate content **without changing the live managed ref**. Git objects are immutable; where an index is used for staging, give **each request its own temporary index** (or use another isolated tree construction mechanism). Do not share a mutable Git index or working tree across concurrent requests.
3. Create a candidate commit `C` with parent `R` only when changes succeeded. This commit object is not yet a confirmed local change.
4. Publish it conditionally, e.g. `git update-ref refs/heads/<managed-branch> <C> <R>`. Git accepts it only if the ref still equals `R`. If another writer published first, Git rejects the update; report a **request-level revision conflict**, not successful partial application.
5. On a CAS conflict, do not blindly auto-merge or replay against an unknown revision. For the first increment, return a clear retryable `REVISION_CONFLICT` with the current revision. An agent or caller can reread and retry; a carefully revalidated automatic retry is a **later** optimization only if observed contention warrants it.
6. Readers pin the Git revision they read and never combine data from two commits. Publication to the remote Git hub remains distinct from local CAS and must not take place while holding a local per-request lock (none is needed).
7. A rejected candidate commit may leave Git objects not reachable from any ref; they are not accepted document changes and are subject to normal Git object lifecycle. Audit must record rejection separately and must not declare the candidate committed.

**Git internal ref locking is not an awl lock manager.** `git update-ref` validates the expected old object ID and synchronizes ref mutation; it does *not* make an arbitrary sequence of read, edit and commit operations atomic. Building isolated candidates and publishing by CAS is what closes that gap. This approach does not promise that simultaneous non-overlapping writes both succeed; the later writer may need to retry. That conservative behavior is a conscious YAGNI tradeoff.

**Initialization is separate:** two worktrees trying first-time setup need one safe, short-lived **create-if-absent** publication of the canonical local binding, or an initialization-only OS coordination primitive if platform tests show it necessary. Do not introduce a lock around every document request just to solve setup races. If a setup contender loses, validate/reuse the winner's binding and repository; never silently overwrite the config or delete its data.

**Why not a process lock around all operations?** It would introduce lock lifecycle, timeouts, crash handling, portability and lock contention into the hot path before there is evidence the additional serialization is needed. Keep it as a fallback if a concrete selected Git implementation has shared mutable state that cannot safely be isolated.

**Validation required:** simultaneous same-base writers, a composite with successes and member failures followed by a losing CAS, read consistency, per-request temporary index isolation, abrupt process termination before/after ref publication, and concurrent first-time initialization. No awl runtime test has yet been executed.

Sources: [Git update-ref](https://git-scm.com/docs/git-update-ref) · [Git commit-tree](https://git-scm.com/docs/git-commit-tree) · [Git update-index](https://git-scm.com/docs/git-update-index).

### 9.5 Proposed errors and minimum validation

| Situation | Observable candidate error/behavior |
| --- | --- |
| Missing or malformed `project.yaml`, unsupported `schema` | `PROJECT_NOT_CONFIGURED` or `INVALID_PROJECT_CONFIG` |
| Invalid/missing shared local binding after setup | `LOCAL_BINDING_MISSING` or `PROJECT_BINDING_CONFLICT` |
| Unexpected independent `.awl/local.yaml` in a worktree | `LOCAL_CONFIG_AMBIGUOUS` |
| Changed Git remote/slug across project branches | `PROJECT_BINDING_CONFLICT` before any write |
| Missing/invalid Git identity when a commit needs it | `IDENTITY_UNAVAILABLE` |
| An existing managed path is occupied by another project | `MANAGED_REPO_CONFLICT` |
| Concurrent first-time setup cannot claim the canonical binding | Explicit setup-in-progress or initialized-elsewhere result; validate and reuse safely |
| Concurrent ref changed unexpectedly | `REVISION_CONFLICT` with current revision if safe to disclose |

The uppercase error strings are **candidate internal/public names**, not an approved protocol. Test a main worktree, one linked worktree, two intentionally different Git worktree emails, a shared awl identity override, concurrent initialization, concurrent composite writes, crash during setup, and a source repository relocation. Do not choose language, Git binding, filesystem-lock library, or OS support matrix before the first bounded spike.

### 9.6 Owner review gates

**Confirmed by the owner (2026-11-04):** effective Git identity follows the invoking worktree unless explicitly overridden by the shared canonical local configuration; proceed with the minimal YAML schema and persisted `binding`/`storage.localRepo` design baseline. **Still proposed:** conditional managed Git ref publication via CAS without a document-write lock; safe initialization-only create-if-absent, and error/atomicity details. No implementation is authorized.
