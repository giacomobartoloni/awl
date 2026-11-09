# Metadata CLI — Experimental Separate Command Group

**Date:** 2026-11-09
**Status:** draft implementation on stacked PR #9, not a stable CLI contract or production release.
**Base:** draft PR #7 (experiment/go-local-init-read-write-2026-11-06), not main.

## Purpose

Keep awl edit restricted to the Markdown body. Add awl metadata for YAML frontmatter with no Activity/Spec/Plan-specific semantics. The direction follows core requirements FR-05/FR-16 and RFC 6901 JSON Pointer.

## Experimental commands

    awl metadata get Activities/example.md /status --json
    awl metadata set Activities/example.md /status completed --json
    awl metadata remove Activities/example.md /completed_at --json
    awl metadata insert Activities/example.md /tags/1 new --json
    awl metadata insert Activities/example.md /tags/- newest --json

    printf '%s' '{"/status":"completed","/completed_at":"2026-11-09"}' |
      awl metadata set Activities/example.md --input - --json

- get is read-only and reports a managed Git revision in JSON mode. Plain string output is unquoted, while other types print as JSON.
- set with a positional VALUE treats it as a string. To send a typed JSON literal, use --input FILE rather than a positional VALUE.
- remove deletes an existing scalar mapping field; it is distinct from assigning null.
- set with a path and explicit --input FILE (no pointer argument) accepts a nonempty JSON object mapping JSON Pointers directly to typed JSON values. Updates are ordered deterministically and staged all-or-nothing for the document. A wrapper such as {"set": {...}} is not accepted.
- insert accepts an existing list and a JSON Pointer ending with an index from 0 through the current list length, or `/-` for insertion at the end. No separate `awl metadata append` exists. A positional argument is a string; typed JSON can be passed with explicit `--input`, as with single-field set.
- --request-id uses the experimental audited Git replay contract for mutations. --json uses the draft response envelope.
- Multiple distinct operations in awl apply retain their established independent-failure/continue behavior.

The example stores quoted YAML strings, including completed_at: "2026-11-09". It does not guess the intended implicit YAML timestamp type.

## Supported subset and preservation

The code parses YAML to locate a value, but changes only the original scalar byte span rather than serializing the complete document. Untouched metadata, legacy links, body and original CRLF/LF line endings remain unchanged in supported cases.

Supported in this experimental increment: read nested mappings and list elements; replace existing scalar values in block mappings (including nested maps); remove an existing scalar field; change several existing fields atomically in one metadata operation; and insert into safely isolated, one-line YAML flow sequences or simple block sequences of scalar items. For insertion, the original elements are verified after a byte-local change. All other fields and the Markdown body remain byte-identical.

Unsupported and rejected: creating missing keys, root or mapping replacement, mutation through a positional parent list, multi-line flow sequences, complex/nested flow-list layouts, block sequences with non-scalar or multi-line original elements, anchored/aliased/tagged/duplicate/non-string-key YAML, block scalar rewrite, inline flow-map mutation and unsafe source spans. List replacement/removal is not yet supported. No implicit migration or status lifecycle rules.

Concurrency: a failed local Git ref update rechecks targeted scalar values before rebasing scalar metadata operations. Sequence insertion is **never automatically rebased** after another writer advances the Git head; it reports a conflict instead. A caller operating on an already stale list snapshot cannot yet prove its expected insertion position: an explicit positional-precondition design remains a review gate before declaring indexed insertion production-safe. These checks do not prove power-loss durability.

## Verification and remaining decisions

The CLI tests cover status/completed_at transitions, preserving CRLF body and comments, stable request replay, failed multi-field atomicity, nested JSON Pointer and literal-dot differentiation, list-read versus list-write rejection, scalar removal, and malformed/ambiguous YAML rejection. GitHub Actions on this PR provide the actual test status.

Open for later review: command/JSON grammar, structured edit-list payloads, arbitrary YAML syntax and formatting, missing-key semantics, robust sequence preconditions for stale callers, duplicate JSON input keys, concurrency fault injection, and production persistence gates.

Sources: [Core Requirements](../requirements/core-requirements.md), [Document Operation Contract](../requirements/document-operation-contract.md), [YAML path research](../research/2026-10-23-yaml-paths-and-partial-reads.md), [CLI proposal PR #6](https://github.com/giacomobartoloni/awl/pull/6), [Go draft PR #7](https://github.com/giacomobartoloni/awl/pull/7).
