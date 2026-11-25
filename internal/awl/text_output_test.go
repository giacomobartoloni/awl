package awl

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCLIContractAgentTextOutcomes(t *testing.T) {
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	body := []byte("# First\n\nOriginal text.\n")
	rc, out, stderr := c.call(body, "create", "Notes/first.md", "--request-id", "create-first")
	if rc != 0 || stderr != "" {
		t.Fatalf("text create: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	for _, want := range []string{"status: applied\n", "project: \"demo\"\n", "request_id: \"create-first\"\n", "local_commit: ", "document_tree: ", "create \"Notes/first.md\""} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	rc, replay, stderr := c.call(body, "create", "Notes/first.md", "--request-id", "create-first")
	if rc != 0 || stderr != "" || !strings.Contains(string(replay), "replayed: true\n") {
		t.Fatalf("text replay: exit=%d stdout=%q stderr=%q", rc, replay, stderr)
	}
	if strings.Replace(string(replay), "replayed: true\n", "", 1) != string(out) {
		t.Fatalf("replay changed original outcome: original=%q replay=%q", out, replay)
	}
	payload := []byte(`{"request_id":"batch-1","operations":[{"id":"duplicate","op":"create","path":"Notes/first.md","content":"not written"},{"id":"new","op":"create","path":"Notes/second.md","content":"# Second"}]}`)
	rc, out, stderr = c.call(payload, "apply")
	if rc != 3 || stderr != "" {
		t.Fatalf("partial text apply: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	got := string(out)
	for _, want := range []string{"status: partial\n", "request_id: \"batch-1\"\n", "operations: 2\n", "failed \"duplicate\" create \"Notes/first.md\"", "DOCUMENT_EXISTS:", "applied \"new\" create \"Notes/second.md\""} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Index(got, "failed \"duplicate\"") >= strings.Index(got, "applied \"new\"") {
		t.Fatalf("operation order changed: %q", got)
	}
	rc, out, stderr = c.call(payload, "apply", "--json")
	var decoded result
	if rc != 3 || stderr != "" || json.Unmarshal(out, &decoded) != nil || decoded.Status != "partial" || !decoded.Replayed || len(decoded.Members) != 2 || decoded.Members[0].Status != "failed" || decoded.Members[1].Status != "applied" {
		t.Fatalf("JSON replay: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	rc, out, stderr = c.call(nil, "read", "Notes/first.md")
	if rc != 0 || stderr != "" || !bytes.Equal(out, body) {
		t.Fatalf("raw read: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
	rc, out, stderr = c.call(nil, "ls", "Notes")
	if rc != 0 || stderr != "" || string(out) != "Notes/first.md\nNotes/second.md\n" {
		t.Fatalf("plain list: exit=%d stdout=%q stderr=%q", rc, out, stderr)
	}
}

func TestWriteTextResultRemoteAndEscaping(t *testing.T) {
	var out bytes.Buffer
	writeTextResult(&out, result{
		Status:       "failed",
		Project:      "demo",
		RequestID:    "request\nstatus: applied",
		Commit:       "local",
		Tree:         "tree",
		RemoteBranch: "worklogs/demo",
		RemoteCommit: "remote",
		Members: []member{{
			ID:     "move",
			Op:     "move",
			Path:   "Notes/a.md",
			To:     "Notes/b.md",
			Status: "failed",
			Error:  &diagnostic{Code: "DOCUMENT_EXISTS", Message: "already exists\n\x1b[31m"},
		}},
	})
	want := "status: failed\nproject: \"demo\"\nrequest_id: \"request\\nstatus: applied\"\nlocal_commit: local\ndocument_tree: tree\nremote_branch: \"worklogs/demo\"\nremote_commit: remote\n\noperations: 1\n  failed \"move\" move \"Notes/a.md\" -> \"Notes/b.md\"\n    DOCUMENT_EXISTS: \"already exists\\n\\x1b[31m\"\n"
	if got := out.String(); got != want {
		t.Fatalf("text result mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}
