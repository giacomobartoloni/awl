package awl

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type binding struct {
	Slug      string
	Remote    string
	LocalRepo string
}
type project struct {
	Root      string
	Common    string
	Slug      string
	Remote    string
	GitDir    string
	UserName  string
	UserEmail string
}

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

// validateRepoDir accepts an explicit new bare-repository location, not a
// base directory. The parent must already exist, and symlinked parents are
// rejected so initialization cannot silently redirect the chosen location.
func validateRepoDir(path, root, common string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.TrimSpace(path) != path ||
		strings.ContainsAny(path, "\x00\r\n\t#\\\"'{}[]") {
		return fail("INVALID_REPO_DIR", "--repo-dir requires a clean absolute path compatible with the local binding")
	}
	parent := filepath.Dir(path)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return fail("INVALID_REPO_DIR", "--repo-dir parent must exist without symlink redirects")
	}
	st, err := os.Lstat(parent)
	if err != nil || !st.IsDir() {
		return fail("INVALID_REPO_DIR", "--repo-dir parent must be an existing directory")
	}
	if pathWithin(root, path) || pathWithin(common, path) {
		return fail("INVALID_REPO_DIR", "--repo-dir must be outside the source worktree and its Git directory")
	}
	return nil
}

func pathWithin(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func initialize(slug, remote, repoDir, explicit string) (project, error) {
	root, common, e := source()
	if e != nil {
		return project{}, e
	}
	// Malformed first-use arguments must not create even the .awl directory.
	firstProjectFile := filepath.Join(root, ".awl", "project.yaml")
	if _, e = os.Lstat(firstProjectFile); os.IsNotExist(e) && !slugPattern.MatchString(slug) {
		return project{}, fail("INIT_INPUT_REQUIRED", "new project needs --slug; --remote is optional")
	}
	if explicit != "" && slug != "" && explicit != slug {
		return project{}, fail("PROJECT_CONFLICT", "explicit project does not match init slug")
	}
	if repoDir != "" {
		if e = validateRepoDir(repoDir, root, common); e != nil {
			return project{}, e
		}
		// When no canonical binding exists, refuse an occupied target before
		// creating .awl or any Git-common-dir bookkeeping. Reinitialization
		// with the same already-bound location remains permitted below.
		if _, bindingErr := os.Lstat(filepath.Join(common, "awl", "local.yaml")); os.IsNotExist(bindingErr) {
			if _, targetErr := os.Lstat(repoDir); targetErr == nil {
				return project{}, fail("REPO_DIR_EXISTS", "explicit repository directory already exists")
			} else if !os.IsNotExist(targetErr) {
				return project{}, targetErr
			}
		} else if bindingErr != nil {
			return project{}, bindingErr
		}
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
			return project{}, fail("REMOTE_CHANGE_REQUIRES_COMMAND", "use awl remote add to configure an unconfigured remote; replacing an existing remote is unsupported")
		}
		slug, remote = existingSlug, existingRemote
	} else if !os.IsNotExist(e) {
		return project{}, e
	} else if !slugPattern.MatchString(slug) {
		return project{}, fail("INIT_INPUT_REQUIRED", "new project needs --slug; --remote is optional")
	}
	if explicit != "" && explicit != slug {
		return project{}, fail("PROJECT_CONFLICT", "--project disagrees with source project")
	}
	if e = validRemote(remote); e != nil {
		return project{}, e
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
		if repoDir != "" && repoDir != bind.LocalRepo {
			return project{}, fail("PROJECT_CONFLICT", "--repo-dir cannot relocate an initialized project")
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
	dest := repoDir
	if dest == "" {
		data, err := appDataHome()
		if err != nil {
			return project{}, err
		}
		base := filepath.Join(data, "awl", "repos")
		if err = os.MkdirAll(base, 0700); err != nil {
			return project{}, err
		}
		nonce, err := randomID()
		if err != nil {
			return project{}, err
		}
		dest = filepath.Join(base, slug+"-"+nonce)
	} else {
		// Only a new, explicitly named directory is accepted. Never let
		// git init --bare reuse or modify an existing user-owned path.
		if err := os.Mkdir(dest, 0700); err != nil {
			if os.IsExist(err) {
				return project{}, fail("REPO_DIR_EXISTS", "explicit repository directory already exists")
			}
			return project{}, fail("INVALID_REPO_DIR", "cannot create explicit repository directory: "+err.Error())
		}
	}
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
		body := fmt.Sprintf("schema: 1\nproject:\n  slug: %s\n", slug)
		if remote != "" {
			body += fmt.Sprintf("storage:\n  remote: %s\n", remote)
		}
		if e = atomicNewFile(projPath, []byte(body)); e != nil {
			return project{}, e
		}
	} else if e != nil {
		return project{}, e
	}
	local := fmt.Sprintf("schema: 1\nbinding:\n  slug: %s\n", slug)
	if remote != "" {
		local += fmt.Sprintf("  remote: %s\n", remote)
	}
	local += fmt.Sprintf("storage:\n  localRepo: %s\n", dest)
	if e = atomicNewFile(localPath, []byte(local)); e != nil {
		return project{}, e
	}
	return p, nil
}

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

// withRemote inserts a missing scalar without normalizing existing user-owned
// configuration bytes (including comments and CRLF). It accepts only the
// already-validated, minimal schema supported by configRead.
func withRemote(original []byte, section, remote string) ([]byte, error) {
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
	sharedSlug, sharedRemote, e := parseShared(sharedPath)
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
	for _, existing := range []string{sharedRemote, bind.Remote} {
		if existing != "" && existing != remote {
			return project{}, false, fail("REMOTE_ALREADY_CONFIGURED", "a different remote is already configured; replacement is unsupported")
		}
	}
	st, e := os.Lstat(bind.LocalRepo)
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return project{}, false, fail("MANAGED_REPO_MISSING", "configured managed repository unavailable")
	}
	// Configuring a remote does not create a Git commit and therefore must not
	// depend on author identity being configured in the source worktree.
	p := project{Root: root, Common: common, Slug: sharedSlug, Remote: remote, GitDir: bind.LocalRepo}
	if _, e = headCommit(p); e != nil {
		return project{}, false, e
	}
	if sharedRemote == remote && bind.Remote == remote {
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
		after, err := withRemote(before, item.section, remote)
		if err != nil {
			return project{}, false, err
		}
		if err := replaceConfigAtomically(item.path, before, after); err != nil {
			return project{}, false, err
		}
	}
	return p, true, nil
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
	// Identity is only required when committing a mutation, not when reading.
	return project{Root: root, Common: common, Slug: slug, Remote: remote, GitDir: bind.LocalRepo}, nil
}

func configRead(file string, allowed map[string]bool) (map[string]string, error) {
	b, e := os.ReadFile(file)
	if e != nil {
		return nil, e
	}
	m := map[string]string{}
	scope := ""
	seenSections := map[string]bool{}
	for _, raw := range strings.Split(string(b), "\n") {
		// Accept CRLF as well as LF without rewriting the user's config bytes.
		raw = strings.TrimSuffix(raw, "\r")
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if strings.HasSuffix(s, ":") && raw == s {
			scope = strings.TrimSuffix(s, ":")
			if seenSections[scope] {
				return nil, fail("INVALID_CONFIG", "duplicate YAML section")
			}
			seenSections[scope] = true
			if !allowed[scope] {
				return nil, fail("INVALID_CONFIG", "unknown config section")
			}
			continue
		}
		name, v, ok := strings.Cut(s, ":")
		if !ok {
			return nil, fail("INVALID_CONFIG", "malformed YAML field")
		}
		key := strings.TrimSpace(name)
		if strings.HasPrefix(raw, "  ") && scope != "" {
			key = scope + "." + key
		} else if raw != s {
			return nil, fail("INVALID_CONFIG", "unsupported YAML indentation")
		} else {
			scope = ""
		}
		if !allowed[key] {
			return nil, fail("INVALID_CONFIG", "unknown YAML field: "+key)
		}
		if _, duplicate := m[key]; duplicate {
			return nil, fail("INVALID_CONFIG", "duplicate YAML field: "+key)
		}
		value := strings.TrimSpace(v)
		if value == "" || strings.ContainsAny(value, "\n\r#\"'{}[]") || strings.HasPrefix(value, "!") {
			return nil, fail("INVALID_CONFIG", "unsupported YAML scalar: "+key)
		}
		m[key] = value
	}
	if m["schema"] != "1" {
		return nil, fail("INVALID_CONFIG", "expected schema: 1")
	}
	return m, nil
}
func parseShared(path string) (string, string, error) {
	m, e := configRead(path, map[string]bool{"project": true, "storage": true, "schema": true, "project.slug": true, "storage.remote": true})
	if e != nil {
		return "", "", e
	}
	slug, remote := m["project.slug"], m["storage.remote"]
	if !slugPattern.MatchString(slug) {
		return "", "", fail("INVALID_CONFIG", "missing or invalid project slug")
	}
	if e = validRemote(remote); e != nil {
		return "", "", e
	}
	return slug, remote, nil
}
func parseLocal(path string) (binding, error) {
	m, e := configRead(path, map[string]bool{"binding": true, "storage": true, "schema": true, "binding.slug": true, "binding.remote": true, "storage.localRepo": true})
	if e != nil {
		return binding{}, e
	}
	b := binding{m["binding.slug"], m["binding.remote"], m["storage.localRepo"]}
	if !slugPattern.MatchString(b.Slug) || !filepath.IsAbs(b.LocalRepo) {
		return b, fail("INVALID_CONFIG", "incomplete local binding")
	}
	if e = validRemote(b.Remote); e != nil {
		return b, e
	}
	return b, nil
}

func flock(file string) (func(), error) {
	f, e := os.OpenFile(file, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		f.Close()
		return nil, e
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
