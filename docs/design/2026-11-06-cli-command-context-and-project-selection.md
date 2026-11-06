# CLI Command Context and Project Selection — Proposal

**Status:** owner-selected top-level command shape and implicit-project behavior; remaining concrete command details proposed for review, **not implemented**.  
**Date:** 2026-11-06.  
**Product:** AgentWorklog **CLI** (`awl`), distinct from the existing AgentWorklog Markdown conventions.  
**Scope:** command layout, project selection, safe initialization and integration with the existing logical document-operation contract. No new storage authority or application implementation is defined here.

## 1. Provenance and decision status

**Owner-selected 2026-11-06:**

- Everyday invocation follows **`awl <command> [arguments]`**, not a mandatory `awl document <command>` or `awl <project> <command>` prefix.
- The **project is implicit** when it can be safely determined from the invoking repository/worktree context.
- An **explicit `--project <slug>` selector** is part of the CLI direction. Its initial semantics are scoped to an already resolvable source-repository context; how it could locate a project **outside that source repository is deliberately not defined at this stage**.
- CLI remains shell-invocable and harness-agnostic. Git synchronization and search-index refresh remain **separate, explicitly invoked, recoverable operations**, not automatically executing jobs.

**Not yet approved:** exhaustive verb list; detailed flags; exact global-option parsing positions; request JSON schema; numerical exit codes; auto-completion; non-Git project discovery; QMD selection. **Explicitly deferred, without design work now:** resolution of `--project <slug>` when invoked outside that project's source repository. Commands shown below illustrate the intended interface but are not evidence of executable `awl` code.

**Previous research:** The [2026-10-23 AgentWorklog core-requirements working specification](https://drive.google.com/file/d/1Y0GFyS76sZWsEaJ5k0CQ6qoUo33Diqzf/view) proposed `awl read muse/Activities/example.md` and `awl ls muse/Activities` as *non-normative* examples. The owner has since favored **separate implicit/explicit project selection** instead of embedding the slug in the document path. [PR #2](https://github.com/giacomobartoloni/awl/pull/2) contains project/worktree configuration and a candidate `awl read Activities/example.md` command; [PR #4](https://github.com/giacomobartoloni/awl/pull/4) contains request-level Git publication and audit behavior; [PR #5](https://github.com/giacomobartoloni/awl/pull/5) evaluates QMD. All are still documentation proposals, not merged runtime functionality.

## 2. Proposed command surface (illustrative)

| Group | Example | Purpose and boundary |
| --- | --- | --- |
| Bootstrap | `awl init` | Explicitly initialize/bind the current project; no setup side effects from reads |
| Discovery | `awl project list` | List visible configured projects, including their identities; discovery mechanism TBD |
| Documents | `awl ls Activities` | List project-relative Markdown/document paths |
| Documents | `awl read Activities/example.md` | Return content or revision-pinned excerpts; no write side effects |
| Documents | `awl create Specs/new.md --input -` | Create only if absent; accept safe multiline input |
| Documents | `awl edit Specs/new.md --input -` | Targeted **body-only** edit, not unconditional whole-file replacement |
| History | `awl history Specs/new.md` | Read document history within the managed Git authority |
| Metadata | `awl metadata get Specs/new.md /status` | Read a YAML-frontmatter value by JSON Pointer |
| Metadata | `awl metadata set Specs/new.md /status --input -` | Typed YAML value mutation; other get/remove/insert/append operations remain in logical scope |
| Composite | `awl apply --input -` | One ordered request with complete member outcomes and one audited Git publication boundary |
| Retrieval | `awl search "architecture" --mode lexical` | Search project documents and report index revision/freshness; QMD is a candidate |
| Remote | `awl sync` | Explicit fetch, conservative reconciliation, push and remote verification |
| Index | `awl index refresh` | Explicit lexical/content index refresh on committed documents |
| Index | `awl index embed` | Explicit vector embedding update after appropriate model provisioning |
| Status | `awl status` | Local, remote and search-index status, each with its own freshness/uncertainty |
| Audit | `awl request status <request-id>` | Recover a published request outcome using request ID/digest where available |

This is **one possible verb taxonomy**, not a new list of mandatory P1 implementation features. In particular `init`, `ls`, `read`, `create`, `edit`, `apply`, `status`, `sync`, `search`, `index` and `metadata` are proposed spellings; operation semantics already recorded in the requirements take priority. `project list`, `metadata ...`, `request status` and `index ...` are grouped subcommands only where a second object/operation distinction materially helps. The single object-type prefix `document` is intentionally absent.

Example for an agent from a configured source worktree:

```sh
awl read Activities/example.md
awl edit Activities/example.md --input - --json
awl apply --input - --json
awl status --json
```

Explicit selection while already in a configured `muse` source worktree (illustrative):

```sh
awl --project muse read Activities/example.md
awl --project muse sync
```

**These examples do not specify or promise behavior for `--project muse` from outside the `muse` source repository.**

**Input/output convention:** `--input -` and `--json` are *candidate* flag names; the 2026-11-04 [PR #2 CLI section](https://github.com/giacomobartoloni/awl/pull/2) used `--format json` and structured stdin as examples. Choose one output spelling during final parser review. Structured input must remain unambiguous and must not rely on shell interpolation; no mutation may require caller-supplied Git revision IDs.

## 3. Implicit project-resolution contract

For a project-scoped command **without** `--project`:

1. Use the process invocation working directory to discover an enclosing **source Git worktree** and its **common Git directory**; work from a nested subdirectory as well as a worktree root.
2. Read the source worktree's `.awl/project.yaml` for the shared project identity. Read canonical personal settings via the source Git **common directory**, not per-worktree copies; a local symlink is optional.
3. Validate the accepted initialized project binding and managed-repository identity. Incompatible `.awl/project.yaml` between branches/worktrees, unexpected `.awl/local.yaml` files/symlinks, an unsafe managed-path override or a different remote target must be **reported before any write**.
4. Resolve the **same project and managed Git store** from all linked source worktrees with a compatible shared binding.
5. If the context is missing, ambiguous, unsupported or not initialized, **fail with an actionable error**. Do not guess from source-folder name, use the last accessed project, consult the source Git `origin` as a worklog remote, create a new managed repository or silently fall back to another project.

Document path arguments are **project-relative document paths**, regardless of whether the CLI was invoked at the source worktree root or a nested directory. Do not combine the slug with the path in the new canonical examples. Reject absolute/out-of-scope paths, traversal and unsupported symlink escapes.

**No auto-initialization:** `read`, `ls`, `search`, `status` and other ordinary operations must not create source config, a managed Git repository or a search index as a side effect of resolving project context. The specific `init` command is an explicitly invoked setup boundary.

## 4. Explicit `--project <slug>` selection

- Within a resolvable source-repository context, an explicit selector must not bypass project authorization, source-binding validation, Git audit, or target path rules. The full precedence/override behavior remains a CLI parser design detail.
- Reject unknown, duplicate or ambiguous slugs rather than choosing whichever configured project is found first; never treat a source repository's name or an arbitrary filesystem directory as the slug.
- Do **not** silently combine one source worktree's identity/configuration with a different project's managed repository. Selection of another project from an unrelated source worktree is **not specified by this initial contract**.
- **Owner-directed scope deferral (2026-11-06):** Do **not** define, design, or select a mechanism for resolving `awl --project muse ...` when executed **outside the `muse` source repository**. In particular, do not specify a global project registry, directory search, mapping file, lookup algorithm or out-of-context error contract. This use case is excluded from the first implementation contract and its acceptance tests. Revisit only after a separate, explicit product decision.
- Keep parsing deterministic. Repeated conflicting project flags and unknown options are errors; precise flag position rules and the relationship to future environment variables remain unselected.

## 5. Initialization boundary

`awl init` is a **candidate name** for the explicit first-use operation described in [PR #2 §9.3](https://github.com/giacomobartoloni/awl/pull/2). It is distinct from implicit project resolution.

In a source Git worktree, setup should validate or prepare `.awl/project.yaml`, check that `.awl/local.yaml` cannot be accidentally tracked, establish the canonical local association via the common Git directory, and provision/reuse **one** awl-managed Git repository under user application data. Never silently rewrite existing user files, `.gitignore`, source Git config or worklog content. A linked worktree invoking `init` must discover and reuse an existing compatible binding, not create a duplicate.

**Owner-selected:** a Git remote **may be omitted during initial project setup** and added explicitly later. **Open:** how a new project's slug and any optional remote are provided (flags, config-first, or safe optional prompt); how stand-alone projects without source Git are initialized; and whether `--project` selects a pre-existing binding during setup or a separate `--slug`/positional input identifies a **new** project. Do not overload a selection flag with implicit create semantics without review.

## 6. Process, publication and synchronization guarantees

- A command that mutates managed Markdown goes through the one approved **local Git request publication** boundary and the complete operation-level audit. A completed no-change/all-ordinary-failed request makes one audit-only commit; an unsafe concurrent request-level conflict makes **no published commit**. Ordinary member failures within an accepted composite continue. Caller-supplied revision tokens are not required for writes.
- A successful local mutation response **never waits** for remote sync or QMD. `awl sync`, `awl index refresh` and `awl index embed` are explicit, recoverable operations; their own callers may wait. There is **no mandatory daemon or queued background worker** in the first slice.
- `awl sync` owns Git fetch/reconcile/push/confirmation rather than delegating `git pull` to a QMD hook. `awl index` refreshes only a controlled projection of committed document content; a Git **audit-only** commit does not require document reindexing when the indexed tree is unchanged.
- Use a stable human result plus machine-readable result mode with explicit local/remote/index states, actionable error categories and sensible exit statuses. Never conflate a remotely rejected push or a stale index with a locally rejected request. If a response is lost or publication uncertain, report/recover uncertainty rather than fabricating success/failure.
- Go and **external Git CLI** were owner-selected for the first implementation increment; QMD remains the **first search engine to test**, not an approved mandatory runtime dependency. Neither an embedded Git library nor a special agent harness is required.

## 7. Acceptance examples for project-aware CLI design (proposed, not executed)

| ID | Given / when | Expected result |
| --- | --- | --- |
| CTX-AC-01 | `awl read` invoked at source root and its nested directory | Same project and document root; relative document path resolution unchanged |
| CTX-AC-02 | `awl read` invoked from two linked worktrees with consistent `.awl/project.yaml` | Same canonical personal config and managed Git repo; no per-worktree duplicates |
| CTX-AC-03 | `awl read` without `--project` outside any recognized initialized source context | Actionable `PROJECT_NOT_RESOLVED`-class error; no arbitrary project fallback |
| CTX-AC-04 | The shared branch's slug or remote target conflicts with initialized local binding | Fail before writing; no silent project rebind or new managed repository |
| CTX-AC-05 | From the initialized `muse` source worktree, invoke `awl --project muse read ...` | Select only the already validated `muse` binding; do not infer a different repository or context |
| CTX-AC-06 | From an initialized source worktree, specify `--project` with a slug incompatible with its validated binding | Never silently redirect its writes to another managed repository; cross-project lookup and outside-repository resolution remain out of scope |
| CTX-AC-07 | Safe re-run of `awl init` from another linked worktree | Reuse compatible initialized association; no duplicate Git store, overwrite or automatic source commit |
| CTX-AC-08 | `awl read` before initialization | Read-only failure; no new config, Git store, search index or linked worktree created |
| CTX-AC-09 | `awl create`, `awl edit` or `awl apply` with a project resolved implicitly | Same audit/concurrency and one-request publication guarantees as an explicitly selected project |
| CTX-AC-10 | Local write succeeds, network and QMD are unavailable | Local commit acknowledged; remote/index states pending/unavailable, no automatic job run |
| CTX-AC-11 | JSON mode invoked without a TTY; operation fails or returns partial composite results | Stable structured output plus process exit classification; no pager/prompts/ANSI noise in JSON |
| CTX-AC-12 | An existing legacy Markdown/wikilink/frontmatter file is read | Preserve original bytes; no implicit normalization, reformatting or migration |

## 8. Review gates and scope control

1. **Confirm concrete command spellings** and whether `awl project list` is an exception or `awl projects` should be used. Resolve `--json` versus `--format json`, structured input flag and stdin ambiguity.
2. **Defer out-of-repository explicit project selection.** No lookup, global registry, mapping, directory search or acceptance contract for `awl --project muse ...` outside the `muse` source repository is needed now. Test only source-context identity and remote validation in the first increment. Stand-alone non-Git project setup remains a separate future topic.
3. **Specify initial `init` inputs**, partial setup recovery, same-source linked-worktree behavior and path/ignore safety, without changing source Git config automatically.
4. **Verify CLI parser and process behavior** on Go code using representative harness shells, read/write inputs, failures, non-TTY execution and linked worktrees. Do not substitute mock help output for tests.
5. **Keep PRs separate:** PR #2 configuration/worktrees; PR #4 concurrency/audit; PR #5 QMD evaluation; this document supplies a **cross-cutting CLI-facing proposal** for eventual reconciliation, not a backdoor merge of their unmerged requirements.

**Evidence limit:** This document records the user's CLI-context decision and derives a safe interface proposal from existing repository and AgentWorklog sources. **No CLI implementation, runtime parser, integration test, init/bootstrap, Git commit from awl, search index, merge or ADR** is claimed.


## 9. Owner-selected local-only bootstrap and later remote setup (2026-11-06)

**Owner-selected:** `awl init` may create the managed Git project **without a remote**. `awl remote add <git-url>` then configures the first authoritative **Git** remote explicitly, with no automatic fetch, push, search-index refresh or daemon. Both command spellings are implemented only as a **bounded experimental Go CLI** in [PR #7](https://github.com/giacomobartoloni/awl/pull/7); their final flag/parser/exit-code design remains reviewable.

```sh
# Within an initialized source Git worktree:
awl init --slug muse                   # local-only bootstrap
awl create Activities/one.md --input -  # valid offline write
awl remote add git@example.invalid:team/muse-worklog.git  # configuration only
# awl sync is separate and is not implemented by this experiment.
```

Remote setup must preserve existing local documents, the managed Git head and request audit. Re-adding the same URL is idempotent; conflicting replacement is rejected and left for a future migration policy. Source `.awl/project.yaml` and the Git-common-dir canonical binding both reflect the optional URL; interrupted two-file config updates remain a recovery/durability design gate.

**Do not conflate sync modes:** the planned remote is Git authority; rsync and rclone are candidate satellite transports, not automatically interchangeable remote backends. See [PR #8](https://github.com/giacomobartoloni/awl/pull/8). Out-of-source-repository `--project` lookup remains explicitly deferred.
