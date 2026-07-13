# Content Plan Move-Back Fix Design

## Problem

An accepted Opportunity appears in Content Plan with a working “Move back to Opportunities” control, but confirming the action returns `404: action not found or no longer reversible`.

Production evidence for action `9b7b8551-30a3-4e49-b545-4a50ccfe9707` shows that it is legitimately reversible:

- action status: `ready_for_review`
- lifecycle stage: `added_to_plan`
- opportunity status: `converted`
- no `published_at`
- no topic or draft article

Executing the return query inside a rolled-back production transaction reveals the actual database error:

```text
ERROR: column "updated_at" of relation "articles" does not exist
```

The return query’s optional draft-withdrawal CTE writes `articles.updated_at`, but the `articles` table has no such column. PostgreSQL validates the CTE even when the action has no draft article, so every move-back attempt fails. The handler converts every query error into a 404, hiding the server defect as an invalid user action.

## Decision

Fix the query at the source and improve error classification:

1. Withdraw an in-progress draft by setting only the real `articles.status` column to `rejected`.
2. Keep the existing atomic transaction: action becomes `returned`, opportunity becomes `open`, and an eligible draft becomes `rejected` together.
3. Return 404 only when sqlc reports `pgx.ErrNoRows`, which means the action does not exist or is no longer reversible.
4. Return a generic 500 for any other database failure so infrastructure defects are not presented as user mistakes.

Adding an `articles.updated_at` column is rejected because it expands schema and backfill scope without serving this workflow. Removing draft withdrawal is also rejected because it could strand an approved draft in Publish.

## Data Flow

```text
Content Plan confirmation
  -> POST /seo/actions/{actionID}/return-to-opportunity
  -> MarkContentActionReturnedToOpportunity
     -> lock eligible unpublished action
     -> action.status = returned
     -> opportunity.status = open
     -> linked active article.status = rejected (when present)
  -> commit
  -> enqueue opportunity.reviewed event
  -> Content Plan removes the card; Opportunities shows the reopened item
```

## Error Behavior

- Missing, published, or otherwise irreversible action: `404 action not found or no longer reversible`.
- Database/query failure: `500 could not move action back to Opportunities`.
- Workflow-event enqueue failure remains a 500 after the database commit, matching current behavior and remaining outside this focused repair.

## Verification

- A regression test must fail against the current query because the draft-withdrawal CTE references `articles.updated_at`.
- A handler test must prove `pgx.ErrNoRows` maps to 404 and other query errors map to 500.
- After the fix, targeted tests, full Go tests, vet, build, and web tests/typecheck/build must pass.
- Production verification must move the reported action back, confirm it disappears from Content Plan, and confirm the source Opportunity is visible in Opportunities.
