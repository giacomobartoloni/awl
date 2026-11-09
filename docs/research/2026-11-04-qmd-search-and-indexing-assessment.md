# QMD as a Candidate Search and Indexing Engine for awl

**Status:** exploratory assessment; no engine, dependency, API schema, or implementation selected.  
**Date:** 2026-11-04.  
**Scope:** AgentWorklog CLI (`awl`), not the AgentWorklog Markdown worklog conventions.  
**Related:** [Core Requirements](../requirements/core-requirements.md) · [Document and Operation Contract](../requirements/document-operation-contract.md).

## 1. Requirement boundary

The project owner confirmed on 2026-11-04 that document indexing and retrieval belong in the **core document-management capability**. The intended retrieval modes are lexical/full-text, semantic, and hybrid. Indexing and **structured querying/filtering of YAML-frontmatter metadata are not in the current search scope**.

This is not a reversal of generic YAML metadata document operations: `metadata.get/set/remove/insert/append` remain separately proposed core document-management operations. No migration, reformatting, or `qmd.metadata` insertion into user-owned Markdown is authorized.

The existing owner-selected but unmerged Git-hub direction remains intact. Search results must be grounded in the project's managed local document state; any index is derived data, not an authoritative copy of Markdown or Git history.

## 2. QMD capabilities inspected

Source inspected: [tobi/qmd](https://github.com/tobi/qmd) (public repository, accessed 2026-11-04), [README](https://github.com/tobi/qmd/blob/main/README.md), [package manifest](https://github.com/tobi/qmd/blob/main/package.json), [changelog](https://github.com/tobi/qmd/blob/main/CHANGELOG.md), and [metadata extractor](https://github.com/tobi/qmd/blob/main/src/metadata.ts).

- QMD exposes local lexical/BM25, vector, and hybrid search over indexed documents; CLI and programmatic integration surfaces exist.
- QMD's current package manifest requires Node.js 22 or newer. Whether to use a QMD subprocess, SDK, MCP or another boundary remains open; this observation does **not** select awl's programming language.
- The latest released version observed via GitHub Releases on 2026-11-04 was `v2.8.3` (released 2026-08-16). The QMD `main` changelog describes YAML metadata filtering and discovery under **Unreleased**. Do not rely on unreleased metadata features for the awl core.
- QMD's development metadata extractor reads a dedicated `qmd.metadata` namespace, supports scalars and flat homogeneous arrays, and does not provide general querying of arbitrary nested YAML. This gap is out of the **current** awl indexing/search scope rather than a reason to modify awl frontmatter.
- QMD uses project-local configuration and may support update hooks; review the upstream trust/approval rules and ensure indexing user documents cannot trigger unapproved commands or out-of-scope reads.

These are observations from source documentation, **not** performance measurements or an integration test. Pin and re-check the tested revision before drawing implementation conclusions.

## 3. Candidate integration boundary

A possible read-only logical flow is:

1. Resolve project slug and a managed local Git document revision/snapshot via awl.
2. Index an explicitly scoped view of Markdown documents with QMD, using private configuration and isolated derived index state.
3. Return results through awl with canonical project-relative paths, grounded snippets, and information sufficient to identify index freshness against local revisions.
4. Refresh/rebuild the index after successful local document changes or Git reconciliation, with explicit reporting if indexing is pending, stale or failed.

The content boundary is unresolved: whether frontmatter is included as *ordinary text* in full-text/vector indexing or the indexed text consists only of Markdown bodies. Either choice must preserve the original document and return navigable original-source references. No structured metadata filter is required.

Do not expose QMD index files, `qmd://` identifiers, generated projections, external model URLs or cache paths as the awl canonical document identity.

## 4. Questions to answer before selecting QMD

- **Scope:** one project versus explicit multi-project search; inclusion/exclusion of non-document files, ignored content, and Markdown frontmatter text.
- **Consistency:** refresh triggers on local commits, sync/fetch/rebase, deletion/rename; how query results declare current, stale, indexing or unavailable states without falsely claiming snapshot consistency.
- **Offline behavior:** lexical retrieval without AI models; semantic/hybrid modes after explicit local model provisioning; expected response when models are absent, interrupted or too expensive.
- **Result integrity:** canonical document paths, stable revision association, snippet/line navigation, deduplication, limits, rank quality and behavior of changed files.
- **Safety:** read-only source access, no implicit Markdown rewrites, no unapproved hooks, constrained paths, acceptable disk/CPU/memory use, and recovery from index corruption.
- **Upstream stability:** released vs. unreleased functionality, license, dependency and platform compatibility, pinned version/commit and upgrade/rebuild behavior.
- **Metadata boundary:** reject or explicitly mark structured YAML-filter requests as unsupported; do not silently ignore them.

## 5. Proposed acceptance scenarios (unverified)

| ID | Scenario | Expected behavior |
| --- | --- | --- |
| QMD-AC-01 | Search two independent projects with overlapping filenames | Results retain the correct project and project-relative path; no unintended cross-project hits |
| QMD-AC-02 | Search for an exact term, then for a concept described differently | Lexical and semantic/hybrid modes are distinguishable and yield grounded source hits according to their documented contracts |
| QMD-AC-03 | Change a Markdown document through awl, then search | Index freshness/rebuild state is observable; stale results are not represented as current |
| QMD-AC-04 | Delete or rename a source document through an authorized future operation or reconcile a Git change | Obsolete hits are not silently treated as current; index repair/rebuild is possible |
| QMD-AC-05 | Search offline after local index initialization | Supported retrieval works without hidden network access; unavailable models/modes report explicit errors or degradation |
| QMD-AC-06 | Search a document with complex YAML metadata | No implicit metadata conversion or edit; content search works subject to the selected text-scope rule |
| QMD-AC-07 | Submit a structured metadata filter | The initial interface rejects the unsupported capability explicitly rather than silently dropping the filter |
| QMD-AC-08 | Index an untrusted Markdown repository with QMD configuration present | No unapproved command executes, no path escapes project scope, and original documents remain unchanged |

## 6. Decision status and follow-up

**Selected product scope:** indexing, lexical search, semantic search, and hybrid retrieval are core capabilities. Structured metadata indexing, discovery and filtering are deferred; generic frontmatter document operations are unchanged.

**Not selected:** QMD itself, a particular version/adapter, a mandatory daemon, storage schema, index format, model set, query syntax, ranking, frontmatter text inclusion, refresh scheduling, or cross-project defaults.

Next action: compare a pinned released QMD build against representative awl Markdown fixtures and the scenarios above. The result should inform an implementation-independent search contract and a later explicit engine-selection decision. No implementation or benchmark has been executed for this assessment.


## 7. Reassessment on 2026-11-06: QMD is the first engine to trial

**Decision status:** The owner proposed QMD and requested an evaluation before implementation. This is an evidence-based **preferred candidate for a bounded integration spike**, **not** an approved mandatory dependency or an implemented backend. The 2026-11-06 owner selections of **Go + external Git CLI** for awl's initial mutation CLI remain in force. QMD would be a *separate search component*, not the local document authority and not part of the concurrent Git write transaction.

### 7.1 Sources rechecked

- Verified [upstream QMD repository](https://github.com/tobi/qmd), [release v2.8.3 (2026-08-16)](https://github.com/tobi/qmd/releases/tag/v2.8.3), [v2.8.3 README](https://github.com/tobi/qmd/blob/v2.8.3/README.md), and [package manifest at v2.8.3](https://github.com/tobi/qmd/blob/v2.8.3/package.json) on 2026-11-06. Package declares MIT license and Node >=22. The README also documents Bun >=1.0.0. New features seen in upstream **main** must not be silently attributed to the released tag.
- GitHub [core requirements FR-19, FR-21, FR-22](../requirements/core-requirements.md) require a **derived** project-scoped index; lexical, semantic, and hybrid search; and explicit freshness relative to managed Git revisions. FR-23 defers structured arbitrary YAML-frontmatter filtering. The generic YAML edit operations stay independently in scope.
- Current [first-slice plan in AgentWorklog](https://drive.google.com/file/d/1buh36I4TnCtWIr3EEakN9kqds0wg9yMS/view) proposes Git-local mutation first; full search is separate from that first slice.

### 7.2 Fit and important constraints

| Concern | Observed upstream capability | awl consequence |
| --- | --- | --- |
| Lexical search | QMD \`search\`, BM25 via SQLite FTS5; JSON output | Strong fit for no-model keyword retrieval if an index exists |
| Semantic search | QMD \`vsearch\`, local GGUF embedding model | Fits the requested mode **after explicit model provisioning**, not guaranteed on fresh offline installation |
| Hybrid retrieval | QMD \`query\`, lexical/vector fusion, expansion and reranking | Fits the requested mode, but depends on local models and extra runtime cost |
| Go/CLI deployment | QMD is Node/Bun-based (Node >=22), contains native SQLite/vector and llama.cpp-related dependencies | A Go awl binary remains native but **search has an additional executable/runtime and model deployment story**; a true single-binary distribution is not established |
| Model footprint | README defaults: approximately 300 MB embedding + 640 MB reranker + 1.1 GB query-expansion GGUFs | Roughly **2 GB** combined disk download before full-featured operation; budget CPU/RAM/VRAM and download permissions separately |
| Source index | Collections refer to filesystem directories and QMD maintains its own SQLite index | awl's Git blobs/trees are authoritative. A controlled **per-project projection** of committed Markdown, not a per-request writable source worktree, may be needed for QMD indexing |
| Index freshness | QMD exposes \`status\`, \`update\`, \`embed\` and diagnostics, but does not intrinsically prove correspondence to an awl Git head | awl must track last successfully indexed Git commit, pending/failure state, and query freshness; never label stale results as current |
| Result identity | QMD reports collection/document paths or \`qmd://\` IDs | awl translates to canonical project-relative document identities and revision-aware excerpts; never expose cache locations as authoritative paths |
| Configuration and hooks | QMD config supports collection update shell commands, arbitrary scoped roots and project-local overrides with trust controls | Use **awl-owned isolated config** and allowlisted projection roots; never execute unapproved update hooks or consume user-owned \`.qmd\` config implicitly |
| YAML metadata | Development-main metadata extraction/filtering exists; release-tag behavior is different and arbitrary nested YAML query is not the approved awl scope | Do not depend on unreleased metadata features; preserve frontmatter untouched |

**Source distinction:** The table describes upstream documentation and design inference; it is **not** a runtime result on representative awl documents.

### 7.3 Minimal candidate integration

\`\`\`text
awl (Go) ── controlled Git read/mutation ──> managed authoritative Git history
     |
     └── search orchestration (outside write/CAS critical section)
              |
         committed Git snapshot S
              |
         awl-owned, scoped, reproducible Markdown projection
              |
         QMD CLI subprocess (pinned release; private index/config)
              |
         derived SQLite index + optional local vector models
              |
         awl search result: project path, snippet, indexed_revision=S,
                            freshness=current|stale|pending|failed
\`\`\`

- Prefer **one QMD subprocess boundary** for the spike, with structured arguments/JSON parsing; no Node SDK embedding into Go, mandatory MCP server, daemon, extra provider abstraction, or a second document authority.
- Do not index private request staging or partially applied composite members. Reindex **after** successful local Git publication and separately after successful remote history reconciliation. Do not block accepted Git writes on QMD/model failures. Coalesce repeated refreshes rather than running embeddings inside the local CAS lock.
- Start with a **read-only scoped projection** of already committed Markdown; avoid indexing the source coding repository, the full user home, audit caches or arbitrary provider paths. A bare Git store cannot be assumed to expose a filesystem collection directly to QMD.
- Store a **derived** Git revision checkpoint only after verified QMD refresh; consider conservative stale status during refresh/failure/crash. A search at revision S must not claim to reflect newer committed Git state H unless S=H and the index is validated. Search results must not rewrite user-owned Markdown.
- Treat installed QMD, native runtime dependencies and model caches as a **separate provisioning requirement** from a Go binary. Lexical search should remain usable offline with an existing local index even if AI models are absent; semantic/hybrid modes must fail clearly or explicitly degrade. No surprise downloads during ordinary search.
- The current QMD release's \`--index\` / \`QMD_CONFIG_DIR\` settings can be investigated to isolate per-project state; no specific CLI invocation or config file format is selected yet.

### 7.4 Falsifiable spike and review gates

Use the **released v2.8.3 tag pinned exactly** on representative Markdown files, not a floating \`main\` checkout. Record the installed executable version, OS, Node/Bun runtime, SQLite/native dependencies and local model variants.

1. **Fresh install and offline setup:** install/launch on agreed target platforms, run lexical indexing/query without model downloads; explicitly test offline semantic/hybrid behavior before and after authorised provisioning. Record disk, RAM, CPU, initial download and cold-start costs.
2. **Markdown fidelity and scope:** include existing AgentWorklog documents with YAML comments/aliases, CRLF/LF, Unicode, Obsidian links, duplicate names, irrelevant non-Markdown files and path/permission attacks. Verify **no source byte changes** and no out-of-scope indexing.
3. **Search quality:** labelled exact-keyword, Italian/English paraphrase, semantic and hybrid queries over a representative multi-project corpus. Compare lexical/semantic/hybrid recall, precision/ranking and empty/negative cases. Do not invent a quality target without an agreed corpus.
4. **Freshness:** commit, no-op/audit-only commit, rename, delete, restored file and remote history integration. Demonstrate last-indexed **content tree/revision** mapping, explicit stale/pending/failure responses, no orphaned hits falsely labelled current, and rebuild after corruption.
5. **Isolation and security:** separate projects with same relative path, disallow external QMD config and update shell hooks, stop model auto-downloads until authorised, handle symlinks and untrusted frontmatter.
6. **Concurrent workload:** many awl local commits during queued QMD refreshes and simultaneous read queries. Verify Git mutation remains independent of QMD failure, no SQLite partial results presented as current, bounded refresh coalescing and idempotent repair.
7. **Output contract and dependency failure:** inspect v2.8.3 JSON output schema, canonical path/snippet mapping, missing QMD/Node/native library, model missing or version mismatch, timeout, crash, and index migration/rebuild behavior.

**Decision:** If the spike passes these gates at acceptable distribution and resource cost, **select QMD as the first search engine**, keeping Git as the only content authority and the QMD database disposable. If its runtime dependency or snapshot/projection burden defeats the product's deployment goals, reconsider a Go-native lexical fallback or a narrower initial search implementation as a new documented tradeoff; **do not preimplement parallel engines**.

**Not yet done:** no pinned QMD installation, fixture indexing, search relevance evaluation, native-model provisioning test, benchmark, awl runtime integration, or OS-matrix validation is claimed by this reassessment.


## 8. Owner-selected explicit, recoverable refresh policy (2026-11-06)

**Status:** Owner-confirmed behavioral boundary for awl's **first implementation**, not proof that QMD itself has been selected, installed or tested. The owner prefers **explicit later-recoverable operations** for both Git remote sync and QMD/index refresh, without any mandatory automatic worker, daemon, per-request background subprocess or persistent work-queue engine.

- Completing a local request must not wait for a QMD refresh or model embedding. The already accepted local Git commit remains accepted even if the index is missing, stale or corrupt.
- An explicitly invoked **index refresh** (name/syntax undecided) reads a coherent committed Git content tree via an awl-controlled, project-scoped projection; if QMD is chosen, awl launches the QMD CLI itself with private configuration. The caller of this explicit operation may wait for completion. No claim is made that work continues after its process exits.
- A separate explicit Git remote **sync** invocation is the only owner of Git fetch/reconcile/push. Never delegate a Git pull to QMD collection update hooks, and do not implicitly run configured shell hooks in untrusted QMD config.
- A successful refresh records a **derived** content-tree checkpoint only after QMD confirms the relevant index is updated. An interrupted or failed refresh stays pending/stale/failed, not falsely current. When vectors lag behind refreshed lexical content, expose their availability separately; no silent declaration that hybrid retrieval is current.
- A **Git audit-only commit** leaves document tree bytes unchanged; it can advance the remote-sync state but need not trigger content reindexing. The relevant search freshness comparison is content-tree identity (and independently verified derived-index state), **not just local commit SHA**.
- Multiple commits can be coalesced into one explicit refresh of the latest available committed tree. Many CLI instances invoking refresh concurrently must not corrupt the shared QMD index or expose a partially built index as current; choose cross-process locking or safe isolation only after QMD spike evidence.
- On a later CLI invocation, derive whether refresh is necessary from current committed Git document tree, last verified index checkpoint, and index availability. No automatic scheduling or durable queue is required for the first release. The exact checkpointer, status shape, retry policy and CLI command spellings remain design questions.
- QMD is still the **first engine to trial**, **not an approved mandatory runtime dependency**. The upstream QMD 2.8.3 binary, Node/Bun requirements, model provisioning and refresh failure semantics must still pass the staged spike in §7.4.

**Added acceptance proposals:** (QMD-AC-09) local Git content write acknowledged while QMD is absent -> index unavailable/pending, no rollback; (QMD-AC-10) completed audit-only commit -> no unnecessary content reindex; (QMD-AC-11) refresh invoked explicitly after multiple commits -> one up-to-date index checkpoint for the latest projected tree; (QMD-AC-12) interrupted refresh -> later explicit retry repairs derived data without rewriting source documents; (QMD-AC-13) CLI process exits without refresh -> no claim of automatic background completion.

**No product implementation, running worker, QMD integration test, benchmark or automatic scheduling** is asserted by this documentation amendment.
