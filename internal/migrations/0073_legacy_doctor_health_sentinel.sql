-- Historical Doctor runs represented a healthy report as an active finding.
-- 0048 added finding_kind with a broken default, so those sentinels became
-- actionable repairs. Preserve the original passed URLs as transparent healthy
-- coverage before resolving the legacy row.
set local lock_timeout = '5s';
set local statement_timeout = '30s';

with legacy_health as (
  select distinct on (finding.run_id)
         finding.run_id,
         coalesce(nullif(finding.normalized_urls, '[]'::jsonb), finding.affected_urls, '[]'::jsonb) as covered_urls
  from seo_doctor_findings finding
  where finding.issue_type = 'no_active_technical_blockers'
    and finding.category = 'coverage'
    and finding.severity = 'Info'
    and lower(btrim(fix_intent)) = 'no repair needed.'
    and finding.evidence->>'source' = 'technical_checks'
    and finding.finding_kind = 'broken'
  order by finding.run_id, finding.created_at desc
)
update seo_doctor_runs run
set healthy_coverage = run.healthy_coverage || jsonb_build_array(jsonb_build_object(
      'check', 'legacy_report_health',
      'checked_urls', legacy_health.covered_urls,
      'passed_urls', legacy_health.covered_urls,
      'failed_urls', '[]'::jsonb,
      'skipped_urls', '[]'::jsonb,
      'source', 'legacy_health_sentinel_backfill',
      'note', 'Preserved from a historical Doctor report that recorded no active technical blockers.'
    )),
    updated_at = now()
from legacy_health
where run.id = legacy_health.run_id
  and run.status = 'completed'
  and run.healthy_coverage = '[]'::jsonb
  and not exists (
    select 1
    from jsonb_array_elements(healthy_coverage) coverage
    where coverage->>'check' = 'legacy_report_health'
  );

-- A sentinel could briefly be sent to Site Fixes while the old UI exposed it.
-- Quarantine that impossible repair without mutating append-only evidence-merge
-- rows, and release its active work reservation.
with dependent_sentinel_fixes as (
  select fix.id, fix.project_id, fix.work_signature_id
  from site_fixes fix
  join seo_doctor_findings finding
    on finding.project_id = fix.project_id and finding.id = fix.doctor_finding_id
  where finding.issue_type = 'no_active_technical_blockers'
    and finding.category = 'coverage'
    and finding.severity = 'Info'
    and lower(btrim(finding.fix_intent)) = 'no repair needed.'
    and finding.evidence->>'source' = 'technical_checks'
)
update work_signature_registry signature
set status = 'failed_terminal', active = false, updated_at = now()
from dependent_sentinel_fixes fix
where signature.project_id = fix.project_id
  and signature.id = fix.work_signature_id
  and signature.active = true;

update site_fixes fix
set status = 'failed_terminal',
    failure_reason = coalesce(failure_reason, 'Historical Doctor health sentinel is not repairable.'),
    verification_snapshot = verification_snapshot || jsonb_build_object(
      'legacy_health_sentinel_quarantined', true,
      'legacy_health_sentinel_quarantined_at', now()
    ),
    updated_at = now()
from seo_doctor_findings finding
where finding.project_id = fix.project_id
  and finding.id = fix.doctor_finding_id
  and finding.issue_type = 'no_active_technical_blockers'
  and finding.category = 'coverage'
  and finding.severity = 'Info'
  and lower(btrim(finding.fix_intent)) = 'no repair needed.'
  and finding.evidence->>'source' = 'technical_checks'
  and fix.status in (
    'proposed','approved','preparing','ready_to_apply','applying',
    'awaiting_deploy','verifying','failed_retryable','reopened'
  );

update seo_doctor_findings finding
set finding_kind = case
      when exists (
        select 1 from site_fixes fix
        where fix.project_id = finding.project_id and fix.doctor_finding_id = finding.id
      ) or exists (
        select 1 from site_fix_evidence_merges merge
        where merge.project_id = finding.project_id and merge.doctor_finding_id = finding.id
      ) then finding.finding_kind
      else 'healthy'
    end,
    status = 'resolved',
    evidence = finding.evidence || jsonb_build_object(
      'legacy_health_reclassification_version', 'v1',
      'legacy_health_previous_finding_kind', finding.finding_kind,
      'legacy_health_previous_status', finding.status,
      'legacy_health_kind_preserved_for_dependencies', exists (
        select 1 from site_fixes fix
        where fix.project_id = finding.project_id and fix.doctor_finding_id = finding.id
      ) or exists (
        select 1 from site_fix_evidence_merges merge
        where merge.project_id = finding.project_id and merge.doctor_finding_id = finding.id
      )
    ),
    autofix_eligible = false,
    review_required = false,
    fix_intent = 'No repair needed.',
    developer_instructions = '',
    likely_files_or_surfaces = '[]'::jsonb,
    acceptance_tests = '[]'::jsonb,
    resolved_at = coalesce(resolved_at, now()),
    updated_at = now()
where finding.issue_type = 'no_active_technical_blockers'
  and finding.category = 'coverage'
  and finding.severity = 'Info'
  and lower(btrim(fix_intent)) = 'no repair needed.'
  and finding.evidence->>'source' = 'technical_checks'
  and finding.finding_kind = 'broken'
  and finding.evidence->>'legacy_health_reclassification_version' is distinct from 'v1';

reset statement_timeout;
reset lock_timeout;
