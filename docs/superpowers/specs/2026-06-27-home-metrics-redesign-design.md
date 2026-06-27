# Home Metrics Redesign Design

## Goal

The project Home page should open on the most important growth metrics instead of a narrative hero or a context-refresh prompt.

## Requirements

- Remove the top hero copy, the top manual Refresh button, and the "Your next step / Refresh context" card from the normal Home page.
- Promote Metrics into the first viewport, with one larger Organic traffic card and supporting metric cards for AI citations, Published pages, and In motion.
- Every metric card must link to the page where the user can inspect or act on that metric.
- Every metric card must show a current value and an honest change/status value. If there is no historical data source, the card should use real available state such as this-month counts, connected state, or active queue counts rather than inventing a trend.
- Show other projects connected to the current account below Metrics only when there is more than one project.
- Keep Pipeline and Activity below the project switcher area.

## Architecture

The work stays inside the existing project Home client component, `web/app/projects/[id]/workspace.tsx`, because that file already owns the Home data fetch and page-level derived state. The component will add `api.listProjects()` to the refresh batch so it can render other projects without a new API route. Existing pipeline and activity logic remains in place.

## Data Flow

`Workspace.refresh()` will load the current project, profile, inventory, topics, review groups, articles, runs, SEO overview, SEO opportunities, SEO actions, and all account projects. Derived metric cards will be built from current local state: clicks/impressions from `seoOverview.last_28_days`, published count from published canonical articles this month, AI citation signal count from AI-related opportunities, and active work from plan/review/publish/measurement counts.

## UX Details

The first section becomes an asymmetric metric grid. Organic traffic is the large primary card. Supporting cards include labels, current values, change/status text, and a small "View" affordance via the whole-card link. The other-projects section uses compact linked rows/cards and is omitted when `projects.filter(project.id !== projectId)` is empty. Pipeline and Activity retain their existing headings and behavior.

## Testing

Update the dashboard contract test to assert that Home no longer contains the removed top hero/next-step elements, includes linked metric cards with hrefs, includes change/status text for each metric, fetches account projects, conditionally renders other projects, and keeps Pipeline and Activity after the metric/project area. Then run the focused contract test, full web tests, typecheck, build, and browser visual verification.
