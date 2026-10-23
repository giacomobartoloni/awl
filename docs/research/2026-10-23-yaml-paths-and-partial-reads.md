# Research Note — Addressing YAML Metadata and Partial Document Reads

**Status:** evaluated design alternatives; **not approved as final API syntax**.  
**Date:** 2026-10-23.  
**Scope:** AgentWorklog CLI (awl), document/metadata operation API.  
**Related:** [Document and Operation Contract](../requirements/document-operation-contract.md).

## 1. Context and decisions already made

During the 2026-10-23 product discussion the owner selected:
- `document.edit` operates on **the Markdown body only**, never raw YAML frontmatter;
- a stale read revision alone does not force failure when the replacement target remains uniquely and safely identifiable;
- a composite **continues after an operation fails**, reporting every member's result;
- **one Git commit per request** incorporating successful changes (when the request actually changes one or more documents).

The owner explicitly deferred the final partial-read interface and asked for established approaches to nested YAML paths. This note investigates both without deciding the exact syntax.

## 2. YAML addressing requirements

- Read, set and remove arbitrary frontmatter fields without interpreting their domain semantics.
- Address nested maps and sequences.
- Distinguish **literal keys containing periods, slashes or tildes** from structural separators.
- Avoid surprises with numeric indexes, absent parents, arrays that shift, and YAML constructs outside JSON's data model.
- Preserve untouched fields, comments and body content as far as the selected parser can reliably guarantee; fail rather than silently lose user-owned structure.
- Separate **document location** (project + file path) from **metadata value location** (address inside frontmatter).

Example:

~~~yaml
build.version: "1.2"
build:
  version: "1.3"
reviewers:
  - name: Alice
  - name: Bob
~~~

## 3. Established path representations

| Syntax | Example | Strength | Risk / qualification |
| --- | --- | --- | --- |
| Dot notation (e.g. yq) | `build.version`, `reviewers[0].name` | Human-friendly and common among YAML processors | Requires escaping/quoting literal periods; dialects differ |
| JSON Pointer — RFC 6901 | `/build/version`, `/reviewers/0/name` | Standardized single-node address, simple evaluation | Numeric tokens are interpreted by container type; escaping `/` and `~` is necessary; YAML mapping keys need an agreed subset |
| JSONPath — RFC 9535 | `$.reviewers[0].name` | Standardized query language supporting filtered/multiple selections | Powerful query semantics are unnecessary and risky for a single-target write API |
| Structured segment array | `["reviewers", 0, "name"]` | Unambiguous typed key/index sequence, easy to generate as structured data | Verbose for human CLI; no single universal plain-YAML standard |
| JSON Patch — RFC 6902 | `{"op":"replace","path":"/reviewers/0/name","value":"Eve"}` | Standard ordered changes (`add`, `remove`, `replace`, `test` etc.) | Designed for JSON, not YAML round-trip comment/alias preservation; entire protocol need not be adopted |

JSON Pointer: a pointer is a string of zero or more `/`-prefixed reference tokens. Escape `~` as `~0` and `/` as `~1`. A literal dot requires **no** special escape. The empty pointer `""` addresses the root, though a root-replacement operation is not required for an MVP.

In this example:
- `/build.version` addresses literal top-level key `build.version`;
- `/build/version` addresses nested key `version`;
- `/reviewers/0/name` addresses the first reviewer's name;
- `/external~1id` would address literal key `external/id`.

### Candidate recommendation

**Prefer JSON Pointer as the canonical single-target locator** for frontmatter operations, with `metadata.set`, `metadata.remove`, `metadata.get`. Dot syntax may be considered *only* as an optional CLI convenience translation when unambiguous; do not make two incompatible path dialects normative.

Example of a conceptual operation (illustrative, not final schema):

~~~yaml
op: metadata.set
path: Activities/example.md     # document path
pointer: /reviewers/0/name      # location inside frontmatter
value: Alice
~~~

This is a **recommendation to review, not an approved naming or syntax choice**.

A structured segment array is a defensible second choice for typed programmatic clients. JSONPath is better reserved for possible future queries, where multiple matches can be explicit. JSON Patch's `test` semantics are worth considering separately as an optional conditional metadata update without requiring full JSON Patch support.

### Safety boundaries requiring later design

- Only one target may be changed by a single metadata operation; no implicit wildcard expansion.
- Decide whether metadata map keys are restricted to strings in the supported mutation subset; do not silently stringify arbitrary YAML keys.
- Define behavior for absent parent maps, absent leaves, a `null` value versus removal, indexes beyond arrays, and negative indexes.
- For indexed list edits on stale revisions, consider optional preconditions or expected-value tests, because elements can shift even if an index remains valid.
- YAML tag, aliases/anchors, duplicate keys and comments need explicit round-trip expectations and failure modes.
- Do not quietly reinterpret an existing literal key with a dot as a path.

## 4. Partial-read conventions in agent tools

### Observed established convention

The OpenCode `read` tool takes a file `path` and optional `offset` and `limit`, describes offsets as starting at a **line**, and outputs **numbered lines**. Its currently published documentation states a maximum of **2,000 lines and 50 KiB per page**, with individual overlong lines shortened. These are observations about one tool, **not recommended hard limits for awl**.

For awl, the transferable principles are:
- Full read without a range remains a straightforward supported operation.
- A partial read uses **explicit line-based start and maximum count**, not raw bytes/characters ambiguously called offsets.
- Numbered lines help coding agents use returned context and produce subsequent edits.
- Return the **same local revision** associated with the excerpt; line coordinates are meaningless without a known version.
- Bound the response and communicate truncation or availability of subsequent material. Avoid silently returning a "full read" that is actually partial.
- Treat line numbers as **navigation hints**, not sufficient safety preconditions for writes.

### Illustrative read request (not selected interface)

~~~yaml
op: document.read
path: Activities/example.md
range:
  start_line: 120
  max_lines: 80
~~~

Illustrative response fields:

~~~json
{
  "path": "Activities/example.md",
  "revision": "rev-example-4",
  "range": {
    "start_line": 120,
    "lines_returned": 80,
    "next_line": 200,
    "has_more": true
  },
  "lines": ["..."]
}
~~~

**Open read boundary:** determine whether the line coordinates are measured across the *full raw Markdown file including YAML frontmatter* or relative to **body-only**. Body-only coordinates make a clear pairing with `document.edit`, but full-file lines are familiar to programmers and shell tools. The response may need to carry both coordinate systems or one clearly labeled system; do not conflate them.

**Other open questions:** 1-based versus 0-based; long-line behavior; UTF-8/codepoint and line-ending semantics; read-beyond-EOF; maximum response cap; pagination across moving revisions; whether metadata and body reads are separate methods; ability to pin reads to a revision.

### Preliminary recommendation for a prototype

Use **1-based, explicit line coordinates** with `start_line` and `max_lines`, return numbered lines or an equivalent structured span, attach an opaque local revision, and expose `has_more` and any truncation reason. Keep the coordinate frame and limits **unapproved** until compared with representative agent tools and tested on actual worklogs.

## 5. Explicitly deferred

This research does **not** define:
- an execution location, scheduling service or deployment of Google Drive or other satellite connectors;
- full mutation/query syntax, YAML round-trip library or Git implementation details;
- range truncation limits or a canonical JSON/YAML wire schema;
- new requirements for search, metadata queries or automatic fallback rerendering;
- three-way merge/rebase and contextual patch algorithms.

## 6. Source references (accessed 2026-10-23)

1. [RFC 6901 — JSON Pointer](https://www.rfc-editor.org/rfc/rfc6901.html): reference-token syntax and escaping.
2. [RFC 6902 — JSON Patch](https://www.rfc-editor.org/rfc/rfc6902.html): ordered operations including `test`, `replace` and `remove`.
3. [RFC 9535 — JSONPath](https://www.rfc-editor.org/rfc/rfc9535.html): query purpose and distinction from JSON Pointer.
4. [mikefarah/yq — How It Works](https://mikefarah.gitbook.io/yq/how-it-works): dot notation and nested YAML navigation.
5. [mikefarah/yq — Path](https://mikefarah.gitbook.io/yq/operators/path): programmatic sequence paths and `setpath`/`delpaths`.
6. [OpenCode — Tools / Read](https://opencode.ai/v2/docs/tools): line-based `offset`/`limit`, numbered lines and bounded pages.

**Evidence:** comparison based on published docs and standards; no code was executed and no round-trip or performance tests were run.
