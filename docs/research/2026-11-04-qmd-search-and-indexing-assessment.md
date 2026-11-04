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
