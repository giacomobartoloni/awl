package awl

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
)

// safeListPrefix accepts a project-relative directory, not a document edit
// target. An empty prefix selects the entire managed document tree.
func safeListPrefix(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	trimmed := strings.TrimSuffix(raw, "/")
	if trimmed == "" || strings.Contains(trimmed, "\\") || strings.ContainsRune(trimmed, '\x00') ||
		strings.Contains(trimmed, ":") || filepath.IsAbs(trimmed) {
		return "", fail("INVALID_PATH", "expected a project-relative directory prefix")
	}
	clean := filepath.ToSlash(filepath.Clean(trimmed))
	if clean != trimmed || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") ||
		clean == ".git" || strings.HasPrefix(clean, ".git/") ||
		clean == ".awl" || strings.HasPrefix(clean, ".awl/") {
		return "", fail("INVALID_PATH", "invalid or unsafe directory prefix")
	}
	return clean, nil
}

// listDocuments enumerates regular Markdown blobs in one committed managed
// snapshot. It never traverses the source worktree or creates a Git commit.
func listDocuments(p project, revision, prefix string) ([]string, error) {
	out, err := git(p.GitDir, nil, nil, "ls-tree", "-r", "-z", revision)
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for _, record := range bytes.Split(out, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		separator := bytes.IndexByte(record, '\t')
		if separator < 0 {
			return nil, fail("MANAGED_REPO_CORRUPT", "malformed Git tree entry")
		}
		info := strings.Fields(string(record[:separator]))
		if len(info) != 3 {
			return nil, fail("MANAGED_REPO_CORRUPT", "malformed Git entry metadata")
		}
		if info[1] != "blob" || info[0] != "100644" && info[0] != "100755" {
			continue
		}
		name := string(record[separator+1:])
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		if _, err := safePath(name); err != nil {
			return nil, fail("MANAGED_REPO_CORRUPT", "managed Markdown tree contains an unsafe path")
		}
		if prefix != "" && !strings.HasPrefix(name, prefix+"/") {
			continue
		}
		paths = append(paths, name)
	}
	sort.Strings(paths)
	return paths, nil
}
