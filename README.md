# awl — AgentWorklog CLI

`awl` is an experimental, local-first Go CLI for managing project-scoped Markdown documents and YAML frontmatter. It is distinct from the existing AgentWorklog Markdown worklog conventions.

## Install

- **Prebuilt binaries (when tagged releases are published):** [GitHub Releases](https://github.com/giacomobartoloni/awl/releases) for macOS, Linux, and Windows.
- **From Go source:** `go install github.com/giacomobartoloni/awl/cmd/awl@latest` (requires Go 1.23 or later).

The CLI invokes an **external `git` executable**: Git must be installed and available on `PATH` for document operations. Go is not required when using a prebuilt binary.

See [installation instructions](docs/installation.md) for the platform matrix, checksum verification, version checks, and release limitations.

## Status

The CLI and release process are experimental. An available build does not imply that every supported operation has completed cross-platform or durability qualification. No public release is implied by these instructions.

Run `awl --help` to inspect the commands available in your installed version.
