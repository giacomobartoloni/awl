package awl

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSIGKILLAtGitPublicationBoundary exercises two actual awl process kills.
// The test-only Git wrapper sends SIGKILL to its parent either before issuing
// update-ref or after Git has accepted the update but before the CLI responds.
// This tests process termination, not machine power-loss or filesystem fsync.
func TestSIGKILLAtGitPublicationBoundary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("POSIX shell and SIGKILL test requires Linux; platform behavior is unverified")
	}
	for _, mode := range []string{"before", "after"} {
		t.Run(mode, func(t *testing.T) {
			c := fixture(t)
			c.mustCall(t, nil, "init", "--slug", "demo", "--json")
			repos, err := filepath.Glob(filepath.Join(c.Data, "awl", "repos", "*"))
			if err != nil || len(repos) != 1 {
				t.Fatalf("managed Git repository: %v, paths=%v", err, repos)
			}
			p := project{GitDir: repos[0]}
			headBefore, err := headCommit(p)
			if err != nil {
				t.Fatal(err)
			}
			historyBefore, err := gitText(p.GitDir, nil, nil, "rev-list", managedRef)
			if err != nil {
				t.Fatal(err)
			}

			realGit, err := exec.LookPath("git")
			if err != nil {
				t.Fatal(err)
			}
			wrapperDir := t.TempDir()
			marker := filepath.Join(t.TempDir(), "crash-point-reached")
			// The wrapper is used exclusively in the test harness. The
			// production awl CLI still invokes Git directly, without a shell.
			wrapper := "#!/bin/sh\n" +
				"if [ \"$2\" = \"update-ref\" ]; then\n" +
				"  case \"$AWL_TEST_CRASH_MODE\" in\n" +
				"    before)\n" +
				"      : > \"$AWL_TEST_CRASH_MARKER\" || exit 81\n" +
				"      kill -KILL \"$PPID\" || exit 82\n" +
				"      exit 83\n" +
				"      ;;\n" +
				"    after)\n" +
				"      \"$AWL_TEST_REAL_GIT\" \"$@\" || exit $?\n" +
				"      : > \"$AWL_TEST_CRASH_MARKER\" || exit 84\n" +
				"      kill -KILL \"$PPID\" || exit 85\n" +
				"      exit 86\n" +
				"      ;;\n" +
				"  esac\n" +
				"fi\n" +
				"exec \"$AWL_TEST_REAL_GIT\" \"$@\"\n"
			if err := os.WriteFile(filepath.Join(wrapperDir, "git"), []byte(wrapper), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("AWL_TEST_REAL_GIT", realGit)
			t.Setenv("AWL_TEST_CRASH_MARKER", marker)
			t.Setenv("AWL_TEST_CRASH_MODE", mode)
			t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			requestID := "sigkill-" + mode
			document := []byte("---\nstatus: active\n---\n# Interrupted " + mode + "\n")
			code, output, stderr := c.call(document, "create", "Specs/interrupted.md", "--request-id", requestID, "--json")
			if code == 0 {
				t.Fatalf("CLI unexpectedly acknowledged a killed operation: output=%q stderr=%q", output, stderr)
			}
			if len(output) != 0 {
				t.Fatalf("killed CLI emitted a completion response: output=%q stderr=%q", output, stderr)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatalf("SIGKILL injection did not execute: %v; exit=%d stderr=%s", err, code, stderr)
			}
			// Disable the injection before inspecting state and retrying.
			t.Setenv("AWL_TEST_CRASH_MODE", "")

			headAfterKill, err := headCommit(p)
			if err != nil {
				t.Fatal(err)
			}
			historyAfterKill, err := gitText(p.GitDir, nil, nil, "rev-list", managedRef)
			if err != nil {
				t.Fatal(err)
			}
			auditAfterKill, publishedAfterKill, err := findAudit(p, headAfterKill, requestID)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "before":
				if headAfterKill != headBefore || historyAfterKill != historyBefore || publishedAfterKill != "" {
					t.Fatalf("pre-publication kill changed reachable history: before=%s after=%s audit=%+v commit=%s", headBefore, headAfterKill, auditAfterKill, publishedAfterKill)
				}
				if rc, out, _ := c.call(nil, "read", "Specs/interrupted.md"); rc == 0 {
					t.Fatalf("pre-publication kill exposed the document: %q", out)
				}
			case "after":
				if headAfterKill == headBefore || publishedAfterKill != headAfterKill || auditAfterKill.RequestID != requestID {
					t.Fatalf("post-publication kill lost audit or ref: before=%s after=%s audit=%+v commit=%s", headBefore, headAfterKill, auditAfterKill, publishedAfterKill)
				}
				if len(strings.Split(historyAfterKill, "\n")) != len(strings.Split(historyBefore, "\n"))+1 {
					t.Fatalf("post-publication kill did not publish exactly one commit")
				}
				if got := c.read(t, "Specs/interrupted.md"); !bytes.Equal(got, document) {
					t.Fatalf("post-publication kill lost document bytes: %q", got)
				}
			}

			// A caller that received no acknowledgement can resubmit the
			// identical request ID. The existing Git audit distinguishes a
			// previously committed request from one killed before publication.
			recovered := c.mustCall(t, document, "create", "Specs/interrupted.md", "--request-id", requestID, "--json")
			headAfterRetry, err := headCommit(p)
			if err != nil {
				t.Fatal(err)
			}
			if recovered.Commit != headAfterRetry || recovered.Status != "applied" {
				t.Fatalf("retry did not recover the acknowledged local state: %+v", recovered)
			}
			if mode == "before" && (recovered.Replayed || headAfterRetry == headBefore) {
				t.Fatalf("pre-publication retry should publish exactly once: %+v", recovered)
			}
			if mode == "after" && (!recovered.Replayed || headAfterRetry != headAfterKill) {
				t.Fatalf("post-publication retry duplicated an accepted commit: %+v", recovered)
			}
			historyAfterRetry, err := gitText(p.GitDir, nil, nil, "rev-list", managedRef)
			if err != nil {
				t.Fatal(err)
			}
			if len(strings.Split(historyAfterRetry, "\n")) != len(strings.Split(historyBefore, "\n"))+1 {
				t.Fatalf("recovery created the wrong number of reachable commits")
			}
			if got := c.read(t, "Specs/interrupted.md"); !bytes.Equal(got, document) {
				t.Fatalf("recovery changed Markdown bytes: %q", got)
			}
		})
	}
}
