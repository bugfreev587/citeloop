# PRD: CiteLoop Landing Page

## 1. Summary

CiteLoop needs a focused public landing page at `/` that introduces the product before asking users to enter the app. The current home page behaves like a logged-in project management surface; this PRD replaces that mixed experience with a compact SaaS landing page inspired by the structure of [ChatSEO](https://chatseo.app/) while preserving CiteLoop's own product language and dashboard workflows.

The page should make one promise: CiteLoop connects to a site's SEO and AI-search signals, identifies the next content opportunities, and moves those opportunities through review, publishing, and measurement. The experience should feel lightweight, useful, and operational rather than like a long marketing site.

## 2. Goals

1. Present CiteLoop as a clear SEO and GEO content operating system for new visitors.
2. Keep the first viewport simple: brand, auth actions, strong headline, short value proposition, and product preview.
3. Show the correct right-nav actions by authentication state.
4. Give signed-in users a `Dashboard` entry point that routes to the right app destination.
5. Avoid adding backend persistence for "recently visited project" because cross-device and cross-browser sync is not required.
6. Preserve the existing Clerk and project management flows.

## 3. Non-Goals

1. Do not build a long-form marketing site with pricing, testimonials, FAQ, or footer-heavy SEO pages.
2. Do not add backend tables, API endpoints, or project metadata for visit recency.
3. Do not redesign the logged-in project workspace.
4. Do not change Clerk sign-in or sign-up provider configuration.
5. Do not create a new onboarding wizard.

## 4. Audience

Primary users are founders, marketers, and SEO operators who want to stop guessing what to publish next. They need to understand that CiteLoop is connected to real site data and produces prioritized actions, not generic AI suggestions.

Secondary users are returning signed-in users who land on `/` and need a quick path back to the app.

## 5. User Stories

1. As a signed-out visitor, I can understand what CiteLoop does in the first viewport.
2. As a signed-out visitor, I can join with Google or start for free from the top-right navigation.
3. As a signed-in user with no projects, I can click `Dashboard` and land on `/projects` to create or manage projects.
4. As a signed-in user with projects, I can click `Dashboard` and land on the most recently visited project home page in this browser.
5. As a signed-in user with projects but no recent project stored in this browser, I can click `Dashboard` and land on a valid project home page.
6. As a signed-in user whose stored recent project was deleted or no longer belongs to me, I can click `Dashboard` and fall back to a valid destination.

## 6. Page Structure

### 6.1 Top Navigation

The top navigation should be minimal and stable:

- Left: `CiteLoop` brand mark and wordmark.
- Optional center links: keep to at most two links, such as `Docs` and `Product`, only if they do not crowd mobile.
- Signed-out right actions:
  - `Join with Google` secondary button, linked to `/sign-in`.
  - `Start for free` primary button, linked to `/sign-up`.
- Signed-in right actions:
  - `Dashboard` button.
  - Clerk `UserButton` avatar to the right of `Dashboard`.

Button labels must match the requested copy exactly: `Join with Google`, `Start for free`, and `Dashboard`.

### 6.2 Hero

Use the selected Option A direction:

- Left-aligned content.
- Warm off-white page background.
- Compact label: `SEO + GEO AUTOPILOT`.
- Headline: `The content engine that already knows your site.`
- Supporting copy should explain that CiteLoop connects site data, finds opportunities, and moves work through review, publishing, and measurement.
- Primary CTA for signed-out visitors: `Start for free`.
- Secondary CTA for signed-out visitors: `Join with Google`.
- Signed-in visitors may still see product messaging, but the primary action should be `Dashboard`.

### 6.3 Product Preview

The hero should include a compact product preview panel, not a decorative illustration. It should show a believable CiteLoop workflow:

- Connected site or Search Console context.
- Prioritized opportunity or next action.
- Draft/review stage.
- Publishing or measurement status.

The preview can be static markup. It should not require live API data. It should look like a lightweight version of the actual dashboard, using the existing CiteLoop accent color and operational language.

### 6.4 Supporting Section

Below the first viewport, include a short three-step explanation:

1. Connect your site data.
2. Prioritize the next SEO and AI-search wins.
3. Review, publish, and measure shipped content.

This section should remain compact enough that the page still feels concise.

## 7. Authentication Behavior

The page must use existing Clerk configuration patterns:

- If Clerk is configured and the user has no server token, treat the visitor as signed out.
- If Clerk is configured and the user has a token, treat the visitor as signed in.
- If Clerk is not configured in allowed non-production contexts, preserve the existing bypass behavior instead of breaking local development.
- Production must continue to fail closed when Clerk is not configured.

Signed-out navigation must not render the Clerk avatar. Signed-in navigation must render the avatar to the right of `Dashboard`.

## 8. Dashboard Routing Behavior

### 8.1 Recent Project Storage

Use browser-local storage because cross-device and cross-browser sync is not required.

Recommended localStorage key:

```text
citeloop:last-project-id
```

When a user visits a project home route or any route under `/projects/[id]`, store that project id in localStorage.

### 8.2 Dashboard Button Resolution

When signed-in users click `Dashboard`:

1. Load the signed-in user's project list from the existing API.
2. If there are no projects, route to `/projects`.
3. If localStorage contains `citeloop:last-project-id` and that id exists in the loaded project list, route to `/projects/{id}`.
4. If the stored id is missing, invalid, or no longer belongs to the user, route to the first project in the loaded list.
5. If loading projects fails, route to `/projects` so the existing project management page can show its API error state.

The first project fallback can use the existing API ordering. The current backend returns owner projects in descending `created_at` order, so this fallback is "most recently created" rather than "most recently visited." That is acceptable only when no valid local recent project exists.

### 8.3 Loading State

The `Dashboard` button should have a brief disabled or busy state while resolving the destination. It must not trigger duplicate navigation if clicked repeatedly.

## 9. Visual Design Requirements

1. Match the selected Option A layout: focused hero plus product preview.
2. Use the existing CiteLoop accent color `#d93820` sparingly for status and CTA emphasis.
3. Avoid generic AI-purple gradients, oversized hero type, and decorative backgrounds.
4. Avoid card-heavy marketing layout. Use cards only for the product preview or small workflow steps.
5. Maintain responsive behavior:
   - Desktop: two-column hero with content and preview.
   - Mobile: single-column hero with nav buttons wrapping cleanly.
6. Use `min-h-[100dvh]` for viewport-safe sections.
7. Keep text readable and avoid overlap at mobile widths.

## 10. Content Requirements

Draft copy:

- Nav brand: `CiteLoop`
- Hero label: `SEO + GEO AUTOPILOT`
- Hero headline: `The content engine that already knows your site.`
- Hero body: `Connect your site data, find the pages worth creating or improving, and move each opportunity through review, publishing, and measurement.`
- Primary signed-out CTA: `Start for free`
- Secondary signed-out CTA: `Join with Google`
- Signed-in CTA: `Dashboard`
- Supporting section headline: `From signal to shipped content`

Avoid vague marketing phrases such as "elevate," "unlock," "next-gen," or "seamless."

## 11. Accessibility Requirements

1. All buttons and links must have accessible text labels.
2. The Clerk avatar must remain keyboard accessible through Clerk's `UserButton`.
3. Color contrast must pass WCAG AA for body text and primary buttons.
4. The Dashboard button loading state must preserve a readable label.
5. The page must preserve logical heading order.

## 12. Implementation Notes

Suggested file boundaries:

- `web/app/page.tsx`: server component that determines signed-in state, fetches projects when signed in, and renders the landing page.
- `web/app/landing-dashboard-button.tsx`: client component for localStorage lookup and Dashboard navigation.
- `web/app/project-visit-recorder.tsx` or a small client leaf inside `ProjectShell`: records the current project id when a project route is visited.
- `web/app/lib/dashboard-routing.ts`: pure helpers for selecting the Dashboard destination from projects and a stored id.
- `web/app/lib/landing-page-contract.test.mjs`: contract tests for requested copy, signed-in/signed-out controls, helper behavior, and localStorage key usage.

The pure routing helper should be tested first. It should accept a project list and a stored id, then return `/projects` or `/projects/{id}`.

## 13. Testing Requirements

Use the existing web test runner:

```bash
cd web
npm test
npm run typecheck
```

Minimum contract coverage:

1. Home page contains `Join with Google`, `Start for free`, and `Dashboard`.
2. Home page no longer renders the project management client directly.
3. The routing helper returns `/projects` when the project list is empty.
4. The routing helper returns the stored project when it exists in the project list.
5. The routing helper falls back to the first project when the stored id is invalid.
6. The project shell or visit recorder writes `citeloop:last-project-id`.

Manual verification:

1. Signed-out `/` shows landing page and the two requested buttons.
2. Signed-in `/` shows `Dashboard` and avatar.
3. Signed-in with no projects: `Dashboard` routes to `/projects`.
4. Signed-in with projects and no localStorage key: `Dashboard` routes to a valid project home.
5. Signed-in after visiting `/projects/{id}`: `Dashboard` routes back to that project home.
6. Mobile width does not overlap nav buttons, hero copy, or preview panel.

## 14. Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| Stored project id becomes stale after project deletion | Validate against loaded project list before routing. |
| API fails while resolving Dashboard | Route to `/projects`, where the existing page can show the API error state. |
| Landing page becomes too marketing-heavy | Keep only hero, product preview, and one compact supporting section. |
| Local development breaks when Clerk is not configured | Preserve existing `clerkServerAuthConfigured` and bypass behavior. |
| Product preview drifts from real app language | Reuse existing labels such as Context, Review, Publish, Visibility, and Next action. |

## 15. Acceptance Criteria

1. `/` is a concise landing page, not a project management page.
2. Signed-out users see `Join with Google` and `Start for free` in the top-right navigation.
3. Signed-in users see `Dashboard` and the profile avatar in the top-right navigation.
4. `Dashboard` routes signed-in users with no projects to `/projects`.
5. `Dashboard` routes signed-in users with projects to the most recently visited project home in the current browser when available.
6. `Dashboard` falls back to a valid project home when no valid recent project exists.
7. Visiting a project route updates the current browser's recent project id.
8. The implementation requires no backend schema or API changes.
9. Contract tests, typecheck, and manual responsive checks pass before release.
