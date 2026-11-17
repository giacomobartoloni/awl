package awl

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestCLIContractRemoteSetBranchPreservesManagedHistory(t *testing.T) {
	c := fixture(t)
	url := "git@example.invalid:team/worklogs.git"
	c.mustCall(t, nil, "init", "--slug", "demo", "--remote", url, "--json")
	created := c.mustCall(t, []byte("---
status: active
---
# Keep
"), "create", "Notes/keep.md", "--json")
	shared, local := remoteBranchPaths(c)
	before, err := parseLocal(local)
	if err != nil {
		t.Fatal(err)
	}
	res := c.mustCall(t, nil, "remote", "set-branch", "worklog/awl", "--json")
	if res.Status != "remote_branch_updated" || res.RemoteBranch != "worklog/awl" || res.Project != "demo" {
		t.Fatalf("unexpected branch update: %+v", res)
	}
	assertRemoteBranch(t, c, url, "worklog/awl")
	if got := c.mustCall(t, nil, "remote", "set-branch", "worklog/awl", "--json"); got.Status != "remote_branch_unchanged" {
		t.Fatalf("retry should be idempotent: %+v", got)
	}
	after, err := parseLocal(local)
	if err != nil || after.LocalRepo != before.LocalRepo {
		t.Fatalf("managed repository relocated: before=%+v after=%+v err=%v", before, after, err)
	}
	head, err := gitText(after.LocalRepo, nil, nil, "rev-parse", managedRef)
	if err != nil || head != created.Commit {
		t.Fatalf("local document history changed: %s %v", head, err)
	}
	if string(c.read(t, "Notes/keep.md")) != "---
status: active
---
# Keep
" {
		t.Fatal("document bytes changed")
	}
	if got := c.mustCall(t, nil, "remote", "set-branch", "awl", "--json"); got.Status != "remote_branch_updated" {
		t.Fatalf("return to default branch: %+v", got)
	}
	assertRemoteBranch(t, c, url, "awl")
	for _, path := range []string{shared, local} {
		content, err := os.ReadFile(path)
		if err != nil || bytes.Contains(content, []byte("remoteBranch:")) {
			t.Fatalf("default branch must remain implicit: %s: %q %v", path, content, err)
		}
	}
}

func TestCLIContractRemoteSetBranchPreservesCommentsAndCRLF(t *testing.T) {
	c := fixture(t)
	url := "git@example.invalid:team/worklogs.git"
	c.mustCall(t, nil, "init", "--slug", "demo", "--remote", url, "--json")
	shared, local := remoteBranchPaths(c)
	for _, path := range []string{shared, local} {
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		withComments := append([]byte("# User-owned comment
"), bytes.ReplaceAll(original, []byte("
"), []byte("
"))...)
		if err := os.WriteFile(path, withComments, 0600); err != nil {
			t.Fatal(err)
		}
	}
	c.mustCall(t, nil, "remote", "set-branch", "worklog/awl", "--json")
	assertRemoteBranch(t, c, url, "worklog/awl")
	for _, path := range []string{shared, local} {
		content, err := os.ReadFile(path)
		if err != nil || !bytes.HasPrefix(content, []byte("# User-owned comment
")) ||
			!bytes.Contains(content, []byte("  remoteBranch: worklog/awl
")) ||
			bytes.Contains(bytes.ReplaceAll(content, []byte("
"), nil), []byte("
")) {
			t.Fatalf("rewrote unrelated bytes or line endings: %s: %q %v", path, content, err)
		}
	}
}

func TestCLIContractRemoteSetBranchRecoversOneFileInterruption(t *testing.T) {
	for _, advanced := range []string{"shared", "local"} {
		t.Run(advanced, func(t *testing.T) {
			c := fixture(t)
			url := "git@example.invalid:team/worklogs.git"
			c.mustCall(t, nil, "init", "--slug", "demo", "--remote", url, "--json")
			shared, local := remoteBranchPaths(c)
			path := shared
			if advanced == "local" {
				path = local
			}
			old, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			halfway, err := withRemoteBranch(old, "worklog/awl")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, halfway, 0600); err != nil {
				t.Fatal(err)
			}
			changed := c.mustCall(t, nil, "remote", "set-branch", "worklog/awl", "--json")
			if changed.Status != "remote_branch_updated" {
				t.Fatalf("failed to complete partial update: %+v", changed)
			}
			assertRemoteBranch(t, c, url, "worklog/awl")
			unchanged, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(unchanged, halfway) {
				t.Fatalf("modified already updated file: %q %v", unchanged, err)
			}
		})
	}
}

func TestCLIContractRemoteSetBranchRejectsUnsafeRequests(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	shared, local := remoteBranchPaths(c)
	for _, branch := range []string{"", "-bad", "refs/heads/worklog/awl", "worklog//awl", "@{-1}", "with spaces"} {
		rc, out, _ := c.call(nil, "remote", "set-branch", branch, "--json")
		if rc != 2 || !bytes.Contains(out, []byte("INVALID_REMOTE_BRANCH")) {
			t.Fatalf("invalid branch %q accepted: %d %q", branch, rc, out)
		}
	}
	rc, out, _ := c.call(nil, "remote", "set-branch", "worklog/awl", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("REMOTE_NOT_CONFIGURED")) {
		t.Fatalf("allowed change with no configured URL: %d %q", rc, out)
	}
	url := "git@example.invalid:team/worklogs.git"
	c.mustCall(t, nil, "remote", "add", url, "--json")
	rc, out, _ = c.call(nil, "--project", "other", "remote", "set-branch", "worklog/awl", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("PROJECT_CONFLICT")) {
		t.Fatalf("wrong project accepted: %d %q", rc, out)
	}
	rc, out, _ = c.call(nil, "remote", "set-branch", "worklog/awl", "--branch", "other", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("USAGE")) {
		t.Fatalf("unexpected option accepted: %d %q", rc, out)
	}
	original, err := os.ReadFile(shared)
	if err != nil {
		t.Fatal(err)
	}
	mismatch := bytes.Replace(original, []byte(url), []byte("git@example.invalid:other.git"), 1)
	if err := os.WriteFile(shared, mismatch, 0600); err != nil {
		t.Fatal(err)
	}
	rc, out, _ = c.call(nil, "remote", "set-branch", "worklog/awl", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("PROJECT_CONFLICT")) {
		t.Fatalf("source/canonical URL mismatch was accepted: %d %q", rc, out)
	}
	b, err := parseLocal(local)
	if err != nil || b.RemoteBranch != "awl" {
		t.Fatalf("rejected request changed canonical binding: %+v %v", b, err)
	}
}

func TestCLIContractRemoteSetBranchRejectsVerifiedSyncCheckpoint(t *testing.T) {
	remote := syncTestBareRemote(t)
	c := syncTestProject(t, remote, "awl")
	c.mustCall(t, []byte("# Accepted locally
"), "create", "Notes/keep.md", "--json")
	published := syncTestStatus(t, c, "published")
	rc, out, stderr := c.call(nil, "remote", "set-branch", "worklog/awl", "--json")
	if rc != 2 || stderr != "" || !bytes.Contains(out, []byte("REMOTE_BRANCH_MIGRATION_REQUIRED")) {
		t.Fatalf("changed verified authority: %d %s %s", rc, out, stderr)
	}
	assertRemoteBranch(t, c, remote, "awl")
	if current := syncTestTip(t, remote, "awl"); current != published.Commit {
		t.Fatalf("remote history changed: %q vs %q", current, published.Commit)
	}
	if string(c.read(t, "Notes/keep.md")) != "# Accepted locally
" {
		t.Fatal("local document changed after rejected branch switch")
	}
	// Reusing the existing target is always safe, even after verification.
	if got := c.mustCall(t, nil, "remote", "set-branch", "awl", "--json"); got.Status != "remote_branch_unchanged" {
		t.Fatalf("same verified branch should remain idempotent: %+v", got)
	}
}

func TestCLIContractRemoteSetBranchRejectsMissingManagedStore(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--remote", "git@example.invalid:team/worklogs.git", "--json")
	shared, local := remoteBranchPaths(c)
	bind, err := parseLocal(local)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(bind.LocalRepo, bind.LocalRepo+"-moved"); err != nil {
		t.Fatal(err)
	}
	rc, out, _ := c.call(nil, "remote", "set-branch", "worklog/awl", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("MANAGED_REPO_MISSING")) {
		t.Fatalf("missing managed store was silently replaced: %d %q", rc, out)
	}
	for _, path := range []string{shared, local} {
		body, err := os.ReadFile(path)
		if err != nil || strings.Contains(string(body), "remoteBranch:") {
			t.Fatalf("modified binding despite missing managed store: %s %v", path, err)
		}
	}
}
