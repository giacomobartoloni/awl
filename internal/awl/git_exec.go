package awl

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	localGitTimeout = 45 * time.Second
	gitStdoutLimit  = 32 << 20
	gitStderrLimit  = 1 << 20
)

// boundedOutput drains subprocess output while retaining at most limit bytes.
// Always accepting complete writes avoids blocking a child that produced more
// output than we can safely keep in memory.
type boundedOutput struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	size := len(p)
	available := b.limit - b.Len()
	if available < len(p) {
		b.overflow = true
		if available < 0 {
			available = 0
		}
		p = p[:available]
	}
	_, err := b.Buffer.Write(p)
	return size, err
}

// git provides a bounded local default for existing callers. Future CLI
// request cancellation can be propagated through gitContext explicitly.
func git(dir string, env []string, input []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), localGitTimeout)
	defer cancel()
	return gitContext(ctx, dir, env, input, args...)
}

func gitContext(ctx context.Context, dir string, env []string, input []byte, args ...string) ([]byte, error) {
	opts := []string{}
	if dir != "" {
		opts = append(opts, "--git-dir="+dir)
	}
	opts = append(opts, args...)
	cmd := exec.CommandContext(ctx, "git", opts...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = gitEnv(env)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	stdout := &boundedOutput{limit: gitStdoutLimit}
	stderr := &boundedOutput{limit: gitStderrLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("git %q interrupted: %w", args, ctx.Err())
	}
	if stdout.overflow || stderr.overflow {
		return nil, fail("GIT_OUTPUT_LIMIT", "Git command output exceeded the bounded capture limit")
	}
	if runErr != nil {
		return nil, fmt.Errorf("git %q failed: %w: %s", args, runErr, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
func gitEnv(extra []string) []string {
	ret := []string{}
	for _, x := range os.Environ() {
		k, _, _ := strings.Cut(x, "=")
		if strings.HasPrefix(k, "GIT_") {
			continue
		}
		ret = append(ret, x)
	}
	return append(ret, extra...)
}
func gitText(dir string, env []string, input []byte, args ...string) (string, error) {
	b, e := git(dir, env, input, args...)
	return strings.TrimSuffix(string(b), "\n"), e
}
func gitSource(root string, args ...string) ([]byte, error) {
	a := append([]string{"-C", root}, args...)
	s, e := gitText("", nil, nil, a...)
	return []byte(s), e
}
func source() (string, string, error) {
	cwd, e := os.Getwd()
	if e != nil {
		return "", "", e
	}
	root, e := gitSource(cwd, "rev-parse", "--show-toplevel")
	if e != nil {
		return "", "", fail("PROJECT_NOT_RESOLVED", "not inside a source Git worktree")
	}
	common, e := gitSource(cwd, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil {
		return "", "", fail("PROJECT_NOT_RESOLVED", "cannot determine Git common directory")
	}
	return string(root), string(common), nil
}
func identity(root string) (string, string, error) {
	name, e := gitSource(root, "config", "user.name")
	if e != nil || len(name) == 0 {
		return "", "", fail("IDENTITY_UNAVAILABLE", "configure source Git user.name")
	}
	email, e := gitSource(root, "config", "user.email")
	if e != nil || len(email) == 0 {
		return "", "", fail("IDENTITY_UNAVAILABLE", "configure source Git user.email")
	}
	return string(name), string(email), nil
}
