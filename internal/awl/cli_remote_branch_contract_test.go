package awl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func remoteBranchPaths(c testCLI) (string, string) {
	return filepath.Join(c.Source, ".awl", "project.yaml"),
		filepath.Join(c.Source, ".git", "awl", "local.yaml")
}

func assertRemoteBranch(t *testing.T, c testCLI, url, branch string) {
	t.Helper()
	shared, local := remoteBranchPaths(c)
	slug, actualURL, actualBranch, err := parseSharedTarget(shared)
	if err != nil || slug != "demo" || actualURL != url || actualBranch != branch {
		t.Fatalf("shared target: slug=%q remote=%q branch=%q err=%v", slug, actualURL, actualBranch, err)
	}
	bind, err := parseLocal(local)
	if err != nil || bind.Slug != "demo" || bind.Remote != url || bind.RemoteBranch != branch {
		t.Fatalf("canonical local target: %+v err=%v", bind, err)
	}
}

func TestCLIContractRemoteBranchDefaultsToLegacyAwl(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	url := "git@example.invalid:team/worklogs.git"
	if got := c.mustCall(t, nil, "remote", "add", url, "--json"); got.Status != "remote_added" {
		t.Fatalf("remote add: %+v", got)
	}
	assertRemoteBranch(t, c, url, "awl")
	shared, local := remoteBranchPaths(c)
	for _, path := range []string{shared, local} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(body, []byte("remoteBranch:")) {
			t.Fatalf("legacy config unexpectedly rewritten: %s: %q", path, body)
		}
	}
	if got := c.mustCall(t, nil, "remote", "add", url, "--branch", "awl", "--json"); got.Status != "remote_unchanged" {
		t.Fatalf("explicit legacy target: %+v", got)
	}
}

func TestCLIContractRemoteBranchCanBeChosenOnInit(t *testing.T) {
	c := fixture(t)
	url, branch := "git@example.invalid:team/worklogs.git", "worklogs/api"
	res := c.mustCall(t, nil, "init", "--slug", "demo", "--remote", url,
		"--remote-branch", branch, "--json")
	if res.Status != "initialized" {
		t.Fatalf("init: %+v", res)
	}
	assertRemoteBranch(t, c, url, branch)
	if got := c.mustCall(t, nil, "init", "--json"); got.Status != "initialized" {
		t.Fatalf("reinit did not reuse branch: %+v", got)
	}
	c.mustCall(t, []byte("# Keep me\n"), "create", "Notes/keep.md", "--json")
	rc, out, stderr := c.call(nil, "init", "--remote-branch", "worklogs/other", "--json")
	if rc != 2 || stderr != "" || !bytes.Contains(out, []byte("REMOTE_ALREADY_CONFIGURED")) {
		t.Fatalf("changed remote branch accepted: %d %s %s", rc, out, stderr)
	}
	assertRemoteBranch(t, c, url, branch)
	if string(c.read(t, "Notes/keep.md")) != "# Keep me\n" {
		t.Fatal("reinit failure damaged local worklog")
	}
}

func TestCLIContractRemoteBranchAddIsIdempotentAndImmutable(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	url, branch := "git@example.invalid:team/worklogs.git", "worklogs/api"
	original := []byte("# Keep me\n")
	write := c.mustCall(t, original, "create", "Notes/keep.md", "--json")
	res := c.mustCall(t, nil, "remote", "add", url, "--branch", branch, "--json")
	if res.Status != "remote_added" {
		t.Fatalf("remote add: %+v", res)
	}
	assertRemoteBranch(t, c, url, branch)
	for _, args := range [][]string{
		{"remote", "add", url, "--json"},
		{"remote", "add", url, "--branch", branch, "--json"},
		{"remote", "add", "--branch=" + branch, url, "--json"},
	} {
		if got := c.mustCall(t, nil, args...); got.Status != "remote_unchanged" {
			t.Fatalf("non-idempotent remote add %v: %+v", args, got)
		}
	}
	for _, args := range [][]string{
		{"remote", "add", url, "--branch", "worklogs/other", "--json"},
		{"remote", "add", "git@example.invalid:other.git", "--branch", branch, "--json"},
	} {
		rc, out, _ := c.call(nil, args...)
		if rc != 2 || !bytes.Contains(out, []byte("REMOTE_ALREADY_CONFIGURED")) {
			t.Fatalf("rebind accepted %v: %d %s", args, rc, out)
		}
	}
	assertRemoteBranch(t, c, url, branch)
	if got := c.read(t, "Notes/keep.md"); !bytes.Equal(got, original) {
		t.Fatalf("remote config changed document: %q", got)
	}
	bind, err := parseLocal(filepath.Join(c.Source, ".git", "awl", "local.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cmdOut, err := gitText(bind.LocalRepo, nil, nil, "rev-parse", managedRef)
	if err != nil || cmdOut != write.Commit {
		t.Fatalf("remote config changed managed Git history: %q %v", cmdOut, err)
	}
}

func TestCLIContractRemoteBranchRejectsInvalidNamesAndNoRemote(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	shared, local := remoteBranchPaths(c)
	beforeShared, err := os.ReadFile(shared)
	if err != nil {
		t.Fatal(err)
	}
	beforeLocal, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"", "-main", "refs/heads/team", "../escape", "worklogs//api",
		"worklogs/api.lock", "worklogs/a..b", "worklogs/a:b",
		"worklogs/api?", "@{-1}", "foo bar", "worklogs/a\ninvalid",
	} {
		rc, out, stderr := c.call(nil, "remote", "add", "git@example.invalid:worklogs.git", "--branch", name, "--json")
		if rc != 2 || stderr != "" || !bytes.Contains(out, []byte("INVALID_REMOTE_BRANCH")) {
			t.Errorf("invalid branch %q accepted or wrong error: exit=%d out=%q stderr=%q", name, rc, out, stderr)
		}
	}
	rc, out, _ := c.call(nil, "init", "--remote-branch", "worklogs/api", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("REMOTE_REQUIRED")) {
		t.Fatalf("branch without remote was accepted: %d %s", rc, out)
	}
	if after, _ := os.ReadFile(shared); !bytes.Equal(after, beforeShared) {
		t.Fatalf("invalid branch changed shared config: %q", after)
	}
	if after, _ := os.ReadFile(local); !bytes.Equal(after, beforeLocal) {
		t.Fatalf("invalid branch changed local config: %q", after)
	}
}

func TestCLIContractRemoteBranchRepairsInterruptedConfig(t *testing.T) {
	url, branch := "git@example.invalid:team/worklogs.git", "worklogs/api"
	for _, advanced := range []string{"shared", "local"} {
		t.Run(advanced, func(t *testing.T) {
			c := fixture(t)
			c.mustCall(t, nil, "init", "--slug", "demo", "--json")
			shared, local := remoteBranchPaths(c)
			target, section := shared, "storage"
			if advanced == "local" {
				target, section = local, "binding"
			}
			before, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			// Existing CRLF and comments must survive the interrupted update.
			if advanced == "shared" {
				before = bytes.ReplaceAll(before, []byte("\n"), []byte("\r\n"))
				before = append([]byte("# User comment\r\n"), before...)
			}
			halfway, err := withRemoteTarget(before, section, url, branch)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, halfway, 0600); err != nil {
				t.Fatal(err)
			}
			if got := c.mustCall(t, nil, "remote", "add", url, "--json"); got.Status != "remote_added" {
				t.Fatalf("failed to repair interrupted configuration: %+v", got)
			}
			assertRemoteBranch(t, c, url, branch)
			body, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(body, halfway) {
				t.Fatalf("repair rewrote already configured file: %s %q %v", target, body, err)
			}
			if got := c.mustCall(t, nil, "remote", "add", url, "--json"); got.Status != "remote_unchanged" {
				t.Fatalf("repair retry not idempotent: %+v", got)
			}
		})
	}
}

func TestCLIContractInitBranchWithoutURLHasNoSideEffects(t *testing.T) {
	c := fixture(t)
	rc, out, stderr := c.call(nil, "init", "--slug", "demo", "--remote-branch", "worklogs/demo", "--json")
	if rc != 2 || stderr != "" || !bytes.Contains(out, []byte("REMOTE_REQUIRED")) {
		t.Fatalf("branch-only init should fail: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	if _, err := os.Lstat(filepath.Join(c.Source, ".awl")); !os.IsNotExist(err) {
		t.Fatalf("invalid init created source configuration: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(c.Source, ".git", "awl")); !os.IsNotExist(err) {
		t.Fatalf("invalid init created canonical binding: %v", err)
	}
}

func TestCLIContractRemoteBranchDetectsProjectBindingMismatch(t *testing.T) {
	c := fixture(t)
	url := "git@example.invalid:team/worklogs.git"
	c.mustCall(t, nil, "init", "--slug", "demo", "--remote", url, "--remote-branch", "worklogs/api", "--json")
	shared, _ := remoteBranchPaths(c)
	b, err := os.ReadFile(shared)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shared, []byte(strings.Replace(string(b), "remoteBranch: worklogs/api", "remoteBranch: worklogs/web", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	rc, out, _ := c.call(nil, "ls", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("PROJECT_CONFLICT")) {
		t.Fatalf("mismatched branch did not fail closed: %d %s", rc, out)
	}
}
