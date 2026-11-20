package awl

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

func (p project) commitEnv() []string {
	return []string{"GIT_AUTHOR_NAME=" + p.UserName, "GIT_AUTHOR_EMAIL=" + p.UserEmail, "GIT_COMMITTER_NAME=" + p.UserName, "GIT_COMMITTER_EMAIL=" + p.UserEmail}
}
func headCommit(p project) (string, error) {
	h, e := gitText(p.GitDir, nil, nil, "rev-parse", "--verify", managedRef)
	if e != nil {
		return "", fail("MANAGED_REPO_CORRUPT", "cannot read managed Git head")
	}
	return h, nil
}
func treeOID(p project, head string) (string, error) {
	return gitText(p.GitDir, nil, nil, "rev-parse", head+"^{tree}")
}
func readEntry(p project, head, path string) (entry, error) {
	b, e := git(p.GitDir, nil, nil, "ls-tree", "-r", "-z", head, "--", path)
	if e != nil {
		return entry{}, e
	}
	var match []byte
	for _, record := range bytes.Split(b, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		tab := bytes.IndexByte(record, '\t')
		if tab < 0 {
			continue
		}
		if string(record[tab+1:]) == path {
			match = record[:tab]
			break
		}
	}
	if match == nil {
		return entry{}, nil
	}
	parts := strings.Fields(string(match))
	if len(parts) != 3 || parts[1] != "blob" || parts[0] != "100644" && parts[0] != "100755" {
		return entry{}, fail("UNSUPPORTED_ENTRY", "target is not an ordinary regular Git blob")
	}
	bytes, e := git(p.GitDir, nil, nil, "cat-file", "blob", parts[2])
	if e != nil {
		return entry{}, e
	}
	return entry{Mode: parts[0], OID: parts[2], Bytes: bytes, Exists: true}, nil
}
func writeTree(p project, head string, changes map[string]entry) (string, error) {
	if len(changes) == 0 {
		return treeOID(p, head)
	}
	temp, e := os.CreateTemp("", "awl-index-*")
	if e != nil {
		return "", e
	}
	path := temp.Name()
	temp.Close()
	defer os.Remove(path)
	env := []string{"GIT_INDEX_FILE=" + path}
	if _, e = git(p.GitDir, env, nil, "read-tree", head); e != nil {
		return "", e
	}
	// Remove paths before adding new ones so moves can replace a file path
	// with a directory path (or vice versa) within a single Git tree.
	for name, file := range changes {
		if file.Exists {
			continue
		}
		// Bare Git stores have no worktree: remove through index-info, using
		// NUL-delimited paths to preserve unusual but valid Markdown names.
		deletion := []byte("0 " + strings.Repeat("0", 40) + "\t" + name + "\x00")
		if _, e = git(p.GitDir, env, deletion, "update-index", "-z", "--index-info"); e != nil {
			return "", e
		}
	}
	for name, file := range changes {
		if !file.Exists {
			continue
		}
		oid, e := gitText(p.GitDir, nil, file.Bytes, "hash-object", "-w", "--stdin")
		if e != nil {
			return "", e
		}
		if _, e = git(p.GitDir, env, nil, "update-index", "--add", "--cacheinfo", file.Mode+","+oid+","+name); e != nil {
			return "", e
		}
	}
	return gitText(p.GitDir, env, nil, "write-tree")
}
func findAudit(p project, head, id string) (audit, string, error) {
	list, e := gitText(p.GitDir, nil, nil, "rev-list", head)
	if e != nil {
		return audit{}, "", e
	}
	hashes := strings.Split(list, "\n")
	if len(hashes) > 5001 {
		return audit{}, "", fail("AUDIT_LOOKUP_LIMIT", "audit lookup limit exceeded; refusing possible duplicate request")
	}
	var found audit
	var foundSHA string
	for _, sha := range hashes {
		if sha == "" {
			continue
		}
		msg, e := gitText(p.GitDir, nil, nil, "show", "-s", "--format=%B", sha)
		if e != nil {
			return audit{}, "", e
		}
		if !strings.HasPrefix(msg, "AWL-AUDIT-V1\n") {
			continue
		}
		var v audit
		payload := strings.TrimSpace(strings.TrimPrefix(msg, "AWL-AUDIT-V1\n"))
		if e = json.Unmarshal([]byte(payload), &v); e != nil {
			return audit{}, "", fail("AUDIT_CORRUPT", "committed audit message is malformed")
		}
		if v.Schema != 1 {
			return audit{}, "", fail("AUDIT_CORRUPT", "unsupported audit version")
		}
		if v.RequestID == id {
			if foundSHA != "" {
				return audit{}, "", fail("AUDIT_DUPLICATE", "more than one historical commit claims this request ID")
			}
			found, foundSHA = v, sha
		}
	}
	return found, foundSHA, nil
}
