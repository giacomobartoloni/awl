package awl

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCLIContractMetadataInsertFlowAndAppend(t *testing.T) {
	c := fixture(t)
	c.init(t)
	path := "Activities/flow.md"
	before := []byte("---\r\ntags: [activity, awl] # preserve\r\nstatus: active\r\n---\r\nKeep [[the body]] unchanged.\r\n")
	c.mustCall(t, before, "create", path, "--json")

	add := c.mustCall(t, nil, "metadata", "insert", path, "/tags/1", "review", "--request-id", "insert-review-01", "--json")
	if add.Status != "applied" {
		t.Fatalf("indexed insert: %+v", add)
	}
	first := []byte("---\r\ntags: [activity, \"review\", awl] # preserve\r\nstatus: active\r\n---\r\nKeep [[the body]] unchanged.\r\n")
	if got := c.read(t, path); !bytes.Equal(got, first) {
		t.Fatalf("indexed flow insertion changed unrelated bytes: %q", got)
	}
	replay := c.mustCall(t, nil, "metadata", "insert", path, "/tags/1", "review", "--request-id", "insert-review-01", "--json")
	if !replay.Replayed || replay.Commit != add.Commit {
		t.Fatalf("request-ID replay: %+v", replay)
	}

	c.mustCall(t, nil, "metadata", "insert", path, "/tags/-", "done", "--json")
	want := []byte("---\r\ntags: [activity, \"review\", awl, \"done\"] # preserve\r\nstatus: active\r\n---\r\nKeep [[the body]] unchanged.\r\n")
	if got := c.read(t, path); !bytes.Equal(got, want) {
		t.Fatalf("flow append changed unrelated bytes: %q", got)
	}
	rc, _, _ := c.call(nil, "metadata", "get", path, "/tags/-", "--json")
	if rc != 2 {
		t.Fatal("/- must not be readable as an element")
	}
}

func TestCLIContractMetadataInsertBlockAndTypedValue(t *testing.T) {
	c := fixture(t)
	c.init(t)
	path := "Specs/block.md"
	before := []byte("---\nteam:\n  members:\n    - alice # owner\n    - bob\nother: keep\n---\n# Body\n")
	c.mustCall(t, before, "create", path, "--json")

	c.mustCall(t, nil, "metadata", "insert", path, "/team/members/1", "charlie", "--json")
	c.mustCall(t, []byte(`{"name":"Dana","active":true}`), "metadata", "insert", path, "/team/members/-", "--input", "-", "--json")
	want := []byte("---\nteam:\n  members:\n    - alice # owner\n    - \"charlie\"\n    - bob\n    - {\"name\":\"Dana\",\"active\":true}\nother: keep\n---\n# Body\n")
	if got := c.read(t, path); !bytes.Equal(got, want) {
		t.Fatalf("block insertion changed unrelated bytes: %q", got)
	}
	rc, out, _ := c.call(nil, "metadata", "get", path, "/team/members/3/name", "--json")
	if rc != 0 || !bytes.Contains(out, []byte(`"value":"Dana"`)) {
		t.Fatalf("nested typed insert: exit=%d payload=%s", rc, out)
	}
}

func TestCLIContractMetadataInsertEmptyListAndInvalidPositions(t *testing.T) {
	c := fixture(t)
	c.init(t)
	path := "Activities/empty-list.md"
	before := []byte("---\ntags: [] # keep\nstatus: active\n---\nBody\n")
	c.mustCall(t, before, "create", path, "--json")
	c.mustCall(t, nil, "metadata", "insert", path, "/tags/-", "first", "--json")
	c.mustCall(t, nil, "metadata", "insert", path, "/tags/1", "second", "--json")
	want := []byte("---\ntags: [\"first\", \"second\"] # keep\nstatus: active\n---\nBody\n")
	if got := c.read(t, path); !bytes.Equal(got, want) {
		t.Fatalf("empty flow insertion failed: %q", got)
	}
	for _, pointer := range []string{"/tags/01", "/tags/-1", "/tags/3", "/status/-", "/nonexistent/-", "/tags/x"} {
		rc, _, _ := c.call(nil, "metadata", "insert", path, pointer, "bad", "--json")
		if rc != 2 {
			t.Fatalf("unsafe insertion succeeded for %q", pointer)
		}
		if got := c.read(t, path); !bytes.Equal(got, want) {
			t.Fatalf("rejected insertion changed document for %q", pointer)
		}
	}
}

func TestCLIContractMetadataInsertIsAtomicWithinOneOperation(t *testing.T) {
	c := fixture(t)
	c.init(t)
	path := "Activities/insert-atomic.md"
	before := []byte("---\ntags: [a, b]\nstatus: active\n---\nBody\n")
	c.mustCall(t, before, "create", path, "--json")
	payload := []byte(`{"request_id":"atomic-sequence","operations":[{"id":"metadata-1","op":"metadata","path":"Activities/insert-atomic.md","metadata":[{"op":"insert","pointer":"/tags/1","value":"new"},{"op":"set","pointer":"/missing","value":"changed"}]}]}`)
	rc, out, stderr := c.call(payload, "apply", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("METADATA_TARGET_NOT_FOUND")) {
		t.Fatalf("atomic metadata operation expected failure: %d %s %s", rc, out, stderr)
	}
	if got := c.read(t, path); !bytes.Equal(got, before) {
		t.Fatalf("failed metadata insert partially wrote: %q", got)
	}
}

func TestMetadataInsertRejectsUnsafeSequenceLayouts(t *testing.T) {
	for _, raw := range []string{
		"---\nitems: [a,\n b]\n---\nBody\n",
		"---\nitems: [[a], b]\n---\nBody\n",
		"---\nitems:\n  - name: Alex\n  - name: Bob\n---\nBody\n",
		"---\nitems:\n  - first\n    continued\n  - second\n---\nBody\n",
	} {
		doc := []byte(raw)
		updated, err := metadataApply(doc, []metadataEdit{{Action: "insert", Pointer: "/items/-", Value: json.RawMessage(`"new"`)}})
		if err == nil || updated != nil {
			t.Fatalf("unsupported list layout was accepted: %q => %q", raw, updated)
		}
	}
}

func TestMetadataInsertNeverRebasesAfterGitCASConflict(t *testing.T) {
	doc := []byte("---\ntags: [a, b]\n---\nBody\n")
	for _, pointer := range []string{"/tags/1", "/tags/-"} {
		edits := []metadataEdit{{Action: "insert", Pointer: pointer, Value: json.RawMessage(`"new"`)}}
		if metadataTargetsStable(doc, doc, edits) {
			t.Fatalf("sequence insertion at %s would be rebased", pointer)
		}
	}
}
