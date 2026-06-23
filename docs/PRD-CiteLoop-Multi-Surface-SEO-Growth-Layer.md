# PRD: CiteLoop Multi-Surface SEO Growth Layer

> 阶段：建立在 `PRD-CiteLoop-SEO-Operations-Loop.md`、`PRD-CiteLoop-GEO-Visibility-Layer.md` 和 `PRD-CiteLoop-SEO-Autopilot.md` 之上。  
> 日期：2026-06-23  
> 目标：让 CiteLoop 从 blog post 生成与发布工具，升级为覆盖多种 SEO/GEO 内容资产、站内优化动作、受控外部分发和结果复盘的增长运营系统。

## 1. 背景

CiteLoop 当前主要围绕 blog 生成、审核和发布内容。这个能力能覆盖 SEO 的一部分，但不足以支撑半自动、未来全自动的 SEO/GEO 运营。

主流 SEO 工具的能力边界已经超出“写文章”。Semrush 覆盖 keyword research、content optimization、link building、rank tracking、technical SEO、social media、AI visibility 等模块；Ahrefs 覆盖 content ideas、link prospects、brand mentions、site audit、rank tracking、AI/LLM visibility 等方向。Google 官方也强调 SEO 的核心是帮助搜索引擎理解内容、帮助用户发现站点，而不是依赖单一发布动作。

因此 CiteLoop 需要从 “blog post autopublisher” 升级为 “SEO/GEO action system”：发现机会、选择动作、生成资产、发布或分发、验证上线、衡量结果，并把结果反馈给下一轮计划。

## 2. 第一性原理

SEO 不是“发更多帖子”，而是在有限预算和风险边界内，让搜索引擎、AI answer engine 和目标用户更容易发现、理解、信任并引用产品相关内容。

系统必须同时满足：

1. **资产多样化。** Blog 只是内容资产之一；comparison、alternative、template、glossary、docs、tool、report、schema、internal links 都可能更高杠杆。
2. **动作证据化。** 每个动作必须来自 crawl、GSC/GA4、SERP、GEO observation、competitor gap 或人工输入。
3. **分发受控化。** 外部分发用于扩大发现和实体信号，不用于自动 spam、伪外链或操纵排名。
4. **风险可分类。** metadata、internal link、sitemap 与 homepage rewrite、redirect、noindex 不是同一风险等级。
5. **结果可回看。** 每次发布、分发或站内优化都必须有 baseline、verification 和 measurement window。
6. **GEO 与 SEO 共用闭环。** AI answer visibility 不应另起一套孤立系统，而应进入同一个 opportunity/action/review/measure loop。

## 3. 目标

1. 支持 blog 之外的 SEO asset 类型。
2. 把 SEO 动作从“新写内容”扩展到刷新、内链、metadata、技术修复、分发和结果复盘。
3. 支持受控外部分发，但不做批量 spam。
4. 建立 owned surfaces、external surfaces、GEO citation surfaces 的统一 inventory。
5. 所有动作进入 review、publish、verify、measure 闭环。
6. 为后续 Autopilot 提供可分类、可回滚、可审计的 action contract。
7. 让单人运营者每周看到一组高杠杆 action portfolio，而不是只看到 topic backlog。

## 4. 非目标

- 不做自动论坛 spam、自动评论、PBN、虚假 backlink。
- 不承诺排名、流量或 AI answer 引用。
- 不把社交媒体发布当作直接 SEO 排名因子。
- 不自动购买、交换或制造链接。
- 不自动删除、noindex、merge、redirect 高风险页面。
- 不绕过 robots.txt、WAF、平台 ToS 或用户授权边界。
- 不把普通 blog URL 伪装成可以通过 Google Indexing API 批量强制索引。
- 不把外部分发平台当作重复内容农场。

## 5. 产品原则

1. **Owned assets first**：优先增强客户自有站点资产，而不是到处发帖。
2. **Evidence before action**：每个动作必须能追溯到数据、观察或人工输入。
3. **Distribution is gated**：外部分发默认需要人工审核或策略确认。
4. **SEO and GEO share one loop**：传统搜索和 AI answer visibility 不拆成两套系统。
5. **No spam by design**：系统级禁止低质量批量分发和操纵链接。
6. **Action beats content volume**：系统优化的是动作组合质量，而不是文章数量。
7. **Verification is part of publishing**：发布成功不等于动作完成；必须验证 URL、canonical、indexability、links、metadata 和 tracking。

## 6. 用户场景

### 6.1 Internal Operator

1. 周一打开 CiteLoop。
2. 看到本周 action portfolio：1 个 comparison page、2 个 metadata rewrites、3 个 internal link patches、1 个 external distribution draft、1 个 GEO evidence block。
3. 接受低风险动作，审核中风险动作，拒绝或暂缓高风险动作。
4. 系统生成草稿、diff 或发布任务。
5. 发布后自动验证 URL、canonical、sitemap、metadata 和 tracking。
6. 7/14/28 天后系统回看 outcome，并更新下一轮 prioritization。

### 6.2 Growth Operator

1. CiteLoop 发现竞品在 “best API social media scheduler” 和 “Buffer alternatives” 相关 query/prompt 中持续出现。
2. 系统建议创建 `UniPost vs Buffer` comparison page 和 `Buffer alternatives for API-first teams` alternative page。
3. 用户审核 evidence blocks、allowed product claims 和 target prompts。
4. 系统发布 owned page，并生成 LinkedIn/Dev.to/Hashnode 的受控 derivative drafts。
5. 后续 observation 追踪搜索曝光、referral、品牌提及和 AI citation。

### 6.3 SaaS Customer

1. 用户只输入 product domain。
2. CiteLoop 完成 sitemap、robots、page inventory、blog/docs path 和 crawler access audit。
3. 如果用户尚未连接 GSC/GA4 或 publisher access，系统只生成 public-only 机会。
4. 用户完成 guided permission onboarding 后，系统解锁可验证的 action queue。

## 7. 功能范围

### 7.1 SEO Asset Type Registry

CiteLoop 新增 asset type registry。每个 asset type 必须定义生成方式、发布面、风险等级、验证方式和 measurement window。

初始 asset types：

| asset type | 说明 | 默认风险 |
|---|---|---:|
| `blog_post` | 现有 canonical article / supporting article | medium |
| `comparison_page` | `A vs B`、竞品对比页 | medium |
| `alternative_page` | `best X alternatives`、竞品替代页 | medium |
| `use_case_page` | 按 persona/workflow/problem 组织的用例页 | medium |
| `integration_page` | 与第三方工具或平台的集成页 | medium |
| `template_or_checklist` | 模板、清单、可下载或可复制资产 | medium |
| `glossary_definition` | 定义页、术语页、概念解释块 | low/medium |
| `benchmark_report` | 小型数据报告、行业 benchmark、survey summary | medium |
| `dataset_or_stats_page` | 可被引用的数据、统计、source bundle | medium/high |
| `docs_or_how_to_page` | 文档、教程、how-to | medium |
| `landing_page_section` | 现有 landing page 的 SEO section | high |
| `faq_block` | FAQ/Q&A block，可配合 structured data | low/medium |
| `internal_link_patch` | 给已发布页面补内链 | low/medium |
| `metadata_rewrite` | title/meta description rewrite | low |
| `schema_patch` | JSON-LD / structured data patch | medium |
| `sitemap_update` | sitemap create/update/submit | low |

每个 asset/action 必须声明：

- target intent。
- target query / prompt。
- evidence source。
- publication surface。
- risk level。
- required review policy。
- rollback strategy。
- measurement window。

### 7.2 Surface Inventory

统一 surface inventory 用于记录 CiteLoop 运营、分发或观察到的所有 URL。实现上不得新建一套与 GEO 观察脱节的 surface 表；应泛化现有 `geo_external_surfaces`，或在迁移中明确将其重命名/扩展为通用 `seo_surfaces`。

Surface categories：

- owned site surfaces：blog、docs、landing、tools、templates、reports。
- managed external surfaces：Dev.to、Hashnode、Medium、LinkedIn article、GitHub README。
- observed external surfaces：Product Hunt、directories、review pages、partner mentions、press mentions。
- GEO surfaces：被 ChatGPT、Perplexity、Claude、Google AI features 引用或可能引用的 URL。

每个 surface 记录：

- URL。
- owner type：`owned`、`managed_external`、`third_party`。
- platform。
- canonical/source URL。
- backlink/canonical status。
- indexability status。
- owner confidence。
- publication status。
- last verified time。
- observed traffic/referral/citation signals。
- related asset/action IDs。

现有 `geo_external_surfaces.owner_type` 使用 `project`、`user`、`third_party`。本 PRD 的目标语义是 `owned`、`managed_external`、`third_party`。迁移时必须做显式映射，避免 observer、publisher 和 dashboard 对 owner 类型各自解释。

### 7.3 Opportunity Types

新增或扩展 opportunity 类型：

| opportunity type | 说明 | 推荐动作 |
|---|---|---|
| `missing_comparison_page` | 竞品/对比 intent 有需求但站内无页面 | create comparison page |
| `missing_alternative_page` | `alternative to X` intent 缺口 | create alternative page |
| `missing_template_asset` | 用户需要可复制资产但站内无 template/checklist | create template/checklist |
| `missing_integration_page` | integration query/prompt 有需求但无页面 | create integration page |
| `metadata_ctr_opportunity` | GSC 显示曝光高、CTR 低 | rewrite title/meta |
| `content_decay_refresh` | 旧页面 impressions/clicks/position 下降 | refresh content |
| `internal_link_gap` | 重要页面缺少站内入口 | add internal links |
| `schema_gap` | 页面适合结构化数据但缺失或不一致 | add/fix structured data |
| `sitemap_gap` | 重要 URL 未出现在 sitemap 或 sitemap 陈旧 | update/submit sitemap |
| `external_distribution_candidate` | canonical 内容适合受控分发 | create distribution draft |
| `unlinked_brand_mention` | 第三方提及品牌但未链接 | create outreach task |
| `geo_competitor_cited_project_absent` | AI answer 引用竞品但未引用项目；沿用现有 GEO analyzer 命名 | create citation-ready asset |
| `geo_evidence_gap` | 已有页面缺少可引用证据块 | add evidence block |
| `geo_crawler_access_blocked` | AI/search crawler access 存在阻断；沿用现有 GEO audit 命名 | create technical fix task |

所有 opportunity 必须幂等，不能因为 analyzer rerun 重复制造 open item。

默认幂等 key：

`project_id + opportunity_type + normalized_target_url/topic + intent_type + engine/provider + evidence_window`

当前 `seo_opportunities` 的唯一约束包含 `created_by_run_id`，这允许不同 analyzer run 为同一问题创建新行。实现本 PRD 前必须增加 dedupe/backfill 迁移：先合并现有重复 open/accepted/converted 机会，再调整唯一约束或查询级 upsert key。不能在保留旧约束语义的情况下声明 analyzer rerun 幂等。

### 7.4 Action Portfolio

每周生成 action portfolio，不再只是 topic backlog。

实现上必须复用 `seo_action_plans` 和 `autopilot_runs.selected_actions/rejected_actions`，而不是创建独立 portfolio workflow。Portfolio 是现有 autopilot plan 的 richer view：它把 selected/deferred/rejected actions、policy snapshot、budget snapshot、risk summary 和 approval 状态组织成运营视图。

Action buckets：

- create new asset。
- refresh existing page。
- rewrite title/meta。
- add internal links。
- add structured data。
- submit/update sitemap。
- distribute canonical variant。
- monitor external mention。
- request backlink from real mention。
- create GEO evidence block。

排序依据：

- business value。
- search/GEO opportunity。
- implementation cost。
- risk level。
- freshness need。
- existing authority。
- measurement confidence。
- required permission readiness。
- policy allow/deny patterns。

Portfolio 输出必须包含：

- selected actions。
- rejected/deferred actions。
- reason codes。
- risk summary。
- required approvals。
- expected measurement windows。
- estimated cost。

### 7.5 External Distribution Policy

外部分发不是默认全自动发布，而是策略化动作。

当前外部分发已经有 `articles.kind='syndication_variant'`、`platform`、`RewriteForDistribution` 和 semi-manual compose URL。Dev.to/Hashnode/Medium 的真实 API 发布、OAuth/token storage、per-platform client 和 verification 仍未实现；这些属于 connector workstream，不应被一个 Phase 2 小 bullet 隐含带过。

初始支持平台：

- Dev.to。
- Hashnode。
- Medium。
- LinkedIn article。
- GitHub README/docs。
- Product Hunt listing metadata。
- manual external URL tracking。

发布规则：

- 默认只发布 canonical summary / derivative variant。
- 必须保留 canonical/source link。
- 社区型平台如 Reddit、HN 只生成人工草稿，不自动发布。
- 不允许自动评论、自动私信、自动刷帖。
- 不允许为了 backlink 创建低质量重复内容。
- 不允许把同一篇文章无差异复制到多个平台并伪装成原创。
- 每个平台必须有 platform-specific variant policy。

平台能力矩阵：

| platform | 初始模式 | 自动发布 | 说明 |
|---|---|---:|---|
| Dev.to | gated draft + compose URL now; API connector later | later | 适合工程/开发者内容 |
| Hashnode | gated draft + compose URL now; API connector later | later | 适合技术博客 syndication |
| Medium | gated draft | later | 需明确 canonical/source 策略 |
| LinkedIn article | draft only | no | 品牌/Founder distribution |
| Reddit | manual draft only | never by default | 社区语境强，不做自动发布 |
| Hacker News | manual draft only | never by default | 只生成人工提交建议 |
| Product Hunt | listing metadata/checklist | no | 作为实体 surface 追踪 |
| GitHub README/docs | PR/diff based | gated | 适合 developer-facing products |

### 7.6 GEO / AI Visibility

CiteLoop 需要把 AI answer visibility 当作一等指标。

能力包括：

- prompt set management。
- AI crawler access audit。
- answer/citation observation。
- competitor citation tracking。
- citation-ready asset brief。
- entity consistency audit。

Google 对 AI features 的建议仍强调基础 SEO：页面可抓取、可索引、内容可理解、结构化数据与可见内容一致。OpenAI、Perplexity、Anthropic 也分别公开了搜索 crawler / user agent 访问方式。因此 CiteLoop 应优先做 crawler access、可引用资产和 citation observation，而不是“AI 魔法文案”。

GEO opportunity 可以生成以下动作：

- create comparison page。
- create alternative page。
- create source-backed definition section。
- create template/checklist asset。
- create data-backed mini report。
- refresh canonical with evidence block。
- add internal links from supporting pages。
- distribute canonical variant to external platform。
- add external surface URL for monitoring。

### 7.7 Technical SEO Actions

技术 SEO 动作进入同一个 action queue，不作为后台隐藏任务。

初始动作类型：

- sitemap update/submit。
- robots/access audit。
- canonical mismatch fix task。
- title/meta duplication fix。
- missing H1/title/meta fix。
- JSON-LD validation task。
- broken internal link fix。
- orphan page internal link task。
- indexability/noindex review。
- AI crawler access review。

自动执行边界：

- sitemap update 可在 policy 允许时自动执行。
- metadata rewrite 仅低流量页面可自动执行。
- robots/canonical/noindex/redirect 只生成 plan 或 review task。

## 8. Data Model Reconciliation

本 PRD 不是 greenfield schema。SEO Operations、Autopilot 和 GEO Visibility 已经迁移出主要事实表；本层必须扩展这些表，避免形成第二套 action、surface、portfolio 和 distribution workflow。

### 8.0 Existing Schema Mapping

| PRD concept | Existing table/code | Decision |
|---|---|---|
| asset type registry | `geo_asset_briefs.asset_type` free string, `internal/platform` registry | Create net-new `seo_asset_types` lookup/registry; use it to validate asset/action semantics. |
| action records | `content_actions` | Extend `content_actions`; do not create `seo_actions`. |
| weekly portfolio | `seo_action_plans`, `autopilot_runs.selected_actions`, `autopilot_runs.rejected_actions` | Extend `seo_action_plans`/`autopilot_runs`; do not create `action_portfolios`. |
| surface inventory | `geo_external_surfaces` | Generalize/rename to `seo_surfaces`, or extend in place with a migration; do not dual-write. |
| distribution variants | `articles.kind='syndication_variant'`, `articles.platform`, `publisher.RewriteForDistribution` | Extend `articles`/publisher metadata; do not create `distribution_variants`. |
| citation-ready briefs | `geo_asset_briefs` | Extend `geo_asset_briefs` to cover non-GEO asset briefs; do not create a separate brief table. |
| opportunity queue | `seo_opportunities` | Extend types and idempotency; migrate the unique constraint. |
| risk classification | `internal/autopilot/risk.go`, `seo_policies`, `risk_classification_rules` | Extend existing classifier input/rules; do not create a new risk engine. |
| verification | `internal/seo.Service.checkURL`, `technical_checks`, `scheduler.verifyPublishedURL`, `guardrail_checks`, `articles.repair_status` | Reassemble/extend existing checks; do not create a parallel verifier. |

### 8.1 Net-New: `seo_asset_types`

This is the only clearly net-new table in the MVP.

Fields:

- `id`
- `key`
- `name`
- `description`
- `default_risk_level`
- `default_measurement_window_days`
- `supported_publication_surfaces`
- `requires_evidence`
- `requires_review_by_default`
- `default_generation_path`: `topic_article`, `direct_patch`, `external_draft`, `technical_task`
- `created_at`
- `updated_at`

`seo_asset_types.key` must be referenced by `content_actions.asset_type` and `geo_asset_briefs.asset_type` once those columns are normalized.

### 8.2 Extend: `content_actions`

Existing `content_actions` already owns the action lifecycle: opportunity link, action type, status, target URL/article, draft article, baseline window, measurement window, published time and outcome summary.

Add columns:

- `asset_type`
- `target_surface_id`
- `risk_reasons`
- `evidence_snapshot`
- `input_snapshot`
- `output_snapshot`
- `diff_snapshot`
- `review_required`
- `approved_by`
- `approved_at`
- `verified_at`
- `verification_snapshot`

Status additions:

- `verification_failed`
- `recovery_required`

New-asset actions must still flow through the existing topic/article path unless explicitly declared otherwise. A `create_new_asset` action creates or links a `topics` row through `topics.source_content_action_id`, then produces canonical/variant `articles` as today. Direct patch actions such as metadata rewrite or internal link patch can stay action-first and may not need a topic.

### 8.3 Extend: `seo_action_plans` and `autopilot_runs`

Existing `seo_action_plans` already has plan windows, status, `actions`, expected impact/effort, aggregate risk, approval fields and a link to `autopilot_runs`.

Add or standardize JSON fields inside `seo_action_plans.actions`:

- `selected_actions`
- `deferred_actions`
- `rejected_actions`
- `reason_codes`
- `policy_snapshot`
- `budget_snapshot`
- `risk_summary`
- `required_approvals`
- `measurement_schedule`

`autopilot_runs.selected_actions` and `autopilot_runs.rejected_actions` remain the run-level immutable snapshot. `seo_action_plans` is the operator-facing portfolio document.

### 8.4 Generalize: `geo_external_surfaces`

`geo_external_surfaces` already stores URL, normalized URL, platform, surface type, owner type, canonical target, backlink state, HTTP status and citation timestamp.

Implementation options:

1. Rename to `seo_surfaces` and migrate existing references.
2. Keep table name for compatibility but extend semantics and add query/API aliases.

The preferred long-term model is `seo_surfaces`; the first implementation may extend in place if a rename creates unnecessary churn.

Add fields:

- `source_url`
- `canonical_status`
- `indexability_status`
- `publication_status`
- `owner_confidence`
- `last_verified_at`
- `verification_snapshot`
- `related_action_ids`

Owner type migration:

- existing `project` maps to `owned` for project-controlled URLs.
- existing `user` maps to `managed_external` when CiteLoop has a tracked/manual publishing relationship.
- existing `third_party` remains `third_party`.

### 8.5 Extend: `articles` for Distribution Variants

Distribution variants already live in `articles` with `kind='syndication_variant'` and `platform`. `content_md`, `seo_meta`, `canonical_url`, `status`, `publish_result`, and `published_at` already cover most proposed fields.

Add distribution metadata either as columns or inside `seo_meta` / `publish_result`:

- `publication_mode`: `draft_only`, `gated_publish`, `auto_allowed`
- `source_url`
- `external_url`
- `verification_status`
- `external_surface_id`

Real API publishing for Dev.to/Hashnode/Medium requires extending `publisher_connections.kind`, `publisher_credentials.kind`, and implementing per-platform publisher clients. Until then, semi-manual compose URLs remain the only supported external publishing mode.

### 8.6 Extend: `geo_asset_briefs`

`geo_asset_briefs` already contains `asset_type`, `target_prompts`, `required_evidence`, `recommended_outline`, `internal_link_plan`, and `publication_surface`.

Extend it rather than creating a new asset-brief table:

- allow non-GEO source opportunities.
- add `target_queries`.
- add `target_personas`.
- add `expected_citation_mechanism`.
- add `source_type`: `geo`, `seo`, `distribution`, `technical`.

### 8.7 Migrate: `seo_opportunities` Idempotency

Current unique key: `project_id + type + normalized_page_url + query + created_by_run_id`.

Target semantics: rerunning an analyzer should update evidence on an existing open/accepted/converted opportunity when the underlying issue is the same.

Implementation requirements:

1. Backfill an `opportunity_key` or equivalent normalized dedupe key.
2. Merge duplicate open/accepted/converted rows, preserving evidence history.
3. Keep dismissed rows closed unless new evidence materially changes or a user explicitly allows reopen.
4. Replace the created-by-run scoped unique key with an idempotent key based on project, type, normalized target/topic/query/prompt/engine and evidence window.

Existing GEO names such as `geo_competitor_cited_project_absent` and `geo_crawler_access_blocked` should remain canonical unless a migration renames historical rows.

## 9. Risk Model

Risk model 已存在于 `internal/autopilot/risk.go`，并由 `seo_policies`、`risk_classification_rules`、`guardrail_checks` 和 `autopilot_audit_events` 支撑。本 PRD 不要求重建 risk classifier；只要求扩展现有 `RiskInput` 和规则表，让多资产、多 surface 动作进入同一分类器。

低风险：

- sitemap update。
- metadata rewrite for low-traffic page。
- internal link patch from approved source。
- external URL tracking。
- surface verification。

中风险：

- new comparison/alternative page。
- content refresh。
- Dev.to/Hashnode/Medium syndication。
- schema patch。
- docs/how-to page update。

高风险：

- homepage/pricing/docs major rewrite。
- noindex/delete/merge/redirect。
- robots/canonical change。
- community post automation。
- paid/reciprocal link campaign。
- product claim change without evidence。

高风险动作默认只生成 plan 或 draft，必须人工 review。

Extend existing `RiskInput` with:

- `AssetType`
- `PublicationSurface`
- `DistributionPlatform`
- `ExternalOwnerType`
- `SchemaChange`

Existing inputs remain authoritative:

- action type。
- page type。
- diff scope。
- clicks/impressions/traffic percentile。
- confidence。
- product claim presence。
- canonical/robots/redirect involvement。
- merge/noindex/delete involvement。

Existing `RiskResult` remains the output:

- `risk_level`
- `risk_reasons`
- `classifier_version`
- `low_traffic`

`review_required` should be derived from `RiskResult` + `seo_policies.requires_review_action_types` + publication surface rules, not duplicated as a separate classifier output.

## 10. Publisher Contract

现有 Publisher 已有 `Platform()`、`Mode()`、`SupportsCanonical()`、`Publish()`、dry-run behavior、semi-manual lane 和 `Result{URL, Mode, Detail, Path, CommitSHA, Phase, DeployHook, Distribute}`。本 PRD 的工作是把它从 blog-oriented publishing 扩展为 surface-aware publishing，而不是引入一套新的 publisher abstraction。

Publisher 输入：

- project。
- asset type。
- target surface。
- canonical/source URL。
- content payload。
- metadata payload。
- structured data payload。
- policy snapshot。

Publisher 输出：

- publication status。
- target URL。
- external URL。
- commit/PR/deploy reference。
- verification result。
- rollback reference。

Publisher 必须支持 dry-run。

已存在能力：

- GitHub/Next.js blog publisher 的 dry-run 和 auto-commit path。
- Semi-manual external distribution compose URL。
- `publisher_connections.capabilities` 可表达 create/update/metadata/canonical/publish/delete/rollback 等能力。
- `publisher_credentials` 可存储连接凭据。

阻塞依赖：

1. **Non-blog owned page publish target.** Comparison、alternative、template、glossary、docs 等 owned pages 到底写入 UniPost content repo、客户 CMS、CiteLoop hosted surface，还是多目标，都必须在 Phase 1 决定。没有这个决定，Phase 2 不能开始 non-blog owned publishing。
2. **Rollback/delete are not implemented.** 现有能力矩阵可以声明 rollback/delete，但 publisher 还没有真实执行能力。高风险 recovery 只能在实现 rollback/delete 或明确 manual recovery contract 后解锁。
3. **External API connectors are not implemented.** Dev.to/Hashnode/Medium 当前只能生成 human compose URL。真实 gated publish 需要扩展 `publisher_connections.kind`、credential kind、OAuth/token refresh、API client、verification 和 platform-specific error handling。

## 11. Verification

每个动作完成后必须进入 verification。

Verification 不应从零实现。现有能力已经分散在：

- `internal/seo.Service.checkURL` / `TechnicalResult`：HTTP status、canonical、robots/noindex、title、meta description、H1、structured data、internal/outbound link counts。
- `technical_checks`：持久化技术检查。
- `scheduler.verifyPublishedURL`：发布后 2xx polling。
- `guardrail_checks`：记录 action-level blocking/warning checks。
- `articles.repair_status`：承接自动恢复与人工决策状态。

Verification checks：

- URL returns 2xx。
- canonical matches expected。
- title/meta/H1 matches expected。
- robots/meta noindex 状态符合预期。
- sitemap contains expected URL when applicable。
- internal links present when applicable。
- structured data parses and matches visible content when applicable。
- external distribution page links back to canonical/source when applicable。
- analytics/referral/citation tracking markers present when applicable。

现有检查可直接覆盖 HTTP、canonical、title/meta/H1、robots/noindex、structured data presence 和 link counts。需要新增或增强：

- sitemap contains expected URL。
- structured data matches visible content, not just JSON-LD presence。
- external distribution page links back to canonical/source。
- distribution variant canonical/source backfill verification。
- AI/search crawler access evidence linkage for GEO actions。

失败处理：

- low-risk action 自动重试一次。
- medium-risk action 标记为 `verification_failed` 并进入 review。
- high-risk action 不自动修复，生成 recovery task。

失败状态应写回 `content_actions.status`、`guardrail_checks` 和相关 `articles.repair_status`，而不是创建独立 verification workflow。

## 12. Measurement

每个 action 都必须安排 measurement window。

现有 `content_actions.measurement_window` 和 `seo_experiments` 支持单个 baseline/measurement pair。本 PRD 需要多 checkpoint 观察。MVP 采用 `content_actions.measurement_window` 内的 checkpoint array，避免先引入新事实表；后续如果 checkpoint 查询和告警变复杂，再提升为 normalized `measurement_checkpoints` 表。

Example `measurement_window` shape:

```json
{
  "baseline": {"start": "2026-06-01", "end": "2026-06-14"},
  "checkpoints": [
    {"day": 7, "status": "scheduled"},
    {"day": 14, "status": "scheduled"},
    {"day": 28, "status": "scheduled"}
  ],
  "primary_metric": "clicks",
  "secondary_metrics": ["impressions", "ctr", "position"]
}
```

默认窗口：

- metadata rewrite：7/14/28 天。
- internal link patch：14/28/56 天。
- new asset：14/28/56/90 天。
- external distribution：7/14/28 天。
- GEO citation-ready asset：weekly observation for 8 weeks。
- technical fix：1/7/14 天 verification + 28 天 impact review。

Outcome metrics：

- impressions。
- clicks。
- CTR。
- average position。
- indexed status。
- referral sessions。
- engaged sessions。
- conversions/key events。
- brand mentions。
- project-owned citations。
- competitor citations。
- backlinks/mentions discovered。

系统不得把短期波动当作因果证明。Outcome 必须标注 confidence。

Metric sources:

- Search metrics derive from `page_performance_daily`, `search_performance_daily`, `search_appearance_daily`。
- Action lifecycle metrics derive from `content_actions`, `seo_action_plans`, `autopilot_runs`。
- Cost/status/timing derive from `generation_runs`, `seo_runs`, `geo_runs`, `autopilot_runs`。
- GEO mention/citation metrics derive from `geo_observations` and `geo_visibility_scores`。
- Technical issue metrics derive from `technical_checks`。

## 13. API and UI Surface

This layer extends the existing dashboard and API; it must not create a separate planning UI.

API requirements:

- Extend existing SEO opportunity/action endpoints to expose asset type, surface, risk, verification and measurement fields.
- Extend autopilot/plan endpoints to expose weekly action portfolio shape from `seo_action_plans`.
- Extend GEO endpoints to surface generalized external/owned surfaces and asset briefs.
- Add publisher capability endpoints for non-blog owned publishing readiness and external connector readiness.

UI requirements:

- SEO overview shows action portfolio grouped by action bucket and risk.
- Review queue shows asset type, target surface, evidence snapshot, diff snapshot, verification status and measurement schedule.
- GEO view links prompt/citation gaps to asset briefs and content actions.
- Publisher readiness view distinguishes GitHub/Next.js blog publishing, non-blog owned publishing, semi-manual distribution and future API connector publishing.

Implementation should extend existing `web` dashboard screens rather than introducing a disconnected admin surface.

## 14. Success Metrics

Product metrics：

- 每周 generated action portfolio 数量。
- accepted action rate。
- publish success rate。
- verification success rate。
- time from opportunity to published asset。
- percent actions with measurement schedule。
- review burden per week。

SEO metrics：

- indexed URL count。
- impressions/clicks uplift by page cohort。
- CTR improvement after metadata rewrite。
- internal link coverage。
- technical issue reduction。
- referring domains/real mentions tracked。

GEO metrics：

- priority prompt brand mention rate。
- project-owned citation rate。
- competitor citation gap reduction。
- AI crawler access pass rate。
- citation-ready asset coverage。

Safety metrics：

- spam policy violation count。
- high-risk action auto-publish count, must be zero by default。
- verification failure rate。
- rollback/recovery count。
- duplicate external distribution count。

## 15. MVP Scope

### Phase 1: Planning and Inventory Foundation

- Resolve non-blog owned publish target as a prerequisite.
- Add `seo_asset_types` registry.
- Extend `content_actions`, `seo_action_plans`, `geo_external_surfaces`, `geo_asset_briefs`, and `articles` according to §8.
- Add `seo_opportunities` dedupe/backfill migration and idempotent key strategy.
- Expand opportunity type taxonomy using existing canonical GEO names.
- Implement action portfolio planner on top of `seo_action_plans`.
- Extend existing dashboard to show action portfolio grouped by action bucket and risk.
- Add manual external surface tracking through generalized surface inventory.

### Phase 2: Multi-Surface Execution

- Implement publisher support for the selected non-blog owned page target.
- Add metadata/internal-link/schema/sitemap actions through `content_actions`.
- Add sitemap URL containment verification and external canonical/source link verification.
- Add measurement checkpoint arrays to `content_actions.measurement_window`.
- Add rollback/delete publisher contract or explicit manual recovery path before high-risk actions can execute.

External connector workstream:

- Extend `publisher_connections.kind` and `publisher_credentials.kind` for Dev.to/Hashnode/Medium only when API publishing is explicitly in scope.
- Implement per-platform API clients, token handling, capability probes and verification.
- Keep semi-manual compose URL as the default external distribution mode until connector verification is reliable.

### Phase 3: GEO and Entity Expansion

- Extend existing GEO prompt observation and Perplexity/manual fixture provider path.
- Extend AI crawler access audit into generalized surface inventory.
- Convert citation gap opportunities into `geo_asset_briefs` and `content_actions`.
- Add entity/external surface monitoring.
- Add citation-ready asset briefs for comparison, alternative, template, glossary and data-backed assets.

## 16. Blocking Decisions and Open Questions

Blocking before Phase 1 design freeze:

1. 非 blog owned pages 的真实发布目标是什么：UniPost content repo、客户 CMS、CiteLoop hosted surface，还是三者都支持？
2. 是否接受将 `geo_external_surfaces` 迁移/泛化为 `seo_surfaces`，还是短期保留表名并用 API alias 隐藏历史命名？
3. `seo_opportunities` dedupe/backfill 的历史行合并规则是什么，尤其是 dismissed rows 是否允许 reopen？

Open but not blocking:

1. Comparison/alternative pages 默认按现有 risk classifier 视为 medium risk，需要人工 review；问题是未来什么条件下可提升为 auto publish。
2. LinkedIn/X 这类社媒内容是否只作为 referral/brand distribution，不进入 SEO outcome score？
3. External distribution 的首批真实 API connector 应优先 Dev.to/Hashnode，还是 GitHub README/docs？
4. Measurement checkpoints 是否长期保留在 JSONB，还是 Phase 3 提升为 normalized 表？

Already decided by existing implementation:

- GEO observation 首批策略沿用 manual fixture + answer-engine provider 分层；`manual_fixture`、`answer_engine`、`serp_probe`、`manual_required` 已存在。
- Perplexity provider path 已存在，可作为第一批自动 answer-engine provider。
- Comparison/alternative 默认不自动发布；现有 risk policy 将中风险动作送 review。

## 17. References

- Semrush Features: https://www.semrush.com/features/
- Semrush Social Media Management: https://www.semrush.com/features/social-media-marketing/
- Ahrefs Content Explorer: https://ahrefs.com/content-explorer
- Ahrefs Site Audit: https://ahrefs.com/site-audit
- Google SEO Starter Guide: https://developers.google.com/search/docs/fundamentals/seo-starter-guide
- Google Spam Policies: https://developers.google.com/search/docs/essentials/spam-policies
- Google AI Features: https://developers.google.com/search/docs/appearance/ai-features
- Google Sitemaps: https://developers.google.com/search/docs/crawling-indexing/sitemaps/overview
- OpenAI Publishers FAQ: https://help.openai.com/en/articles/12627856-publishers-and-developers-faq
- Perplexity Crawlers: https://docs.perplexity.ai/docs/resources/perplexity-crawlers
- Anthropic Crawlers: https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler
