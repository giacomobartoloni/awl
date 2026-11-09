---
name: awl-cli
description: Use when a task requires the awl command-line tool to inspect or change managed Markdown documents, edit YAML frontmatter, initialize a project, or handle write failures and request retries.
---

# awl CLI

Use `awl` to manage Markdown documents associated with the current project. The CLI resolves project context from the current Git repository or linked worktree. Managed documents are not written directly to the source working tree.

## Quick reference

| Task | Command |
| --- | --- |
| Inspect CLI | `awl --help`, `awl --version` |
| Command help | `awl help COMMAND` |
| Check project | `awl ls --json` |
| List/read documents | `awl ls [DIRECTORY]`, `awl read DOCUMENT.md` |
| Create document | `awl create DOCUMENT.md --input FILE --json` |
| Edit body | `awl edit DOCUMENT.md --input FILE --json` |
| Edit frontmatter | `awl metadata get/set/insert/remove ...` |
| Ordered requests | `awl apply --input FILE --json` |

## Safe operating procedure

1. Check `command -v awl`; inspect `awl --help` or `awl --version` without initializing a project. A development build identifier does not imply a published release.
2. Run commands from the intended source Git repository or linked worktree. Use `awl ls --json` to verify the resolved `project` and `revision`. Initialize or change project configuration only with explicit authorization.
3. Read the target document or YAML pointer before editing. Mutations require a Git author identity.
4. Use the narrowest supported command. `edit` replaces one unique exact match in the Markdown **body**; `metadata` handles supported frontmatter changes. `create` never overwrites an existing document.
5. Supply a stable `--request-id` for retriable writes. Retry an ambiguous operation only with its **original request ID and identical payload**; inspect current state first.
6. Check exit code, response `status`, `members` and errors; read back the document to verify changes. A failed or no-op request may still create an audit commit without changing document content.

## Limits and common mistakes

- A leading `--project SLUG` confirms the current project; it does not select an unrelated repository.
- `awl metadata set` changes existing supported fields; it does not add missing keys. Unsupported YAML layouts must not be silently rewritten.
- Ordered `apply` requests may partially succeed. Never report `partial` as full success.
- `awl remote add` only configures a URL. It does not fetch, push, or synchronize documents.
- The CLI has documented limitations around concurrent sequence insertions and power-loss durability. Avoid destructive recovery or automatic data migration.

## Detailed references

- [Commands and responses](references/commands.md): project setup, help/version, reads, writes, audit and retries.
- [YAML metadata](references/metadata.md): RFC 6901 pointers, typed JSON, atomic sets, supported sequences and rejection rules.

Prefer the installed CLI's `--help` output when its behavior differs from these reference examples.
