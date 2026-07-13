# Draft and Review Queue Consistency Design

## Problem

Content Plan classifies a content action by only the article stored in `content_actions.draft_article_id`. For a `both` publishing strategy, that row points at the canonical article while the same topic can also have syndication variants. Once the canonical is approved but variants remain `pending_review`, the action is incorrectly returned to Accepted content work and exposes Draft Content again.

The generation endpoint separately blocks duplicate generation when any non-rejected article exists, but returns every such article as if it were currently in Review. The client therefore reports a count that can include approved, published, or distributed articles.

## Confirmed Production Case

For project `1459b054-cdc3-4d9b-9dd4-18e12458c61a`, topic `2a427911-9a4f-4aee-a5f8-148a79890f70` has:

- one canonical article in `approved`;
- three syndication variants in `pending_review`;
- a content action linked to the approved canonical.

The topic belongs in the Content Plan handoff drawer and its link must target a pending variant. It must not reappear as an accepted brief with Draft Content.

## Design

Content Plan will resolve review ownership at the topic level. A content action is in the Review handoff state when either its linked draft is `pending_review` or the Review API contains any pending article for its topic. The pending topic article ID is used for the Review deep link when the linked canonical has already advanced.

An action with a non-rejected linked draft that has advanced beyond Review is not an accepted brief. When no sibling remains pending, it leaves Content Plan and is owned by Publish or the later lifecycle surface.

The generation endpoint will keep using all non-rejected articles to prevent duplicate generation, but its response will distinguish pending Review drafts from articles that have already advanced. The client will only label and count `pending_review` rows as Review queue drafts; an advanced-only response will say that the brief has already moved beyond Review.

## Testing

- A content-plan logic unit test will reproduce an approved canonical with a pending sibling and require the sibling ID as the Review link target.
- A content-plan logic unit test will require an approved-only action to be excluded from accepted work.
- An API handler test will require an existing mixed-status topic response to expose only pending Review articles in its Review count.
- Existing web and Go suites, typecheck, and build will run before merge.

