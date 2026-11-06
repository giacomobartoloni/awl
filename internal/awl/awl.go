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
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unicode/utf8"
)

const managedRef = "refs/heads/awl"

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type result struct {
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
	ID      string `json:"id"`
	Op      string `json:"op"`
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Old     string `json:"old,omitempty"`
	New     string `json:"new,omitempty"`
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
type binding struct {
	Slug      string
	Remote    string
	LocalRepo string
}
type project struct {
	Root      string
	Common    string
	Slug      string
	Remote    string
	GitDir    string
	UserName  string
	UserEmail string
}
type entry struct {
	Mode   string
	OID    string
	Bytes  []byte
	Exists bool
}
type cli struct {
	jsonOut  bool
	stdout   io.Writer
	stderr   io.Writer
	stdin    io.Reader
	explicit string
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

func Run(args []string, in io.Reader, out, errOut io.Writer) int {
	c := &cli{stdout: out, stderr: errOut, stdin: in}
	// Only a leading global --project is recognized in this experimental parser.
	if len(args) >= 2 && args[0] == "--project" {
		c.explicit = args[1]
		args = args[2:]
	}
	if len(args) == 0 {
		return c.emitError(fail("USAGE", "expected init, read, create, edit, or apply"))
	}
	name, args := args[0], args[1:]
	if name == "read" || name == "create" || name == "edit" {
		args = pathFlagsFirst(args)
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	switch name {
	case "init":
		slug := fs.String("slug", "", "project slug for new configuration")
		remote := fs.String("remote", "", "authoritative worklog Git remote for new configuration")
		fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
		if e := fs.Parse(args); e != nil || fs.NArg() != 0 {
			return c.emitError(fail("USAGE", "init accepts --slug, --remote, --json"))
		}
		p, e := initialize(*slug, *remote, c.explicit)
		if e != nil {
			return c.emitError(e)
		}
		return c.emit(result{Status: "initialized", Project: p.Slug})
	case "read":
		fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
		if e := fs.Parse(args); e != nil || fs.NArg() != 1 {
			return c.emitError(fail("USAGE", "read requires project-relative Markdown path"))
		}
		p, e := resolve(c.explicit)
		if e != nil {
			return c.emitError(e)
		}
		path, e := safePath(fs.Arg(0))
		if e != nil {
			return c.emitError(e)
		}
		head, e := headCommit(p)
		if e != nil {
			return c.emitError(e)
		}
		file, e := readEntry(p, head, path)
		if e != nil {
			return c.emitError(e)
		}
		if !file.Exists {
			return c.emitError(fail("DOCUMENT_NOT_FOUND", "document does not exist"))
		}
		if c.jsonOut {
			if !utf8.Valid(file.Bytes) {
				return c.emitError(fail("INVALID_CONTENT", "document is not valid UTF-8 for JSON output"))
			}
			return c.emitRead(p, file.Bytes, head)
		}
		_, e = c.stdout.Write(file.Bytes)
		if e != nil {
			return c.emitError(e)
		}
		return 0
	case "create", "edit":
		fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
		input := fs.String("input", "-", "request payload source: - for stdin, otherwise a file")
		rid := fs.String("request-id", "", "stable request ID for retries")
		if e := fs.Parse(args); e != nil || fs.NArg() != 1 {
			return c.emitError(fail("USAGE", name+" requires one project-relative path"))
		}
		path, e := safePath(fs.Arg(0))
		if e != nil {
			return c.emitError(e)
		}
		data, e := readInput(c.stdin, *input)
		if e != nil {
			return c.emitError(e)
		}
		op := operation{ID: "op-1", Op: name, Path: path}
		if name == "create" {
			if !utf8.Valid(data) {
				return c.emitError(fail("INVALID_CONTENT", "Markdown must be UTF-8"))
			}
			op.Content = string(data)
		} else {
			var fields struct {
				Old string `json:"old"`
				New string `json:"new"`
			}
			if e = strictJSON(data, &fields); e != nil || fields.Old == "" {
				return c.emitError(fail("INVALID_INPUT", "edit expects JSON object with nonempty old and a new string"))
			}
			op.Old, op.New = fields.Old, fields.New
		}
		return c.writeRequest(request{RequestID: *rid, Operations: []operation{op}})
	case "apply":
		fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
		input := fs.String("input", "-", "JSON request input")
		if e := fs.Parse(args); e != nil || fs.NArg() != 0 {
			return c.emitError(fail("USAGE", "apply accepts --input and --json"))
		}
		data, e := readInput(c.stdin, *input)
		if e != nil {
			return c.emitError(e)
		}
		var req request
		if e = strictJSON(data, &req); e != nil {
			return c.emitError(fail("INVALID_INPUT", "apply requires JSON request"))
		}
		return c.writeRequest(req)
	default:
		return c.emitError(fail("USAGE", "unsupported command; experimental slice supports init, read, create, edit, apply"))
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
		case "--input", "--request-id":
			options = append(options, arg)
			if i+1 < len(args) {
				i++
				options = append(options, args[i])
			}
		default:
			if strings.HasPrefix(arg, "--json=") || strings.HasPrefix(arg, "--input=") || strings.HasPrefix(arg, "--request-id=") {
				options = append(options, arg)
			} else {
				positional = append(positional, arg)
			}
		}
	}
	return append(options, positional...)
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
	if c.jsonOut {
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

func git(dir string, env []string, input []byte, args ...string) ([]byte, error) {
	opts := []string{}
	if dir != "" {
		opts = append(opts, "--git-dir="+dir)
	}
	opts = append(opts, args...)
	cmd := exec.Command("git", opts...)
	cmd.Env = gitEnv(env)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, e := cmd.Output()
	if e != nil {
		return nil, fmt.Errorf("git %q failed: %w: %s", args, e, strings.TrimSpace(stderr.String()))
	}
	return b, nil
}
func gitEnv(extra []string) []string {
	ret := []string{}
	for _, x := range os.Environ() {
		k, _, _ := strings.Cut(x, "=")
		if strings.HasPrefix(k, "GIT_") {
			continue
		}
		ret = append(ret, x)
	}
	return append(ret, extra...)
}
func gitText(dir string, env []string, input []byte, args ...string) (string, error) {
	b, e := git(dir, env, input, args...)
	return strings.TrimSuffix(string(b), "\n"), e
}
func gitSource(root string, args ...string) ([]byte, error) {
	a := append([]string{"-C", root}, args...)
	s, e := gitText("", nil, nil, a...)
	return []byte(s), e
}
func source() (string, string, error) {
	cwd, e := os.Getwd()
	if e != nil {
		return "", "", e
	}
	root, e := gitSource(cwd, "rev-parse", "--show-toplevel")
	if e != nil {
		return "", "", fail("PROJECT_NOT_RESOLVED", "not inside a source Git worktree")
	}
	common, e := gitSource(cwd, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil {
		return "", "", fail("PROJECT_NOT_RESOLVED", "cannot determine Git common directory")
	}
	return string(root), string(common), nil
}
func identity(root string) (string, string, error) {
	name, e := gitSource(root, "config", "user.name")
	if e != nil || len(name) == 0 {
		return "", "", fail("IDENTITY_UNAVAILABLE", "configure source Git user.name")
	}
	email, e := gitSource(root, "config", "user.email")
	if e != nil || len(email) == 0 {
		return "", "", fail("IDENTITY_UNAVAILABLE", "configure source Git user.email")
	}
	return string(name), string(email), nil
}
func configRead(file string, allowed map[string]bool) (map[string]string, error) {
	b, e := os.ReadFile(file)
	if e != nil {
		return nil, e
	}
	m := map[string]string{}
	scope := ""
	seenSections := map[string]bool{}
	for _, raw := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if strings.HasSuffix(s, ":") && raw == s {
			scope = strings.TrimSuffix(s, ":")
			if seenSections[scope] {
				return nil, fail("INVALID_CONFIG", "duplicate YAML section")
			}
			seenSections[scope] = true
			if !allowed[scope] {
				return nil, fail("INVALID_CONFIG", "unknown config section")
			}
			continue
		}
		name, v, ok := strings.Cut(s, ":")
		if !ok {
			return nil, fail("INVALID_CONFIG", "malformed YAML field")
		}
		key := strings.TrimSpace(name)
		if strings.HasPrefix(raw, "  ") && scope != "" {
			key = scope + "." + key
		} else if raw != s {
			return nil, fail("INVALID_CONFIG", "unsupported YAML indentation")
		} else {
			scope = ""
		}
		if !allowed[key] {
			return nil, fail("INVALID_CONFIG", "unknown YAML field: "+key)
		}
		if _, duplicate := m[key]; duplicate {
			return nil, fail("INVALID_CONFIG", "duplicate YAML field: "+key)
		}
		value := strings.TrimSpace(v)
		if value == "" || strings.ContainsAny(value, "\n\r#\"'{}[]") || strings.HasPrefix(value, "!") {
			return nil, fail("INVALID_CONFIG", "unsupported YAML scalar: "+key)
		}
		m[key] = value
	}
	if m["schema"] != "1" {
		return nil, fail("INVALID_CONFIG", "expected schema: 1")
	}
	return m, nil
}
func parseShared(path string) (string, string, error) {
	m, e := configRead(path, map[string]bool{"project": true, "storage": true, "schema": true, "project.slug": true, "storage.remote": true})
	if e != nil {
		return "", "", e
	}
	slug, remote := m["project.slug"], m["storage.remote"]
	if !slugPattern.MatchString(slug) || remote == "" {
		return "", "", fail("INVALID_CONFIG", "missing or invalid slug/remote")
	}
	return slug, remote, nil
}
func parseLocal(path string) (binding, error) {
	m, e := configRead(path, map[string]bool{"binding": true, "storage": true, "schema": true, "binding.slug": true, "binding.remote": true, "storage.localRepo": true})
	if e != nil {
		return binding{}, e
	}
	b := binding{m["binding.slug"], m["binding.remote"], m["storage.localRepo"]}
	if !slugPattern.MatchString(b.Slug) || b.Remote == "" || !filepath.IsAbs(b.LocalRepo) {
		return b, fail("INVALID_CONFIG", "incomplete local binding")
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
func flock(file string) (func(), error) {
	f, e := os.OpenFile(file, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		f.Close()
		return nil, e
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}

// The rest of the local Git plumbing is implemented in gitstore.go.
