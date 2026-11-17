package awl

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// repositorySlug derives a stable candidate from the source Git repository.
// For ordinary Git repositories and linked worktrees, the shared .git directory
// identifies the same original repository even if worktree folder names differ.
// Other Git layouts fall back to the current worktree root.
func repositorySlug(root, common string) (string, error) {
	name := filepath.Base(root)
	if filepath.Base(common) == ".git" {
		name = filepath.Base(filepath.Dir(common))
	}
	var normalized []byte
	lastSeparator := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			normalized = append(normalized, byte(r))
			lastSeparator = false
		case len(normalized) > 0 && !lastSeparator:
			normalized = append(normalized, '-')
			lastSeparator = true
		}
	}
	slug := strings.TrimRight(string(normalized), "-")
	if slug == "" {
		return "", fail("INVALID_SLUG", "cannot derive a slug from the current Git repository name; supply --slug")
	}
	if slug[0] >= '0' && slug[0] <= '9' {
		slug = "repo-" + slug
	}
	if len(slug) > 64 {
		slug = strings.TrimRight(slug[:64], "-")
	}
	if !slugPattern.MatchString(slug) {
		return "", fail("INVALID_SLUG", "cannot derive a valid slug from the current Git repository name; supply --slug")
	}
	return slug, nil
}

const defaultRemoteBranch = "awl"

type binding struct {
	Slug         string
	Remote       string
	RemoteBranch string
	LocalRepo    string
}
type project struct {
	Root         string
	Common       string
	Slug         string
	Remote       string
	RemoteBranch string
	GitDir       string
	UserName     string
	UserEmail    string
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
	return validateOptionalLocalLinkWithIgnore(root, common, false)
}

func validateOptionalLocalLinkWithIgnore(root, common string, allowUnignored bool) error {
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
	if !allowUnignored {
		if _, e := gitSource(root, "check-ignore", "-q", "--", ".awl/local.yaml"); e != nil {
			return fail("LOCAL_CONFIG_NOT_IGNORED", "optional .awl/local.yaml symlink must be Git-ignored; run awl init to add the ignore rule")
		}
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

func resolve(explicit string) (project, error) {
	root, common, e := source()
	if e != nil {
		return project{}, e
	}
	projectFile := filepath.Join(root, ".awl", "project.yaml")
	if st, e := os.Lstat(projectFile); e == nil && st.Mode()&os.ModeSymlink != 0 {
		return project{}, fail("UNSAFE_PATH", "source project configuration cannot be a symlink")
	}
	slug, remote, branch, e := parseSharedTarget(projectFile)
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
	if bind.Slug != slug || bind.Remote != remote || bind.RemoteBranch != branch {
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
	return project{Root: root, Common: common, Slug: slug, Remote: remote, RemoteBranch: branch, GitDir: bind.LocalRepo}, nil
}
