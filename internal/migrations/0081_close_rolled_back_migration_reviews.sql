with rolled_back_reviews as (
  select item.id
  from migration_review_items item
  join migration_batches batch
    on batch.project_id = item.project_id
   and batch.id = item.migration_batch_id
  where batch.status = 'rolled_back'
    and item.status = 'pending'
)
update migration_review_items item
set status = 'dismissed',
    resolution_snapshot = jsonb_build_object(
      'reason', 'migration_rolled_back',
      'migration_batch_id', item.migration_batch_id
    ),
    resolved_by = 'phase5-rollback-review-cleanup',
    resolved_at = now(),
    updated_at = now()
from rolled_back_reviews review
where item.id = review.id;
