# PRD: CiteLoop Visibility Analysis to Action Loop

> Date: 2026-07-02
> Status: execution PRD
> Source discussion: user feedback that the current experience is clear at layer 1
> (blog publishing) but unclear at layer 2 (operator deliverables) and layer 3
> (feedback / operations signals).
> Product shift: from "content pipeline" to "proactive SEO/GEO action loop".

## 1. Product Thesis

CiteLoop should not feel like a backend that eventually publishes a blog post. It
should feel like a system that finds SEO/GEO visibility gaps, explains what to do
next, executes the chosen work, and reports what happened after publication or
application.

Chat-first SEO/GEO products can answer user questions and produce recommendations
when the user asks. CiteLoop can be more proactive: it should continuously emit
prioritized action items, explain why each action matters, push accepted work into
execution, and close the loop with measurement and operational health signals.

The user-facing loop is:

```text
Signals -> Opportunity Briefs -> Action Portfolio -> Published / Applied Assets -> Impact Reports -> Learning
```

## 2. User-Perceived Outputs

### 2.1 Layer 1: Published or Applied Assets

This is the layer users already understand: CiteLoop can publish canonical blog
content to `unipost.dev` and, as the product expands, can publish or apply other
asset types.

Layer 1 outputs include:

- New canonical articles.
- Refreshed existing articles.
- Title, meta, H1, intro, evidence, or internal-link patches.
- GEO citation-ready assets such as comparison, alternative, glossary, benchmark,
  source, template, or docs pages.
- Technical visibility fixes when the publisher/CMS integration can apply them
  safely.

### 2.2 Layer 2: Operator Deliverables

The operator should not need to infer what CiteLoop is doing from database-like
statuses. CiteLoop must create explicit working objects:

| Deliverable | What It Is | How Users Use It | SEO/GEO Contribution |
|---|---|---|---|
| Opportunity Brief | A prioritized recommendation with evidence, "why now", expected impact, risk, effort, and suggested action. | Review, dismiss, or create an action. | Turns raw SEO/GEO signals into concrete work. |
| Action Portfolio | The selected/deferred/rejected set of actions, grouped by risk, action type, asset type, approval need, and measurement schedule. | Decide what CiteLoop should execute now versus later. | Prevents every signal from becoming a blog; makes metadata, refresh, internal-link, GEO, and technical actions visible. |
| Human Decision Gate | The subset of work needing product, factual, brand, safety, or publishing judgment. | Approve, reject, edit, or request regeneration. | Keeps high-risk SEO/GEO changes controlled. |
| Asset / Patch Preview | The concrete article, patch, diff, external surface, or technical task before it is published/applied. | Inspect before approval. | Makes execution trustable and reversible. |

### 2.3 Layer 3: Feedback and Operations Signals

Layer 3 is not "more content". It is how users understand whether the loop is
healthy and whether previous actions are contributing to SEO/GEO outcomes.

| Signal | What It Is | How Users Use It | SEO/GEO Contribution |
|---|---|---|---|
| Impact Report | Before/after measurement for an action, with outcome label, confidence, data gaps, and confounders. | Decide whether a tactic worked, needs more time, or should be changed. | Feeds future prioritization and avoids fake causality. |
| Measurement Queue | Actions waiting for due checkpoints. | Know what is too early versus stuck. | Keeps published/applied work accountable. |
| Health / Blockers | GSC, GA4, publisher, crawl, verification, budget, policy, and safe-mode blockers. | Fix the bottleneck that prevents better recommendations or execution. | Improves signal quality and execution reliability. |
| Learning Signal | Aggregated patterns from completed actions. | Understand which action types, asset types, pages, or prompts are working. | Adjusts future prioritization without pretending every change has guaranteed impact. |

## 3. Current Mainline Baseline

This PRD is based on the current `origin/main`, not the older review snapshot.
Several items that were previously gaps are already implemented in mainline.

Implemented baseline:

- Backend route `GET /api/projects/{projectID}/seo/visibility/summary` exists and
  returns capability mode, primary status, blockers, open opportunities, actions
  in loop, lifecycle counts, measurement updates, and diagnostics health.
- Backend lifecycle derivation maps raw action/opportunity data into presentation
  states: `detected`, `added_to_plan`, `planned`, `drafting`,
  `ready_for_review`, `approved`, `published_or_applied`, `measuring`,
  `learned`, and `blocked`.
- `acceptSEOOpportunity` aliases the shared content-action creation path and no
  longer creates new accepted-without-action orphan rows.
- `content_actions` has multi-surface execution fields including `asset_type`,
  `target_surface_id`, `risk_reasons`, `evidence_snapshot`, `input_snapshot`,
  `output_snapshot`, `diff_snapshot`, `review_required`, `approved_at`,
  `verified_at`, and `verification_snapshot`.
- `input_snapshot` is now on `content_actions`; older wording that cited
  `seo_experiments` as the precedent was incorrect. The historical similar field
  was on `autopilot_runs`.
- `seo_opportunities.status` supports `open`, `accepted`, `dismissed`,
  `converted`, `done`, and `stale`. Product lifecycle derivation must handle all
  of them even when the UI mostly shows open/converted/dismissed.
- GSC-backed opportunities exist for low CTR, query gap, striking distance, and
  content decay.
- Action Portfolio UI and portfolio-shaped API contracts exist.
- `measurement.window_due` is handled by the scheduler, due measuring actions are
  selected, checkpoints are updated, `action_measurements` rows are upserted, and
  `content_actions.outcome_summary` is updated.
- Analysis and Results are split into separate routes; legacy SEO/Visibility
  routes redirect to Results.

Known remaining product gaps:

- Home and Content Plan still derive some opportunity/action counts separately
  instead of fully consuming the shared visibility summary lifecycle source.
- The user can still experience Content Plan as the center of gravity because the
  strongest visual affordances are around topics/drafts, not around the broader
  action portfolio.
- Direct patch / technical / internal-link actions have schema support, but they
  need production-proven routing so they do not create unnecessary article topics.
- Remaining analyzer classes such as internal-link gap, schema gap,
  cannibalization, thin evidence page, and deeper technical visibility issues are
  not yet all first-class.
- Production verification has to be performed phase by phase after merge.

## 4. Product Principles

1. Action over dashboard: every insight should have a next action or a clear
   reason it cannot be acted on.
2. One loop: SEO and GEO share opportunity, action, review, publish/apply, and
   measurement lifecycle.
3. No fake metrics: disconnected GSC/GA4 projects must not show CTR, position,
   clicks, sessions, or citation movement as facts.
4. No fake causality: Results can describe associated movement and confidence,
   but cannot claim guaranteed ranking, traffic, conversion, or citation gains.
5. Human where judgment matters: product facts, positioning, brand risk,
   deletion/noindex/redirect/canonical changes, and high-risk publishing require
   human review.
6. Backend-owned lifecycle: user-facing lifecycle counts must come from one
   backend summary contract, not from independent frontend aggregations.

## 5. Lifecycle Contract

The shared lifecycle source is:

```text
GET /api/projects/{projectID}/seo/visibility/summary
```

The endpoint owns the presentation lifecycle. Frontend helpers may normalize and
render the result, but should not become a second source of truth for cross-page
counts.

Lifecycle derivation rules:

- `detected`: open opportunity with no content action yet.
- `added_to_plan`: content action exists, but no topic, draft, output snapshot,
  diff snapshot, publication, verification, or measurement has started.
- `planned`: action is linked to a topic or equivalent execution plan.
- `drafting`: action or linked topic/article is being generated.
- `ready_for_review`: draft, output snapshot, diff snapshot, or review payload is
  ready for human decision.
- `approved`: action is approved but not yet published/applied.
- `published_or_applied`: action has `published_at`, `verified_at`, status
  `published`, or equivalent applied verification.
- `measuring`: action is in a measurement window.
- `learned`: action has completed measurement and has an outcome summary.
- `blocked`: failed, verification failed, recovery required, or missing required
  setup prevents progress.

Closed raw opportunity statuses:

- `dismissed`, `done`, and `stale` are not active lifecycle rows unless a linked
  content action is still in motion.
- Legacy `accepted` rows without a content action are blocked data that must be
  cleaned or converted; new accept flows must create an action.

## 6. Strict Phase Gates

Every phase below has explicit exit criteria. A phase is not complete until all of
its exit criteria pass in automated checks and the production verification steps
pass after merge/deployment.

### Phase 0: PRD Gate and Current-State Reconciliation

Goal: make the PRD executable before additional product/code work continues.

Scope:

- Add this PRD to the repo.
- Correct the factual review items against current `origin/main`.
- Map every acceptance criterion to a phase.
- Keep existing behavior unchanged except documentation/plan updates.

Exit criteria:

1. The PRD states the current mainline baseline and does not claim implemented
   mainline features are missing.
2. Each phase has strict exit criteria, automated verification, production
   verification, and "cannot move next" conditions.
3. The PRD explicitly resolves:
   - visibility summary is the single lifecycle source;
   - accept creates an action instead of accepted-only status;
   - `added_to_plan` versus `ready_for_review` is derived by linked topic, draft,
     output, or diff evidence rather than raw status alone;
   - measurement due work is handled by scheduler sweep/event processing.
4. Baseline tests pass in a clean worktree from latest `origin/main`:
   - `make test`
   - `npm test` in `web`
   - `npm run typecheck` in `web`
5. The Phase 0 PR is merged to `origin/main`.
6. Production deploy completes or the deployment system explicitly reports that a
   docs-only change did not trigger a deploy.
7. Production smoke check confirms the app still loads Home, Analysis, Content
   Plan, and Results for an existing project.

Cannot move to Phase 1 if:

- The PRD still has unassigned phase acceptance criteria.
- Baseline tests fail for a reason introduced by the Phase 0 branch.
- The Phase 0 PR is not merged.
- Production smoke cannot be verified and the blocker is not documented.

### Phase 1: Shared Visibility Summary UX

Goal: make users feel the loop, not just a content queue, across Home, Analysis,
Content Plan, and Results.

Scope:

- Use visibility summary as the shared source for open opportunity count,
  lifecycle counts, actions in loop, blockers, and measurement updates.
- Preserve Analysis as the primary Opportunity Brief review surface.
- Make Home communicate the next SEO/GEO action item, not only content progress.
- Make Content Plan explain when accepted analysis is becoming topics and when an
  accepted action is not a blog topic.

Exit criteria:

1. Home, Analysis, Content Plan, and Results use the same visibility summary
   response for lifecycle/open/in-loop counts, with no contradictory numbers for
   the same project state.
2. Home's primary action can point to Analysis when opportunities are detected and
   to Results when measurement/verification needs attention.
3. Analysis first fold shows:
   - Opportunity Briefs / growth findings first;
   - Loop in motion counts;
   - setup/data blockers when present;
   - no raw "content pipeline" wording as the only explanation.
4. Content Plan differentiates:
   - topic backlog;
   - accepted actions being planned;
   - non-topic actions that should be reviewed/applied elsewhere.
5. Disconnected GSC projects do not show CTR, position, clicks, impressions, or
   traffic movement as measured facts.
6. At 1440x900 and 390x844, the default Analysis page reveals the review queue
   and loop status within 1.4 viewport heights using seeded/fixture data.
7. Contract tests cover the shared summary usage and lifecycle copy.
8. Production verification captures Home, Analysis, Content Plan, and Results
   screenshots plus the production visibility summary API response for one
   project.

Cannot move to Phase 2 if:

- Any page derives a lifecycle count independently and can disagree with summary.
- The first fold still makes the product feel like only "publish a blog".
- GSC-disconnected production view displays unavailable search metrics as facts.

### Phase 2: Multi-Surface Action Routing

Goal: prove that CiteLoop outputs more than blog posts.

Scope:

- Route each accepted opportunity to the right action shape:
  - new article/page actions create or link a topic;
  - refresh actions can target existing articles;
  - metadata, internal-link, schema, technical, distribution, and GEO citation
    actions can exist without unnecessary article topics.
- Use `asset_type`, `input_snapshot`, `output_snapshot`, `diff_snapshot`,
  `review_required`, `verified_at`, and `verification_snapshot` for non-blog
  outputs.

Exit criteria:

1. New-asset actions create or link a topic with
   `topics.source_content_action_id`.
2. Direct patch or technical actions do not create topics unless the action truly
   needs a new content asset.
3. Direct patch or technical actions produce reviewable output or diff snapshots
   before approval.
4. UI copy and CTAs distinguish at least:
   - create content task;
   - create refresh task;
   - create technical task;
   - create internal-link task;
   - create GEO asset task.
5. Action cards answer:
   - why now;
   - what to do;
   - how it contributes to SEO/GEO;
   - output type;
   - what happened after execution.
6. Automated tests prove both topic-backed and non-topic action routing.
7. Production verification creates or inspects one topic-backed action and one
   non-topic action, and confirms their records and UI states differ correctly.

Cannot move to Phase 3 if:

- Every accepted action still becomes a topic/blog by default.
- Non-blog actions have no visible deliverable for the user to review.
- Traceability from opportunity to action to output is missing.

### Phase 3: Analyzer Expansion to Actionable SEO/GEO Items

Goal: supply enough high-signal recommendations that users see non-blog SEO/GEO
work without having to ask a chat interface.

Scope:

- Keep existing GSC metric opportunities:
  - low CTR;
  - query gap;
  - striking distance;
  - content decay.
- Add or harden first-class analyzers for:
  - internal-link gap;
  - schema gap;
  - cannibalization;
  - thin evidence page;
  - technical visibility issue;
  - GEO citation/source gap where observation data exists.

Exit criteria:

1. Each analyzer has deterministic fixture tests that produce at least one
   opportunity when data supports it and zero opportunities when required data is
   absent.
2. Each opportunity has `source`, `why_now`, `scoring_method` or equivalent
   rationale, `recommended_action`, `expected_impact`, confidence, effort, risk,
   and idempotency key.
3. Analyzer reruns do not create duplicate open opportunities for the same
   project/problem.
4. At least one supported analyzer can generate a non-blog action recommendation.
5. The UI can show a non-blog recommendation in Analysis with the correct CTA and
   risk/review copy.
6. Production verification runs analysis on a project with supporting data or a
   controlled fixture project and confirms the expected opportunity types appear.

Cannot move to Phase 4 if:

- Analyzer output is mostly generic topic/blog suggestions.
- GSC/GEO unavailable states produce fake precision.
- Idempotency allows repeated analyzer runs to flood the queue.

### Phase 4: Measurement Closure and Impact Reports

Goal: make the feedback layer visible and trustworthy.

Scope:

- Publish/apply actions transition to measuring with baseline and measurement
  windows.
- Scheduler closes due checkpoints.
- Results presents Impact Reports without overstating causality.
- Measurement gaps become useful health signals.

Exit criteria:

1. Publish/apply paths set enough execution metadata for measurement:
   `published_at` or `verified_at`, measurement window, primary metric, and
   checkpoint schedule.
2. A due checkpoint creates or updates `action_measurements` and refreshes
   `content_actions.outcome_summary`.
3. Results shows outcome labels including positive, negative, mixed,
   inconclusive, and insufficient data where applicable.
4. Results copy separates measured movement, confidence, confounders, and data
   gaps.
5. Measurement queue distinguishes waiting, too early, blocked, and completed
   states.
6. Automated tests cover checkpoint due processing and Results rendering
   contracts.
7. Production verification advances or recomputes at least one measurement state
   and confirms the Results page reflects it.

Cannot move to Phase 5 if:

- Published/applied actions disappear after execution.
- Measurement updates write backend data but do not appear in Results.
- Results claims guaranteed SEO/GEO lift.

### Phase 5: Information Architecture, Diagnostics, and Learning

Goal: make the whole product read as a proactive growth operating system.

Scope:

- Refine page roles:
  - Home: Growth Control Center.
  - Analysis: Opportunity Brief workspace.
  - Content Plan: topic backlog plus action portfolio handoff.
  - Review: human decision gate.
  - Publish: assets and distribution.
  - Results: Impact Reports and learning.
  - Activity/Settings: operations health and diagnostics.
- Surface learning and health signals without burying them in logs.

Exit criteria:

1. Navigation and page copy no longer imply every workflow ends in a blog post.
2. Home exposes at least one of:
   - highest-priority Opportunity Brief;
   - action blocked by operations health;
   - measurement result needing attention;
   - learning signal from completed work.
3. Results separates Impact Reports from diagnostics and raw logs.
4. Settings/Activity exposes operational blockers without replacing product
   pages as the main user output.
5. Learning signals are conservative: they can inform prioritization but do not
   auto-change risky behavior without policy gates.
6. Production verification includes desktop and mobile screenshots for Home,
   Analysis, Content Plan, Results, and Activity/Settings.

Cannot finish the epic if:

- Users still primarily perceive the backend as "a content pipeline that publishes
  blog posts".
- Layer 2 deliverables and Layer 3 feedback are not visible on product pages.
- Production verification evidence is missing for any phase.

## 7. Acceptance Matrix

| AC | Phase | Strict Acceptance |
|---|---:|---|
| AC-01 | 0 | PRD has phase-specific exit criteria, verification commands, production checks, and cannot-move-next conditions. |
| AC-02 | 0 | Current mainline baseline is reconciled with actual code; no stale factual claims remain. |
| AC-03 | 1 | Home, Analysis, Content Plan, and Results agree on lifecycle/open/in-loop counts from visibility summary. |
| AC-04 | 1 | Analysis first fold reveals Opportunity Briefs and Loop in motion within 1.4 viewport heights at 1440x900 and 390x844. |
| AC-05 | 1 | GSC-disconnected projects do not show CTR, position, clicks, impressions, or movement as facts. |
| AC-06 | 1 | Converted opportunities remain visible as actions in motion instead of disappearing from the user. |
| AC-07 | 2 | New-asset actions create/link topics through `topics.source_content_action_id`. |
| AC-08 | 2 | Direct patch/technical/internal-link actions do not create unnecessary article topics. |
| AC-09 | 2 | Action cards explain why now, action, SEO/GEO contribution, output type, and post-execution state. |
| AC-10 | 3 | Analyzer can produce at least one non-blog SEO/GEO recommendation when data supports it. |
| AC-11 | 3 | Analyzer reruns are idempotent and do not flood duplicate open opportunities. |
| AC-12 | 4 | Publish/apply moves action into measuring with checkpoint schedule. |
| AC-13 | 4 | At least one due checkpoint writes `action_measurements` and updates `outcome_summary`. |
| AC-14 | 4 | Results renders Impact Reports with confidence, confounders, and insufficient-data states. |
| AC-15 | 5 | Product pages visibly separate Opportunity Briefs, Action Portfolio, Impact Reports, and operations health. |

## 8. Required Execution Protocol

For each phase:

1. Start from latest `origin/main` in a clean branch/worktree.
2. Write failing tests before behavior changes.
3. Implement the smallest change that satisfies the phase exit criteria.
4. Run backend and frontend verification required by that phase.
5. Commit and push the phase branch.
6. Create PR to `origin/main`.
7. Merge the PR.
8. Wait for deployment or document why no deployment was triggered.
9. Verify production against the phase production checklist.
10. Only then move to the next phase.

Minimum local verification for any code phase:

```bash
make test
cd web
npm test
npm run typecheck
```

When UI changes are included, add browser verification for the affected pages at:

- 1440x900 desktop.
- 390x844 mobile.

## 9. Open Decisions

These are intentionally left as phase inputs, not blockers to Phase 0:

1. Whether non-blog patch outputs should remain in `content_actions.output_snapshot`
   / `diff_snapshot` or move to normalized patch tables after volume grows.
2. Whether lightweight "Ask why" chat should be added after action item contracts
   stabilize, or deferred until later.
3. How aggressive learning signals can be before policy gates require human
   review.

