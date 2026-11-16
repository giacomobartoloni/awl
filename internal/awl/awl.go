// Package awl contains an experimental local-only Git-backed document CLI.
// This is a bounded Linux mechanism spike, not a production persistence contract.
package awl

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const managedRef = "refs/heads/awl"

type diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type result struct {
	RemoteCommit string    `json:"remote_commit,omitempty"`
	RemoteBranch string    `json:"remote_branch,omitempty"`
	Status    string      `json:"status"`
	Project   string      `json:"project,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Commit    string      `json:"commit,omitempty"`
	Tree      string      `json:"document_tree,omitempty"`
	Replayed  bool        `json:"replayed,omitempty"`
	Members   []member    `json:"members,omitempty"`
	Error     *diagnostic `json:"error,omitempty"`
}
type member struct {
	ID     string      `json:"id"`
	Op     string      `json:"op"`
	Path   string      `json:"path"`
	Status string      `json:"status"`
	Error  *diagnostic `json:"error,omitempty"`
}
type operation struct {
	ID       string         `json:"id"`
	Op       string         `json:"op"`
	Path     string         `json:"path"`
	Content  string         `json:"content,omitempty"`
	Old      string         `json:"old,omitempty"`
	New      string         `json:"new,omitempty"`
	Metadata []metadataEdit `json:"metadata,omitempty"`
}
type request struct {
	RequestID  string      `json:"request_id"`
	Operations []operation `json:"operations"`
}
type audit struct {
	Schema      int      `json:"schema"`
	RequestID   string   `json:"request_id"`
	Digest      string   `json:"digest"`
	Members     []member `json:"members"`
	Status      string   `json:"status"`
	ActorSource string   `json:"actor_source"`
}
type entry struct {
	Mode   string
	OID    string
	Bytes  []byte
	Exists bool
}
type cli struct {
	jsonOut       bool
	errorJSONSeen bool
	errorJSON     bool
	stdout        io.Writer
	stderr        io.Writer
	stdin         io.Reader
	explicit      string
}

type domainError struct{ code, msg string }

func (e domainError) Error() string { return e.msg }
func fail(code, msg string) error   { return domainError{code, msg} }
func coded(err error) *diagnostic {
	var e domainError
	if errors.As(err, &e) {
		return &diagnostic{e.code, e.msg}
	}
	return &diagnostic{"INTERNAL_ERROR", err.Error()}
}

// Run executes the experimental awl command and reports a process exit code.
// It accepts streams explicitly so CLI behavior can be tested without relying
// on global standard input and output variables.
func Run(args []string, in io.Reader, out, errOut io.Writer) int {
	c := &cli{stdout: out, stderr: errOut, stdin: in}
	// Only a leading global --project is recognized in this experimental parser.
	if len(args) >= 2 && args[0] == "--project" {
		c.explicit = args[1]
		args = args[2:]
	}
	if exit, handled := c.handleInfo(args); handled {
		return exit
	}
	if len(args) == 0 {
		return c.emitError(fail("USAGE", "expected init, remote, sync, ls, read, create, edit, metadata, or apply"))
	}
	name, args := args[0], args[1:]
	c.errorJSONSeen, c.errorJSON = requestedJSONErrorMode(args)
	if name == "ls" || name == "read" || name == "create" || name == "edit" || name == "metadata" || name == "remote" {
		args = pathFlagsFirst(args)
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	switch name {
	case "init":
		return c.commandInit(fs, args)
	case "remote":
		return c.commandRemote(fs, args)
	case "sync":
		return c.commandSync(fs, args)
	case "ls":
		return c.commandList(fs, args)
	case "read":
		return c.commandRead(fs, args)
	case "create", "edit":
		return c.commandMutate(name, fs, args)
	case "metadata":
		return c.commandMetadata(fs, args)
	case "apply":
		return c.commandApply(fs, args)

	default:
		return c.emitError(fail("USAGE", "unsupported command; experimental slice supports init, remote add, sync, ls, read, create, edit, metadata, apply"))
	}
}

// pathFlagsFirst accepts the user-facing path-first form without relaxing flag validation.
// Only known options are rearranged; unknown options still fail in flag.Parse.
func pathFlagsFirst(args []string) []string {
	options := []string{}
	positional := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			options = append(options, arg)
		case "--input", "--request-id", "--branch":
			options = append(options, arg)
			if i+1 < len(args) {
				i++
				options = append(options, args[i])
			}
		default:
			if strings.HasPrefix(arg, "--json=") || strings.HasPrefix(arg, "--input=") || strings.HasPrefix(arg, "--request-id=") || strings.HasPrefix(arg, "--branch=") {
				options = append(options, arg)
			} else {
				positional = append(positional, arg)
			}
		}
	}
	return append(options, positional...)
}

// requestedJSONErrorMode preserves an explicit structured-error preference when
// flag.Parse stops at an earlier invalid option. Known value-taking options
// must not cause their literal "--json" argument to enable JSON output.
func requestedJSONErrorMode(args []string) (seen, enabled bool) {
	for i := 0; i < len(args); i++ {
		if cliFlagTakesValue(args[i]) {
			if i+1 < len(args) {
				i++
			}
			continue
		}
		switch arg := args[i]; {
		case arg == "--json" || arg == "-json":
			seen, enabled = true, true
		case strings.HasPrefix(arg, "--json=") || strings.HasPrefix(arg, "-json="):
			_, value, _ := strings.Cut(arg, "=")
			parsed, err := strconv.ParseBool(value)
			seen = true
			// Even a malformed --json value signals a request for a structured
			// parsing error. A valid explicit false disables it.
			enabled = err != nil || parsed
		}
	}
	return seen, enabled
}

// strictJSON refuses unknown fields and concatenated values. Duplicate-key
// rejection remains a separate input-contract review gate for this spike.
func strictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var next any
	if err := decoder.Decode(&next); err != io.EOF {
		return fail("INVALID_INPUT", "unexpected trailing JSON input")
	}
	return nil
}

func (c *cli) emitRead(p project, body []byte, revision string) int {
	v := struct {
		Status   string `json:"status"`
		Project  string `json:"project"`
		Revision string `json:"revision"`
		Content  string `json:"content"`
	}{"ok", p.Slug, revision, string(body)}
	return c.encode(v)
}
func (c *cli) encode(v any) int {
	if e := json.NewEncoder(c.stdout).Encode(v); e != nil {
		fmt.Fprintln(c.stderr, e)
		return 4
	}
	return 0
}
func (c *cli) emit(r result) int {
	if c.jsonOut {
		if n := c.encode(r); n != 0 {
			return n
		}
	} else {
		fmt.Fprintf(c.stdout, "%s", r.Status)
		if r.Commit != "" {
			fmt.Fprintf(c.stdout, " %s", r.Commit)
		}
		fmt.Fprintln(c.stdout)
	}
	if r.Status == "partial" {
		return 3
	}
	if r.Status == "failed" {
		return 2
	}
	return 0
}
func (c *cli) emitError(err error) int {
	d := coded(err)
	jsonError := c.jsonOut
	if c.errorJSONSeen {
		jsonError = c.errorJSON
	}
	if jsonError {
		c.encode(result{Status: "error", Error: d})
	} else {
		fmt.Fprintf(c.stderr, "awl: %s: %s\n", d.Code, d.Message)
	}
	return 2
}
func readInput(in io.Reader, src string) ([]byte, error) {
	var r io.Reader = in
	if src != "-" {
		f, e := os.Open(src)
		if e != nil {
			return nil, fail("INPUT_UNAVAILABLE", e.Error())
		}
		defer f.Close()
		r = f
	}
	b, e := io.ReadAll(io.LimitReader(r, (8<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 8<<20 {
		return nil, fail("INPUT_TOO_LARGE", "maximum input is 8 MiB")
	}
	return b, nil
}

func safePath(raw string) (string, error) {
	if raw == "" || strings.Contains(raw, "\\") || filepath.IsAbs(raw) || strings.ContainsRune(raw, '\x00') || strings.Contains(raw, ":") {
		return "", fail("INVALID_PATH", "project-relative Markdown path required")
	}
	clean := filepath.ToSlash(filepath.Clean(raw))
	if clean == "." || clean != raw || strings.HasPrefix(clean, "../") || clean == ".." || strings.HasPrefix(clean, ".git/") || strings.HasPrefix(clean, ".awl/") || !strings.HasSuffix(strings.ToLower(clean), ".md") {
		return "", fail("INVALID_PATH", "invalid or unsafe Markdown path")
	}
	return clean, nil
}
func randomID() (string, error) {
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func hash(data []byte) string { b := sha256.Sum256(data); return hex.EncodeToString(b[:]) }
