package awl

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIContractInitNoUpdateGitignoreLeavesSourceUntouched(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--no-update-gitignore", "--json")
	if _, err := os.Lstat(filepath.Join(c.Source, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf("opt-out init must not create .gitignore: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(c.Source, ".awl", "local.yaml")); !os.IsNotExist(err) {
		t.Fatalf("opt-out init must not create an optional local symlink: %v", err)
	}
	if got := c.git(t, "ls-files", "--others", "--exclude-standard", "--", ".awl/project.yaml"); got != ".awl/project.yaml" {
		t.Fatalf("shared project configuration must remain versionable: %q", got)
	}
}

func TestCLIContractInitDefaultReinitUpdatesPreviouslySkippedGitignore(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--no-update-gitignore", "--json")
	c.mustCall(t, nil, "init", "--no-update-gitignore", "--json")
	path := filepath.Join(c.Source, ".gitignore")
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("opt-out reinit must leave .gitignore absent: %v", err)
	}
	c.mustCall(t, nil, "init", "--json")
	content, err := os.ReadFile(path)
	if err != nil || string(content) != ".awl/local.yaml\n" {
		t.Fatalf("default reinit must install ignore rule: %v %q", err, content)
	}
}

func TestCLIContractInitGitignoreDefaultCreationIsIdempotent(t *testing.T) {
	c := fixture(t)
	first := c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	if first.Status != "initialized" {
		t.Fatalf("unexpected init result: %+v", first)
	}
	ignore := filepath.Join(c.Source, ".gitignore")
	expected := []byte(".awl/local.yaml\n")
	got, err := os.ReadFile(ignore)
	if err != nil || !bytes.Equal(got, expected) {
		t.Fatalf("unexpected new .gitignore: %v %q", err, got)
	}
	c.mustCall(t, nil, "init", "--json")
	got, err = os.ReadFile(ignore)
	if err != nil || !bytes.Equal(got, expected) {
		t.Fatalf("reinit duplicated or changed ignore rule: %v %q", err, got)
	}
	if got := c.git(t, "check-ignore", ".awl/local.yaml"); got != ".awl/local.yaml" {
		t.Fatalf("local entry point must be ignored: %q", got)
	}
	if got := c.git(t, "ls-files", "--others", "--exclude-standard", "--", ".awl/project.yaml"); got != ".awl/project.yaml" {
		t.Fatalf("shared project configuration must remain versionable: %q", got)
	}
}

func TestCLIContractInitPreservesGitignoreContentAndLineEndings(t *testing.T) {
	c := fixture(t)
	ignore := filepath.Join(c.Source, ".gitignore")
	initial := []byte("# existing rules\r\nbuild/\r\n# tail without newline")
	if err := os.WriteFile(ignore, initial, 0600); err != nil {
		t.Fatal(err)
	}
	c.mustCall(t, nil, "init", "--json")
	expected := append(append([]byte{}, initial...), []byte("\r\n.awl/local.yaml\r\n")...)
	got, err := os.ReadFile(ignore)
	if err != nil || !bytes.Equal(got, expected) {
		t.Fatalf("source .gitignore content was not preserved: %v %q", err, got)
	}
	c.mustCall(t, nil, "init", "--json")
	got, err = os.ReadFile(ignore)
	if err != nil || !bytes.Equal(got, expected) {
		t.Fatalf("second init modified .gitignore: %v %q", err, got)
	}
}

func TestCLIContractInitDoesNotChangeGitignoreWhenAlreadyIgnored(t *testing.T) {
	c := fixture(t)
	exclude := filepath.Join(c.Source, ".git", "info", "exclude")
	if err := os.WriteFile(exclude, []byte(".awl/local.yaml\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c.mustCall(t, nil, "init", "--json")
	if _, err := os.Lstat(filepath.Join(c.Source, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf("effective Git exclusion must not create .gitignore: %v", err)
	}
}

func TestCLIContractInitDefaultGitignoreRepairsUnignoredLocalLink(t *testing.T) {
	c := fixture(t)
	if err := os.Mkdir(filepath.Join(c.Source, ".awl"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(c.Source, ".git", "awl"), 0700); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(c.Source, ".git", "awl", "local.yaml")
	if err := os.Symlink(canonical, filepath.Join(c.Source, ".awl", "local.yaml")); err != nil {
		t.Skipf("symlinks unavailable on this system: %v", err)
	}
	code, output, stderr := c.call(nil, "init", "--slug", "demo", "--no-update-gitignore", "--json")
	if code != 2 || stderr != "" || !bytes.Contains(output, []byte("LOCAL_CONFIG_NOT_IGNORED")) {
		t.Fatalf("unsafe opt-out init accepted: exit=%d output=%q stderr=%q", code, output, stderr)
	}
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	c.mustCall(t, nil, "init", "--json")
	if _, err := os.Stat(canonical); err != nil {
		t.Fatalf("missing canonical local binding after repair: %v", err)
	}
}

func TestCLIContractInitDefaultGitignoreRejectsUnsafeTarget(t *testing.T) {
	c := fixture(t)
	other := filepath.Join(filepath.Dir(c.Source), "untouched.txt")
	if err := os.WriteFile(other, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(c.Source, ".gitignore")); err != nil {
		t.Skipf("symlinks unavailable on this system: %v", err)
	}
	code, output, stderr := c.call(nil, "init", "--json")
	if code != 2 || stderr != "" || !bytes.Contains(output, []byte("GITIGNORE_CONFLICT")) {
		t.Fatalf("unsafe .gitignore was accepted: exit=%d output=%q stderr=%q", code, output, stderr)
	}
	got, err := os.ReadFile(other)
	if err != nil || string(got) != "keep" {
		t.Fatalf("symlink target must remain untouched: %v %q", err, got)
	}
}
