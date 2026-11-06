# Go Code Review and Local-Only Remote Configuration — Experimental Follow-up

**Status:** code review and test-backed experimental update; **not a production quality certification or an ADR**.  
**Date:** 2026-11-06.  
**Scope:** PR #7 Go implementation (`cmd/awl`, `internal/awl`) and the owner's request to initialize without a remote and assign an authoritative Git remote later. No Git network sync, rsync/rclone adapter, QMD integration or product release is implemented.

## 1. Scope and exact behavior

```sh
# Start a managed awl project completely offline/local-only.
awl init --slug demo --json
awl create Activities/one.md --input - --json < one.md
awl read Activities/one.md

# Store the initial remote location only when one is available.
awl remote add git@example.invalid:team/demo-worklog.git --json
# An identical second call reports remote_unchanged; a different URL is rejected.

# Still supported as a single-step choice:
awl init --slug demo --remote git@example.invalid:team/demo-worklog.git --json
```

- ".awl/project.yaml" may omit the `storage.remote` key entirely; the canonical `<git-common-dir>/awl/local.yaml` binding similarly omits `binding.remote`. Missing remote means **deliberately local-only**, not a corrupt project.
- `remote add` changes these two awl configuration files only. It checks the previously initialized source binding and managed bare Git repo, never creates a document/request commit, does not modify the source Git `origin`, and performs **no network I/O**. Retrying the same remote returns `remote_unchanged`; a different existing remote returns `REMOTE_ALREADY_CONFIGURED`.
- A crash/interruption after only one config file is updated leaves a detectable binding mismatch. An explicit identical `remote add` can repair those two narrowly defined partial states. This does **not** establish a cross-file atomic transaction, safe editing against an out-of-band writer, or power-loss durability.
- No new global project registry, remote transport type selector, `--project` out-of-repository lookup, automatic Git push, or rsync/rclone configuration was introduced.

## 2. Go idioms and Clean Code review: concrete observations

| Check | Finding | Action or gate |
| --- | --- | --- |
| **Standard idioms** | Uses Go's `error` return pattern, small unexported data types, `defer` for resource cleanup, `testing.T` with `t.TempDir()`, `io.Reader`/`io.Writer` for CLI streams, and `encoding/json`. Go stdlib-only module; no speculative plugin/dependency container. | Keep concrete types and avoid premature interfaces; idiom aligns with Go's [Code Review Comments](https://go.dev/wiki/CodeReviewComments) and [Effective Go](https://go.dev/doc/effective_go). |
| **Formatting/static checks** | All source is `gofmt`-clean; `go vet ./...` and two full `go test ./... -race -count=2` runs passed in Linux/amd64. | Not evidence of OS portability, crash durability or semantic conflict safety. |
| **Cohesion** | The previous large `gitstore.go` mixed config/bootstrap/remote mutation with Git object/read/write logic. | Split the concrete implementation into `awl.go` (CLI/operations input), `config.go` (binding/init/remote/config validation), and `gitstore.go` (Git subprocess and local history/audit). No new interface for a not-yet-implemented backend. |
| **Read-only concerns** | Initial resolution erroneously required Git author identity even to read a document or add a URL. | Corrected: only mutation/bootstrap that produces Git commits requires identity; later `read` and `remote add` work without it. Added integration test. |
| **Markdown/config fidelity** | The narrow YAML config reader initially rejected CRLF line endings during remote add. | Corrected parsing without normalizing original bytes; tests verify comments and CRLF survive one-sided repair. The configuration parser **is still not a general YAML implementation**. |
| **Shell / Git invocation** | `exec.Command` passes explicit Git arguments and strips inherited `GIT_*` environment variables before applying intended overrides. | Good bounded experiment; **missing context deadlines/cancellation**, remote credential/process environment controls and bounded Git-output handling need hardening before remote operations. Prefer `exec.CommandContext` with an explicit `context.Context` at the future sync boundary. |
| **Request integrity** | Private Git index, controlled ref update, request-ID/digest and commit-message audit demonstrate a useful local mechanism. | **Not production-proven**: same-target deterministic CAS fault injection, after-ref lost-response tests, unreachable objects, history-wide audit lookup scaling, duplicate JSON key detection, controlled crash/power-loss recovery remain gates. |
| **Config atomicity** | Config files are each written through a temporary file plus `rename`; one coarse cooperative awl setup lock is used. | This is **not cross-file ACID**, nor a proven guarded compare-and-replace against another external config writer. Preserve fail-closed mismatches and review fsync/permission/path/symlink handling. |
| **OS / portability** | Uses `syscall.Flock`, filesystem hard links and XDG Linux application-data path in the spike. | Platform-specific implementation; do not infer Windows/macOS support. Choose target OS profile and lock/fsync policy before shipping. |
| **Code size** | Local engine remains moderately sized and concrete; the current `config.go` handles both setup and narrow YAML parsing. | Prefer small behaviorally coherent functions and extracted files only where actual maintenance/tests require them; a universal `Repository`, `Provider`, `SyncStrategy` hierarchy is **not justified** by the current Go code. |

**Assessment:** the code is **gofmt/vet/race-test clean and more cohesive after targeted fixes**, but this does **not** justify claiming that all Go or Clean Code best practices have been satisfied or that the local publication engine is production-safe. The high-risk items above are explicit blockers. `staticcheck`/`golangci-lint` were **not executed**, as those tools were not installed in the available environment.

## 3. Test evidence (actual Go code)

On **Go 1.23.2**, **Git 2.47.3**, **Linux/amd64**, the following were executed on the local experimental source and passed:

```sh
gofmt -l cmd/awl/main.go internal/awl/*.go
# No formatting differences reported.
go test ./... -count=1
go vet ./...
go test ./... -race -count=2
```

**14 distinct top-level Go integration tests passed**, versus 9 on the prior PR #7 snapshot. The new ones cover local-only initialization, remote-add idempotency and conflict detection, invalid-remote rejection/no config mutation, one-sided interrupted config repair with CRLF/comment preservation, two concurrent identical remote adds, and identity-free reads/remote configuration after bootstrap. The tests use isolated temporary source and managed Git repositories; no actual network fetch/push, rsync or rclone invocation was tested. The race detector reports no race in Go-managed memory; it does not prove cross-process Git or filesystem crash safety.

## 4. Interface boundaries for future sync engines

The Go code intentionally does **not** acquire a `SyncBackend`/`Transport` interface solely because the product might later support rsync or rclone. Preserve a narrow concrete Git-local store. Review the independent [sync modes and backends design](../architecture/2026-11-06-sync-backends-and-transports-design-proposal.md) before writing network components:

1. Git remains authoritative under current merged requirements; `awl remote add` configures a **Git** URL, not an arbitrary rclone endpoint.
2. Git fetch/reconcile/push has different semantics from one-way rsync/rclone file projections. Do not hide the difference under a lowest-common-denominator `Sync()` interface.
3. Introduce capability-based ports only when a second actual transport is prototyped and tested, following Go's [avoid-unnecessary-interfaces](https://google.github.io/styleguide/go/best-practices.html) recommendation.
4. Keep explicit sync/index commands separate from locally accepted mutation commits. A missing Git remote is not a local-write failure.

## 5. Unresolved high-priority review tasks

- Determine precise `UNKNOWN_OUTCOME` and ref durability behavior under process termination, actual filesystem failure and lost-response retry.
- Deterministically inject incompatible same-target concurrent edits and prove rejected-request/no-published-commit behavior in Go.
- Decide production YAML semantics and forward-compatible configuration migration before adding arbitrary remote/provider fields.
- Add bounded subprocess cancellation, more robust error classification and output/path limits; validate security of Git environment/config and remote URL syntax, including credential-free SSH forms.
- Select portability profile, supported Git versions, implementation release and test strategy; measure history scanning overhead before removing the 5k-audit limit.
- Review options for out-of-band file writer races during two-file `remote add`; do not claim external-writer CAS or transactional updates from current tests.

**Verification limit:** No code from this branch was merged to `main`; no backend design ADR was accepted. This is an experimental refinement of PR #7 plus a separate architectural proposal, not a deployment milestone.


## 6. Follow-up source review: explicit Go hardening gates (2026-11-06)

**Source re-inspected:** current experiment branch files internal/awl/awl.go, internal/awl/config.go and internal/awl/gitstore.go, plus the existing Linux integration suite. This addendum is a **static review**, not another execution of go test/vet/race. The earlier recorded 14-test result remains the only stated runtime evidence.

### Required before promoting this to production

| Priority | Current code observation | Required idiomatic Go change / proof |
| --- | --- | --- |
| P0 — subprocess lifetime | gitstore.go:git uses exec.Command and cmd.Output without an invocation context, deadline or bounded capture. | Introduce request-scoped context.Context and exec.CommandContext; use explicit cancellation/timeouts appropriate to local vs network calls, bounded stdout/stderr and controlled process environment. No ambient shell. Ensure an interrupted command does not lie about ref publication. |
| P0 — accurate publication errors | writeRequest currently handles **any** failed git update-ref as if it were a lost compare-and-swap race. A repository I/O/permission failure may be retried and end as a misleading conflict. | Only retry when **the managed ref was actually observed to advance** and revalidation is safe. Preserve a typed infrastructure error if the ref remains unchanged; for ambiguous interruption, inspect whether the intended request commit was published before reporting a definitive result. |
| P0 — crash and CAS safety | CAS candidate commit is created before ref update; local races are partially tested, but the experimental suite does not inject same-target CAS failure or process death before/after accepted ref update. | Add deterministic hooks/fault fixtures with two processes, after-ref lost response and audit-only commit replay; distinguish unreachable speculative object creation from published history. State fsync/durability limitations. |
| P1 — configuration consistency | remote add updates source project.yaml and canonical local.yaml in two separate atomic-per-file replacements. Code checks existing content before rename while an out-of-band writer might race. | Treat a one-sided update as observable/recoverable; test changed input and symlink swaps, reject unexpected external concurrent edits, and decide cross-file durability before claiming atomicity. Preserve CRLF/comments and reject unsafe YAML. |
| P1 — metadata/input robustness | configRead is a consciously narrow YAML subset; strictJSON rejects unknown fields/trailing values but does not detect duplicate object keys. | Select and test a real safe YAML strategy before supporting arbitrary project config; add duplicate JSON key rejection for machine requests and bounded audit parsing. Do not introduce broad YAML reserialization that modifies unrelated bytes. |
| P1 — audit growth | findAudit scans up to roughly 5,000 commits, failing closed above that limit. | Measure the real lookup cost/retention and design a correct recovery/lookup approach without prematurely adding another authoritative database. |
| P1 — platform guarantee | Linux XDG paths, syscall.Flock and hardlink publication are platform-specific. | Choose explicit supported OS/filesystem profile and run cross-platform lock/atomicity tests before portability claims. |

### Clean Code / YAGNI choices that remain intentional

- Keep the existing small package and **concrete** local Git functions for now; splitting CLI input, config/bootstrap and Git store was useful because each has independent behavior/tests. Do not create empty packages, a service locator, inheritance-like abstractions or a generic provider manager.
- Future GitSync and snapshot export have **distinct semantics**. Introduce a tiny consumer-owned interface only when a second tested implementation requires one; see the separate [capability-based sync design](../architecture/2026-11-06-sync-backends-and-transports-design-proposal.md), PR #8. Do not retrofit rsync/rclone into the local document writer.
- Follow Go idioms: errors returned and wrapped with context; errors.Is/As for domain boundaries; context passed explicitly as the first argument to cancellable operations; explicit ownership of locks/files; table-driven cases where useful; use temp repositories for integration tests. Favor readable functions over arbitrary method-count/line-count rules.
- Record reproducible review evidence: gofmt, go vet, go test, go test -race and any additional linter results with tool versions and environment. **A clean linter result is not a substitute for conflict/crash correctness.**

**Verification status:** documentation-only addendum; the code and test results have not changed in this amendment. These issues are **review gates**, not claims that the behavior is already corrected.
