package awl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// withRemoteBranch changes only the remoteBranch scalar of an already
// validated minimal config. It preserves unrelated bytes, comments and CRLF.
func withRemoteBranch(original []byte, branch string) ([]byte, error) {
	nl := "\n"
	if bytes.Contains(original, []byte("\r\n")) {
		nl = "\r\n"
	}
	lines := bytes.SplitAfter(original, []byte("\n"))
	hasBranch := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(string(line)), "remoteBranch:") {
			hasBranch = true
			break
		}
	}
	var out bytes.Buffer
	sawRemote := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		value := strings.TrimSpace(string(line))
		switch {
		case strings.HasPrefix(value, "remoteBranch:"):
			if branch != defaultRemoteBranch {
				ending := ""
				if bytes.HasSuffix(line, []byte("\r\n")) {
					ending = "\r\n"
				} else if bytes.HasSuffix(line, []byte("\n")) {
					ending = "\n"
				}
				out.WriteString("  remoteBranch: " + branch + ending)
			}
		case strings.HasPrefix(value, "remote:"):
			sawRemote = true
			out.Write(line)
			if !hasBranch && branch != defaultRemoteBranch {
				if !bytes.HasSuffix(line, []byte("\n")) {
					out.WriteString(nl)
				}
				out.WriteString("  remoteBranch: " + branch + nl)
			}
		default:
			out.Write(line)
		}
	}
	if !sawRemote {
		return nil, fail("INVALID_CONFIG", "configured remote field is missing")
	}
	return out.Bytes(), nil
}

// setRemoteBranch changes the configured Git destination only; it never
// fetches, pushes, modifies managed history or relocates the local store.
// An already verified remote history needs a separate, explicit migration.
// Retrying after a one-file interruption is supported when one side already
// matches the requested branch and project/remote identity still agree.
func setRemoteBranch(requested, explicit string) (project, bool, error) {
	if err := validRemoteBranch(requested); err != nil {
		return project{}, false, err
	}
	root, common, err := source()
	if err != nil {
		return project{}, false, err
	}
	commonDir := filepath.Join(common, "awl")
	if st, e := os.Lstat(commonDir); e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return project{}, false, fail("PROJECT_NOT_INITIALIZED", "run awl init before changing the remote branch")
	}
	// Lock against sync first: no sync may resolve an intermediate two-file
	// configuration. The init lock also serializes configuration writers.
	unlockSync, err := flock(filepath.Join(commonDir, "sync.lock"))
	if err != nil {
		return project{}, false, err
	}
	defer unlockSync()
	unlockInit, err := flock(filepath.Join(commonDir, "init.lock"))
	if err != nil {
		return project{}, false, err
	}
	defer unlockInit()

	sharedPath := filepath.Join(root, ".awl", "project.yaml")
	localPath := filepath.Join(commonDir, "local.yaml")
	for _, path := range []string{sharedPath, localPath} {
		st, e := os.Lstat(path)
		if os.IsNotExist(e) {
			return project{}, false, fail("PROJECT_NOT_INITIALIZED", "missing project configuration; run awl init")
		}
		if e != nil {
			return project{}, false, e
		}
		if !st.Mode().IsRegular() {
			return project{}, false, fail("UNSAFE_PATH", "configuration must be an ordinary file")
		}
	}
	if err := validateOptionalLocalLink(root, common); err != nil {
		return project{}, false, err
	}
	slug, remote, sharedBranch, err := parseSharedTarget(sharedPath)
	if err != nil {
		return project{}, false, err
	}
	bind, err := parseLocal(localPath)
	if err != nil {
		return project{}, false, err
	}
	if (explicit != "" && explicit != slug) || bind.Slug != slug || bind.Remote != remote {
		return project{}, false, fail("PROJECT_CONFLICT", "remote branch update requires matching project and remote URL bindings")
	}
	if remote == "" {
		return project{}, false, fail("REMOTE_NOT_CONFIGURED", "configure a Git remote with awl remote add first")
	}
	p := project{Root: root, Common: common, Slug: slug, Remote: remote, RemoteBranch: requested, GitDir: bind.LocalRepo}
	st, err := os.Lstat(bind.LocalRepo)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return project{}, false, fail("MANAGED_REPO_MISSING", "configured managed repository unavailable")
	}
	if _, err := headCommit(p); err != nil {
		return project{}, false, err
	}
	if sharedBranch == requested && bind.RemoteBranch == requested {
		return p, false, nil
	}
	if sharedBranch != bind.RemoteBranch && sharedBranch != requested && bind.RemoteBranch != requested {
		return project{}, false, fail("PROJECT_CONFLICT", "both remote branch bindings disagree with the requested branch")
	}
	verified, err := gitText(bind.LocalRepo, nil, nil, "for-each-ref", "--format=%(objectname)", syncVerifiedRef)
	if err != nil {
		return project{}, false, err
	}
	if verified != "" {
		return project{}, false, fail("REMOTE_BRANCH_MIGRATION_REQUIRED", "the previous remote branch has verified history; an explicit history migration is required")
	}
	for _, path := range []string{sharedPath, localPath} {
		before, e := os.ReadFile(path)
		if e != nil {
			return project{}, false, e
		}
		current := sharedBranch
		if path == localPath {
			current = bind.RemoteBranch
		}
		if current == requested {
			continue
		}
		after, e := withRemoteBranch(before, requested)
		if e != nil {
			return project{}, false, e
		}
		if e := replaceConfigAtomically(path, before, after); e != nil {
			return project{}, false, e
		}
	}
	return p, true, nil
}
