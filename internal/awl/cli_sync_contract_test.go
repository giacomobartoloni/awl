package awl

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func syncTestGit(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func syncTestBareRemote(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "worklogs.git")
	syncTestGit(t, "init", "--bare", "-q", path)
	return path
}

func syncTestProject(t *testing.T, url, branch string) testCLI {
	t.Helper()
	c := fixture(t)
	args := []string{"init", "--slug", "demo", "--remote", url}
	if branch != "" {
		args = append(args, "--remote-branch", branch)
	}
	c.mustCall(t, nil, append(args, "--json")...)
	return c
}

func syncTestStatus(t *testing.T, c testCLI, expected string) result {
	t.Helper()
	res := c.mustCall(t, nil, "sync", "--json")
	if res.Status != expected || res.RemoteCommit == "" || res.RemoteBranch == "" {
		t.Fatalf("sync status %q: %+v", expected, res)
	}
	return res
}

func syncTestRejected(t *testing.T, c testCLI, code string) {
	t.Helper()
	rc, out, stderr := c.call(nil, "sync", "--json")
	var got result
	if err := json.Unmarshal(out, &got); err != nil || rc != 2 || stderr != "" ||
		got.Error == nil || got.Error.Code != code {
		t.Fatalf("sync wanted %q: exit=%d stdout=%q stderr=%q parsed=%+v", code, rc, out, stderr, got)
	}
}

func syncTestTip(t *testing.T, url, branch string) string {
	t.Helper()
	return syncTestGit(t, "--git-dir="+url, "rev-parse", "refs/heads/"+branch)
}

func TestCLIContractSyncInitialPublicationAndIdempotentRetry(t *testing.T) {
	url := syncTestBareRemote(t)
	c := syncTestProject(t, url, "worklogs/api")
	local := c.mustCall(t, []byte("# One\n"), "create", "Notes/one.md", "--request-id", "first-01", "--json")
	first := syncTestStatus(t, c, "published")
	if first.RemoteCommit != local.Commit || first.Commit != local.Commit ||
		syncTestTip(t, url, "worklogs/api") != local.Commit {
		t.Fatalf("incorrect first publication: %+v vs %+v", first, local)
	}
	if _, err := os.Stat(filepath.Join(url, "refs", "heads", "awl")); err == nil {
		t.Fatal("sync used legacy branch despite selected destination")
	}
	second := syncTestStatus(t, c, "up_to_date")
	if second.RemoteCommit != first.RemoteCommit || second.Commit != first.Commit {
		t.Fatalf("idempotent sync changed published state: %+v", second)
	}
	got := c.read(t, "Notes/one.md")
	if !bytes.Equal(got, []byte("# One\n")) {
		t.Fatalf("sync modified accepted document: %q", got)
	}
}

func TestCLIContractSyncPristineAdoptionAndFastForwardBetweenComputers(t *testing.T) {
	url := syncTestBareRemote(t)
	a := syncTestProject(t, url, "worklogs/api")
	first := a.mustCall(t, []byte("# First computer\n"), "create", "Notes/a.md", "--request-id", "a-01", "--json")
	syncTestStatus(t, a, "published")

	b := syncTestProject(t, url, "worklogs/api")
	empty, err := resolveFromFixture(b)
	if err != nil {
		t.Fatal(err)
	}
	boot, err := headCommit(empty)
	if err != nil {
		t.Fatal(err)
	}
	if boot == first.Commit {
		t.Fatal("two initializers unexpectedly shared the same bootstrap")
	}
	adopted := syncTestStatus(t, b, "adopted")
	if adopted.Commit != first.Commit || string(b.read(t, "Notes/a.md")) != "# First computer\n" {
		t.Fatalf("second computer failed to adopt history: %+v", adopted)
	}
	newDoc := b.mustCall(t, []byte("# Second computer\n"), "create", "Notes/b.md", "--request-id", "b-01", "--json")
	syncTestStatus(t, b, "published")
	fastForward := syncTestStatus(t, a, "fast_forwarded")
	if fastForward.Commit != newDoc.Commit || string(a.read(t, "Notes/b.md")) != "# Second computer\n" {
		t.Fatalf("first computer failed to fast-forward: %+v", fastForward)
	}
	if got := syncTestTip(t, url, "worklogs/api"); got != newDoc.Commit {
		t.Fatalf("remote history was not preserved: %s != %s", got, newDoc.Commit)
	}
}

func resolveFromFixture(c testCLI) (project, error) {
	// The test fixture executes awl in a separate process, so load its
	// configured store using the canonical binding without changing cwd.
	b, err := parseLocal(filepath.Join(c.Source, ".git", "awl", "local.yaml"))
	if err != nil {
		return project{}, err
	}
	return project{GitDir: b.LocalRepo}, nil
}

func TestCLIContractSyncRejectsIndependentMeaningfulHistories(t *testing.T) {
	url := syncTestBareRemote(t)
	a := syncTestProject(t, url, "worklogs/api")
	origin := a.mustCall(t, []byte("# Remote\n"), "create", "Notes/remote.md", "--json")
	syncTestStatus(t, a, "published")

	b := syncTestProject(t, url, "worklogs/api")
	local := b.mustCall(t, []byte("# Offline\n"), "create", "Notes/local.md", "--json")
	syncTestRejected(t, b, "SYNC_DIVERGED")
	if got := syncTestTip(t, url, "worklogs/api"); got != origin.Commit {
		t.Fatalf("rejected merge changed remote: %s", got)
	}
	if string(b.read(t, "Notes/local.md")) != "# Offline\n" {
		t.Fatal("rejected sync lost offline document")
	}
	p, err := resolveFromFixture(b)
	if err != nil {
		t.Fatal(err)
	}
	current, err := headCommit(p)
	if err != nil || current != local.Commit {
		t.Fatalf("rejected sync replaced local audit: %s %v", current, err)
	}
}

func TestCLIContractSyncAuditOnlyIsNotPristine(t *testing.T) {
	url := syncTestBareRemote(t)
	a := syncTestProject(t, url, "")
	a.mustCall(t, []byte("# Remote\n"), "create", "Notes/remote.md", "--json")
	syncTestStatus(t, a, "published")
	b := syncTestProject(t, url, "")
	// A completed failed edit publishes an audit-only commit on an empty tree.
	rc, out, _ := b.call([]byte(`{"old":"missing","new":"other"}`), "edit", "Notes/missing.md", "--request-id", "failed-01", "--json")
	if rc != 2 || !bytes.Contains(out, []byte(`"status":"failed"`)) {
		t.Fatalf("fixture failed request was not audited: exit=%d %q", rc, out)
	}
	syncTestRejected(t, b, "SYNC_DIVERGED")
}

func TestCLIContractSyncUsesSeparateBranchesInSharedRepository(t *testing.T) {
	url := syncTestBareRemote(t)
	a := syncTestProject(t, url, "worklogs/api")
	b := syncTestProject(t, url, "worklogs/web")
	aHead := a.mustCall(t, []byte("API\n"), "create", "Notes/api.md", "--json")
	bHead := b.mustCall(t, []byte("WEB\n"), "create", "Notes/web.md", "--json")
	syncTestStatus(t, a, "published")
	syncTestStatus(t, b, "published")
	if syncTestTip(t, url, "worklogs/api") != aHead.Commit ||
		syncTestTip(t, url, "worklogs/web") != bHead.Commit {
		t.Fatal("shared remote branches are not independent")
	}
	if aHead.Commit == bHead.Commit {
		t.Fatal("independent histories unexpectedly have the same head")
	}
	syncTestStatus(t, a, "up_to_date")
	syncTestStatus(t, b, "up_to_date")
}

func TestCLIContractSyncNeverRecreatesDeletedVerifiedBranch(t *testing.T) {
	url := syncTestBareRemote(t)
	c := syncTestProject(t, url, "worklogs/api")
	c.mustCall(t, []byte("# Keep\n"), "create", "Notes/keep.md", "--json")
	syncTestStatus(t, c, "published")
	syncTestGit(t, "--git-dir="+url, "update-ref", "-d", "refs/heads/worklogs/api")
	syncTestRejected(t, c, "SYNC_REMOTE_DELETED")
	if out := syncTestGit(t, "--git-dir="+url, "show-ref"); strings.Contains(out, "worklogs/api") {
		t.Fatal("sync silently recreated a previously verified deleted branch")
	}
	if string(c.read(t, "Notes/keep.md")) != "# Keep\n" {
		t.Fatal("deleted remote branch caused local data loss")
	}
}

func TestCLIContractSyncRejectsRemoteHistoryRewrite(t *testing.T) {
	url := syncTestBareRemote(t)
	c := syncTestProject(t, url, "worklogs/api")
	c.mustCall(t, []byte("# Keep\n"), "create", "Notes/keep.md", "--json")
	syncTestStatus(t, c, "published")
	p, err := resolveFromFixture(c)
	if err != nil {
		t.Fatal(err)
	}
	root := syncTestGit(t, "--git-dir="+p.GitDir, "rev-list", "--max-parents=0", managedRef)
	syncTestGit(t, "--git-dir="+url, "update-ref", "refs/heads/worklogs/api", root)
	syncTestRejected(t, c, "SYNC_REMOTE_REWRITTEN")
	if string(c.read(t, "Notes/keep.md")) != "# Keep\n" {
		t.Fatal("remote rewrite was propagated into local history")
	}
}

func TestCLIContractSyncReportsUnavailableAndUnconfiguredRemote(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	syncTestRejected(t, c, "REMOTE_NOT_CONFIGURED")
	absent := filepath.Join(t.TempDir(), "nonexistent.git")
	c.mustCall(t, nil, "remote", "add", absent, "--json")
	syncTestRejected(t, c, "SYNC_REMOTE_UNAVAILABLE")
}

func TestCLIContractSyncRejectsInvalidImportedHistory(t *testing.T) {
	url := syncTestBareRemote(t)
	a := syncTestProject(t, url, "worklogs/api")
	a.mustCall(t, []byte("# Valid\n"), "create", "Notes/valid.md", "--json")
	syncTestStatus(t, a, "published")
	// Introduce an unaudited commit without rewriting or forcing the remote.
	root := t.TempDir()
	syncTestGit(t, "clone", "-q", "--branch", "worklogs/api", url, root)
	syncTestGit(t, "-C", root, "config", "user.name", "External Writer")
	syncTestGit(t, "-C", root, "config", "user.email", "external@example.invalid")
	if err := os.WriteFile(filepath.Join(root, "unmanaged.txt"), []byte("unsafe"), 0600); err != nil {
		t.Fatal(err)
	}
	syncTestGit(t, "-C", root, "add", "unmanaged.txt")
	syncTestGit(t, "-C", root, "commit", "-q", "-m", "external untracked change")
	syncTestGit(t, "-C", root, "push", "-q", "origin", "HEAD:refs/heads/worklogs/api")
	b := syncTestProject(t, url, "worklogs/api")
	syncTestRejected(t, b, "SYNC_UNSUPPORTED_HISTORY")
}

func TestCLIContractSyncRejectsUnsupportedMergeOption(t *testing.T) {
	c := fixture(t)
	rc, out, stderr := c.call(nil, "sync", "--allow-unrelated-histories", "--json")
	if rc != 2 || stderr != "" || !bytes.Contains(out, []byte("USAGE")) {
		t.Fatalf("unsupported history merge option accepted: %d %q %s", rc, out, stderr)
	}
}
