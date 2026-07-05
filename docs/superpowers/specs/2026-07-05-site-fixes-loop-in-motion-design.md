# Site Fixes Loop In Motion Design

## Goal

Make approved Site Fixes visibly participate in the same Opportunity Queue -> execution -> measurement -> learning loop as content work.

## User Experience

The Analysis page keeps its current order: Opportunity Queue, Site Fixes, then Loop in motion. Site Fixes stay as the dedicated review queue for approved schema, internal link, crawler, canonical, and technical fixes.

Loop in motion should make it clear that those site-fix actions are also moving through the loop. Summary counts continue to use the shared lifecycle stages, but the published stage label should acknowledge fixes as applied work. Preview cards should show whether the work is headed to Content Plan or Site Fixes, and site-fix preview cards should use Site Fixes language instead of looking like generic content-plan items.

## Data Flow

Analysis already builds `loopActions` from `visibilitySummary.actions_in_loop` plus locally fetched `actions`. Keep that merged source. Do not introduce a separate Site Fixes lifecycle model. The same `deriveVisibilityLifecycleStage` helper remains the lifecycle source for both content actions and direct site-fix actions.

## Constraints

Do not change API response shapes or action routing. Keep the Site Fixes queue before Loop in motion. Keep the Results page link unchanged. Use the existing badge and Tailwind patterns in `seo-client.tsx`.

## Testing

Add a focused contract test that requires Loop in motion to expose destination-specific copy for Site Fixes and to label the shared published/applied lifecycle in a way that includes applied fixes. Run the focused test first and confirm it fails before changing production code.
