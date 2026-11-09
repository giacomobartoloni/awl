package awl

import (
	"fmt"
	"io"
	"runtime/debug"
)

// Version can be set at build time via -ldflags "-X github.com/giacomobartoloni/awl/internal/awl.Version=vX.Y.Z".
// Unreleased source builds report "dev" and, when available, their VCS revision.
var Version = "dev"

const globalHelp = "Usage:\n  awl [--project SLUG] COMMAND [arguments] [options]\n  awl --help\n  awl --version\n\nCommands:\n  init        Initialize a local managed worklog\n  remote add  Configure the worklog Git remote (does not sync)\n  ls          List Markdown document paths\n  read        Read a Markdown document\n  create      Create a Markdown document without overwriting\n  edit        Replace an exact match in a document body\n  metadata    Get or modify YAML frontmatter using JSON Pointers\n  apply       Apply a JSON document-operation request\n\nGlobal options:\n  --project SLUG   Confirm the current Git project's slug (before COMMAND)\n  --help, -h       Show help\n  --version        Show build version\n\nCommand options:\n  --json           Structured responses/errors for operational commands\n\nUse \"awl help COMMAND\" or \"awl COMMAND --help\" for syntax and examples.\nPaths are relative to the managed worklog root, not the source worktree.\nExperimental local-only CLI; commands and output contracts may change.\nExit codes: 0 success, 2 error, 3 partial apply, 4 output failure.\n"

func infoForCommand(name string) (string, bool) {
	switch name {
	case "init":
		return "Usage: awl [--project SLUG] init [--slug SLUG] [--repo-dir ABSOLUTE_PATH] [--remote GIT_URL] [--json]\n\nInitialize a new local managed Git-backed worklog for the current source Git repository.\n  --slug       Explicit project slug (otherwise inferred from source Git)\n  --repo-dir   Exact path of a NEW managed bare Git repository\n  --remote     Configure a remote URL; no network synchronization occurs\n  --json       Machine-readable result or error\n\nExample:\n  awl init --slug demo --json\n", true
	case "remote":
		return "Usage: awl [--project SLUG] remote add GIT_URL [--json]\n\nConfigure the worklog's Git remote URL. Does not fetch or push.\n  --json       Machine-readable result or error\n\nExample:\n  awl remote add git@example.invalid:worklog.git --json\n", true
	case "ls":
		return "Usage: awl [--project SLUG] ls [DIRECTORY] [--json]\n\nList sorted project-relative Markdown paths from the managed worklog.\n  DIRECTORY    Optional project-relative directory filter\n  --json       Return status, project, revision, and paths\n\nExample:\n  awl ls Activities --json\n", true
	case "read":
		return "Usage: awl [--project SLUG] read DOCUMENT.md [--json]\n\nRead one project-relative Markdown document from the managed worklog.\nWithout --json, stdout contains the exact document bytes.\n  --json       Return status, project, revision, and content\n\nExample:\n  awl read Activities/example.md --json\n", true
	case "create":
		return "Usage: awl [--project SLUG] create DOCUMENT.md [--input FILE|-] [--request-id ID] [--json]\n\nCreate a new UTF-8 Markdown document; never overwrite an existing path.\n  --input      Read document bytes from FILE (default: stdin via -)\n  --request-id Stable ID for safe request replay\n  --json       Machine-readable result or error\n\nExample:\n  awl create Activities/example.md --input draft.md --json\n", true
	case "edit":
		return "Usage: awl [--project SLUG] edit DOCUMENT.md [--input FILE|-] [--request-id ID] [--json]\n\nReplace one unique exact match in the Markdown BODY (not YAML frontmatter).\nInput is a JSON object with nonempty \"old\" and string \"new\" fields.\n  --input      Read JSON payload from FILE (default: stdin via -)\n  --request-id Stable ID for safe request replay\n  --json       Machine-readable result or error\n\nExample:\n  printf '%s' '{\"old\":\"draft\",\"new\":\"complete\"}' | awl edit Activities/example.md --json\n", true
	case "metadata":
		return "Usage:\n  awl [--project SLUG] metadata get DOCUMENT.md POINTER [--json]\n  awl [--project SLUG] metadata set DOCUMENT.md POINTER VALUE [--request-id ID] [--json]\n  awl [--project SLUG] metadata set DOCUMENT.md --input FILE|- [--request-id ID] [--json]\n  awl [--project SLUG] metadata insert DOCUMENT.md POINTER VALUE [--request-id ID] [--json]\n  awl [--project SLUG] metadata remove DOCUMENT.md POINTER [--request-id ID] [--json]\n\nRead or change YAML frontmatter using RFC 6901 JSON Pointers.\n  POINTER       E.g. /status, /build/version, or /tags/-\n  VALUE         Positional values are strings; --input supplies typed JSON\n  set --input   A JSON object mapping existing pointers to typed values, atomically\n  insert        Insert into supported scalar YAML sequences\n  --request-id  Stable ID for safe request replay (writes)\n  --json        Machine-readable result or error\n\nExamples:\n  awl metadata get Activities/example.md /status --json\n  awl metadata set Activities/example.md /status completed --json\n  printf '%s' '{\"/status\":\"completed\",\"/completed_at\":\"2026-11-09\"}' | awl metadata set Activities/example.md --input - --json\n\nOnly a conservative YAML subset is writable; unsafe forms fail without rewriting.\n", true
	case "apply":
		return "Usage: awl [--project SLUG] apply [--input FILE|-] [--json]\n\nApply a JSON request containing request_id and ordered operations.\n  --input      Read JSON request from FILE (default: stdin via -)\n  --json       Machine-readable result or error\n\nExample:\n  awl apply --input request.json --json\n\nA request may partially succeed; exit code 3 reports a partial result.\n", true
	default:
		return "", false
	}
}

func (c *cli) handleInfo(args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch args[0] {
	case "--help", "-h", "help":
		if len(args) == 1 {
			return c.writeInfo(globalHelp), true
		}
		if args[0] == "help" && len(args) == 2 {
			return c.printCommandHelp(args[1]), true
		}
		return c.emitError(fail("USAGE", "help accepts one optional command name")), true
	case "--version", "version":
		if len(args) != 1 {
			return c.emitError(fail("USAGE", "version takes no arguments")), true
		}
		return c.writeInfo("awl " + buildVersion() + "\n"), true
	}
	if _, ok := infoForCommand(args[0]); ok && helpRequested(args[1:]) {
		return c.printCommandHelp(args[0]), true
	}
	return 0, false
}

func helpRequested(args []string) bool {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--slug", "--repo-dir", "--remote", "--input", "--request-id":
			// A value equal to --help is not a help request.
			if i+1 < len(args) {
				i++
			}
		case "--help", "-h":
			return true
		}
	}
	return false
}

func (c *cli) printCommandHelp(name string) int {
	if name == "remote" || name == "remote add" {
		name = "remote"
	}
	message, ok := infoForCommand(name)
	if !ok {
		return c.emitError(fail("USAGE", "unknown help topic: "+name))
	}
	return c.writeInfo(message)
}

func (c *cli) writeInfo(message string) int {
	if _, err := io.WriteString(c.stdout, message); err != nil {
		fmt.Fprintln(c.stderr, err)
		return 4
	}
	return 0
}

func buildVersion() string {
	if Version != "" && Version != "dev" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			revision := setting.Value
			if len(revision) > 12 {
				revision = revision[:12]
			}
			return "dev (commit " + revision + ")"
		}
	}
	return "dev"
}

