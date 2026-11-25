package awl

import (
	"fmt"
	"io"
)

// writeTextResult presents the existing operation result for human and agent
// comprehension. JSON remains the machine-readable representation.
func writeTextResult(w io.Writer, r result) {
	fmt.Fprintf(w, "status: %s\n", r.Status)
	if r.Project != "" {
		fmt.Fprintf(w, "project: %q\n", r.Project)
	}
	if r.RequestID != "" {
		fmt.Fprintf(w, "request_id: %q\n", r.RequestID)
	}
	if r.Commit != "" {
		fmt.Fprintf(w, "local_commit: %s\n", r.Commit)
	}
	if r.Tree != "" {
		fmt.Fprintf(w, "document_tree: %s\n", r.Tree)
	}
	if r.RemoteBranch != "" {
		fmt.Fprintf(w, "remote_branch: %q\n", r.RemoteBranch)
	}
	if r.RemoteCommit != "" {
		fmt.Fprintf(w, "remote_commit: %s\n", r.RemoteCommit)
	}
	if r.Replayed {
		fmt.Fprintln(w, "replayed: true")
	}
	if r.Error != nil {
		fmt.Fprintf(w, "error: %s: %q\n", r.Error.Code, r.Error.Message)
	}
	if len(r.Members) == 0 {
		return
	}
	fmt.Fprintf(w, "\noperations: %d\n", len(r.Members))
	for _, m := range r.Members {
		fmt.Fprintf(w, "  %s %q %s %q", m.Status, m.ID, m.Op, m.Path)
		if m.To != "" {
			fmt.Fprintf(w, " -> %q", m.To)
		}
		fmt.Fprintln(w)
		if m.Error != nil {
			fmt.Fprintf(w, "    %s: %q\n", m.Error.Code, m.Error.Message)
		}
	}
}
