package awl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type listCLIOutput struct {
	Status   string   `json:"status"`
	Project  string   `json:"project"`
	Revision string   `json:"revision"`
	Paths    []string `json:"paths"`
}

func mustListCLI(t *testing.T, c testCLI, args ...string) listCLIOutput {
	t.Helper()
	code, out, stderr := c.call(nil, args...)
	if code != 0 {
		t.Fatalf("awl %v: exit=%d output=%q stderr=%q", args, code, out, stderr)
	}
	var got listCLIOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("awl %v: invalid JSON %q: %v", args, out, err)
	}
	if got.Status != "ok" || got.Project != "demo" || got.Revision == "" || got.Paths == nil {
		t.Fatalf("awl %v: incomplete listing response: %+v", args, got)
	}
	return got
}

func TestCLIContractListOnlyCommittedManagedMarkdown(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")

	empty := mustListCLI(t, c, "ls", "--json")
	if len(empty.Paths) != 0 {
		t.Fatalf("new managed project should list no documents: %+v", empty)
	}

	// A source-worktree Markdown file must never appear in the managed listing.
	sourceDir := filepath.Join(c.Source, "Activities")
	if err := os.MkdirAll(sourceDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "source-only.md"), []byte("# Source only\n"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"Specs/summary.md", "Activities/z.md", "Activities/a.md"} {
		c.mustCall(t, []byte("# Managed\n"), "create", name, "--json")
	}
	want := []string{"Activities/a.md", "Activities/z.md", "Specs/summary.md"}
	all := mustListCLI(t, c, "ls", "--json")
	if !reflect.DeepEqual(all.Paths, want) {
		t.Fatalf("expected sorted managed paths only, got %q", all.Paths)
	}
	if all.Revision == empty.Revision {
		t.Fatal("document publication must change the visible managed revision")
	}

	code, text, stderr := c.call(nil, "ls")
	if code != 0 || string(text) != strings.Join(want, "\n")+"\n" || stderr != "" {
		t.Fatalf("text listing: exit=%d output=%q stderr=%q", code, text, stderr)
	}
	prefixWant := want[:2]
	for _, args := range [][]string{
		{"ls", "Activities", "--json"},
		{"ls", "Activities/", "--json"},
		{"ls", "--json", "Activities"},
		{"--project", "demo", "ls", "Activities", "--json"},
	} {
		got := mustListCLI(t, c, args...)
		if !reflect.DeepEqual(got.Paths, prefixWant) || got.Revision != all.Revision {
			t.Fatalf("prefix listing %v: %+v", args, got)
		}
	}
	if got := mustListCLI(t, c, "ls", "Activity", "--json"); len(got.Paths) != 0 {
		t.Fatalf("directory prefix must match full path segments: %+v", got)
	}
	if got := mustListCLI(t, c, "ls", "Specs", "--json"); !reflect.DeepEqual(got.Paths, []string{"Specs/summary.md"}) {
		t.Fatalf("Specs directory listing: %+v", got)
	}
	if after := mustListCLI(t, c, "ls", "--json"); after.Revision != all.Revision {
		t.Fatal("listing must not publish a new managed revision")
	}
}

func TestCLIContractListRevisionTracksEditsNotRemoteConfiguration(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	c.mustCall(t, []byte("Old body\n"), "create", "Activities/entry.md", "--json")
	before := mustListCLI(t, c, "ls", "--json")
	c.mustCall(t, []byte(`{"old":"Old body","new":"New body"}`), "edit", "Activities/entry.md", "--json")
	afterEdit := mustListCLI(t, c, "ls", "--json")
	if afterEdit.Revision == before.Revision || !reflect.DeepEqual(afterEdit.Paths, before.Paths) {
		t.Fatalf("edit should advance revision without changing the path list: before=%+v after=%+v", before, afterEdit)
	}
	c.mustCall(t, nil, "remote", "add", "git@example.invalid:demo.git", "--json")
	afterRemote := mustListCLI(t, c, "ls", "--json")
	if !reflect.DeepEqual(afterRemote, afterEdit) {
		t.Fatalf("remote configuration must not change managed document snapshot: before=%+v after=%+v", afterEdit, afterRemote)
	}
}

func TestCLIContractListRejectsUnsafeDirectoryPrefixes(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	c.mustCall(t, []byte("# Retained\n"), "create", "Activities/entry.md", "--json")
	before := mustListCLI(t, c, "ls", "--json")

	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"ls", "../Activities", "--json"}, "INVALID_PATH"},
		{[]string{"ls", "Activities/../Specs", "--json"}, "INVALID_PATH"},
		{[]string{"ls", "./Activities", "--json"}, "INVALID_PATH"},
		{[]string{"ls", "/tmp", "--json"}, "INVALID_PATH"},
		{[]string{"ls", ".git", "--json"}, "INVALID_PATH"},
		{[]string{"ls", ".awl", "--json"}, "INVALID_PATH"},
		{[]string{"ls", "Activities\\hidden", "--json"}, "INVALID_PATH"},
		{[]string{"ls", ".", "--json"}, "INVALID_PATH"},
		{[]string{"ls", "Activities//", "--json"}, "INVALID_PATH"},
		{[]string{"ls", "Activities", "Specs", "--json"}, "USAGE"},
		{[]string{"ls", "--unsupported", "--json"}, "USAGE"},
	} {
		rc, out, stderr := c.call(nil, tc.args...)
		if rc == 0 || !strings.Contains(string(out), tc.code) {
			t.Errorf("awl %v should fail with %s: exit=%d output=%q stderr=%q", tc.args, tc.code, rc, out, stderr)
		}
	}
	rc, out, stderr := c.call(nil, "--project", "other", "ls", "--json")
	if rc == 0 || !strings.Contains(string(out), "PROJECT_NOT_RESOLVED") {
		t.Fatalf("mismatched project selection accepted: exit=%d output=%q stderr=%q", rc, out, stderr)
	}
	if after := mustListCLI(t, c, "ls", "--json"); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected listings changed managed state: before=%+v after=%+v", before, after)
	}
	if string(c.read(t, "Activities/entry.md")) != "# Retained\n" {
		t.Fatal("listing altered document content")
	}
}

func TestCLIContractListBeforeInitDoesNotBootstrap(t *testing.T) {
	c := fixture(t)
	rc, out, stderr := c.call(nil, "ls", "--json")
	if rc == 0 || !strings.Contains(string(out), "PROJECT_NOT_INITIALIZED") {
		t.Fatalf("ls before init: exit=%d output=%q stderr=%q", rc, out, stderr)
	}
	for _, path := range []string{filepath.Join(c.Source, ".awl"), c.Data} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("ls created configuration or data at %q: %v", path, err)
		}
	}
}
