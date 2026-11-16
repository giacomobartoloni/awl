package awl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

func (c *cli) writeRequest(req request) int {
	p, e := resolve(c.explicit)
	if e != nil {
		return c.emitError(e)
	}
	// Only mutations require Git attribution. Reads and remote configuration
	// remain usable without a current author identity.
	p.UserName, p.UserEmail, e = identity(p.Root)
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
		if o.Op != "create" && o.Op != "edit" && o.Op != "metadata" {
			return c.emitError(fail("INVALID_REQUEST", "only create/edit/metadata are in this spike"))
		}
		if o.Op == "metadata" {
			if e := validateMetadataEdits(o.Metadata); e != nil {
				return c.emitError(e)
			}
		} else if len(o.Metadata) != 0 {
			return c.emitError(fail("INVALID_REQUEST", "only metadata operations accept metadata edits"))
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
		// A failed update-ref is not proof of a CAS race. Observe the actual
		// managed ref before deciding whether publication succeeded or can be
		// safely retried against a different snapshot.
		newer, observeErr := headCommit(p)
		if observeErr != nil {
			return c.emitError(fail("PUBLICATION_OUTCOME_UNKNOWN", "Git ref update failed and the resulting head cannot be verified; inspect and retry using the same request ID"))
		}
		if newer == commit {
			// The ref now points at our audited commit, despite the subprocess
			// reporting failure (for example, after an interrupted response).
			return c.emit(result{Status: status, Project: p.Slug, RequestID: req.RequestID, Commit: commit, Tree: tree, Members: members})
		}
		if newer == head {
			return c.emitError(fail("GIT_PUBLICATION_FAILED", "Git ref update failed without advancing the managed head; request was not published and can be retried"))
		}
		// Another writer advanced the ref. Re-evaluate the original intent
		// against the new complete snapshot, without dropping prior results.
		if _, e = gitText(p.GitDir, nil, nil, "merge-base", "--is-ancestor", head, newer); e != nil {
			return c.emitError(fail("CONCURRENT_REQUEST_CONFLICT", "managed Git history was replaced during publication"))
		}
		if prev, _, e := findAudit(p, newer, req.RequestID); e != nil {
			return c.emitError(e)
		} else if prev.RequestID != "" {
			continue
		}
		// A lost CAS must not overwrite an independently changed YAML target.
		for _, o := range req.Operations {
			if o.Op != "metadata" {
				continue
			}
			before, oldErr := readEntry(p, head, o.Path)
			after, newErr := readEntry(p, newer, o.Path)
			if oldErr != nil || newErr != nil || !before.Exists || !after.Exists ||
				!metadataTargetsStable(before.Bytes, after.Bytes, o.Metadata) {
				return c.emitError(fail("CONCURRENT_REQUEST_CONFLICT", "concurrent metadata target changed; reread and resubmit"))
			}
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
		case "metadata":
			if !file.Exists {
				m.Status = "failed"
				m.Error = &diagnostic{"DOCUMENT_NOT_FOUND", "document does not exist"}
			} else {
				replacement, e := metadataApply(file.Bytes, o.Metadata)
				if e != nil {
					m.Status = "failed"
					m.Error = coded(e)
				} else if bytes.Equal(file.Bytes, replacement) {
					m.Status = "no_op"
				} else {
					file.Bytes = replacement
					changed[o.Path] = file
				}
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
