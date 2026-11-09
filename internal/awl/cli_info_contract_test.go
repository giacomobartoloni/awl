package awl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIContractHelpAndVersionAreDiscoverableWithoutInit(t *testing.T) {
	c := fixture(t)
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"--help"}, []string{"Usage:", "Commands:", "metadata", "--version", "Exit codes:"}},
		{[]string{"-h"}, []string{"Usage:", "Commands:"}},
		{[]string{"help"}, []string{"Usage:", "Commands:"}},
		{[]string{"--project", "demo", "--help"}, []string{"Usage:", "Commands:"}},
		{[]string{"help", "read"}, []string{"Usage: awl", "read DOCUMENT.md", "--json"}},
		{[]string{"read", "--help"}, []string{"Usage: awl", "read DOCUMENT.md", "--json"}},
		{[]string{"read", "Activities/example.md", "-h"}, []string{"read DOCUMENT.md"}},
		{[]string{"init", "--help"}, []string{"--slug", "--repo-dir"}},
		{[]string{"remote", "add", "--help"}, []string{"remote add GIT_URL", "Does not fetch or push"}},
		{[]string{"metadata", "set", "Activities/example.md", "--help"}, []string{"metadata get", "metadata set", "metadata insert", "metadata remove", "JSON Pointers"}},
		{[]string{"apply", "--help"}, []string{"request_id", "exit code 3"}},
	} {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			rc, out, stderr := c.call(nil, tc.args...)
			if rc != 0 || stderr != "" {
				t.Fatalf("awl %v: exit=%d stdout=%q stderr=%q", tc.args, rc, out, stderr)
			}
			text := string(out)
			for _, fragment := range tc.want {
				if !strings.Contains(text, fragment) {
					t.Errorf("awl %v: help missing %q: %s", tc.args, fragment, text)
				}
			}
		})
	}
	for _, args := range [][]string{{"--version"}, {"version"}, {"--project", "demo", "--version"}} {
		rc, out, stderr := c.call(nil, args...)
		if rc != 0 || stderr != "" || !strings.HasPrefix(string(out), "awl ") || strings.Count(string(out), "\n") != 1 || strings.TrimSpace(string(out)) == "awl" {
			t.Errorf("awl %v: expected a single version line on stdout: exit=%d stdout=%q stderr=%q", args, rc, out, stderr)
		}
	}
	if _, err := os.Lstat(filepath.Join(c.Source, ".awl")); !os.IsNotExist(err) {
		t.Fatalf("help or version created project state: %v", err)
	}
	if _, err := os.Lstat(c.Data); !os.IsNotExist(err) {
		t.Fatalf("help or version created managed data: %v", err)
	}
}

func TestCLIContractHelpAndVersionRejectAmbiguousInvocations(t *testing.T) {
	c := fixture(t)
	for _, args := range [][]string{
		{"--version", "unexpected"},
		{"--help", "unexpected"},
		{"help", "unknown"},
		{"help", "read", "unexpected"},
		{"init", "--slug", "--help"}, // a flag value is not a help request
		{"apply", "--input", "--help"}, // a flag value is not a help request
	} {
		rc, out, stderr := c.call(nil, args...)
		if rc != 2 || len(out) != 0 || !strings.HasPrefix(stderr, "awl: ") {
			t.Errorf("awl %v: expected plain error, not help: exit=%d stdout=%q stderr=%q", args, rc, out, stderr)
		}
	}
	if _, err := os.Lstat(filepath.Join(c.Source, ".awl")); !os.IsNotExist(err) {
		t.Fatalf("invalid info command created configuration: %v", err)
	}
}
