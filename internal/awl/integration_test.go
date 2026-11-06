package awl

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type testCLI struct{ Binary, Source, Data string }

func fixture(t *testing.T) testCLI {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "source")
	if err := os.Mkdir(src, 0700); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Clean(filepath.Join("..", ".."))
	bin := filepath.Join(root, "awl")
	build := exec.Command("go", "build", "-o", bin, "./cmd/awl")
	build.Dir = repo
	if out, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build: %v: %s", e, out)
	}
	c := testCLI{Binary: bin, Source: src, Data: filepath.Join(root, "data")}
	c.git(t, "init", "-q", "-b", "main")
	c.git(t, "config", "user.name", "Fixture User")
	c.git(t, "config", "user.email", "fixture@example.invalid")
	return c
}
func (c testCLI) git(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = c.Source
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("git %v: %v: %s", args, e, out)
	} else {
		return strings.TrimSpace(string(out))
	}
	return ""
}
func (c testCLI) call(input []byte, args ...string) (int, []byte, string) {
	cmd := exec.Command(c.Binary, args...)
	cmd.Dir = c.Source
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = append(os.Environ(), "XDG_DATA_HOME="+c.Data)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	e := cmd.Run()
	if e == nil {
		return 0, stdout.Bytes(), stderr.String()
	}
	if x, ok := e.(*exec.ExitError); ok {
		return x.ExitCode(), stdout.Bytes(), stderr.String()
	}
	return 127, stdout.Bytes(), e.Error() + stderr.String()
}
func (c testCLI) mustCall(t *testing.T, input []byte, args ...string) result {
	t.Helper()
	rc, out, err := c.call(input, args...)
	if rc != 0 {
		t.Fatalf("awl %v: exit %d: %s; stderr: %s", args, rc, out, err)
	}
	var v result
	if e := json.Unmarshal(out, &v); e != nil {
		t.Fatalf("awl %v invalid JSON: %s: %v", args, out, e)
	}
	return v
}
func (c testCLI) init(t *testing.T) {
	t.Helper()
	r := c.mustCall(t, nil, "init", "--slug", "demo", "--remote", "git@example.invalid:demo-worklog.git", "--json")
	if r.Status != "initialized" {
		t.Fatalf("init: %+v", r)
	}
}
func (c testCLI) read(t *testing.T, path string) []byte {
	t.Helper()
	rc, out, err := c.call(nil, "read", path)
	if rc != 0 {
		t.Fatalf("read %s: %v %s", path, rc, err)
	}
	return out
}
func TestCreateReadEditExactBytesAndIdempotentAudit(t *testing.T) {
	c := fixture(t)
	c.init(t)
	initial := []byte("---\r\nstatus: active\r\n---\r\nHello, α!\r\n")
	made := c.mustCall(t, initial, "create", "Activities/demo.md", "--request-id", "create-01", "--json")
	if len(made.Members) != 1 || made.Members[0].Status != "applied" {
		t.Fatalf("create: %+v", made)
	}
	if got := c.read(t, "Activities/demo.md"); !bytes.Equal(got, initial) {
		t.Fatalf("create changed bytes: %q != %q", got, initial)
	}
	again := c.mustCall(t, initial, "create", "Activities/demo.md", "--request-id", "create-01", "--json")
	if !again.Replayed || again.Commit != made.Commit {
		t.Fatalf("idempotency: %+v vs %+v", again, made)
	}
	changed := c.mustCall(t, []byte(`{"old":"Hello, α!","new":"Hello, β!"}`), "edit", "Activities/demo.md", "--request-id", "edit-01", "--json")
	if changed.Tree == made.Tree {
		t.Fatal("edit should change tree")
	}
	expected := []byte("---\r\nstatus: active\r\n---\r\nHello, β!\r\n")
	if got := c.read(t, "Activities/demo.md"); !bytes.Equal(got, expected) {
		t.Fatalf("frontmatter/EOL damaged: %q != %q", got, expected)
	}
	noop := c.mustCall(t, []byte(`{"old":"Hello, β!","new":"Hello, β!"}`), "edit", "Activities/demo.md", "--request-id", "noop-01", "--json")
	if noop.Tree != changed.Tree || noop.Commit == changed.Commit || noop.Members[0].Status != "no_op" {
		t.Fatalf("no-op audit: %+v", noop)
	}
	rc, out, _ := c.call([]byte(`{"old":"unmatched","new":"new"}`), "edit", "Activities/demo.md", "--request-id", "fail-01", "--json")
	var failed result
	if e := json.Unmarshal(out, &failed); e != nil {
		t.Fatal(e)
	}
	if rc != 2 || failed.Status != "failed" || failed.Tree != changed.Tree || failed.Commit == noop.Commit {
		t.Fatalf("ordinary failure must publish audit-only: exit %d %+v", rc, failed)
	}
	rc, out, _ = c.call([]byte(`{"old":"Hello, β!","new":"different"}`), "edit", "Activities/demo.md", "--request-id", "edit-01", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("REQUEST_ID_CONFLICT")) {
		t.Fatalf("changed retry must be rejected: %d %s", rc, out)
	}
	if got := c.read(t, "Activities/demo.md"); !bytes.Equal(got, expected) {
		t.Fatal("retry changed source document")
	}
}
func TestCompositeContinuesOrdinaryFailureAndAuditsOnce(t *testing.T) {
	c := fixture(t)
	c.init(t)
	req := `{"request_id":"composite-01","operations":[{"id":"first","op":"create","path":"Activities/a.md","content":"A\n"},{"id":"middle","op":"edit","path":"Activities/absent.md","old":"old","new":"new"},{"id":"last","op":"create","path":"Specs/b.md","content":"B\n"}]}`
	rc, out, err := c.call([]byte(req), "apply", "--input", "-", "--json")
	var res result
	if e := json.Unmarshal(out, &res); e != nil {
		t.Fatalf("bad JSON: %s / %v", out, e)
	}
	if rc != 3 || res.Status != "partial" || len(res.Members) != 3 || res.Members[0].Status != "applied" || res.Members[1].Status != "failed" || res.Members[2].Status != "applied" {
		t.Fatalf("composite: %d %+v stderr %s", rc, res, err)
	}
	if string(c.read(t, "Activities/a.md")) != "A\n" || string(c.read(t, "Specs/b.md")) != "B\n" {
		t.Fatal("missing composite content")
	}
	rc, out, _ = c.call([]byte(req), "apply", "--input", "-", "--json")
	var replay result
	json.Unmarshal(out, &replay)
	if rc != 3 || !replay.Replayed || replay.Commit != res.Commit {
		t.Fatalf("composite replay: %d %+v", rc, replay)
	}
}
func TestInitIsExplicitAndSharesGitCommonDirAcrossWorktrees(t *testing.T) {
	c := fixture(t)
	rc, _, _ := c.call(nil, "read", "Activities/one.md")
	if rc == 0 {
		t.Fatal("read must not bootstrap")
	}
	if _, e := os.Stat(filepath.Join(c.Source, ".awl")); !os.IsNotExist(e) {
		t.Fatalf("read created configuration: %v", e)
	}
	c.init(t)
	c.git(t, "add", ".awl/project.yaml")
	c.git(t, "commit", "-q", "-m", "fixture project binding")
	other := filepath.Join(filepath.Dir(c.Source), "linked")
	c.git(t, "worktree", "add", "-q", "-b", "linked", other)
	linked := c
	linked.Source = other
	r := linked.mustCall(t, nil, "init", "--json")
	if r.Status != "initialized" {
		t.Fatalf("linked init: %+v", r)
	}
	b := c.mustCall(t, []byte("Same shared state\n"), "create", "Activities/shared.md", "--json")
	if b.Commit == "" || string(linked.read(t, "Activities/shared.md")) != "Same shared state\n" {
		t.Fatal("linked worktree did not resolve same store")
	}
	rc, out, _ := linked.call(nil, "--project", "other", "read", "Activities/shared.md", "--json")
	if rc == 0 || !bytes.Contains(out, []byte("PROJECT_NOT_RESOLVED")) {
		t.Fatalf("unrelated project selection unexpectedly succeeded: %d %s", rc, out)
	}
}
func TestUnsafePathsAndMalformedRequestsAreRejectedBeforePublication(t *testing.T) {
	c := fixture(t)
	c.init(t)
	cases := []string{"../secret.md", "/etc/passwd.md", "Activities/../../bad.md", ".awl/private.md", "docs//bad.md"}
	for _, path := range cases {
		rc, out, _ := c.call([]byte("oops"), "create", path, "--json")
		if rc == 0 || !bytes.Contains(out, []byte("INVALID_PATH")) {
			t.Fatalf("unsafe path %q: %d %s", path, rc, out)
		}
	}
	rc, out, _ := c.call([]byte("not JSON"), "edit", "Activities/file.md", "--json")
	if rc == 0 || !bytes.Contains(out, []byte("INVALID_INPUT")) {
		t.Fatalf("invalid edit: %d %s", rc, out)
	}
}
func TestConcurrentDisjointCreatesDoNotLoseCommittedData(t *testing.T) {
	c := fixture(t)
	c.init(t)
	type callResult struct {
		code int
		out  []byte
		err  string
	}
	cases := []string{"Activities/first.md", "Activities/second.md"}
	ch := make(chan callResult, len(cases))
	var wg sync.WaitGroup
	for i, name := range cases {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			code, out, err := c.call([]byte("document"), "create", path, "--request-id", "parallel-0"+string(rune('1'+i)), "--json")
			ch <- callResult{code, out, err}
		}(i, name)
	}
	wg.Wait()
	close(ch)
	for r := range ch {
		if r.code != 0 {
			t.Fatalf("concurrent operation failed: %d %s %s", r.code, r.out, r.err)
		}
	}
	for _, name := range cases {
		if string(c.read(t, name)) != "document" {
			t.Fatalf("lost content: %s", name)
		}
	}
}

func TestFrontmatterBOMAndMalformedFenceAreProtected(t *testing.T) {
	c := fixture(t)
	c.init(t)
	initial := append([]byte{0xef, 0xbb, 0xbf}, []byte("---\r\nstatus: active\r\n---\r\nBody\r\n")...)
	c.mustCall(t, initial, "create", "Specs/bom.md", "--json")
	bodyEdit := []byte(`{"old":"Body","new":"Changed"}`)
	c.mustCall(t, bodyEdit, "edit", "Specs/bom.md", "--json")
	got := c.read(t, "Specs/bom.md")
	expected := append([]byte{0xef, 0xbb, 0xbf}, []byte("---\r\nstatus: active\r\n---\r\nChanged\r\n")...)
	if !bytes.Equal(got, expected) {
		t.Fatalf("BOM/frontmatter mismatch: %q", got)
	}
	rc, out, _ := c.call([]byte(`{"old":"active","new":"destroyed"}`), "edit", "Specs/bom.md", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("EDIT_TARGET_MISSING")) {
		t.Fatalf("edited YAML frontmatter: %d %s", rc, out)
	}
	c.mustCall(t, []byte("---\nstatus: active\nmissing closing fence\n"), "create", "Specs/malformed.md", "--json")
	rc, out, _ = c.call([]byte(`{"old":"active","new":"unsafe"}`), "edit", "Specs/malformed.md", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("MALFORMED_FRONTMATTER")) {
		t.Fatalf("malformed YAML edit: %d %s", rc, out)
	}
}

func TestRejectedInitAndDuplicateOperationIDsHaveNoSideEffects(t *testing.T) {
	c := fixture(t)
	rc, _, _ := c.call(nil, "init", "--slug", "demo", "--json")
	if rc == 0 {
		t.Fatal("missing remote accepted")
	}
	if _, e := os.Stat(filepath.Join(c.Source, ".awl")); !os.IsNotExist(e) {
		t.Fatalf("invalid init created source .awl: %v", e)
	}
	c.init(t)
	req := `{"request_id":"bad-members","operations":[{"id":"duplicate","op":"create","path":"one.md","content":"A"},{"id":"duplicate","op":"create","path":"two.md","content":"B"}]}`
	rc, out, _ := c.call([]byte(req), "apply", "--json")
	if rc == 0 || !bytes.Contains(out, []byte("INVALID_REQUEST")) {
		t.Fatalf("duplicate accepted: %d %s", rc, out)
	}
	if rc, _, _ := c.call(nil, "read", "one.md"); rc == 0 {
		t.Fatal("bad composite changed documents")
	}
}

func TestCommitMessageAuditAndNoUnexpectedSourceCommit(t *testing.T) {
	c := fixture(t)
	c.init(t)
	if log := c.git(t, "rev-list", "--all", "--count"); log != "0" {
		t.Fatalf("init unexpectedly committed to source Git: %s", log)
	}
	bindingFile := filepath.Join(c.Source, ".git", "awl", "local.yaml")
	b, e := parseLocal(bindingFile)
	if e != nil {
		t.Fatalf("local binding: %v", e)
	}
	req := `{"request_id":"audit-01","operations":[{"id":"ok","op":"create","path":"Activities/a.md","content":"hello"},{"id":"bad","op":"edit","path":"Activities/missing.md","old":"value","new":"new"}]}`
	rc, out, _ := c.call([]byte(req), "apply", "--json")
	if rc != 3 {
		t.Fatalf("expected partial success: %d %s", rc, out)
	}
	cmd := exec.Command("git", "--git-dir="+b.LocalRepo, "log", "-1", "--format=%B", managedRef)
	payload, e := cmd.Output()
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.HasPrefix(payload, []byte("AWL-AUDIT-V1\n")) || !bytes.Contains(payload, []byte(`"request_id":"audit-01"`)) || !bytes.Contains(payload, []byte(`"id":"bad"`)) || !bytes.Contains(payload, []byte(`"status":"failed"`)) {
		t.Fatalf("incomplete audit: %s", payload)
	}
	tree := exec.Command("git", "--git-dir="+b.LocalRepo, "rev-parse", managedRef+"^{tree}")
	if out, e := tree.CombinedOutput(); e != nil || len(bytes.TrimSpace(out)) != 40 {
		t.Fatalf("invalid tree: %s %v", out, e)
	}
}

func TestOptionalLocalSymlinkIsValidOnlyWhenCanonicalAndIgnored(t *testing.T) {
	c := fixture(t)
	c.init(t)
	c.mustCall(t, []byte("present"), "create", "Docs/one.md", "--json")
	link := filepath.Join(c.Source, ".awl", "local.yaml")
	if e := os.Symlink(filepath.Join(c.Source, ".git", "awl", "local.yaml"), link); e != nil {
		t.Fatal(e)
	}
	rc, out, _ := c.call(nil, "read", "Docs/one.md", "--json")
	if rc == 0 || !bytes.Contains(out, []byte("LOCAL_CONFIG_NOT_IGNORED")) {
		t.Fatalf("unignored private config link accepted: %d %s", rc, out)
	}
	if e := os.WriteFile(filepath.Join(c.Source, ".gitignore"), []byte("/.awl/local.yaml\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if got := c.read(t, "Docs/one.md"); string(got) != "present" {
		t.Fatalf("valid optional link rejected: %q", got)
	}
	os.Remove(link)
	if e := os.Symlink(filepath.Join(c.Source, "outside-local.yaml"), link); e != nil {
		t.Fatal(e)
	}
	rc, out, _ = c.call(nil, "read", "Docs/one.md", "--json")
	if rc == 0 || !bytes.Contains(out, []byte("LOCAL_CONFIG_CONFLICT")) {
		t.Fatalf("unexpected symlink accepted: %d %s", rc, out)
	}
}
