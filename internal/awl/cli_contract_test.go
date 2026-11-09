package awl

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These black-box contract tests exercise the awl binary and its observable
// document/response behavior. They intentionally do not inspect Git objects,
// Git refs, commit messages, or storage implementation details.
func TestCLIContractLocalOnlyProjectWorkflow(t *testing.T) {
	c := fixture(t)
	initialized := c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	if initialized.Status != "initialized" || initialized.Project != "demo" {
		t.Fatalf("init without remote: %+v", initialized)
	}

	initial := []byte("---\r\nstatus: active\r\n---\r\nOriginal body\r\n")
	created := c.mustCall(t, initial, "create", "Specs/overview.md", "--json")
	if created.Status != "applied" || created.Project != "demo" || len(created.Members) != 1 {
		t.Fatalf("create response: %+v", created)
	}

	nested := c
	nested.Source = filepath.Join(c.Source, "src", "feature")
	if err := os.MkdirAll(nested.Source, 0700); err != nil {
		t.Fatal(err)
	}
	if got := nested.read(t, "Specs/overview.md"); !bytes.Equal(got, initial) {
		t.Fatalf("read from nested source directory changed bytes: %q", got)
	}

	edited := nested.mustCall(t, []byte(`{"old":"Original body","new":"Updated body"}`), "edit", "Specs/overview.md", "--json")
	if edited.Status != "applied" || edited.Project != "demo" {
		t.Fatalf("edit response: %+v", edited)
	}
	expected := []byte("---\r\nstatus: active\r\n---\r\nUpdated body\r\n")
	if got := c.read(t, "Specs/overview.md"); !bytes.Equal(got, expected) {
		t.Fatalf("edit changed frontmatter or line endings: %q", got)
	}

	added := c.mustCall(t, nil, "remote", "add", "git@example.invalid:demo.git", "--json")
	if added.Status != "remote_added" {
		t.Fatalf("add remote after local-only init: %+v", added)
	}
	unchanged := c.mustCall(t, nil, "remote", "add", "git@example.invalid:demo.git", "--json")
	if unchanged.Status != "remote_unchanged" {
		t.Fatalf("idempotent remote configuration: %+v", unchanged)
	}
	if got := nested.read(t, "Specs/overview.md"); !bytes.Equal(got, expected) {
		t.Fatalf("remote configuration changed local documents: %q", got)
	}
}

func TestCLIContractCompositePartialResultAndIdempotentRetry(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	c.mustCall(t, []byte("# Existing\n"), "create", "Activities/existing.md", "--json")

	request := []byte(`{"request_id":"cli-partial-01","operations":[{"id":"duplicate","op":"create","path":"Activities/existing.md","content":"Do not overwrite"},{"id":"new","op":"create","path":"Activities/new.md","content":"# New\n"}]}`)
	code, output, stderr := c.call(request, "apply", "--json")
	if code != 3 {
		t.Fatalf("expected distinct partial-composite exit status, got %d output=%s stderr=%s", code, output, stderr)
	}
	var got result
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("invalid structured composite response: %s: %v", output, err)
	}
	if got.Status != "partial" || got.RequestID != "cli-partial-01" || len(got.Members) != 2 {
		t.Fatalf("composite response missing member outcomes: %+v", got)
	}
	if got.Members[0].ID != "duplicate" || got.Members[0].Status != "failed" ||
		got.Members[0].Error == nil || got.Members[0].Error.Code != "DOCUMENT_EXISTS" ||
		got.Members[1].ID != "new" || got.Members[1].Status != "applied" {
		t.Fatalf("composite did not preserve member order and failure reason: %+v", got.Members)
	}
	if body := c.read(t, "Activities/existing.md"); string(body) != "# Existing\n" {
		t.Fatalf("existing document was overwritten: %q", body)
	}
	if body := c.read(t, "Activities/new.md"); string(body) != "# New\n" {
		t.Fatalf("successful member was lost: %q", body)
	}

	replayCode, replayOutput, replayStderr := c.call(request, "apply", "--json")
	var replay result
	if err := json.Unmarshal(replayOutput, &replay); err != nil {
		t.Fatalf("invalid replay response: %s: %v", replayOutput, err)
	}
	if replayCode != code || !replay.Replayed || replay.Status != got.Status || replay.RequestID != got.RequestID {
		t.Fatalf("retry should return original result, got exit=%d response=%+v stderr=%s", replayCode, replay, replayStderr)
	}
	if body := c.read(t, "Activities/new.md"); string(body) != "# New\n" {
		t.Fatalf("retry changed accepted document: %q", body)
	}
}

func TestCLIContractProjectAndInputErrorsAreObservable(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	for _, tc := range []struct {
		name, path, wantCode string
	}{
		{"parent traversal", "../outside.md", "INVALID_PATH"},
		{"not markdown", "Specs/document.txt", "INVALID_PATH"},
		{"reserved path", ".awl/project.yaml", "INVALID_PATH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rc, output, stderr := c.call([]byte("# Unsafe\n"), "create", tc.path, "--json")
			if rc == 0 || !bytes.Contains(output, []byte(tc.wantCode)) {
				t.Fatalf("unexpected create result: exit=%d output=%q stderr=%q", rc, output, stderr)
			}
		})
	}
	code, output, stderr := c.call([]byte("# Wrong project\n"), "--project", "other", "create", "Specs/wrong.md", "--json")
	if code == 0 || !bytes.Contains(output, []byte("PROJECT_NOT_RESOLVED")) {
		t.Fatalf("wrong implicit project accepted: exit=%d output=%q stderr=%q", code, output, stderr)
	}
	code, output, stderr = c.call(nil, "read", "Specs/wrong.md", "--json")
	if code == 0 || !strings.Contains(string(output), "DOCUMENT_NOT_FOUND") {
		t.Fatalf("rejected request changed project: exit=%d output=%q stderr=%q", code, output, stderr)
	}
}
