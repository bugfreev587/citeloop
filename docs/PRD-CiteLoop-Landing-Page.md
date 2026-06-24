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
4. Do not change Clerk sign-in or sign-up provider configuration; the implementation assumes Google OAuth is already configured in Clerk for environments where the `Join with Google` button is shown.
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
- No center navigation links for this release. Do not add `Docs`, `Product`, pricing, FAQ, or other marketing links to the landing page header.
- Signed-out right actions:
  - `Join with Google` secondary button that initiates Clerk Google OAuth directly. It must not be a plain link to the generic `/sign-in` page.
  - `Start for free` primary button, linked to `/sign-up`.
- Signed-in right actions:
  - `Dashboard` button.
  - Clerk `UserButton` avatar to the right of `Dashboard`.

Button labels must match the requested copy exactly: `Join with Google`, `Start for free`, and `Dashboard`.

Because the label says `Join with Google`, its action must match that label. The implementation should use the Clerk-supported Google OAuth redirect flow for the installed `@clerk/nextjs` version. If the exact API surface needs confirmation during implementation, confirm it before writing the production component.

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

`Join with Google` requires a client component because it triggers Clerk's browser OAuth flow. The component must use Clerk's Google OAuth redirect API rather than forwarding users to the generic hosted sign-in UI.

## 8. Dashboard Routing Behavior

### 8.1 Recent Project Storage

Use browser-local storage because cross-device and cross-browser sync is not required.

Recommended localStorage key:

```text
citeloop:last-project-id
```

When a user visits a project home route or any route under `/projects/[id]`, store that project id in localStorage.

The storage write must happen only in a client component and only inside `useEffect` or another browser-only event path. Server components, render-time module scope, and hydration-sensitive paths must not access `window` or `localStorage`.

If a user deletes the project currently stored in `citeloop:last-project-id` from this browser, the delete success path should clear the key or replace it with another valid project id when doing so is straightforward. This cleanup is a polish requirement; Dashboard must still validate the stored id on every click so stale storage cannot break routing.

### 8.2 Dashboard Button Resolution

The landing page should server-prefetch the signed-in user's project list with the existing API and pass that list into the client `Dashboard` button as initial data. This avoids an extra network round trip on click and matches the current server-component pattern already used by the existing home page.

When signed-in users click `Dashboard`:

1. Use the server-prefetched project list passed into the button.
2. If there are no projects, route to `/projects`.
3. If localStorage contains `citeloop:last-project-id` and that id exists in the loaded project list, route to `/projects/{id}`.
4. If the stored id is missing, invalid, or no longer belongs to the user, route to the first project in the loaded list.
5. If the server prefetch failed, route to `/projects` so the existing project management page can show its API error state.

The first project fallback can use the existing API ordering. The current backend returns owner projects in descending `created_at` order, so this fallback is "most recently created" rather than "most recently visited." That is acceptable only when no valid local recent project exists.

### 8.3 Loading State

The `Dashboard` button should have a brief disabled or busy state after click while reading browser-local state and pushing the resolved route. It must not trigger duplicate navigation if clicked repeatedly.

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

Rendered landing page copy must not include the banned phrases above. Add a contract assertion or review checklist item for this requirement so it is not enforced only by memory.

## 11. Accessibility Requirements

1. All buttons and links must have accessible text labels.
2. The Clerk avatar must remain keyboard accessible through Clerk's `UserButton`.
3. Color contrast must pass WCAG AA for body text and primary buttons.
4. The Dashboard button loading state must preserve a readable label.
5. The page must preserve logical heading order.

## 12. Implementation Notes

Suggested file boundaries:

- `web/app/page.tsx`: server component that determines signed-in state, fetches projects when signed in, and renders the landing page.
- `web/app/landing-google-button.tsx` or `web/app/landing-auth-actions.tsx`: client component that initiates Clerk Google OAuth for `Join with Google`.
- `web/app/landing-dashboard-button.tsx`: client component that receives server-prefetched projects, reads localStorage in a browser-only path, and handles Dashboard navigation.
- `web/app/project-visit-recorder.tsx` or a small client leaf inside `ProjectShell`: records the current project id when a project route is visited, with all localStorage writes inside `useEffect`.
- `web/app/lib/dashboard-routing.ts`: pure helpers for selecting the Dashboard destination from projects and a stored id.
- `web/app/lib/landing-page-contract.test.mjs`: contract tests for requested copy, signed-in/signed-out controls, helper behavior, and localStorage key usage.

The pure routing helper should be tested first. It should accept a project list and a stored id, then return `/projects` or `/projects/{id}`.

No backend schema, SQL, Go API, or generated DB code should be modified for this feature.

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
3. `Join with Google` is implemented through a Clerk Google OAuth client action, not as a plain link to `/sign-in`.
4. The Dashboard button receives server-prefetched projects rather than fetching the list on click in the normal path.
5. The routing helper returns `/projects` when the project list is empty.
6. The routing helper returns the stored project when it exists in the project list.
7. The routing helper falls back to the first project when the stored id is invalid.
8. The Dashboard component falls back to `/projects` when the server project prefetch failed.
9. The project shell or visit recorder writes `citeloop:last-project-id` only from a client effect/browser path.
10. Rendered landing page copy does not include the banned marketing phrases from Section 10.

Manual verification:

1. Signed-out `/` shows landing page and the two requested buttons.
2. Signed-out `Join with Google` starts the Google OAuth flow rather than landing on the generic sign-in screen.
3. Signed-in `/` shows `Dashboard` and avatar.
4. Signed-in with no projects: `Dashboard` routes to `/projects`.
5. Signed-in with projects and no localStorage key: `Dashboard` routes to a valid project home.
6. Signed-in after visiting `/projects/{id}`: `Dashboard` routes back to that project home.
7. Mobile width does not overlap nav buttons, hero copy, or preview panel.

## 14. Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| Stored project id becomes stale after project deletion | Validate against loaded project list before routing. |
| Stored project id remains after deleting the current project | Clear or replace the key from the delete success path when straightforward, and still validate on Dashboard click. |
| API fails while prefetching Dashboard projects | Route to `/projects`, where the existing page can show the API error state. |
| `Join with Google` label does not match behavior | Implement direct Clerk Google OAuth rather than a plain `/sign-in` link. |
| Landing page becomes too marketing-heavy | Keep only hero, product preview, and one compact supporting section. |
| Local development breaks when Clerk is not configured | Preserve existing `clerkServerAuthConfigured` and bypass behavior. |
| Product preview drifts from real app language | Reuse existing labels such as Context, Review, Publish, Visibility, and Next action. |

## 15. Acceptance Criteria

1. `/` is a concise landing page, not a project management page.
2. The header has no center `Docs`, `Product`, pricing, FAQ, or other marketing links.
3. Signed-out users see `Join with Google` and `Start for free` in the top-right navigation.
4. `Join with Google` starts Clerk Google OAuth directly.
5. Signed-in users see `Dashboard` and the profile avatar in the top-right navigation.
6. `Dashboard` uses the server-prefetched project list in the normal path.
7. `Dashboard` routes signed-in users with no projects to `/projects`.
8. `Dashboard` routes signed-in users with projects to the most recently visited project home in the current browser when available.
9. `Dashboard` falls back to a valid project home when no valid recent project exists.
10. Visiting a project route updates the current browser's recent project id from a client effect/browser path.
11. The implementation requires no backend schema or API changes.
12. Contract tests, typecheck, and manual responsive checks pass before release.
