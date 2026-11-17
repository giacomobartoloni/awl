package awl

import (
	"fmt"
	"os"
	"path/filepath"
)

func initialize(slug, remote, repoDir, explicit string) (project, error) {
	return initializeWithBranch(slug, remote, "", repoDir, explicit)
}

// initializeWithBranch leaves legacy projects on the "awl" remote branch.
func initializeWithBranch(slug, remote, requestedBranch, repoDir, explicit string) (project, error) {
	return initializeWithBranchAndIgnore(slug, remote, requestedBranch, repoDir, explicit, false)
}

func initializeWithBranchAndIgnore(slug, remote, requestedBranch, repoDir, explicit string, updateGitignore bool) (project, error) {
	branchProvided := requestedBranch != ""
	if branchProvided {
		if e := validRemoteBranch(requestedBranch); e != nil {
			return project{}, e
		}
	}
	root, common, e := source()
	if e != nil {
		return project{}, e
	}
	// A new project infers its slug before creating any configuration or
	// store. Re-initializing an existing project uses its recorded identity.
	firstProjectFile := filepath.Join(root, ".awl", "project.yaml")
	// Reject a branch-only first initialization before creating any project
	// directories. Reinitialization may use an already configured URL.
	if branchProvided && remote == "" {
		if _, check := os.Lstat(firstProjectFile); os.IsNotExist(check) {
			return project{}, fail("REMOTE_REQUIRED", "--remote-branch requires a remote URL")
		} else if check != nil {
			return project{}, check
		}
	}
	if _, e = os.Lstat(firstProjectFile); os.IsNotExist(e) {
		if slug == "" {
			slug, e = repositorySlug(root, common)
			if e != nil {
				return project{}, e
			}
		}
		if !slugPattern.MatchString(slug) {
			return project{}, fail("INVALID_SLUG", "new project requires a valid --slug")
		}
	} else if e != nil {
		return project{}, e
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
	if e = validateOptionalLocalLink(root, common, updateGitignore); e != nil {
		return project{}, e
	}
	if st, check := os.Lstat(projPath); check == nil && st.Mode()&os.ModeSymlink != 0 {
		return project{}, fail("UNSAFE_PATH", "source project configuration must not be a symlink")
	}
	if _, e = os.Stat(projPath); e == nil {
		existingSlug, existingRemote, existingBranch, e := parseSharedTarget(projPath)
		if e != nil {
			return project{}, e
		}
		if slug != "" && slug != existingSlug {
			return project{}, fail("PROJECT_CONFLICT", "init slug differs from existing configuration")
		}
		if remote != "" && remote != existingRemote {
			return project{}, fail("REMOTE_CHANGE_REQUIRES_COMMAND", "use awl remote add to configure an unconfigured remote; replacing an existing remote is unsupported")
		}
		if branchProvided && existingRemote == "" {
			return project{}, fail("REMOTE_REQUIRED", "--remote-branch requires a remote URL")
		}
		if branchProvided && requestedBranch != existingBranch {
			return project{}, fail("REMOTE_ALREADY_CONFIGURED", "changing the remote branch requires a separate migration")
		}
		slug, remote, requestedBranch = existingSlug, existingRemote, existingBranch
	} else if !os.IsNotExist(e) {
		return project{}, e
	} else if !slugPattern.MatchString(slug) {
		return project{}, fail("INVALID_SLUG", "new project requires a valid --slug")
	}
	if explicit != "" && explicit != slug {
		return project{}, fail("PROJECT_CONFLICT", "--project disagrees with source project")
	}
	if e = validRemote(remote); e != nil {
		return project{}, e
	}
	if branchProvided && remote == "" {
		return project{}, fail("REMOTE_REQUIRED", "--remote-branch requires a remote URL")
	}
	remoteBranch := requestedBranch
	if remoteBranch == "" {
		remoteBranch = defaultRemoteBranch
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
		if bind.Slug != slug || bind.Remote != remote || bind.RemoteBranch != remoteBranch {
			return project{}, fail("PROJECT_CONFLICT", "canonical project binding conflicts with source configuration")
		}
		if repoDir != "" && repoDir != bind.LocalRepo {
			return project{}, fail("PROJECT_CONFLICT", "--repo-dir cannot relocate an initialized project")
		}
		if _, e = os.Stat(bind.LocalRepo); e != nil {
			return project{}, fail("MANAGED_REPO_MISSING", "configured managed repository is missing")
		}
		p := project{Root: root, Common: common, Slug: slug, Remote: remote, RemoteBranch: remoteBranch, GitDir: bind.LocalRepo, UserName: user, UserEmail: email}
		if _, e = headCommit(p); e != nil {
			return project{}, e
		}
		if updateGitignore {
			if e = ensureLocalConfigIgnored(root); e != nil {
				return project{}, e
			}
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
	p := project{Root: root, Common: common, Slug: slug, Remote: remote, RemoteBranch: remoteBranch, GitDir: dest, UserName: user, UserEmail: email}
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
			if remoteBranch != defaultRemoteBranch {
				body += fmt.Sprintf("  remoteBranch: %s\n", remoteBranch)
			}
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
		if remoteBranch != defaultRemoteBranch {
			local += fmt.Sprintf("  remoteBranch: %s\n", remoteBranch)
		}
	}
	local += fmt.Sprintf("storage:\n  localRepo: %s\n", dest)
	if e = atomicNewFile(localPath, []byte(local)); e != nil {
		return project{}, e
	}
	if updateGitignore {
		if e = ensureLocalConfigIgnored(root); e != nil {
			return project{}, e
		}
	}
	return p, nil
}
