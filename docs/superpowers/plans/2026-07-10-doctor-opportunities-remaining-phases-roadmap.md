# Doctor / Opportunities Remaining Phases Roadmap

**Objective:** Complete the approved two-line PRD from the live Phase 1A foundation through Phase 5, with one canonical writer at every cutover, independent deployability, rollback evidence, and production verification after every slice.

## Current authoritative state

- Phase 0 inventory foundation and Phase 1A deterministic shadow identity are live through PRs #303 and #304.
- Production shadow run `1556d7cb-7fc7-48ad-b12b-aed24d2b0d62` completed with 1 Doctor candidate, 6 Opportunity candidates, 3 identity-ready candidates, and 4 fail-closed candidates.
- Legacy Doctor and Opportunity writers remain canonical. No enforced arbitration, dedicated Site Fix model, Growth contract, visible two-loop UI, or legacy cutover has happened yet.

## Delivery slices

| Slice | Formal phase | Canonical result | Production proof |
|---|---|---|---|
| 1B. Semantic arbitration | Phase 1 | AI comparisons occur outside locks; bucket snapshots, decisions, review memory, and atomic reservations are persisted; stale snapshots fail closed | authenticated admin prepare/review flow, concurrency contract, AI ledger row, no duplicate active reservation |
| Doctor Site Fix ownership | Phase 2 | `site_fixes` becomes Doctor's dedicated work object; technical work no longer depends on `seo_opportunities` or `content_actions` | migrated real technical work, application/PR/deploy/verify lifecycle, Doctor stops at `verified` |
| Opportunity Growth contract | Phase 3 | Growth Opportunity/Action requires hypothesis, baseline, metric, finite window, outcome and learning; Signal Scan and AI Discovery become internal stages | production opportunity runs contain complete contracts and terminal measurement policy |
| Scheduler consolidation | Phase 3 | shared evidence refresh has collection-spec idempotency; one scheduler per product line; standalone weekly GEO authority removed | scheduler diagnostics show no duplicate source/target/window/provider runs |
| Visible closed loops | Phase 4 | Doctor, Opportunities, Home and Results expose separate linked lifecycles without internal hold queues | browser verification of both loops from evidence to verified/measured result |
| Legacy migration and cleanup | Phase 5 | migration ledger, row conservation, one-of-source applications, review-memory migration, legacy writer fencing and rollback drills | dry-run and forward/rollback reports on production data, tightened constraints, legacy routes link canonical resources |

## Non-negotiable gates

1. Every code slice starts from the latest `origin/main` in a fresh worktree.
2. Every behavior change follows test-first red/green evidence.
3. No provider call occurs inside a database transaction, row lock, or advisory lock.
4. No cutover enables dual-write; each project has exactly one canonical writer.
5. Automatic semantic suppression remains disabled until a versioned real-data gold set proves the frozen precision/recall/backlog threshold and Ops capacity.
6. Every migration-derived row has a batch ledger and versioned inverse operation before cutover.
7. A phase is not complete until its PR is merged, deployment is successful, and its production proof is observed.

## Completion audit

The final audit maps PRD acceptance criteria 1–47 and every Given/When/Then scenario to code, database constraints, tests, migration reports, and production/browser evidence. Missing or indirect evidence keeps the overall objective active.
