# PRD: CiteLoop Loop Lifecycle and Content Plan UX Clarification

> Date: 2026-07-05
> Status: Draft revised after Claude Code review
> Scope: Analysis `Loop in motion`, Content Plan handoff, topic backlog, lifecycle labels, and deep-link behavior
> Source discussion: user observed that an opportunity appears as `Planned` in `Loop in motion` while the same item appears under `Accepted opportunities` in Content Plan, with `Planned topics` showing `0`.
> Relationship to prior PRDs: This document supersedes the work-queue status
> language in `docs/PRD-CiteLoop-Opportunity-Review-and-Work-Queues.md`
> Section 10.2 for shared lifecycle labels.

## 0. Summary

CiteLoop currently exposes the growth loop as a set of lifecycle metrics in
Analysis and as execution queues in Content Plan. The underlying product model is
reasonable, but the UI language makes one item appear to be in two conflicting
states:

```text
Analysis / Loop in motion: Planned
Content Plan: Accepted opportunities
Content Plan / Planned topics: 0
```

The user problem is not just a badge mismatch. Three different objects are being
compressed into two similar labels:

- an accepted opportunity;
- the content action created from that accepted opportunity;
- the planned topic created from that content action.

This PRD defines the product semantics, UX changes, routing behavior, and
acceptance criteria needed to make the loop understandable. After this work, a
user should be able to answer three questions without reading internal status
rules:

1. Where is this opportunity now?
2. What object currently represents it: action, topic, draft, site fix, or result?
3. What is the next useful action?

## 1. Problem Statement

### 1.1 User-Visible Confusion

The current UI can show the same work item as:

- `Planned` in Analysis `Loop in motion`;
- `Accepted` inside Content Plan;
- absent from `Planned topics`.

This creates a perceived contradiction. A user reasonably expects:

- if something is `Planned`, it should appear in `Planned topics`;
- if it is still `Accepted`, it should not be counted as `Planned`;
- if `Planned topics` is `0`, there should not be a planned content item.

### 1.2 Root Cause

The UI currently mixes object lifecycle, source state, and queue membership:

| Concept | What It Means | Current UI Risk |
|---|---|---|
| Accepted opportunity | A recommendation from Opportunity Queue has been accepted. | It is presented as if it is still the primary state even after downstream work starts. |
| Content action | The execution object created from the accepted opportunity. It can be a content brief, direct patch, site fix, or review item. | It appears in Content Plan without a clear action-level lifecycle. |
| Planned topic | A topic backlog item created from a content action that needs content generation. | It uses the word `planned`, which overlaps with Loop lifecycle `Planned`. |

The product model needs explicit language for each layer.

## 2. Goals

1. Make Analysis lifecycle labels and Content Plan queue labels non-contradictory.
2. Make `Loop in motion` metric cards behave as filters into the exact current objects behind each count.
3. Make every non-zero lifecycle card reveal cards that link to the precise current surface.
4. Distinguish `Content handoff` from `Topic backlog`.
5. Clarify when an accepted opportunity becomes a planned topic.
6. Clarify how planned topics participate in the Growth loop.
7. Preserve the broader SEO/GEO action loop, including direct actions that never become topics.

## 3. Non-Goals

- Do not redesign the entire dashboard navigation.
- Do not change backend database object names unless needed by implementation.
- Do not remove direct actions, site fixes, schema fixes, metadata rewrites, or technical fixes from the Growth loop.
- Do not force every accepted opportunity to become a topic.
- Do not introduce new analytics claims or success metrics.
- Do not merge Analysis and Content Plan into one page.
- Do not turn this PRD into an implementation plan; implementation should follow after review.

## 4. Product Terminology

The UI should consistently use these meanings.

### 4.1 Opportunity

An opportunity is a recommendation found by Analysis. It represents a possible
SEO/GEO improvement before the user commits to doing the work.

Examples:

- refresh an existing page;
- add structured data;
- add internal links;
- create supporting content;
- improve title or metadata;
- fix a technical visibility issue.

### 4.2 Accepted Content Work

Accepted content work is an opportunity that has been accepted and converted into
a content action. This is the handoff object from Analysis into execution.

It may or may not become a topic.

### 4.3 Content Action

A content action is the execution object. It can route to:

- Content Plan, when it needs content planning or drafting;
- Site Fixes / Analysis, when it is a direct fix or technical action;
- Review, when a draft, diff, or patch is waiting for approval;
- Results, after publication, application, measurement, or learning.

### 4.4 Topic

A topic is a backlog item for content generation. It is created only when the
accepted content action needs a content asset.

Examples that may become topics:

- new blog post;
- supporting page section;
- existing page expansion;
- comparison page;
- glossary or explainer asset;
- evidence-led page refresh that needs a draft.

Examples that should not become topics:

- metadata rewrite;
- title or meta description patch;
- schema patch;
- sitemap update;
- internal link patch;
- robots, canonical, crawler, or technical SEO fix.

### 4.5 Draft

A draft is generated content or a reviewable output. Once a draft exists, the
work should be discoverable from Review, not only from Content Plan.

### 4.6 Result

A result is an action that has been published, applied, verified, measured, or
learned from. Results should be represented on the Results surface and deep-link
to attribution details.

## 5. Current Baseline

Current code already has useful foundations that should be reused:

- `GET /api/projects/{projectID}/seo/visibility/summary` exposes
  `actions_in_loop` and `lifecycle_counts`.
- Analysis `Loop in motion` renders lifecycle counts.
- Non-zero metric cards can reveal filtered action cards.
- Content Plan consumes `VisibilitySummary`.
- Content Plan has a handoff section for actions from Opportunity Queue.
- Content Plan has a separate topic backlog section.
- `planSEOContentAction` can create a topic from a content action.
- Direct actions are filtered out of topic planning by asset/action type.
- Auto mode can convert accepted content work into planned topics and drafts on
  cadence.
- Auto Off pauses automatic planning/drafting while preserving manual workflow.

The remaining gap is product clarity, not raw capability.

## 6. Target Information Architecture

Content Plan should have two clearly separated sections.

### 6.1 Section A: Content Handoff

Replace:

```text
Accepted opportunities
Sent from Opportunity Queue
```

With:

```text
Content handoff
Accepted from Opportunity Queue
```

Purpose:

- Show accepted work that has entered execution.
- Make clear that these are content actions, not raw opportunities.
- Show the action's current lifecycle.
- Provide the next action or precise link.

This section can include items that are accepted, topic planned, drafting, or
ready for review. The section name should not imply all items are still only
accepted.

### 6.2 Section B: Topic Backlog

Replace:

```text
Planned topics
Draft queue
```

With one of the following:

Recommended:

```text
Topic backlog
Draft queue
```

Alternative:

```text
Planned topic backlog
Draft queue
```

Purpose:

- Show actual `Topic` objects.
- Avoid confusing topic backlog count with action lifecycle count.
- Make it obvious that not every accepted action belongs here.

## 7. Lifecycle Label Model

Analysis `Loop in motion` labels should describe current execution state, not
source history.

This section is the single product vocabulary source for shared lifecycle labels
across Analysis, Content Plan, Review, Publish/Apply, and Results. Other PRDs may
describe page-specific helper copy, but should not define competing labels for
the same lifecycle keys.

### 7.1 Recommended Labels

| Lifecycle Key | Current Label | Recommended Label | Meaning |
|---|---|---|---|
| `added_to_plan` | Added | Added | A content action exists, but no topic or draft exists yet. |
| `planned` | Planned | Topic planned | The action has a linked topic. |
| `drafting` | Drafting | Drafting | The system is generating content/output. |
| `ready_for_review` | Review | Review | A draft, patch, diff, or output is waiting for human decision. |
| `approved` | Approved | Approved | Work was approved but not yet published/applied. |
| `published_or_applied` | Published | Published/Applied | Work is live or applied. |
| `measuring` | Measuring | Measuring | The result is inside the measurement window. |
| `learned` | Learned | Learned | Measurement has produced an outcome. |
| `blocked` | Blocked | Blocked | The work cannot progress without recovery. |

The key change is `planned -> Topic planned`. This makes the lifecycle label
specific enough that users do not expect every accepted item to appear under a
generic `Planned topics` label.

Do not rename `added_to_plan` to `Accepted` in the metric strip. `Accepted` is
the Opportunity Queue decision history; `Approved` is a later execution state.
Using both as lifecycle metric labels would create a second vocabulary collision.
The metric should keep `Added`, while Content Plan can still say the work was
`Accepted from Opportunity Queue` at the section/source level.

Current code derives `planned` only when a topic exists (`topic_id` or
`topic_status`). Direct actions do not enter `planned` in V1. Their route is:

```text
Added -> Review/Approved -> Published/Applied -> Measuring -> Learned
```

### 7.2 Label Principles

- Labels should describe current state, not where the item came from.
- Labels should be object-aware when needed: `Topic planned` is clearer than
  `Planned`.
- Labels should not imply that a direct patch or technical fix became a topic.
- Card badges should show both lifecycle and route when helpful.
- Reuse the existing destination badge mapping introduced for loop cards:
  `Site Fixes` uses blue with the wrench icon; `Content Plan` uses violet with
  the document icon. Do not introduce a second color language for the same
  destinations.

Example badge combinations:

```text
Added + Content handoff
Topic planned + Content Plan
Drafting + AI Editor
Review + Review
Published/Applied + Results
Blocked + Needs recovery
```

## 8. Content Handoff Card Behavior

Each card in Content Handoff should answer:

1. What is this work?
2. Where is it now?
3. What is the next action?

### 8.1 Card Fields

Required:

- lifecycle badge;
- route/surface badge;
- title;
- target URL or affected surface;
- next action CTA;
- deep-link target.

Recommended optional fields:

- action type: `Page update`, `New content`, `Metadata`, `Schema`, `Internal link`, `Technical fix`;
- risk badge when medium/high risk.

Auto state is page-level in V1, not card-level. Content Plan should explain
`Auto on` / `Auto paused` with a page banner or header control. Individual cards
should not invent per-card Auto state unless the backend later exposes per-card
automation ownership.

### 8.2 Badge Rules

Do not hardcode every handoff card as `Accepted`.

Use lifecycle-aware badges:

| Condition | Primary Badge |
|---|---|
| no topic, no draft | `Added` |
| topic exists and no draft exists | `Topic planned` |
| action/topic is generating | `Drafting` |
| draft or reviewable output exists | `Review` |
| action is approved | `Approved` |
| failed/recovery required | `Blocked` |

`Published/Applied`, `Measuring`, and `Learned` are Results-owned states. They
may appear in `Loop in motion`, but should not remain in the default Content
Handoff queue after Results has taken over.

### 8.3 CTA Rules

| Current Object | CTA |
|---|---|
| accepted content action with no topic | `Review brief` or `Draft content` |
| topic exists and no draft exists | `Open topic` or `Draft now` |
| draft exists | `View in Review` |
| direct/site fix output exists | `Review fix` |
| published/applied/measuring/learned when shown from history or Loop cards | `View results` |
| blocked | `Resolve blocker` |

Auto Off should not hide the manual path. It should explain:

```text
Auto paused. Manual drafting is still available.
```

### 8.4 Entry and Exit Rules

Content Handoff is an active handoff queue, not a second Results page.

Default Content Handoff includes:

- `added_to_plan` content actions;
- `planned` content actions, collapsed by default into an `In topic backlog (n)`
  group once their topic card exists;
- `drafting` content actions while generation is active;
- `ready_for_review` content actions until Review is clearly the owning surface.

Default Content Handoff excludes:

- `published_or_applied`;
- `measuring`;
- `learned`;
- dismissed, archived, stale, or completed items unless the user explicitly asks
  to show history.

Topic-backed work should not permanently duplicate between handoff and Topic
Backlog. Once a topic exists, the default handoff view should either collapse it
into an `In topic backlog (n)` group or replace the full handoff card with a
compact receipt link. The full topic card belongs in Topic Backlog. Once work is
published/applied, Results owns the default card.

## 9. Loop In Motion Interaction

Metric cards should remain filters, not direct links.

### 9.1 Default State

When no metric is selected:

- show metric cards only;
- do not duplicate Recently Sent content under the metrics;
- show a short empty/help line only if needed.

### 9.2 Selected Metric State

When the user clicks a non-zero metric:

- highlight the selected metric;
- show only cards belonging to that lifecycle;
- show cards at content/action granularity;
- each card links to the exact current object or focuses the exact current surface;
- zero metrics remain disabled.

### 9.3 Card Link Rules

| Lifecycle/Object | Link Target | Implementation Status |
|---|---|---|
| content action without topic | `/projects/{projectID}/plan?action={actionID}` | Existing; keep for handoff cards. |
| topic exists | `/projects/{projectID}/plan?topic={topicID}` | New; add in Phase 3. |
| draft exists | `/projects/{projectID}/review?article={draftArticleID}` | Existing; keep and verify from loop cards. |
| direct/site fix action | Analysis page in-page focus by action ID | Existing from loop card click; no persistent URL required in V1. |
| published/applied/measuring/learned | `/projects/{projectID}/results?action={actionID}` | Existing; keep and verify attribution focus. |
| blocked | current owning page plus action ID, with blocker section focused | New only if the blocker owner is not already addressable. |

The important UX rule: metric card selection shows the objects; the object cards
own the links. This is better than making the metric card itself a broad link.

## 10. Planned Topic Creation Rules

An accepted opportunity should become a Planned Topic only when it produces a
content asset that needs topic/draft handling.

### 10.1 Eligible for Topic Backlog

Eligible examples:

- create a new article;
- expand an existing page;
- create a supporting section;
- create a comparison, alternative, benchmark, glossary, or explainer asset;
- generate a content refresh brief that needs draft/review.

### 10.2 Not Eligible for Topic Backlog

Direct action examples:

- metadata rewrite;
- title patch;
- meta description patch;
- schema patch;
- internal link patch;
- sitemap update;
- robots/canonical/crawler fix;
- technical SEO fix.

These should still participate in the Growth loop, but through Site Fixes,
Review, Publish/Apply, Results, and measurement rather than Topic Backlog.

### 10.3 Auto On

When Auto is on:

```text
Accepted content work -> Topic backlog -> Drafting -> Review
```

The system may create topics and start drafting based on cadence, buffer, budget,
and safety gates.

### 10.4 Auto Off

When Auto is off:

```text
Accepted content work -> Content handoff
```

The item stays in handoff until the user manually reviews, drafts, dismisses, or
re-enables Auto. The UI must make this state explicit.

## 11. Growth Loop Participation

Topic Backlog is not separate from the Growth loop. It is one execution phase for
content-generating work.

The expected path is:

```text
Opportunity Queue
-> Accepted content work
-> Content handoff
-> Topic backlog
-> Drafting
-> Review
-> Published/Applied
-> Measuring
-> Learned
```

Direct actions use a parallel route:

```text
Opportunity Queue
-> Accepted direct action
-> Site Fix / Review
-> Applied
-> Measuring
-> Learned
```

Both paths should appear in `Loop in motion`. Only topic-backed work should
appear in Topic Backlog.

## 12. Empty States

### 12.1 Content Handoff Empty

When there are no accepted content actions:

```text
No accepted work in handoff
Accept opportunities from Analysis to create content work, site fixes, or reviewable actions.
```

### 12.2 Topic Backlog Empty

When there are no topic objects:

```text
No topics in backlog
Accepted content work that needs writing will appear here after it is planned. Direct fixes are tracked in Site Fixes instead.
```

This is more precise than:

```text
No planned topics yet
Review opportunities to add accepted content work.
```

The old copy makes users think any accepted opportunity should become a planned
topic.

## 13. Data Contract Expectations

The frontend should prefer the shared visibility lifecycle contract for cross-page
counts and status labels.

Confirmed V1 fields on `VisibilityActionInLoop`:

- `id`
- `opportunity_id`
- `lifecycle_stage`
- `status`
- `asset_type`
- `action_type`
- `target_url`
- `normalized_target_url`
- `topic_id`
- `topic_status`
- `topic_title`
- `draft_article_id`
- `draft_article_status`
- `published_at`
- `verified_at`
- `outcome_summary`

The frontend may derive display affordances from these fields, but it should not
create an independent cross-page lifecycle source that disagrees with
`visibility/summary`.

Fields to audit before Phase 2:

- `output_snapshot`
- `diff_snapshot`
- `input_snapshot`
- `evidence_snapshot`
- `risk_reasons`

The current frontend normalizer can default these fields from the broader
`SEOContentAction` type, but the visibility summary DTO should not be assumed to
emit them unless verified. If lifecycle-aware cards need these snapshots for
direct patch previews or evidence copy, Phase 2 must either add the missing DTO
fields or avoid depending on them.

## 14. Acceptance Criteria

### 14.1 Analysis `Loop in motion`

- Metric cards are filters.
- Zero metrics are disabled.
- Selecting a metric reveals only cards in that lifecycle.
- Cards are not duplicated from Recently Sent by default.
- The `planned` lifecycle is displayed as `Topic planned`.
- Every revealed card has a precise destination or in-page focus action.
- Results links open/focus the relevant Results attribution view.
- Content Plan links can target either `action` or `topic`.

### 14.2 Content Plan

- The top handoff section is named `Content handoff`.
- The handoff subtitle is `Accepted from Opportunity Queue`.
- Handoff cards show lifecycle-aware badges instead of always showing `Accepted`.
- The topic section is named `Topic backlog` or `Planned topic backlog`.
- Topic Backlog contains only actual topic objects.
- Empty state explains that direct fixes do not become topics.
- Auto Off clearly explains why accepted work remains in handoff.
- Manual drafting remains available when Auto is off.

### 14.3 Deep Links

- Existing `/plan?action={actionID}` scrolls to and highlights the handoff action.
- New `/plan?topic={topicID}` scrolls to and highlights the topic card.
- Existing `/review?article={articleID}` opens or focuses the draft review item.
- Existing `/results?action={actionID}` opens or focuses the attribution/result detail.
- Existing Site Fix/direct action loop cards focus the relevant Analysis surface without
  sending the user to a generic page top.

### 14.4 Product Semantics

- Accepted opportunity, content action, and planned topic are not used
  interchangeably in user-visible copy.
- Direct actions remain part of the Growth loop without being forced into Topic
  Backlog.
- The same item should not appear with only source-level `Accepted` copy on one
  page and lifecycle-level `Topic planned` copy on another without explanation.
- Content Handoff does not become a long-lived Results mirror; Results-owned
  stages leave the default handoff queue.

## 15. Suggested Implementation Phases

### Phase 1: Copy and Label Cleanup

Scope:

- Rename Content Plan `Accepted opportunities` to `Content handoff`.
- Rename `Planned topics` to `Topic backlog`.
- Rename Loop `Planned` label to `Topic planned`.
- Update empty states and helper text.
- Keep the `added_to_plan` metric label as `Added`.

Exit criteria:

- No code behavior changes beyond labels/copy.
- Screenshots confirm the previous contradiction is less likely.
- Contract tests cover the new labels.
- Responsive screenshots confirm the eight metric cards remain legible at the
  `sm` breakpoint; longer labels such as `Topic planned` and
  `Published/Applied` must not create broken truncation or overlap.

### Phase 2: Lifecycle-Aware Handoff Cards

Scope:

- Replace hardcoded `Accepted` badges with lifecycle-aware badges.
- Add route/surface badges.
- Add next-action CTA rules.
- Add or verify the page-level Auto state banner/header copy.
- Audit the visibility summary DTO fields before relying on snapshots,
  evidence, or risk data in cards; add missing DTO fields only if the card design
  needs them.
- Implement Content Handoff entry/exit rules, including default exit at
  `published_or_applied` and collapsed `In topic backlog (n)` handling for
  topic-backed items.

Exit criteria:

- Cards with topic show `Topic planned`.
- Cards without topic show `Added`.
- Draft-backed cards show `Review`.
- Direct actions show direct/site-fix route rather than topic route.
- Results-owned stages do not remain as full cards in default Content Handoff.
- Auto state appears once at the page/header level, not as invented per-card
  state.

### Phase 3: Topic Deep Links

Scope:

- Add `?topic={topicID}` support to Content Plan.
- Scroll/focus/highlight topic cards.
- Keep existing `?action={actionID}` behavior for handoff cards.
- Verify existing `/review?article={articleID}`, `/results?action={actionID}`,
  and Site Fix in-page focus from Loop cards.

Exit criteria:

- Analysis `Topic planned` cards with `topic_id` link to topic cards.
- Analysis cards without `topic_id` link to handoff cards.
- Browser verification confirms focus and highlight behavior for topic, action,
  review, results, and site-fix destinations.

### Phase 4: Growth Loop Explanation Polish

Scope:

- Add concise in-product copy where needed.
- Avoid visible tutorial blocks unless they directly reduce ambiguity.
- Prefer hover/help text, empty-state copy, and precise section labels.

Exit criteria:

- Users can infer why Topic Backlog can be empty while Content Handoff has items.
- Users can infer why direct fixes do not become topics.
- No page becomes visually heavier or more card-heavy than before.

## 16. UX Review Checklist

Claude Code should review the eventual implementation against these questions:

1. Can a user tell whether the item is an opportunity, action, topic, draft, or result?
2. Does any user-visible label use `planned` ambiguously?
3. Does Content Plan still imply every accepted opportunity becomes a topic?
4. Does clicking a Loop metric reveal precise cards rather than navigating to a broad page?
5. Does each revealed card have a precise link or focus behavior?
6. Are direct fixes kept out of Topic Backlog while still appearing in the loop?
7. Does Auto Off explain why accepted work remains in handoff?
8. Are zero metrics disabled?
9. Do empty states teach the right model without becoming a tutorial wall?
10. Does the implementation avoid adding unrelated dashboard redesign work?
11. Does Content Handoff have a default exit path so it does not duplicate
    Results?
12. Are destination badge colors/icons reused from the existing Loop card
    mapping?

## 17. Open Questions

1. Should the lifecycle label be `Topic planned`, `In plan`, or `Planned work`?

   Recommendation: use `Topic planned` because it is specific and matches the
   actual source of the `planned` lifecycle when a topic exists.

2. Should direct actions have a separate `Site fix planned` lifecycle?

   Recommendation: not yet. Keep the lifecycle shared and use route/surface
   badges for direct actions. Add a new lifecycle only if direct actions need
   materially different scheduling semantics.

3. Should Content Handoff hide items once they appear in Topic Backlog?

   Recommendation: do not keep full duplicate cards in the default view. Once a
   topic exists, collapse topic-backed handoff receipts into an
   `In topic backlog (n)` group or replace the full handoff card with a compact
   receipt link. Keep the source-opportunity connection available, but make Topic
   Backlog the owner of the full topic card.

4. Should the product keep `Recently Sent` and `Loop in motion` near each other?

   Recommendation: yes, but with different roles. `Recently Sent` is a recency
   feed. `Loop in motion` is a lifecycle filter. They should not render the same
   cards unconditionally.

## 18. Success Definition

This UX is successful when a user can look at the unipost.dev example and
understand:

- the opportunity was accepted from Analysis;
- it is currently in the content execution handoff;
- it may or may not have become a topic yet;
- if it has a topic, it is `Topic planned`;
- if Topic Backlog is empty, the work either has not been planned into a topic
  yet or is a direct action that should not become a topic;
- the next click takes the user to the exact current object.
