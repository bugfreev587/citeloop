# Results Attribution Drawer Design

## Goal

Make `Results > Action-level attribution` scan like a card list and move the current expanded attribution details into a right-side drawer opened by clicking a card.

## User Experience

The Results page keeps the existing outcome summary, measurement queue, and learning signal sections. The Action-level attribution section changes from fully expanded rows to compact action cards. Each card shows the outcome badge, queue badge, action status, action title, target URL, published date, measurement window, and a short outcome reason. The full attribution report opens in a right-side drawer.

The drawer reuses the Analysis Growth findings drawer behavior: right-side panel, scrim, Escape close, focus trap, focus restore, and body scroll lock. It contains the detail fields currently rendered inside each expanded row: asset, review, verification, checkpoint state, why now, SEO/GEO contribution, output type, after execution, before, after, outcome reason, attribution confidence, confounders, measurement details, target URL, and manual verification controls.

## Constraints

Use the existing `citeloop-drawer-*` CSS animations and Tailwind style conventions. Do not change API endpoints or attribution data shapes. Keep actions accessible from keyboard by using a real button/card trigger. Keep empty state and recompute behavior unchanged.

## Testing

Update the Results attribution contract test so the code must contain Results drawer state, drawer refs, a card trigger marker, and drawer detail rendering. Run the focused Node contract test first to see it fail, then make the component pass.
