package awl

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestConcurrentIncompatibleCompositeAbortsWithoutCommit makes two independent
// awl CLI processes stage conflicting requests against exactly the same Git
// head. A test-only Git wrapper blocks both before their guarded ref update,
// then serializes those updates to exercise deterministic CAS loss.
func TestConcurrentIncompatibleCompositeAbortsWithoutCommit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test requires a POSIX Git wrapper; portability is unverified")
	}
	c := fixture(t)
	c.mustCall(t, nil, "init", "--slug", "demo", "--json")
	initial := []byte("---\nstatus: active\n---\nshared\n")
	c.mustCall(t, initial, "create", "Specs/shared.md", "--json")

	repos, err := filepath.Glob(filepath.Join(c.Data, "awl", "repos", "*"))
	if err != nil || len(repos) != 1 {
		t.Fatalf("managed repo: %v; candidates=%v", err, repos)
	}
	gitDir := repos[0]
	beforeHead, err := headCommit(project{GitDir: gitDir})
	if err != nil {
		t.Fatal(err)
	}
	beforeHistory, err := gitText(gitDir, nil, nil, "rev-list", managedRef)
	if err != nil {
		t.Fatal(err)
	}

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	gateDir, wrapperDir := t.TempDir(), t.TempDir()
	// Gate both subprocesses AFTER they have already prepared commits against
	// the same old ref. Then let one update finish before running the other.
	// This is POSIX shell code ONLY inside the integration-test harness.
	wrapper := "#!/bin/sh\n" +
		"if [ \"$2\" = \"update-ref\" ]; then\n" +
		"  : > \"$AWL_TEST_GATE_DIR/arrive.$$\" || exit 81\n" +
		"  tries=0\n" +
		"  while [ ! -f \"$AWL_TEST_GATE_DIR/release\" ]; do\n" +
		"    tries=$((tries+1))\n" +
		"    if [ \"$tries\" -ge 600 ]; then exit 82; fi\n" +
		"    sleep 0.02\n" +
		"  done\n" +
		"  if mkdir \"$AWL_TEST_GATE_DIR/first\" 2>/dev/null; then\n" +
		"    \"$AWL_TEST_REAL_GIT\" \"$@\"\n" +
		"    rc=$?\n" +
		"    : > \"$AWL_TEST_GATE_DIR/first-done\"\n" +
		"    exit \"$rc\"\n" +
		"  fi\n" +
		"  tries=0\n" +
		"  while [ ! -f \"$AWL_TEST_GATE_DIR/first-done\" ]; do\n" +
		"    tries=$((tries+1))\n" +
		"    if [ \"$tries\" -ge 600 ]; then exit 83; fi\n" +
		"    sleep 0.02\n" +
		"  done\n" +
		"fi\n" +
		"exec \"$AWL_TEST_REAL_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(wrapperDir, "git"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWL_TEST_REAL_GIT", realGit)
	t.Setenv("AWL_TEST_GATE_DIR", gateDir)
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	type candidate struct {
		id      string
		newBody string
		sidecar string
	}
	cases := []candidate{
		{"collision-a", "writer A\n", "Specs/a-only.md"},
		{"collision-b", "writer B\n", "Specs/b-only.md"},
	}
	type outcome struct {
		id     string
		code   int
		stdout []byte
		stderr string
	}
	results := make(chan outcome, len(cases))
	for _, tc := range cases {
		payload, err := json.Marshal(request{
			RequestID: tc.id,
			Operations: []operation{
				{ID: "edit-shared", Op: "edit", Path: "Specs/shared.md", Old: "shared\n", New: tc.newBody},
				{ID: "create-sidecar", Op: "create", Path: tc.sidecar, Content: "# Sidecar\n"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		go func(id string, input []byte) {
			code, output, stderr := c.call(input, "apply", "--json")
			results <- outcome{id: id, code: code, stdout: output, stderr: stderr}
		}(tc.id, payload)
	}

	// Both guarded Git ref updates must reach the barrier before either can
	// publish. This removes timing luck from the conflict test.
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
waitForBoth:
	for {
		entries, err := os.ReadDir(gateDir)
		if err != nil {
			t.Fatal(err)
		}
		arrivals := 0
		for _, item := range entries {
			if strings.HasPrefix(item.Name(), "arrive.") {
				arrivals++
			}
		}
		if arrivals == len(cases) {
			break waitForBoth
		}
		select {
		case <-deadline.C:
			t.Fatalf("both Git updates did not reach barrier, arrivals=%d", arrivals)
		case <-ticker.C:
		}
	}
	if err := os.WriteFile(filepath.Join(gateDir, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	outcomes := make(map[string]outcome, len(cases))
	for range cases {
		select {
		case got := <-results:
			outcomes[got.id] = got
		case <-time.After(15 * time.Second):
			t.Fatal("concurrent CLI request did not finish after barrier release")
		}
	}

	var winner, loser candidate
	var accepted result
	for _, tc := range cases {
		item := outcomes[tc.id]
		var got result
		if err := json.Unmarshal(item.stdout, &got); err != nil {
			t.Fatalf("%s invalid JSON: %q: %v; stderr=%s", tc.id, item.stdout, err, item.stderr)
		}
		switch {
		case item.code == 0 && got.Status == "applied" && len(got.Members) == 2:
			if winner.id != "" {
				t.Fatalf("both requests accepted, second=%s", tc.id)
			}
			for _, member := range got.Members {
				if member.Status != "applied" {
					t.Fatalf("accepted composite had a failed member: %+v", got)
				}
			}
			winner, accepted = tc, got
		case item.code != 0 && got.Error != nil && got.Error.Code == "CONCURRENT_REQUEST_CONFLICT":
			if loser.id != "" {
				t.Fatalf("both requests rejected, second=%s", tc.id)
			}
			loser = tc
		default:
			t.Fatalf("%s unexpected result: exit=%d response=%+v stderr=%s", tc.id, item.code, got, item.stderr)
		}
	}
	if winner.id == "" || loser.id == "" {
		t.Fatalf("want one accepted request and one rejected conflict: %+v", outcomes)
	}

	afterHead, err := headCommit(project{GitDir: gitDir})
	if err != nil {
		t.Fatal(err)
	}
	if afterHead != accepted.Commit || afterHead == beforeHead {
		t.Fatalf("unexpected published head before=%s after=%s accepted=%+v", beforeHead, afterHead, accepted)
	}
	afterHistory, err := gitText(gitDir, nil, nil, "rev-list", managedRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(afterHistory, "\n")) != len(strings.Split(beforeHistory, "\n"))+1 {
		t.Fatalf("rejected composite has a reachable commit: before=%q after=%q", beforeHistory, afterHistory)
	}
	p := project{GitDir: gitDir}
	if _, published, err := findAudit(p, afterHead, loser.id); err != nil || published != "" {
		t.Fatalf("rejected request published an audit: commit=%s error=%v", published, err)
	}
	if _, published, err := findAudit(p, afterHead, winner.id); err != nil || published != accepted.Commit {
		t.Fatalf("accepted request audit is missing: commit=%s error=%v", published, err)
	}
	want := append([]byte("---\nstatus: active\n---\n"), []byte(winner.newBody)...)
	if got := c.read(t, "Specs/shared.md"); !bytes.Equal(got, want) {
		t.Fatalf("shared content differs: got=%q want=%q", got, want)
	}
	if got := c.read(t, winner.sidecar); string(got) != "# Sidecar\n" {
		t.Fatalf("accepted sidecar missing: %q", got)
	}
	if code, out, stderr := c.call(nil, "read", loser.sidecar); code == 0 {
		t.Fatalf("rejected composite sidecar exists: %s stderr=%s", out, stderr)
	}
}
