# Opportunity Discovery Growth Radar Design

## Problem

CiteLoop's Opportunity Discovery is running, but its current inputs and selection rules can make a healthy scheduler look stalled. Production inspection on July 13, 2026 found one active project with 1,404 active GEO prompts. The observer is capped at ten prompts per run and reads prompts in `priority DESC, created_at ASC` order, so recent runs repeatedly selected the same oldest ten prompts instead of exploring the portfolio. Across the latest 17 observer runs, 162 observations covered only ten distinct prompt IDs.

Those prompts were generated from an unbounded Cartesian product of profile topics, personas, and competitors. Internal implementation details such as `AES-256-GCM` had entered the public topic list, producing low-value questions such as product comparisons “for AES-256-GCM.” The generic OpenAI-compatible answer provider returned no cited URLs, no competitor citations, and no project citations. The downstream analyzer only creates an Opportunity for narrow citation or brand-mention conditions, so these observations correctly yielded zero candidates under the existing rules even though they consumed nearly all discovery capacity.

The signal path is also narrow. Google Search Console repeatedly identifies the same canonical query gap, which deduplication suppresses, while the AI path does not currently research search results, confirmed competitors, or market content. It observes a static prompt inventory and treats answer metadata as its principal evidence. This explains why daily workflows can succeed for days without producing a new Opportunity.

The desired system is a Growth Radar: it continuously identifies evidence-backed gaps across topics, search intent, audiences, asset types, and publishing surfaces. It must improve the upstream discovery mechanism without rebuilding the existing Opportunity-to-content pipeline.

## Inspiration and Boundaries

[SuperX](https://superx.so/) is an example of the output cadence and content breadth enabled by an SEO content system; it is not a CiteLoop competitor and must never be inserted into a project's competitor set or monitored as a special case. A July 2026 sample of its public blog showed a large, high-frequency library spanning how-to, comparison, analytics, growth, content-strategy, and glossary patterns.

The relevant product reference is [Outrank](https://www.outrank.so/), which describes a workflow that researches a business, niche, audience, and competitors; finds potential keywords; builds a content plan; creates linked articles and images; and publishes through multiple integrations. CiteLoop should borrow the architectural lesson—continuous research feeding a content plan—not copy SuperX topics, titles, cadence, or programmatic duplication.

This design does not create a second Writer, Review, Publisher, or syndication system. The repository already supports Opportunity handoff, canonical content, publishing variants, and multiple asset types and destinations. Growth Radar produces a richer, evidence-backed Opportunity Spec and passes it to that existing chain. Exact target selection and platform-native generation are governed by the separate [Platform Content Contracts design](./2026-07-13-platform-content-contracts-design.md).

## Desired Behavior

- Discovery refreshes external and first-party evidence continuously instead of exhausting a static prompt list.
- Project context is sanitized into public product concepts, customer problems, audiences, confirmed competitors, and internal-only details before prompts or topics are generated.
- The prompt portfolio is bounded, diverse, and rotated by least-recent observation rather than creation time.
- Candidates can recommend the existing range of asset types, not only blog posts, and can target the existing supported publication surfaces.
- Every candidate receives a deterministic score, deduplication identity, evidence bundle, and disposition reason.
- A run that creates zero Opportunities still explains what was scanned and why each candidate was deduplicated, held, placed on a watchlist, or filtered.
- The system targets three to ten high-quality Opportunities per project per week as an operating health range, never as a quota. It must not lower thresholds or manufacture work to hit the range.
- An Opportunity may include an image brief. The existing Writer later decides whether the article benefits from a hero image, explanatory visual, comparison visual, or workflow visual.
- Image generation failure never blocks review or publication of otherwise valid content.

## Existing-System Ownership

Growth Radar owns research, evidence normalization, topic clustering, coverage-gap analysis, candidate scoring, arbitration, and Opportunity creation.

The existing downstream system continues to own:

- Opportunity acceptance and conversion to content work;
- Writer drafting and asset-contract compliance;
- canonical article and platform-variant creation;
- Review decisions;
- publication and syndication;
- measurement and learning.

Existing asset types remain authoritative, including blog posts, comparison and alternative pages, use-case pages, integration pages, templates or checklists, glossary definitions, benchmark reports, metadata rewrites, internal-link patches, schema patches, and sitemap updates. Existing destinations remain authoritative, including the owned blog/site and configured Dev.to, Hashnode, Medium, LinkedIn, Reddit, Hacker News, docs, landing, and hosted-asset surfaces. Growth Radar recommends from capabilities currently enabled for the project; it does not promise a destination that is not configured.

## Architecture

The daily funnel is:

1. **Evidence refresh** reads project context, site inventory, Search Console changes, search-result evidence, AI-answer observations, and configured competitor evidence.
2. **Context classification** separates public product concepts from internal or sensitive implementation details.
3. **Topic map update** groups evidence into stable topic, intent, audience, and journey-stage clusters.
4. **Coverage analysis** compares external demand with existing and planned assets across types and surfaces.
5. **Candidate generation** proposes the smallest useful action that closes each gap.
6. **Scoring and policy gates** score evidence, reject unsafe or irrelevant work, and place borderline work on the watchlist.
7. **Deduplication and arbitration** merge equivalent candidates and resolve conflicts with active Opportunities, drafts, published assets, and site fixes.
8. **Opportunity creation** writes approved Opportunity Specs into the existing workflow.
9. **Funnel reporting** records counts, reasons, provenance, cost, and coverage for the run.

The weekly maintenance job rebuilds topic clusters and the active prompt portfolio from current evidence. A material Project Context update schedules the same reevaluation immediately. Daily jobs refresh evidence and candidate scores without rebuilding the entire portfolio unnecessarily.

## Source Authority and Evidence

Sources have explicit authority:

1. User-confirmed Project Context and user-configured competitors are authoritative for product identity and competitor membership.
2. The project's published site and approved content inventory are authoritative for what CiteLoop currently claims and covers.
3. First-party performance data such as Search Console is authoritative for observed demand and performance on the owned property.
4. Search results, public pages from confirmed competitors, and answer-engine outputs are external observations with collection time and source URL.
5. LLM-generated hypotheses are proposals only. They cannot independently satisfy the evidence requirement, establish a competitor, or prove a citation.

A generic LLM response with an empty citation array is recorded as an uncited answer observation. The system must not infer citations from prose, convert mentioned names into cited sources, or award citation-evidence points. Search-result and page evidence must retain normalized URL, title, source type, query or prompt, rank when available, collected time, and content hash.

Confirmed competitors come only from user configuration or an explicit user-approved context update. Discovered domains can be proposed for confirmation but remain market sources until approved. SuperX is not seeded or treated specially.

Collectors respect robots directives, provider terms, rate limits, and per-project budgets. Secrets, credentials, private repository text, and raw internal diagnostics are never sent to public search or image-generation providers.

## Context Classification and Sanitization

Every context term is assigned one of these classes before use:

- public product capability;
- customer problem or use case;
- ideal-customer profile or buyer intent;
- confirmed competitor;
- search or answer-engine language;
- internal or sensitive implementation detail;
- unknown, requiring evidence or confirmation.

Internal storage, encryption, database, credential, deployment, and protocol details are excluded from topic generation by default. A term such as `AES-256-GCM` can become discoverable only when first-party search data or independently collected market evidence demonstrates relevant customer demand and the product is authorized to make a public claim about it. Unknown or unconfirmed product capabilities place the candidate on hold rather than allowing the model to invent support.

The classifier emits both the accepted public vocabulary and rejected terms with reason codes. These reason codes are visible in run diagnostics and are testable. Prompt generation and image briefs consume only accepted vocabulary.

## Topic Map and Coverage Model

A cluster is keyed by normalized topic, intent, audience, and journey stage. It aggregates related queries, prompts, pages, assets, and evidence without collapsing materially different intents.

Supported intent classes are informational, how-to, problem-solving, comparison, alternative, use case, integration, template, glossary, evidence or benchmark, navigational, and transactional. Journey stages are awareness, consideration, decision, adoption, and expansion.

Coverage is evaluated across:

- published canonical content;
- configured platform variants;
- approved and in-progress content;
- active Opportunities;
- site structure and internal links;
- existing search impressions, clicks, position, and answer-engine presence.

A cluster is not considered covered merely because a page mentions a keyword. Coverage records the asset's primary intent, audience, type, freshness, quality state, and measured outcome. Growth Radar can therefore recommend an internal-link patch, comparison page, integration page, Reddit article, glossary definition, or benchmark report when that action fits better than another blog post.

## Bounded Prompt Portfolio and Rotation

Each project has at most 60 active answer-engine prompts. A cluster contributes at most six active prompts and no more than two prompts for the same intent and audience combination. The weekly builder selects a balanced portfolio across active clusters, intents, audiences, and journey stages. Replaced prompts are archived with provenance rather than deleted.

Each observation run selects at most ten prompts using weighted fair rotation:

1. prompts never observed;
2. least recently observed prompts;
3. clusters with the lowest trailing-seven-day coverage;
4. higher evidence-backed demand;
5. stable prompt ID as the final tiebreaker.

No prompt may be selected again while another eligible active prompt in the same priority band has not been observed, unless freshness policy requires a targeted recheck. A targeted recheck records its reason and does not silently consume the exploration budget. Observation rows record prompt, provider, model, provenance, selection reason, citation capability, latency, cost, and parse quality.

The prompt cap prevents Cartesian growth. The portfolio builder creates candidates from topic clusters and templates, then applies semantic deduplication and diversity constraints before activation. It never materializes every topic × persona × competitor combination.

## Opportunity Spec

Every created Opportunity includes a versioned spec with:

- `intent` and `journey_stage`;
- `audience`;
- `topic_cluster_id` and normalized topic;
- `asset_type` using the existing asset taxonomy;
- `canonical_target`, identifying the owned source destination;
- exact `target_platforms`, ordered and deduplicated, rather than only `blog`, `syndication`, or `both`;
- `evidence`, containing source records and a concise rationale;
- `recommended_action` and expected user value;
- optional `image_brief` describing informational purpose, not visual decoration;
- `success_metric` with measurement window;
- `dedupe_identity` and related existing work;
- component score breakdown, penalties, final score, and policy decisions;
- source and classifier versions for reproducibility.

The deduplication identity is a stable hash of project, normalized topic cluster, intent, audience, asset type, and canonical surface. Semantic similarity can merge candidates inside that identity family but cannot merge different user intents merely because they share keywords.

## Scoring, Gates, and Dispositions

The positive score totals 100 points:

- observed demand: 25;
- cross-asset coverage gap: 20;
- product and audience relevance: 15;
- commercial or growth value: 15;
- freshness or change significance: 10;
- cross-platform reuse potential: 10;
- evidence quality and diversity: 5.

Penalties are applied after the positive score:

- duplicate active or completed work: −40;
- likely keyword or intent cannibalization: −30.

Hard policy gates override the numeric score:

- unconfirmed product capability: hold for confirmation;
- LLM-only evidence: watchlist, regardless of score;
- off-product, sensitive, or unsupported claim: filter;
- dismissed candidate without materially new evidence: do not reopen;
- unresolved conflict with canonical work: arbitration queue.

After penalties and gates, a score of 75 or more creates an Opportunity, 60–74 enters the watchlist, and below 60 is filtered. Watchlist entries are rescored when evidence changes and expire after 90 days without new evidence. A dismissed identity can reopen only when a new source, a material metric change, a newly confirmed capability, or a distinct intent changes its evidence fingerprint.

## Deduplication and Arbitration

Candidate arbitration checks, in order:

1. exact dedupe identity;
2. active Opportunity or approved content work;
3. published canonical asset with the same primary intent;
4. planned platform variants;
5. conflicting site fix or metadata action;
6. semantic overlap and cannibalization risk.

Equivalent evidence is merged into the existing record. A better asset-type recommendation can update a watchlist candidate but cannot silently mutate accepted or in-progress work. Conflicts create an explainable arbitration record and preserve both proposals until a deterministic rule or user decision resolves them.

## Multi-Asset and Surface Routing

Routing chooses the action that best matches evidence and intent:

- how-to and problem-solving gaps commonly route to a blog, docs article, checklist, or community article;
- high-intent comparisons route to comparison or alternative pages and may produce adapted community variants;
- integration demand routes to an integration page or documentation;
- repeated definitional demand routes to a glossary definition or FAQ block;
- original quantitative evidence routes to a benchmark or source-backed evidence page;
- an existing relevant page with weak discovery routes to metadata, schema, internal-link, or sitemap work instead of net-new content.

The Opportunity selects an exact canonical target and exact target platforms before generation. Selection is restricted by the versioned Platform Content Contract capability matrix and project configuration. The existing content pipeline still validates connection and publishing readiness. The canonical asset owns the factual source and revision history. Each platform target receives an independently generated or adapted artifact under its pinned contract; variants are not created by changing only the platform name.

## Run Funnel and Explainability

Every discovery run persists a funnel summary:

- sources scheduled, succeeded, skipped, and failed;
- evidence records added, changed, reused, and expired;
- context terms accepted, rejected, held, and their reasons;
- prompt portfolio size, selected prompts, coverage, and rotation reasons;
- candidates generated;
- exact and semantic duplicates;
- arbitration conflicts;
- watchlist additions and updates;
- filtered candidates by reason;
- Opportunities created;
- provider cost, latency, and citation capability.

The UI presents a compact run summary and drill-down reason codes. “0 new Opportunities” is a valid outcome only when the run shows the inspected evidence and dispositions. A run with no usable sources or no prompt rotation is degraded, not successful-zero.

## Article Images

Images extend the existing content chain; they are not generated during discovery. An Opportunity may contain an image brief. After Writer has a draft and outline, it converts that brief into zero or more article asset plans based on informational value.

The default maximum is one hero plus two inline assets. Glossary definitions, FAQ blocks, short community posts, and content that gains no explanatory value may use zero. Supported roles are:

- `hero`;
- `inline_explainer`;
- `comparison_visual`;
- `workflow_visual`.

Benchmark charts and visuals based on real numeric data are rendered deterministically from cited data rather than generated as illustrative images. Generated assets must not contain competitor logos, fabricated product interfaces, customer portraits, or unsupported capability claims. Prompts use the approved brief and outline, not raw scraped pages.

### Data Model

A new `article_assets` record contains:

- `id`, `project_id`, `article_id`, and `role`;
- `status`: `planned`, `generating`, `ready`, or `failed`;
- `image_brief`, `generation_prompt`, `alt_text`, and optional `caption`;
- `provider`, `model`, dimensions, MIME type, and storage URL;
- `content_hash`, `brief_hash`, revision, and optional superseded asset ID;
- cost in USD and provider request ID;
- failure code and sanitized failure detail;
- created, updated, started, completed, and last-retried timestamps.

The uniqueness key is article ID, role, and brief hash. Re-running unchanged work reuses a ready asset. Editing only alt text or caption does not regenerate the bitmap. An explicit regeneration creates a new revision and preserves the prior asset for rollback and audit.

### Provider and Storage Contract

An image provider accepts a versioned generation request and returns binary content, MIME type, dimensions, provider metadata, cost, and request ID. Phase 3 ships with an OpenAI Images API adapter configured through a separate encrypted admin credential; text-only TokenGate credentials are never assumed to support images. The interface permits later providers without changing Writer, Review, or Publisher.

Ready assets are stored in the existing durable object-storage boundary with a stable CiteLoop URL. The canonical article owns the asset. Dev.to and Hashnode reuse the canonical image where supported; other adapters either reuse, adapt, or omit it according to platform rules. Publisher uploads or rewrites references idempotently and records the final platform URL.

### Review and Failure Behavior

Review shows the image, role, alt text, caption, generation status, and failure reason alongside the draft. A reviewer can edit alt text or caption, omit an asset, or request regeneration.

Projects receive a configurable daily image count and cost budget. Budget exhaustion, missing credentials, provider errors, safety rejection, upload failure, and timeout set a clear non-blocking status. Retryable failures use bounded exponential retry; permanent failures require a configuration change or reviewer action. The article remains reviewable and publishable without the image. If upload succeeds but article publication fails, the stored asset is reused on retry.

## Error Handling and Recovery

- A failed evidence source does not erase prior evidence; it marks it stale and lowers freshness until successful refresh or expiry.
- A malformed answer or missing citations produces an uncited observation, never fabricated evidence.
- A failed weekly portfolio rebuild leaves the prior bounded portfolio active and marks discovery degraded.
- Concurrent runs use idempotency keys for evidence, observations, candidate identities, Opportunities, and image assets.
- Partial candidate generation can resume from persisted evidence without repeating successful provider calls.
- Provider cost or rate-limit exhaustion stops optional calls and records skipped work; it does not bypass scoring gates.
- Project deletion and context revocation prevent future collection while preserving required audit records under existing retention policy.

## Testing

- Context-classifier tests prove internal terms such as `AES-256-GCM` are excluded absent explicit public demand and claim authorization.
- Competitor tests prove only configured or user-confirmed competitors enter competitor research; SuperX is never special-cased.
- Portfolio tests prove the 60-prompt project cap, six-prompt cluster cap, semantic deduplication, balanced selection, least-recent rotation, and targeted-recheck accounting.
- Observation tests prove generic LLM output without citations cannot become citation evidence.
- Scoring tests cover every component, penalty, hard gate, threshold, watchlist expiry, dismissal reopening, and deterministic tiebreaker.
- Coverage and routing tests prove different intents can yield comparison pages, alternative pages, use-case pages, integration pages, templates, glossary definitions, benchmark reports, community variants, or site actions rather than always a blog post.
- Deduplication tests cover active Opportunities, drafts, published content, variants, site fixes, semantic overlap, and cannibalization.
- Funnel tests prove every input and disposition is counted and a degraded run cannot masquerade as a successful zero-result run.
- Image tests prove planning after draft, zero-image eligibility, maximum counts, provider abstraction, budget enforcement, idempotent reuse, revision behavior, alt-text edits without regeneration, deterministic benchmark charts, non-blocking failures, review preview, and multi-platform reuse.
- End-to-end tests cover evidence refresh through Opportunity creation and Opportunity through canonical draft, asset review, platform adaptation, publication, and measurement.

## Phased Rollout

### Phase 1: Stop Blind Repetition

Sanitize context, cap and rebuild prompt portfolios, implement fair rotation, distinguish cited from uncited observations, and expose the complete run funnel. Migrate existing projects by archiving prompts outside the new cap and selecting a balanced active set. Existing observations remain historical evidence but do not receive fabricated citation metadata.

### Phase 2: Growth Radar

Add normalized market and search evidence, topic maps, coverage analysis, deterministic scoring, watchlists, arbitration, and versioned multi-asset Opportunity Specs. Feed accepted specs into the existing Writer and publishing pipeline.

### Phase 3: Article Assets

Add article asset persistence, the provider and storage boundary, OpenAI image generation, Review controls, platform reuse, budgets, and failure recovery.

Each phase is independently deployable behind a per-project feature flag. Production rollout begins with the existing project in observe-only mode, compares proposed dispositions with the current system, then enables Opportunity creation after funnel and rotation acceptance pass. Phase 3 starts with a low image budget and explicit reviewer visibility.

## Production Acceptance

After the implementation PR is merged and the deployed production services contain the merge SHA:

1. The active project has no more than 60 active prompts, no cluster has more than six, and successive observer runs rotate beyond the same ten prompt IDs.
2. Internal-only terms such as `AES-256-GCM` do not appear in generated public prompts without qualifying evidence and claim authorization.
3. Competitor research contains only configured or user-confirmed competitors and does not treat SuperX as a competitor.
4. An uncited generic LLM answer is visible as uncited and cannot trigger citation-gap scoring.
5. A discovery run exposes source, candidate, duplicate, conflict, watchlist, filter, and created counts with drill-down reasons, including when zero Opportunities are created.
6. Evidence-backed test cases route to at least three distinct existing asset types and preserve their intended canonical and secondary publication surfaces.
7. Repeated evidence does not create duplicate Opportunities or reopen a dismissal without materially new evidence.
8. A drafted article can show a ready generated image with alt text and caption in Review, reuse it on compatible platform variants, and retain its published stable URL.
9. Missing image credentials and a forced provider failure are clearly reported but do not block article review or text-only publication.
10. Scheduler, API, database, Writer, Review, and Publisher paths complete without unhandled production errors, and observed provider cost stays within the configured project budgets.
