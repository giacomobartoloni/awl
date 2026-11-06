# Experimental Go CLI: Local Initialization and Markdown Publication

**Status:** bounded experimental implementation; **not production-ready, not a merged or approved public CLI schema**.  
**Date:** 2026-11-06.  
**Project:** AgentWorklog **CLI** (`awl`), not the existing Markdown AgentWorklog conventions.  
**Code:** `cmd/awl`, `internal/awl` (Go standard library + external Git CLI).  
**Related:** [Project Configuration PR #2](https://github.com/giacomobartoloni/awl/pull/2) · [Request-Level Publication PR #4](https://github.com/giacomobartoloni/awl/pull/4) · [Command Context PR #6](https://github.com/giacomobartoloni/awl/pull/6).

## Scope and provenance

This executable spike exercises the owner's selected Go + external Git CLI stack, implicit project resolution from a source Git worktree, read-only document reads, byte-preserving local Markdown creation/body edits, and compact per-request Git commit-message audit. It is **not** the implementation of every operation in the proposed requirements. In particular, it does **not** select QMD or change the decision to defer external `--project` discovery.

The existing first-slice plan at `AgentWorklog/awl/Plans/2026-11-06-local-git-request-publication-first-slice-plan.md` explicitly allows a bounded technical spike before production gates are resolved. Previously reported 63 Git fixture scenario executions were **Python-only standalone mechanism tests**; the tests documented below exercise **real Go code** and must not be conflated with those fixtures.

## Experimental command examples

Run from a source Git worktree with configured effective `user.name` and `user.email`:

```sh
# New binding: creates .awl/project.yaml and a separate local managed bare Git store.
# Remote is recorded but no network operation is performed.
awl init --slug demo --remote 'git@example.invalid:team/demo-worklog.git' --json

# Direct project-scoped commands, with the project derived from the source worktree.
printf '# Overview\n' | awl create Specs/overview.md --input - --request-id create-001 --json
awl read Specs/overview.md
printf '{"old":"Overview","new":"Summary"}' | awl edit Specs/overview.md --input - --request-id edit-001 --json

# A composite of ordered operations in one audited request.
awl apply --input - --json <<'JSON'
{
  "request_id":"request-002",
  "operations":[
    {"id":"first","op":"create","path":"Activities/one.md","content":"First\n"},
    {"id":"second","op":"edit","path":"Activities/missing.md","old":"Pending","new":"Done"}
  ]
}
JSON
```

These command spellings, input envelopes, numeric exit codes, and optional `--json` flags are **temporary test interfaces**, not approved product commitments. No `awl document` prefix exists in this spike. A leading `--project <slug>` can only confirm the **same** project already resolved from the source worktree; use outside the project's repository remains intentionally **undefined and unsupported**. No global project registry is introduced.

## Implemented mechanism (limited)

1. **Explicit init:** Resolve the invoking source Git root/common directory; validate or create a minimal `schema: 1` `.awl/project.yaml`; establish the canonical local binding under Git common dir; allocate a unique bare managed Git store under Linux XDG user data; create a bootstrap empty Git tree/commit **without changing source Git history**. Re-run from a compatible linked worktree reuses the same managed store. The private `.awl/local.yaml` symlink is optional and accepted only if correctly targeted, untracked and ignored. No network call, migration or modification to source `.gitignore`/Git settings is made.
2. **Read:** Read a single project-relative Markdown blob from one managed Git HEAD, returning original bytes on normal stdout. No read-side init or checkout; malformed/out-of-scope paths fail. The optional JSON mode reports revision and content and rejects invalid UTF-8 rather than silently lossy encoding.
3. **Write:** `create` refuses overwrite; `edit` replaces one unique byte-exact target **only inside the Markdown body** (with conservative YAML frontmatter-fence recognition and BOM handling); `apply` processes up to 100 create/edit members in order and continues after ordinary member errors. Unaffected bytes, frontmatter, CRLF/LF and links are preserved. This is **not** a general YAML/Markdown parser or structural merge engine.
4. **Audit and Git publication:** Stage private Git blobs/index, create a single content+audit commit for a completed changing request or an **audit-only commit** for completed no-op/all-ordinary-failed requests. Store compact versioned ordered member outcomes and request-ID+payload digest in the Git commit message. Publish with expected-old-ref `git update-ref`. Idempotent repeated IDs reuse a matching published result; same ID with changed content is rejected. Recheck operations against a new head on a failed CAS; abort if member outcomes change or history is replaced.
5. **Separation:** No automatic sync, QMD, daemon, background worker, metadata mutation, global discovery, source Git commit or silent remote publication.

The initial bootstrap currently **requires a remote URL** for a new binding; whether remote configuration can be omitted is an **open product design question**, not a permanent approved requirement.

## Environment and verification

Observed on the isolated tool environment on 2026-11-06: **Go 1.23.2**, **Git 2.47.3**, **Linux/amd64**. The implementation uses Linux/POSIX-style `syscall.Flock` for first-use serialization and link-based no-clobber configuration publication; no Windows/macOS portability claim is made.

Commands executed against this code in the isolated runtime:

```sh
go test ./... -count=1 -v
go vet ./...
go test ./... -race -count=2
```

**Observed:** 9/9 distinct Go integration tests passed, including exact CRLF/BOM byte preservation, ordinary errors and audit-only publication, same-ID replay, changed-payload rejection, composite partial completion, linked worktree reuse, invalid preflight rejection, optional canonical symlink safety, commit-message audit inspection, and **two concurrent disjoint creates**. A separate manual CLI smoke test verified actual `init`, `create`, `read`, `edit`, failed edit, and replay output. Go tests use temporary source and bare managed Git repositories, not the user's actual vault or production repository. No race-detector alert was reported in two consecutive test suite repetitions.

## Critical unverified gates and exclusions

- **No simulated same-target CAS collision was deterministically injected** into this Go implementation. Disjoint concurrent creates passed, but the strict rejected-conflict/no-published-commit contract needs controlled conflict/fault injection against real code.
- **Durability is not proven** under `SIGKILL`, host power loss or filesystem crash. The exact object/ref/fsync policy, unknown-outcome recovery, and whether speculative *unreachable* Git commits are permitted remain open.
- **Audit lookup scans Git history** and currently fails closed after ~5,000 commits. It is not a scalable index or proven duplicate-ID reconciliation algorithm across divergent remote histories. JSON duplicate-key input rejection and the exact versioned message schema still require design.
- **Configuration YAML is intentionally a narrow parser** for fields written by this spike, not a general YAML parser. It does not support arbitrary quoting, aliases or complex YAML structures. A production decision must address YAML validation/compatibility and explicit `init` inputs before relying on these bytes.
- **Edit safety is limited** to a unique literal body-text replacement. No semantic patch, concurrent same-target automatic merge, full frontmatter parser, structured YAML mutation, delete/rename, multi-page read, large-file memory budget, or binary document support is provided.
- **CLI and OS support remain experimental:** no Windows/macOS build or test; no pinned minimum Git floor; no stable request schema, option placement, help/version UX, output limits, standardized exit status, error schema, packaging, or installation/upgrade procedure.
- **Search/sync are separate future slices:** QMD is still an evaluation candidate, not a mandatory dependency; remote Git reconciliation and push remain unimplemented and are never triggered by local writes.
- This is **not a replacement for PR #2/#4/#6 review**, and no source/document migration, repository merge, automatic sync, global project lookup or production rollout is authorized by the test results.

## Next small increment

Before promoting this to a product implementation, review the actual Go code for input/path/Git environment safety and resolve the production-gating behavior in the existing plan. Prioritize deterministic same-target conflict injection; pre/post-ref process interruption and idempotent recovery; audit completeness and scalable lookup; and a coherent first-use configuration schema. Keep all changes in a draft PR until those review gates are satisfied.
