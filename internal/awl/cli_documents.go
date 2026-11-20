package awl

import (
	"flag"
	"fmt"
	"unicode/utf8"
)

func (c *cli) commandList(fs *flag.FlagSet, args []string) int {
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
	if e := fs.Parse(args); e != nil || fs.NArg() > 1 {
		return c.emitError(fail("USAGE", "ls accepts an optional project-relative directory and --json"))
	}
	prefix, e := safeListPrefix(fs.Arg(0))
	if e != nil {
		return c.emitError(e)
	}
	p, e := resolve(c.explicit)
	if e != nil {
		return c.emitError(e)
	}
	revision, e := headCommit(p)
	if e != nil {
		return c.emitError(e)
	}
	paths, e := listDocuments(p, revision, prefix)
	if e != nil {
		return c.emitError(e)
	}
	if c.jsonOut {
		return c.encode(struct {
			Status   string   `json:"status"`
			Project  string   `json:"project"`
			Revision string   `json:"revision"`
			Paths    []string `json:"paths"`
		}{"ok", p.Slug, revision, paths})
	}
	for _, path := range paths {
		if _, e := fmt.Fprintln(c.stdout, path); e != nil {
			return c.emitError(e)
		}
	}
	return 0
}

func (c *cli) commandRead(fs *flag.FlagSet, args []string) int {
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
	if e := fs.Parse(args); e != nil || fs.NArg() != 1 {
		return c.emitError(fail("USAGE", "read requires project-relative Markdown path"))
	}
	p, e := resolve(c.explicit)
	if e != nil {
		return c.emitError(e)
	}
	path, e := safePath(fs.Arg(0))
	if e != nil {
		return c.emitError(e)
	}
	head, e := headCommit(p)
	if e != nil {
		return c.emitError(e)
	}
	file, e := readEntry(p, head, path)
	if e != nil {
		return c.emitError(e)
	}
	if !file.Exists {
		return c.emitError(fail("DOCUMENT_NOT_FOUND", "document does not exist"))
	}
	if c.jsonOut {
		if !utf8.Valid(file.Bytes) {
			return c.emitError(fail("INVALID_CONTENT", "document is not valid UTF-8 for JSON output"))
		}
		return c.emitRead(p, file.Bytes, head)
	}
	_, e = c.stdout.Write(file.Bytes)
	if e != nil {
		return c.emitError(e)
	}
	return 0
}

func (c *cli) commandMutate(name string, fs *flag.FlagSet, args []string) int {
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
	input := fs.String("input", "-", "request payload source: - for stdin, otherwise a file")
	rid := fs.String("request-id", "", "stable request ID for retries")
	if e := fs.Parse(args); e != nil || fs.NArg() != 1 {
		return c.emitError(fail("USAGE", name+" requires one project-relative path"))
	}
	path, e := safePath(fs.Arg(0))
	if e != nil {
		return c.emitError(e)
	}
	data, e := readInput(c.stdin, *input)
	if e != nil {
		return c.emitError(e)
	}
	op := operation{ID: "op-1", Op: name, Path: path}
	if name == "create" {
		if !utf8.Valid(data) {
			return c.emitError(fail("INVALID_CONTENT", "Markdown must be UTF-8"))
		}
		op.Content = string(data)
	} else {
		var fields struct {
			Old string `json:"old"`
			New string `json:"new"`
		}
		if e = strictJSON(data, &fields); e != nil || fields.Old == "" {
			return c.emitError(fail("INVALID_INPUT", "edit expects JSON object with nonempty old and a new string"))
		}
		op.Old, op.New = fields.Old, fields.New
	}
	return c.writeRequest(request{RequestID: *rid, Operations: []operation{op}})
}

// commandMove relocates a managed document without interpreting its contents.
func (c *cli) commandMove(fs *flag.FlagSet, args []string) int {
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
	rid := fs.String("request-id", "", "stable request ID for retries")
	if e := fs.Parse(args); e != nil || fs.NArg() != 2 {
		return c.emitError(fail("USAGE", "move requires source and destination project-relative Markdown paths"))
	}
	from, e := safePath(fs.Arg(0))
	if e != nil {
		return c.emitError(e)
	}
	to, e := safePath(fs.Arg(1))
	if e != nil {
		return c.emitError(e)
	}
	return c.writeRequest(request{RequestID: *rid, Operations: []operation{{ID: "op-1", Op: "move", Path: from, To: to}}})
}

func (c *cli) commandApply(fs *flag.FlagSet, args []string) int {
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
	input := fs.String("input", "-", "JSON request input")
	if e := fs.Parse(args); e != nil || fs.NArg() != 0 {
		return c.emitError(fail("USAGE", "apply accepts --input and --json"))
	}
	data, e := readInput(c.stdin, *input)
	if e != nil {
		return c.emitError(e)
	}
	var req request
	if e = strictJSON(data, &req); e != nil {
		return c.emitError(fail("INVALID_INPUT", "apply requires JSON request"))
	}
	return c.writeRequest(req)
}
