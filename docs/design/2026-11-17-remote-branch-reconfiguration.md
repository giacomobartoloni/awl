# Remote branch reconfiguration (experimental implementation)

**Status:** implemented in [PR #19](https://github.com/giacomobartoloni/awl/pull/19), stacked on [PR #18](https://github.com/giacomobartoloni/awl/pull/18); neither PR is merged by this change.

## Purpose and current boundary

An AgentWorklog *source Git branch* is not the project's *authoritative remote Git branch*. Existing project state is bound by the source worktree's `.awl/project.yaml` and the Git common directory's `awl/local.yaml`. The managed local document/audit history remains at `refs/heads/awl`; the configured remote Git URL and branch select where an explicit `awl sync` will exchange that history.

A user who has configured the default remote branch `awl` may need to select `worklog/awl` instead. Re-running `awl init`, using `remote add`, or manually editing both YAML files is the wrong interface.

## CLI behavior

```sh
awl remote set-branch worklog/awl --json
```

- Requires the current source Git repository, an initialized canonical binding, an already configured URL, and valid Git branch syntax. The URL is never changed.
- Updates `storage.remoteBranch` in `.awl/project.yaml` and `binding.remoteBranch` in `<git-common-dir>/awl/local.yaml`. Default `awl` is stored by omitting the field.
- A changed branch returns `remote_branch_updated`; a repeat returns `remote_branch_unchanged`. Both identify the project and selected `remote_branch`.
- No contact with the remote host, no managed ref update, no document/audit mutation, no storage relocation, no implicit setup.
- If either file has reached the requested branch after an interrupted two-file update, retry is supported **only when both still agree on project slug and URL**. Unrelated inconsistent bindings are rejected.

## Safety and synchronization

Both `awl remote set-branch` and `awl sync` use a shared sync lock. Sync acquires that lock *before* resolving the configured target, preventing a stale or partial resolution during a change. Branch reconfiguration additionally holds the existing initialization lock while validating and replacing the two files. This is short-lived local serialization, not a distributed transaction.

Once `refs/awl/verified` exists in the managed Git store, the command **refuses any different branch** with `REMOTE_BRANCH_MIGRATION_REQUIRED`. That ref records a previously verified remote history. Reassigning its meaning to another branch without a migration would undermine deletion/history-rewrite checks. Repeating the already selected branch is harmless and remains allowed.

A migration after remote verification (including a server-side rename, checkpoint reset, relationship verification, or publication history transfer) is a **separate feature** requiring its own acceptance criteria; `--force` is not offered here.

The two YAML replacements cannot commit atomically across files. On an interrupted update, readers reject inconsistent bindings; retry the explicit same branch command after checking the current context. Existing comment bytes, line endings, URL and store path are preserved where possible. No claim of sudden power-loss durability is made.

## Tests and constraints

The dedicated black-box CLI tests cover change and idempotency, implicit default rollback, existing document bytes/local Git commit preservation, comment/CRLF preservation, one-file retry, invalid branch and wrong project/URL rejections, missing managed store, and verified checkpoint refusal. Remote sync tests on PR #18 exercise a real local bare Git remote.

**Out of scope:** changing the remote URL, automatic publication or fetch, remote branch creation/deletion, historical migration after verified sync, arbitrary YAML rewriting, multiple authorities, or background services.

## Related source

- [Project and linked-worktree configuration](../design/2026-11-04-project-config-and-worktree-resolution.md)
- [CLI command reference](../../skills/awl-cli/references/commands.md)
- [First Git sync PR #18](https://github.com/giacomobartoloni/awl/pull/18)
