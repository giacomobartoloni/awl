package awl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Rename the fixture source repository so the public CLI derives the project
// identity from the actual repository name instead of the fixture's "source".
func renamedSourceRepo(t *testing.T, name string) testCLI {
	t.Helper()
	c := fixture(t)
	path := filepath.Join(filepath.Dir(c.Source), name)
	if err := os.Rename(c.Source, path); err != nil {
		t.Fatal(err)
	}
	c.Source = path
	return c
}

func TestCLIContractInitDerivesSlugFromSourceRepository(t *testing.T) {
	tests := []struct {
		repository string
		slug       string
	}{
		{"My.Project Repo", "my-project-repo"},
		{"Geco82_awl CLI", "geco82-awl-cli"},
		{"123 Project!", "repo-123-project"},
		{"--Hello___WORLD--", "hello-world"},
		{strings.Repeat("A", 80), strings.Repeat("a", 64)},
	}
	for _, tc := range tests {
		t.Run(tc.slug, func(t *testing.T) {
			c := renamedSourceRepo(t, tc.repository)
			res := c.mustCall(t, nil, "init", "--json")
			if res.Status != "initialized" || res.Project != tc.slug {
				t.Fatalf("derived project mismatch: %+v, want %q", res, tc.slug)
			}
			shared, err := os.ReadFile(filepath.Join(c.Source, ".awl", "project.yaml"))
			if err != nil || !bytes.Contains(shared, []byte("  slug: "+tc.slug+"\n")) {
				t.Fatalf("source project binding is wrong: %v %q", err, shared)
			}
			local, err := os.ReadFile(filepath.Join(c.Source, ".git", "awl", "local.yaml"))
			if err != nil || !bytes.Contains(local, []byte("  slug: "+tc.slug+"\n")) {
				t.Fatalf("canonical local binding is wrong: %v %q", err, local)
			}
			matches, err := filepath.Glob(filepath.Join(c.Data, "awl", "repos", tc.slug+"-*"))
			if err != nil || len(matches) != 1 {
				t.Fatalf("unexpected managed store location: %v %v", err, matches)
			}
			content := []byte("# No explicit slug\n")
			c.mustCall(t, content, "create", "Activities/first.md", "--json")
			if got := c.read(t, "Activities/first.md"); !bytes.Equal(got, content) {
				t.Fatalf("created Markdown was changed: %q", got)
			}
		})
	}
}

func TestCLIContractInitDerivedSlugWorksWithExplicitRepoDir(t *testing.T) {
	c := renamedSourceRepo(t, "My.Awl Tool")
	parent := filepath.Join(filepath.Dir(c.Source), "local stores")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(parent, "custom.git")
	init := c.mustCall(t, nil, "init", "--repo-dir", dest, "--json")
	if init.Project != "my-awl-tool" || init.Status != "initialized" {
		t.Fatalf("init with derived slug and explicit store: %+v", init)
	}
	if stat, err := os.Lstat(dest); err != nil || !stat.IsDir() {
		t.Fatalf("missing custom managed Git repository: %v %v", stat, err)
	}
	if _, err := os.Lstat(filepath.Join(c.Data, "awl", "repos")); !os.IsNotExist(err) {
		t.Fatalf("custom path must not create the default store: %v", err)
	}

	// Explicit --slug continues to override the inferred name on first init.
	c2 := renamedSourceRepo(t, "Another.Repository")
	got := c2.mustCall(t, nil, "init", "--slug", "chosen", "--json")
	if got.Project != "chosen" {
		t.Fatalf("explicit slug was ignored: %+v", got)
	}
	again := c2.mustCall(t, nil, "init", "--json")
	if again.Project != "chosen" {
		t.Fatalf("reinitialization changed the existing explicit slug: %+v", again)
	}
}

func TestCLIContractInitWithoutSlugReusesBindingAcrossLinkedWorktrees(t *testing.T) {
	c := renamedSourceRepo(t, "My.Current Repo")
	first := c.mustCall(t, nil, "init", "--json")
	if first.Project != "my-current-repo" {
		t.Fatalf("unexpected initial slug: %+v", first)
	}
	c.git(t, "add", ".awl/project.yaml")
	c.git(t, "commit", "-q", "-m", "share project binding")
	linkedPath := filepath.Join(filepath.Dir(c.Source), "different-branch-name")
	c.git(t, "worktree", "add", "-q", "-b", "linked", linkedPath)
	linked := c
	linked.Source = linkedPath
	again := linked.mustCall(t, nil, "init", "--json")
	if again.Project != first.Project {
		t.Fatalf("linked worktree switched slug: parent=%+v linked=%+v", first, again)
	}
	data := []byte("# Shared repo\n")
	linked.mustCall(t, data, "create", "Specs/shared.md", "--json")
	if got := c.read(t, "Specs/shared.md"); !bytes.Equal(got, data) {
		t.Fatalf("linked worktree did not reuse one managed store: %q", got)
	}
}

func TestCLIContractInitRejectsUnusableDerivedSlugBeforeSetup(t *testing.T) {
	c := renamedSourceRepo(t, "---___ !!!")
	rc, out, stderr := c.call(nil, "init", "--json")
	if rc != 2 || stderr != "" || !bytes.Contains(out, []byte("INVALID_SLUG")) {
		t.Fatalf("expected structured invalid derived slug: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	if _, err := os.Lstat(filepath.Join(c.Source, ".awl")); !os.IsNotExist(err) {
		t.Fatalf("invalid derived slug created source configuration: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(c.Source, ".git", "awl")); !os.IsNotExist(err) {
		t.Fatalf("invalid derived slug created Git binding: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(c.Data, "awl")); !os.IsNotExist(err) {
		t.Fatalf("invalid derived slug created managed data: %v", err)
	}
	// Explicit --slug remains available for repositories with unsuitable names.
	valid := c.mustCall(t, nil, "init", "--slug", "chosen", "--json")
	if valid.Project != "chosen" {
		t.Fatalf("explicit slug failed as a fallback: %+v", valid)
	}
}

func TestCLIContractInitRejectsExplicitEmptySlug(t *testing.T) {
	c := renamedSourceRepo(t, "Can-Be-Derived")
	rc, out, stderr := c.call(nil, "init", "--slug", "", "--json")
	if rc != 2 || stderr != "" || !bytes.Contains(out, []byte("INVALID_SLUG")) {
		t.Fatalf("explicitly empty --slug must not silently derive: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	if _, err := os.Lstat(filepath.Join(c.Source, ".awl")); !os.IsNotExist(err) {
		t.Fatalf("explicit empty --slug created source configuration: %v", err)
	}
}

// First use from a linked worktree must not derive its arbitrary folder name.
func TestCLIContractInitDerivedSlugFromUninitializedLinkedWorktree(t *testing.T) {
	c := renamedSourceRepo(t, "Original.Repo")
	c.git(t, "commit", "-q", "--allow-empty", "-m", "fixture initial commit")
	linkedPath := filepath.Join(filepath.Dir(c.Source), "unrelated-feature-worktree")
	c.git(t, "worktree", "add", "-q", "-b", "linked", linkedPath)
	linked := c
	linked.Source = linkedPath

	got := linked.mustCall(t, nil, "init", "--json")
	if got.Status != "initialized" || got.Project != "original-repo" {
		t.Fatalf("first init from linked worktree used the wrong identity: %+v", got)
	}
	shared, err := os.ReadFile(filepath.Join(linkedPath, ".awl", "project.yaml"))
	if err != nil || !bytes.Contains(shared, []byte("  slug: original-repo\n")) {
		t.Fatalf("linked project configuration has wrong slug: %v %q", err, shared)
	}
}

func TestCLIContractInitDerivedSlugRespectsLeadingProjectSelector(t *testing.T) {
	c := renamedSourceRepo(t, "User.Worklog")
	rc, out, stderr := c.call(nil, "--project", "other", "init", "--json")
	if rc != 2 || stderr != "" || !bytes.Contains(out, []byte("PROJECT_CONFLICT")) {
		t.Fatalf("conflicting --project was accepted: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	if _, err := os.Lstat(filepath.Join(c.Source, ".awl")); !os.IsNotExist(err) {
		t.Fatalf("conflicting --project created configuration: %v", err)
	}
	got := c.mustCall(t, nil, "--project", "user-worklog", "init", "--json")
	if got.Project != "user-worklog" || got.Status != "initialized" {
		t.Fatalf("matching explicit project selector was rejected: %+v", got)
	}
}
