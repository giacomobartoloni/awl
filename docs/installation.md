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
- After successful publication, the reusable Homebrew workflow verifies the latest stable published release and proposes a tap pull request when the formula is behind. It never merges that PR or republishes binaries.
- Signing, notarization, Scoop, WinGet, Debian and RPM packaging remain outside this workflow.

## Homebrew (macOS and Linux)

Install or upgrade from the reviewed [Homebrew tap](https://github.com/giacomobartoloni/homebrew-tap):

```sh
brew install giacomobartoloni/tap/awl
brew update
brew upgrade giacomobartoloni/tap/awl
awl --version
```

The tap pins four prebuilt archives and requires external Git, not Go. The tap validation job runs a native macOS audit, installation and test; this does not certify every OS or CPU target.

### Maintaining the tap automation

The release workflow calls `.github/workflows/update-homebrew-tap.yml` after GoReleaser succeeds. Do not rely on a separate release-published event from the repository's `GITHUB_TOKEN`, which normally suppresses subsequent workflow runs.

Configure the following in the **awl** source repository:

- Actions variable `AWL_TAP_APP_CLIENT_ID`: client ID of a GitHub App installed on `giacomobartoloni/homebrew-tap`.
- Actions secret `AWL_TAP_APP_PRIVATE_KEY`: that App's private key. Give the App only Contents and Pull requests write permissions on the tap (Metadata read is implicit). The same narrowly scoped App used for MathBoard may be reused if the owner approves. Do not grant the App source-repository write access.

To retry without publishing anything, manually run **Propose Homebrew tap update** on `main`. The workflow selects the highest canonical, non-draft, non-prerelease published version; validates every macOS/Linux archive against `checksums.txt` and available GitHub asset digests; refuses downgrades and same-version checksum changes; and opens or refreshes a PR on `automation/update-awl` limited to `Formula/awl.rb` and the README version. It obtains an App token only when a new version is needed. A run against an already-pinned version still verifies its archives and finishes without writing.

Review the generated PR, its release and all four checksums, and wait for tap CI before merging. Delete the automation branch after merging (the tap's automatic head-branch deletion setting can do this). If an orphan branch remains or an unexpected PR occupies the branch, the updater fails closed; inspect it manually rather than forcing an overwrite. No automatic merge or new release publication is performed.

The PR-only test job uses synthetic fixtures and no credentials. It does not substitute for a real manual run or the tap's native validation.
