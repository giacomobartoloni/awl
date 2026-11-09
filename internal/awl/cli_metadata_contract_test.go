package awl

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCLIContractMetadataAtomicSetPreservesMarkdownBytes(t *testing.T) {
	c := fixture(t)
	c.init(t)
	path := "Activities/finish.md"
	before := []byte("---\r\nstatus: active # existing comment\r\ncompleted_at: null\r\ntags: [activity, awl]\r\n---\r\n# Outcome\r\nKeep [[links]] and body bytes.\r\n")
	c.mustCall(t, before, "create", path, "--json")

	update := []byte(`{"/status":"completed","/completed_at":"2026-11-09"}`)
	applied := c.mustCall(t, update, "metadata", "set", path, "--input", "-", "--request-id", "metadata-finish-01", "--json")
	if applied.Status != "applied" || len(applied.Members) != 1 || applied.Members[0].Status != "applied" {
		t.Fatalf("metadata set: %+v", applied)
	}
	expected := []byte("---\r\nstatus: \"completed\" # existing comment\r\ncompleted_at: \"2026-11-09\"\r\ntags: [activity, awl]\r\n---\r\n# Outcome\r\nKeep [[links]] and body bytes.\r\n")
	if got := c.read(t, path); !bytes.Equal(got, expected) {
		t.Fatalf("unrelated bytes changed:\n got %q\nwant %q", got, expected)
	}
	replayed := c.mustCall(t, update, "metadata", "set", path, "--input", "-", "--request-id", "metadata-finish-01", "--json")
	if !replayed.Replayed || replayed.Commit != applied.Commit {
		t.Fatalf("metadata set replay failed: %+v", replayed)
	}

	rc, out, stderr := c.call(nil, "metadata", "get", path, "/status", "--json")
	if rc != 0 {
		t.Fatalf("metadata get: exit=%d out=%s err=%s", rc, out, stderr)
	}
	var got struct {
		Status string `json:"status"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(out, &got); err != nil || got.Status != "ok" || got.Value != "completed" {
		t.Fatalf("metadata get value: %s, %v", out, err)
	}
}

func TestCLIContractMetadataSetFailureDoesNotPartiallyChangeDocument(t *testing.T) {
	c := fixture(t)
	c.init(t)
	path := "Activities/atomic.md"
	before := []byte("---\nstatus: active\ncompleted_at: null\n---\nBody unchanged\n")
	c.mustCall(t, before, "create", path, "--json")
	update := []byte(`{"/status":"completed","/unknown":"value"}`)
	rc, out, stderr := c.call(update, "metadata", "set", path, "--input", "-", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("METADATA_TARGET_NOT_FOUND")) {
		t.Fatalf("expected failed atomic metadata set: %d %s %s", rc, out, stderr)
	}
	if got := c.read(t, path); !bytes.Equal(got, before) {
		t.Fatalf("failed metadata update changed document: %q", got)
	}
}

func TestCLIContractMetadataNestedPointerAndRemoval(t *testing.T) {
	c := fixture(t)
	c.init(t)
	path := "Specs/nested.md"
	before := []byte("---\nbuild.version: old\nbuild:\n  version: old\nobsolete: remove-me\nitems: [first, second]\n---\nText\n")
	c.mustCall(t, before, "create", path, "--json")
	changed := c.mustCall(t, nil, "metadata", "set", path, "/build/version", "new", "--json")
	if changed.Status != "applied" {
		t.Fatalf("nested set: %+v", changed)
	}
	removed := c.mustCall(t, nil, "metadata", "remove", path, "/obsolete", "--json")
	if removed.Status != "applied" {
		t.Fatalf("remove: %+v", removed)
	}
	expected := []byte("---\nbuild.version: old\nbuild:\n  version: \"new\"\nitems: [first, second]\n---\nText\n")
	if got := c.read(t, path); !bytes.Equal(got, expected) {
		t.Fatalf("unexpected nested mutation: %q", got)
	}

	rc, out, _ := c.call(nil, "metadata", "get", path, "/items/1", "--json")
	if rc != 0 || !bytes.Contains(out, []byte(`"value":"second"`)) {
		t.Fatalf("read nested list value: %d %s", rc, out)
	}
	rc, out, _ = c.call(nil, "metadata", "set", path, "/items/0", "other", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("UNSUPPORTED_METADATA")) {
		t.Fatalf("positional writes must not silently proceed: %d %s", rc, out)
	}
}

func TestCLIContractMetadataRejectsUnsupportedYAMLWithoutRewrite(t *testing.T) {
	c := fixture(t)
	c.init(t)
	for _, tc := range []struct {
		name, text string
	}{
		{"duplicate", "---\nstatus: active\nstatus: draft\n---\nBody\n"},
		{"alias", "---\nstatus: &state active\nother: *state\n---\nBody\n"},
		{"bad syntax", "---\nstatus: [unterminated\n---\nBody\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := "Activities/" + strings.ReplaceAll(tc.name, " ", "-") + ".md"
			initial := []byte(tc.text)
			c.mustCall(t, initial, "create", path, "--json")
			rc, out, _ := c.call(nil, "metadata", "set", path, "/status", "completed", "--json")
			if rc != 2 || !(bytes.Contains(out, []byte("UNSUPPORTED_METADATA")) || bytes.Contains(out, []byte("INVALID_METADATA"))) {
				t.Fatalf("unsafe YAML accepted: %d %s", rc, out)
			}
			if got := c.read(t, path); !bytes.Equal(got, initial) {
				t.Fatalf("unsafe YAML changed: %q", got)
			}
		})
	}
}

func TestCLIContractMetadataTypedValuesAndEscapedPointer(t *testing.T) {
	c := fixture(t)
	c.init(t)
	path := "Specs/typed.md"
	initial := []byte("---\ncount: 1\nflag: false\nexternal/id: old\n---\nBody\n")
	c.mustCall(t, initial, "create", path, "--json")

	number := []byte("9007199254740993")
	c.mustCall(t, number, "metadata", "set", path, "/count", "--input", "-", "--json")
	c.mustCall(t, []byte("true"), "metadata", "set", path, "/flag", "--input", "-", "--json")
	c.mustCall(t, nil, "metadata", "set", path, "/external~1id", "new", "--json")
	want := []byte("---\ncount: 9007199254740993\nflag: true\nexternal/id: \"new\"\n---\nBody\n")
	if got := c.read(t, path); !bytes.Equal(got, want) {
		t.Fatalf("metadata typed update changed bytes or precision: %q", got)
	}
	c.mustCall(t, []byte("null"), "metadata", "set", path, "/count", "--input", "-", "--json")
	if got := c.read(t, path); !bytes.Contains(got, []byte("count: null\n")) {
		t.Fatalf("null was not written as a typed YAML value: %q", got)
	}
}

func TestCLIContractMetadataRejectsMultilineScalarWithoutDataLoss(t *testing.T) {
	c := fixture(t)
	c.init(t)
	path := "Activities/multiline.md"
	initial := []byte("---\nstatus: first\n  second\nother: preserved\n---\nBody\n")
	c.mustCall(t, initial, "create", path, "--json")
	rc, out, _ := c.call(nil, "metadata", "set", path, "/status", "completed", "--json")
	if rc != 2 || !bytes.Contains(out, []byte("UNSUPPORTED_METADATA")) {
		t.Fatalf("multiline scalar was not rejected: %d %s", rc, out)
	}
	if got := c.read(t, path); !bytes.Equal(got, initial) {
		t.Fatalf("multiline scalar edit changed unrelated content: %q", got)
	}
}

func TestCLIContractMetadataSetRequiresUnambiguousInputs(t *testing.T) {
	c := fixture(t)
	c.init(t)
	path := "Activities/inputs.md"
	initial := []byte("---\nstatus: active\ncompleted_at: null\n---\nBody\n")
	c.mustCall(t, initial, "create", path, "--json")
	for _, tc := range []struct {
		name  string
		input []byte
		args  []string
	}{
		{"missing value", nil, []string{"metadata", "set", path, "/status", "--json"}},
		{"missing map input", nil, []string{"metadata", "set", path, "--json"}},
		{"conflicting value sources", []byte(`"other"`), []string{"metadata", "set", path, "/status", "completed", "--input", "-", "--json"}},
		{"removed update command", []byte(`{"/status":"completed"}`), []string{"metadata", "update", path, "--input", "-", "--json"}},
		{"old update envelope", []byte(`{"set":{"/status":"completed"}}`), []string{"metadata", "set", path, "--input", "-", "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rc, _, _ := c.call(tc.input, tc.args...)
			if rc != 2 {
				t.Fatalf("expected rejected metadata invocation, exit=%d", rc)
			}
			if got := c.read(t, path); !bytes.Equal(got, initial) {
				t.Fatalf("rejected input changed document: %q", got)
			}
		})
	}
}
