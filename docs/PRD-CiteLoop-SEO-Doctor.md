# PRD: CiteLoop SEO Doctor

> Date: 2026-07-02
> Status: product PRD
> Owner: Product
> Depends on:
> - `docs/PRD-CiteLoop-SEO-Operations-Loop.md`
> - `docs/PRD-CiteLoop-Visibility-Analysis-to-Content-Loop.md`
> - `docs/PRD-CiteLoop-Multi-Surface-SEO-Growth-Layer.md`

## 1. Product Thesis

CiteLoop should not wait for Search Console data before telling a user whether
their site is technically healthy. The first user action is entering a URL, so
the first product output should be a concrete site diagnosis: what is healthy,
what is broken, why it matters, and exactly what an AI coding tool or developer
should change.

SEO Doctor is the cold-start and recurring health layer for CiteLoop. It runs
once during initial URL onboarding, then automatically every week. The user can
also run it manually at any time from a dedicated Doctor page. Its output is not
just a visual audit; every issue must have structured evidence, priority, repair
guidance, and acceptance tests that can be handed to an AI coding tool.

The user-facing loop is:

```text
URL -> Doctor Run -> Health Report -> Fixable Issues -> Action Queue -> Verification
```

## 2. Why Now

The current product already has crawl, SEO sync, technical checks, visibility
summary, opportunities, action portfolio, and Results. The remaining gap is that
users do not see a full "site health exam" immediately after entering a URL, and
technical SEO work is mixed into broader opportunities instead of being a clear
diagnostic report.

The expected product response is direct:

- broken URL variants should not return fake `200 OK` pages;
- `www`, `http`, trailing slash, and locale variants should resolve to one
  canonical format;
- temporary redirects should not be used for permanent canonical routing;
- invalid JSON-LD, broken social images, missing canonical tags, and duplicated
  URL versions should be called out directly;
- every finding should include repair instructions that a coding agent can use.

SEO Doctor makes this expectation a first-class product surface.

## 3. Goals

1. Run an SEO Doctor scan automatically when a user first creates a project from
   a URL.
2. Add a dedicated Doctor page at `/projects/{projectID}/doctor`.
3. Add a Home entry point that takes the user into the Doctor page.
4. Let users manually start a Doctor run at any time.
5. Run Doctor automatically once per week for every active project.
6. Show a progress indicator while a Doctor run is queued or running.
7. Produce a complete report covering original site pages and CiteLoop-created or
   newly changed pages.
8. Convert fixable findings into SEO opportunities and technical/content actions.
9. Produce an AI-coding-tool-friendly report with evidence, repair steps, and
   acceptance tests.
10. Preserve the existing Visibility/Analysis/Results loop: Doctor findings must
    feed the action system instead of becoming a parallel workflow.

## 4. Non-Goals

- Do not promise ranking, traffic, conversion, or AI answer visibility gains.
- Do not use private GSC/GA4 metrics when the project is `public_only`.
- Do not bypass Google, CMS, or hosting authorization boundaries.
- Do not automatically edit customer-owned code, redirects, `robots.txt`,
  canonical rules, schema, or sitemap unless a publisher integration and policy
  explicitly allow that low-risk action.
- Do not build a generic enterprise crawler in V1. The scan must stay bounded and
  respectful of robots, rate limits, page caps, and timeout caps.
- Do not make PageSpeed/Core Web Vitals, Rich Results Test API integration, or
  third-party SEO data mandatory for V1.

## 5. Official Fact Boundaries

SEO Doctor must phrase findings according to stable public SEO rules and avoid
overstating causality:

- Google Search Essentials define technical requirements, spam policies, and key
  best practices for appearing in Google Search. Meeting them does not guarantee
  crawl, index, or ranking.
  Source: https://developers.google.com/search/docs/essentials
- Sitemaps help search engines discover important URLs and improve crawling for
  larger, newer, or media-heavy sites, but they do not guarantee crawling or
  indexing.
  Source: https://developers.google.com/search/docs/crawling-indexing/sitemaps/overview
- `robots.txt` manages crawler access and crawl load. It is not a reliable way to
  keep a web page out of Google Search; `noindex`, password protection, or
  removal are the appropriate controls for that goal.
  Source: https://developers.google.com/search/docs/crawling-indexing/robots/intro
- Canonicalization can use `rel="canonical"`, HTTP canonical headers, sitemap
  inclusion, and redirects. Sitemap canonicals are weaker than explicit canonical
  mappings, and HTTPS is preferred unless there are conflicting signals.
  Source: https://developers.google.com/search/docs/crawling-indexing/consolidate-duplicate-urls
- Permanent redirects tell Google to show the target URL in search results;
  temporary redirects keep the source page as the search result candidate.
  Source: https://developers.google.com/search/docs/crawling-indexing/301-redirects
- Structured data should be representative of visible page content, not
  misleading, and JSON-LD is the recommended format.
  Source: https://developers.google.com/search/docs/appearance/structured-data/sd-policies
- Search Console URL Inspection API shows the Google-indexed version status and
  does not perform a live URL test.
  Source: https://developers.google.com/webmaster-tools/v1/urlInspection.index/inspect

## 6. Codebase Baseline and Reuse Contract

SEO Doctor must be a user-facing diagnosis and repair layer on top of existing
CiteLoop infrastructure, not a second crawler or second SEO operations system.

Existing infrastructure to reuse:

- `internal/crawl/` for bounded fetching, robots handling, sitemap discovery, URL
  normalization, and article/page discovery.
- `technical_checks` for page-level raw observations: HTTP status, canonical,
  robots, title, meta description, H1, structured data, sitemap status, internal
  link count, outbound link count, content hash, unsafe MDX, raw details, and
  check time.
- `seo_runs` for broad SEO audit logging where a Doctor run also needs to be
  correlated with existing sync/analyze/brief activity.
- `seo_opportunities` and `content_actions` for action handoff. Doctor findings
  must not create a separate execution queue.
- Scheduler patterns from `TickGEO` and `TickContextRefresh` for weekly project
  sweeps and "skip when recently completed" behavior.

New Doctor-owned objects are limited to product-layer needs that existing tables
do not model cleanly:

- user-visible run progress;
- report history;
- grouped findings;
- new/persistent/resolved comparison across runs;
- AI-coding-tool repair contracts;
- links back to opportunities and actions.

### 6.1 Reuse vs New Collection

Most V1 checks are report grouping on top of existing page observations. Doctor
should call or share the same low-level page check code used for
`technical_checks`, then group raw observations into findings with severity,
repair instructions, and acceptance tests.

Doctor must not duplicate requests made by an immediately adjacent SEO sync. If a
fresh technical observation exists for the same normalized URL and compatible
scope, Doctor may reuse it instead of refetching the page. "Fresh" means checked
within the last 24 hours for manual/onboarding runs and within the same weekly
run window for scheduled runs, unless the run was explicitly forced.

New collection logic is required only for checks that cannot be represented by a
single existing page observation:

- URL variant probes for `http`/`https`, `www`/non-`www`, trailing slash, and
  locale-format variants.
- Soft 404 probes using intentionally missing URLs.
- Redirect chain and redirect loop tracing with hop details.
- Social preview image fetches for `og:image` and similar image references.
- JSON-LD parse validation, unresolved-template detection, and structured-data
  reference checks.

Phase 1 implementation must split these into two sub-milestones:

1. Reuse existing crawl/check observations and produce Doctor findings/report.
2. Add the new active probes above, each with focused tests and crawler budget
   accounting.

## 7. Users and Jobs

### 7.1 New SaaS User

The user enters a product URL and expects CiteLoop to explain whether the site is
ready for SEO/GEO operations. The user should not need to understand Search
Console, GA4, sitemap syntax, redirect status codes, schema validation, or
canonicalization before receiving useful feedback.

### 7.2 Internal Growth Operator

The operator wants a weekly list of the most important technical blockers across
owned and managed content surfaces. The operator needs a short report for review
and structured tasks that can be passed to an AI coding agent or developer.

### 7.3 Developer or AI Coding Tool

The developer or AI coding tool needs deterministic instructions: reproduce the
problem, locate the likely implementation surface, apply a fix, and run
acceptance checks.

## 8. User Experience

### 8.1 Onboarding

During project creation, the current setup flow should change from a generic
visibility baseline to a visible SEO Doctor step:

1. Create control center.
2. Read domain context.
3. Run SEO Doctor.
4. Open Home with the Doctor summary surfaced.

If the first Doctor run is still active when the project opens, Home shows a
compact progress module and a link to the Doctor page.

### 8.2 Home Entry

Home must contain a clear entry point under the Home context. The entry should be
a first-fold module, not buried in Settings or Analysis.

Required Home states:

- `never_run`: prompt to run the first Doctor scan.
- `running`: show current progress and "View Doctor" link.
- `healthy`: show health score, last run time, and next scheduled run.
- `needs_attention`: show P0/P1 issue count and top issue.
- `blocked`: show the blocker, such as robots disallowed, fetch timeout, or
  missing site URL.

The dedicated page route is:

```text
/projects/{projectID}/doctor
```

Navigation placement:

- Add Doctor as a Home-scoped product surface.
- Home page must show a Doctor module in the first fold.
- Sidebar navigation must add a `Doctor` item in the same primary section as
  `Home`, directly under the `Home` item.
- It must not replace Analysis. Analysis remains the review queue for accepted
  opportunities and action creation.

### 8.3 Doctor Page

The Doctor page is the user-facing report and run control surface.

Required sections:

1. Header: health score, last run time, next weekly run, capability mode.
2. Run controls: `Run Doctor` button, disabled while a run is active.
3. Progress indicator: visible while queued/running.
4. Priority findings: P0/P1/P2 issue list with evidence and affected URLs.
5. AI fix report: copyable/downloadable JSON or Markdown for AI coding tools.
6. Changed since last run: newly found, resolved, still open.
7. Coverage: pages scanned, pages skipped, sitemap count, generated URLs scanned.
8. Action handoff: findings converted into opportunities/actions.
9. History: previous Doctor runs with status, duration, health score, and issue
   counts.

### 8.4 Progress Indicator

Doctor run progress must not be a spinner alone. It should expose stages and
percentage where possible.

Stages:

| Stage | Start | End | Description |
|---|---:|---:|---|
| `queued` | 0 | 0 | Run request accepted, waiting for worker. |
| `discovering` | 1 | 10 | Resolve canonical host, robots, sitemap, URL variants. |
| `crawling` | 10 | 35 | Fetch bounded page set and generated URL set. |
| `checking` | 35 | 78 | Evaluate HTTP, redirect, canonical, robots, metadata, schema, links, sitemap, social previews. |
| `classifying` | 78 | 90 | Group raw checks into prioritized findings. |
| `writing_report` | 90 | 96 | Create human report and AI-fixable report. |
| `handoff` | 96 | 99 | Upsert opportunities and action candidates. |
| `completed` | 100 | 100 | Report ready. |

`progress_percent` is not just the stage start value. Within `crawling` and
`checking`, it must interpolate using `pages_fetched / pages_discovered` and
`pages_checked / pages_discovered` where those denominators are known:

```text
progress_percent = stage_start + floor(stage_span * stage_unit_progress)
```

If the denominator is unknown, progress can advance to the stage midpoint and
then hold until the next stage starts. The UI must show page counts beside the
bar so a long checking stage still feels alive.

Each progress response should include:

```json
{
  "run_id": "uuid",
  "status": "running",
  "stage": "checking",
  "progress_percent": 60,
  "message": "Checking canonical, redirects, schema, and social previews",
  "started_at": "2026-07-02T12:00:00Z",
  "updated_at": "2026-07-02T12:01:04Z",
  "pages_discovered": 42,
  "pages_checked": 19,
  "issues_found": 6
}
```

Frontend polling cadence:

- Poll every 2 seconds while `queued` or `running`.
- Poll every 5 seconds after 60 seconds of runtime.
- Stop polling when status is `completed`, `failed`, or `cancelled`.

## 9. Scan Scope

Doctor V1 must scan a bounded but useful page set:

1. Project site URL and canonical homepage.
2. HTTP/HTTPS and www/non-www variants for homepage and important paths.
3. `robots.txt`.
4. Sitemap URLs from robots and conventional sitemap locations.
5. Sitemap URL samples, capped by project crawl config.
6. Existing `content_inventory` URLs.
7. Published CiteLoop canonical article URLs.
8. Newly published or recently changed CiteLoop-generated URLs.
9. Important URLs from GSC page rows when GSC is connected.

The report must label coverage explicitly:

- `scanned`
- `skipped_by_robots`
- `skipped_out_of_scope`
- `skipped_due_to_cap`
- `fetch_failed`
- `requires_gsc_connection`

## 10. V1 Checks

### 10.1 URL and Canonical Health

Checks:

- canonical homepage resolves to one preferred scheme and host;
- `http` variant redirects to `https` when HTTPS is available;
- `www` and non-`www` do not both serve separate `200` versions;
- trailing slash and non-trailing slash variants do not both serve duplicate
  content;
- locale variants use one canonical format;
- canonical tag exists on indexable HTML pages;
- canonical tag target is absolute or resolvable;
- canonical target does not point to a broken, redirected-loop, or non-indexable
  URL;
- sitemap URL and page canonical URL agree.

Initial issue types:

- `url_variant_duplicate`
- `temporary_canonical_redirect`
- `canonical_missing`
- `canonical_target_invalid`
- `sitemap_canonical_mismatch`

### 10.2 HTTP, Redirect, and Soft 404

Checks:

- important pages return `2xx`;
- intentionally missing pages return real `404` or `410`;
- fake paths do not return homepage with `200 OK`;
- permanent canonical host/path moves use `301` or `308`;
- redirect chains stay below configured maximum;
- redirect loops are detected.

Initial issue types:

- `soft_404`
- `http_error`
- `redirect_chain`
- `redirect_loop`
- `temporary_redirect_for_permanent_url`

Soft 404 V1 algorithm:

1. Generate two same-origin missing paths using a per-run random token:
   `/citeloop-doctor-missing-{token}` and
   `/citeloop-doctor-missing-{token}/nested`.
2. Fetch the canonical homepage and both missing paths with the same user agent,
   redirect policy, timeout, and body limit.
3. A high-confidence `soft_404` requires both missing probes to return `2xx`,
   HTML content, no `noindex`, and either:
   - final URL equals the canonical homepage URL; or
   - stripped-body token similarity to the homepage is at least `0.85`.
4. A medium-confidence soft 404 candidate requires one missing probe to return
   `2xx` with similarity at least `0.75`, or both probes to return `2xx` with
   similarity between `0.65` and `0.85`.
5. If the page visibly communicates a not-found state in title, H1, canonical
   content, or robots meta, downgrade to P2 or Info unless the response is also
   canonicalized to the homepage.
6. P0 severity is allowed only for high-confidence soft 404 on canonical host
   variants or generated-content paths. Medium-confidence cases default to P1 or
   P2 with lower confidence.

Similarity can be implemented with normalized token sets, shingles, or SimHash.
The implementation must store the chosen method and threshold in finding
evidence.

### 10.3 Crawl and Index Controls

Checks:

- `robots.txt` fetch state;
- important pages are not disallowed when they are expected to rank;
- indexable pages do not contain `noindex`;
- pages intended to be hidden use `noindex`, auth, or removal rather than
  relying on robots alone;
- canonical and noindex do not conflict on important pages.

Initial issue types:

- `robots_blocks_important_page`
- `unexpected_noindex`
- `robots_used_as_index_control`
- `canonical_noindex_conflict`

### 10.4 Metadata and Social Preview

Checks:

- title exists and is not duplicated across many pages;
- meta description exists for important pages;
- exactly one primary H1 exists for main content pages;
- `og:title`, `og:description`, and `og:image` exist for shareable pages;
- social image URL returns an image content type and successful status;
- broken relative social image paths are reported.

Initial issue types:

- `title_missing`
- `title_duplicate`
- `meta_description_missing`
- `h1_missing`
- `h1_multiple`
- `social_preview_image_broken`

### 10.5 Structured Data

Checks:

- JSON-LD blocks parse as JSON;
- JSON-LD does not contain unresolved template code;
- schema type is relevant to the page type when detectable;
- structured data does not point to broken image or URL references;
- schema is present on pages where CiteLoop generated an article, FAQ, how-to,
  product, organization, breadcrumb, or software-app page and the page template
  supports it.

Initial issue types:

- `structured_data_missing`
- `structured_data_invalid_json`
- `structured_data_template_leak`
- `structured_data_reference_broken`
- `structured_data_type_mismatch`

### 10.6 Sitemap and Internal Links

Checks:

- sitemap exists or absence is explicitly acceptable for small, well-linked
  sites;
- sitemap contains important canonical pages;
- sitemap does not contain `404`, redirected, noindexed, or non-canonical URLs;
- important pages have internal links;
- generated pages are linked from at least one indexable internal page or
  expected generated hub.

Initial issue types:

- `sitemap_missing`
- `sitemap_has_dead_url`
- `sitemap_has_noncanonical_url`
- `important_page_missing_from_sitemap`
- `internal_link_gap`
- `orphan_generated_page`

### 10.7 GSC-Connected Enhancements

When Search Console is connected, Doctor may include:

- indexed state from URL Inspection API for sampled important URLs;
- pages in sitemap with no impressions after enough time;
- pages with GSC activity that are missing from current sitemap or inventory;
- GSC-discovered URL variants not seen in public crawl.

These checks must be labeled as GSC-backed and must not run or display fake
metrics in `public_only` mode.

## 11. Issue Severity and Health Score

Severity must be deterministic and explainable.

| Severity | Meaning | Examples |
|---|---|---|
| P0 | Likely blocks crawl, indexing, canonical consolidation, or report trust for important pages. | Soft 404, important page noindex, redirect loop, canonical target broken, robots blocks important generated pages. |
| P1 | Likely weakens discovery, parsing, click capture, or sharing for important pages. | Temporary canonical redirect, missing canonical, invalid schema, sitemap dead URL, broken `og:image`. |
| P2 | Hygiene or opportunity issue with lower immediate risk. | Missing meta description, low internal link count, optional schema missing. |
| Info | Non-blocking observation. | Sitemap not needed for a small, fully linked site. |

Priority score formula:

```text
priority_score =
  severity_weight +
  affected_url_weight +
  important_page_weight +
  generated_content_weight +
  freshness_weight +
  confidence_weight
```

The score is used for ordering only. It must not claim impact magnitude.

### 11.1 Health Score

`health_score` is a product health indicator, not an SEO outcome forecast. It
answers "how cleanly can CiteLoop crawl, understand, and verify this site right
now?"

V1 score formula:

```text
raw_deduction =
  sum(P0 findings * 20 * importance_multiplier * confidence_multiplier) +
  sum(P1 findings * 8  * importance_multiplier * confidence_multiplier) +
  sum(P2 findings * 2  * importance_multiplier * confidence_multiplier)

health_score = max(0, round(100 - min(raw_deduction, 100)))
```

Multipliers:

- `importance_multiplier = 1.25` for homepage, pricing, docs-critical, legal, or
  top generated-content hub pages.
- `importance_multiplier = 1.15` for CiteLoop-generated or recently changed
  URLs.
- `importance_multiplier = 1.0` for ordinary pages.
- `confidence_multiplier = 1.0` for confidence `>= 80`.
- `confidence_multiplier = 0.75` for confidence `60-79`.
- `confidence_multiplier = 0.5` for confidence `< 60`.

Confidence mapping:

- `high` confidence maps to numeric confidence `90`.
- `medium` confidence maps to numeric confidence `70`.
- `low` confidence maps to numeric confidence `50`.
- Direct deterministic checks from a successfully fetched page default to `90`
  unless the check uses an inferred heuristic.
- Heuristic checks such as soft 404 similarity, duplicate-title grouping, or
  page-importance inference must write both the label and numeric confidence into
  finding evidence.

Score caps:

- Any active P0 caps health score at `69`.
- Any active P1 caps health score at `84`.
- A failed run with no usable latest completed report has no health score and
  renders as `blocked`.

Home/report display states:

| Display State | Derivation |
|---|---|
| `never_run` | No active run and no completed Doctor run exists. |
| `running` | Any run has status `queued` or `running`. |
| `blocked` | Latest run failed with `block_reason`, and there is no usable completed report newer than the failure. |
| `healthy` | Latest completed run has score `>= 90` and no active P0/P1 findings. |
| `needs_attention` | Latest completed run has score `< 90` or any active P0/P1 finding. |

`blocked` is a presentation state derived from run status and `block_reason`; it
is not a separate run status.

## 12. Report Contracts

### 12.1 Human Report

The human report includes:

```json
{
  "health_score": 58,
  "status": "needs_attention",
  "summary": "6 actionable issues found across 24 checked URLs; 4 info observations excluded from issue total",
  "issue_counts": {
    "P0": 1,
    "P1": 3,
    "P2": 2,
    "Info": 4
  },
  "top_findings": [],
  "coverage": {},
  "changed_since_last_run": {},
  "data_source_notes": []
}
```

`summary` issue totals count actionable P0/P1/P2 findings. `Info` rows are
reported in `issue_counts` and the detail view, but excluded from the actionable
issue total unless the summary explicitly says "observations".

### 12.2 AI Coding Tool Report

Every finding must have a machine-friendly repair contract:

```json
{
  "id": "temporary-canonical-redirect-www-en",
  "severity": "P1",
  "category": "redirect",
  "issue_type": "temporary_canonical_redirect",
  "affected_urls": ["http://www.example.com/en"],
  "evidence": {
    "observed_status": 302,
    "expected_status": "301 or 308",
    "final_url": "https://example.com/en"
  },
  "why_it_matters": "Temporary redirects keep the source URL eligible as the search result candidate.",
  "fix_intent": "Make the canonical host redirect permanent.",
  "developer_instructions": [
    "Find redirect rules for host and scheme variants.",
    "Change the temporary redirect for this canonical route to a permanent redirect.",
    "Ensure the canonical tag and sitemap use the same final URL."
  ],
  "repo_context_available": false,
  "likely_files_or_surfaces": [
    "hosting redirect rules",
    "framework redirect configuration",
    "CMS redirect settings",
    "edge middleware"
  ],
  "acceptance_tests": [
    "curl -I http://www.example.com/en returns 301 or 308.",
    "Location is https://example.com/en.",
    "https://example.com/en returns 200.",
    "The page canonical tag matches https://example.com/en."
  ],
  "risk_level": "medium",
  "review_required": true,
  "autofix_eligible": false
}
```

Rules:

- The AI report must be downloadable or copyable as JSON.
- Markdown export may be added, but JSON is the source of truth.
- Instructions must be concrete enough for a coding agent but must not invent
  repository-specific file paths unless CiteLoop has publisher/repo context. If
  repo context is unavailable, `likely_files_or_surfaces` must use generic
  surfaces such as "hosting redirect rules" or "CMS template settings". If repo
  context is available, findings may add a separate `repo_hints` array with
  concrete paths and confidence.
- Each issue must include acceptance tests.
- Each issue must include evidence collected by Doctor, not LLM-only reasoning.

## 13. Data Model

### 13.1 Recommended Tables

`seo_doctor_runs`

- `id`
- `project_id`
- `trigger`: `onboarding`, `manual`, `weekly`, `post_publish`
- `status`: `queued`, `running`, `completed`, `failed`, `cancelled`
- `stage`
- `progress_percent`
- `message`
- `block_reason`
- `pages_discovered`
- `pages_fetched`
- `pages_checked`
- `issues_found`
- `started_at`
- `updated_at`
- `finished_at`
- `health_score`
- `input_snapshot`
- `output_summary`
- `error`
- `created_by_user_id`

V1 accepts storing progress and final summary fields in `seo_doctor_runs` to keep
the contract simple. To limit write churn, workers should persist progress only
when the stage changes, the integer percentage changes, or at least 2 seconds
have elapsed since the previous progress write. If Doctor volume grows, progress
can later move to a separate append-only table without changing the public API.

`seo_doctor_findings`

- `id`
- `project_id`
- `run_id`
- `finding_key`
- `severity`
- `category`
- `issue_type`
- `status`: `open`, `converted`, `dismissed`, `resolved`, `stale`
- `affected_urls`
- `normalized_urls`
- `evidence`
- `why_it_matters`
- `fix_intent`
- `developer_instructions`
- `likely_files_or_surfaces`
- `acceptance_tests`
- `risk_level`
- `review_required`
- `autofix_eligible`
- `linked_opportunity_id`
- `linked_content_action_id`
- `first_seen_at`
- `last_seen_at`
- `resolved_at`

`seo_doctor_findings.finding_key` must be stable across runs:

```text
project_id + issue_type + normalized_primary_url + normalized_evidence_target
```

The normalization used for `finding_key` must reuse the same URL normalization
path used by crawl/check logic. Doctor is specifically looking for
scheme/host/trailing-slash/canonical problems, so the key must preserve the
variant that is evidence for the issue while normalizing irrelevant fragments,
tracking query parameters, default ports, and percent-encoding consistently.

This allows weekly reports to show new, persistent, and resolved findings. A
repeated unfixed issue must be marked persistent on the second run rather than
appearing as a new issue.

### 13.2 Relationship to Existing Tables

- `seo_runs` remains the broad SEO sync/analyze/brief audit log.
- `technical_checks` continues to store page-level raw technical observations.
- `seo_doctor_runs` stores user-facing progress and final report metadata.
- `seo_doctor_findings` stores grouped findings with AI repair instructions.
- Selected findings are upserted into `seo_opportunities`.
- Accepted findings create `content_actions` when they can be handled through the
  existing action loop.

Do not model Doctor as only another `seo_opportunities` query. Users need report
history, progress, coverage, and resolved-versus-new comparisons that do not fit
the opportunity table cleanly.

## 14. APIs

Required endpoints:

```text
GET  /api/projects/{projectID}/seo/doctor
POST /api/projects/{projectID}/seo/doctor/runs
GET  /api/projects/{projectID}/seo/doctor/runs/{runID}
GET  /api/projects/{projectID}/seo/doctor/runs/{runID}/findings
GET  /api/projects/{projectID}/seo/doctor/latest
POST /api/projects/{projectID}/seo/doctor/findings/{findingID}/convert
POST /api/projects/{projectID}/seo/doctor/findings/{findingID}/dismiss
```

`POST /runs` body:

```json
{
  "trigger": "manual",
  "scope": "default",
  "force": false
}
```

Behavior:

- If a run is active, `POST /runs` returns the active run instead of starting a
  duplicate.
- An active run is any run for the same project with status `queued` or
  `running`, regardless of trigger.
- Manual runs are rate-limited to 3 started runs per project per hour. Returning
  an already-active run does not count against this rate limit.
- If onboarding auto-starts a run and the user clicks `Run Doctor`, the manual
  request returns the onboarding run.
- If a weekly run is active and the user clicks `Run Doctor`, the request returns
  the weekly run.
- If a manual run completes successfully, the weekly scheduler treats it as a
  fresh Doctor run and skips automatic weekly Doctor for the next 6 days.
- The endpoint should return immediately with run status; scanning happens in a
  background worker or workflow.
- Latest report endpoint returns the most recent completed run plus active run
  progress if one exists.

## 15. Scheduler and Triggers

Triggers:

| Trigger | When | Scope |
|---|---|---|
| `onboarding` | After project creation from URL. | Default bounded public scan. |
| `manual` | User clicks `Run Doctor`. | Default bounded scan unless user chooses a narrower future scope. |
| `weekly` | Once per week per active project. | Default bounded scan plus changed/new generated URLs. |
| `post_publish` | After CiteLoop publishes or verifies a URL. | New/changed URL plus dependent sitemap/canonical checks. |

V1 required:

- `onboarding`
- `manual`
- `weekly`

V1 optional:

- `post_publish`

Weekly scheduling should reuse the scheduler infrastructure and avoid starting a
Doctor run if any onboarding, manual, weekly, or post-publish Doctor run
completed successfully in the last 6 days. Manual runs are still allowed during
that window, subject to active-run dedupe and rate limits.

## 16. Action Handoff

Doctor findings should feed the existing action system:

| Finding Category | Opportunity Type | Default Action |
|---|---|---|
| Redirect/canonical blocker | `technical_visibility_issue` | Create technical task |
| Structured data invalid/missing | `schema_gap` | Create schema patch task |
| Metadata missing | `metadata_ctr_opportunity` or `technical_visibility_issue` | Create metadata rewrite |
| Sitemap issue | `sitemap_gap` | Create sitemap update task |
| Internal link gap | `internal_link_gap` | Create internal link patch |
| Social preview broken | `technical_visibility_issue` | Create technical task |
| Soft 404 | `technical_visibility_issue` | Create technical task |

Current migrations do not constrain `seo_opportunities.type` with a database
enum, but implementation must still preserve existing analyzer and UI routing
for `technical_visibility_issue`, `schema_gap`, `internal_link_gap`, and GSC
metric opportunity types. If `sitemap_gap` or metadata-specific routing is not
yet implemented when Doctor starts, Doctor must either add the routing in the
same phase or map the finding to `technical_visibility_issue` with explicit
evidence.

Conversion rules:

- P0 findings should appear as high-priority opportunities by default.
- P1 findings should appear as opportunities unless they are duplicate children
  of a P0 root cause.
- P2 findings remain in Doctor unless they are selected by the user or needed for
  generated content verification.
- Findings converted to opportunities retain `doctor_run_id` and `finding_id` in
  evidence snapshots.

## 17. Permissions and Safety

Doctor can always run in `public_only` mode against public URLs within crawl
bounds. More powerful checks require permissions:

| Capability | Required State |
|---|---|
| Public crawl, redirects, metadata, schema, sitemap fetch | `public_only` |
| GSC index status and private search signals | `search_read` |
| Repo-aware likely file paths | publisher/repo connection |
| Auto-generated patch/diff | publisher write connection and policy |
| Auto-apply low-risk fix | guarded automation policy plus publisher write |

High-risk actions always require review:

- deleting pages;
- noindexing;
- redirect/canonical rule changes;
- robots changes;
- homepage/pricing/legal/docs-critical changes;
- large content rewrites;
- product claim changes.

## 18. Error Handling

Run statuses:

- `queued`
- `running`
- `completed`
- `failed`
- `cancelled`

Failure modes:

- invalid site URL;
- DNS failure;
- TLS failure;
- robots disallowed;
- crawl cap reached;
- sitemap too large;
- repeated timeout;
- response body too large;
- unsupported content type;
- internal worker failure.

The UI must distinguish partial success from total failure:

- A run with some skipped pages can still be `completed` with warnings.
- A run that cannot fetch the canonical homepage is `failed`.
- A run blocked by robots should be `completed` or `failed` based on whether any
  allowed pages were checked, but the report must clearly identify the blocker.
- `block_reason` stores the machine-readable root cause for failed or degraded
  runs, such as `homepage_fetch_failed`, `robots_disallowed_all`,
  `missing_site_url`, `dns_failure`, or `tls_failure`.
- Home's `blocked` state is derived from `status=failed` plus `block_reason`
  when no usable completed report is newer than that failed run.

## 19. Measurement and Results

Doctor findings are not impact reports. Doctor verifies whether a technical state
changed.

For each converted action, Results should later show:

- before Doctor evidence;
- after verification evidence;
- whether the finding is resolved;
- whether related search/traffic metrics are unavailable, too early, or measured.

Do not claim a Doctor fix caused rankings to improve unless the Results
attribution layer has enough evidence and still labels confidence and
confounders.

For findings converted to actions, post-publish or post-apply verification should
run a targeted Doctor check against the affected URL and directly related
sitemap/canonical references. This targeted check can mark the finding resolved
before the next weekly full Doctor run.

## 20. UI Requirements

### 20.1 Doctor Page First Fold

At desktop 1440x900:

- health score, P0/P1 count, last run, next run, and `Run Doctor` control visible
  without scrolling;
- active progress visible without scrolling;
- first priority issue title row visible within the first viewport.

At mobile 390x844:

- health score and active progress visible in the first viewport;
- issue list begins within 1.4 viewport heights, with at least the first issue
  title row visible.

### 20.2 Progress UI

Progress must show:

- stage label;
- progress bar;
- checked pages / discovered pages where available;
- issue count so far;
- elapsed time;
- a link to run history once complete.

### 20.3 Issue UI

Each issue row must show:

- severity;
- category;
- issue title;
- affected URL count;
- top evidence;
- recommended action;
- convert/dismiss controls;
- whether it is AI-fixable and whether review is required.

Issue detail drawer must show:

- full evidence;
- affected URLs;
- AI report JSON;
- acceptance tests;
- source notes;
- linked opportunity/action if converted.

## 21. Phased Delivery

### Phase 0: PRD and Contract Tests

Scope:

- Add this PRD.
- Add contract tests for route/API names, progress states, and AI report fields.
- No runtime behavior changes required beyond tests if this phase is split.

Exit criteria:

1. PRD exists in `docs/PRD-CiteLoop-SEO-Doctor.md`.
2. Contract tests assert the required progress stages.
3. Contract tests assert AI report findings include evidence, instructions, and
   acceptance tests.
4. Contract tests assert health score thresholds and display-state derivation.
5. Contract tests assert soft 404 high-confidence and medium-confidence
   classification behavior.

### Phase 1: Backend Doctor Run and Report

Scope:

- Add Doctor tables/migrations.
- Add run creation and latest report APIs.
- Add progress persistence.
- Reuse existing crawl/check observations for V1 checks already represented by
  `technical_checks`.
- Add finding grouping and stable finding keys.
- Add active-run dedupe, manual rate limit, and weekly freshness skip.

Exit criteria:

1. Manual run can be created and polled.
2. Onboarding can enqueue first Doctor run.
3. Weekly scheduler can enqueue Doctor run.
4. Report includes coverage, issue counts, findings, and AI report JSON.
5. Re-running Doctor marks persistent, new, and resolved findings.
6. Existing `technical_checks` observations can feed Doctor findings without a
   duplicate fetch.
7. Progress percent interpolates within crawl/check stages when page totals are
   known.

### Phase 1B: New Active Probes

Scope:

- Add URL variant probes.
- Add soft 404 probes and similarity evidence.
- Add redirect chain/loop tracing.
- Add social preview image checks.
- Add JSON-LD parse/template/reference validation.

Exit criteria:

1. New probes share crawl timeout, rate limit, robots, and request-budget
   controls.
2. Soft 404 only emits P0 for high-confidence cases.
3. Redirect findings include hop evidence.
4. Social image findings include fetched status and content type.
5. Structured data findings include parse or template evidence.

### Phase 2: Doctor Page and Home Entry

Scope:

- Add `/projects/{projectID}/doctor`.
- Add Home module entry and active progress state.
- Add run button, progress indicator, report view, AI report export, issue
  detail drawer, and history list.

Exit criteria:

1. Home links to Doctor under Home context.
2. Sidebar navigation shows Doctor directly under Home.
3. User can manually run Doctor.
4. Active run shows staged progress.
5. Latest report renders issue counts, coverage, and AI report.
6. UI handles `never_run`, `running`, `healthy`, `needs_attention`, and `blocked`.

### Phase 3: Action Handoff

Scope:

- Convert P0/P1 findings into `seo_opportunities`.
- Let user convert a finding into a technical/content action.
- Attach Doctor evidence to opportunity/action snapshots.

Exit criteria:

1. P0/P1 findings appear in Analysis as appropriate.
2. Converted findings link back to Doctor.
3. Accepted technical findings create direct technical actions, not unnecessary
   article topics.
4. Results can show before/after Doctor verification.

### Phase 4: GSC and Post-Publish Enhancements

Scope:

- Add GSC-backed index and sitemap checks for connected projects.
- Add post-publish targeted Doctor checks for new/changed CiteLoop URLs.
- Add notification event for Doctor P0 issues.

Exit criteria:

1. `public_only` projects still work without fake private metrics.
2. Connected projects get GSC-backed findings with source labels.
3. Newly published CiteLoop URL gets checked after publish/verify.
4. Weekly report highlights new, resolved, and repeated issues.

## 22. Acceptance Criteria

1. A new project created from URL triggers an onboarding Doctor run.
2. A dedicated Doctor page exists at `/projects/{projectID}/doctor`.
3. Home contains a clear Doctor entry point and active progress module.
4. Users can manually run Doctor any time, subject to rate limits and active-run
   dedupe.
5. Weekly scheduler runs Doctor once per active project per week.
6. Active runs show stage, progress percent, message, checked page count, and
   issue count.
7. Doctor report includes health score, issue counts, coverage, findings, and
   changed-since-last-run state.
8. Every finding includes severity, issue type, affected URLs, evidence, repair
   instructions, likely surfaces, acceptance tests, risk level, and review
   requirement.
9. The AI coding tool report is available as JSON.
10. P0/P1 findings can become opportunities or content actions.
11. Disconnected GSC/GA4 projects do not show private metrics.
12. Generated CiteLoop pages are included in scan scope.
13. Repeated runs do not duplicate open findings for the same issue.
14. Resolved findings are marked resolved instead of disappearing silently.
15. Production verification confirms onboarding run, manual run, progress polling,
   weekly scheduling configuration, and report rendering.
16. The same unresolved finding appears as persistent on the next run, not new.
17. Manual run completion resets the weekly freshness window.
18. Active-run dedupe returns the existing run for simultaneous onboarding,
   manual, or weekly requests.

## 23. V1 Decisions

These decisions are fixed for V1:

- Doctor gets its own page.
- Home is the primary entry point.
- Manual run is required.
- Weekly automatic run is required.
- Progress indicator is required and must be staged, not spinner-only.
- V1 uses public crawl and deterministic technical checks first.
- GSC-backed checks are an enhancement when permission exists.
- AI report JSON is required; Markdown is optional.
- Doctor feeds Analysis/Action/Results rather than becoming a separate execution
  workflow.
