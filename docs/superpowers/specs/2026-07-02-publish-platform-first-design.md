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
- Primary action is state-driven; see `Header CTA State Model`.
- Secondary action is `Schedule` when a project has an enabled publisher or scheduled items.

Below the title, the layout splits into:

- `Destinations`: the primary area, using platform tiles.
- `Ready now`: a narrow action strip for the most immediate approved canonical items and publish retries.

On desktop, `Destinations` owns the larger area and `Ready now` is a narrow action strip. On mobile, keep the mental model platform-first without burying the user's next action:

1. Header.
2. Primary GitHub/Next.js destination tile.
3. `Ready now` compact strip.
4. Manual syndication destination tiles.
5. Roadmap or `More` destinations.

## Header CTA State Model

The header action should tell users what to do next. It should not be a generic `Manage` button when a clearer action exists.

| State | Primary CTA | Target |
| --- | --- | --- |
| No GitHub/Next.js connection | `Connect GitHub` | `/projects/:id/settings#publisher` |
| Connected but disabled | `Enable publishing` | Settings publisher section or enable action if already loaded |
| Connection `error` or `revoked` | `Fix connection` | `/projects/:id/settings#publisher` |
| Publisher enabled and ready canonical items exist | `Publish next` | First ready canonical item or `Ready now` list |
| Publisher enabled and no ready items exist | `Manage destinations` | `/projects/:id/settings#publisher` |
| Only scheduled items exist | `View schedule` | Operational drawer filtered to `Scheduled` |

`Schedule` remains a secondary control. It opens the existing cadence/mode controls and must not be confused with destination capability.

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

## Tile Interaction Model

Every destination tile has a predictable click target. Tiles are not direct publish buttons unless the visible action says so.

| Tile type | Click behavior | Primary actions inside detail |
| --- | --- | --- |
| GitHub/Next.js connected | Opens publisher status drawer | `Publish next`, `View schedule`, `Manage in Settings` |
| GitHub/Next.js not connected or disabled | Opens setup/status drawer | `Connect GitHub` or `Enable publishing` |
| GitHub/Next.js error/revoked | Opens recovery drawer | `Fix connection`, `Retry test` |
| Manual syndication platform | Opens platform draft drawer | `Copy`, `Open compose`, `Mark distributed` |
| More manual platforms | Opens manual platform list | Platform-specific draft counts and details |
| CMS roadmap tile | Opens roadmap explanation | No publish action; optional `Learn more` |

This keeps the first viewport simple while giving every visual object an obvious next step.

## Ready Now Strip

`Ready now` stays compact so it does not turn the page into a dense workflow dashboard.

Each item shows:

- Draft title.
- Type and destination, such as `Canonical / GitHub/Next.js`.
- One primary action: `Publish` or `Retry`.
- Optional secondary action: `Preview`.

Do not use `Review` as the primary action here. Items in Publish are already past the review gate; `Preview` is clearer for a final pre-publish look.

Only a few items should show in the first viewport. Longer queues can live behind `View all` or below the fold.

If there are no ready canonical items, `Ready now` shows a compact empty state:

- `No approved posts ready`
- Secondary text: `Approved canonical posts appear here after review.`
- Optional link: `Go to Review` when pending review items exist.

## Manual Syndication Drafts

Manual syndication is represented as small counts or chips after the canonical publish path:

- `Dev.to 2`
- `Hashnode 2`
- `Reddit 1`

Counts should be driven by actual unlocked `syndication_variant` rows. Manual syndication drafts unlock only after the canonical URL is published and verified. Approved variants still waiting on canonical verification should not appear as ready manual drafts.

For Reddit and Hacker News, the visible wording should lean toward `Submit draft` or `Pending submit` rather than implying a saved draft exists on the external platform.

Manual syndication chips are interactive. Clicking a chip opens that platform's draft drawer with row-level actions:

- `Copy`: copies the prepared variant.
- `Open`: opens the platform compose URL when known.
- `Mark distributed`: records that the user posted externally.

The drawer can also show waiting variants under a separate `Waiting on canonical` group, but those rows must not expose `Copy` or `Mark distributed`.

## Scheduled, Published, And Failed States

The redesign must not recreate the current six-lane first viewport.

- `Failed`: retryable canonical failures can appear in `Ready now` with `Retry`.
- `Scheduled`: appears in a unified `View all` operational drawer under `Scheduled`.
- `Published`: appears in the same operational drawer under `Published`, with deeper outcome details staying in Results.
- `Waiting on canonical`: appears in manual syndication details, not as a top-level first-viewport lane.
- `Ready to distribute`: appears as manual syndication chips/counts and in a distribution detail list.

This keeps the first viewport focused while preserving access to operational states.

The `View all` drawer is the single home for non-first-viewport publishing state. It groups items as:

- `Ready`
- `Scheduled`
- `Published`
- `Failed`
- `Waiting on canonical`
- `Ready to distribute`

The drawer can reuse existing lane semantics internally, but the first viewport must not render those groups as six visible columns.

## Page States

The page needs explicit loading, empty, and error states so the simplified UI does not become ambiguous.

| State | UX treatment | Primary action |
| --- | --- | --- |
| Connections loading | Skeleton destination tiles and compact `Ready now` skeleton rows | None |
| No publisher connection | GitHub/Next.js tile in `Not connected`; `Ready now` publish actions disabled | `Connect GitHub` |
| Publisher disabled | GitHub/Next.js tile in `Disabled`; ready items visible but publish disabled | `Enable publishing` |
| Publisher error/revoked | GitHub/Next.js tile in `Needs attention`; show concise inline warning | `Fix connection` |
| No ready canonical items | Compact `Ready now` empty state | `Go to Review` only if pending review exists |
| No unlocked syndication drafts | Manual chips hidden or show zero-state inside manual drawer | None |
| Publish action in flight | Button-level busy state and disabled duplicate actions | None |
| Publish failure | Item appears in `Ready now` with `Retry` and short failure reason | `Retry` |

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
- Users can click every destination tile and understand what happened.
- Users can access scheduled, published, failed, waiting, and ready-to-distribute items through one `View all` drawer.
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
- Header CTA state maps to connection/readiness state.
- Destination tiles expose the expected interaction target for connected, missing, disabled, error, manual, and roadmap states.
- Scheduled, published, failed, waiting, and ready-to-distribute items are available through the operational drawer model outside the first-viewport `Ready now` strip.
- Mobile ordering prioritizes the primary GitHub/Next.js destination and `Ready now` before secondary/manual roadmap tiles.
