# Results Opportunity Trace ID Design

## Context

Recently Published cards already show a short workflow lineage ID such as `OPP-14WVNX`. The linked Content Action card in Results does not show that ID, so users must infer the relationship from the title and URL.

Results currently aggregates Content Actions. A canonical article and any generated platform variants remain one Result because they share the same Content Action and measurement schedule.

## Goals

- Show the same short opportunity lineage ID on the linked Results Content Action card.
- Repeat the ID in the Content Action attribution drawer so the relationship remains visible after opening details.
- Preserve the existing Recently Published-to-Results deep link and Results aggregation behavior.

## Non-goals

- Do not add or change API fields, database columns, or measurement records.
- Do not split platform variants into separate Results cards.
- Do not add opportunity IDs to Site Fix Result cards, which use a different attribution model.

## Design

Use the existing `workflowTraceLabelForAction(action)` helper. It derives the label from `action.opportunity_id` and falls back to `action.id` for legacy data, matching the stable workflow-lineage convention already used across the content workflow.

On each Results Content Action card, add a neutral `OPP-xxxxxx` badge immediately after the `Content Action` badge in the existing top badge row. On the Content Action attribution drawer, add the same neutral badge at the start of the existing status badge row. Both locations call the same helper directly so they cannot drift.

The ID is visible text, not tooltip-only content. Card layout remains responsive and may wrap the existing badge row on narrow screens.

## Data Flow and Aggregation

The Results API already returns `opportunity_id` on every Content Action. The client passes the action to the shared formatter and renders the returned label; no normalization or backend work is required.

Results continues to use one row per `content_action`. Canonical and platform-specific article variants remain associated artifacts under that Result. Measurements continue to be unique by project, Content Action, and checkpoint day.

## Testing

- Add a failing contract assertion proving Results imports and uses `workflowTraceLabelForAction`.
- Assert the Content Action card renders the lineage badge.
- Assert the Content Action attribution drawer renders the same lineage badge.
- Keep the existing deep-link test for `?article=` and the workflow lineage formatter tests passing.
- Run the complete Web test suite, TypeScript typecheck, and production build before merge.

## Acceptance Criteria

- A Results Content Action reached from Recently Published visibly shows the same `OPP-xxxxxx` value as its source card.
- Opening that Result shows the same value in the attribution drawer.
- Legacy actions without an opportunity ID still receive a stable fallback label.
- Site Fix cards and multi-platform aggregation behavior are unchanged.
