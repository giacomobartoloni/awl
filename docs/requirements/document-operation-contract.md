# AgentWorklog CLI — Document and Operation Contract

**Status:** draft functional specification for review, **not an approved command/API schema**.  
**Date:** 2026-10-23.  
**Product:** AgentWorklog CLI (awl).  
**Related:** [Core Requirements](core-requirements.md) · [Git Hub-and-Spoke Architecture Evaluation](../architecture/2026-10-23-local-first-storage-evaluation.md) · [YAML paths and partial reads research](../research/2026-10-23-yaml-paths-and-partial-reads.md).  
**Provenance:** The user's 2026-10-23 product discussion about LLM-agent edit tools, composite operations, local-first Git persistence, and a central remote Git hub.

## 1. Objective

Define the **observable behavior** of awl operations on project-scoped Markdown documents and YAML-frontmatter metadata. The interface should be predictable for LLM agents without exposing Git details or requiring direct mutation of a managed local worktree.

This document proposes a **logical contract**. Operation identifiers and JSON/YAML envelopes below are examples, not finalized CLI command names, an MCP schema, serialized protocol or implementation language. This specification does not authorize implementation.

### Confirmed product boundaries used as inputs

- Projects are addressed by **slug**; document paths are relative to their project.
- Documents are portable Markdown with arbitrary YAML frontmatter. The CLI does not impose Activity/Spec/Plan-specific lifecycles.
- awl maintains managed **local Git** state, usable offline, and publishes shared history through the project's authoritative **remote Git**. Satellite systems such as Google Drive integrate at the Git hub, not the local editing API.
- Client writes go through awl's operations rather than directly changing managed local files. This is a supported API boundary, not an OS-level security guarantee.
- awl supports individual and **composite** document/metadata operations with per-operation outcomes and audit correlation. Global multi-document transactionality or rollback is not required.
- A successful local operation must be distinguishable from remote Git publication and optional satellite propagation.
- **Owner-selected decisions (2026-10-23):** body-only text edits; safely applicable exact edits may proceed despite stale read revisions; composites continue on member errors; one local Git content commit per request that results in changes.
- User-owned content and legacy links/frontmatter are not implicitly migrated or normalized.

### Explicitly not specified here

CLI syntax, Git object/branch layout, service/MCP interfaces, Git libraries, authentication, satellite integrations or their execution location, exact conflict algorithms, patch formats beyond candidate behavior, query/indexing, and deployment.

## 2. Logical resources and terms

| Name | Meaning | Contract considerations |
| --- | --- | --- |
| Project | Slug-addressed awl context | Slug need not match a source repo or remote repository name |
| Document | Markdown resource at a project-relative path | Path names are user-owned; only explicit operations can alter them |
| Content | Full raw Markdown, potentially including YAML frontmatter | Read results should not silently normalize bytes |
| Body | Markdown content outside YAML frontmatter, when it can be recognized | Text editing versus raw-document editing scope remains open |
| Metadata | YAML values in frontmatter | Arbitrary keys/types; no domain-specific interpretation |
| Document revision | Opaque local state identifier returned with a read | May be used to detect or recover from stale edits; concrete Git mapping TBD |
| Request ID | Correlation identifier for one submitted operation or composite | Identity source/generation and retry semantics TBD |
| Operation ID | Identifier for one member of a composite | Enables ordered, per-operation reporting |
| Local commit | State accepted and retained by managed local Git | Not necessarily published to the remote Git hub |
| Publication state | Whether the relevant Git history is accepted at the central Git remote | Independent from local operation success |
| Satellite state | Whether an optional integration reflected remote Git state | Not a precondition for successful local edits |

**Visibility invariant:** When awl reports a change as *locally applied*, the resulting revision is durable and discoverable after restart. If durability cannot be established, the operation must not report success. This is a safety outcome, not a mandate to use an RDBMS/ACID transaction system.

## 3. Candidate operation set

This table describes **behavioral operations**, not finalized public method names.

| Candidate operation | Input concept | Success result | Important failure |
| --- | --- | --- | --- |
| `project.list` | Optional selection constraints | Accessible configured project descriptors | Project/config access problem |
| `document.list` | Project slug, optional relative prefix | Document paths and optional revisions | Unknown project, invalid prefix |
| `document.read` | Project slug, document path, optional excerpt selector | Content/excerpt + local revision + freshness context | Not found, unreadable, invalid path |
| `document.create` | Project slug, absent path, Markdown content | Created local document revision | Already exists; never silently overwrite |
| `document.edit` | Project slug, existing path, ordered **body-only** targeted text edits | New local revision and applied-edit outcomes | Missing/ambiguous target, unsafe stale edit |
| `metadata.get` | Project slug, path, optional key | Typed frontmatter value(s) + document revision | Missing document; malformed/ambiguous YAML |
| `metadata.set` | Project slug, path, metadata key and typed YAML value | Updated local revision | Invalid key/value, parsing failure |
| `metadata.remove` | Project slug, path, metadata key | Updated local revision or explicit no-change result | Parsing failure; missing-key behavior TBD |
| `composite.apply` | Project slug, ordered operations with IDs | Continue-on-error with per-operation outcomes + one resulting Git content commit when content changed | Partial completion is reportable, never silent |
| `operation.status` / `project.sync_status` | Request ID or project slug | Audit and local/remote progress | Unknown request, unavailable status |
| `document.history` | Project slug, document path | Relevant local history identifiers | Missing document/history unavailable |

All writes require a local Git-backed project state; offline writes **must not require remote network availability** once the project has been initialized locally.

**Later / unselected:** unified-diff or Codex-style `apply_patch`, context/hunk matching, hashline editing, a semantic Markdown-section editor, metadata query, content search, document moves/deletes and higher-level worklog lifecycle operations. Do not imply these are implemented or forbidden forever.

## 4. Request and response concepts

Candidate request envelope:

~~~json
{
  "request_id": "req-example-17",
  "project": "muse",
  "operations": [
    {
      "id": "edit-outcome",
      "op": "document.edit",
      "path": "Activities/example.md",
      "base_revision": "rev-example-1",
      "edits": [
        {
          "old_string": "Pending verification.",
          "new_string": "Implementation completed."
        }
      ]
    }
  ]
}
~~~

The same operation model can be submitted as a single action or within a composite. A one-operation wrapper is an illustrative transport choice, not a required JSON schema.

### Common response information

- **Request identity:** request ID and the project slug actually addressed.
- **Per-operation outcome:** submitted operation ID, operation kind, status, structured error if any, and relevant local revision before/after a change.
- **Request summary:** applied/failed/not-executed counts; a composite may have a *partial* outcome.
- **Publication state:** clearly distinguish local Git acceptance from central Git publication. Optional satellite state, when available, is independent.
- **Machine usability:** machine-readable error code and contextual fields; human-readable diagnostic message as supplemental text.
- **Audit correlation:** enough information to retrieve the locally recorded intent and outcome. Commit IDs may be returned but must not replace the operation ID.

Example successful local response (illustrative values and serialization):

~~~json
{
  "request_id": "req-example-17",
  "project": "muse",
  "status": "applied",
  "operations": [
    {
      "id": "edit-outcome",
      "status": "applied",
      "path": "Activities/example.md",
      "previous_revision": "rev-example-1",
      "revision": "rev-example-2"
    }
  ],
  "git_publication": "pending"
}
~~~

`applied` means **durably accepted locally**; it is not shorthand for Git push, Google Drive export, or semantic validation.

### Revision visibility

A read returns the local revision used to produce its content. Later writes may refer to that revision.

**Selected policy for body editing:** a stale `base_revision` does **not by itself** force failure or a reread. The exact target must be uniquely found in the **current Markdown body**, and awl must be able to establish that applying the requested replacement is safe. Otherwise return `TARGET_NOT_FOUND`, `AMBIGUOUS_MATCH` or `REVISION_CONFLICT`. No fuzzy matching or blind last-writer-wins fallback.

The exact safety condition for "still safely applicable" needs focused design and tests; simple string uniqueness does not prove semantic correctness. Strict revision preconditions could be an optional mode, while contextual patch/rebase/three-way merge remains deferred.

This stale-body-edit policy must not be presumed safe for all metadata edits, particularly numeric indexes into changed YAML sequences.

## 5. Reading documents

A read should return:

- The content being read (full document or explicitly requested excerpt).
- The corresponding **local document revision**.
- The project/path identifying that content.
- An indicator that content comes from local state, not a guarantee that the Git remote is current.

Optional bounded reads are desirable for agent token efficiency, but the choice of **line-, byte- or character-based ranges**, base index (0 or 1), and whether returned line numbers are included must be specified before shipping that feature. Do not silently mix indexing schemes.

If an agent receives line numbers or anchors, these can be used as *hints* for later edits; line number alone is not a safe stale-write precondition.

Reading an existing legacy document must not modify its YAML, links or line endings.

## 6. Targeted document editing

**Owner-selected scope: `document.edit` modifies only the Markdown body.** YAML frontmatter can be changed only with structured `metadata.*` operations.

A candidate minimal editing operation describes ordered `old_string` → `new_string` replacements, inspired by coding-agent Edit tools. The public wire schema remains a proposal.

Proposed single-operation behavior:

1. Load a working copy of the current local Markdown document and identify the body boundary without altering frontmatter.
2. Apply each declared replacement in order against the working body only.
3. By default each `old_string` must match exactly **one** body occurrence; zero matches => `TARGET_NOT_FOUND`; multiple => `AMBIGUOUS_MATCH`. A possible opt-in `replace_all` is not yet approved.
4. If one replacement fails, the entire **individual `document.edit` operation** is unsuccessful and contributes no modification to the request's staged state. Earlier independent operations in the same request remain eligible to commit.
5. If all replacements succeed, stage the complete resulting document for **the request's single eventual local Git commit**. No partial-document write or unrelated frontmatter normalization is acceptable.
6. Return per-edit diagnostics identifying the failing edit index, and optionally bounded current body context so an agent need not issue a full reread.

Example (illustrative):

~~~yaml
id: finish-outcome
op: document.edit
path: Activities/example.md
base_revision: rev-example-1
edits:
  - old_string: "Pending verification."
    new_string: "Implementation completed."
  - old_string: "## Follow-up\nTBD"
    new_string: "## Follow-up\nNo further action required."
~~~

The old revision can be stale, but the current body target must still be safely and uniquely applicable. An edit matching text **only inside frontmatter** is reported as missing; it must never edit YAML by accident.

**Deferred:** context-aware unified diffs, line/hash anchors, three-way merge, strict-conditional mode syntax and permitted automatic retries. Body/frontmatter boundary detection for malformed documents still requires a documented policy.

## 7. Generic frontmatter metadata

Treat metadata as **arbitrary YAML data**: do not interpret `type`, `status`, `workflow-status`, or custom keys as workflow commands. Structured metadata operations are the **only** supported way to mutate frontmatter.

**Nested-path notation is open.** [Research](../research/2026-10-23-yaml-paths-and-partial-reads.md) compares dot paths (yq), RFC 6901 JSON Pointer, RFC 9535 JSONPath, typed segment arrays and RFC 6902 JSON Patch. JSON Pointer is the current recommendation for a **single-node locator**, not yet a settled command syntax. Examples below use top-level `key` for simplicity.

Candidate input:

~~~yaml
id: complete-activity
op: metadata.set
path: Activities/example.md
key: workflow-status
value: completed
~~~

And removal:

~~~yaml
id: remove-obsolete-key
op: metadata.remove
path: Activities/example.md
key: obsolete-field
~~~

Minimum behavior:

- `metadata.get` returns parsed values without a lossy string-only transformation.
- `metadata.set` accepts typed YAML-compatible values (including objects, lists, booleans, numbers and `null`, if supported by the selected YAML subset).
- `metadata.remove` removes the key; it must be distinguishable from setting the value to YAML `null`.
- Unknown fields survive unrelated changes. Body content, ordinary links and unrelated metadata must not be intentionally rewritten or normalized.
- Duplicate YAML keys, aliases, comments, malformed frontmatter and nested-path semantics need an explicit policy rather than silent data loss.
- Metadata mutation must fail safely on content it cannot interpret or preserve reliably.
- Whether missing frontmatter is created automatically and whether a missing removal target is a no-op or error remain open.

## 8. Composite requests

A composite is **one request containing ordered, heterogeneous operations**; it is not a workflow engine.

**Owner-selected execution policy: continue after each operation failure.** Every operation is attempted in declared order unless a request-level fatal error prevents further execution. Later operations observe successfully staged earlier changes, even when an intermediate operation fails. A failed operation contributes **no** partial modification of its own.

**Owner-selected commit granularity: one local Git content commit per request.** If at least one operation produces a valid content change, all successful staged changes are represented by **one** local Git commit after processing the request. If all operations fail or produce no changes, no empty content commit is required; the outcomes still need auditable representation.

This is not an ACID transaction mechanism and does not require rollback across separately accepted requests. If final Git persistence fails, **none** of the staged operations may be reported as durably `applied`; a failure/uncertain outcome must be returned with recoverable diagnostics. The handling of interruption and retry IDs remains open.

Example request:

~~~yaml
project: muse
request_id: req-example-18
operations:
  - id: update-status
    op: metadata.set
    path: Activities/example.md
    key: workflow-status
    value: completed
  - id: edit-outcome
    op: document.edit
    path: Activities/example.md
    edits:
      - old_string: "Pending verification."
        new_string: "Implementation completed."
  - id: create-follow-up
    op: document.create
    path: Plans/next-phase.md
    content: |
      # Next Phase
~~~

Example of a **partial** outcome assuming `edit-outcome` failed but other operations succeeded:

~~~json
{
  "request_id": "req-example-18",
  "status": "partial",
  "operations": [
    {"id": "update-status", "status": "applied"},
    {"id": "edit-outcome", "status": "failed", "code": "TARGET_NOT_FOUND"},
    {"id": "create-follow-up", "status": "applied"}
  ],
  "git_publication": "pending"
}
~~~

**Exactly one local Git content commit** contains the successful status change and new follow-up document. The audit still records the failed edit. Operation identity and errors must not be inferred from the Git diff alone.

**Open:** request deduplication/idempotency, error categories that justify aborting an entire request, explicit dependencies between operations, and storage of failures when there is no Git content commit.

## 9. Error vocabulary and safety

Proposed error categories (names subject to interface review):

| Code | Meaning | Required safety consequence |
| --- | --- | --- |
| `PROJECT_NOT_FOUND` | Project slug cannot be resolved | No mutation |
| `INVALID_PATH` | Path escapes or is outside configured project scope | No mutation |
| `DOCUMENT_NOT_FOUND` | Requested existing document is absent | No mutation |
| `DOCUMENT_EXISTS` | Create target already exists | No overwrite |
| `INVALID_METADATA` | YAML cannot be parsed or changed safely | No affected-document mutation |
| `TARGET_NOT_FOUND` | Exact edit target absent | No affected-operation mutation |
| `AMBIGUOUS_MATCH` | Exact target occurs more than allowed | No affected-operation mutation |
| `REVISION_CONFLICT` | Revision or patch intent cannot safely be reconciled | No silent replacement |
| `PERMISSION_DENIED` | Operation outside authorized scope | No mutation |
| `LOCAL_PERSISTENCE_FAILED` | Git commit/state durability cannot be confirmed | No reported success; state may need recovery |
| `REMOTE_PUBLICATION_FAILED` | Central Git publication did not succeed | Local commit remains visible and pending; local success must not be rewritten as failure |
| `UNKNOWN_OUTCOME` | A requested action's final state cannot be determined safely | Expose uncertainty and allow inspection; never retry destructively without checks |

Each error should include project/path/operation identifiers where known and safe to disclose. The mapping to process exit codes, stderr, JSON output and later remote-tool errors remains to be designed.

Reject path traversal, unsafe absolute paths and unsupported symlink/path escapes where applicable. Do not overwrite documents on create or silently rewrite legacy markup.

## 10. Audit and synchronization visibility

The operation layer must be able to describe:

1. **Intent and identity:** original request, submitted operations, timestamp, caller-provided versus authenticated actor if known.
2. **Application:** per-operation outcomes, affected paths, local revisions and recorded errors.
3. **Git publication:** whether corresponding history is pending, published, blocked or uncertain.
4. **Satellites (if configured):** their independent projection status, **without requiring a connector to run locally**.

Audit representation, retention, where failed requests are recorded, and whether the audit is replicated into the central Git hub remain explicit open issues. Git history provides versioning but cannot substitute for these application-level records.

## 11. Candidate acceptance examples

| ID | Given / when | Observable expectation | Source requirement |
| --- | --- | --- | --- |
| OP-AC-01 | A known document is read offline | Content and its opaque local revision are returned | FR-03, FR-04 |
| OP-AC-02 | A create targets an existing path | Explicit `DOCUMENT_EXISTS`; existing bytes unchanged | FR-04, FR-13 |
| OP-AC-03 | An exact edit target appears once | Edit produces a durable new local revision | FR-06, FR-09 |
| OP-AC-04 | An exact edit target occurs twice | Explicit ambiguity; no arbitrary occurrence chosen | FR-06, FR-14 |
| OP-AC-05 | Several edits are submitted in one `document.edit` and one fails | No partially written document; failed edit is identified | FR-07, NFR-01 |
| OP-AC-06 | A document has arbitrary, unknown YAML keys | An unrelated metadata update preserves them and body text | FR-05, FR-16 |
| OP-AC-07 | An existing legacy document is read | No implicit migration of frontmatter or wikilinks | FR-13 |
| OP-AC-08 | A composite's middle operation fails while its last operation can run | Later operation still executes; results identify the applied and failed members | FR-08, FR-17 |
| OP-AC-09 | A local edit succeeds offline | The result says locally applied with Git publication pending | FR-09, FR-11 |
| OP-AC-10 | A remote push fails after local success | The durable local change is still discoverable and publication failure is visible | FR-11, FR-12 |
| OP-AC-11 | A stale-base body edit still has a uniquely and safely applicable exact target | May apply without forcing a full reread; no fuzzy or silent overwrite | FR-06, FR-12 |
| OP-AC-12 | Metadata YAML is malformed | The metadata operation fails without silently normalizing/replacing it | FR-05, FR-14 |
| OP-AC-13 | A composite's first operation changes a file and a later one uses it | Later operation sees staged successful state; one final Git content commit records resulting changes | FR-08 |
| OP-AC-14 | An agent attempts to modify an out-of-project path | Operation denied; no arbitrary filesystem write | FR-15 |
| OP-AC-15 | A request includes two successful document changes and one failed operation | One local Git content commit includes both changes; every result is audited | FR-08, FR-09, FR-10 |
| OP-AC-16 | A request changes nothing or its only operation fails | No content commit is necessary; audit reports the outcome | FR-08, FR-10 |
| OP-AC-17 | An edit target appears only inside frontmatter | `document.edit` fails without modifying frontmatter | FR-05, FR-06 |

These are **specification-level acceptance scenarios**, not implemented test results.

## 12. Open design questions and review gates

### Decisions recorded from the project owner

1. `document.edit` changes the Markdown **body only**.
2. An obsolete read revision may still yield a safe exact-target body edit without forcing a new read.
3. Composite execution **continues** after member errors, reporting each outcome.
4. One Git **content commit per request** records all successful changes.

### Review next

1. **Nested YAML paths:** [research note](../research/2026-10-23-yaml-paths-and-partial-reads.md) recommends JSON Pointer as a candidate canonical single-target locator. Dot notation, typed segments, and JSONPath are alternatives; YAML edge cases remain open.
2. **Partial reads:** [research note](../research/2026-10-23-yaml-paths-and-partial-reads.md) documents line-based `offset`/`limit` practice in agent tools. Coordinate frame, truncation, pinning to a revision, indexing and response caps remain unselected.
3. **Stale edits:** specify what "safe to apply" means beyond exact uniqueness, and optional strict revision preconditions.
4. **Audit:** define durable capture of failed/no-op requests separately from Git content commits.
5. **Composite implementation:** fatal request-level errors, interrupted commit handling, idempotent retries, and final outcome reporting.

### Explicitly defer

- Git library, Git internals, push cadence, hosting, and branch/ref conventions.
- Connector execution/deployment location, satellite direction and sync infrastructure.
- Contextual patches, hashline anchors, three-way merge/rebase.
- Search/query engines, mandatory validation, migration of legacy Markdown, and later MCP/public CLI syntax.

## 13. Status and next step

This document is a proposal added to an **unmerged GitHub draft PR**, not an approved operation specification. It narrows the interface discussion without locking implementation choices.

**Suggested next task:** Review nested YAML path options and agent-oriented partial-read behavior before a small Git-backed vertical implementation plan. No code or tests are claimed by this document.
