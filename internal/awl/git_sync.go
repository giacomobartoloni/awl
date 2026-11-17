package awl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// These references belong to the managed bare store, not the source Git repo.
// fetched is disposable observation; verified is the last confirmed authority.
const (
	syncFetchedRef  = "refs/awl/fetched"
	syncVerifiedRef = "refs/awl/verified"
	syncMaxCommits  = 5000
)

func (c *cli) commandSync(fs *flag.FlagSet, args []string) int {
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable sync result")
	if e := fs.Parse(args); e != nil || fs.NArg() != 0 {
		return c.emitError(fail("USAGE", "sync accepts only --json; merge and unrelated-history options are not implemented"))
	}
	// Resolve the project only after taking the sync lock. A branch update
	// holds that lock for both config writes, so sync cannot use a stale target.
	_, common, e := source()
	if e != nil {
		return c.emitError(e)
	}
	// Do not create synchronization lock state in an uninitialized source
	// repository. The canonical directory is created only by explicit init.
	commonDir := filepath.Join(common, "awl")
	st, e := os.Lstat(commonDir)
	if os.IsNotExist(e) {
		return c.emitError(fail("PROJECT_NOT_INITIALIZED", "run awl init before syncing"))
	}
	if e != nil {
		return c.emitError(e)
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return c.emitError(fail("UNSAFE_PATH", "canonical awl configuration directory is unsafe"))
	}
	unlock, e := flock(filepath.Join(commonDir, "sync.lock"))
	if e != nil {
		return c.emitError(e)
	}
	defer unlock()
	p, e := resolve(c.explicit)
	if e != nil {
		return c.emitError(e)
	}
	if p.Remote == "" {
		return c.emitError(fail("REMOTE_NOT_CONFIGURED", "configure a Git remote with awl remote add before syncing"))
	}

	status, local, remote, e := syncFastForward(p)
	if e != nil {
		return c.emitError(e)
	}
	return c.emit(result{
		Status: status, Project: p.Slug, Commit: local,
		RemoteCommit: remote, RemoteBranch: p.RemoteBranch,
	})
}

func remoteRef(p project) string {
	return "refs/heads/" + p.RemoteBranch
}

func remoteGit(p project, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), localGitTimeout)
	defer cancel()
	return gitContext(ctx, p.GitDir, []string{"GIT_TERMINAL_PROMPT=0"}, nil, args...)
}

func gitExitCode(err error, code int) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == code
}

// remoteHead distinguishes an absent selected branch from a failed remote
// operation. Other branches in the same remote repository are irrelevant.
func remoteHead(p project) (string, error) {
	ref := remoteRef(p)
	out, e := remoteGit(p, "ls-remote", "--exit-code", "--refs", p.Remote, ref)
	if e != nil {
		if gitExitCode(e, 2) {
			return "", nil
		}
		return "", fail("SYNC_REMOTE_UNAVAILABLE", "cannot read the configured remote Git branch: "+e.Error())
	}
	line := strings.TrimSpace(string(out))
	fields := strings.Fields(line)
	if len(fields) != 2 || fields[1] != ref || !validGitOID(fields[0]) {
		return "", fail("SYNC_REMOTE_INVALID", "remote did not report one exact Git branch ref")
	}
	return fields[0], nil
}

func validGitOID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' && c < 'a' || c > 'f' {
			return false
		}
	}
	return true
}

func optionalGitRef(p project, ref string) (string, error) {
	_, e := git(p.GitDir, nil, nil, "show-ref", "--verify", "--quiet", ref)
	if gitExitCode(e, 1) {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	return gitText(p.GitDir, nil, nil, "rev-parse", "--verify", ref)
}

func gitAncestor(p project, ancestor, descendant string) (bool, error) {
	_, e := git(p.GitDir, nil, nil, "merge-base", "--is-ancestor", ancestor, descendant)
	if gitExitCode(e, 1) {
		return false, nil
	}
	return e == nil, e
}

func fetchRemoteHead(p project, observed string) error {
	refspec := "+" + remoteRef(p) + ":" + syncFetchedRef
	if _, e := remoteGit(p, "fetch", "--no-tags", "--no-write-fetch-head", p.Remote, refspec); e != nil {
		return fail("SYNC_FETCH_FAILED", "could not fetch the selected remote branch: "+e.Error())
	}
	fetched, e := gitText(p.GitDir, nil, nil, "rev-parse", "--verify", syncFetchedRef+"^{commit}")
	if e != nil || fetched != observed {
		return fail("SYNC_REMOTE_CHANGED", "remote branch changed while fetching; retry sync")
	}
	return nil
}

// A bootstrap is pristine only when the entire reachable local history is one
// unparented awl bootstrap commit with an empty tree. Audit-only commits must
// never be classified as disposable merely because their tree is empty.
func pristineBootstrap(p project, head string) (bool, error) {
	parents, e := gitText(p.GitDir, nil, nil, "rev-list", "--parents", head)
	if e != nil {
		return false, e
	}
	if strings.TrimSpace(parents) != head {
		return false, nil
	}
	message, e := gitText(p.GitDir, nil, nil, "show", "-s", "--format=%B", head)
	if e != nil {
		return false, e
	}
	if strings.TrimSpace(message) != "awl bootstrap" {
		return false, nil
	}
	tree, e := treeOID(p, head)
	if e != nil {
		return false, e
	}
	empty, e := gitText(p.GitDir, nil, []byte{}, "hash-object", "-t", "tree", "--stdin")
	return e == nil && tree == empty, e
}

// Before importing a history, accept only the linear form produced by the
// current awl CLI. A future merge slice can extend this explicit grammar.
func validateImportedHistory(p project, head string) error {
	log, e := gitText(p.GitDir, nil, nil, "rev-list", "--parents", head)
	if e != nil {
		return fail("SYNC_UNSUPPORTED_HISTORY", "cannot inspect imported Git history")
	}
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) > syncMaxCommits {
		return fail("SYNC_HISTORY_LIMIT", "remote history exceeds the current audit-verification limit")
	}
	empty, e := gitText(p.GitDir, nil, []byte{}, "hash-object", "-t", "tree", "--stdin")
	if e != nil {
		return e
	}
	roots := 0
	ids := map[string]bool{}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 1 || len(fields) > 2 {
			return fail("SYNC_UNSUPPORTED_HISTORY", "only linear awl history is supported by this sync increment")
		}
		message, err := gitText(p.GitDir, nil, nil, "show", "-s", "--format=%B", fields[0])
		if err != nil {
			return fail("SYNC_UNSUPPORTED_HISTORY", "cannot read a remote commit message")
		}
		if len(fields) == 1 {
			roots++
			tree, err := treeOID(p, fields[0])
			if err != nil || tree != empty || strings.TrimSpace(message) != "awl bootstrap" {
				return fail("SYNC_UNSUPPORTED_HISTORY", "remote root is not a recognized awl bootstrap")
			}
			continue
		}
		if !strings.HasPrefix(message, "AWL-AUDIT-V1\n") {
			return fail("SYNC_UNSUPPORTED_HISTORY", "remote contains a commit without awl request audit")
		}
		var record audit
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(message, "AWL-AUDIT-V1\n"))), &record); err != nil ||
			record.Schema != 1 || record.RequestID == "" {
			return fail("SYNC_UNSUPPORTED_HISTORY", "remote contains an invalid request audit")
		}
		if ids[record.RequestID] {
			return fail("SYNC_AUDIT_DUPLICATE", "remote contains duplicate request IDs")
		}
		ids[record.RequestID] = true
	}
	if roots != 1 {
		return fail("SYNC_UNSUPPORTED_HISTORY", "remote history must have exactly one awl bootstrap root")
	}
	return validateRemoteTree(p, head)
}

func validateRemoteTree(p project, head string) error {
	raw, e := git(p.GitDir, nil, nil, "ls-tree", "-r", "-z", head)
	if e != nil {
		return fail("SYNC_UNSAFE_REMOTE_TREE", "cannot inspect remote document tree")
	}
	for _, item := range bytes.Split(raw, []byte{0}) {
		if len(item) == 0 {
			continue
		}
		tab := bytes.IndexByte(item, '\t')
		if tab < 0 {
			return fail("SYNC_UNSAFE_REMOTE_TREE", "invalid remote Git tree entry")
		}
		fields := strings.Fields(string(item[:tab]))
		if len(fields) != 3 || fields[1] != "blob" || fields[0] != "100644" && fields[0] != "100755" {
			return fail("SYNC_UNSAFE_REMOTE_TREE", "remote contains non-document or non-file entries")
		}
		path := string(item[tab+1:])
		if _, err := safePath(path); err != nil {
			return fail("SYNC_UNSAFE_REMOTE_TREE", "remote contains unsupported document paths")
		}
	}
	return nil
}

func checkpointRemote(p project, oid string) error {
	if _, e := gitText(p.GitDir, nil, nil, "update-ref", syncVerifiedRef, oid); e != nil {
		return fail("SYNC_CHECKPOINT_FAILED", "remote state was confirmed but its local checkpoint could not be recorded")
	}
	return nil
}

func verifyRemote(p project, wanted string) error {
	actual, e := remoteHead(p)
	if e != nil {
		return fail("SYNC_OUTCOME_UNKNOWN", "remote confirmation is unavailable; retry sync")
	}
	if actual != wanted {
		return fail("SYNC_REMOTE_CHANGED", "remote branch moved during sync; retry to reclassify its history")
	}
	return checkpointRemote(p, actual)
}

func syncFastForward(p project) (status, local, remote string, err error) {
	local, err = headCommit(p)
	if err != nil {
		return "", "", "", err
	}
	known, err := optionalGitRef(p, syncVerifiedRef)
	if err != nil {
		return "", "", "", err
	}
	remote, err = remoteHead(p)
	if err != nil {
		return "", "", "", err
	}
	if remote == "" {
		if known != "" {
			return "", "", "", fail("SYNC_REMOTE_DELETED", "previously verified remote branch is now absent; refusing to recreate it")
		}
		return publishRemote(p, local, "")
	}
	if err = fetchRemoteHead(p, remote); err != nil {
		return "", "", "", err
	}
	if known != "" {
		ok, ancestryErr := gitAncestor(p, known, remote)
		if ancestryErr != nil {
			return "", "", "", ancestryErr
		}
		if !ok {
			return "", "", "", fail("SYNC_REMOTE_REWRITTEN", "remote branch no longer descends from its last verified commit")
		}
	}
	if local == remote {
		if err = verifyRemote(p, remote); err != nil {
			return "", "", "", err
		}
		now, e := headCommit(p)
		if e != nil {
			return "", "", "", e
		}
		if now != remote {
			return "local_pending", now, remote, nil
		}
		return "up_to_date", local, remote, nil
	}
	if ok, e := gitAncestor(p, remote, local); e != nil {
		return "", "", "", e
	} else if ok {
		return publishRemote(p, local, remote)
	}
	pristine, e := pristineBootstrap(p, local)
	if e != nil {
		return "", "", "", e
	}
	isAncestor, e := gitAncestor(p, local, remote)
	if e != nil {
		return "", "", "", e
	}
	if !isAncestor && !pristine {
		return "", "", "", fail("SYNC_DIVERGED", "local and remote histories diverged; conservative merge is not in this increment")
	}
	if err = validateImportedHistory(p, remote); err != nil {
		return "", "", "", err
	}
	if _, e = gitText(p.GitDir, nil, nil, "update-ref", managedRef, remote, local); e != nil {
		actual, readErr := headCommit(p)
		if readErr != nil || actual != remote {
			return "", "", "", fail("SYNC_LOCAL_CHANGED", "managed local history changed during sync; retry without discarding local commits")
		}
	}
	if err = verifyRemote(p, remote); err != nil {
		return "", "", "", err
	}
	now, e := headCommit(p)
	if e != nil {
		return "", "", "", e
	}
	if now != remote {
		return "local_pending", now, remote, nil
	}
	if isAncestor {
		return "fast_forwarded", remote, remote, nil
	}
	return "adopted", remote, remote, nil
}

// The only publication mechanism is an ordinary non-forcing Git push. A
// negative/ambiguous push response is checked against the remote before
// declaring the attempt failed or successful.
func publishRemote(p project, local, before string) (status, newLocal, remote string, err error) {
	actual, e := headCommit(p)
	if e != nil || actual != local {
		return "", "", "", fail("SYNC_LOCAL_CHANGED", "local history advanced while preparing the push; retry sync")
	}
	_, pushErr := remoteGit(p, "push", "--porcelain", p.Remote, managedRef+":"+remoteRef(p))
	seen, readErr := remoteHead(p)
	if readErr != nil {
		return "", "", "", fail("SYNC_OUTCOME_UNKNOWN", "push outcome cannot be verified; retry sync")
	}
	if seen != local {
		if pushErr != nil && seen == before {
			return "", "", "", fail("SYNC_PUSH_FAILED", "remote rejected the update; local history is preserved: "+pushErr.Error())
		}
		// The destination moved while the push was in flight. It might now
		// include the intended commit, so do not report a proven failure.
		return "", "", "", fail("SYNC_OUTCOME_UNKNOWN", "remote changed during publication; retry sync to observe ancestry")
	}
	if e = checkpointRemote(p, seen); e != nil {
		return "", "", "", e
	}
	actual, e = headCommit(p)
	if e != nil {
		return "", "", "", e
	}
	if actual != local {
		return "local_pending", actual, seen, nil
	}
	return "published", local, seen, nil
}
