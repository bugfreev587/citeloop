# PRD: CiteLoop Analysis Workflow and Dashboard Isolation

> 日期: 2026-06-24
> 状态: Draft
> 范围: Dashboard 信息架构、Analysis workflow、Search Console 接入入口、Analysis 与 Content Generation 的产品边界
> 上游文档:
> - `docs/PRD-CiteLoop-Dashboard-Control-Center-Redesign.md`
> - `docs/PRD-CiteLoop-SEO-Operations-Loop.md`
> - `docs/PRD-CiteLoop-Multi-Surface-SEO-Growth-Layer.md`

## 0. 摘要

CiteLoop 当前的核心能力已经覆盖内容生成、审核、发布和部分可见性反馈。代码库也已经包含 SEO operations loop 的主要数据模型和一部分页面能力。随着 Google Search Console / GA4 驱动的真实 SEO 数据进入产品，Dashboard 需要把 "分析机会" 和 "生产内容" 明确隔离。

本 PRD 的目标不是从零新增一套分析系统，而是把现有 Opportunities / Visibility / SEO data layer 产品化为独立的 Analysis workflow:

```text
Context -> Analysis -> Content Plan -> Review -> Publish -> Measure
```

Analysis 负责呈现公开数据、GSC/GA4 私有数据、SERP/GEO 信号，并把这些信号转化为可解释的 opportunity 和 action recommendation。Content Plan 只接收已经被用户或策略接受的工作，不再承担原始分析、数据连接和机会判断职责。

这让 CiteLoop 从 "content generation dashboard" 升级为 "analysis to action to publishing loop"，同时避免用户把所有 SEO 数据、机会、选题和内容草稿混在一个页面里理解。

## 1. 背景

当前 Dashboard 已有以下导航:

```text
Home
Context
Opportunities
Content Plan
Review
Publish
Visibility
Settings
Admin
```

这个结构在早期 MVP 中可用，但在引入 GSC/GA4 后会出现三个问题:

1. **Opportunities 和 Content Plan 边界不清。**
   用户难以判断 opportunity 是待分析信号、待接受建议，还是已经进入生产的内容计划。

2. **Visibility 同时承担发现和复盘。**
   Visibility 既展示 SEO/GEO 信号，又承担机会审核、结果回看和诊断，页面心智过重。

3. **Content Generation 被迫承接 Analysis 缺口。**
   当系统不知道真实搜索表现时，Content Plan 只能像 topic backlog。接入 GSC 后，应该由 Analysis 判断 "为什么做这件事"，再把被接受的 action 交给 Content Plan 或其他执行页面。

ChatSEO 的定价结构也印证了这个边界: Analysis 是独立价值层，Content 是执行层。CiteLoop 不应只复制 ChatSEO 的分析助手，而应把分析结果接入已有的 content / review / publish / measure loop。

### 1.1 当前代码基线

截至 `main@0001e063951bf06b931811efd33ae7e15d4d605a`，以下能力已经存在，实施本 PRD 时应复用而不是重建:

- `internal/migrations/0007_seo_operations_loop.sql` 已创建 `seo_properties`、`seo_integrations`、`seo_runs`、`search_performance_daily`、`page_performance_daily`、`search_appearance_daily`、`url_index_snapshots`、`technical_checks`、`seo_opportunities`、`content_actions`。
- `page_performance_daily` 已包含 `ga4_sessions`、`ga4_engaged_sessions`、`ga4_conversions` 字段，GA4 不应被当作完全 greenfield storage。
- `seo_opportunities` 已包含 `type`、`status`、`priority_score`、`confidence`、`evidence`、`recommended_action`、`expected_impact`、`effort`、`risk_level`、`opportunity_key` 等 Analysis 需要的核心字段。
- `content_actions` 已经承担 opportunity -> action -> article/result 的桥接，包含 `baseline_window`、`measurement_window`、`outcome_summary`。
- `web/app/projects/[id]/opportunities/page.tsx` 和 `web/app/projects/[id]/visibility/page.tsx` 已经分别渲染 `OpportunitiesClient` 与 `VisibilityClient`，两者共享 `web/app/projects/[id]/seo/seo-client.tsx` 中的 `SEOClient`。
- 当前 Google 数据连接由 `internal/googledata/auth.go` 的 service-account JWT 实现，scope 为 `webmasters.readonly` 和 `analytics.readonly`。当前代码没有 end-user Google OAuth consent、用户可访问 property 列表、GSC refresh token 捕获。本 PRD 将 self-serve GSC OAuth 明确列为需要新增的 connection layer。

因此 Phase 1-3 应被视为 IA 重构和页面职责拆分: `Opportunities` 重命名/迁移到 `Analysis`，`Visibility` 收敛为 `Results`，并把共享 `SEOClient` 拆成更清楚的 Analysis / Results surfaces。

## 2. 产品目标

1. 在 Dashboard 中将现有 Opportunities / Visibility 能力重组为独立的 Analysis workflow，并与 Content Generation 隔离。
2. 让用户通过 domain-first onboarding 开始项目，再通过 self-serve Google Search Console OAuth 解锁真实搜索分析。service account 保留为 internal/admin fallback，不作为默认客户路径。
3. 把 GSC/GA4 信号转化为可解释、可接受、可路由的 SEO/GEO actions。
4. 让 Content Plan 只展示已接受的生产工作，不展示未筛选的原始机会。
5. 让 Measure 页面专注发布后的结果和闭环反馈，不再混入原始机会发现。
6. 简化 sidebar，把 Settings 移到左下角 Docs 下方，Admin 保持左下角入口，移除主导航中的 SYSTEM 分组。
7. 保持 Home 作为控制中心，只展示当前最重要的状态、下一步和数据连接 gate。
8. 降低默认页面的信息负担，只展示用户需要知晓、决策、批准或处理的内容。

## 3. 非目标

- 不在本 PRD 的 Phase 1-3 中实现 end-user Google OAuth、GSC ingestion 或 GA4 ingestion 的完整后端方案。Self-serve GSC OAuth 从 Phase 4 开始进入本 PRD 范围。
- 不把当前 service-account 连接模型伪装成用户 OAuth。两者必须在 UI、权限和文档中明确区分。
- 不在本 PRD 中重写现有 Review、Publish、Publisher 或 Article detail 页面。
- 不把 Analysis 做成全量 SEO dashboard 或 Semrush/Ahrefs 替代品。
- 不自动执行高风险 SEO 动作，例如 redirect、noindex、delete、merge。
- 不承诺排名、流量、转化或 AI answer citation 提升。
- 不引入普通用户必须理解的 GSC property、OAuth scope、credential ref、GA4 property id 等工程概念。

## 4. 核心原则

### 4.1 Analysis owns why

Analysis 回答:

- 为什么这个机会存在?
- 证据来自哪里?
- 这个动作优先级为什么高?
- 不做会损失什么?
- 做完后如何衡量?

### 4.2 Content Plan owns what gets produced

Content Plan 回答:

- 哪些工作已经被接受?
- 需要生成什么资产或修改什么页面?
- 什么时候生成?
- 生成约束、brief、target keyword、evidence block 是什么?

### 4.3 Measure owns what happened

Measure 回答:

- 发布后的 URL 是否可访问?
- Google 是否开始产生 impressions?
- CTR、position、clicks、AI citation、referral 是否变化?
- 这个 action 是否应该影响下一轮 prioritization?

### 4.4 Home is the control center

Home 不展示完整 analytics，不展示所有机会列表。Home 只展示:

- 当前项目是否 ready。
- 下一个最重要动作。
- Search data 是否连接。
- 本周 loop 里最重要的阻塞或成果。

### 4.5 User language beats provider language

默认用户可见 UI 不出现:

- `gsc_site_url`
- `service account`
- `credential ref`
- `Search Analytics API`
- `Run Strategist`
- `reconcile`
- `canonical_url missing`

这些字段仍可作为内部 schema、API payload 或 admin diagnostics 存在。本原则只约束面向普通用户的 copy 和默认 workflow，不要求重命名数据库字段。

默认 UI 应表达:

- `Connect Search Console`
- `Real search data connected`
- `Find pages with impressions and low CTR`
- `Create refresh brief`
- `Draft title and meta update`
- `Live URL is not confirmed yet`

### 4.6 Progressive disclosure protects user attention

CiteLoop should not expose every generated artifact, sync result, diagnostic, or historical record by default. Default pages should be action-first and decision-focused.

Default visible content should be limited to:

- current status
- next action
- items requiring user approval
- blockers that need user attention
- concise reason why an action is recommended
- confidence, risk, and expected outcome when they affect the decision

Default hidden or collapsed content:

- raw GSC rows
- full evidence tables
- sync logs
- completed automation steps
- generated intermediate briefs that do not need approval
- old dismissed opportunities
- provider diagnostics
- token and credential details
- long historical lists
- metrics not connected to a user decision

These details should be available through deliberate disclosure controls such as:

- `Why this?`
- `View evidence`
- `Show completed`
- `View diagnostics`
- `See measurement details`

The default experience should feel like a prioritized work queue, not an operations console.

## 5. 推荐信息架构

### 5.1 主导航

新的项目 sidebar:

```text
Home
Context

ANALYZE
Analysis

CREATE
Content Plan
Review

DELIVER
Publish

MEASURE
Results
```

左下角 utility 区:

```text
Budget
Projects
Docs
Settings
Admin
Account / Workspace switcher
```

变更说明:

- 移除主导航中的 `SYSTEM` 分组。
- `Settings` 移到左下角 `Docs` 下方。
- `Admin` 保持左下角入口，不在主工作流中出现。
- `Opportunities` 不再作为顶层主导航项。机会队列归入 `Analysis`。
- `Visibility` 重命名或收敛为 `Results`，专注 Measure 阶段。

### 5.2 页面职责

| 页面 | 主要职责 | 不应该承担 |
|---|---|---|
| Home | 当前状态、下一步、连接 gate、loop health | 全量 analytics、完整机会列表 |
| Context | domain 理解、产品定位、证据、竞争对手、内容规则 | 搜索表现分析、内容生产排期 |
| Analysis | GSC/GA4/SERP/GEO 信号、机会队列、action recommendation | 内容草稿编辑、发布状态复盘 |
| Content Plan | 已接受 action 的生产 backlog、brief、schedule、generation intent | 原始 SEO 数据探索 |
| Review | 草稿是否可发布、证据是否充分、QA blocking | 机会优先级判断 |
| Publish | canonical publish、variant unlock、publish failure、URL verification | 数据分析和机会发现 |
| Results | 已发布 action 的 measurement、traffic/citation/CTR/position outcome | 未接受 opportunity queue |

## 6. Analysis Workflow

### 6.1 用户路径

目标客户路径基于 self-serve Google Search Console OAuth:

```text
1. 用户输入 product domain
2. CiteLoop 完成 public discovery
3. Home 和 Analysis 显示 Search data gate
4. 项目 owner/admin 点击 Connect Google Search Console
5. Google OAuth consent 返回该用户可访问的 GSC properties
6. CiteLoop 自动推荐与 product domain 匹配的 property
7. 用户确认 property，或手动选择正确 property
8. CiteLoop 安全存储 OAuth refresh token 和 selected property
9. 系统 backfill 最近 90 天搜索数据
10. Analysis 生成 opportunity queue
11. 用户接受、暂缓或 dismiss opportunity
12. 被接受的 action 进入 Content Plan / Review / Publish / Content Plan technical_task
13. 发布或执行后进入 Results measurement
```

Internal/admin fallback 路径继续支持 service account，但不作为默认客户 onboarding:

```text
1. Operator 在 Settings / Admin 中配置 `site_url`、`gsc_site_url`、`credential_ref`
2. 系统用 service-account JWT 读取 GSC / GA4 数据
3. UI 明确显示为 internal/admin-managed connection
4. 用户仍然在 Analysis 中查看机会、接受 action 和进入 Results measurement
```

Self-serve OAuth 是本 PRD 的正式目标。Phase 1-3 先完成 IA 和页面职责隔离，Phase 4 开始交付 OAuth connection system，Phase 5 将 OAuth 数据接入 GSC analysis。

### 6.2 Search data 状态

| 状态 | 含义 | UI 行为 |
|---|---|---|
| `public_only` | 只有公开 crawl / sitemap / robots / SERP 数据 | 显示 public opportunities，不展示 CTR/position 事实 |
| `oauth_not_connected` | 项目尚未连接用户授权的 GSC property | 对项目 owner/admin 显示 Connect Search Console；对无权限成员显示 ask admin |
| `oauth_authorizing` | 用户正在 Google OAuth consent / callback 流程中 | 显示连接中状态，失败后回到可重试状态 |
| `oauth_authorized_property_missing` | 用户授权了 Google，但没有匹配 product domain 的 property | 显示创建/验证 GSC property 指引，允许选择其他 property |
| `oauth_property_selected` | 用户选择了 GSC property，等待首次同步 | 显示 backfill 状态 |
| `gsc_backfilling` | 正在拉取历史数据 | 显示 skeleton 和预计可用时间 |
| `gsc_connected` | Search data 可用 | 展示真实 opportunity queue |
| `gsc_stale` | 数据过期或同步失败 | 降级展示最后可用数据，并提示有权限用户 reconnect |
| `gsc_property_mismatch` | 配置的 GSC property 与 discovered domain/canonical host 不匹配 | 显示 mismatch warning，阻止使用错误数据做高置信 recommendation |
| `gsc_permission_revoked` | Google token 失效、用户撤销授权或 scope 不足 | 显示 reconnect CTA，不删除历史 measurement |
| `service_account_configured` | internal/admin 使用 service account 配置了 first-party search data | 普通用户看到 connected 状态；Settings/Admin 中显示 internal-managed diagnostics |
| `ga4_connected` | GA4 engagement/conversion 可用 | 在 opportunity priority 中加入 business value |

### 6.3 Analysis 页面结构

```text
Analysis
├─ Search data status
│  ├─ public-only / OAuth not connected / authorizing / property selected / connected / stale / mismatch / revoked / backfilling
│  └─ Connect, select property, reconnect, or admin handoff action
├─ Weekly analysis brief
│  └─ 本周推荐 action portfolio
├─ Opportunity queue
│  ├─ Quick wins
│  ├─ Near page-one keywords
│  ├─ Low CTR pages
│  ├─ Content decay
│  ├─ Missing content assets
│  ├─ Internal linking opportunities
│  ├─ Indexing and technical issues
│  └─ GEO citation gaps
└─ Evidence inspector, collapsed by default
   ├─ query
   ├─ page
   ├─ impressions
   ├─ clicks
   ├─ CTR
   ├─ average position
   ├─ trend window
   ├─ source
   └─ confidence
```

Analysis default view should show a small number of prioritized recommendation cards, not a complete table of all signals. Each card should answer:

- what happened
- why it matters
- recommended action
- expected impact
- risk or confidence
- accept / dismiss / snooze

Raw evidence, query lists, diagnostic details, and generated reasoning should be hidden behind `View evidence` or `Why this?` controls.

### 6.4 Opportunity taxonomy

| Opportunity type | Primary evidence | Recommended action | Destination |
|---|---|---|---|
| `low_ctr_page` | impressions high, CTR below expected range | Draft title/meta update | Content Plan (`metadata_rewrite`) or Review |
| `near_page_one_query` | average position 8-20 with meaningful impressions | Create refresh brief or internal link task | Content Plan |
| `content_decay` | clicks/impressions/position decline over comparison window | Refresh existing page | Content Plan |
| `missing_content_asset` | query cluster has impressions but no dedicated asset | Create new asset brief | Content Plan |
| `internal_link_gap` | source pages can support target page | Create internal link task | Content Plan (`technical_task`) |
| `indexing_issue` | URL absent, excluded, or sitemap mismatch | Create technical SEO task | Content Plan (`technical_task`) |
| `geo_citation_gap` | competitor cited in AI answer, project absent | Create citation-ready asset | Content Plan |
| `backlink_or_mention_gap` | public market data or backlink provider signal | Create outreach or evidence asset | Content Plan or Manual task |

### 6.4.1 Existing type mapping

`seo_opportunities.type` is stored as text, not a database enum, but existing analyzers already emit specific names. Implementation should map product taxonomy to existing types where possible and only introduce new type strings with explicit tests and migration notes.

| Product taxonomy | Existing or proposed `seo_opportunities.type` | Migration/API impact |
|---|---|---|
| `indexing_issue` | `indexing_anomaly` | Existing type; reuse. |
| `geo_citation_gap` | `geo_competitor_cited_project_absent`, `geo_project_mentioned_without_citation` | Existing GEO analyzer types; group in UI. |
| crawler access issue | `geo_crawler_access_blocked` | Existing type; group under technical / GEO blockers. |
| public cold-start content | `cold_start_context_plan`, `cold_start_competitive_gap`, `cold_start_evidence_page` | Existing types; keep in public-only Analysis. |
| `low_ctr_page` | `low_ctr_page` | New generator/type if not already emitted. Requires tests and copy mapping. |
| `near_page_one_query` | `near_page_one_query` | New generator/type if not already emitted. Requires tests and copy mapping. |
| `content_decay` | `content_decay` | New generator/type if not already emitted. Requires tests and copy mapping. |
| `internal_link_gap` | `internal_link_gap` | New generator/type if not already emitted. Requires tests and action routing. |
| `backlink_or_mention_gap` | `backlink_or_mention_gap` | Future provider-dependent type; not Phase 1. |

### 6.5 Action routing

Analysis must not assume every opportunity creates a new article.

| Recommended action | Route |
|---|---|
| `create_new_asset` | Content Plan |
| `refresh_existing_page` | Content Plan |
| `draft_title_meta_update` | Content Plan or Review, depending on publisher capability |
| `add_internal_links` | Content Plan (`technical_task`) |
| `create_schema_update` | Content Plan (`technical_task`) or Review |
| `fix_indexing_issue` | Content Plan (`technical_task`) |
| `wait_and_measure` | Results watchlist |
| `dismiss` | Analysis archive |

Accepted opportunities must carry evidence into downstream work:

- source type
- source URL or GSC property
- query cluster
- target URL
- baseline window
- expected impact
- confidence
- recommended measurement window
- risk level

## 7. Home Changes

Home should keep the current control center direction, but add a clearer Search data gate.

### 7.1 Without GSC

Home should show:

```text
Search data is not connected
Connect Search Console to unlock query, CTR, position, and content decay opportunities.
[Connect Search Console]
```

If the current user cannot manage project integrations, the action label becomes:

```text
Ask an admin to connect Search Console
```

Metric cards should avoid fake precision:

- Organic traffic: `Limited`
- Detail: `Connect Search Console for real traffic data`
- AI citations: use existing visibility/GEO connection state
- Published pages: keep current real value
- In motion: keep current workflow value

### 7.2 With GSC

Home should show one compact analysis summary:

```text
3 search opportunities need review
2 low CTR pages, 1 page-one push, 1 content refresh candidate.
[Review analysis]
```

Home should not show the full opportunity table.

### 7.3 Next step priority

Home next action priority should become:

1. Critical publish/review blockers.
2. Required context confirmation.
3. Search data connection missing, if project has enough public discovery to benefit.
4. New Analysis opportunities that need acceptance.
5. Content Plan / Review / Publish actions.
6. Results waiting for measurement.

## 8. Content Plan Isolation

Content Plan should only contain accepted work.

Allowed Content Plan items:

- accepted new content asset
- accepted refresh brief
- accepted metadata rewrite
- accepted internal link patch
- accepted GEO evidence asset
- scheduled generated draft
- accepted manually seeded topic

Not allowed in Content Plan:

- raw GSC rows
- unreviewed opportunities
- rejected opportunities
- search data connection prompts
- provider diagnostics
- broad visibility score explanations
- generated intermediate artifacts that do not require review
- completed items unless the user opens Completed or History

Content Plan default view should prioritize:

- work that needs approval
- work scheduled next
- blocked work
- manually seeded topics

Completed work, in-progress automation details, and generated supporting artifacts should be summarized, not laid out flat.

When an Analysis opportunity is accepted, Content Plan should show the reason in user language:

```text
Why this is planned:
This page has 14,200 impressions in 28 days, but CTR is 0.7%.
Updating title and meta may increase clicks without creating a new page.
```

## 9. Results and Measure Isolation

`Results` replaces or narrows the current `Visibility` mental model.

Results should show:

- published action outcomes
- URL verification status
- impressions after publish
- clicks / CTR / average position changes
- AI citation observations
- referral or engagement signals if GA4 is connected
- measurement windows
- inconclusive / positive / negative / waiting states

Results should not be the place where users decide whether to accept raw opportunities. That belongs in Analysis.

Results default view should show outcome summaries and exceptions. Long per-URL measurement details, historical rows, and inconclusive low-signal records should be collapsed behind measurement detail views.

## 10. Settings and Admin Placement

### 10.1 Settings

Settings moves to the left utility area under Docs.

Settings owns:

- project configuration
- publisher connections
- Search Console OAuth connection management
- GA4 connection management
- notification channels
- crawl settings
- advanced diagnostics

Settings should not be a primary workflow step.

Search data connection management should be available to project owners/admins through a self-serve Google OAuth flow. Analysis may show the CTA to everyone, but members without integration-management permission should receive an explanation and an admin handoff path. Internal operators may still configure service-account access through Admin/diagnostics, but that path should be labeled as internal-managed and should not replace the default OAuth onboarding.

### 10.2 Admin

Admin remains in the left utility area. It should not appear in the main workflow navigation.

Admin owns:

- internal-only tools
- privileged maintenance
- raw diagnostics
- operator utilities

### 10.3 Sidebar grouping

The `SYSTEM` section is removed from the primary nav. Utility links are visually separated at the bottom and should not compete with daily workflow pages.

## 11. Empty States

### 11.1 Analysis empty state before GSC

Primary message:

```text
Connect real search data
CiteLoop can already inspect your public site. Connect Search Console to find pages with impressions, low CTR, ranking drops, and near page-one keywords.
```

Primary action:

```text
Connect Search Console
```

Secondary action:

```text
Review public opportunities
```

### 11.2 Analysis empty state after GSC but no opportunity

Primary message:

```text
No search actions need review
CiteLoop will keep watching query, page, CTR, position, and content decay signals.
```

Secondary content:

- last sync time
- number of pages observed
- next scheduled analysis

### 11.3 Content Plan empty state

Primary message:

```text
No accepted work yet
Review Analysis opportunities or add a manual topic to create the next production item.
```

Primary action:

```text
Open Analysis
```

## 12. Data and API Implications

This PRD does not require final schema design, but implementation should preserve these product boundaries.

### 12.1 Existing concepts to preserve

- `content_actions` remains the action lifecycle owner.
- `articles` remains generated content output.
- `seo_opportunities` or equivalent remains the raw/accepted opportunity source.
- `publisher_connections` remains publishing capability state.
- `seo_properties` / integration records should express data readiness.

Implementation must not create replacement tables for these concepts during Phase 1-3. If Analysis needs a new UI shape, adapt API serializers or view models over the existing tables first.

### 12.2 OAuth connection model

Self-serve GSC OAuth requires a new connection layer around the existing SEO data model:

- Google OAuth app configuration with the minimum required Search Console scope: `https://www.googleapis.com/auth/webmasters.readonly`.
- OAuth start and callback endpoints with CSRF/state protection.
- Encrypted storage for refresh tokens and token metadata.
- Token refresh, revocation, expired-token handling, and reconnect flows.
- Property listing from the authorized Google account.
- Domain-to-property matching for `sc-domain:example.com`, `https://example.com/`, `https://www.example.com/`, and subdomain variants.
- User confirmation of the selected property before any data is treated as first-party evidence.
- Persistence of the selected property into existing `seo_properties` / `seo_integrations` concepts where possible.
- Audit metadata for who connected, when it was connected, and when it last synced.
- Internal service-account fallback that can reuse the same downstream sync and opportunity-generation pipeline.

The default customer path must not require the user to manually create credentials or share service-account access. The only expected customer-side prerequisite is that they have access to the relevant GSC property in their Google account.

### 12.3 Required product states

Analysis needs enough API surface to express:

- search data connection status
- connected Google account display label
- available / selected GSC property display labels
- active GSC property display label
- last sync time
- backfill status
- token health and reconnect need
- opportunity type
- opportunity evidence
- recommended action
- accepted / dismissed / archived status
- downstream destination
- measurement window

Connection states should be derived from OAuth token health, selected property, `seo_properties`, `seo_integrations`, run recency, sync errors, and user access level. Service-account projects should still resolve to the same downstream readiness states, with admin-only diagnostics exposing `credential_ref`.

### 12.4 Downstream contract

When Analysis routes work downstream, it must create or update a durable action record with:

- action type
- source opportunity id
- evidence summary
- target page or new asset type
- accepted at
- accepted by
- risk level
- status
- expected measurement dates

## 13. Rollout Plan

### Phase 1: Information architecture only

- Update sidebar grouping.
- Move Settings to bottom utility area.
- Keep Admin bottom utility only.
- Rename existing `Opportunities` surface to `Analysis`.
- Add `/projects/[id]/analysis` as the canonical route.
- Redirect `/projects/[id]/opportunities` to Analysis for bookmarks and old links.
- Move opportunity review entry points out of `Visibility` and into `Analysis`.
- Keep existing backend APIs.

### Phase 2: Analysis page productization

- Add Search data status block derived from existing `seo_properties` / `seo_integrations` / SEO run state.
- Add GSC connection gate.
- Add weekly analysis brief shell.
- Add typed opportunity queue.
- Add evidence inspector.
- Ensure accepted opportunities route to Content Plan.

### Phase 3: Results cleanup

- Narrow `Visibility` into `Results`.
- Remove raw opportunity review from Results.
- Show published action outcomes and measurement windows.
- Keep GEO/AI citation tracking as a Results signal.

### Phase 4: Self-serve GSC OAuth onboarding

- Implement Google OAuth start / callback flow for Search Console.
- Request `https://www.googleapis.com/auth/webmasters.readonly` for query and page performance.
- Store refresh tokens and token metadata securely.
- List authorized GSC properties and recommend the property matching the project domain.
- Persist the selected property and connected account metadata.
- Add reconnect, revoke, expired-token, denied-consent, and no-matching-property states.
- Keep service-account configuration as internal/admin fallback only.

### Phase 5: OAuth-powered GSC Analysis

- Use the selected OAuth-backed GSC property as the default source for first-party search data.
- Backfill and sync GSC search performance.
- Generate opportunity types from real query/page metrics.
- Add stale, backfilling, and error states.

### Phase 6: GA4 and business-value prioritization

- Use existing GA4 storage fields in `page_performance_daily` and add missing ingestion/UI only as needed.
- Use engagement/conversion as prioritization signal.
- Mark recommendations without GA4 as missing business-value signal, not as failed.

## 14. Acceptance Criteria

### Phase 1 IA

1. Sidebar no longer has a primary `SYSTEM` section.
2. Settings appears in the left bottom utility area under Docs.
3. Admin remains in the left bottom utility area.
4. Analysis appears as a distinct workflow area, separate from Content Plan.
5. `/projects/[id]/opportunities` redirects to Analysis or remains as a compatibility alias.
6. UI copy uses user-facing language and avoids provider or internal job terminology in default user surfaces.
7. Primary workflow pages default to action-first summaries, not flat lists of every generated artifact or diagnostic.

### Phase 2 Analysis surface

1. Content Plan does not show raw, unaccepted opportunities.
2. Analysis owns opportunity acceptance and dismissal.
3. Accepted opportunities carry evidence and reason into downstream production work.
4. Empty states route users to one next action, not a grid of future modules.
5. Analysis can explain public-only, OAuth not connected, property missing, property selected, connected, stale, revoked, mismatch, and no-opportunity states.
6. Analysis shows prioritized recommendation cards by default; raw evidence and full signal tables are collapsed behind explicit disclosure controls.
7. Completed, dismissed, or snoozed opportunities are not visible in the default queue unless they become relevant again.

### Phase 3 Results surface

1. Results / Visibility no longer owns opportunity acceptance.
2. Results shows published action outcomes, measurement windows, and waiting/inconclusive/positive/negative states.
3. Home does not become a full analytics page after GSC is connected.
4. Results summarizes outcomes and exceptions by default; long per-URL or per-query detail is hidden behind measurement detail views.

### Phase 4 Self-serve GSC OAuth onboarding

1. Home displays a clear Connect Search Console gate when first-party search data is missing.
2. Project owners/admins can start Google OAuth from Home, Analysis, or Settings.
3. Users who deny consent, lack a matching property, or lose token access receive a recoverable state.
4. CiteLoop lists authorized GSC properties and recommends the best match for the project domain.
5. Users must confirm the selected property before the first backfill begins.
6. Non-admin users do not hit an admin-only dead end when clicking the Analysis search-data CTA.
7. Service-account connection remains available only as internal/admin fallback.

### Phase 5 GSC search data activation

1. Backfilling, stale, connected, revoked, and mismatch states are derived from real OAuth/integration/run state.
2. OAuth-backed GSC data writes into the existing SEO operations data model or an explicitly compatible extension.
3. Analysis can generate low CTR, near page-one, content decay, and indexing opportunities from real query/page metrics.
4. Accepted OAuth-backed opportunities carry property, query/page evidence, baseline window, and measurement window downstream.

## 15. Risks

1. **Analysis becomes another overloaded dashboard.**
   Mitigation: keep default view to status, weekly brief, opportunity queue, evidence inspector.

2. **Pages become flat walls of generated content.**
   Mitigation: enforce progressive disclosure. Default views show only current status, next action, required approvals, and blockers; diagnostics, history, and raw evidence stay collapsed.

3. **Content Plan loses context after isolation.**
   Mitigation: accepted work must carry evidence summary and source opportunity.

4. **Users without GSC feel blocked.**
   Mitigation: public-only mode remains useful and clearly labeled.

5. **GSC property mismatch creates wrong recommendations.**
   Mitigation: show selected property label, canonical domain match confidence, and mismatch warnings.

6. **OAuth implementation expands scope beyond IA.**
   Mitigation: split IA, OAuth onboarding, and OAuth-powered analysis into separate rollout phases. Phase 1-3 can ship before OAuth, but Phase 4 makes self-serve onboarding first-class.

7. **Google OAuth verification or consent setup delays launch.**
   Mitigation: start with the minimum read-only Search Console scope, prepare clear consent-screen copy, and keep internal service-account fallback for controlled pilots.

8. **Token storage or revocation handling creates security risk.**
   Mitigation: encrypt refresh tokens, store only required metadata, support explicit disconnect, and treat revoked/expired tokens as recoverable connection states.

9. **Permission model is unclear for team projects.**
   Mitigation: project owners/admins can connect and disconnect GSC; other members can view status and ask an admin to connect.

10. **Visibility to Results rename causes migration confusion.**
   Mitigation: keep route compatibility or redirect old visibility route during rollout.

11. **Too many action types overwhelm users.**
   Mitigation: group recommendations by job-to-be-done, not internal type.

## 16. Product Success Metrics

These measure whether the IA change works without promising SEO rankings:

1. Opportunity acceptance rate: percentage of open Analysis opportunities accepted or dismissed within 7 days.
2. Time from signal to action: median time from opportunity creation to accepted `content_actions` record.
3. Self-serve connection completion: percentage of eligible projects that complete OAuth, select a property, and start first backfill.
4. Search data connection readiness: percentage of active projects in `gsc_connected`, `gsc_backfilling`, or an explicitly understood public-only state.
5. Content Plan cleanliness: percentage of Content Plan items with a source opportunity or manual seed reason.
6. Results coverage: percentage of published actions with a measurement window and at least one recorded outcome state.

## 17. Product Decisions

1. Sidebar label is `Analysis`.
   The page title may use `Search Intelligence` to describe the specific job, but the workflow label remains `Analysis`.

2. `Opportunities` is removed from the primary sidebar.
   The opportunity queue becomes the primary working area inside Analysis.

3. `Visibility` becomes `Results` in the visible product IA.
   The implementation may keep the old `/visibility` route as a compatibility alias during rollout.

4. Analysis owns the connect/reconnect CTA for Search Console.
   Settings owns detailed connection management, revocation, diagnostics, and advanced configuration.

5. Technical SEO tasks stay inside Content Plan as `technical_task`.
   A separate Tasks page is out of scope until task volume proves Content Plan cannot carry these actions cleanly.

6. Default Search data connection model is self-serve end-user Google OAuth.
   Service account remains supported only as internal/admin fallback or migration bridge.

7. The first customer path is OAuth-first.
   A user should be able to create a project with a domain, connect their own GSC property, and reach Analysis without operator setup.

8. Default surfaces are collapsed by design.
   User-facing pages should show what needs attention now. Evidence, diagnostics, completed work, raw records, and generated intermediate content should be available but hidden by default.
