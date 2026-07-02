# Publish Platform-First Design

## Goal

Make Publish immediately read as the place where users choose destinations and ship approved content. The first viewport should be lighter than an operations dashboard: destination tiles are the main object, ready actions are secondary, and explanatory copy is kept short.

## Product Decision

Use the approved C2 direction: `Catalog + Action Strip`.

Publish becomes a platform-first page with two visible jobs:

- Show which destinations CiteLoop can publish to or prepare syndication drafts for.
- Show a small list of approved canonical items that can be acted on now.

The page should not lead with long descriptions, readiness prose, or operational metrics. Those details move into tile details, tooltips, drawers, or Settings.

## Domain Terms

The UI can use short labels, but the spec and implementation must stay mapped to the existing domain model.

| UI phrase | Code/domain meaning |
| --- | --- |
| Canonical item | `articles.kind === "canonical"` |
| Syndication draft | `articles.kind === "syndication_variant"` |
| Manual draft | A syndication variant surfaced through the semi-manual distribution lane |
| Copy draft | UI label for a canonical-capable syndication platform; same semi-manual flow |
| Submit draft | UI label for forum-style syndication such as Reddit or Hacker News; same semi-manual flow |
| Mark distributed | Existing `distributed` / `markDistributed` action after the user posts externally |

`Copy draft` and `Submit draft` do not require separate backend publishing paths in this design. They are visual labels over the same semi-manual compose/copy and mark-distributed flow.

## First Viewport

The top of the page uses a simple title area:

- Title: `Publish`
- Short subtitle: `Choose a destination. Ship approved content.`
- Primary action: `Connect` or `Manage`
- Secondary action: `Schedule`

Below the title, the layout splits into:

- `Destinations`: the primary area, using platform tiles.
- `Ready now`: a narrow action strip for the most immediate approved canonical items and publish retries.

On mobile, `Ready now` stacks below `Destinations`.

## Destination Matrix

The destination model must align with `internal/platform/platform.go` and the Settings CMS roadmap copy.

| Destination | Source of truth | First viewport treatment | Tile state |
| --- | --- | --- | --- |
| GitHub/Next.js | `publisher_connections.kind === "github_nextjs"` | Always show | Connection-driven canonical destination |
| Dev.to | syndication enum; V1 default target | Show by default | `Copy draft` |
| Hashnode | syndication enum; V1 default target | Show by default | `Copy draft` |
| Reddit | syndication enum; V1 default target | Show by default | `Submit draft` |
| Medium | syndication enum; not V1 default target | Show only in More/manual details unless a project has Medium variants | `Copy draft` |
| LinkedIn | syndication enum; not V1 default target | Show only in More/manual details unless a project has LinkedIn variants | `Copy draft` |
| Hacker News | syndication enum; not V1 default target | Show only in More/manual details unless a project has Hacker News variants | `Submit draft` |
| WordPress | Settings roadmap connector | Roadmap area or grouped CMS roadmap tile | `Roadmap` |
| Webflow | Settings roadmap connector | Roadmap area or grouped CMS roadmap tile | `Roadmap` |
| Shopify | Settings roadmap connector | Roadmap area or grouped CMS roadmap tile | `Roadmap` |
| Custom CMS | Settings roadmap connector | Roadmap area or grouped CMS roadmap tile | `Roadmap` |

The first viewport should stay concise. It may show GitHub/Next.js plus the V1 default syndication targets and one compact `More` or `CMS roadmap` tile instead of showing every possible platform tile.

Medium is not a canonical connector in this design. If shown, it must be treated as a supported manual syndication destination, not as a not-connected publisher account.

## Tile State Rules

Each tile must answer three things without paragraphs:

- Platform name.
- Current capability or state.
- Optional target detail.

GitHub/Next.js states are driven by publisher connection data:

- `Auto publish`: a default `github_nextjs` connection is `enabled === true` and `status === "connected"`.
- `Not connected`: no GitHub/Next.js publisher connection exists, or the connection is `missing`.
- `Disabled`: a GitHub/Next.js connection exists and is connected, but `enabled === false`.
- `Needs attention`: connection status is `error` or `revoked`.

`Auto publish` is a destination capability label. It does not mean the project's publish cadence mode is `auto`. The existing project mode control (`manual`, `scheduled`, `auto`) remains separate and should continue to determine whether approved canonical items publish automatically, on schedule, or only by manual action.

Syndication states are driven by the platform registry and article variants:

- Canonical-capable syndication platforms use `Copy draft`.
- Forum/aggregator platforms that do not support canonical use `Submit draft`.
- A syndication platform with no current variants can be hidden behind `More` to keep the first viewport clean.
- Ready syndication counts come from unlocked variants, not from raw platform support.

Roadmap states are driven by the existing Settings roadmap list:

- WordPress, Webflow, Shopify, and Custom CMS are roadmap CMS connectors.
- Roadmap tiles use dashed borders and muted styling.
- Roadmap tiles must not look like publish actions.

## Ready Now Strip

`Ready now` stays compact so it does not turn the page into a dense workflow dashboard.

Each item shows:

- Draft title.
- Type and destination, such as `Canonical / GitHub/Next.js`.
- One primary action: `Publish` or `Retry`.
- Optional secondary action: `Preview`.

Do not use `Review` as the primary action here. Items in Publish are already past the review gate; `Preview` is clearer for a final pre-publish look.

Only a few items should show in the first viewport. Longer queues can live behind `View all` or below the fold.

## Manual Syndication Drafts

Manual syndication is represented as small counts or chips after the canonical publish path:

- `Dev.to 2`
- `Hashnode 2`
- `Reddit 1`

Counts should be driven by actual unlocked `syndication_variant` rows. Manual syndication drafts unlock only after the canonical URL is published and verified. Approved variants still waiting on canonical verification should not appear as ready manual drafts.

For Reddit and Hacker News, the visible wording should lean toward `Submit draft` or `Pending submit` rather than implying a saved draft exists on the external platform.

## Scheduled, Published, And Failed States

The redesign must not recreate the current six-lane first viewport.

- `Failed`: retryable canonical failures can appear in `Ready now` with `Retry`.
- `Scheduled`: appears in a below-fold queue, a `View all` drawer, or destination details.
- `Published`: appears in below-fold history, destination details, or results/measurement surfaces.
- `Waiting on canonical`: appears in manual syndication details, not as a top-level first-viewport lane.
- `Ready to distribute`: appears as manual syndication chips/counts and in a distribution detail list.

This keeps the first viewport focused while preserving access to operational states.

## Settings Boundary

Connection setup remains in Settings for real configuration detail. Publish can deep-link to Settings but should not become a full connection setup form.

Publish may show a lightweight `Connect` or `Manage` entry point that routes to `/projects/:id/settings#publisher`.

## Non-Goals

- Do not implement automatic Reddit, Medium, LinkedIn, Hacker News, WordPress, Webflow, Shopify, or Custom CMS posting in this design.
- Do not make roadmap platforms look available for publishing.
- Do not add a long readiness checklist to the first viewport.
- Do not move credential entry back into Publish.
- Do not reintroduce the six-lane operations layout in the first viewport.

## UX Acceptance Criteria

- A new user can tell within a few seconds that the page is for publishing destinations.
- Users can distinguish canonical publishing from manual syndication without reading a paragraph.
- Users can distinguish destination capability from project publish cadence mode.
- Users can see whether anything is ready to publish or retry now.
- Disabled or roadmap platforms are visibly unavailable.
- The first viewport remains scan-friendly and avoids dense explanatory copy.

## Implementation Shape For Testing

This repo's frontend contract tests use Node `node:test` with pure logic modules, not DOM rendering tests. The Publish redesign should expose a pure logic module, for example `web/app/lib/publish-destinations-logic.ts`, and have the component render from that output.

Suggested pure functions:

- `buildPublishDestinations(input)` returns destination tiles with platform, source, state, label, target detail, and action availability.
- `buildReadyNow(input)` returns compact canonical publish/retry items.
- `buildManualSyndicationSummary(input)` returns unlocked manual syndication counts and waiting counts.

Frontend contract tests should assert:

- Enabled connected GitHub/Next.js maps to `Auto publish`.
- Missing GitHub/Next.js maps to `Not connected`.
- Connected but disabled GitHub/Next.js maps to `Disabled`.
- Error or revoked GitHub/Next.js maps to `Needs attention`.
- `Auto publish` tile state does not override project publish cadence mode.
- Dev.to and Hashnode map to manual syndication `Copy draft`, not auto-publish.
- Reddit and Hacker News map to manual syndication `Submit draft`.
- Medium and LinkedIn are supported manual syndication platforms but not V1 default first-viewport tiles unless variants exist.
- WordPress, Webflow, Shopify, and Custom CMS map to non-active roadmap states.
- Canonical publish action is unavailable when no enabled connected publisher exists.
- Manual syndication variants appear in ready counts only after the canonical article is published and `canonical_url_verified_at` is set.
- Scheduled and published items are available outside the first-viewport `Ready now` strip.
