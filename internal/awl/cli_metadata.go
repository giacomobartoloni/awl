package awl

import (
	"encoding/json"
	"flag"
	"fmt"
)

func (c *cli) commandMetadata(fs *flag.FlagSet, args []string) int {
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable response")
	input := fs.String("input", "-", "typed JSON value or multi-pointer set payload source")
	rid := fs.String("request-id", "", "stable request ID for retries")
	if e := fs.Parse(args); e != nil || fs.NArg() < 2 {
		return c.emitError(fail("USAGE", "metadata requires get, set, insert or remove and a Markdown path"))
	}
	action := fs.Arg(0)
	path, e := safePath(fs.Arg(1))
	if e != nil {
		return c.emitError(e)
	}
	if action == "get" {
		if fs.NArg() != 3 {
			return c.emitError(fail("USAGE", "metadata get requires a path and JSON Pointer"))
		}
		p, e := resolve(c.explicit)
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
		value, e := metadataGet(file.Bytes, fs.Arg(2))
		if e != nil {
			return c.emitError(e)
		}
		if c.jsonOut {
			return c.encode(struct {
				Status   string `json:"status"`
				Project  string `json:"project"`
				Revision string `json:"revision"`
				Path     string `json:"path"`
				Pointer  string `json:"pointer"`
				Value    any    `json:"value"`
			}{"ok", p.Slug, head, path, fs.Arg(2), value})
		}
		if text, ok := value.(string); ok {
			_, e = fmt.Fprintln(c.stdout, text)
		} else {
			e = json.NewEncoder(c.stdout).Encode(value)
		}
		if e != nil {
			return c.emitError(e)
		}
		return 0
	}
	var edits []metadataEdit
	hasInput := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "input" {
			hasInput = true
		}
	})
	switch action {
	case "set", "insert":
		if action == "set" && fs.NArg() == 2 {
			// Multiple JSON Pointer values supplied in one atomic set.
			if !hasInput {
				return c.emitError(fail("USAGE", "metadata set requires a pointer and value or explicit --input"))
			}
			data, err := readInput(c.stdin, *input)
			if err != nil {
				return c.emitError(err)
			}
			edits, e = metadataSetEdits(data)
			if e != nil {
				return c.emitError(e)
			}
			break
		}
		var value []byte
		switch fs.NArg() {
		case 3: // Typed JSON value, supplied explicitly via --input.
			if !hasInput {
				return c.emitError(fail("USAGE", "metadata "+action+" requires a value or explicit --input"))
			}
			value, e = readInput(c.stdin, *input)
		case 4: // Positional values are always strings, never inferred types.
			if hasInput {
				return c.emitError(fail("USAGE", "metadata "+action+" cannot combine positional value with --input"))
			}
			value, e = json.Marshal(fs.Arg(3))
		default:
			return c.emitError(fail("USAGE", "metadata "+action+" requires a path, pointer and value (or explicit --input)"))
		}
		if e != nil {
			return c.emitError(e)
		}
		edits = []metadataEdit{{Action: action, Pointer: fs.Arg(2), Value: value}}
	case "remove":
		if fs.NArg() != 3 || hasInput {
			return c.emitError(fail("USAGE", "metadata remove requires path and pointer without --input"))
		}
		edits = []metadataEdit{{Action: "remove", Pointer: fs.Arg(2)}}
	default:
		return c.emitError(fail("USAGE", "metadata supports get, set, insert and remove"))
	}
	if e := validateMetadataEdits(edits); e != nil {
		return c.emitError(e)
	}
	return c.writeRequest(request{RequestID: *rid, Operations: []operation{{ID: "op-1", Op: "metadata", Path: path, Metadata: edits}}})
}
