# ADR 0001 — Use a Modular Monolith for awl

- **Status:** Accepted by the project owner on 2026-11-06; repository integration pending review and merge
- **Date:** 2026-11-06
- **Decision makers:** Project owner
- **Product:** AgentWorklog CLI (`awl`), not the existing AgentWorklog Markdown worklog conventions
- **Related:** [Synchronization modes and transport design](../architecture/2026-11-06-sync-backends-and-transports-design-proposal.md) · [Document and operation contract](../requirements/document-operation-contract.md) · [Local-first storage evaluation](../architecture/2026-10-23-local-first-storage-evaluation.md)

## Context

awl is a Go CLI for humans and coding agents, with an offline-capable managed local Git document store and request-level audit. The current Git authority model and the experimental local Go implementation are distinct from proposed future transports such as rsync and rclone, search-index engines, and potential provider-neutral remote authorities.

The owner considered whether the project needs a full Hexagonal Architecture (ports and adapters) now. A mandatory ports/interfaces layer, dependency-injection framework, generic plugin registry, or one package per technical role would introduce abstractions before another backend is implemented and its actual capabilities verified.

At the same time, putting CLI parsing, Markdown mutation policy, Git object/ref operations, remote synchronization, and external transfer commands in a single undifferentiated component would make future changes and tests harder.

## Decision

Build awl as a **modular monolith**.

1. **One deployable CLI and in-process application**, rather than a service mesh, compulsory daemon, separate provider service, or generalized runtime plugin host.
2. **Separate cohesive responsibilities** in code and tests as they become necessary:
   - CLI input/output and argument parsing;
   - project initialization, identity and configuration resolution;
   - document operation validation, ordered composite outcomes and audit rules;
   - durable local managed Git publication and recovery;
   - explicit remote synchronization with provider-specific reconciliation/acknowledgement semantics;
   - optional snapshot transfer/export and derived search indexing.
3. **Protect the application contract from external mechanisms.** Document operations should not depend directly on rsync/rclone flags, Git subprocess argument strings, Google Drive HTTP details or QMD internals. Configuration and Git store internals should not decide domain-specific Markdown conflict policy by accident. Use understandable function/package boundaries rather than mandatory architectural layers.
4. **Prefer concrete Go types and ordinary function calls initially.** Introduce a small Go interface at the package that consumes it only when testing or a second real implementation demonstrates a shared behavioral contract. Do not create `StorageBackend`, `SyncBackend`, `Repository`, or `Transport` interfaces merely in anticipation of future providers. Explicit capabilities and failure modes matter more than a uniform `Sync()` signature.
5. **Keep local acceptance separate from downstream effects.** Local request publication and audit happen through the managed Git store; remote synchronization, optional rsync/rclone export, and search-index refresh are independently invoked, recoverable operations. A downstream failure cannot roll back an acknowledged local commit.
6. **Treat different backends according to their actual semantics.** Git remote authority exchanges history and refs; rsync/rclone are primarily file-transfer tools. Alternative authoritative remote providers remain possible only after a separate decision and verified versioning, conditional publication, conflict, audit, and recovery capabilities.
7. **Do not freeze a package layout in this ADR.** Extract or reorganize packages when cohesion, maintainability and tests justify it. Go idioms, clear ownership, explicit errors, cancellation, small coherent units and YAGNI govern implementation.

Applying selected dependency-inversion and ports/adapters principles is compatible with this decision. **A full Hexagonal Architecture is not required.**

## Consequences

### Positive

- Minimal conceptual and operational overhead for a CLI-first, offline-first product.
- Existing experimental Go code can evolve by responsibility without a wholesale rewrite.
- Remote Git sync and future satellite export can be developed independently of document edits.
- Abstractions are based on proven behavior and real provider capability boundaries rather than invented interchangeability.

### Costs and constraints

- Future non-Git authoritative backends may require refactoring and formalization of ports after sufficient evidence exists.
- Developers must keep responsibility boundaries explicit; "monolith" does not justify mixing CLI parsing, document policy and external subprocess execution.
- A modular monolith alone does **not** establish correctness for concurrency, crashes, durability, remote acknowledgement, or provider migrations. Those remain separately tested design gates.

## Alternatives considered

- **Full Hexagonal Architecture from day one:** not selected because the provider contracts, multi-backend semantics and consumers of the proposed interfaces are not yet proven.
- **Undifferentiated single-package/script-like CLI:** not selected because it couples domain behavior to Git, configuration and transport details, impairing maintenance and testing.
- **Multiple services/mandatory daemon or plugin framework:** not selected; adds operational complexity without an accepted requirement.

## Scope and implementation status

**Accepted:** architectural style **modular monolith**, with selective ports/adapters and dependency inversion only where useful.

**Not accepted by this ADR:** replacing the currently recorded Git authority with a non-Git authority; implementing rsync/rclone/bisync; a package hierarchy; public Go backend interfaces; configuration schema additions; remote project discovery; an automatic worker; or any production-readiness claim.

**Implementation status (2026-11-06):** the Go local CLI remains an experimental draft in [PR #7](https://github.com/giacomobartoloni/awl/pull/7). The synchronization/transport proposal is draft [PR #8](https://github.com/giacomobartoloni/awl/pull/8). This ADR is a repository decision record, not evidence that provider implementations or safety gates were completed. It becomes part of the repository's mainline only when its containing PR is merged.

## Review trigger

Revisit this ADR only if validated requirements or measured maintenance problems justify a different structure—for example multiple independently implemented authoritative providers or a separately deployable component with a clear lifecycle and security boundary. Document supersession with another ADR rather than silently rewriting this decision.
