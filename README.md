<div align="center">

# awl

### Keep a clear log of the work done by your team's coding agents.

**A local-first CLI for recording agent work across projects, sessions, and team handoffs.**

[![Go checks](https://github.com/giacomobartoloni/awl/actions/workflows/go.yml/badge.svg)](https://github.com/giacomobartoloni/awl/actions/workflows/go.yml)
![Go 1.23+](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)
![Experimental](https://img.shields.io/badge/status-experimental-ffb347)
[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)

[Install](#installation) · [Get started](#quick-start) · [Commands](#the-cli-today) · [Design](#how-it-works) · [Status](#project-status)

</div>

---

## Keep your team's agent work visible.

When several coding agents contribute to a project, it's easy to lose track of **what each agent worked on, why changes were made, what decisions were taken, and what still needs doing**. That context matters for reviews, handoffs, and future sessions.

**`awl` (AgentWorklog CLI)** helps a team maintain a clear, durable **work log of coding-agent activity**. It provides explicit commands to create, read, and update project-scoped Markdown records: activities, notes, plans, decisions, or other documents your team uses.

The records remain plain Markdown with YAML frontmatter, readable by people and agents alike. `awl` works locally and is independent of any particular coding-agent harness. **Automatic session capture and background synchronization are not implemented**; agents record their work through explicit CLI operations, and a first explicit fast-forward-only Git sync is available.

## The idea, in four lines

- **One work log for each project.** Keep activities, decisions, plans, and follow-ups organized so teammates can understand an agent's work.
- **Plain Markdown, always.** Records stay readable outside `awl`, with YAML frontmatter for useful metadata.
- **Safer, explicit edits.** Create without overwriting, replace a unique body match, and update supported metadata with targeted commands.
- **Agent-friendly automation.** Structured JSON output, request IDs, audit information, and visible failures help agents work predictably.

## Installation

> [!IMPORTANT]
> `awl` is **experimental**. No public GitHub Release has been published yet. Prebuilt archive installation will be available after the first tagged release; until then, build from source or use Go's install command. Use disposable projects and noncritical documents when evaluating it.

**Before installing:** Review the [full prerequisites](docs/installation.md#prerequisites). Precompiled binaries do not require the Go toolchain.

### Prebuilt archives (after the first release)

Download the matching archive and `checksums.txt` from the **same** [GitHub Release](https://github.com/giacomobartoloni/awl/releases):

| Platform | Archive suffix |
| --- | --- |
| macOS Apple Silicon | `darwin_arm64.tar.gz` |
| macOS Intel | `darwin_amd64.tar.gz` |
| Linux x86-64 | `linux_amd64.tar.gz` |
| Linux ARM64 | `linux_arm64.tar.gz` |
| Windows x86-64 | `windows_amd64.zip` |

Archive names follow `awl_VERSION_OS_ARCH.tar.gz` (or `.zip` on Windows), where `VERSION` omits the leading `v` from the release version. **Replace `ARCHIVE`** in the commands below with your downloaded archive's base filename (without the `.tar.gz` or `.zip` extension).

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

These `PATH` changes apply only to the current terminal session; configure your shell or user `PATH` for subsequent sessions. Verify `awl --help` after installation and check the [runtime prerequisites](docs/installation.md#prerequisites). Release archives are not yet signed or notarized on macOS; follow the operating system's security policies rather than bypassing them.

### Install with Go

If Go 1.23+ is installed:

```sh
go install github.com/giacomobartoloni/awl/cmd/awl@latest
```

Add Go's binary directory (typically `$(go env GOPATH)/bin`) to `PATH`. Source-installed builds may show Go module or development version metadata in `awl --version`. To use a specific published tag when releases exist, replace `@latest` with `@vX.Y.Z`.

### Build from a checkout

Download the source from the [project repository](https://github.com/giacomobartoloni/awl), open a terminal in the extracted project root, and run:

```sh
mkdir -p ./bin
go build -o ./bin/awl ./cmd/awl
export PATH="$(pwd)/bin:$PATH"
awl --help
```

See [detailed installation notes](docs/installation.md) for prerequisites, checksum checks, supported build targets and maintainer release steps. Homebrew, Scoop, WinGet and Linux package repositories are **not published** by this PR.

## Quick start

> [!IMPORTANT]
> `awl` is still experimental. Use a test project and noncritical documents while exploring it.

Install `awl` using one of the methods above, and follow the [project setup prerequisites](docs/installation.md#prerequisites). Then, from an **existing project repository** with author identity configured:

```sh
cd /path/to/your/project

# Explicitly initialize a separate managed Markdown store.
awl init --slug my-project

# Add a Markdown document (stdin is the default input).
printf '# First note\n\nSomething worth remembering.\n' | \
  awl create Notes/first-note.md --request-id first-note-001

# Explore the managed documents.
awl ls
awl read Notes/first-note.md
```

The project directory supplies **context**; the activity records live in a **separate managed local document store**, not among the project's source files.

`awl init` leaves the source repository's `.gitignore` untouched by default. The shared `.awl/project.yaml` is suitable for version control; the personal binding lives in Git's common directory. If you choose to expose the personal binding via the optional `.awl/local.yaml` symlink, that path must be Git-ignored. Run `awl init --update-gitignore` to explicitly add `.awl/local.yaml` to the source `.gitignore` **only if effective Git ignore rules do not already exclude it**. This never ignores the entire `.awl/` directory, creates no symlink and does not stage or commit files.

Optional setup for a shared team Git repository (configuration only; no fetch or push):

```sh
awl remote add git@example.com:team/worklogs.git --branch worklogs/my-project
```

Alternatively use `awl init --slug my-project --remote URL --remote-branch worklogs/my-project` during first-time initialization. Omitting the branch preserves the legacy `awl` default.

## The CLI today

| What you want to do | Command |
| --- | --- |
| Explore usage | `awl --help`, `awl help metadata`, `awl --version` |
| Initialize project storage | `awl init [--slug NAME] [--update-gitignore]` |
| List Markdown documents | `awl ls [DIRECTORY]` |
| Read a document | `awl read Notes/example.md` |
| Create without overwriting | `awl create Notes/example.md --input draft.md` |
| Replace a unique body match | `awl edit Notes/example.md --input edit.json` |
| Read or change YAML frontmatter | `awl metadata get ...` (also `set`, `insert`, `remove`) |
| Apply ordered operations | `awl apply --input request.json` |
| Configure a remote Git target | `awl remote add URL [--branch BRANCH]` |
| Change the remote branch before publication | `awl remote set-branch BRANCH [--json]` |
| Explicit safe Git synchronization | `awl sync [--json]` |

Most operational commands support `--json`. Mutation commands support request IDs for safely recognizing an identical retry; an `apply` request can **partially succeed**, so always inspect its per-operation results. `awl remote add` saves a Git remote URL and optional branch name—it **does not synchronize** anything. The default remote branch is `awl`; a team can choose `worklogs/api` or `worklogs/web` in the same remote Git repository while keeping the histories independent. A project always has one authoritative remote URL-and-branch target. To change only the branch **before its history has been verified by sync**, run `awl remote set-branch worklog/my-project --json`. This changes both project bindings but neither local Git history nor any remote ref, and it performs no network operation. Repeating the same request is a no-op. Once a remote history has been verified, branch changes require an explicit migration not implemented in this increment. Replacing the URL is also unsupported. No per-project access isolation is provided by Git branches; repository access typically covers all branches.

`awl sync` now supports first non-forcing publication to an absent branch, fast-forward publication and import, and adoption from a recognized pristine local bootstrap. It verifies the selected remote ref and preserves acknowledged local commits on errors. **Divergent histories, unrelated meaningful histories, merge commits, history rewrites and `--allow-unrelated-histories` are not supported in this first increment.** A remote target selected on first use is trusted as a user-configured destination; its branch name does not prove project identity.

For exact syntax, payload formats, return codes, and safety rules, see the [CLI command reference](skills/awl-cli/references/commands.md) and [metadata reference](skills/awl-cli/references/metadata.md).

## How it works

```text
Your team's project / agent workspace
                │
                │ record and review agent work
                ▼
           awl CLI
                │
                ▼
     Local managed work log
       ├── Agent activities and handoffs
       ├── Notes, plans, and decisions
       └── Markdown metadata and audit records
```

The CLI keeps worklog operations explicit and project-scoped. It does **not** require Obsidian, Google Drive, a daemon, or an agent-specific SDK. Teams can adopt their own Markdown conventions for describing an agent's objective, progress, outcome, and next steps.

Fully automatic/shared reconciliation and document retrieval remain future directions; the current CLI supports only explicit, fast-forward Git exchange.

## Project status

**Early-stage / experimental.** The local Go command set, basic document operations, guarded YAML metadata edits, and the [reusable agent skill](skills/awl-cli/SKILL.md) are in the repository. The interface and persistence guarantees may change.

**Not implemented yet:** conservative merge for divergent or unrelated meaningful histories, automatic background synchronization, and integrated document search/indexing. Hardening for crash/power-loss recovery, concurrency edge cases, and platform support remains open. A cross-platform GitHub Releases packaging workflow is under review; no public release has been published. No production durability guarantee is claimed.

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

**Keep the agent work visible. Keep the context. Keep it in Markdown.**

</div>