package awl

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These cases exercise the awl executable and observable files, without
// inspecting internal managed Git refs, objects, or storage implementation.
func TestCLIContractInitExplicitRepoDirAndReuse(t *testing.T) {
	c := fixture(t)
	parent := filepath.Join(filepath.Dir(c.Source), "worklog storage")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(parent, "demo.git")
	created := c.mustCall(t, nil, "init", "--slug", "demo", "--repo-dir", dest, "--json")
	if created.Status != "initialized" || created.Project != "demo" {
		t.Fatalf("unexpected init: %+v", created)
	}
	info, err := os.Lstat(dest)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("explicit store was not created as a real directory: %v, %v", info, err)
	}
	if _, err := os.Lstat(filepath.Join(c.Data, "awl", "repos")); !os.IsNotExist(err) {
		t.Fatalf("explicit init unexpectedly created default store: %v", err)
	}
	local := filepath.Join(c.Source, ".git", "awl", "local.yaml")
	cfg, err := os.ReadFile(local)
	if err != nil || !bytes.Contains(cfg, []byte("localRepo: "+dest+"\n")) {
		t.Fatalf("canonical project binding must name exact store: %v %q", err, cfg)
	}

	body := []byte("---\nstatus: active\n---\n# Managed\n")
	c.mustCall(t, body, "create", "Activities/one.md", "--json")
	nested := c
	nested.Source = filepath.Join(c.Source, "nested", "agent")
	if err := os.MkdirAll(nested.Source, 0700); err != nil {
		t.Fatal(err)
	}
	if got := nested.read(t, "Activities/one.md"); !bytes.Equal(got, body) {
		t.Fatalf("nested context did not resolve chosen store: %q", got)
	}
	again := nested.mustCall(t, nil, "init", "--repo-dir", dest, "--json")
	if again.Status != "initialized" {
		t.Fatalf("reinit with same directory failed: %+v", again)
	}
	withoutDir := c.mustCall(t, nil, "init", "--json")
	if withoutDir.Status != "initialized" || string(c.read(t, "Activities/one.md")) != string(body) {
		t.Fatalf("reinit should reuse existing binding: %+v", withoutDir)
	}
	cfgAgain, err := os.ReadFile(local)
	if err != nil || !bytes.Equal(cfg, cfgAgain) {
		t.Fatalf("reinit changed binding unexpectedly: %v %q", err, cfgAgain)
	}
}

func TestCLIContractInitExplicitRepoDirRejectsExistingAndUnsafeTargets(t *testing.T) {
	for _, tc := range []struct {
		name string
		target func(testCLI, string) (string, error)
		wantCode string
	}{
		{"relative", func(c testCLI, root string) (string, error) {
			return "relative/demo.git", nil
		}, "INVALID_REPO_DIR"},
		{"noncanonical", func(c testCLI, root string) (string, error) {
			return root + "/./demo.git", nil
		}, "INVALID_REPO_DIR"},
		{"nonexistent parent", func(c testCLI, root string) (string, error) {
			return filepath.Join(root, "absent", "demo.git"), nil
		}, "INVALID_REPO_DIR"},
		{"existing directory", func(c testCLI, root string) (string, error) {
			dest := filepath.Join(root, "existing.git")
			if err := os.Mkdir(dest, 0700); err != nil { return "", err }
			if err := os.WriteFile(filepath.Join(dest, "untouched.txt"), []byte("Keep me"), 0600); err != nil { return "", err }
			return dest, nil
		}, "REPO_DIR_EXISTS"},
		{"existing file", func(c testCLI, root string) (string, error) {
			dest := filepath.Join(root, "file.git")
			return dest, os.WriteFile(dest, []byte("Keep me"), 0600)
		}, "REPO_DIR_EXISTS"},
		{"symlink parent", func(c testCLI, root string) (string, error) {
			link := filepath.Join(root, "shortcut")
			if err := os.Symlink(root, link); err != nil { return "", err }
			return filepath.Join(link, "demo.git"), nil
		}, "INVALID_REPO_DIR"},
		{"source worktree", func(c testCLI, root string) (string, error) {
			return filepath.Join(c.Source, "managed.git"), nil
		}, "INVALID_REPO_DIR"},
		{"unsupported yaml scalar", func(c testCLI, root string) (string, error) {
			return filepath.Join(root, "unsafe#name.git"), nil
		}, "INVALID_REPO_DIR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixture(t)
			root := filepath.Join(filepath.Dir(c.Source), "explicit")
			if err := os.Mkdir(root, 0700); err != nil { t.Fatal(err) }
			dest, err := tc.target(c, root)
			if err != nil { t.Fatal(err) }
			rc, stdout, stderr := c.call(nil, "init", "--slug", "demo", "--repo-dir", dest, "--json")
			if rc != 2 || stderr != "" { t.Fatalf("unexpected error mode: exit=%d out=%q err=%q", rc, stdout, stderr) }
			var got result
			if err := json.Unmarshal(stdout, &got); err != nil || got.Error == nil || got.Error.Code != tc.wantCode {
				t.Fatalf("expected %s: exit=%d out=%q parse=%v", tc.wantCode, rc, stdout, err)
			}
			if _, err := os.Lstat(filepath.Join(c.Source, ".awl")); !os.IsNotExist(err) {
				t.Fatalf("rejected target should not initialize source config: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(c.Data, "awl", "repos")); !os.IsNotExist(err) {
				t.Fatalf("rejected target should not create default store: %v", err)
			}
			if tc.name == "existing file" || tc.name == "existing directory" {
				keep := dest
				if tc.name == "existing directory" { keep = filepath.Join(dest, "untouched.txt") }
				if b, err := os.ReadFile(keep); err != nil || string(b) != "Keep me" {
					t.Fatalf("existing target was overwritten: %q: %v", b, err)
				}
			}
		})
	}
}

func TestCLIContractInitExplicitRepoDirCannotRebindExistingProject(t *testing.T) {
	c := fixture(t)
	parent := filepath.Join(filepath.Dir(c.Source), "stores")
	if err := os.Mkdir(parent, 0700); err != nil { t.Fatal(err) }
	first := filepath.Join(parent, "first.git")
	second := filepath.Join(parent, "second.git")
	c.mustCall(t, nil, "init", "--slug", "demo", "--repo-dir", first, "--json")
	original := []byte("# Retained\n")
	c.mustCall(t, original, "create", "Specs/keep.md", "--json")
	rc, out, stderr := c.call(nil, "init", "--repo-dir", second, "--json")
	if rc != 2 || stderr != "" || !strings.Contains(string(out), "PROJECT_CONFLICT") {
		t.Fatalf("rebind unexpectedly accepted: exit=%d output=%q stderr=%q", rc, out, stderr)
	}
	if _, err := os.Lstat(second); !os.IsNotExist(err) {
		t.Fatalf("rejected rebind created different store: %v", err)
	}
	if got := c.read(t, "Specs/keep.md"); !bytes.Equal(got, original) {
		t.Fatalf("rebind changed managed document: %q", got)
	}
}

func TestCLIContractInitDefaultLocationRemainsSupported(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	matches, err := filepath.Glob(filepath.Join(c.Data, "awl", "repos", "demo-*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("default location was changed: %v %v", err, matches)
	}
	c.mustCall(t, []byte("# Kept\n"), "create", "Activities/kept.md", "--json")
	if got := c.read(t, "Activities/kept.md"); string(got) != "# Kept\n" {
		t.Fatalf("default initialization changed: %q", got)
	}
}
