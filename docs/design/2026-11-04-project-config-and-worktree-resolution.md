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
- **Concurrent use by two worktrees:** protect the shared managed Git state from clobbering; use the smallest practical per-repository coordination boundary appropriate to request-level commits. The lock mechanism is not selected here.
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
| C-09 | Two concurrent requests from different worktrees | Shared state remains safe; incompatible changes surface |
| C-10 | Source repository relocated on disk | Existing binding retained or explicit relocation error, never silent data fork |
| C-11 | A symlink points outside the canonical path, or `.awl/local.yaml` is tracked | Error without following unrelated data or silently changing Git tracking |
| C-12 | Ordinary read before initialization | No newly created state or hidden side effects |

## 8. Open decisions, intentionally limited

1. Exact YAML schema (including persisted local project binding), validation rules and treatment of unknown keys.
2. Initial setup/update commands and safe source-repository ignore configuration.
3. Cross-platform application-data locations, canonical path handling, source repo moves, and Git version minimum.
4. Effective Git identity when worktree-specific configuration differs, and author vs. committer attribution policy.
5. Minimal concurrency/locking and atomic file initialization for shared managed state.
6. Separate explicit-project configuration resolution when no source Git repository exists.

No new CLI command names, runtime, Git abstraction library, registry, background sync, database, authentication model, or migration tool is selected by this proposal.

## References

- [Git rev-parse: `--git-common-dir`](https://git-scm.com/docs/git-rev-parse)
- [Git worktree: common versus worktree-specific config](https://git-scm.com/docs/git-worktree)
- [Git config: includes and worktree settings](https://git-scm.com/docs/git-config)
- [Owner-confirmed configuration requirements](../requirements/core-requirements.md)
