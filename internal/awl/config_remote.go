package awl

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// validRemote validates the narrow plain-YAML scalar that the experimental
// config writer supports. An empty string means an intentionally local-only
// project, not an uninitialized or invalid binding.
func validRemote(remote string) error {
	if remote == "" {
		return nil
	}
	if strings.HasPrefix(remote, "-") || strings.ContainsAny(remote, " \t\n\r#*\\\"'{}[]") {
		return fail("INVALID_REMOTE", "remote URL contains unsupported or unsafe characters")
	}
	for _, r := range remote {
		if r < 0x20 || r == 0x7f {
			return fail("INVALID_REMOTE", "remote URL contains control characters")
		}
	}
	if strings.Contains(remote, "://") {
		parsed, e := url.Parse(remote)
		if e != nil || parsed.Scheme == "" {
			return fail("INVALID_REMOTE", "invalid remote URL")
		}
		if parsed.User != nil {
			if _, hasPassword := parsed.User.Password(); hasPassword {
				return fail("INVALID_REMOTE", "remote URL must not embed a password or token")
			}
		}
	}
	return nil
}

// validRemoteBranch delegates branch syntax to Git, while forbidding ambiguous
// reflog expressions and fully qualified refs as user-supplied branch names.
func validRemoteBranch(branch string) error {
	if branch == "" || strings.HasPrefix(branch, "-") || strings.HasPrefix(branch, "refs/") || strings.Contains(branch, "@{") {
		return fail("INVALID_REMOTE_BRANCH", "expected a Git branch name such as worklogs/api")
	}
	out, e := gitText("", nil, nil, "check-ref-format", "--branch", branch)
	if e != nil || out != branch {
		return fail("INVALID_REMOTE_BRANCH", "invalid Git branch name")
	}
	return nil
}

// withRemote inserts a missing scalar without normalizing existing user-owned
// configuration bytes (including comments and CRLF). It accepts only the
// already-validated, minimal schema supported by configRead.
func withRemote(original []byte, section, remote string) ([]byte, error) {
	return withRemoteTarget(original, section, remote, defaultRemoteBranch)
}

func withRemoteTarget(original []byte, section, remote, branch string) ([]byte, error) {
	nl := "\n"
	if bytes.Contains(original, []byte("\r\n")) {
		nl = "\r\n"
	}
	var out bytes.Buffer
	found := false
	for _, line := range bytes.SplitAfter(original, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		text := strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
		out.Write(line)
		if text == section+":" {
			found = true
			if line[len(line)-1] != '\n' {
				out.WriteString(nl)
			}
			out.WriteString("  remote: " + remote + nl)
			if branch != defaultRemoteBranch {
				out.WriteString("  remoteBranch: " + branch + nl)
			}
		}
	}
	if !found {
		if section != "storage" {
			return nil, fail("INVALID_CONFIG", "missing local binding section")
		}
		if len(original) > 0 && original[len(original)-1] != '\n' {
			out.WriteString(nl)
		}
		out.WriteString("storage:" + nl + "  remote: " + remote + nl)
		if branch != defaultRemoteBranch {
			out.WriteString("  remoteBranch: " + branch + nl)
		}
	}
	return out.Bytes(), nil
}

// replaceConfigAtomically is a conservative best-effort single-file update;
// there is no cross-file atomic rename or proven power-loss guarantee. A
// halfway remote-add can be completed by invoking the same command again.
func replaceConfigAtomically(path string, before, after []byte) error {
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() {
		return fail("UNSAFE_PATH", "configuration path must be a regular file")
	}
	actual, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if !bytes.Equal(actual, before) {
		return fail("CONFIG_CHANGED", "configuration changed since validation; retry explicitly")
	}
	tmp, e := os.CreateTemp(filepath.Dir(path), ".awl-remote-*")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	if e = tmp.Chmod(st.Mode().Perm()); e == nil {
		_, e = tmp.Write(after)
	}
	if e == nil {
		e = tmp.Sync()
	}
	if closeErr := tmp.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp.Name(), path)
}

// addRemote only configures a previously empty remote; it never contacts the
// remote, changes the managed Git ref, or replaces a different remote. It is
// restartable if a process stopped between the two configuration updates.
func addRemote(remote, explicit string) (project, bool, error) {
	return addRemoteWithBranch(remote, "", explicit)
}

// An omitted branch reuses the configured target for retry/idempotence.
func addRemoteWithBranch(remote, requestedBranch, explicit string) (project, bool, error) {
	if requestedBranch != "" {
		if e := validRemoteBranch(requestedBranch); e != nil {
			return project{}, false, e
		}
	}
	if remote == "" {
		return project{}, false, fail("REMOTE_REQUIRED", "remote add requires a nonempty URL")
	}
	if e := validRemote(remote); e != nil {
		return project{}, false, e
	}
	root, common, e := source()
	if e != nil {
		return project{}, false, e
	}
	commonDir := filepath.Join(common, "awl")
	if st, err := os.Lstat(commonDir); err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return project{}, false, fail("PROJECT_NOT_INITIALIZED", "run awl init before remote add")
	}
	unlock, e := flock(filepath.Join(commonDir, "init.lock"))
	if e != nil {
		return project{}, false, e
	}
	defer unlock()
	sharedPath := filepath.Join(root, ".awl", "project.yaml")
	localPath := filepath.Join(commonDir, "local.yaml")
	for _, path := range []string{sharedPath, localPath} {
		st, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return project{}, false, fail("PROJECT_NOT_INITIALIZED", "run awl init before remote add")
		}
		if err != nil {
			return project{}, false, err
		}
		if !st.Mode().IsRegular() {
			return project{}, false, fail("UNSAFE_PATH", "configuration must be an ordinary file")
		}
	}
	if e = validateOptionalLocalLink(root, common); e != nil {
		return project{}, false, e
	}
	sharedSlug, sharedRemote, sharedBranch, e := parseSharedTarget(sharedPath)
	if e != nil {
		return project{}, false, e
	}
	bind, e := parseLocal(localPath)
	if e != nil {
		return project{}, false, e
	}
	if explicit != "" && explicit != sharedSlug || bind.Slug != sharedSlug {
		return project{}, false, fail("PROJECT_CONFLICT", "remote add requires the initialized source project binding")
	}
	branch := requestedBranch
	if branch == "" {
		switch {
		case sharedRemote != "":
			branch = sharedBranch
		case bind.Remote != "":
			branch = bind.RemoteBranch
		default:
			branch = defaultRemoteBranch
		}
	}
	for _, target := range []struct{ url, branch string }{
		{sharedRemote, sharedBranch}, {bind.Remote, bind.RemoteBranch},
	} {
		if target.url != "" && (target.url != remote || target.branch != branch) {
			return project{}, false, fail("REMOTE_ALREADY_CONFIGURED", "changing remote URL or branch requires an explicit migration")
		}
	}
	st, e := os.Lstat(bind.LocalRepo)
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return project{}, false, fail("MANAGED_REPO_MISSING", "configured managed repository unavailable")
	}
	// Configuring a remote does not create a Git commit and therefore must not
	// depend on author identity being configured in the source worktree.
	p := project{Root: root, Common: common, Slug: sharedSlug, Remote: remote, RemoteBranch: branch, GitDir: bind.LocalRepo}
	if _, e = headCommit(p); e != nil {
		return project{}, false, e
	}
	if sharedRemote == remote && sharedBranch == branch && bind.Remote == remote && bind.RemoteBranch == branch {
		return p, false, nil
	}
	// A retry also repairs the only supported interrupted states: one config
	// already contains this exact remote and the other is still unconfigured.
	for _, item := range []struct {
		path, section, existing string
	}{
		{sharedPath, "storage", sharedRemote},
		{localPath, "binding", bind.Remote},
	} {
		if item.existing != "" {
			continue
		}
		before, err := os.ReadFile(item.path)
		if err != nil {
			return project{}, false, err
		}
		after, err := withRemoteTarget(before, item.section, remote, branch)
		if err != nil {
			return project{}, false, err
		}
		if err := replaceConfigAtomically(item.path, before, after); err != nil {
			return project{}, false, err
		}
	}
	return p, true, nil
}
