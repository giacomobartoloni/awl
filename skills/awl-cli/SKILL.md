---
name: awl-cli
description: Use when operating the experimental awl CLI to initialize an explicitly authorized Git-backed project, inspect managed Markdown documents, create or edit content, apply requests, or read and change YAML frontmatter metadata. Covers safe invocation, request replay, verification, and current command limits. Does not provide Obsidian integration or workflow-specific rules.
---

# awl CLI

Use this skill to operate the **experimental** `awl` executable. It manages Markdown documents in a **separate bare Git repository**, using the current source Git worktree to resolve project context. It does not write documents into the source working tree.

This skill documents behavior on the stacked experimental metadata branch (PR #9 on PR #7), not a stable release contract.

## Before running commands

1. Confirm `command -v awl` succeeds in the agent's execution environment. This build has no `awl --version` command; do not use that flag as a health check.
2. Run commands from the relevant source Git repository or its linked worktree. A leading `--project <slug>` can check the current project identity; it cannot discover an unrelated project.
3. For an existing project, run `awl ls --json` and check the returned `project` and `revision`. Do **not** run `awl init` automatically when the project is not initialized.
4. Before writing, establish that the user authorized this project and operation; Git author name/email must be available for audited mutations.
5. Treat user-owned Markdown and managed history as data to preserve. Never overwrite, migrate, normalize, delete, or rewrite content as an implicit recovery step.

## Operating workflow

1. Read the relevant document and/or metadata before changing it.
2. Select the narrowest implemented command. Read [commands](references/commands.md) for initialization, listing, reads, body edits and composites; read [metadata](references/metadata.md) for frontmatter.
3. Prefer `--json` for machine-readable responses. For a mutation, provide a **stable, unique `--request-id`** when retryability matters.
4. Check the **process exit code**, top-level `status`, per-operation `members` and any `error.code`; never infer success merely from the existence of a Git commit.
5. Read the document or metadata back and verify the intended change and preservation of unrelated content. Report the actual result, commit and request ID where available.
6. On conflict or ambiguous publication, inspect the current state first. Reuse the **same request ID and identical payload** for a retry of the same logical request. Never use a fresh ID to bypass a conflict.

## Boundaries

- `awl edit` changes a **unique exact Markdown body** target, not YAML frontmatter. Use `awl metadata` for supported YAML changes.
- `create` rejects an existing document. Metadata `set` requires existing fields. No unrestricted YAML rewrite or automatic data migration is available.
- A failed or no-op request may publish an **audit-only commit** with an unchanged `document_tree`. `apply` may return partial success.
- `remote add` records configuration only; it does **not** fetch, push or synchronize documents. There is no automatic filesystem or vault sync.
- The implementation is experimental. Power-loss durability, broad filesystem and OS portability, arbitrary YAML layouts, and safe stale-position sequence insertion are **not** production guarantees.
- Do not introduce personal organization rules, agent identity mappings, Obsidian, qmd, or any specific worklog directory convention into this reusable skill. Those belong in the user's separate instructions.

## Reference files

- [Command reference](references/commands.md): `init`, `remote add`, `ls`, `read`, `create`, `edit`, `apply`; responses and retries.
- [Metadata reference](references/metadata.md): `get`, `set`, `insert`, `remove`; RFC 6901 pointers, typed values, preservation and unsupported YAML.

Source of current experimental behavior: `cmd/awl`, `internal/awl`, and `docs/experiments/` in this repository. Recheck those files when working against a different CLI revision.
