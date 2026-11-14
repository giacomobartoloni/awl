# Installing awl

`awl` is an experimental command-line tool. It manages project-scoped Markdown documents; it is not the AgentWorklog Markdown worklog format itself.

## Prerequisites

- An external **Git executable** on `PATH`. Verify it with `git --version`.
- A shell with the installed `awl` executable on `PATH`.
- **No Go runtime** is needed when using a precompiled release archive.

The release packaging currently targets:

| Operating system | CPU architecture | Archive |
| --- | --- | --- |
| macOS | arm64, amd64 | `.tar.gz` |
| Linux | arm64, amd64 | `.tar.gz` |
| Windows | amd64 | `.zip` |

This is a build matrix, **not a platform-support certification**. The release workflow smoke-tests startup on native macOS, Linux, and Windows runners; it does not certify all operations, storage durability, or every CPU architecture. Windows/arm64 is intentionally deferred until it can be validated.

## Precompiled releases

1. Open [GitHub Releases](https://github.com/giacomobartoloni/awl/releases) and select a published version (if available).
2. Download the archive matching your operating system and CPU architecture, along with `checksums.txt` from the **same release**.
3. Verify the archive's SHA-256 digest against `checksums.txt`, then extract it:
   - macOS/Linux: `shasum -a 256 ARCHIVE.tar.gz` (macOS) or `sha256sum ARCHIVE.tar.gz` (Linux); extract with `tar -xzf ARCHIVE.tar.gz`.
   - Windows PowerShell: `Get-FileHash ARCHIVE.zip -Algorithm SHA256`; extract using your archive utility.
4. Move `awl` (or `awl.exe` on Windows) to a user-controlled directory on `PATH`. Avoid overwriting a differently sourced executable without checking which one you invoke.
5. Run `awl --version`, `awl --help`, and `git --version`.

Archives use the naming scheme `awl_VERSION_OS_ARCH.tar.gz` or `awl_VERSION_windows_amd64.zip`. `VERSION` does not include the leading `v` from the Git tag.

**macOS security:** Release binaries are not yet signed or notarized as part of this workflow. Follow the operating system's security prompts and your organization's software policies; do not bypass platform protections.

## Install using Go

If Go 1.23 or later is already installed:

```sh
go install github.com/giacomobartoloni/awl/cmd/awl@latest
```

Make sure the Go binary installation directory (usually `$(go env GOPATH)/bin`) is on `PATH`. To pin a specific published release, replace `@latest` with `@vX.Y.Z`.

Go source installations do **not** use GoReleaser's version injection; `awl --version` may therefore report Go build metadata or a development identifier.

## Release process (maintainers)

- The `Release` GitHub Actions workflow tests native CLI startup on macOS, Linux, and Windows.
- Pull requests touching release configuration or Go source produce a **local GoReleaser snapshot**, without publishing artifacts.
- Pushing a maintainer-created version tag beginning with `v` (for example `v0.1.0`) triggers the native smoke checks and then GoReleaser to publish archives and `checksums.txt` to GitHub Releases.
- The existing `Go checks` workflow continues to run its deeper Linux test suite on pull requests and pushes to `main`. A tag alone does not re-run that workflow.
- No public release, package-manager publishing, signing, notarization, auto-update, or installation script is created by this change.

Homebrew, Scoop, WinGet, Debian, and RPM packaging should be evaluated only after the first archive-based release has been validated. Their repositories, credentials, package manifests, and support claims are deliberately out of scope for the initial release foundation.
