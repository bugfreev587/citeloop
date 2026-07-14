# Platform Content Contracts Design

## Problem

CiteLoop currently selects `blog`, `syndication`, or `both` at Content Plan time. Writer always creates a canonical article and, for syndication topics, generates a fixed set of Dev.to, Hashnode, and Reddit variants. Each persisted variant has a platform, but the upstream Opportunity does not select exact targets and the platform instruction primarily changes the platform name plus canonical-versus-source-link behavior.

That is insufficient. Platforms differ in output shape, metadata, length, markup, discovery mechanisms, community expectations, image support, canonical handling, and publishing workflow. Reddit also varies by subreddit. Hacker News is normally a link submission, not another copy of a long-form article. A generic rewrite cannot reliably produce native, valid content.

This module makes exact target selection and platform-native generation explicit. It is independently implementable from Growth Radar and from future automatic publishing connectors.

## Goals

- Every Opportunity that creates content selects its exact canonical target and exact target platforms before generation.
- Every generated platform artifact pins an immutable Platform Content Contract version.
- Contracts define output schema, hard limits, metadata, formatting, canonical/source rules, media capabilities, quality rubric, and deterministic validators.
- Asset-type and platform compatibility is checked before work enters Writer.
- Writer produces a separate native artifact per target; it never simulates support by replacing the platform name in a generic prompt.
- Review exposes the target, contract version, validation results, and platform-specific preview.
- Publishing capability remains separate from content-generation capability.
- Platform rule changes create a new contract version without silently changing existing drafts.

## Non-Goals

- This module does not add automatic APIs for Medium, LinkedIn, Reddit, or Hacker News.
- It does not make every asset type compatible with every platform.
- It does not bypass subreddit, publication, account, moderation, anti-spam, or provider rules.
- It does not duplicate the canonical factual source for each target. Platform artifacts retain canonical provenance and measurement linkage.
- It does not require Growth Radar to understand platform prompt details. Growth Radar consumes only the contract capability projection.

## Current Baseline

The existing registry names `blog`, `dev_to`, `hashnode`, `medium`, `linkedin`, `reddit`, and `hacker_news`. Blog canonical publication is automated through GitHub/Next.js. External destinations use a semi-manual compose flow. The default Writer target list is Dev.to, Hashnode, and Reddit; Medium, LinkedIn, and Hacker News are registered but are not in the default generation list.

The existing article model already distinguishes `canonical` from `syndication_variant` and stores `platform` on variants. This design preserves those identities while replacing broad channel inference and the fixed target list with explicit target records and pinned contracts.

## Domain Model

### Opportunity Targets

A content-producing Opportunity carries:

- `canonical_target`: the owned source destination required for the factual master copy;
- `target_platforms`: an ordered set of exact external or owned targets;
- `asset_type`;
- `target_rationale` per target;
- `contract_capability_snapshot`, recording why the combination was eligible at selection time.

`blog`, `syndication`, and `both` remain derived UI summaries for backward compatibility. They are not the source of truth:

- only canonical target selected → `blog`;
- canonical target plus one or more external targets → `both`;
- a legacy syndication request still resolves to a canonical source plus explicit external targets because V1 external publishing requires a verified source URL.

An Opportunity cannot enter generation with an empty canonical target, an unknown target, duplicate targets, or an incompatible asset-type/target pair.

### Content Artifacts

One accepted Opportunity creates:

- one canonical article artifact; and
- zero or more platform artifacts, one for each exact target.

Each artifact records:

- `target_platform`;
- `platform_contract_id` and immutable version;
- `asset_type` using one canonical taxonomy;
- `output_type` such as `long_form_article`, `community_post`, or `link_submission`;
- `source_article_id` for non-canonical artifacts;
- structured platform metadata;
- deterministic and semantic validation results.

The article's existing `platform` remains the materialized target for compatibility. The new target record is the planning and audit source of truth.

## Canonical Asset-Type Taxonomy

The module uses the registry names everywhere:

- `blog_post`;
- `comparison_page`;
- `alternative_page`;
- `use_case_page`;
- `integration_page`;
- `template_or_checklist`;
- `glossary_definition`;
- `benchmark_report`;
- `source_backed_evidence_page`;
- `faq_answer_block`.

Writer aliases such as `template_checklist` and `integration_docs_page` are migrated to these canonical keys. `source_backed_evidence_page` is already emitted by the GEO analyzer and handled by Writer despite being absent from `seo_asset_types`; it is therefore an existing unregistered asset type, not a future-only type. `faq_answer_block` is already recognized by Writer but is also absent from the registry. Both keys are added to the registry, and migration covers existing `source_backed_evidence_page` briefs and derived records before enforcing registry membership. Direct actions such as metadata, schema, internal-link, sitemap, and technical patches are owned-site actions and never become external platform articles.

Opportunity-to-Topic handoff must persist `asset_type` explicitly. It must not encode the type indirectly in `angle`, `format`, expected impact, or free text.

## Platform Contract Registry

`platform_content_contracts` stores immutable versions with:

- platform key and semantic version;
- lifecycle status: `draft`, `active`, `deprecated`, or `retired`;
- official source URLs and source retrieval dates;
- effective and review-due timestamps;
- generation capability and publishing mode;
- allowed output types and compatible asset types;
- required context and metadata fields;
- markup and media capabilities;
- canonical and source-link policy;
- hard limits and deterministic rule configuration;
- prompt instruction template;
- semantic quality rubric;
- preview renderer key;
- migration notes and superseding contract ID.

Only one active version exists per platform. Activating a contract requires passing its fixture suite. Existing artifacts stay pinned to their original version. Regeneration uses the pinned version unless the user explicitly upgrades the artifact, in which case CiteLoop creates a new revision and shows the contract diff.

Contracts are reviewed at least every 90 days. In this scope, change detection is an explicit administrative process, not an automated promise: a scheduled job only marks contracts `review_due`; an administrator follows the stored official source URLs, records the retrieval date, creates a draft contract version, runs fixtures, and activates it. A user-reported platform rejection can also open a review task. Automated official-page diffing may be added later but is not required for acceptance. An overdue contract remains usable for existing work but is marked `review_due`; administrators can pause new generation without breaking drafts.

## Target Context and Reddit Rule Versions

Platform-level rules and target-specific context have different lifecycles. Immutable `platform_target_contexts` revisions store:

- `id`, `project_id`, `platform`, and normalized `target_key`, such as `r/saas`;
- monotonically increasing version and status: `draft`, `confirmed`, `expired`, or `superseded`;
- `source_kind`: `user_pasted_rules`, `user_confirmed_rules`, or a future approved provider;
- official target URL and optional rules URL;
- verbatim user-supplied rules text plus normalized allowed post types, required flair, link policy, self-promotion policy, disclosure requirements, and notes;
- content hash, `confirmed_by`, `confirmed_at`, and `expires_at`;
- optional `supersedes_context_id` and timestamps.

Only one confirmed, unexpired revision exists per project, platform, and target key. Opportunity targets and generated artifacts pin `target_context_id` and version alongside the platform contract version.

Reddit V1 is manual and does not require Reddit API access. In Settings or the target picker, the user selects a subreddit, opens the provided official rules link, pastes the current rules or confirms the displayed manually entered rules, completes the structured link/flair/self-promotion fields, and checks `I verified these community rules`. Confirmation creates an immutable revision valid for 30 days. The user may reconfirm an unchanged hash to create a new time-bounded revision or edit the rules to create a superseding revision. CiteLoop never represents manually entered rules as API-fetched.

Missing, draft, expired, or contradictory Reddit context blocks new Reddit generation before an LLM call and provides the setup action. Expiry does not mutate an already generated artifact or its pinned rules; it blocks regeneration and warns before manual submission until the user reconfirms. Automated Reddit OAuth or rules ingestion is outside this scope.

## Contract Resolution

Writer resolves constraints in this order:

1. safety, legal, and platform hard rules;
2. target-specific context such as subreddit or Hashnode publication;
3. active Platform Content Contract;
4. asset-type structure contract;
5. project facts, banned claims, voice, and style;
6. Opportunity brief and evidence.

Higher-priority constraints cannot be overridden by lower-priority instructions. A conflict is reported before model invocation when deterministic resolution is possible. For example, a platform that only accepts link submissions cannot receive a benchmark-report body, and a subreddit that forbids promotional links cannot be selected for a promotional use-case post.

The resolved generation request is persisted with hashes for the contract, asset contract, project context, and source evidence. Raw secrets and private context are excluded.

## Supported Platform Contracts

The initial contract baselines are grounded in current official documentation: [Forem API](https://developers.forem.com/api/v1), [Hashnode Public API](https://apidocs.hashnode.com/), [Medium canonical links](https://help.medium.com/hc/en-us/articles/360033930293-Set-a-canonical-link), [Medium images](https://help.medium.com/hc/en-us/articles/215679797-Using-images), [LinkedIn Articles](https://www.linkedin.com/help/linkedin/answer/a522427/publish-articles-on-linkedin), [Reddit posting guidance](https://support.reddithelp.com/hc/en-us/articles/360060422572-How-do-I-post-and-comment-on-Reddit), [Reddit Responsible Builder Policy](https://support.reddithelp.com/hc/en-us/articles/42728983564564-Responsible-Builder-Policy), and [Hacker News Guidelines](https://news.ycombinator.com/newsguidelines.html). These URLs and their retrieval dates are stored with the activated contract version.

### Owned Blog / GitHub-Next.js

- Output: canonical long-form MDX article.
- Requires title, slug, description, H1, target query, supported claims, and canonical URL resolution.
- Allows the complete content-producing asset taxonomy when supported by the owned-site renderer.
- Validates MDX syntax, front matter, internal links, image references, claim evidence, and publish-path safety.
- Publishing capability: automatic when the configured GitHub/Next.js connection is healthy.

### Dev.to / Forem

- Output: Markdown article with title, body, tags, optional series/organization context, cover image, and canonical URL.
- Uses the current Forem API/article schema as the contract source even while publication remains semi-manual.
- Validates supported Markdown, tag count and format from the active contract, required canonical provenance for republished content, and absence of owned-site-only MDX components.
- Publishing capability in this scope: semi-manual. A later connector can consume the same structured artifact.

### Hashnode

- Output: publication-scoped Markdown article with title, slug, tags, canonical URL, cover image, and optional subtitle/series fields supported by the active API.
- Requires a selected Hashnode publication context before generation or marks the artifact `needs_target_context`.
- Validates the current GraphQL publish-post input shape, tag identifiers, Markdown portability, canonical provenance, and publication capability.
- Publishing capability in this scope: semi-manual.

### Medium

- Output: Medium story package with title, optional subtitle, topics, body, preview/featured image, alt text, and canonical URL.
- Treats cross-posted content as republished source material and requires canonical provenance.
- Validates Medium-compatible formatting and images; owned-site MDX, unsupported embeds, and platform-specific widgets are removed or adapted.
- Publishing capability in this scope: semi-manual/import-assisted.

### LinkedIn Articles

- Output: professional long-form article with title, body, optional cover image, SEO title, SEO description, article URL suggestion, and feed commentary.
- Distinguishes an article from a short feed post or newsletter edition; those become separate output types only after contracts are added.
- Validates professional framing, supported rich elements, image metadata, and absence of unsupported Markdown assumptions.
- Publishing capability in this scope: semi-manual.

### Reddit

- Output: subreddit-specific community post, not a generic syndication copy.
- Requires exact subreddit and a confirmed, unexpired `platform_target_contexts` revision containing allowed post type, current community rules, required flair when applicable, and confirmation timestamp.
- Produces title, body or link submission, flair suggestion, disclosure/source treatment, and reviewer notes.
- Enforces discussion-first framing, avoids duplicate mass posting, strips generic marketing calls to action, and respects community-specific link and self-promotion rules.
- If rules are missing, stale, contradictory, or prohibit the intended content, generation is blocked or rerouted; CiteLoop does not guess.
- Publishing capability in this scope: manual submission only.

### Hacker News

- Output: `link_submission`, normally an original-source URL plus a non-editorialized title; it is not a rewritten long-form article.
- Requires a published canonical source URL before the submission becomes ready.
- Validates topic relevance, original source, title hygiene, removal of site-name decoration and promotional adjectives, and absence of voting solicitation.
- CiteLoop does not generate HN comments. The official HN guidelines' restrictions on generated or AI-edited comments are treated as a hard prohibition.
- Publishing capability in this scope: manual submission only.

## Capability Matrix

The backend exposes a computed matrix for every platform, asset type, output type, and project:

- `generation_supported`;
- `target_context_ready`;
- `connection_ready`;
- `publish_mode`;
- `canonical_required`;
- `source_url_required_before_publish`;
- `image_roles_supported`;
- `block_reasons`;
- active contract version and review state.

Growth Radar and Content Plan consume this matrix. A platform can be generation-supported but not auto-publishable. UI must not present those states as equivalent.

## Selection Experience

Opportunity displays a recommended target set derived from intent, audience, asset type, evidence, project configuration, and the capability matrix. Before generation, Content Plan shows:

- canonical target;
- exact target platforms;
- output type per target;
- recommendation rationale;
- readiness and publishing mode;
- missing context or incompatibility reasons.

The user can add or remove eligible targets. Changing targets before generation updates the target plan. Adding a target after generation creates only the missing artifact; removing a target archives its unapproved artifact and never deletes published work.

The old three-way `Blog / Syndication / Both` control becomes a summary/filter, not the editor for target selection.

## Generation and Validation Pipeline

For each exact target:

1. load the pinned Platform Content Contract;
2. verify target context and asset compatibility;
3. resolve project, asset, platform, and evidence constraints;
4. create the target-specific structured generation request;
5. generate the target artifact independently;
6. parse it into the contract's output schema;
7. run deterministic validators;
8. run the platform semantic rubric and claim QA;
9. attempt at most two bounded repairs using concrete validation failures;
10. persist the artifact and complete validation report for Review.

A failure for one optional target does not discard successful artifacts. The canonical failure blocks dependent external readiness because source provenance cannot be established. A target-specific failure can be retried independently without regenerating the canonical or other targets.

## Deterministic Validation

Every contract must validate, where applicable:

- required fields and output type;
- current length, count, and character constraints;
- title and slug rules;
- supported markup and removal of incompatible MDX;
- tags, topics, publication, subreddit, flair, and other target context;
- canonical/source URL policy;
- image type, size, alt text, caption, and role compatibility;
- link count and prohibited link patterns;
- prohibited promotional or voting-solicitation phrases;
- platform-specific metadata schema;
- absence of unresolved placeholders.

Semantic QA evaluates native fit, factual support, audience match, non-duplication, and compliance with community expectations. Semantic QA cannot waive a deterministic failure.

## Review and Preview

Review groups artifacts under their canonical source and shows:

- target platform and output type;
- pinned contract version and review date;
- platform-native preview;
- deterministic failures and semantic warnings;
- canonical/source relationship;
- publication mode and readiness;
- missing target context;
- contract upgrade availability.

Approval is per artifact. Approving the canonical does not automatically approve platform variants. External artifacts remain locked until the canonical URL is verified when their contracts require a source URL.

## Publishing Boundary

The Platform Content Contract answers “is this artifact valid and native for the target?” The Publisher answers “can CiteLoop deliver it now?”

- Blog remains automatic through the configured GitHub/Next.js publisher.
- Dev.to, Hashnode, Medium, and LinkedIn remain semi-manual in this implementation.
- Reddit and Hacker News remain manual submissions.
- Future API publishers consume the same validated structured artifact and must report platform-side validation failures without changing the contract silently.

## Migration

- Existing canonical articles are marked with the legacy blog contract version.
- Existing Dev.to, Hashnode, and Reddit variants are marked `legacy-v1` and are not claimed as contract-valid until regenerated or explicitly validated.
- Existing `topic.channel` is converted into a target plan: `blog` selects only the canonical target; `syndication` and `both` select the canonical target plus only the variants that already exist or the current project default for new, undrafted topics.
- The fixed `platform.SyndicationTargets` list stops being Writer's source of truth after migration.
- Asset-type aliases are normalized without changing historical display metadata.
- Add registry entries for `source_backed_evidence_page` and `faq_answer_block` before adding any registry foreign key or validation gate. Backfill existing `geo_asset_briefs.asset_type = 'source_backed_evidence_page'`, linked Content Actions, explicit Topic asset-type fields, and `articles.seo_meta.asset_type` without rewriting historical evidence, angle, format, or article bodies. Records already using the canonical key retain it; missing derived fields are populated from the linked GEO brief. Conflicting non-empty values are quarantined for migration review rather than overwritten.
- No Reddit target is inferred from a legacy generic Reddit variant because historical rows do not identify a subreddit or rules revision. Those artifacts remain `legacy-v1`; selecting Reddit for new generation requires the manual target-context setup.

## Error Handling

- Missing or retired contract: block new generation and preserve existing artifacts.
- Stale community context: block Reddit generation until refreshed.
- Missing publication/account context: mark the target `needs_target_context` without invoking the model.
- Contract and asset incompatibility: reject target selection with a stable reason code.
- Parse or validation failure: bounded repair, then route only that artifact to Review with failures.
- Contract update during generation: finish against the pinned version; never mix versions in one artifact.
- Platform-side rejection: retain the artifact and rejection detail, then require contract review or target-specific repair.

## Testing

- Registry tests enforce one active immutable contract per platform and safe version transitions.
- Capability tests cover every asset-type/platform/output-type combination and publishing mode.
- Handoff tests prove Opportunity target selection survives Content Action and Topic creation without being reduced to `blog/syndication/both`.
- Writer tests prove exact target lists replace the fixed default and every artifact pins a contract.
- Platform fixture suites test valid and invalid Blog, Dev.to, Hashnode, Medium, LinkedIn, Reddit, and Hacker News outputs.
- Reddit tests cover missing/stale subreddit rules, prohibited promotion, flair requirements, and duplicate mass posting.
- Target-context tests cover immutable versions, one active revision, 30-day expiry, unchanged reconfirmation, supersession, project isolation, manual-source labeling, and pinned-artifact behavior.
- Hacker News tests prove it produces a link-submission package, uses the original source and title policy, and never creates comments.
- Alias tests prove registry and Writer use the same canonical asset-type keys.
- Review tests expose platform preview, validation, version, readiness, and per-artifact approval.
- Migration tests preserve existing drafts and prevent legacy artifacts from being mislabeled as contract-valid.

## Independent Delivery Plan

1. **Contract foundation:** schema, registry, capability matrix, canonical asset keys, immutable versions, and backend API.
2. **Exact target planning:** Opportunity fields, Content Plan target selector, handoff persistence, and migration from broad channels.
3. **Native generation:** contract resolver, output schemas, deterministic validators, repair loop, and removal of the fixed target list.
4. **Review integration:** platform previews, validation reports, per-artifact approval, and readiness states.
5. **Publishing adapters later:** optional automatic connectors built independently on validated artifacts.

This sequence can ship independently of Growth Radar. Growth Radar integrates through the capability matrix and exact target fields once phases 1 and 2 are available.

## Production Acceptance

1. An Opportunity cannot start generation until it has one canonical target and an explicit, compatible target-platform list.
2. Choosing Dev.to and Reddit creates exactly one canonical, one Dev.to artifact, and one subreddit-specific Reddit artifact; it does not create Hashnode implicitly.
3. Every artifact records an immutable platform contract version and canonical asset type.
4. The same source produces structurally different native outputs for Dev.to, LinkedIn, Reddit, and Hacker News; changing the platform name alone cannot satisfy fixtures.
5. Hacker News produces only a compliant link-submission package and never a rewritten article or generated comment.
6. Reddit generation is blocked without a user-confirmed, unexpired subreddit rule revision; the UI provides the manual paste/confirm setup path and stores the pinned revision on the artifact.
7. Registry types `template_or_checklist` and `integration_page` reach their matching Writer structure contracts without aliases or free-text inference.
8. A deterministic platform violation triggers repair or Review and cannot be waived by model scoring.
9. Existing drafts remain readable and publishable under legacy behavior, while new artifacts use exact target plans.
10. Blog automatic publishing and all current semi-manual compose flows continue to work without regression.
11. Existing `source_backed_evidence_page` GEO briefs and derived work are registered and backfilled without loss, duplication, or conversion to a generic blog type.
