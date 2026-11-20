package awl

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCLIContractMovePreservesBytesLinksAndAudit(t *testing.T) {
	c := fixture(t)
	c.init(t)
	document := []byte("---\r\nstatus: active\r\n---\r\nSee [[Notes/other]] and [relative](../Notes/other.md).\r\n")
	backlinks := []byte("[source](../Notes/source.md)\n[[Notes/source]]\n")
	created := c.mustCall(t, document, "create", "Notes/source.md", "--json")
	c.mustCall(t, backlinks, "create", "Notes/links.md", "--json")

	moved := c.mustCall(t, nil, "move", "Notes/source.md", "Plans/source.md", "--request-id", "move-001", "--json")
	if moved.Tree == created.Tree || len(moved.Members) != 1 ||
		moved.Members[0].Status != "applied" || moved.Members[0].Op != "move" ||
		moved.Members[0].Path != "Notes/source.md" || moved.Members[0].To != "Plans/source.md" {
		t.Fatalf("unexpected move result: %+v", moved)
	}
	if got := c.read(t, "Plans/source.md"); !bytes.Equal(got, document) {
		t.Fatalf("move rewrote document bytes: %q", got)
	}
	if got := c.read(t, "Notes/links.md"); !bytes.Equal(got, backlinks) {
		t.Fatalf("move rewrote backlinks: %q", got)
	}
	if rc, out, _ := c.call(nil, "read", "Notes/source.md", "--json"); rc != 2 || !bytes.Contains(out, []byte("DOCUMENT_NOT_FOUND")) {
		t.Fatalf("source still available: %d %s", rc, out)
	}
	replayed := c.mustCall(t, nil, "move", "Notes/source.md", "Plans/source.md", "--request-id", "move-001", "--json")
	if !replayed.Replayed || replayed.Commit != moved.Commit || replayed.Tree != moved.Tree {
		t.Fatalf("move replay created a new commit: %+v vs %+v", replayed, moved)
	}
}

func TestCLIContractMoveRejectsCollisionAndInvalidPaths(t *testing.T) {
	c := fixture(t)
	c.init(t)
	c.mustCall(t, []byte("source"), "create", "Notes/source.md", "--json")
	last := c.mustCall(t, []byte("destination"), "create", "Plans/destination.md", "--json")

	code, output, _ := c.call(nil, "move", "Notes/source.md", "Plans/destination.md", "--request-id", "move-collision", "--json")
	var collided result
	if err := json.Unmarshal(output, &collided); err != nil {
		t.Fatal(err)
	}
	if code != 2 || collided.Status != "failed" || collided.Tree != last.Tree ||
		len(collided.Members) != 1 || collided.Members[0].Error == nil ||
		collided.Members[0].Error.Code != "DOCUMENT_EXISTS" || collided.Commit == last.Commit {
		t.Fatalf("collision should audit a failed member without changing documents: %d %+v", code, collided)
	}
	if string(c.read(t, "Notes/source.md")) != "source" || string(c.read(t, "Plans/destination.md")) != "destination" {
		t.Fatal("collision changed content")
	}
	code, output, _ = c.call(nil, "move", "Notes/absent.md", "Plans/new.md", "--json")
	if code != 2 || !bytes.Contains(output, []byte("DOCUMENT_NOT_FOUND")) {
		t.Fatalf("missing source accepted: %d %s", code, output)
	}
	for _, paths := range [][2]string{
		{"Notes/source.md", "Notes/source.md"},
		{"Notes/source.md", "../escape.md"},
		{"../escape.md", "Plans/next.md"},
		{"Notes/source.md", ".awl/private.md"},
		{"Notes/source.md", "Plans/../../outside.md"},
	} {
		code, output, _ = c.call(nil, "move", paths[0], paths[1], "--json")
		if code != 2 || (!bytes.Contains(output, []byte("INVALID_PATH")) && !bytes.Contains(output, []byte("INVALID_REQUEST"))) {
			t.Fatalf("invalid move accepted %q -> %q: %d %s", paths[0], paths[1], code, output)
		}
	}
}

func TestCLIContractMoveInOrderedComposite(t *testing.T) {
	c := fixture(t)
	c.init(t)
	payload := `{"request_id":"move-composite","operations":[
		{"id":"create","op":"create","path":"Activities/first.md","content":"Original"},
		{"id":"move","op":"move","path":"Activities/first.md","to":"Plans/moved.md"},
		{"id":"edit","op":"edit","path":"Plans/moved.md","old":"Original","new":"Revised"},
		{"id":"missing","op":"move","path":"Activities/not-found.md","to":"Plans/unused.md"},
		{"id":"create-again","op":"create","path":"Activities/first.md","content":"Replacement"}
	]}`
	code, output, errText := c.call([]byte(payload), "apply", "--json")
	var outcome result
	if err := json.Unmarshal(output, &outcome); err != nil {
		t.Fatalf("composite output is not JSON: %v %s", err, output)
	}
	if code != 3 || outcome.Status != "partial" || len(outcome.Members) != 5 ||
		outcome.Members[1].Status != "applied" || outcome.Members[2].Status != "applied" ||
		outcome.Members[3].Status != "failed" || outcome.Members[4].Status != "applied" {
		t.Fatalf("unexpected composite: code=%d output=%+v stderr=%s", code, outcome, errText)
	}
	if got := c.read(t, "Plans/moved.md"); string(got) != "Revised" {
		t.Fatalf("subsequent edit did not see moved document: %q", got)
	}
	if got := c.read(t, "Activities/first.md"); string(got) != "Replacement" {
		t.Fatalf("source path not reusable in same request: %q", got)
	}
	replayedCode, replayedOutput, _ := c.call([]byte(payload), "apply", "--json")
	var replayed result
	if err := json.Unmarshal(replayedOutput, &replayed); err != nil {
		t.Fatal(err)
	}
	if replayedCode != 3 || !replayed.Replayed || replayed.Commit != outcome.Commit {
		t.Fatalf("composite replay: %d %+v", replayedCode, replayed)
	}
}

func TestCLIContractMoveRejectsUnexpectedPayload(t *testing.T) {
	c := fixture(t)
	c.init(t)
	c.mustCall(t, []byte("source"), "create", "Notes/source.md", "--json")
	cases := []string{
		`{"request_id":"bad-move-1","operations":[{"op":"move","path":"Notes/source.md"}]}`,
		`{"request_id":"bad-move-2","operations":[{"op":"move","path":"Notes/source.md","to":"Plans/new.md","content":"replacement"}]}`,
		`{"request_id":"bad-move-3","operations":[{"op":"create","path":"Notes/another.md","to":"Plans/new.md","content":"other"}]}`,
	}
	for _, payload := range cases {
		code, output, _ := c.call([]byte(payload), "apply", "--json")
		if code != 2 || !bytes.Contains(output, []byte("INVALID_")) {
			t.Fatalf("invalid payload accepted: %d %s", code, output)
		}
	}
	if string(c.read(t, "Notes/source.md")) != "source" {
		t.Fatal("invalid payload changed document")
	}
}
