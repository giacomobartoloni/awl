# awl CLI command reference (experimental)

This reference describes the Go implementation in the stacked PR #9 / PR #7. Exact flags, JSON envelopes and exit codes are not yet an approved stable public interface.

## Context and project setup

- Run from an existing **source Git repository or linked worktree**; it supplies the project context. The Markdown documents live in a separate managed **bare Git repository**, not in the source Git working tree.
- For an initialized project, inspect with `awl ls --json`. If initialization is missing, stop and request authorization; do not auto-initialize, repair or rebind it.
- `awl init` is an **explicit bootstrap**, only when authorized. An omitted `--slug` infers a slug from the source repository identity. Optional `--repo-dir` selects an exact **new, absent absolute directory** for a local managed repository; its existing parent must be safe and must already exist. It must not point inside the source worktree or Git common dir.
- The source project configuration uses `.awl/project.yaml`; a canonical local binding is maintained under the source repository's Git common dir. Do not edit either file by hand.
- Mutations need Git author name/email (available via Git configuration). Reads and remote configuration do not require author identity.

Examples, from an authorized source Git worktree:

```sh
awl ls --json
# Only for authorized first-time initialization:
awl init --slug demo --repo-dir /absolute/new-store/demo.git --json
# Configures a URL only; performs no fetch/push/network sync:
awl remote add https://example.invalid/managed.git --json
```

The optional leading `awl --project demo ...` is an identity check against the **currently resolved** source project, not out-of-repository project discovery.

## Read and list

```sh
awl ls --json
awl ls Notes --json
awl read Notes/example.md
awl read Notes/example.md --json
```

- `ls` returns sorted project-relative Markdown paths from the committed managed tree. JSON adds `project`, `revision` and `paths`.
- `read` returns raw Markdown bytes in plain mode, or `content` plus managed `revision` and `project` in JSON mode. JSON mode requires UTF-8.
- Separate reads may see different revisions if another writer publishes between commands; do not assume a cross-command snapshot transaction.
- Paths are **project-relative Markdown paths**, e.g. `Notes/example.md`, not absolute source-file paths. Invalid/traversal/reserved paths are rejected.

## Create a new document

```sh
awl create Notes/example.md --input /absolute/path/to/draft.md \
  --request-id create-notes-example-001 --json
```

- `--input -` (default) reads stdin; otherwise use an existing input file. The input must be UTF-8 Markdown.
- A document that already exists is **not overwritten**. A duplicate create may return `DOCUMENT_EXISTS` and an audit-only commit.
- A stable request ID permits replay **only with the identical operation payload**. A different payload with the same ID results in a request-ID conflict.
- Without an explicit request ID, the CLI generates one; do not assume an automatically generated ID is suitable for a retry after an ambiguous outcome.

## Edit existing Markdown body text

```sh
printf '%s' '{"old":"Draft paragraph","new":"Reviewed paragraph"}' |
  awl edit Notes/example.md --input - \
    --request-id edit-notes-example-001 --json
```

- Input is a JSON object with nonempty `old` and string `new`.
- `old` must match **exactly once in the body**, not in frontmatter. Missing or ambiguous matches fail; unrelated bytes are preserved in supported cases.
- Do not use `edit` for frontmatter. Use `metadata` as described in [metadata](metadata.md).

## Ordered composite requests

Use `awl apply` only when an ordered multi-operation request is needed. The JSON input has a `request_id` and an `operations` array. Supported members are `create`, `edit` and `metadata`.

```sh
cat > /tmp/awl-request.json <<'JSON'
{
  "request_id": "composite-demo-001",
  "operations": [
    {"id": "first", "op": "create", "path": "Notes/first.md", "content": "# First\n"},
    {"id": "second", "op": "create", "path": "Notes/second.md", "content": "# Second\n"}
  ]
}
JSON
awl apply --input /tmp/awl-request.json --json
```

- Each member runs in order against the staged document state.
- Ordinary member failures do **not** automatically roll back successful sibling members: a composite can return `status: partial` and exit 3. Do not call a partial result fully successful.
- One `metadata` member can contain multiple edits; that member's document update is staged all-or-nothing on failure. This is **not** an all-or-nothing promise for an entire multi-member `apply`.
- Requests are audited in local Git; outcomes and request IDs are observable through the JSON response.

## Responses, retries, and failures

Observed exit codes in the experiment:

| Exit | Meaning |
| --- | --- |
| 0 | Accepted success, including a legitimate `no_op` or replay |
| 2 | Rejected command or failed request |
| 3 | Accepted composite with partial member failure |
| 4 | JSON response encoding failure |

Mutation JSON generally contains `status`, `project`, `request_id`, `commit`, `document_tree`, and ordered `members`. A replay also has `replayed: true`. Validation failures can return `status: error` and `error: {code, message}`; ordinary member failures can return `status: failed` with errors inside `members`.

- Check **both exit code and member statuses**; do not equate `commit` with a changed document tree.
- For ambiguous outcomes (e.g. `PUBLICATION_OUTCOME_UNKNOWN`), inspect state and retry the **same request ID and bytes**, not a new request.
- For `CONCURRENT_REQUEST_CONFLICT`, reread before deciding on an action. Never silently overwrite the winner.
- `awl --version`, `awl sync` and broad project discovery are **not available** in this experimental version.

See [metadata](metadata.md) for frontmatter operations. No command here synchronizes a remote or writes Markdown into the source Git working tree.
