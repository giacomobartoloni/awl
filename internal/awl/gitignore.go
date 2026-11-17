package awl

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

const localConfigIgnoreRule = ".awl/local.yaml"

// ensureLocalConfigIgnored changes only the source .gitignore, and only when
// explicitly requested. Git's effective ignore rules take precedence.
func ensureLocalConfigIgnored(root string) error {
	tracked, err := gitSource(root, "ls-files", "--cached", "--", localConfigIgnoreRule)
	if err != nil {
		return err
	}
	if len(tracked) != 0 {
		return fail("LOCAL_CONFIG_CONFLICT", ".awl/local.yaml is already tracked; untrack it explicitly before updating .gitignore")
	}

	_, err = gitSource(root, "check-ignore", "-q", "--", localConfigIgnoreRule)
	if err == nil {
		return nil
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return err
	}

	path := filepath.Join(root, ".gitignore")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return atomicNewFile(path, []byte(localConfigIgnoreRule+"\n"))
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fail("GITIGNORE_CONFLICT", "source .gitignore is not a regular file")
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	newline := "\n"
	if bytes.Contains(existing, []byte("\r\n")) {
		newline = "\r\n"
	}
	prefix := ""
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		prefix = newline
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) {
		return fail("GITIGNORE_CONFLICT", "source .gitignore changed during initialization")
	}
	if _, err = file.Write([]byte(prefix + localConfigIgnoreRule + newline)); err != nil {
		return err
	}
	return file.Sync()
}
