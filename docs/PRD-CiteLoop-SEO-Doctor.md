# PRD: CiteLoop Doctor

> Date: 2026-07-02
> Status: Product PRD, revised scope
> Owner: Product
> Scope: read-only SEO/GEO diagnosis for a user-provided product URL and related public product pages
> Supersedes: earlier "SEO Doctor" framing that treated Doctor as an action handoff layer
> Related docs:
> - `docs/PRD-CiteLoop-SEO-Operations-Loop.md`
> - `docs/PRD-CiteLoop-GEO-Visibility-Layer.md`
> - `docs/PRD-CiteLoop-Analysis-Workflow.md`
> - `docs/PRD-CiteLoop-Visibility-Analysis-to-Content-Loop.md`

## 0. Executive Summary

CiteLoop must separate two product layers:

1. **Doctor**: read-only SEO/GEO diagnosis.
2. **Growth Loops**: optimization pipeline.

Doctor is the product's first diagnostic surface. A user provides a URL, CiteLoop
discovers related public product pages, checks their current SEO/GEO health, and
returns a report. The report explains what is healthy, what is risky, what is
blocked, why it matters, and what evidence supports each conclusion.

Doctor does not create opportunities, does not create content actions, does not
draft, does not publish, and does not measure impact. It is a health exam for
existing pages.

Growth Loops starts only after the user explicitly chooses to act on a Doctor
report or other visibility signals. Growth Loops owns opportunity discovery,
prioritization, content plans, review, publishing, verification, and results.

The product thesis:

```text
Doctor tells users what is unhealthy on their current pages.
Growth Loops helps users fix, optimize, publish, and measure.
```

## 1. Background

The current CiteLoop codebase already contains several pieces that look like a
Doctor, but they are mixed with the growth pipeline:

- Onboarding and Insight can fetch a landing URL and crawl up to a bounded set of
  public pages, then write product profile and content inventory.
- SEO sync and analyzer can check technical state for published canonical URLs
  and generate opportunities.
- GEO crawler audit can inspect crawler access for selected URLs and target user
  agents.
- The current SEO Doctor implementation and older PRD language allow findings to
  convert into opportunities and actions.

This creates a product positioning problem. The user expectation for "Doctor" is
not "start my optimization workflow." It is "tell me what is wrong with my
existing product pages."

Doctor must become a self-contained diagnostic product. It should reuse existing
crawl, SEO, and GEO check logic, but it must not have Growth Loop side effects.

## 2. Product Positioning

### 2.1 Product Hierarchy

```text
CiteLoop
|-- Doctor
|   |-- URL diagnosis
|   |-- SEO health report
|   |-- GEO readiness report
|   |-- crawl/access evidence
|   `-- read-only export/share
`-- Growth Loops
    |-- Analysis
    |-- Opportunities
    |-- Content Plan
    |-- Review
    |-- Publish
    `-- Results
```

### 2.2 Doctor Positioning

Doctor is a read-only health report for a product URL and its related public
pages.

Doctor answers:

- Can search and AI crawlers access the important pages?
- Are key pages indexable and canonicalized correctly?
- Do pages expose useful titles, descriptions, structured data, internal links,
  and sitemap signals?
- Can answer engines extract and cite clear product facts?
- Which existing pages are healthy, risky, blocked, missing, or hard to parse?
- What evidence supports each diagnosis?

Doctor does not answer:

- Which content should we create next?
- Which opportunity should we prioritize?
- What draft should CiteLoop write?
- Did a fix improve rankings, traffic, or citations?
- What should Autopilot do?

### 2.3 Growth Loops Positioning

Growth Loops is the execution and measurement layer. It starts from an explicit
user decision:

```text
Doctor report -> user selects findings -> Start Growth Loop -> opportunities/actions
```

Growth Loops owns:

- opportunity generation;
- priority scoring;
- brief creation;
- content and technical action planning;
- review and approval;
- publishing or manual handoff;
- verification and measurement.

## 3. Target Users and Jobs

### 3.1 SaaS Founder or Marketer

The user has a public product site and wants to know whether it is ready for
search and AI discovery.

Primary jobs:

- Enter product URL.
- Get a clear health report.
- Understand top risks.
- Export or share the report with a developer, SEO contractor, or content lead.

### 3.2 Growth Operator

The operator wants a quick diagnostic before deciding whether a project is ready
for Growth Loops.

Primary jobs:

- See whether technical blockers make optimization premature.
- Compare current report to previous runs.
- Decide whether to start a Growth Loop.

### 3.3 Developer or SEO Implementer

The implementer receives a report and needs reproducible evidence.

Primary jobs:

- See affected URLs.
- See observed status, tags, headers, and crawler evidence.
- Understand expected state.
- Validate fixes with concrete checks.

## 4. Goals

1. Let any registered project user run a Doctor diagnosis from the project's
   product URL without connecting GSC, GA4, CMS, GitHub, or publisher
   credentials.
2. Discover a bounded, relevant set of product pages associated with the input
   URL.
3. Produce a read-only SEO/GEO health report for existing public pages.
4. Clearly separate Doctor report findings from Growth Loop opportunities and
   actions.
5. Support repeat runs and report history without overwriting prior reports.
6. Show page coverage, skipped pages, crawl limits, and source confidence.
7. Provide evidence-backed findings with severity, affected URLs, confidence,
   and recommended owner.
8. Provide optional handoff to Growth Loops only after explicit user action.
9. Preserve public-page `public_only` usefulness while allowing optional GSC/GA4
   enhanced diagnostics when permission exists. This does not mean anonymous
   public-user access.
10. Make the Doctor product understandable as a standalone "site health exam."

## 5. Non-Goals

Doctor V1 does not:

- provide an anonymous, public, or free pre-signup scanner;
- create `seo_opportunities`;
- create `content_actions`;
- create topics;
- generate drafts;
- publish or update pages;
- verify published fixes as outcomes;
- measure ranking, traffic, conversion, or AI citation impact;
- promise SEO or GEO improvement;
- crawl unbounded whole sites;
- bypass login walls, robots, WAF, CAPTCHA, or platform terms;
- require GSC or GA4 for the basic report;
- run Core Web Vitals, Lighthouse, PageSpeed, or mobile-performance audits;
- run full hreflang or international SEO validation;
- run competitor SERP analysis;
- impersonate third-party crawlers through HTTP user agents;
- show private metrics when the project is public-only;
- run Autopilot;
- mix report output with the Growth Loop decision queue.

## 6. Current Implementation Gap

The existing SEO Doctor direction must be corrected in these areas:

| Area | Current or previous direction | Required Doctor direction |
|---|---|---|
| Product layer | Doctor feeds action queue directly | Doctor is read-only; Growth Loop handoff is explicit and optional |
| Scope | Technical SEO repair layer | SEO + GEO health diagnosis for current pages |
| Side effects | Findings can convert into opportunities/actions | No opportunities/actions during Doctor run |
| User promise | "Fixable issues" and AI repair handoff | "Health report" with evidence and suggested owner |
| Navigation | Doctor as part of optimization workflow | Doctor as standalone diagnostic product surface |
| Data model | `seo_doctor_findings` linked to actions | `doctor_findings` are report artifacts first; action links are handoff metadata only |
| GEO coverage | Mainly crawler access | GEO readiness includes crawlability, extractability, product facts, citation readiness |
| Report semantics | Technical repair backlog | Diagnosis, severity, evidence, page coverage, confidence |

Required retrofit:

1. Remove automatic conversion of Doctor findings into opportunities/actions.
2. Move `convert finding` affordances out of default Doctor report UI.
3. Rename user-facing copy from "SEO Doctor" to "Doctor" where the scope is
   SEO/GEO.
4. Keep current low-level technical checks, but present them as report findings.
5. Add GEO readiness sections beyond crawler access.
6. Add explicit "Start Growth Loop from this report" handoff.
7. Ensure Doctor runs remain useful in `public_only` mode.

### 6.1 Shipped SEO Doctor Migration Plan

Recent SEO Doctor work shipped a technical repair-oriented flow:

- Doctor run stages include `handoff` near the end of run execution.
- Doctor APIs include finding conversion endpoints.
- Doctor UI includes per-finding AI repair handoff affordances.
- Findings can carry links to opportunities or content actions.

This PRD intentionally changes that product contract. The migration must be
handled as a breaking product-scope change, not as a copy-only update.

Required migration:

1. **Run pipeline**
   - Remove `handoff` from the default Doctor run stage sequence.
   - Doctor run completion must stop after report composition.
   - Existing stored runs with `stage=handoff` remain readable as historical
     records, but new runs must not enter that stage.

2. **Conversion API**
   - Existing convert endpoints must be deprecated or moved behind the explicit
     Growth Loop handoff command.
   - Calling legacy convert endpoints directly must require the same
     confirmation and permission checks as `start-growth-loop`.
   - Legacy endpoints must not be invoked by Doctor run workers.

3. **Per-finding AI repair handoff UI**
   - Remove repair/action buttons from the default Doctor finding rows.
   - Preserve report-friendly guidance such as suggested owner, suggested next
     step, and evidence.
   - Move AI repair JSON, implementation instructions, and action creation into
     the Growth Loop handoff flow after the user selects findings and confirms
     side effects.

4. **Data compatibility**
   - Keep existing `linked_opportunity_id` and `linked_content_action_id` fields,
     if already shipped, as nullable historical/handoff metadata.
   - New Doctor findings created by a run must leave those fields empty.
   - Only explicit handoff may populate them.

5. **Testing**
   - Add regression tests that fail if Doctor run execution writes
     opportunities, content actions, or linked action IDs.
   - Add regression tests that prove old `/seo/doctor` routes, if retained, use
     the read-only semantics.

## 7. User Experience

### 7.1 Primary Flow

```text
User enters URL
-> Doctor validates and normalizes URL
-> Doctor discovers related pages
-> Doctor checks SEO/GEO health
-> Doctor writes report snapshot
-> User reads report
-> Optional: export/share
-> Optional: Start Growth Loop
```

### 7.2 Entry Points

Required entry points:

- Project-level "Run Doctor" URL input for authenticated users.
- Project Home Doctor module.
- Project sidebar item: `Doctor`.
- Report history page or panel.

Doctor should be usable before a user commits to the full Growth Loop, but it is
not a public or anonymous scanner. Current product policy requires registration,
authentication, and a project. Future packaging may make Doctor the first paid or
trial product layer, but not a pre-signup free tool.

### 7.3 Doctor Page Sections

Doctor report page must include:

1. **Header**
   - Input URL.
   - Canonical host.
   - Run status.
   - Last run time.
   - Overall health state.

2. **Scorecards**
   - Overall health score.
   - SEO health score.
   - GEO readiness score.
   - Crawl/access score.
   - Content clarity score.

3. **Top Findings**
   - Critical blockers.
   - Warnings.
   - Informational observations.
   - Recommended owner: marketing, SEO, developer, platform/admin.

4. **Pages Checked**
   - URL.
   - Page type.
   - Status.
   - Indexability.
   - GEO readiness.
   - Finding count.

5. **SEO Diagnostics**
   - Access, redirects, canonical, metadata, structured data, sitemap, internal
     links.

6. **GEO Diagnostics**
   - AI/search crawler access.
   - Body extractability.
   - Product fact clarity.
   - Answer-engine citation readiness.
   - Entity consistency.

7. **Evidence**
   - Raw observations are hidden by default but available in drawers/details.

8. **Report Actions**
   - Export JSON.
   - Export Markdown; PDF only if enabled in a later phase.
   - Copy share summary.
   - Start Growth Loop from selected findings.

### 7.4 Empty and Error States

Required states:

- `never_run`: user has not run Doctor.
- `running`: diagnosis is active.
- `completed`: report is ready.
- `completed_with_warnings`: report is ready but some pages were skipped or
  checks degraded.
- `blocked`: Doctor could not check the primary URL.
- `failed`: internal error or unrecoverable run failure.

Blocked examples:

- invalid URL;
- DNS failure;
- TLS failure;
- homepage fetch failure;
- robots disallows all relevant pages;
- repeated timeout;
- unsupported content type.

### 7.5 Copy Guidelines

Use product and health language:

- `Run diagnosis`
- `View report`
- `Pages checked`
- `Critical blockers`
- `GEO readiness`
- `Export report`
- `Start Growth Loop`

Avoid execution-pipeline language in Doctor default views:

- `opportunity`
- `content action`
- `autopilot`
- `publish`
- `measurement window`
- `accepted`
- `converted`

These terms may appear only after the user chooses to start Growth Loops.

## 8. Scan Scope

### 8.1 Input

Required:

- product URL.

Optional:

- product or brand name, used for entity consistency checks;
- target locale and market, stored for report context only in V1;
- known competitors, used only to detect obvious comparison/alternative page
  gaps in V1;
- GSC/GA4 connection for enhanced diagnostics;
- reduced crawl cap when the user wants a smaller run.

V1 does not use locale/market for hreflang validation or market-specific SERP
analysis. V1 does not use known competitors for ranking, SERP share, or
competitor traffic claims.

### 8.2 Page Discovery

Doctor discovers a bounded page set from:

1. Input URL and canonical homepage.
2. Same-origin navigation links.
3. `robots.txt`.
4. Sitemap URLs from robots and conventional sitemap locations.
5. Common product paths:
   - `/pricing`
   - `/features`
   - `/docs`
   - `/blog`
   - `/resources`
   - `/changelog`
   - `/integrations`
   - `/customers`
   - `/compare`
   - `/alternatives`
6. Existing project inventory URLs when available.
7. Published CiteLoop canonical URLs when available.
8. Optional GSC page URLs when Search Console is connected.

### 8.3 Default Caps

Doctor V1 must be bounded:

- Default pages checked: 25.
- Maximum pages checked: 40.
- Sitemap URLs considered: 200.
- Same-origin only by default.
- Always respect robots in V1.
- Request timeout: 5 seconds per page.
- Overall run timeout: 5 minutes.
- Fetch body limit: implementation-defined safe cap, recorded in report.

The report must show when caps affected coverage.

V1 does not allow users or admins to disable robots respect from the product UI
or API. A future internal diagnostic override would require a separate security
review, audit logging, and explicit site-ownership controls.

### 8.4 Page Types

Doctor should classify pages when possible:

- homepage;
- pricing;
- feature;
- docs;
- blog/article;
- changelog;
- integration;
- comparison;
- customer/story;
- legal;
- unknown.

Page type affects severity. For example, a `noindex` on a pricing page is more
important than missing meta description on an old blog post.

## 9. Diagnosis Modules

### 9.1 Access and Crawlability

Checks:

- HTTP status.
- Redirect chain.
- HTTPS availability.
- `www` vs non-`www` behavior.
- trailing slash variants.
- robots.txt fetch and policy.
- meta robots.
- x-robots-tag.
- unsupported content type.
- timeout or body extraction failure.

Findings:

- `homepage_fetch_failed`
- `http_error`
- `redirect_loop`
- `redirect_chain_too_long`
- `temporary_redirect_for_permanent_route`
- `robots_blocks_important_page`
- `unexpected_noindex`
- `x_robots_blocks_page`
- `unsupported_content_type`
- `body_not_extractable`

### 9.2 Canonical and Indexability

Checks:

- canonical tag exists on indexable HTML pages;
- canonical target is valid;
- canonical target is same intended host unless cross-domain is expected;
- canonical target returns indexable status;
- sitemap canonical and page canonical agree;
- duplicate URL variants are consolidated.

Findings:

- `canonical_missing`
- `canonical_target_invalid`
- `canonical_mismatch`
- `url_variant_duplicate`
- `canonical_noindex_conflict`
- `sitemap_canonical_mismatch`

### 9.3 Technical SEO

Checks:

- title exists;
- title is not duplicated across important pages;
- meta description exists on important pages;
- one primary H1 exists;
- structured data parses;
- structured data matches visible content at a basic level;
- Open Graph fields exist for shareable pages;
- social image URL is fetchable;
- internal link count is non-zero for important pages.

Findings:

- `title_missing`
- `title_duplicate`
- `meta_description_missing`
- `h1_missing`
- `h1_multiple`
- `structured_data_missing`
- `structured_data_invalid_json`
- `structured_data_template_leak`
- `social_preview_image_broken`
- `internal_link_gap`

### 9.4 Sitemap and Discovery

Checks:

- sitemap exists or absence is acceptable for a small site;
- sitemap includes important canonical pages;
- sitemap excludes dead, redirected, noindexed, or non-canonical URLs;
- key product pages are discoverable from navigation or sitemap.

Findings:

- `sitemap_missing`
- `sitemap_has_dead_url`
- `sitemap_has_noncanonical_url`
- `important_page_missing_from_sitemap`
- `important_page_not_discoverable`

### 9.5 GEO Readiness

GEO readiness means a page is accessible, extractable, and useful as an answer
source for AI/search systems. Doctor must not claim guaranteed AI citation.

Checks:

- AI/search crawler robots policy for configured user agents.
- Honest CiteLoop probe can access the page.
- Page body is extractable without requiring private login.
- Product category is clear.
- Product name and domain are consistent.
- Page contains verifiable product facts.
- Page contains citation-friendly blocks such as definitions, feature summaries,
  steps, comparisons, FAQs, integration details, or evidence snippets.
- Critical product pages are not thin or purely visual.

Default target agents:

- `Googlebot`
- `Bingbot`
- `OAI-SearchBot`
- `GPTBot`
- `PerplexityBot`
- `Perplexity-User`
- `ClaudeBot`
- `Claude-SearchBot`
- `Claude-User`

The target-agent list must be configuration-driven and updated as public crawler
documentation changes. Doctor UI must distinguish search/user-fetch crawlers
from training crawlers. Blocking a training crawler such as `GPTBot` or
`ClaudeBot` is an observation and policy note; it must not be scored as a GEO
search-readiness blocker unless product policy later defines training access as
part of a paid readiness check.

Evidence rules:

- robots decisions are high-confidence static evidence;
- HTTP/WAF probe signals are inferred unless manually confirmed;
- CiteLoop must use its own honest user agent for probes;
- Doctor must not impersonate third-party crawlers.

Findings:

- `ai_crawler_robots_blocked`
- `suspected_waf_or_bot_challenge`
- `body_not_extractable_for_ai`
- `product_category_unclear`
- `product_facts_missing`
- `citation_ready_blocks_missing`
- `entity_signal_inconsistent`
- `comparison_or_alternative_gap`
- `faq_or_definition_gap`

### 9.6 Optional GSC/GA4 Enhancements

When permission exists, Doctor may include a separate enhanced section:

- GSC-discovered URL variants not found in public crawl.
- Important URLs with no impressions after enough time.
- Sitemap URLs with GSC visibility gaps.
- Pages with search activity but missing from current sitemap.
- GA4 landing pages with high engagement but weak SEO metadata.

Rules:

- Enhanced sections must be clearly labeled as connected-data diagnostics.
- Public-only projects must not show fake CTR, position, sessions, conversion, or
  index facts.
- These checks remain read-only and do not create opportunities.

### 9.7 Deterministic vs Model-Assisted Checks

Doctor must label every finding by check mode:

| Check mode | Source | Examples | Default confidence |
|---|---|---|---|
| `deterministic` | HTTP, headers, HTML tags, robots, sitemap, parser output | HTTP error, canonical missing, noindex, invalid JSON-LD | high |
| `heuristic` | Local algorithm with thresholds | duplicate title grouping, soft 404 similarity, page type inference | medium unless threshold is high |
| `model_assisted` | LLM or embedding-based content judgment | product facts missing, citation-ready blocks missing, product category unclear | medium or low |

Rules for model-assisted checks:

1. Model-assisted findings cannot be `critical` by themselves in V1.
2. Model-assisted findings default to `warning` or `info`.
3. Evidence must include model name or provider class, prompt version, schema
   version, extracted snippets, and a short rationale.
4. Evidence must include the visible page text spans or structured facts that the
   model used. LLM-only reasoning is not enough.
5. Finding keys for model-assisted checks must use stable page URL, issue type,
   prompt version, and extracted evidence anchors, not raw model prose.
6. A model-assisted finding is marked `resolved` only after two consecutive
   successful Doctor runs no longer reproduce the issue, or after deterministic
   evidence/manual review confirms resolution.
7. If model output changes but evidence anchors remain materially similar, the
   finding stays `persistent` instead of becoming a new finding.
8. If the model provider is unavailable, Doctor completes with degraded
   model-assisted coverage and does not fabricate these findings.

This keeps history comparison meaningful while still allowing Doctor to judge
GEO readiness dimensions that cannot be reduced to tags and status codes.

## 10. Report Model

### 10.1 Report Summary

Doctor report includes:

```json
{
  "run_id": "uuid",
  "input_url": "https://example.com",
  "canonical_site_url": "https://example.com/",
  "status": "completed_with_warnings",
  "overall_score": 74,
  "seo_score": 78,
  "geo_score": 62,
  "crawl_access_score": 86,
  "content_clarity_score": 70,
  "summary": "Doctor checked 24 public pages and found 2 critical blockers, 5 warnings, and 8 informational observations.",
  "coverage": {
    "pages_discovered": 63,
    "pages_checked": 24,
    "pages_skipped_by_robots": 2,
    "pages_skipped_due_to_cap": 37,
    "pages_failed": 1
  },
  "issue_counts": {
    "critical": 2,
    "warning": 5,
    "info": 8
  },
  "top_score_deductions": [
    {
      "finding_id": "uuid",
      "label": "Pricing page is missing a canonical tag",
      "deduction": 10,
      "score_area": "seo"
    }
  ],
  "data_source_notes": [
    "public_crawl",
    "robots_static",
    "honest_probe",
    "gsc_not_connected"
  ]
}
```

### 10.2 Finding Contract

Each finding includes:

```json
{
  "id": "uuid",
  "finding_key": "canonical_missing:https://example.com/pricing",
  "severity": "critical",
  "category": "canonical",
  "issue_type": "canonical_missing",
  "title": "Pricing page is missing a canonical tag",
  "affected_urls": ["https://example.com/pricing"],
  "page_type": "pricing",
  "check_mode": "deterministic",
  "confidence": "high",
  "score_impact": {
    "area": "seo",
    "deduction": 10
  },
  "evidence": {
    "http_status": 200,
    "canonical_tag": null,
    "robots": "indexable",
    "checked_at": "2026-07-02T12:00:00Z"
  },
  "why_it_matters": "Search and AI systems need consistent canonical signals to understand the preferred version of this important product page.",
  "recommended_owner": "developer",
  "suggested_next_step": "Add a self-referencing canonical tag to the pricing page template.",
  "growth_loop_eligible": true
}
```

### 10.3 Severity

| Severity | User-facing label | Meaning |
|---|---|---|
| `critical` | Critical blocker | Likely blocks access, indexing, canonical understanding, or GEO extraction for important pages. |
| `warning` | Warning | Weakens discovery, clarity, parsing, sharing, or citation readiness. |
| `info` | Observation | Non-blocking context or low-risk hygiene note. |

Doctor must use simple severity labels in UI. Engineering may map them to P0/P1/P2
internally, but the report should not feel like an internal queue.

### 10.4 Score Semantics

Scores are health indicators, not outcome forecasts.

V1 scoring must be deterministic from persisted findings so repeat runs are
comparable. The score composer must store top deductions in the report summary.

Base score:

```text
score_area = max(0, round(100 - min(total_deduction, 100)))
overall_score = round(
  0.35 * crawl_access_score +
  0.30 * seo_score +
  0.25 * geo_score +
  0.10 * content_clarity_score
)
```

Base deductions:

| Severity | Base deduction |
|---|---:|
| `critical` | 18 |
| `warning` | 7 |
| `info` | 0 |

Area assignment:

| Finding category | Primary score area |
|---|---|
| access, crawlability, robots, redirect, HTTP | `crawl_access_score` |
| canonical, metadata, structured data, sitemap, internal links | `seo_score` |
| AI crawler policy, extractability, citation-ready blocks, entity consistency | `geo_score` |
| product category, product facts, thin page, content clarity | `content_clarity_score` |

Multipliers:

| Factor | Multiplier |
|---|---:|
| Homepage, pricing, docs-critical, feature, integration, comparison page | 1.25 |
| CiteLoop-generated or recently changed page | 1.15 |
| Ordinary page | 1.0 |
| Deterministic high confidence | 1.0 |
| Heuristic medium confidence | 0.75 |
| Model-assisted medium/low confidence | 0.5 |

Per-finding deduction:

```text
deduction = round(base_deduction * page_importance_multiplier * confidence_multiplier)
```

Caps:

- Overall score is capped at 69 when any unresolved critical blocker exists.
- Overall score is capped at 84 when any warning exists and no critical blocker
  exists.
- Failed runs with no usable report do not show a score.
- Scores must explain the top deductions.
- Scores must not claim ranking, traffic, conversion, or citation impact.

Additional rules:

- `info` findings do not deduct score.
- Training-crawler policy observations do not deduct GEO score in V1.
- Model-assisted findings can deduct at most 4 points per finding and at most 12
  total points per score area.
- If a score area has no usable data because the run was degraded, report the
  area as `unavailable` and exclude it from weighted average by re-normalizing
  the remaining area weights.
- Report history must store the scoring version, for example
  `doctor_score_v1`, so future formula changes do not corrupt historical
  comparisons.

## 11. Data Model

Doctor should use Doctor-owned product objects instead of writing directly into
Growth Loop objects.

Recommended tables:

### 11.1 `doctor_runs`

- `id`
- `project_id`
- `input_url`
- `canonical_site_url`
- `trigger`: `onboarding`, `manual`, `scheduled`, `api`
- `status`: `queued`, `running`, `completed`, `completed_with_warnings`, `blocked`, `failed`, `cancelled`
- `stage`
- `progress_percent`
- `scoring_version`
- `started_at`
- `finished_at`
- `pages_discovered`
- `pages_checked`
- `overall_score`
- `seo_score`
- `geo_score`
- `crawl_access_score`
- `content_clarity_score`
- `input_snapshot`
- `report_summary`
- `error`
- `created_by_user_id`

### 11.2 `doctor_pages`

- `id`
- `project_id`
- `run_id`
- `url`
- `normalized_url`
- `page_type`
- `source`: `input`, `navigation`, `sitemap`, `robots`, `inventory`, `gsc`, `generated`
- `status`: `checked`, `skipped`, `failed`
- `skip_reason`
- `http_status`
- `indexability`
- `seo_state`
- `geo_state`
- `raw_observations`
- `checked_at`

### 11.3 `doctor_findings`

- `id`
- `project_id`
- `run_id`
- `page_id`
- `finding_key`
- `severity`
- `category`
- `issue_type`
- `title`
- `affected_urls`
- `normalized_urls`
- `page_type`
- `check_mode`: `deterministic`, `heuristic`, `model_assisted`
- `confidence`
- `score_area`
- `score_deduction`
- `evidence`
- `why_it_matters`
- `recommended_owner`
- `suggested_next_step`
- `growth_loop_eligible`
- `status`: `open`, `muted`, `dismissed`, `resolved`, `stale`
- `dismissed_at`
- `dismissed_by_user_id`
- `dismiss_reason`
- `muted_until`
- `report_note`
- `first_seen_at`
- `last_seen_at`
- `resolved_at`

Finding lifecycle:

- `open`: visible active finding in the report.
- `muted`: hidden from default summary until `muted_until`, but still visible in
  report details and history.
- `dismissed`: user marked as not useful or false positive. Dismissed findings
  do not affect score after dismissal, but remain in history and quality
  metrics.
- `resolved`: finding was present before and is no longer reproduced according
  to its check-mode resolution rule.
- `stale`: finding came from an old run whose page is no longer in scope.

Dismiss and mute are Doctor report controls only. They must not dismiss Growth
Loop opportunities unless the user is already in Growth Loops and confirms that
separate action.

Stable `finding_key` rules:

```text
finding_key =
  issue_type + ":" +
  normalized_primary_url + ":" +
  normalized_evidence_target + ":" +
  check_mode + ":" +
  rule_or_prompt_version
```

Normalization rules:

- Preserve scheme/host/path variants when the variant itself is evidence for the
  issue, such as `http` vs `https` or `www` vs non-`www`.
- Remove fragments, tracking parameters, default ports, and irrelevant query
  ordering noise.
- Normalize percent-encoding and trailing slashes according to the same URL
  normalization path used by crawl/check logic.
- For multi-URL issues such as duplicate titles, use the highest-importance URL
  as `normalized_primary_url` and a stable hash of the sorted affected URL set as
  `normalized_evidence_target`.
- For model-assisted findings, include prompt version and evidence anchors so
  wording changes do not create false new findings.

Cross-run comparison:

- Same `finding_key` in consecutive runs is `persistent`.
- Missing deterministic/heuristic finding in the next successful comparable run
  is `resolved`.
- Missing model-assisted finding requires two consecutive comparable runs before
  `resolved`, unless manual or deterministic evidence confirms resolution.
- A finding dismissed by the user remains dismissed if it reappears with the same
  key and no materially stronger evidence.

### 11.4 `doctor_report_snapshots`

- `id`
- `project_id`
- `run_id`
- `format`: `json`, `markdown`, `pdf`
- `snapshot`
- `storage_url`
- `content_type`
- `created_at`

For V1, JSON and Markdown snapshots may be stored inline. Binary exports such as
PDF must use object storage or an equivalent file store and keep only metadata
and `storage_url` in the database.

### 11.5 Relationship to Existing Tables

Doctor may read from:

- crawl results;
- `technical_checks`;
- SEO property settings;
- GEO crawler snapshots;
- content inventory;
- GSC/GA4 data when connected.

Doctor must not write to during a run:

- `seo_opportunities`;
- `content_actions`;
- topics;
- articles;
- publisher state;
- measurement results.

Optional Growth Loop handoff may later create opportunities or actions, but that
is a separate user-triggered command.

## 12. API Requirements

Required endpoints:

```text
GET  /api/projects/{projectID}/doctor
POST /api/projects/{projectID}/doctor/runs
GET  /api/projects/{projectID}/doctor/runs/{runID}
POST /api/projects/{projectID}/doctor/runs/{runID}/cancel
GET  /api/projects/{projectID}/doctor/runs/{runID}/pages
GET  /api/projects/{projectID}/doctor/runs/{runID}/findings
GET  /api/projects/{projectID}/doctor/latest
GET  /api/projects/{projectID}/doctor/history
GET  /api/projects/{projectID}/doctor/runs/{runID}/export.json
GET  /api/projects/{projectID}/doctor/runs/{runID}/export.md
POST /api/projects/{projectID}/doctor/findings/{findingID}/dismiss
POST /api/projects/{projectID}/doctor/findings/{findingID}/mute
POST /api/projects/{projectID}/doctor/findings/{findingID}/note
POST /api/projects/{projectID}/doctor/runs/{runID}/start-growth-loop
```

Compatibility note:

- Existing `/seo/doctor` endpoints may remain as deprecated read-only aliases
  for one version cycle during migration.
- User-facing API and frontend naming should move toward `/doctor`.

`POST /runs` behavior:

- Returns active run if one already exists.
- Does not start duplicate runs for the same project.
- Manual starts are rate limited.
- Returns immediately with queued/running state.
- Background worker performs crawl/check/report composition.

`POST /runs/{runID}/cancel` behavior:

- Cancels only queued or running jobs owned by the project.
- Stores `status=cancelled` and a user-visible cancellation message.
- Does not delete partial observations.
- Does not create opportunities or actions.

Finding controls:

- `dismiss` requires a reason and stores who dismissed it.
- `mute` requires `muted_until` or a fixed preset such as 30 days.
- `note` stores a report-local note without changing score or lifecycle.
- These controls are report controls, not Growth Loop controls.

`start-growth-loop` behavior:

- Requires explicit user action.
- Accepts selected finding IDs.
- Creates opportunities/actions only after this call.
- Stores source `doctor_run_id` and selected `doctor_finding_ids` in downstream
  evidence.

## 13. Progress and Run Stages

Stages:

| Stage | Meaning |
|---|---|
| `queued` | Run accepted. |
| `discovering` | Normalizing URL, reading robots/sitemap, collecting candidate pages. |
| `checking_access` | Fetching pages, redirects, robots, status, content extraction. |
| `checking_seo` | Canonical, metadata, structured data, sitemap, internal links. |
| `checking_geo` | AI crawler policy, extractability, product facts, citation readiness. |
| `writing_report` | Grouping findings and composing report snapshot. |
| `completed` | Report ready. |

Progress response:

```json
{
  "run_id": "uuid",
  "status": "running",
  "stage": "checking_geo",
  "progress_percent": 72,
  "message": "Checking AI crawler access and extractable product facts",
  "pages_discovered": 38,
  "pages_checked": 21,
  "findings_found": 9
}
```

Frontend polling:

- Poll every 2 seconds while queued/running.
- After 60 seconds, poll every 5 seconds.
- Stop when completed, blocked, failed, or cancelled.

## 14. Growth Loop Handoff

Doctor findings can become Growth Loop inputs only through explicit handoff.

Required UX:

1. User opens report.
2. User selects findings or accepts recommended bundle.
3. User clicks `Start Growth Loop`.
4. CiteLoop explains what will happen:
   - opportunities may be created;
   - content or technical actions may be proposed;
   - future steps may require review or publishing access.
5. User confirms.
6. Growth Loops creates downstream objects.

Rules:

- Doctor run itself never creates opportunities/actions.
- Handoff is optional.
- Handoff records source report and findings.
- Handoff should preserve evidence exactly as Doctor observed it.
- Dismiss/report-note actions inside Doctor do not dismiss Growth Loop
  opportunities unless the user is already in Growth Loops.

## 15. Reporting and Export

### 15.1 Human Summary

The report must be readable by a non-technical operator:

- health score;
- top critical blockers;
- top warnings;
- page coverage;
- SEO readiness;
- GEO readiness;
- what to fix first;
- who should own each fix.

### 15.2 Evidence Detail

Evidence must include:

- checked URL;
- final URL after redirects;
- HTTP status;
- canonical tag;
- robots/meta robots/x-robots state;
- title/meta/H1 observations;
- structured data parse result;
- sitemap source;
- crawler access evidence type;
- confidence;
- timestamp.

### 15.3 Export Formats

Required:

- JSON export.
- Markdown export.

Optional:

- PDF export.
- Shareable hosted report.

Export must not include private credentials, raw tokens, service account fields,
or sensitive provider payloads.

## 16. Permissions and Privacy

Doctor V1 must run in public-page mode but not public-user mode.

Access policy:

- Doctor requires an authenticated registered user.
- Doctor requires a project.
- The user must have permission to view the project.
- Starting a run requires permission to manage or analyze the project.
- No anonymous, public, pre-signup, or free unauthenticated Doctor endpoint is in
  scope.
- The input URL must match the project configured site host unless the user has
  permission to update project settings and explicitly changes the project URL.

URL safety:

- Reject localhost, loopback, link-local, private IP ranges, multicast,
  unspecified addresses, and cloud metadata service addresses.
- Resolve DNS before fetch and reject private or disallowed resolved addresses.
- Re-check the final address after redirects.
- Enforce same-origin by default.
- Enforce scheme allowlist: `http` and `https` only.
- Enforce per-user and per-project run rate limits.
- Store enough audit data to investigate abusive scan attempts.

Capability tiers:

| Capability | Requirement |
|---|---|
| Public crawl and page checks | No additional connection |
| GSC-backed diagnostics | Search Console connection |
| GA4-backed diagnostics | GA4 connection |
| Repo-aware owner hints | Publisher/repo connection |
| Growth Loop execution | Explicit user handoff plus required downstream permissions |

Doctor must not:

- fetch authenticated pages;
- bypass access controls;
- scan arbitrary third-party sites outside the user's configured project host;
- store credentials in report exports;
- expose admin-only provider details to ordinary users;
- imply it has private search data when not connected.

## 17. Metrics

Product metrics:

- Doctor run start rate after URL entry.
- Doctor completion rate.
- Median time to first report.
- Percentage of reports with at least one useful finding.
- Export/share rate.
- Growth Loop handoff rate.
- Repeat run rate.

Quality metrics:

- False positive rate from user dismissals.
- Blocked run rate.
- Page coverage rate.
- Percentage of findings with evidence.
- Percentage of findings with clear owner.
- GSC/GA4 enhanced section usage when connected.

Guardrail metrics:

- Number of Doctor runs that create opportunities without handoff: must be zero.
- Number of Doctor runs that create content actions without handoff: must be zero.
- Number of public-only reports showing private metrics: must be zero.

Phase 1 target baselines:

| Metric | Target |
|---|---:|
| Doctor completion rate for valid project URLs | >= 90% |
| Median time to first report for default 25-page runs | <= 2 minutes |
| P95 time to report for default 25-page runs | <= 5 minutes |
| Findings with affected URL, evidence, confidence, and owner | 100% |
| Reports with explicit coverage counts | 100% |
| Public-page reports with private GSC/GA4 metrics | 0 |
| Runs that create Growth Loop objects before handoff | 0 |

## 18. Phasing

### Phase 0: PRD and Product Boundary

Scope:

- Align Doctor vs Growth Loops positioning.
- Update PRD.
- Identify current implementation that violates read-only Doctor boundary.
- Define migration plan for shipped SEO Doctor repair/handoff features.

Exit criteria:

1. PRD exists and states Doctor is read-only.
2. PRD explicitly separates Doctor from Growth Loops.
3. PRD defines retrofit requirements.
4. PRD defines route, scheduler, and registered-user access decisions.

### Phase 1: Read-Only Doctor Foundation

Scope:

- Doctor run and report storage.
- URL input and bounded page discovery.
- Public SEO/GEO checks.
- Report UI.
- No side effects into opportunities/actions.

Exit criteria:

1. User can run Doctor for a URL.
2. Report is produced without GSC/GA4.
3. Findings are stored as Doctor report artifacts.
4. No `seo_opportunities` are created by Doctor run.
5. No `content_actions` are created by Doctor run.
6. Doctor is available only to registered users inside a project.
7. SSRF and private-network URL protections are specified and tested.

### Phase 2: GEO Readiness Expansion

Scope:

- AI crawler robots/access matrix.
- Extractable product facts.
- Citation-ready content blocks.
- Entity consistency.
- GEO readiness scoring.

Exit criteria:

1. Report includes GEO readiness score.
2. Report distinguishes robots evidence from inferred WAF/probe evidence.
3. Report flags missing product facts or citation-ready blocks.
4. UI does not promise AI citation improvements.

### Phase 3: Report History and Export

Scope:

- Report history.
- Changed since last run.
- JSON and Markdown export.
- Share summary.

Exit criteria:

1. User can view previous reports.
2. User can compare new, persistent, and resolved findings.
3. User can export JSON and Markdown.

### Phase 4: Explicit Growth Loop Handoff

Scope:

- Select findings.
- Start Growth Loop CTA.
- Confirm downstream side effects.
- Create opportunities/actions only after confirmation.

Exit criteria:

1. Doctor report has optional Growth Loop handoff.
2. Handoff requires explicit confirmation.
3. Created downstream objects link back to source Doctor report.
4. Doctor run still has zero automatic Growth Loop side effects.

### Phase 5: Connected-Data Enhancements

Scope:

- GSC-backed diagnostics.
- GA4-backed diagnostics.
- Connected-data labels.

Exit criteria:

1. Public-only reports remain useful.
2. Connected reports add private-data sections only when permission exists.
3. UI clearly labels connected-data findings.

## 19. Acceptance Criteria

### 19.1 Product Boundary

1. Doctor is described in product copy as read-only SEO/GEO diagnosis.
2. Growth Loops is described as the optimization pipeline.
3. Doctor run does not create `seo_opportunities`.
4. Doctor run does not create `content_actions`.
5. Doctor run does not create topics or drafts.
6. Doctor run does not publish or update pages.
7. Doctor run does not measure SEO/GEO impact.
8. Growth Loop handoff requires explicit user confirmation.

### 19.2 Run and Report

9. User can enter a URL and start a Doctor run.
10. Doctor validates and normalizes the URL.
11. Doctor discovers related pages within configured caps.
12. Doctor always respects robots in V1.
13. Doctor reports pages checked, skipped, failed, and skipped due to cap.
14. Doctor returns a completed report when the homepage and at least one page are
    checked successfully.
15. Doctor returns blocked state when the primary URL cannot be checked.
16. Doctor stores report history and does not overwrite prior reports.

### 19.3 SEO Diagnosis

17. Report identifies HTTP errors.
18. Report identifies redirect loops or excessive redirect chains.
19. Report identifies missing or invalid canonical tags.
20. Report identifies unexpected `noindex` or robots blockers.
21. Report identifies missing title, meta description, or H1 on important pages.
22. Report identifies invalid structured data.
23. Report identifies sitemap and internal-link gaps.

### 19.4 GEO Diagnosis

24. Report checks configured AI/search crawler robots policy.
25. Report distinguishes robots-static evidence from inferred probe evidence.
26. Report checks whether page body is extractable.
27. Report checks whether important pages contain clear product facts.
28. Report flags missing citation-ready blocks where appropriate.
29. Report does not claim that a fix will guarantee AI citations.

### 19.5 Evidence and Trust

30. Every finding has affected URL, category, severity, confidence, and evidence.
31. Every finding has a human-readable explanation.
32. Every finding has a suggested next step and recommended owner.
33. Evidence drawers expose raw observations without overwhelming the default
    report.
34. Public-only reports do not show GSC/GA4 metrics.
35. Connected-data sections are clearly labeled.

### 19.6 Export and History

36. User can export report as JSON.
37. User can export report as Markdown.
38. User can view previous report runs.
39. User can see new, persistent, and resolved findings across runs.

### 19.7 Handoff

40. User can select findings and start a Growth Loop.
41. Handoff confirmation explains that opportunities/actions may be created.
42. Downstream opportunities/actions link back to the source Doctor report.
43. Cancelling handoff leaves the Doctor report unchanged.

### 19.8 Regression Guardrails

44. Automated tests prove Doctor run does not create opportunities by default.
45. Automated tests prove Doctor run does not create content actions by default.
46. Automated tests prove public-only reports do not contain private metrics.
47. Automated tests prove existing `/seo/doctor` aliases, if retained, do not
    change read-only semantics.
48. Automated tests prove Growth Loop handoff is the only path from Doctor
    findings to opportunities/actions.
49. Automated tests prove new Doctor runs do not enter a `handoff` stage.
50. Automated tests prove legacy `/seo/doctor` aliases, if retained, preserve the
    read-only contract.
51. Automated tests prove score output is deterministic from persisted findings
    and includes top deductions.
52. Automated tests prove active critical and warning findings cap the overall
    score as specified.
53. Automated tests prove model-assisted findings cannot be critical by
    themselves.
54. Automated tests prove model-assisted findings require two comparable clean
    runs before automatic resolution.
55. Automated tests prove dismiss and mute affect Doctor report display without
    affecting Growth Loop objects.
56. Automated tests prove unauthenticated users cannot start Doctor runs.
57. Automated tests prove users cannot run Doctor against projects they do not
    own or have access to.
58. Automated tests prove private IP, localhost, link-local, and metadata-service
    URLs are rejected before fetch and after redirects.
59. Automated tests prove `cancel` changes active runs to `cancelled` without
    creating opportunities or actions.
60. Phase 1 production verification confirms median report time, completion
    rate, and zero-side-effect guardrails against the target baselines.

## 20. Open Decisions

1. Should PDF export be V1 or V2?
2. Should later phases raise the maximum page cap above 40 for larger sites?
3. What paid packaging, if any, should unlock connected-data Doctor sections?

## 21. V1 Decisions

These are fixed unless product changes them explicitly:

- Doctor is read-only.
- Doctor covers both SEO and GEO.
- Basic Doctor works without GSC/GA4.
- Doctor requires a registered authenticated user and project.
- Doctor is not available as an anonymous, public, or free pre-signup scanner.
- `/projects/{projectID}/doctor` is the canonical project route.
- `/api/projects/{projectID}/doctor` is the canonical API namespace.
- Existing `/seo/doctor` routes may remain as deprecated read-only aliases for
  one version cycle, then redirect or be removed.
- Weekly scheduled Doctor remains in V1 for active registered projects only.
- Doctor findings are report artifacts, not opportunities.
- Growth Loop handoff is explicit and optional.
- Doctor uses bounded public crawl.
- Doctor always respects robots in V1.
- Doctor does not promise rankings, traffic, conversions, or AI citations.
- Doctor exports JSON and Markdown.
- Existing SEO/GEO/Growth Loop infrastructure should be reused where possible,
  but side effects must be removed from Doctor runs.
