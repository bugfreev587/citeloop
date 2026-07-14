# Stage-Aware Opportunity Discovery Design

## Status

Approved product design for implementation planning. This specification extends the [Opportunity Discovery Growth Radar design](./2026-07-13-opportunity-discovery-growth-radar-design.md) and keeps the [Platform Content Contracts design](./2026-07-13-platform-content-contracts-design.md) authoritative for platform-native generation.

## Problem

CiteLoop currently applies one deterministic Opportunity score to every project. That assumes all projects have comparable content inventory, Search Console history, answer-engine observations, platform coverage, and conversion evidence. They do not.

An early project needs foundational coverage and citable owned assets before historical demand can exist. A project with traction should amplify observed SEO and GEO demand. A scaling project should extend proven themes across high-value intents, asset types, and compatible platforms. A mature project should usually improve or refresh existing assets rather than continue creating foundational pages. Applying one score and one evidence gate to all four situations systematically suppresses useful early-stage work and overproduces basic content for mature projects.

Production inspection for UniPost on July 14, 2026 demonstrated the failure mode. The latest run generated 18 raw candidates, persisted 12 unique candidates, and created zero Opportunities. Six candidates concerned the internal term `AES-256-GCM` and were correctly filtered only after consuming discovery capacity. The six relevant candidates scored 43–45 even though their capabilities were confirmed: all had zero demand and zero reuse potential. Search Console was connected and contained 903 rows, but the demand query required an exact match to a complete GEO prompt. All supported platform contracts were active, but Growth Radar selected only the canonical blog and did not populate compatible external targets or additional output types.

The immediate problem is therefore not merely a score threshold. Candidate inputs, GEO demand, target resolution, evidence qualification, and stage-specific review policy must work together.

## Product Decision

Version 1 uses a manually selected project-level growth stage:

- `foundation`: establish topic coverage, citable assets, and platform foundations;
- `traction`: amplify observed SEO and GEO demand;
- `scale`: expand high-value topics, platforms, and content types;
- `optimize`: refresh declining assets and address competitive or conversion gaps.

The stage selector appears in the upper-right corner of the Opportunity page. Existing projects without an explicit selection use Foundation and display an unconfirmed-default notice. Automatic stage inference, automatic switching, and topic-cluster-level stages are explicitly deferred.

## Goals

- Make Opportunity discovery appropriate to the project's selected growth stage.
- Give each stage different candidate priorities, deterministic weights, evidence gates, review standards, and disposition thresholds.
- Allow early projects to create justified foundational Opportunities without requiring nonexistent GSC history.
- Treat SEO and GEO as independent, deterministic demand lanes.
- Remove internal or sensitive terms before public candidate generation.
- Resolve real project platform targets before awarding reuse or external-coverage points.
- Preserve deterministic replay, evidence provenance, deduplication, and global safety gates.
- Explain why each candidate became an Opportunity, watchlist item, hold, or filter result.
- Allow a stage change to rescore active watchlist items without silently changing accepted or in-progress work.

## Non-Goals

- Automatic project-stage recommendation or switching.
- Topic-cluster-specific stages.
- LLM-selected weights, thresholds, evidence classes, or dispositions.
- Lowering quality controls to satisfy an Opportunity-production quota.
- Rebuilding Writer, Review, Publisher, measurement, or Platform Content Contracts.
- Replanning accepted, rejected, drafted, published, or measuring work after a stage change.
- Treating repeated calls to the same answer provider as independent confirmation.

## Ownership and Boundaries

Growth Radar continues to own evidence normalization, public-context sanitization, topic and coverage analysis, candidate generation, deterministic scoring, policy gates, deduplication, arbitration, and Opportunity creation.

The selected stage changes what Growth Radar seeks and how it reviews a candidate. It does not change the downstream workflow. Writer still creates platform-native content under the pinned Platform Content Contract. Review, Publisher, measurement, and learning retain their current ownership.

Global safety, claim-support, duplicate, dismissal, and canonical-conflict rules remain authoritative across every stage. A stage profile may be stricter than the global rules but cannot bypass them.

## User Experience

### Opportunity Page Selector

The upper-right corner of the Opportunity page shows:

```text
Growth Stage: Foundation ▾
```

The dropdown contains all four stages with a one-sentence explanation:

- Foundation — Build essential topic coverage and citable owned assets.
- Traction — Act on emerging SEO and GEO demand.
- Scale — Expand proven themes across high-value content and platforms.
- Optimize — Refresh declining assets and respond to competitive change.

The currently selected stage is visible without opening the dropdown. The selector is project-scoped, not user-scoped.

### Existing-Project Default

An existing project with no stored stage behaves as Foundation and shows `Default stage — confirm selection`. This default never blocks scheduled discovery. Selecting any stage, including Foundation, records an explicit choice and removes the notice.

### Change Confirmation

Changing the stage opens a confirmation dialog that shows:

- current and proposed stage;
- the new stage's primary discovery focus;
- the number of active watchlist candidates scheduled for rescoring;
- a statement that accepted and in-progress Opportunities will not change.

On confirmation, the UI saves the new stage and immediately reflects it. Watchlist rescoring runs asynchronously. The page shows progress, completion, or a retryable failure state. A rescore failure does not revert the selected stage.

### Permissions and Audit

The stage-changing endpoint uses the repository's existing project-management authorization boundary. Every explicit selection records actor, old stage, new stage, stage-profile versions, timestamp, optional user reason, affected-watchlist count, and rescore status.

## Stage-Aware Architecture

The stage is pinned at discovery-run start and flows through the complete candidate path:

```text
Project stage snapshot
  -> stage candidate-source policy
  -> public-context sanitization
  -> evidence and coverage refresh
  -> candidate generation
  -> platform target resolution
  -> canonical raw-signal calculation
  -> stage weight profile
  -> stage evidence gates
  -> global policy gates
  -> dedupe and arbitration
  -> Opportunity / watchlist / hold / filtered
```

Candidate generation must be stage-aware. The system must not generate one undifferentiated candidate set and merely change its final score. Each stage prioritizes different gaps and action types before scoring.

The score implementation remains shared. It calculates canonical raw components, applies a versioned stage profile, and records every input and decision. Four independent scoring code paths are prohibited.

## Versioned Stage Profiles

Each stage profile contains:

- immutable stage key and profile version;
- canonical component weights;
- Opportunity and watchlist thresholds;
- candidate-source priorities;
- stage evidence combinations;
- stage-specific gates;
- preferred and allowed actions;
- asset and platform requirements.

Profiles ship as versioned application configuration and tests, not administrator-editable database rows in V1. Updating a profile requires a new profile version. Historical snapshots remain pinned to their original version.

### Deterministic Weighting

Canonical raw components retain fixed maxima:

| Component | Canonical maximum |
|---|---:|
| Observed Demand | 25 |
| Coverage Gap | 20 |
| Product and Audience Relevance | 15 |
| Commercial or Growth Value | 15 |
| Freshness or Change | 10 |
| Cross-Platform Reuse | 10 |
| Evidence Quality | 5 |

For each component, the stage contribution is:

```text
floor(canonical_raw_points * stage_weight / canonical_maximum)
```

The positive score is the sum of all seven stage contributions and has a maximum of 100. Existing deterministic penalties are applied afterward. Global and stage gates then assign the final disposition. Integer flooring is part of the profile contract and snapshot replay.

### Stage Weights and Thresholds

| Component | Foundation | Traction | Scale | Optimize |
|---|---:|---:|---:|---:|
| Observed Demand | 10 | 25 | 20 | 20 |
| Coverage Gap | 30 | 20 | 15 | 10 |
| Relevance | 20 | 15 | 10 | 10 |
| Commercial Value | 10 | 15 | 20 | 20 |
| Freshness / Change | 10 | 10 | 10 | 25 |
| Reuse Potential | 10 | 10 | 20 | 5 |
| Evidence Quality | 10 | 5 | 5 | 10 |
| Opportunity threshold | 70 | 75 | 78 | 75 |
| Watchlist lower bound | 60 | 60 | 65 | 60 |

A score below the stage's watchlist lower bound is filtered unless another global disposition such as hold, merge, or arbitration applies.

## Canonical Raw Signals

### Observed Demand: 0–25

SEO and GEO are independent lanes. A candidate may have useful SEO demand, useful GEO demand, or both.

#### SEO Demand: 0–15

Search Console demand maps normalized cluster queries rather than requiring equality with the complete GEO prompt. The mapping is deterministic and comes from persisted topic-cluster membership, normalized prompt/query identities, or an explicit user-confirmed alias. LLM semantic similarity cannot create a mapping or award points.

- trailing-28-day impressions across mapped cluster queries: 0 → 0; 1–9 → 2; 10–49 → 4; 50–199 → 6; 200–999 → 8; 1,000 or more → 10;
- change from the preceding 28 days: prior under 10 and current 1–9 → 1; prior under 10 and current at least 10 → 5; otherwise at or below −25% → 0; above −25% through 0% → 1; above 0% through 25% → 2; above 25% through 100% → 4; above 100% → 5.

#### GEO Demand: 0–10

Only non-synthetic observations with provider, model, normalized prompt identity, observed time, and answer or result hash qualify.

- distinct independent answer providers confirming the same normalized gap in the trailing 30 days: one → 2; two → 5; three or more → 7;
- recurrence on distinct UTC observation dates in the trailing 30 days: one date → 0; two dates → 1; three or four dates → 2; five or more dates → 3.

The GEO lane is capped at ten. Repeated runs against the same provider on the same date do not add provider or recurrence points. Multiple model names routed to the same underlying provider count as one provider unless the provider contract establishes independent evidence collection.

An uncited answer may establish a project-absence observation if its provenance is complete. It cannot establish that a URL was cited, prove a competitor citation, or support a factual product claim. Competitor-citation candidates require actual structured citation or URL evidence.

### Coverage Gap: 0–20

The existing canonical calculation remains:

- matching primary-intent asset absent → 12; only stale or failed coverage → 6; current matching coverage → 0;
- no relevant owned internal-link path → 4; one weak or stale path → 2; current paths → 0;
- no compatible artifact on any selected external target → 4; partial target coverage → 2; complete target coverage → 0.

Coverage compares published, planned, accepted, and active work before recommending net-new content.

### Relevance: 0–15

- topic maps to a confirmed public capability, customer problem, or use case → 8;
- audience maps to a confirmed ICP or persona → 4;
- intent maps deterministically to a supported asset taxonomy → 3.

Capability mapping uses normalized aliases and explicit context fields, not arbitrary substring matches alone. An unconfirmed product capability remains a hold rather than receiving relevance points.

### Commercial Value: 0–15

The existing deterministic intent, journey-stage, and configured-conversion mappings remain authoritative. Model prose cannot award commercial points.

### Freshness or Change: 0–10

The existing evidence-age and material-change buckets remain authoritative. GEO provider consensus becoming newly satisfied counts as `new_confirmation`. Recollecting an unchanged observation from the same provider does not constitute material change.

### Cross-Platform Reuse: 0–10

- two points per compatible selected external target, capped at six;
- two points per additional compatible output type beyond the canonical long-form output, capped at four.

Growth Radar must resolve exact targets from project target context plus active Platform Content Contracts before scoring. Merely having a global active contract does not make a platform selected or compatible for the project. `SelectedExternalTargets`, `CompatibleExternalTargets`, `AdditionalOutputTypes`, and `CoveredExternalTargets` must be populated from the resolved plan. A blog-only target receives zero external-target points.

### Evidence Quality: 0–5

The existing source-class diversity, first-party, and complete-provenance calculation remains, with one clarification: an uncited but completely captured answer-engine observation may be a qualified source for an absence claim. It remains unqualified for a citation claim. The snapshot stores the supported claim type for each evidence record.

## Stage Candidate and Review Policies

### Gate Precedence

Disposition gates execute in this fixed order after score arithmetic:

1. exact duplicate → merge evidence into canonical work;
2. sensitive, internal, off-product, or unsupported claim → filter;
3. dismissed identity without material new evidence → remain dismissed;
4. unconfirmed capability → hold;
5. semantic near-duplicate → record the diagnostic penalty and filter as near-duplicate;
6. unresolved canonical conflict or cannibalization → arbitration;
7. missing user configuration required by the selected stage → hold;
8. stage evidence gate not yet satisfied → watchlist only when at least one qualified source exists and the score meets the stage's watchlist lower bound, otherwise filter;
9. stage numeric Opportunity and watchlist thresholds.

A later gate cannot weaken an earlier gate. In particular, no stage score can override sensitive context, an unsupported claim, an exact duplicate, or an unconfirmed capability.

### Foundation

Foundation establishes essential topic coverage, citable owned sources, and initial platform foundations.

Prioritized candidates include:

- confirmed core capabilities without a canonical owned page;
- important ICP, use-case, integration, comparison, and alternative gaps;
- existing content missing evidence blocks, FAQ structure, schema, or internal discovery paths;
- important topics without an appropriate native external artifact;
- project absence in answer engines for a confirmed, relevant capability.

Foundation does not require Search Console history. A candidate may pass its evidence gate with one of these deterministic combinations:

- verified owned-site inventory plus confirmed Project Context;
- two independent answer providers confirming the same absence gap;
- one answer provider plus independent Brave or first-party evidence;
- first-party Search Console demand plus owned-site coverage evidence.

A lone answer-provider observation enters the watchlist even when its numeric score reaches 70. Foundation prefers canonical pages, use-case pages, integration pages, comparison pages, glossary or FAQ assets, evidence blocks, schema, and internal links. It does not treat daily article volume as success.

### Traction

Traction amplifies demand that is already appearing in search or answer engines.

Prioritized candidates include:

- clusters with new or growing Search Console demand and weak coverage;
- repeated project absence or competitor citation across answer providers;
- pages with impressions but weak click, rank, citation, or answer inclusion;
- watchlist candidates that acquired independent confirmation;
- comparison, alternative, how-to, use-case, and integration gaps with observed demand.

The candidate must have either positive SEO Demand or at least two independent GEO providers. It must also have two qualified evidence records; they may belong to the same source class only when they are independent answer providers with distinct observation hashes. A single uncorroborated answer observation cannot create an Opportunity.

### Scale

Scale expands proven themes across commercially valuable intents, asset types, audiences, and platforms.

Prioritized candidates include:

- proven clusters with uncovered ICP, intent, or journey-stage extensions;
- high-value canonical assets missing compatible platform-native variants;
- comparison, integration, template, benchmark, and source-backed assets with reuse value;
- topic clusters that can form a coherent internal-link and distribution network.

Scale requires:

- positive SEO or GEO Demand;
- a persisted success signal in the cluster, defined as a published asset with positive Search Console clicks or impressions, a qualified answer-engine citation, or an approved project conversion signal inside its measurement window;
- at least one resolved compatible external target or one additional compatible output type beyond the canonical asset.

Content that only changes a platform name is a duplicate, not a Scale Opportunity. Each target must receive a contract-native artifact plan.

### Optimize

Optimize improves existing assets in response to decline, staleness, competition, or conversion gaps.

Prioritized candidates include:

- material declines in impressions, clicks, position, answer citation, or configured conversion;
- newly observed competitor displacement;
- stale evidence, broken source links, outdated claims, or changed platform requirements;
- correct-intent pages with weak structure, evidence, CTA, schema, or internal linking.

Optimize requires either first-party material change or two independent external change sources. An unchanged old candidate cannot reopen from repeated scanning. Refresh, evidence-block, metadata, schema, internal-link, and sitemap actions are preferred. A new canonical asset is allowed only when deterministic coverage analysis proves that the existing asset cannot serve the distinct intent or audience.

## Context Sanitization Before Candidate Generation

Every topic, prompt, query expansion, and image brief must consume the accepted public vocabulary emitted by context classification. Terms classified as internal or sensitive must be removed before portfolio construction and candidate generation, not generated and filtered later.

`AES-256-GCM`, credentials, private keys, database implementation details, deployment internals, and similar terms require both independently observed public demand and explicit authorization to make the public claim before they can enter the accepted vocabulary. A key term in Project Context alone is insufficient.

Run diagnostics record rejected terms and reasons. The prompt portfolio must contain zero rejected terms after a successful rebuild.

## Evidence and Reason Codes

Every candidate persists structured decisions rather than only a final disposition string. Required reason-code families include:

- `context.internal_sensitive`
- `context.capability_unconfirmed`
- `demand.no_qualified_signal`
- `demand.single_geo_provider`
- `evidence.insufficient_independent_sources`
- `evidence.citation_claim_without_citation`
- `target.no_project_target`
- `target.no_compatible_contract`
- `coverage.existing_work`
- `duplicate.exact`
- `duplicate.near`
- `conflict.canonical`
- `stage.foundation_gate`
- `stage.traction_gate`
- `stage.scale_gate`
- `stage.optimize_gate`

Each reason stores the relevant input identities and a safe user-facing explanation. The Opportunity page can therefore explain what was scanned, what failed, and what action would make a held or watchlisted candidate eligible.

## Data Model

### Current Stage

A project-stage setting stores:

- project identity;
- stage enum;
- pinned stage-profile version;
- monotonically increasing setting version;
- whether the value is the unconfirmed Foundation default;
- selecting actor and selected time;
- created and updated time.

There is one current row per project. Reads for a missing row return the virtual unconfirmed Foundation default without requiring a blocking migration backfill.

### Stage Events

An append-only event stores:

- event and project identity;
- previous and new stage;
- previous and new profile version;
- expected and committed setting version;
- actor and optional reason;
- affected active-watchlist count;
- rescore status: `pending`, `running`, `complete`, or `failed`;
- sanitized failure code and detail;
- created, started, and completed time.

The event is the idempotency boundary for watchlist rescoring.

### Scoring Snapshot

Every new or rescored candidate snapshot adds:

- project stage and setting version;
- stage-profile version and canonical formula version;
- raw component inputs and canonical raw points;
- stage weights and weighted contributions;
- SEO and GEO demand breakdowns;
- evidence identities, supported claim types, and qualification decisions;
- resolved platform targets and pinned contract versions;
- stage and global gate decisions;
- penalty arithmetic and final disposition.

Changing LLM text alone cannot change a score or evidence identity.

## API Contract

The project API exposes:

- a read operation returning the current or virtual-default stage, profile description, setting version, and unconfirmed flag;
- an authorized update accepting stage, expected setting version, and optional reason;
- a stage-profile list for the four stable UI options and descriptions;
- rescore status associated with the committed stage-change event.

Updates use optimistic concurrency. A stale expected version returns a conflict with the latest setting. Invalid stage keys are rejected. The API never accepts client-supplied weights, thresholds, or profile versions.

## Switching and Concurrency

- A discovery run pins stage, setting version, and profile version at start and uses them for the entire run.
- A stage update is atomic and does not mutate a running discovery snapshot.
- The next run uses the newly committed stage.
- Active watchlist rescoring is asynchronous and idempotent by stage event.
- Rapid successive changes may schedule multiple events, but only a rescore matching the latest setting version can write current watchlist dispositions.
- Accepted, dismissed, drafted, published, measuring, or completed work is never automatically rescored or replanned.
- A failed rescore preserves the new stage, records a failure code, and exposes retry.
- An unknown stored stage or unavailable pinned profile marks discovery degraded and blocks automatic Opportunity creation until repaired.

## Run Funnel and Reporting

Run summaries add:

- pinned stage and profile version;
- candidates generated by stage source policy;
- internal terms rejected before prompt generation;
- candidates by stage gate and structured reason;
- SEO-only, GEO-only, and combined-demand candidates;
- candidates with zero resolved reuse inputs;
- watchlist promotions and demotions caused by stage rescoring;
- Opportunities created by action and asset type.

Zero Opportunities remains valid when the funnel proves that usable evidence was inspected and explains every disposition. Stage policy must never hide provider failures or missing inputs behind a healthy-zero status.

## Migration

- Existing projects receive the virtual unconfirmed Foundation default.
- Existing accepted and in-progress work is unchanged.
- Existing active watchlist candidates are not rewritten during schema migration. They are rescored after the user explicitly confirms or changes a stage, or during the first stage-aware scheduled rescore.
- Historical score snapshots retain the legacy formula and no stage is fabricated into them.
- New snapshots use the stage-aware formula and versions.
- Existing prompts containing rejected internal terms are archived during the next bounded portfolio rebuild and cannot be selected meanwhile.
- Platform contracts remain unchanged; Growth Radar begins consuming project target context and contract resolution correctly.

## Error Handling

- Missing stage row: use virtual unconfirmed Foundation and continue.
- Invalid stage update: reject without changing state.
- Optimistic-concurrency conflict: return the latest stage for user retry.
- Missing stage profile: mark the run degraded and create no automatic Opportunity.
- Target-resolution failure: award no reuse points, attach a target reason code, and block stages whose gate requires a target.
- Evidence-provider failure: preserve prior evidence, reduce freshness as it ages, and report the failed source.
- Watchlist-rescore failure: preserve the selected stage and prior watchlist rows, record failure, and allow retry.
- Partial discovery failure: persist completed evidence and candidate diagnostics under existing idempotency boundaries.

## Testing

### Unit and Contract Tests

- Each stage's weights total exactly 100.
- Integer weighting, thresholds, penalties, and gate order are deterministic at every boundary.
- The same canonical raw inputs produce intentionally different stage scores and dispositions.
- Snapshot replay reproduces component inputs, weighted points, penalties, and disposition.
- SEO query mapping accepts persisted cluster aliases and rejects LLM-only similarity.
- GEO demand deduplicates provider and UTC date and caps at ten.
- Repeated observations from one provider never satisfy multi-provider evidence.
- Uncited answer observations support only absence claims; citation claims require structured citations.
- Foundation can create a justified candidate without GSC history.
- Traction accepts independently sufficient SEO or GEO demand.
- Scale requires a success signal and real resolved reuse target or additional output type.
- Optimize requires material change and prefers refresh actions.
- Internal-sensitive vocabulary never reaches prompt generation.
- Target resolution populates all four reuse inputs and pins contract versions.
- Missing target context does not receive reuse points.
- Structured reason codes accompany every non-created disposition.
- Stage switching uses optimistic concurrency and produces an audit event.
- Watchlist rescoring cannot mutate accepted or in-progress work.
- Stale rescore jobs cannot overwrite results for a newer setting version.

### UI Tests

- The selector appears in the Opportunity page's upper-right corner.
- All four stages show the approved name and description.
- A missing stage displays Foundation with the unconfirmed-default notice.
- Selecting Foundation explicitly clears the notice.
- Confirmation shows impact and affected-watchlist count.
- Loading, success, conflict, failed rescore, and retry states are accessible and unambiguous.
- Unauthorized users cannot change the project stage.

### End-to-End Tests

- A Foundation project with confirmed capability, no GSC history, missing owned coverage, and two independent answer-provider observations creates an Opportunity at or above 70.
- The same single-provider candidate remains watchlisted regardless of numeric score.
- A Traction project promotes a watchlist candidate after an independent GEO provider or qualifying GSC signal arrives.
- A Scale project resolves exact platform targets and produces a platform-native plan rather than renamed copies.
- An Optimize project with a measured decline creates a refresh Opportunity and does not create a redundant canonical article.
- Changing stage rescans active watchlist candidates while preserving accepted work byte-for-byte.

## Rollout

1. Deploy persistence, read API, virtual Foundation default, and read-only UI label.
2. Add versioned stage profiles and shadow-score current candidates without changing disposition.
3. Deploy sanitized candidate inputs, GEO demand, deterministic cluster-query mapping, target resolution, and structured reasons.
4. Compare legacy and stage-aware results for UniPost in production. Inspect false positives, evidence provenance, target plans, and score replay.
5. Enable the selector and watchlist rescoring for UniPost with Foundation selected explicitly.
6. Enable stage-aware Opportunity creation after shadow acceptance passes.
7. Roll out the selector to all projects with the virtual unconfirmed Foundation default.

Every step is reversible through the feature flag except persisted audit history and immutable score snapshots. Rollback restores legacy disposition for new runs and does not rewrite stage-aware historical records.

## Production Acceptance

1. The Opportunity page shows the project-level stage selector in the upper-right corner with the four approved stages.
2. An existing project without a row runs as Foundation, shows the confirmation notice, and continues scheduled discovery.
3. Stage changes are authorized, audited, optimistic-concurrency safe, and visible immediately.
4. Active watchlist candidates rescore under the new stage; accepted and in-progress work remains unchanged.
5. Every stage profile totals 100, replays deterministically, and produces its documented thresholds and evidence gates.
6. No rejected internal-sensitive term, including `AES-256-GCM`, appears in a newly generated public prompt without independent public demand and explicit claim authorization.
7. UniPost's connected Search Console data can contribute through deterministic cluster-query mapping even when no row exactly equals a full GEO prompt.
8. One answer provider cannot satisfy multi-provider consensus; two independent providers can qualify the same normalized gap.
9. An uncited observation is never presented as a citation, while a fully provenanced absence observation can support the documented absence-only rules.
10. Growth Radar resolves exact project targets and fills selected, compatible, additional-output, and covered-target inputs; globally active contracts alone award no points.
11. A Foundation fixture without GSC history creates a valid Opportunity when its coverage, relevance, and independent evidence satisfy the profile.
12. Traction, Scale, and Optimize fixtures create distinct demand-amplification, expansion, and refresh Opportunities respectively.
13. Every filtered, held, arbitrated, or watchlisted candidate carries structured user-facing reason codes.
14. Production shadow runs show no unexplained disposition, no duplicate Opportunity, no unsupported claim, and no score-replay mismatch before automatic creation is enabled.

## Success Measures

- Reduction in candidates generated from rejected internal vocabulary to zero.
- Percentage of candidate dispositions with complete structured reasons reaches 100%.
- Percentage of stage-aware snapshots that replay exactly reaches 100%.
- Foundation projects produce justified Opportunities without requiring Search Console history.
- Traction projects promote candidates when independent SEO or GEO demand becomes available.
- Scale Opportunities include real compatible target or output expansion.
- Optimize Opportunities favor improvement of existing assets over redundant creation.
- UniPost moves from structurally impossible creation to an observed operating range of approximately one to three evidence-backed Opportunities per discovery cycle after independent evidence is available. This is an observation target, not a quota or threshold-adjustment trigger.

## Deferred Evolution

A later specification may add system-recommended stages using content inventory, Search Console history, answer-engine coverage, publishing history, and measured outcomes. Any recommendation must remain explainable and user-confirmed before changing policy. Cluster-level maturity may later adjust the project baseline by at most one adjacent stage. Neither behavior is part of V1.
