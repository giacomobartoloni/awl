# Partial Markdown Reads — Agent Tool Conventions and Proposed Contract

**Status:** research and **proposed** product behavior; not owner-approved and not implemented.  
**Date:** 2026-10-23.  
**Scope:** AgentWorklog CLI (awl), `document.read` only.  
**Related:** [Document and Operation Contract](../requirements/document-operation-contract.md) · [Core Requirements](../requirements/core-requirements.md).

## 1. Question and product constraints

What is the smallest predictable read contract that supports a full Markdown read, token-efficient partial reads, and subsequent agent edits without changing the established boundary that **`document.edit` modifies only the Markdown body**?

Relevant constraints: local Git revisions exist offline; documents are user-owned Markdown with YAML frontmatter; body edits use content-based anchors rather than numeric line addresses; a stale read does not automatically invalidate a safe exact-target body edit; the authoritative Git remote is a separate publication boundary.

Research below documents **external tool behavior**. Recommendations for awl are derived proposals, not claims that competitor APIs are interchangeable.

## 2. Primary-source observations (accessed 2026-10-23)

| Tool | Observed behavior | What is portable to awl | What should not be copied blindly |
| --- | --- | --- | --- |
| OpenCode | Its documented `read(path, offset, limit)` supports bounded, numbered line output. Current docs describe a limit of **2,000 lines and 50 KiB** per page and shortening of lines above 2,000 characters. | Agent-friendly line numbers, explicit range request and page-size bounds. | Provider-specific caps and silent assumptions about shortened lines. |
| Claude Code | Official Read documentation describes numbered lines, `offset`/`limit`, and a page/partial-view mechanism for large files; behavior can depend on file size and explicit parameters. | Explicit PARTIAL indication, range continuation guidance, notice for reads beyond end-of-file. | Its token caps, filesystem-relative path conventions, read-before-edit rules, and release-dependent thresholds. |
| Codex CLI | Public Codex instructions use **1-based** file line/column references; Codex often reads files through shell/file tools rather than a universal dedicated read API. | Human-compatible absolute line labels. | Assuming all coding agents use one stable `offset/limit` contract. |

### Sources

- [OpenCode tools documentation, Read](https://dev.opencode.ai/v2/docs/tools/)
- [Claude Code tools reference, Read / Large files](https://code.claude.com/docs/en/tools-reference)
- [Codex base instructions, File References](https://github.com/openai/codex/blob/main/codex-rs/protocol/src/prompts/base_instructions/default.md)
- [Codex issue discussing chunked reads](https://github.com/openai/codex/issues/8982) — **community issue; not a protocol definition**

Observed consensus: **read ranges using line coordinates**, show numbered positions, bound output, and make partial results explicit. No universal line/byte ceiling exists across tools.

## 3. Recommended awl read contract (proposal, not approved)

### 3.1 Scope and coordinate system

- `document.read` reads the **logical entire original Markdown file**, including YAML frontmatter when present.
- Line numbers are **1-based absolute physical-file lines**, including all frontmatter delimiter lines, frontmatter values and body lines. This agrees with common editors, `cat -n`, diff displays and command-line references.
- `document.edit` still only modifies the **body**; a numbered read line is a **navigation hint**, never an editing authority or precondition. For frontmatter changes use `metadata.*` with JSON Pointer.
- Read output must preserve the underlying document content; any numbered display must be a **presentation overlay**, not additional text in the Markdown bytes.

Why full-file numbering? It avoids two ambiguous "line 12" coordinate spaces and supports pointing a human toward the same line in a file editor. An alternative is body-relative numbering, but that differs from editor file lines and requires explicit conversion when frontmatter length changes. If a future body-only view is added, it must name a separate coordinate system rather than redefine `document.read`.

### 3.2 Range request and stable revision

Illustrative request — names not a finalized CLI/MCP schema:

~~~yaml
op: document.read
path: Activities/example.md
range:
  start_line: 120
  max_lines: 80
revision: rev-example-7   # optional local snapshot pin
~~~

- Omitting `range` requests the whole document. An oversized whole-file response **must not be mislabeled complete**; it may return an explicitly marked first page or an actionable size error, with a deterministic way to request the rest. Preferred behavior is an explicitly marked first page.
- `start_line` is **1-based and inclusive**; `max_lines` is a **positive maximum count**. The selection stops before the first line that would exceed the count or a separate output-size budget.
- `revision` is an optional **opaque local document revision**. If supplied, all returned lines come from that revision, not the latest version. If the revision is no longer available, return an explicit revision-unavailable error; never silently switch to the current version.
- If omitted, `revision` selects the latest locally visible snapshot when the read starts. The response always returns the **actual revision read**.
- A client can page deterministically with the returned revision and `next_start_line`, even if another awl request commits in the meantime. This is especially practical with managed Git snapshots.
- The read operation itself does not fetch/push Git, refresh remote history or mutate files.

### 3.3 Response concepts

Illustrative success for a partial read:

~~~json
{
  "project": "muse",
  "path": "Activities/example.md",
  "revision": "rev-example-7",
  "range": {
    "coordinate_system": "full_file",
    "first_line": 120,
    "last_line": 135,
    "lines_returned": 16,
    "total_lines": 417,
    "has_more": true,
    "next_start_line": 136
  },
  "complete": false,
  "truncation": {
    "reason": "response_size_limit"
  },
  "content": "..."
}
~~~

For a completed read, `complete` is true **only if the whole requested selection was returned**, not merely if the size cap was reached. `has_more` states whether later **file** lines exist; `complete`, `has_more`, and `truncation.reason` must be defined consistently (especially when a bounded request ends before EOF). The exact JSON fields are examples, not a selected wire format.

A partial read must expose enough positional context for an agent to request the next page without inferring hidden offsets. A tool may display numbered lines for readability or return an exact-content span with separate metadata. The source text must never be irreversibly altered just to prefix line numbers.

### 3.4 Output budgets, long lines, EOF and blank files

- Enforce **both a line-count and a byte/token-oriented result budget**. Exact values require empirical evaluation on representative awl files and model contexts; do not hard-code another tool's 2,000-line/50 KiB threshold as an awl requirement.
- A response cut by either budget explicitly reports the reason and the next safe start line. When the requested range is larger than allowed, prefer a valid partial page plus continuation metadata over a generic, non-actionable error.
- **Never silently shorten a line and present it as complete content.** If one physical line alone exceeds a safe output budget, return a targeted `LINE_TOO_LONG` or explicit partial-line state, with a future character/byte-slice read mechanism to be considered. Do not invent a line fragment edit API as a prerequisite.
- An empty file returns an unambiguous empty read (not missing document).
- A `start_line` past EOF returns an explicit `LINE_RANGE_OUT_OF_BOUNDS` with total count, or another comparably explicit non-success outcome. The special case of `total_lines + 1` is an open minor detail; no implicit wraparound.
- Count `CRLF` and `LF` as physical line breaks without normalizing the Markdown file. Define treatment of final newline, trailing empty line and mixed line endings in acceptance tests before implementation.
- Report unavailable revisions explicitly. No read result should claim that the local snapshot necessarily matches the authoritative Git remote.

## 4. Worked frontmatter boundary example

~~~markdown
---
title: Example
status: draft
---
# Outcome
Pending verification.
~~~

Absolute lines for this document:

| Full-file line | Source line | Read/edit interpretation |
| --- | --- | --- |
| 1 | `---` | Frontmatter delimiter |
| 2 | `title: Example` | YAML only; use metadata API to modify |
| 3 | `status: draft` | YAML only; use metadata API to modify |
| 4 | `---` | Frontmatter delimiter |
| 5 | `# Outcome` | Markdown body; eligible for body edits |
| 6 | `Pending verification.` | Markdown body; eligible for body edits |

A request to read lines 5–6 starts at `start_line: 5`, `max_lines: 2`. No separate body-relative offset is implied, and no edit is made by reading.

## 5. Proposed acceptance scenarios

| ID | Scenario | Expected behavior |
| --- | --- | --- |
| RD-AC-01 | A read omits the range for a small document | Whole Markdown including frontmatter is returned, unchanged |
| RD-AC-02 | A ranged read begins at line 5 after a four-line frontmatter | Absolute lines 5 onward are returned; no body-relative offset |
| RD-AC-03 | A ranged read requests 80 lines | At most 80 complete physical lines are returned, subject to output cap |
| RD-AC-04 | A large whole-file read exceeds the size budget | Explicitly incomplete first page or actionable size error; never silently marked full |
| RD-AC-05 | A page is followed by another using returned `revision` and `next_start_line` after a new local commit | Second page comes from the **same original snapshot** |
| RD-AC-06 | Caller supplies an unavailable revision | Explicit revision-unavailable error, not silent fallback to head |
| RD-AC-07 | One line exceeds the output cap | Explicit long-line/partial-line outcome; no hidden truncation passed off as exact content |
| RD-AC-08 | A document contains CRLF and LF endings | Read preserves original content; line coordinate semantics are consistent |
| RD-AC-09 | A line coordinate is beyond EOF | Explicit out-of-range context, including total line count |
| RD-AC-10 | An empty existing Markdown file is read | Valid empty read, distinguishable from `DOCUMENT_NOT_FOUND` |
| RD-AC-11 | A read returns a line number within frontmatter | A subsequent `document.edit` still cannot mutate YAML |
| RD-AC-12 | Two pages overlap intentionally | Same revision yields identical shared line content without re-numbering |

All test cases are **proposals**; none has been executed.

## 6. Alternatives and remaining decisions

| Alternative | Rationale | Recommendation |
| --- | --- | --- |
| Body-relative line coordinates | Convenient next to body-only edit API | Reject as default because editors/Git show full-file positions; consider explicitly named body view only if needed |
| Byte/character offsets as primary range | Useful for very long single lines | Defer; lines are the dominant agent affordance, but a long-line fallback may be necessary |
| Auto-read latest on every page | Simple initial implementation | Reject due to page drift under concurrent commits; support pinned local revisions |
| Silent line truncation | Limits model input | Reject: breaks exact text anchors and correctness |
| Fixed 2,000-line limit from OpenCode | Easy to copy | Defer quantitative limit to realistic file/model testing |
| One long string with prefixed line numbers | Visually convenient | Presentational only; do not alter raw returned Markdown |

**Recommended review gate:** approve or revise (a) full-file 1-based coordinate system including frontmatter, (b) optional snapshot-pinned ranges, and (c) explicit truncation/continuation semantics. Exact quotas and serialization can follow. No satellite execution or deployment decision is necessary.
