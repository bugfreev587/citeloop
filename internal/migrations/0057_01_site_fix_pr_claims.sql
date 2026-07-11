set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table site_change_applications
  add column if not exists pr_claim_token uuid,
  add column if not exists pr_claim_expires_at timestamptz,
  add column if not exists pr_claim_authority_fingerprint text;

alter table site_change_applications
  drop constraint if exists site_change_applications_pr_claim_check;

-- This branch introduces the first durable PR claims. Historical creating_pr
-- rows therefore have no legitimate lease owner. Bound the reconciliation so
-- a surprising production backlog fails closed instead of taking an unbounded
-- write lock; operators can drain/retry those rows before rerunning migration.
do $$
declare orphan_count bigint;
begin
  select count(*) into orphan_count
  from site_change_applications
  where status = 'creating_pr' and pr_claim_token is null;
  if orphan_count > 10000 then
    raise exception 'refusing to rewrite % historical creating_pr rows', orphan_count;
  end if;
end;
$$;

update site_change_applications
set status = 'needs_follow_up',
    failure_reason = coalesce(failure_reason, 'PR creation ownership must be reclaimed'),
    updated_at = now()
where status = 'creating_pr' and pr_claim_token is null;

alter table site_change_applications
  add constraint site_change_applications_pr_claim_check check (
    (status <> 'creating_pr' and pr_claim_token is null and pr_claim_expires_at is null and pr_claim_authority_fingerprint is null)
    or
    (status = 'creating_pr' and pr_claim_token is not null and pr_claim_expires_at is not null
      and length(btrim(pr_claim_authority_fingerprint)) > 0)
  ) not valid;

alter table site_change_applications
  validate constraint site_change_applications_pr_claim_check;
