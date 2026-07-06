# Opportunity Finding Settings And Status Plan

## Goal

Expose Opportunity Finding as a first-class workflow so users can see what ran, when it last ran, what it did, when it will run next, and how Signal Scan and AI Discovery are configured.

## Product Decisions

- `All` means both stages are enabled conceptually: deterministic Signal Scan plus AI Discovery.
- `Signal Scan` means only the existing GSC/crawl/profile analyzer stage is enabled.
- `AI Discovery` means only the AI Discovery stage is selected.
- AI Discovery automation can be `automatic`, `semi_automatic`, or `manual`.
- Deduplication remains mandatory: processed opportunities must not re-enter the decision queue.
- The Opportunities page must separate status from the decision queue so an empty queue still explains recent work.

## Implementation Steps

1. Add project config fields:
   - `opportunity_finding_source_mix`: `all | signal_scan | ai_discovery`
   - `ai_discovery_automation`: `automatic | semi_automatic | manual`
   - Defaults: `all` and `semi_automatic`
2. Add backend status/run APIs:
   - `GET /projects/{id}/seo/opportunity-finding/status`
   - `POST /projects/{id}/seo/opportunity-finding/run`
   - Status reads the latest `seo_analyzer` run and summarizes `generated_anomalies` plus `data_source_notes`.
3. Add web API types and normalizers for Opportunity Finding status.
4. Add a Settings tab named `Opportunity Finding` with mode and AI Discovery automation controls.
5. Add an Opportunities status panel above the queue:
   - Last finding timestamp
   - Duration
   - Work summary
   - Next finding timestamp
   - Manual-mode Run button
6. Verify with backend tests, frontend tests, typecheck, and production smoke test after deploy.

## Verification

- `go test ./...`
- `cd web && npm test`
- `cd web && npm run typecheck`
- Production: confirm Settings tab renders, values save/reload, Opportunities status panel renders on project `1459b054-cdc3-4d9b-9dd4-18e12458c61a`, and manual mode shows the Run button.
