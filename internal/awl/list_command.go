package awl

import (
	"flag"
	"fmt"
	"io"
)

// RunCLI dispatches user-facing commands from the binary. Existing command
// behavior remains in Run while the local read-only list command is introduced.
func RunCLI(args []string, in io.Reader, out, errOut io.Writer) int {
	original := args
	c := &cli{stdout: out, stderr: errOut, stdin: in}
	if len(args) >= 2 && args[0] == "--project" {
		c.explicit = args[1]
		args = args[2:]
	}
	if len(args) == 0 || args[0] != "ls" {
		return Run(original, in, out, errOut)
	}
	return c.runList(pathFlagsFirst(args[1:]))
}

func (c *cli) runList(args []string) int {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return c.emitError(fail("USAGE", "ls accepts an optional project-relative directory and --json"))
	}
	prefix := ""
	if fs.NArg() == 1 {
		var err error
		prefix, err = safeListPrefix(fs.Arg(0))
		if err != nil {
			return c.emitError(err)
		}
	}
	p, err := resolve(c.explicit)
	if err != nil {
		return c.emitError(err)
	}
	head, err := headCommit(p)
	if err != nil {
		return c.emitError(err)
	}
	paths, err := listDocuments(p, head, prefix)
	if err != nil {
		return c.emitError(err)
	}
	if c.jsonOut {
		return c.encode(struct {
			Status   string   `json:"status"`
			Project  string   `json:"project"`
			Revision string   `json:"revision"`
			Paths    []string `json:"paths"`
		}{"ok", p.Slug, head, paths})
	}
	for _, path := range paths {
		if _, err := fmt.Fprintln(c.stdout, path); err != nil {
			return c.emitError(err)
		}
	}
	return 0
}
