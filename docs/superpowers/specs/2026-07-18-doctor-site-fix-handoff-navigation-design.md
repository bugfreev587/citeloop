# Doctor Site Fix Handoff and Persistent Card Selection Design

## Problem

Adding a Doctor finding to Site Fixes can leave the finding visible in Doctor and can navigate the user away from the review queue. Cross-surface handoff links also behave inconsistently: some target cards pulse briefly, while others automatically open a detail drawer.

The Doctor visibility bug has a concrete client-side cause. `DoctorClient` loads both the complete Site Fix list and the narrower Doctor recent-link list, but it passes the recent-link list to `activeDoctorFindings`. A Site Fix with a created pull request is intentionally absent from Recent Findings, so its source finding incorrectly returns to the active Doctor queue even though a canonical Site Fix already exists.

## Goals

- A Doctor finding with any canonical Site Fix is not active in Doctor.
- Creating a Site Fix updates Doctor immediately without navigating away.
- Workflow mutations keep the user on the current review surface.
- Cross-surface navigation occurs from explicit handoff/history cards such as Recently Decided, Recently Drafted, Recently Reviewed, Recently Published, Recently Watched, and Recent Findings.
- A handoff destination scrolls to, focuses, and persistently highlights the target card without opening its drawer.
- The highlight remains until the user opens a target/peer card or leaves the page.
- Direct card clicks continue to open the normal detail drawer.

## Non-goals

- Changing Site Fix lifecycle persistence or arbitration.
- Changing which records qualify for the existing Recently drawers.
- Removing explicit navigation links such as provenance or Results links.
- Reworking card layouts or drawer contents.
- Adding a global client-side navigation framework.

## Chosen Approach

Keep identifier resolution local to each destination surface while enforcing one shared interaction contract. Each surface already owns different data-loading and alias rules: Site Fixes resolves canonical and legacy IDs, Results can require an asynchronous measurement detail, and content workflow pages resolve article or action IDs. A generic hook would need surface-specific escape hatches and would make the current behavior harder to audit.

Each destination will therefore maintain a distinct handoff-highlight ID separate from its drawer-selected ID:

- URL/deep-link resolution sets only the handoff-highlight ID.
- The target is scrolled into view and receives keyboard focus.
- No timer clears the handoff highlight.
- No deep-link effect sets the drawer-selected ID.
- A direct card click clears the handoff highlight and follows the existing drawer behavior.

The visual treatment will reuse the existing linked-card red border/ring language without an indefinitely repeating pulse animation. The target also receives an accessible current/selected indication where the underlying element supports it.

## Doctor Data Flow

`DoctorClient.refresh` continues to load three independent resources:

1. the Doctor report and actionable findings;
2. all canonical Doctor Site Fixes;
3. current Doctor recent links.

The complete Site Fix list is the exclusion source for active findings. The narrower recent-link list remains the source for Recent Findings because it carries the intended dismiss/PR exit behavior.

On successful `createDoctorSiteFix`:

1. merge the returned Site Fix into the complete Site Fix state;
2. merge it into the recent-link state so a newly created eligible link appears immediately;
3. close the finding drawer;
4. show the success notification;
5. remain on Doctor.

This is correct when the API returns either a newly created fix or an existing canonical fix. If the existing fix already has a pull request, `recentDoctorFindingLinks` will omit it, while `activeDoctorFindings` will still exclude its source finding.

If creation fails, the finding remains selected and visible, no list state is changed, and the existing error notification explains that the handoff did not complete.

## Cross-surface Handoff Contract

The following destination behavior is required:

| Destination | Handoff identifier | Target behavior |
| --- | --- | --- |
| Content Plan | content action ID | Focus and persistently highlight the accepted action card |
| Review | article ID | Focus and persistently highlight the review queue card; do not open review details |
| Publish | article ID | Focus and persistently highlight the Ready to post card; do not open a drawer |
| Site Fixes | canonical Site Fix ID or legacy action alias | Resolve the canonical card, focus it, and persistently highlight it; do not open Site Fix details |
| Results content | action ID or published article ID | Resolve the Results action card, focus it, and persistently highlight it; do not open measurement details |
| Results Site Fix | measurement ID | Preserve existing async pin/resolution behavior, then focus and persistently highlight the Site Fix result card without loading/opening its detail drawer |
| Results watchlist | opportunity ID | Focus and persistently highlight the watchlist card |
| Doctor | active finding ID | Focus and persistently highlight an active finding card without opening Finding Details |

Recently Decided Site Fix links must carry the action ID in the `fix` query parameter. The Site Fix page's existing canonical/legacy alias resolver then maps that action to the canonical Site Fix card.

Handoff query parameters may remain in the URL so a reload preserves the user's location and selection. Content workflow path synchronization must preserve the query string while updating `/plan`, `/review`, or `/publish`.

## Interaction Details

- Persistent handoff highlighting has no auto-clear timeout.
- Clicking the highlighted card clears the handoff-only state and opens its normal drawer.
- Clicking a peer primary card clears the old handoff highlight and opens the clicked card.
- Closing a drawer does not recreate a consumed handoff highlight.
- Keyboard users receive the same scroll, focus, and visible selection treatment.
- Reduced-motion preferences disable smooth scrolling; selection remains visible.
- A missing or stale target does not open a fallback drawer or select the wrong card. The page remains usable with its normal list state.

## Scope Audit

The implementation must update every currently nonconforming path:

- Doctor `Add to Site Fixes` automatic navigation.
- Doctor active-finding filtering wired to the recent-link list.
- Content Plan's timed linked-card highlight.
- Review article deep links that currently set the drawer selection.
- Publish's timed linked-card highlight.
- Site Fix deep links that currently set the drawer selection.
- Results action/article deep links that currently open a drawer after a delay.
- Results Site Fix deep links that currently open details after resolution.
- Results watchlist deep links that focus without persistent selected styling.
- Recently Decided Site Fix links that currently omit the target identifier.
- Doctor finding deep links that currently open Finding Details or the Recent Findings drawer.

## Testing Strategy

Tests are written before implementation and must first fail for the existing behavior.

Contract/regression tests will assert:

- Doctor active findings use the complete Site Fix list.
- Successful Doctor creation merges the result into local state and contains no route push.
- Recent Findings still derives from the narrower recent-link list.
- Recently Decided Site Fix links include a target identifier.
- Content Plan and Publish have persistent linked-card state with no highlight-clear timeout.
- Review, Site Fixes, Results, and Doctor deep-link effects never set drawer-selected state.
- Results action, article, Site Fix, and watchlist handoffs retain persistent highlighted IDs.
- Destination cards expose the selected highlight class and clear handoff state only on direct card interaction.
- Content workflow path synchronization preserves the current query string.

Verification includes the focused web tests, the complete web test suite, TypeScript type checking, a production web build, the complete Go test suite, and production browser checks after deployment.

## Acceptance Criteria

1. Add a Doctor finding to Site Fixes. The finding drawer closes, the user stays on Doctor, and the finding immediately leaves the active Findings grid.
2. Return to or reload Doctor. The finding remains absent whenever any canonical Site Fix references it, including a prior fix with a pull request.
3. The new or existing Site Fix remains visible on Site Fixes.
4. Click each supported Recently handoff card. The application navigates to the correct surface and target card.
5. The target card scrolls into view, receives focus, and remains highlighted without opening a drawer.
6. The highlight does not disappear on a timer.
7. Click the highlighted card. Its normal detail drawer opens.
8. Direct mutation actions on Doctor and the other workflow review surfaces do not navigate automatically.
