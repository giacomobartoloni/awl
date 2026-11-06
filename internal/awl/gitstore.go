package awl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func appDataHome() (string, error) {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		if !filepath.IsAbs(v) {
			return "", fail("INVALID_DATA_HOME", "XDG_DATA_HOME must be absolute")
		}
		return v, nil
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(home, ".local", "share"), nil
}
func noSymlinkDir(path string) error {
	st, e := os.Lstat(path)
	if e == nil {
		if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return fail("UNSAFE_PATH", "path is not a real directory: "+path)
		}
		return nil
	}
	if !os.IsNotExist(e) {
		return e
	}
	return os.Mkdir(path, 0700)
}
func atomicNewFile(path string, content []byte) error {
	// Write and sync a private file first, then publish it without replacing a
	// preexisting user file. This is not a demonstrated power-loss guarantee.
	dir := filepath.Dir(path)
	temp, e := os.CreateTemp(dir, ".awl-pending-*")
	if e != nil {
		return e
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if e = temp.Chmod(0600); e != nil {
		temp.Close()
		return e
	}
	if _, e = temp.Write(content); e == nil {
		e = temp.Sync()
	}
	if closeErr := temp.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Link(tempPath, path)
}
func validateOptionalLocalLink(root, common string) error {
	link := filepath.Join(root, ".awl", "local.yaml")
	st, e := os.Lstat(link)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if st.Mode()&os.ModeSymlink == 0 {
		return fail("LOCAL_CONFIG_CONFLICT", "worktree-local local.yaml must be an optional symlink, not an independent file")
	}
	target, e := os.Readlink(link)
	if e != nil {
		return e
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	expected := filepath.Join(common, "awl", "local.yaml")
	if filepath.Clean(target) != filepath.Clean(expected) {
		return fail("LOCAL_CONFIG_CONFLICT", "local.yaml symlink must target canonical Git common-dir configuration")
	}
	tracked, e := gitSource(root, "ls-files", "--cached", "--", ".awl/local.yaml")
	if e != nil {
		return e
	}
	if len(tracked) > 0 {
		return fail("LOCAL_CONFIG_CONFLICT", "worktree local.yaml is tracked by source Git")
	}
	if _, e := gitSource(root, "check-ignore", "-q", "--", ".awl/local.yaml"); e != nil {
		return fail("LOCAL_CONFIG_NOT_IGNORED", "optional .awl/local.yaml symlink must be excluded from source Git")
	}
	return nil
}

func initialize(slug, remote, explicit string) (project, error) {
	root, common, e := source()
	if e != nil {
		return project{}, e
	}
	// Malformed first-use arguments must not create even the .awl directory.
	firstProjectFile := filepath.Join(root, ".awl", "project.yaml")
	if _, e = os.Lstat(firstProjectFile); os.IsNotExist(e) && (!slugPattern.MatchString(slug) || remote == "") {
		return project{}, fail("INIT_INPUT_REQUIRED", "new project needs --slug and --remote")
	}
	if explicit != "" && slug != "" && explicit != slug {
		return project{}, fail("PROJECT_CONFLICT", "explicit project does not match init slug")
	}
	if e = noSymlinkDir(filepath.Join(common, "awl")); e != nil {
		return project{}, e
	}
	unlock, e := flock(filepath.Join(common, "awl", "init.lock"))
	if e != nil {
		return project{}, e
	}
	defer unlock()
	configDir := filepath.Join(root, ".awl")
	if e = noSymlinkDir(configDir); e != nil {
		return project{}, e
	}
	projPath := filepath.Join(configDir, "project.yaml")
	if e = validateOptionalLocalLink(root, common); e != nil {
		return project{}, e
	}
	if st, check := os.Lstat(projPath); check == nil && st.Mode()&os.ModeSymlink != 0 {
		return project{}, fail("UNSAFE_PATH", "source project configuration must not be a symlink")
	}
	if _, e = os.Stat(projPath); e == nil {
		existingSlug, existingRemote, e := parseShared(projPath)
		if e != nil {
			return project{}, e
		}
		if slug != "" && slug != existingSlug {
			return project{}, fail("PROJECT_CONFLICT", "init slug differs from existing configuration")
		}
		if remote != "" && remote != existingRemote {
			return project{}, fail("PROJECT_CONFLICT", "init remote differs from existing configuration")
		}
		slug, remote = existingSlug, existingRemote
	} else if !os.IsNotExist(e) {
		return project{}, e
	} else if !slugPattern.MatchString(slug) || remote == "" {
		return project{}, fail("INIT_INPUT_REQUIRED", "new project needs --slug and --remote")
	}
	if explicit != "" && explicit != slug {
		return project{}, fail("PROJECT_CONFLICT", "--project disagrees with source project")
	}
	if strings.ContainsAny(remote, "\n\r#*\\") || strings.Contains(remote, "://") && strings.Contains(strings.SplitN(strings.SplitN(remote, "://", 2)[1], "/", 2)[0], "@") {
		return project{}, fail("INVALID_CONFIG", "invalid or credential-bearing remote")
	}
	user, email, e := identity(root)
	if e != nil {
		return project{}, e
	}
	localPath := filepath.Join(common, "awl", "local.yaml")
	if _, e = os.Stat(localPath); e == nil {
		bind, e := parseLocal(localPath)
		if e != nil {
			return project{}, e
		}
		if bind.Slug != slug || bind.Remote != remote {
			return project{}, fail("PROJECT_CONFLICT", "canonical project binding conflicts with source configuration")
		}
		if _, e = os.Stat(bind.LocalRepo); e != nil {
			return project{}, fail("MANAGED_REPO_MISSING", "configured managed repository is missing")
		}
		p := project{Root: root, Common: common, Slug: slug, Remote: remote, GitDir: bind.LocalRepo, UserName: user, UserEmail: email}
		if _, e = headCommit(p); e != nil {
			return project{}, e
		}
		return p, nil
	} else if !os.IsNotExist(e) {
		return project{}, e
	}
	data, e := appDataHome()
	if e != nil {
		return project{}, e
	}
	base := filepath.Join(data, "awl", "repos")
	if e = os.MkdirAll(base, 0700); e != nil {
		return project{}, e
	}
	nonce, e := randomID()
	if e != nil {
		return project{}, e
	}
	dest := filepath.Join(base, slug+"-"+nonce)
	if _, e = gitText("", nil, nil, "init", "--bare", dest); e != nil {
		return project{}, e
	}
	p := project{Root: root, Common: common, Slug: slug, Remote: remote, GitDir: dest, UserName: user, UserEmail: email}
	// An empty initial tree is a bootstrap state, not an audited mutation request.
	tree, e := gitText(dest, nil, []byte{}, "hash-object", "-w", "-t", "tree", "--stdin")
	if e != nil {
		return project{}, e
	}
	initial, e := gitText(dest, p.commitEnv(), []byte("awl bootstrap\n"), "commit-tree", tree)
	if e != nil {
		return project{}, e
	}
	if _, e = gitText(dest, nil, nil, "update-ref", managedRef, initial); e != nil {
		return project{}, e
	}
	if _, e = os.Lstat(projPath); os.IsNotExist(e) {
		body := fmt.Sprintf("schema: 1\nproject:\n  slug: %s\nstorage:\n  remote: %s\n", slug, remote)
		if e = atomicNewFile(projPath, []byte(body)); e != nil {
			return project{}, e
		}
	} else if e != nil {
		return project{}, e
	}
	local := fmt.Sprintf("schema: 1\nbinding:\n  slug: %s\n  remote: %s\nstorage:\n  localRepo: %s\n", slug, remote, dest)
	if e = atomicNewFile(localPath, []byte(local)); e != nil {
		return project{}, e
	}
	return p, nil
}
func resolve(explicit string) (project, error) {
	root, common, e := source()
	if e != nil {
		return project{}, e
	}
	projectFile := filepath.Join(root, ".awl", "project.yaml")
	if st, e := os.Lstat(projectFile); e == nil && st.Mode()&os.ModeSymlink != 0 {
		return project{}, fail("UNSAFE_PATH", "source project configuration cannot be a symlink")
	}
	slug, remote, e := parseShared(projectFile)
	if e != nil {
		if os.IsNotExist(e) {
			return project{}, fail("PROJECT_NOT_INITIALIZED", "missing .awl/project.yaml; run awl init")
		}
		return project{}, e
	}
	if explicit != "" && explicit != slug {
		return project{}, fail("PROJECT_NOT_RESOLVED", "out-of-repository --project selection is not part of this spike")
	}
	bind, e := parseLocal(filepath.Join(common, "awl", "local.yaml"))
	if e != nil {
		if os.IsNotExist(e) {
			return project{}, fail("PROJECT_NOT_INITIALIZED", "missing canonical local binding; run awl init")
		}
		return project{}, e
	}
	if bind.Slug != slug || bind.Remote != remote {
		return project{}, fail("PROJECT_CONFLICT", "source configuration differs from initialized canonical binding")
	}
	if e = validateOptionalLocalLink(root, common); e != nil {
		return project{}, e
	}
	st, e := os.Lstat(bind.LocalRepo)
	if e != nil {
		return project{}, fail("MANAGED_REPO_MISSING", "managed repository unavailable")
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return project{}, fail("UNSAFE_PATH", "managed repository is not an ordinary directory")
	}
	name, email, e := identity(root)
	if e != nil {
		return project{}, e
	}
	return project{Root: root, Common: common, Slug: slug, Remote: remote, GitDir: bind.LocalRepo, UserName: name, UserEmail: email}, nil
}
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
func (c *cli) writeRequest(req request) int {
	p, e := resolve(c.explicit)
	if e != nil {
		return c.emitError(e)
	}
	if len(req.Operations) == 0 || len(req.Operations) > 100 {
		return c.emitError(fail("INVALID_REQUEST", "operations count must be 1..100"))
	}
	ids := map[string]bool{}
	for i := range req.Operations {
		o := &req.Operations[i]
		if o.ID == "" {
			o.ID = fmt.Sprintf("op-%d", i+1)
		}
		if ids[o.ID] {
			return c.emitError(fail("INVALID_REQUEST", "duplicate operation ID"))
		}
		ids[o.ID] = true
		path, e := safePath(o.Path)
		if e != nil {
			return c.emitError(e)
		}
		o.Path = path
		if o.Op != "create" && o.Op != "edit" {
			return c.emitError(fail("INVALID_REQUEST", "only create/edit are in this spike"))
		}
		if o.Op == "edit" && o.Old == "" {
			return c.emitError(fail("INVALID_REQUEST", "edit.old is required"))
		}
		if !utf8.ValidString(o.Content) || !utf8.ValidString(o.Old) || !utf8.ValidString(o.New) {
			return c.emitError(fail("INVALID_CONTENT", "invalid UTF-8"))
		}
	}
	if req.RequestID == "" {
		req.RequestID, e = randomID()
		if e != nil {
			return c.emitError(e)
		}
	}
	if len(req.RequestID) > 120 || strings.ContainsAny(req.RequestID, "\n\r\x00") {
		return c.emitError(fail("INVALID_REQUEST", "unsafe request ID"))
	}
	ops, e := json.Marshal(req.Operations)
	if e != nil {
		return c.emitError(e)
	}
	digest := hash(ops)
	for attempt := 0; attempt < 8; attempt++ {
		head, e := headCommit(p)
		if e != nil {
			return c.emitError(e)
		}
		if prior, commit, e := findAudit(p, head, req.RequestID); e != nil {
			return c.emitError(e)
		} else if commit != "" {
			if prior.Digest != digest {
				return c.emitError(fail("REQUEST_ID_CONFLICT", "request ID already refers to a different payload"))
			}
			tree, e := treeOID(p, commit)
			if e != nil {
				return c.emitError(e)
			}
			return c.emit(result{Status: prior.Status, Project: p.Slug, RequestID: req.RequestID, Commit: commit, Tree: tree, Members: prior.Members, Replayed: true})
		}
		members, changed, e := stage(p, head, req.Operations)
		if e != nil {
			return c.emitError(e)
		}
		status := classify(members)
		a := audit{Schema: 1, RequestID: req.RequestID, Digest: digest, Members: members, Status: status, ActorSource: "git-source-worktree"}
		auditBody, e := json.Marshal(a)
		if e != nil {
			return c.emitError(e)
		}
		tree, e := writeTree(p, head, changed)
		if e != nil {
			return c.emitError(e)
		}
		commit, e := gitText(p.GitDir, p.commitEnv(), []byte("AWL-AUDIT-V1\n"+string(auditBody)+"\n"), "commit-tree", tree, "-p", head)
		if e != nil {
			return c.emitError(e)
		}
		if _, e = gitText(p.GitDir, nil, nil, "update-ref", managedRef, commit, head); e == nil {
			return c.emit(result{Status: status, Project: p.Slug, RequestID: req.RequestID, Commit: commit, Tree: tree, Members: members})
		}
		// CAS lost. Re-evaluate original intent against the new complete snapshot.
		newer, e := headCommit(p)
		if e != nil {
			return c.emitError(e)
		}
		if _, e = gitText(p.GitDir, nil, nil, "merge-base", "--is-ancestor", head, newer); e != nil {
			return c.emitError(fail("CONCURRENT_REQUEST_CONFLICT", "managed Git history was replaced during publication"))
		}
		if prev, _, e := findAudit(p, newer, req.RequestID); e != nil {
			return c.emitError(e)
		} else if prev.RequestID != "" {
			continue
		}
		rerun, _, e := stage(p, newer, req.Operations)
		if e != nil {
			return c.emitError(e)
		}
		if !sameMembers(members, rerun) {
			return c.emitError(fail("CONCURRENT_REQUEST_CONFLICT", "concurrent update changed the original operation outcomes; reread and resubmit"))
		}
	}
	return c.emitError(fail("CONCURRENT_REQUEST_CONFLICT", "bounded Git publication retry limit exceeded"))
}
func sameMembers(a, b []member) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Status != b[i].Status || a[i].Error == nil != (b[i].Error == nil) {
			return false
		}
		if a[i].Error != nil && a[i].Error.Code != b[i].Error.Code {
			return false
		}
	}
	return true
}
func classify(m []member) string {
	success, failures := 0, 0
	for _, o := range m {
		if o.Status == "failed" {
			failures++
		} else {
			success++
		}
	}
	if failures == 0 {
		return "applied"
	}
	if success == 0 {
		return "failed"
	}
	return "partial"
}
func stage(p project, head string, ops []operation) ([]member, map[string]entry, error) {
	state := map[string]entry{}
	changed := map[string]entry{}
	members := make([]member, 0, len(ops))
	for _, o := range ops {
		file, seen := state[o.Path]
		if !seen {
			var e error
			file, e = readEntry(p, head, o.Path)
			if e != nil {
				return nil, nil, e
			}
		}
		m := member{ID: o.ID, Op: o.Op, Path: o.Path, Status: "applied"}
		switch o.Op {
		case "create":
			if file.Exists {
				m.Status = "failed"
				m.Error = &diagnostic{"DOCUMENT_EXISTS", "document already exists"}
			} else {
				file = entry{Mode: "100644", Bytes: []byte(o.Content), Exists: true}
				changed[o.Path] = file
			}
		case "edit":
			if !file.Exists {
				m.Status = "failed"
				m.Error = &diagnostic{"DOCUMENT_NOT_FOUND", "document does not exist"}
			} else {
				bodyStart, e := markdownBodyStart(file.Bytes)
				if e != nil {
					m.Status = "failed"
					m.Error = coded(e)
				} else {
					body := file.Bytes[bodyStart:]
					count := bytes.Count(body, []byte(o.Old))
					if count != 1 {
						m.Status = "failed"
						code := "EDIT_TARGET_MISSING"
						if count > 1 {
							code = "EDIT_TARGET_AMBIGUOUS"
						}
						m.Error = &diagnostic{code, "expected unique exact body target"}
					}
					if count == 1 {
						replacement := bytes.Replace(body, []byte(o.Old), []byte(o.New), 1)
						if bytes.Equal(body, replacement) {
							m.Status = "no_op"
						} else {
							file.Bytes = append(append([]byte(nil), file.Bytes[:bodyStart]...), replacement...)
							changed[o.Path] = file
						}
					}
				}
			}
		}
		state[o.Path] = file
		members = append(members, m)
	}
	return members, changed, nil
}
func markdownBodyStart(b []byte) (int, error) {
	// Identify the first YAML fence conservatively; never edit known YAML bytes.
	// A BOM may precede the opening fence.
	offset := 0
	if bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
		offset = 3
	}
	b2 := b[offset:]
	if !bytes.HasPrefix(b2, []byte("---\n")) && !bytes.HasPrefix(b2, []byte("---\r\n")) {
		return 0, nil
	}
	crlf := bytes.HasPrefix(b2, []byte("---\r\n"))
	sep := []byte("\n---\n")
	start := 3
	if crlf {
		sep = []byte("\r\n---\r\n")
	}
	i := bytes.Index(b2[start:], sep)
	if i < 0 {
		return 0, fail("MALFORMED_FRONTMATTER", "no closing YAML frontmatter fence")
	}
	return offset + start + i + len(sep), nil
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
	for name, file := range changes {
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
