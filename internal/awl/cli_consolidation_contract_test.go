package awl

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCLIContractJSONErrorOutputAcrossArgumentOrders(t *testing.T) {
	c := fixture(t)
	for _, args := range [][]string{
		{"init", "--unknown", "--json"},
		{"apply", "--unknown", "--json"},
		{"read", "Specs/entry.md", "--unknown", "--json"},
		{"ls", "Specs", "--unknown", "--json"},
		{"create", "Specs/entry.md", "--unknown", "--json"},
		{"remote", "add", "git@example.invalid:demo.git", "--unknown", "--json"},
	} {
		rc, out, stderr := c.call(nil, args...)
		if rc != 2 || stderr != "" {
			t.Errorf("awl %v should return JSON usage error on stdout: exit=%d stdout=%q stderr=%q", args, rc, out, stderr)
			continue
		}
		var got result
		if err := json.Unmarshal(out, &got); err != nil {
			t.Errorf("awl %v returned non-JSON error: %q (%v)", args, out, err)
			continue
		}
		if got.Status != "error" || got.Error == nil || got.Error.Code != "USAGE" {
			t.Errorf("awl %v returned unexpected error: %+v", args, got)
		}
	}

	// A token used as an option value is not itself a --json request.
	rc, out, stderr := c.call(nil, "apply", "--input", "--json", "--unknown")
	if rc != 2 || len(out) != 0 || !strings.Contains(stderr, "USAGE") {
		t.Fatalf("value that resembles a flag was treated as JSON opt-in: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	rc, out, stderr = c.call(nil, "init", "--unknown", "--json=false")
	if rc != 2 || len(out) != 0 || !strings.Contains(stderr, "USAGE") {
		t.Fatalf("explicit --json=false should use text errors: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	if _, err := os.Lstat(filepath.Join(c.Source, ".awl")); !os.IsNotExist(err) {
		t.Fatalf("failed CLI parsing should not initialize source project: %v", err)
	}
}

func TestCLIContractReadAndListAgreeOnCommittedSnapshot(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	body := []byte("---\r\nstatus: active\r\n---\r\nA [[wikilink]] and unicode β\r\n")
	write := c.mustCall(t, body, "create", "Activities/entry.md", "--json")
	if write.Commit == "" {
		t.Fatalf("create response has no commit: %+v", write)
	}
	listed := mustListCLI(t, c, "ls", "--json")
	if !reflect.DeepEqual(listed.Paths, []string{"Activities/entry.md"}) || listed.Revision != write.Commit {
		t.Fatalf("listing disagrees with successful create: list=%+v write=%+v", listed, write)
	}

	nested := c
	nested.Source = filepath.Join(c.Source, "deep", "nested")
	if err := os.MkdirAll(nested.Source, 0700); err != nil {
		t.Fatal(err)
	}
	nestedList := mustListCLI(t, nested, "ls", "Activities", "--json")
	if !reflect.DeepEqual(nestedList, listed) {
		t.Fatalf("nested project context changed listing: parent=%+v nested=%+v", listed, nestedList)
	}

	rc, out, stderr := nested.call(nil, "read", "Activities/entry.md", "--json")
	if rc != 0 || stderr != "" {
		t.Fatalf("JSON read failed: exit=%d output=%q stderr=%q", rc, out, stderr)
	}
	var read struct {
		Status   string `json:"status"`
		Project  string `json:"project"`
		Revision string `json:"revision"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(out, &read); err != nil {
		t.Fatalf("invalid JSON read response: %s: %v", out, err)
	}
	if read.Status != "ok" || read.Project != listed.Project ||
		read.Revision != listed.Revision || !bytes.Equal([]byte(read.Content), body) {
		t.Fatalf("read and ls disagree on project/revision/content: read=%+v list=%+v", read, listed)
	}
	if got := c.read(t, "Activities/entry.md"); !bytes.Equal(got, body) {
		t.Fatalf("raw read damaged user-owned Markdown: %q", got)
	}

	again := mustListCLI(t, c, "ls", "--json")
	if !reflect.DeepEqual(again, listed) {
		t.Fatalf("read/list should not mutate revision or paths: before=%+v after=%+v", listed, again)
	}
}
