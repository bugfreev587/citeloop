# Publish Platform-First Design

## Goal

Make Publish immediately read as the place where users choose destinations and ship approved content. The first viewport should be lighter than an operations dashboard: platform tiles are the main object, ready actions are secondary, and explanatory copy is kept short.

## Product Decision

Use the approved C2 direction: `Catalog + Action Strip`.

Publish becomes a platform-first page with two visible jobs:

- Show which destinations CiteLoop can publish or prepare content for.
- Show a small list of approved items that can be acted on now.

The page should not lead with long descriptions, readiness prose, or operational metrics. Those details move into tile details, tooltips, drawers, or Settings.

## First Viewport

The top of the page uses a simple title area:

- Title: `Publish`
- Short subtitle: `Choose a destination. Ship approved content.`
- Primary action: `Connect` or `Manage`
- Secondary action: `Schedule`

Below the title, the layout splits into:

- `Destinations`: the primary area, using platform tiles.
- `Ready now`: a narrow action strip for the most immediate approved canonical items.

On mobile, `Ready now` stacks below `Destinations`.

## Destination Tiles

Each tile must answer three things without paragraphs:

- Platform name.
- Current capability or state.
- Optional target detail.

Recommended tile states:

- `Auto publish`: enabled GitHub/Next.js canonical publishing.
- `Copy draft`: manual distribution draft, canonical-capable platform such as Dev.to or Hashnode.
- `Submit draft`: manual forum-style distribution such as Reddit.
- `Not connected`: platform is known but not available for automatic publishing.
- `Roadmap`: connector is not currently available.

Visual rules:

- Connected auto-publish destination is visually strongest.
- Manual distribution destinations are normal-weight tiles.
- Roadmap or unavailable destinations use dashed borders and muted styling.
- Unavailable tiles must not look clickable as publishing actions.

Initial platform presentation:

- GitHub/Next.js: `Auto publish`
- Dev.to: `Copy draft`
- Hashnode: `Copy draft`
- Reddit: `Submit draft`
- Medium: `Not connected`; route to manual/connector details instead of publish
- WordPress: `Roadmap`

## Ready Now Strip

`Ready now` stays compact so it does not turn the page into a dense workflow dashboard.

Each item shows:

- Draft title.
- Type and destination, such as `Canonical · GitHub/Next.js`.
- One action button: `Publish`, `Review`, or `Retry`.

Only a few items should show in the first viewport. Longer queues can live behind a `View all` link or below the fold.

## Manual Drafts

Manual distribution is represented as small counts or chips after the canonical publish path:

- `Dev.to 2`
- `Hashnode 2`
- `Reddit 1`

Manual drafts unlock only after the canonical URL is published and verified. The UI should state this only where users act on manual drafts, not in the page header.

## Settings Boundary

Connection setup remains in Settings for real configuration detail. Publish can deep-link to Settings but should not become a full connection setup form.

Publish may show a lightweight `Connect` or `Manage` entry point that routes to `/projects/:id/settings#publisher`.

## Non-Goals

- Do not implement automatic Reddit, Medium, WordPress, Webflow, Shopify, or CMS posting in this design.
- Do not make roadmap platforms look available for publishing.
- Do not add a long readiness checklist to the first viewport.
- Do not move credential entry back into Publish.

## UX Acceptance Criteria

- A new user can tell within a few seconds that the page is for publishing destinations.
- Users can distinguish auto-publish from manual distribution without reading a paragraph.
- Users can see whether anything is ready to publish now.
- Disabled or roadmap platforms are visibly unavailable.
- The first viewport remains scan-friendly and avoids dense explanatory copy.

## Testing Notes

Frontend contract tests should cover:

- Publish page renders a `Destinations` section.
- GitHub/Next.js appears as an auto-publish destination when an enabled connected publisher exists.
- Manual platforms render as manual draft destinations, not auto-publish accounts.
- Roadmap/unavailable platforms use non-active state copy.
- Publish still blocks canonical publish when no enabled connected publisher exists.
- Ready canonical items render in a compact `Ready now` area.
