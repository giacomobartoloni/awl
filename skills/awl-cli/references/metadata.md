# awl YAML metadata reference

The `awl metadata` command group operates on YAML frontmatter in documents already stored in the managed Git repository. `awl edit` remains body-only.

Metadata mutation supports a **conservative existing-structure subset**, not arbitrary YAML rewriting. Changes are byte-local where supported; unsupported or unsafe layouts are rejected.

## Read a value

```sh
awl metadata get Notes/example.md /status --json
awl metadata get Notes/example.md /tags/0 --json
```

Pointers use **RFC 6901 JSON Pointer**:

- `/status` is a top-level key; `/build/version` targets a nested mapping.
- `/build.version` refers to a **literal key containing a dot**, not the nested path.
- Escape `~` as `~0` and `/` as `~1` within a key segment.
- `get` supports reading sequence members by numeric index. `/tags/-` is **not** a readable member.
- `--json` returns a typed `value` along with `project`, `revision`, `path` and `pointer`. Plain strings are printed without JSON quotes in text mode.

## Set an existing scalar value

Positional VALUE arguments are always **strings**:

```sh
awl metadata set Notes/example.md /status reviewed \
  --request-id set-note-status-001 --json
```

For typed JSON values, use explicit `--input` with a JSON value, not a positional string:

```sh
printf '%s' 'null' |
  awl metadata set Notes/example.md /completed_at --input - \
    --request-id reset-completed-at-001 --json

printf '%s' 'true' |
  awl metadata set Notes/example.md /verified --input - \
    --request-id set-verified-001 --json
```

The field **must already exist**; `set` does not upsert missing keys. Boolean/null/number types must be intentional. A positional `2026-11-09` is a string and the CLI can represent it as a quoted YAML string; do not silently assume implicit timestamp semantics.

### Set several fields in one atomic document operation

With no pointer argument, an explicitly supplied `--input` must contain a **nonempty JSON object mapping JSON Pointers directly to typed values**:

```sh
printf '%s' '{"/status":"completed","/completed_at":"2026-11-09"}' |
  awl metadata set Notes/example.md --input - \
    --request-id close-note-001 --json
```

Do **not** wrap the object in `{"set": {...}}`; there is no `metadata update` command. On a failed target such as `/missing`, none of the fields in **that one metadata operation** is written, though an audit-only commit may be published.

## Insert into an existing sequence

```sh
awl metadata insert Notes/example.md /tags/1 review \
  --request-id insert-tag-001 --json

awl metadata insert Notes/example.md /tags/- done \
  --request-id append-tag-001 --json
```

- `/tags/1` inserts **before** the existing element at index 1; index must be from 0 through the current length.
- `/tags/-` inserts at the end of an **existing** list; there is no separate `metadata append` command.
- Positional arguments are strings. An explicit `--input -` supplies a typed JSON element.
- Supported shapes include one-line simple scalar flow lists (including an empty `[]`) and safely isolated simple scalar block lists. Complex/multiline existing sequences are rejected.
- A sequence insertion is **never automatically rebased** after a concurrent Git compare-and-swap loss. More importantly, an index chosen from an already stale snapshot has no proven caller-intent precondition. Re-read the list before inserting and never assume this constitutes production-safe stale-position handling.

## Remove an existing scalar field

```sh
awl metadata remove Notes/example.md /obsolete \
  --request-id remove-obsolete-001 --json
```

Removal deletes an existing supported scalar mapping entry. It differs from assigning the JSON literal `null`. Do not remove required keys. Removing the last entry of a mapping and removing list elements are not supported in this version.

## Unsupported layouts and preservation

Existing nested **block mappings** with scalar targets are supported. The implementation rejects unsafe or unsupported forms rather than reserializing the full frontmatter:

- Missing keys, mapping/root replacement, arbitrary upsert, or positional list replacement/removal.
- Aliases, anchors, tags, duplicate mapping keys, non-string mapping keys, and malformed YAML.
- Block scalar rewrites, unsafe multiline scalars, flow-map mutation, and complex or multiline flow-list rewriting.
- Complex block-list original elements or insertion into a missing list.

Supported metadata changes preserve unrelated frontmatter bytes, comments, original line endings and Markdown body in the tested subset. Values written by the CLI may be quoted differently from the original scalar representation.

Before writing, read the current document or pointer. After writing, check exit code, JSON `status` and `members`, then read back the affected fields and the document. If the operation is rejected or concurrent state differs, preserve the existing document and report the limitation. Do not force a full-file replacement as a workaround.

Refer to [commands](commands.md) for Git project context, request-ID replay, audits and exit codes.
