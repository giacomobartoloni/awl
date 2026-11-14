<div align="center">

# awl

### Markdown is the format. Git is the history. `awl` is the interface.

**A local-first CLI for project knowledge, built for humans and coding agents.**

[![Go checks](https://github.com/giacomobartoloni/awl/actions/workflows/go.yml/badge.svg)](https://github.com/giacomobartoloni/awl/actions/workflows/go.yml)
![Go 1.23+](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)
![Experimental](https://img.shields.io/badge/status-experimental-ffb347)
[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)

[Install](#installation) · [Get started](#quick-start) · [Commands](#the-cli-today) · [Design](#how-it-works) · [Status](#project-status)

</div>

---

## Your work deserves more than a chat history.

Coding agents can write code. Keeping a useful, durable record of *what* changed, *why* it changed, and *what happens next* is harder—especially across sessions, repositories, and worktrees.

**`awl` (AgentWorklog CLI)** is an experimental command-line tool for managing project-associated Markdown documents through a predictable interface. It keeps content in a managed local Git store, supports YAML frontmatter, and records request outcomes without requiring a particular coding-agent harness or a background service.

It is **not** a replacement format for Markdown, and it is **not** tied to a fixed set of document types. Notes, specs, plans, decisions, and your own conventions can all remain plain text.

## The idea, in four lines

- **Plain Markdown, always.** Documents stay readable outside `awl`; YAML frontmatter remains part of the file.
- **One project context.** Work from a source Git repository or linked worktree; `awl` resolves the associated managed document store.
- **Safer, explicit edits.** Create without overwriting, replace a unique body match, and operate on supported YAML metadata with targeted commands.
- **Built for automation.** Structured JSON output, request IDs, auditable Git history, and visible failures help agents act deliberately rather than guess.

## Installation

> [!IMPORTANT]
> `awl` is **experimental**. No public GitHub Release has been published yet. Prebuilt archive installation will be available after the first tagged release; until then, build from source or use Go's install command. Use disposable projects and noncritical documents when evaluating it.

**Prerequisite:** Install [Git](https://git-scm.com/downloads) and make sure `git --version` works. The `awl` CLI invokes an external Git executable; precompiled binaries do not require the Go toolchain.

### Prebuilt archives (after the first release)

Download the matching archive and `checksums.txt` from the **same** [GitHub Release](https://github.com/giacomobartoloni/awl/releases):

| Platform | Archive suffix |
| --- | --- |
| macOS Apple Silicon | `darwin_arm64.tar.gz` |
| macOS Intel | `darwin_amd64.tar.gz` |
| Linux x86-64 | `linux_amd64.tar.gz` |
| Linux ARM64 | `linux_arm64.tar.gz` |
| Windows x86-64 | `windows_amd64.zip` |

Archive names follow `awl_VERSION_OS_ARCH.tar.gz` (or `.zip` on Windows), where `VERSION` omits the leading `v` from the Git tag. **Replace `ARCHIVE`** in the commands below with your downloaded archive's base filename (without the `.tar.gz` or `.zip` extension).

**macOS / Linux:**

```sh
# Compare this SHA-256 hash with ARCHIVE's entry in checksums.txt:
shasum -a 256 ARCHIVE.tar.gz  # macOS
# sha256sum ARCHIVE.tar.gz     # Linux

tar -xzf ARCHIVE.tar.gz
mkdir -p "$HOME/.local/bin"
install -m 755 awl "$HOME/.local/bin/awl"
export PATH="$HOME/.local/bin:$PATH"
awl --version
```

**Windows (PowerShell):**

```powershell
# Compare this SHA-256 hash with the ZIP's entry in checksums.txt:
Get-FileHash .\ARCHIVE.zip -Algorithm SHA256

New-Item -ItemType Directory -Force "$HOME\bin" | Out-Null
Expand-Archive -LiteralPath .\ARCHIVE.zip -DestinationPath "$HOME\bin"
$env:Path = "$HOME\bin;$env:Path"
awl --version
```

These `PATH` changes apply only to the current terminal session; configure your shell or user `PATH` for subsequent sessions. Verify `awl --help` and `git --version` after installation. Release archives are not yet signed or notarized on macOS; follow the operating system's security policies rather than bypassing them.

### Install with Go

If Go 1.23+ is installed:

```sh
go install github.com/giacomobartoloni/awl/cmd/awl@latest
```

Add Go's binary directory (typically `$(go env GOPATH)/bin`) to `PATH`. Source-installed builds may show Go module or development version metadata in `awl --version`. To use a specific published tag when releases exist, replace `@latest` with `@vX.Y.Z`.

### Build from a checkout

```sh
git clone https://github.com/giacomobartoloni/awl.git
cd awl
mkdir -p ./bin
go build -o ./bin/awl ./cmd/awl
export PATH="$(pwd)/bin:$PATH"
awl --help
```

See [detailed installation notes](docs/installation.md) for prerequisites, checksum checks, supported build targets and maintainer release steps. Homebrew, Scoop, WinGet and Linux package repositories are **not published** by this PR.

## Quick start

> [!IMPORTANT]
> `awl` is still experimental. Use a test project and noncritical documents while exploring it.

Install `awl` using one of the methods above and make sure `git --version` succeeds. Then, from an **existing source Git repository or linked worktree**, with Git author identity configured:

```sh
cd /path/to/your/source-repository

# Explicitly initialize a separate managed Markdown store.
awl init --slug my-project

# Add a Markdown document (stdin is the default input).
printf '# First note\n\nSomething worth remembering.\n' | \
  awl create Notes/first-note.md --request-id first-note-001

# Explore the managed documents.
awl ls
awl read Notes/first-note.md
```

The source repository supplies **project context**; the documents themselves live in a **separate managed local Git repository**, not inside your source working tree.

## The CLI today

| What you want to do | Command |
| --- | --- |
| Explore usage | `awl --help`, `awl help metadata`, `awl --version` |
| Initialize project storage | `awl init [--slug NAME]` |
| List Markdown documents | `awl ls [DIRECTORY]` |
| Read a document | `awl read Notes/example.md` |
| Create without overwriting | `awl create Notes/example.md --input draft.md` |
| Replace a unique body match | `awl edit Notes/example.md --input edit.json` |
| Read or change YAML frontmatter | `awl metadata get ...` (also `set`, `insert`, `remove`) |
| Apply ordered operations | `awl apply --input request.json` |
| Configure a remote URL | `awl remote add URL` |

Most operational commands support `--json`. Mutation commands support request IDs for safely recognizing an identical retry; an `apply` request can **partially succeed**, so always inspect its per-operation results. `awl remote add` only saves a remote URL—it **does not synchronize** anything.

For exact syntax, payload formats, return codes, and safety rules, see the [CLI command reference](skills/awl-cli/references/commands.md) and [metadata reference](skills/awl-cli/references/metadata.md).

## How it works

```text
Your source Git repository / linked worktree
                │
                │ project context
                ▼
           awl CLI
                │
                ▼
     Managed local Git store
       ├── Markdown documents
       ├── YAML frontmatter
       └── request-level audit
```

The CLI is designed to keep document operations explicit and project-scoped. The current local implementation uses Git internally for document state and audit records. It does **not** require Obsidian, Google Drive, a daemon, or an agent-specific SDK.

The long-term direction includes explicit remote Git synchronization and document retrieval, but those designs must not be confused with what the current binary does.

## Project status

**Early-stage / experimental.** The local Go command set, basic document operations, guarded YAML metadata edits, and the [reusable agent skill](skills/awl-cli/SKILL.md) are in the repository. The interface and persistence guarantees may change.

**Not implemented yet:** `awl sync`, remote fetch/reconcile/push, and integrated document search/indexing. Hardening for crash/power-loss recovery, concurrency edge cases, and platform support remains open. A cross-platform GitHub Releases packaging workflow is under review; no public release has been published. No production durability guarantee is claimed.

If you are evaluating `awl`, use disposable data first and keep independent backups of anything important.

## Dig deeper

- [Core requirements](docs/requirements/core-requirements.md) — goals, constraints, and proposed product behavior.
- [Document operation contract](docs/requirements/document-operation-contract.md) — detailed operation semantics and safety boundaries.
- [Architecture decision: modular monolith](docs/adr/0001-modular-monolith.md) — why a focused CLI comes first.
- [Local-first storage evaluation](docs/architecture/2026-10-23-local-first-storage-evaluation.md) — evidence behind the current approach.
- [Synchronization design proposal](docs/architecture/2026-11-06-sync-backends-and-transports-design-proposal.md) — future directions, **not** shipped features.
- [Agent skill](skills/awl-cli/SKILL.md) — usage guidance for compatible coding-agent workflows (installation is harness-specific).

## Contributing

`awl` is open source and still taking shape. Bug reports, reproducible edge cases, documentation improvements, and focused proposals are useful—especially around data integrity, Markdown/YAML compatibility, and agent-friendly CLI behavior.

Start with a [GitHub issue](https://github.com/giacomobartoloni/awl/issues) or a small pull request. For behavior changes, please distinguish implemented behavior from proposed design and include verification where possible.

## License

Licensed under the **GNU Affero General Public License v3.0**. See [LICENSE](LICENSE).

---

<div align="center">

**Keep the knowledge. Keep the history. Keep it in Markdown.**

</div>