package awl

import "flag"

func (c *cli) commandInit(fs *flag.FlagSet, args []string) int {
	slug := fs.String("slug", "", "optional project slug; defaults to current repository name")
	remote := fs.String("remote", "", "authoritative worklog Git remote for new configuration")
	remoteBranch := fs.String("remote-branch", "", "optional remote branch name (default: awl)")
	repoDir := fs.String("repo-dir", "", "absolute path for a new managed bare Git repository")
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
	if e := fs.Parse(args); e != nil || fs.NArg() != 0 {
		return c.emitError(fail("USAGE", "init accepts optional --slug, --remote, --remote-branch, --repo-dir, --json"))
	}
	repoDirProvided, slugProvided, remoteBranchProvided := false, false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "repo-dir":
			repoDirProvided = true
		case "slug":
			slugProvided = true
		case "remote-branch":
			remoteBranchProvided = true
		}
	})
	if repoDirProvided && *repoDir == "" {
		return c.emitError(fail("INVALID_REPO_DIR", "--repo-dir must name a new absolute directory"))
	}
	if slugProvided && *slug == "" {
		return c.emitError(fail("INVALID_SLUG", "--slug must not be empty; omit it to infer from the Git repository"))
	}
	if remoteBranchProvided && *remoteBranch == "" {
		return c.emitError(fail("INVALID_REMOTE_BRANCH", "--remote-branch must be a nonempty Git branch"))
	}
	p, e := initializeWithBranch(*slug, *remote, *remoteBranch, *repoDir, c.explicit)
	if e != nil {
		return c.emitError(e)
	}
	return c.emit(result{Status: "initialized", Project: p.Slug})
}

func (c *cli) commandRemote(fs *flag.FlagSet, args []string) int {
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
	remoteBranch := fs.String("branch", "", "optional remote branch name (default: awl)")
	if e := fs.Parse(args); e != nil || fs.NArg() != 2 || fs.Arg(0) != "add" {
		return c.emitError(fail("USAGE", "remote add requires one Git URL and optional --branch and --json"))
	}
	branchProvided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "branch" {
			branchProvided = true
		}
	})
	if branchProvided && *remoteBranch == "" {
		return c.emitError(fail("INVALID_REMOTE_BRANCH", "--branch must be a nonempty Git branch"))
	}
	p, changed, e := addRemoteWithBranch(fs.Arg(1), *remoteBranch, c.explicit)
	if e != nil {
		return c.emitError(e)
	}
	status := "remote_added"
	if !changed {
		status = "remote_unchanged"
	}
	return c.emit(result{Status: status, Project: p.Slug})
}
