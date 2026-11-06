# Experimental CLI Contract Consolidation — Review Matrix

**Date:** 2026-11-06  
**Status:** reviewed experimental behavior on draft PR #7, **not** a final parser, JSON, exit-code, or production-readiness specification.  
**Authority:** GitHub implementation on `experiment/go-local-init-read-write-2026-11-06`; owner-selected command-first context direction remains a **proposal** in [PR #6](https://github.com/giacomobartoloni/awl/pull/6), and modular-monolith ADR 0001 is on [PR #8](https://github.com/giacomobartoloni/awl/pull/8).

## Scope and provenance

This review consolidates the behavior exposed by the actual `cmd/awl` executable and the `internal/awl` package. The source Git repository/worktree determines the **project context**; its working tree is **not** the document store. Documents live in a separate, local managed bare Git repository, and ordinary reads do not bootstrap one. The current executable uses an intentionally narrow, Linux-oriented Go + external-Git implementation.

The table below describes **currently implemented experimental behavior**. It does not approve final spelling, change proposed requirements, or imply behavior in `main`.

## Observable command matrix

| Command | Accepted experimental invocation | Plain-text success | JSON success | Boundaries / rejection |
| --- | --- | --- | --- | --- |
| `init` | `awl init --slug demo [--remote GitURL] [--json]` | `initialized` | `status=initialized`, `project` | Explicit bootstrap only; source Git worktree required; optional remote configuration does no network operation; incompatible binding rejected |
| `remote add` | `awl remote add GitURL [--json]` | `remote_added` / `remote_unchanged` | `status`, `project` | Requires initialized local binding; same URL idempotent, changed URL rejected; no managed document commit or network I/O |
| `ls` | `awl ls [directory] [--json]` | Sorted, project-relative Markdown paths, one per line | `status=ok`, `project`, `revision`, `paths` (empty array when none) | Enumerates ordinary Markdown blobs in one committed **managed** Git snapshot, not source Git; optional validated relative directory; read-only |
| `read` | `awl read document.md [--json]` | Raw original bytes, no forced newline | `status=ok`, `project`, `revision`, `content` | Exact project-relative Markdown target; missing document is an error; JSON mode requires UTF-8; read-only |
| `create` | `awl create document.md [--input file\| -] [--request-id ID] [--json]` | `applied <commit>` on success | Publication status, `project`, `request_id`, `commit`, `document_tree`, `members` | Does not overwrite existing document; content is UTF-8; one audited local request |
| `edit` | `awl edit document.md [--input file\| -] [--request-id ID] [--json]` | Publication status plus commit | Same mutation response as `create` | JSON input with nonempty `old` and `new` text; unique exact target in body only; preserve frontmatter and unrelated bytes |
| `apply` | `awl apply [--input file\| -] [--json]` | `applied`, `partial`, or `failed`, plus commit where published | `status`, `project`, `request_id`, `commit`, `document_tree`, ordered `members` | JSON composite of `create`/`edit`; ordinary member failures continue, request-level fatal conflicts abort; stable request ID enables replay |

`--project <slug>` is a **leading** experimental selector and may only confirm the identity already resolved from the current source worktree. Its out-of-repository discovery behavior is deliberately **deferred**.

For commands supporting a path before options, the current parser accepts `awl read Activities/a.md --json`, `awl ls Activities --json`, and `awl edit Activities/a.md --input - --json`. This is an observed implementation subset, not a general parser proposal.

## Error, exit, and revision semantics

- Experimental `--json` errors have a structured `status=error` and `error={code,message}`; plaintext errors are written to stderr as `awl: CODE: message`. The consolidation fixes parsing failures where a preceding invalid option previously obscured a later explicit `--json`; an option **value** equal to `--json` does not request structured output.
- The observed exit classes are `0` for accepted successful commands, `2` for rejected or all-ordinary-failed requests, `3` for accepted partial composites, and `4` for a JSON-output encoding failure. These numbers are **implementation observations**, not approved public constants.
- `ls --json` and `read --json` report the managed **commit** revision observed for their respective read. Separate invocations can naturally observe different commits if another writer publishes between them; there is **no** multi-command snapshot transaction. Local read/list does not fetch/push a remote.
- A valid no-op or ordinary failed mutation can still publish an audit-only commit with an unchanged document tree. Therefore `revision` may advance without a change to listed Markdown paths.
- The experimental Git-backed publication path validates managed project context, rejects unsafe document paths, and uses an audited conditional ref update. Neither passing CLI tests nor a successful CI workflow establishes power-loss durability.

## CLI user-acceptance coverage

| Concern | Black-box real-binary coverage | Remaining evidence limit |
| --- | --- | --- |
| Local-only first use, `init`/create/read/edit and `remote add` | `TestCLIContractLocalOnlyProjectWorkflow` | Standalone non-Git project initialization not specified |
| Mixed-success composite, error status and replay | `TestCLIContractCompositePartialResultAndIdempotentRetry` | Full audit/collision and failure-injection proofs remain separate backend tests |
| Invalid path, implicit/explicit project mismatch | `TestCLIContractProjectAndInputErrorsAreObservable` | Final path/exit schema is not approved |
| `ls` sorting, source-vs-managed separation, prefix, revision, read-only behavior and errors | `TestCLIContractList*` | Nonregular managed blobs and large repository limits are not comprehensively characterized |
| `read` and `ls` agree on revision, UTF-8 Markdown bytes and nested source worktree context | `TestCLIContractReadAndListAgreeOnCommittedSnapshot` | The command pair does not promise atomic cross-command reads |
| Structured errors even when invalid flags precede `--json` | `TestCLIContractJSONErrorOutputAcrossArgumentOrders` | General flag grammar, value parsing and JSON/exit schema still need final review |

These tests belong to the existing `CLI user-contract tests` GitHub Actions step (`go test -race -count=2 -run TestCLIContract ./internal/awl`). Separate formatting, vet, full race suite and repeat fault-injection steps are supporting verification layers. See PR #7 for **actual run status**; do not infer a pass merely from the presence of tests.

## Intentional non-goals and review gates

**Not implemented:** document history; YAML metadata operations; project discovery outside a resolvable source Git context; local/remote/index status; fetch/reconcile/push; search; index refresh; remote provider/rsync/rclone adapters; generalized YAML or Markdown parsing; Windows/macOS guarantees.

**Before merge/promotion:** review CLI grammar and errors against [PR #6](https://github.com/giacomobartoloni/awl/pull/6); reconcile ADR 0001 and source Git/project setup design; resolve remaining P0 publication/uncertain-outcome/durability conditions; validate practical Git/OS/version compatibility; perform independent security review and decide how to expose untrusted path/content boundaries. Maintain the existing experimental draft state until those gates are explicitly evaluated.

**Decision boundary:** This review only tightens the current experiment's observable error behavior and test evidence. It does **not** select final parser rules, production JSON envelopes, additional commands, storage providers or a release date.
