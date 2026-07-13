# PRD: CiteLoop Analysis Workflow and Dashboard Isolation

> 日期: 2026-06-24
> Last updated: 2026-07-12
> 状态: Draft
> 范围: Dashboard 信息架构、analysis workflow、Search Console 接入入口、analysis 与 Content Generation 的产品边界
> 上游文档:
> - `docs/PRD-CiteLoop-Dashboard-Control-Center-Redesign.md`
> - `docs/PRD-CiteLoop-SEO-Operations-Loop.md`
> - `docs/PRD-CiteLoop-Multi-Surface-SEO-Growth-Layer.md`

## 0. 摘要

CiteLoop 当前的核心能力已经覆盖内容生成、审核、发布和部分可见性反馈。代码库也已经包含 SEO operations loop 的主要数据模型和一部分页面能力。随着 Google Search Console / GA4 驱动的真实 SEO 数据进入产品，Dashboard 需要把 "分析机会" 和 "生产内容" 明确隔离。

本 PRD 的目标不是从零新增一套分析系统，而是把现有 Opportunities / Visibility / SEO data layer 产品化为 Opportunities 内部的 analysis workflow:

```text
Context -> Opportunities -> Content Plan -> Review -> Publish -> Measure
```

Opportunities 中的 analysis capability 负责呈现公开数据、GSC/GA4 私有数据、SERP/GEO 信号，并把这些信号转化为可解释的 opportunity 和 action recommendation。Content Plan 只接收从已接受 AI-generated Opportunity 派生、且 materially creates or refreshes content 的工作，不再承担原始分析、数据连接、机会判断或新工作创建职责。

这让 CiteLoop 从 "content generation dashboard" 升级为 "analysis to action to publishing loop"，同时避免用户把所有 SEO 数据、机会、选题和内容草稿混在一个页面里理解。

### 0.1 Canonical Supersession Note

As of 2026-07-12, `PRD-CiteLoop-Doctor-Opportunities-Two-Line-Optimization.md` controls the new-work boundary: Doctor and Opportunities are the only user-visible sources, Opportunities is canonical at `/projects/[id]/opportunities`, and Content Plan receives only accepted AI-generated Opportunities that materially create or refresh content. A user-triggered AI run remains allowed, while manual Topic/Opportunity/Brief intake does not. Existing source-less Topics keep only the backward-compatible operations defined in the Content Plan legacy matrix and are never cloned, backfilled, or migrated into new work.

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

3. **Content Generation 被迫承接 analysis 缺口。**
   当系统不知道真实搜索表现时，Content Plan 只能像 topic backlog。接入 GSC 后，应该由 Opportunities 的 analysis capability 判断 "为什么做这件事"，再把被接受的 action 交给 Content Plan 或其他执行页面。

ChatSEO 的定价结构也印证了这个边界: analysis 是独立价值层，Content 是执行层。CiteLoop 不应只复制 ChatSEO 的分析助手，而应把分析结果接入已有的 content / review / publish / measure loop。

### 1.1 当前代码基线

截至 `main@0001e063951bf06b931811efd33ae7e15d4d605a`，以下能力已经存在，实施本 PRD 时应复用而不是重建:

- `internal/migrations/0007_seo_operations_loop.sql` 已创建 `seo_properties`、`seo_integrations`、`seo_runs`、`search_performance_daily`、`page_performance_daily`、`search_appearance_daily`、`url_index_snapshots`、`technical_checks`、`seo_opportunities`、`content_actions`。
- `page_performance_daily` 已包含 `ga4_sessions`、`ga4_engaged_sessions`、`ga4_conversions` 字段，GA4 不应被当作完全 greenfield storage。
- `seo_opportunities` 已包含 `type`、`status`、`priority_score`、`confidence`、`evidence`、`recommended_action`、`expected_impact`、`effort`、`risk_level`、`opportunity_key` 等 analysis capability 需要的核心字段。
- `content_actions` 已经承担 opportunity -> action -> article/result 的桥接，包含 `baseline_window`、`measurement_window`、`outcome_summary`。
- `web/app/projects/[id]/opportunities/page.tsx` 和 `web/app/projects/[id]/visibility/page.tsx` 已经分别渲染 `OpportunitiesClient` 与 `VisibilityClient`，两者共享 `web/app/projects/[id]/seo/seo-client.tsx` 中的 `SEOClient`。
- 当前 Google 数据连接由 `internal/googledata/auth.go` 的 service-account JWT 实现，scope 为 `webmasters.readonly` 和 `analytics.readonly`。当前代码没有 end-user Google OAuth consent、用户可访问 property 列表、GSC refresh token 捕获。本 PRD 将 self-serve GSC OAuth 明确列为需要新增的 connection layer。
- 当前 Publisher layer 已经包含 `Publisher` interface、GitHub/Next.js blog publisher、semi-manual distribution lane、`publisher_connections` 和 `publisher_credentials`。当前代码没有 WordPress、Webflow、Shopify 或 Wix 真实 CMS connector。

因此 Phase 1-3 应被视为 IA 重构和页面职责拆分：`Opportunities` 保持用户可见名称与 canonical route，在页面内承载 analysis capability；`Visibility` 收敛为 `Results`；共享 `SEOClient` 拆成更清楚的 Opportunities / Results surfaces。

## 2. 产品目标

1. 在 Dashboard 中将现有 Opportunities / Visibility 能力重组为 Opportunities 内部的 analysis workflow，并与 Content Generation 隔离。
2. 让用户通过 domain-first onboarding 开始项目，再通过 self-serve Google Search Console OAuth 解锁真实搜索分析。service account 保留为 internal/admin fallback，不作为默认客户路径。
3. 把 GSC/GA4 信号转化为可解释、可接受、可路由的 SEO/GEO actions。
4. 让 Content Plan 只展示从已接受 AI-generated Opportunity 派生的生产工作，不展示未筛选的原始机会或 source-less 新工作。
5. 让 Measure 页面专注发布后的结果和闭环反馈，不再混入原始机会发现。
6. 简化 sidebar，把 Settings 移到左下角 Docs 下方，Admin 保持左下角入口，移除主导航中的 SYSTEM 分组。
7. 保持 Home 作为控制中心，只展示当前最重要的状态、下一步和数据连接 gate。
8. 降低默认页面的信息负担，只展示用户需要知晓、决策、批准或处理的内容。
9. 把 Content Generation 闭环扩展到 CMS draft / update / publish，以 WordPress 作为第一个 self-serve CMS integration。

## 3. 非目标

- 不在本 PRD 的 Phase 1-3 中实现 end-user Google OAuth、GSC ingestion 或 GA4 ingestion 的完整后端方案。Self-serve GSC OAuth 从 Phase 4 开始进入本 PRD 范围。
- 不把当前 service-account 连接模型伪装成用户 OAuth。两者必须在 UI、权限和文档中明确区分。
- 不在本 PRD 中重写现有 Review、Publish、Publisher 或 Article detail 页面。
- 不把 Opportunities 的 analysis capability 做成全量 SEO dashboard 或 Semrush/Ahrefs 替代品。
- 不在 WordPress MVP 中默认自动发布未经批准的内容或站点改动。
- 不在同一阶段同时实现 WordPress、Webflow、Shopify、Wix 的完整 connector。WordPress 是第一优先级，其余平台进入后续 connector roadmap。
- 不自动执行高风险 SEO 动作，例如 redirect、noindex、delete、merge。
- 不承诺排名、流量、转化或 AI answer citation 提升。
- 不引入普通用户必须理解的 GSC property、OAuth scope、credential ref、GA4 property id 等工程概念。

## 4. 核心原则

### 4.1 analysis owns why

Opportunities 的 analysis capability 回答:

- 为什么这个机会存在?
- 证据来自哪里?
- 这个动作优先级为什么高?
- 不做会损失什么?
- 做完后如何衡量?

### 4.2 Content Plan owns what gets produced

Content Plan 回答:

- 哪些 AI-generated Opportunities 已经被接受并进入生产?
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

2026-07-12 更新后的项目 sidebar:

```text
Home
Context

Work sources
Doctor
Opportunities

Execution
Content Plan
Review
Publish

Outcomes
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
- `Doctor` 与 `Opportunities` 是顶层用户可见工作源；analysis 是 Opportunities 内部的 capability，不是第三条 queue。
- `Visibility` 重命名或收敛为 `Results`，专注 Measure 阶段。

### 5.2 页面职责

| 页面 | 主要职责 | 不应该承担 |
|---|---|---|
| Home | 当前状态、下一步、连接 gate、loop health | 全量 analytics、完整机会列表 |
| Context | domain 理解、产品定位、证据、竞争对手、内容规则 | 搜索表现分析、内容生产排期 |
| Doctor | 可立即验证的 finding、Site Fix 与 verification | 延迟增长机会、Content Brief |
| Opportunities | GSC/GA4/SERP/GEO 分析能力、AI-generated Opportunity queue、action recommendation | 内容草稿编辑、发布状态复盘 |
| Content Plan | 已接受 action 的生产 backlog、brief、schedule、generation intent、CMS draft/update intent | 原始 SEO 数据探索 |
| Review | 草稿是否可发布、证据是否充分、QA blocking | 机会优先级判断 |
| Publish | CMS draft/update approval、canonical publish、variant unlock、publish failure、URL verification | 数据分析和机会发现 |
| Results | 已发布 action 的 measurement、traffic/citation/CTR/position outcome | 未接受 opportunity queue |

### 5.3 Page UX contract

Every primary workflow page should answer one user question first:

```text
What do I need to do now?
```

The default page layout should follow the same hierarchy:

1. Current status or page-specific state.
2. The next user action, if any.
3. Items that need approval, decision, or recovery.
4. Relevant summary context.
5. Collapsed details, history, raw evidence, diagnostics, and completed work.

No page should default to a flat table of everything the system knows.

| Page | Primary user question | Default top section | Primary CTA | Hidden by default |
|---|---|---|---|---|
| Home | What needs my attention across the project? | One next step, connection gates, blockers, loop health | Continue / Review / Connect | Full analytics, full opportunity queue, logs |
| Context | Is CiteLoop using the right understanding of my product? | Product summary, audience, competitors, rules needing confirmation | Confirm / Edit context | Raw crawl notes, scraped page lists, old context versions |
| Opportunities | What search-driven growth move needs my decision? | Search performance cockpit, growth findings, compact decision queue, and measurement snapshot | Create task / Dismiss / Watch / Connect GSC | Raw GSC rows, full evidence tables, diagnostics, completed/dismissed opportunities, large recommendation-card feeds |
| Content Plan | What accepted content work is planned next? | Accepted material-content work that is ready, scheduled, blocked, or needs approval | Generate / Edit / Send to Review | Unaccepted opportunities, non-content actions, generated intermediates, completed history |
| Review | What needs my approval before it can move forward? | Drafts or CMS updates requiring human approval | Approve / Request changes / Block | Generation logs, full scoring details, previous revisions |
| Publish | What is ready to publish, update, retry, or verify? | Approved items ready for CMS publish/update and publish failures | Publish / Retry / Verify URL | Provider API responses, post IDs, credential refs, old publish attempts |
| Results | What changed after publishing? | Outcome summaries, measurement windows, positive/negative/inconclusive exceptions | Open details / Find next Opportunity | Long per-query/per-URL tables, low-signal history, raw measurement rows |
| Settings | What connections or project settings do I need to manage? | Connection health and configuration groups | Connect / Reconnect / Disconnect | Advanced diagnostics, credentials, raw provider payloads |

For GSC-specific analysis inside Opportunities:

| State | Opportunities default experience |
|---|---|
| No GSC | Public analysis mode, compact red Search Console status menu, public findings, clear connection path to Settings |
| GSC missing property | GSC setup assistant with DNS / URL-prefix / admin handoff paths |
| GSC connected but backfilling | Compact connected/backfilling status, search snapshot with no fake precision, public findings still usable |
| GSC connected with data | Search performance cockpit, query/page findings, compact decision queue, evidence collapsed |
| GSC stale/revoked/mismatch | Compact red recovery status first; last-known data only if clearly labeled |

For every page, the first viewport should be enough for a non-SEO user to decide whether they need to act now.

## 6. analysis Capabilities Within Opportunities

### 6.1 用户路径

目标客户路径基于 self-serve Google Search Console OAuth:

```text
1. 用户输入 product domain
2. CiteLoop 完成 public discovery
3. Home 和 Opportunities 显示 Search data gate
4. 项目 owner/admin 点击 Connect Google Search Console
5. Google OAuth consent 返回该用户可访问的 GSC properties
6. CiteLoop 自动推荐与 product domain 匹配的 property
7. 用户确认 property，或手动选择正确 property
8. CiteLoop 安全存储 OAuth refresh token 和 selected property
9. 系统 backfill 最近 90 天搜索数据
10. analysis pipeline 生成 candidates 并执行 Doctor / Opportunities ownership arbitration
11. 可立即验证的 candidate 进入 Doctor；AI-generated growth candidate 进入 Opportunities
12. 用户接受、暂缓或 dismiss Opportunity
13. 被接受且 materially creates or refreshes content 的 Opportunity 进入 Content Plan；pure link-only growth strategy 创建 Growth Action；Doctor work 进入 Site Fixes
14. 发布或执行后分别进入 Results measurement 或 Doctor immediate verification
```

Internal/admin fallback 路径继续支持 service account，但不作为默认客户 onboarding:

```text
1. Operator 在 Settings / Admin 中配置 `site_url`、`gsc_site_url`、`credential_ref`
2. 系统用 service-account JWT 读取 GSC / GA4 数据
3. UI 明确显示为 internal/admin-managed connection
4. 用户仍然在 Opportunities 中查看 AI-generated Opportunities、接受 action 和进入 Results measurement
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

### 6.3 Opportunities 页面结构

```text
Opportunities
├─ Page header
│  ├─ "What should I act on now?" framing
│  ├─ Refresh / Sync actions
│  └─ Compact Search Console status control, top-right
│     ├─ green when connected / backfilling
│     ├─ red when not connected, stale, revoked, mismatch, or property missing
│     ├─ dropdown shows selected property, data mode, last/next action, and source confidence
│     └─ dropdown links to Settings -> Search Console connection
├─ Search performance snapshot
│  ├─ clicks, impressions, CTR, average position, observed GSC days
│  ├─ confidence mode: connected, low click depth, backfilling, public-only, stale
│  └─ no fake precision when GSC is absent or still backfilling
├─ Growth findings
│  ├─ compact row/list layout, not large recommendation cards
│  ├─ grouped by job-to-be-done: low CTR, striking distance, decay, query/page mismatch, cold-start, GEO gap; immediate indexing work links to Doctor
│  ├─ each row shows source page/query, signal, priority, risk, and "Review" affordance
│  └─ row expansion or drawer reveals evidence, query/page metrics, source, confidence, and reasoning
├─ Decision queue
│  ├─ compact list of items needing user decision
│  ├─ action-specific CTAs: Create content task, Create refresh task, Open in Doctor, Watch, Dismiss
│  └─ accepted material-content Opportunities route to Content Plan; measured pure link strategies create Growth Actions; Doctor-owned work routes to Doctor/Site Fixes with evidence attached
├─ Measurement snapshot
│  ├─ actions already in plan/publish/results
│  ├─ D+7 / D+14 / D+28 checkpoints when available
│  └─ waiting, positive, negative, inconclusive summaries
└─ analysis brief and diagnostics
   ├─ collapsed by default
   ├─ weekly brief, blockers, GEO diagnostics, raw evidence, and provider details
   └─ opened only through deliberate disclosure
```

Opportunities default view should not be a wall of recommendation cards. The first viewport should use the available space to answer:

- is real search data connected?
- what changed in search performance?
- which findings need a decision now?
- what action will happen if I approve?
- how will the result be measured?

Recommendation details remain available, but only after the user opens a row, drawer, or `View evidence` / `Why this?` control. Raw evidence, query lists, diagnostic details, generated reasoning, completed actions, and provider payloads are hidden by default.

### 6.3.1 Compact Search Console status control

The Search Console connection state belongs in the Opportunities header, not in a large top-of-page card.

Connected state:

```text
[green status] Search Console connected
```

Disconnected or unhealthy state:

```text
[red status] Search Console not connected
```

Clicking the status opens a lightweight dropdown:

- connection label: connected, backfilling, public-only, stale, revoked, mismatch, or property missing
- selected property label when available, for example `sc-domain:unipost.dev`
- current data mode, for example `First-party search data`, `Low click depth`, `Public crawl only`
- whether Opportunities analysis is using live GSC data, last-known data, or public-only data
- one primary route to `Settings -> Search Console connection`
- recovery action when the state is stale, revoked, mismatched, or missing

The dropdown is informational and navigational. It should not become a second Settings page.

### 6.3.2 Growth findings and decision queue behavior

Growth findings are the main body of Opportunities. They should be dense enough for repeated weekly use and should not behave like a blog-feed of generated cards.

Each finding row should show:

- finding type in user language
- page or query being affected
- concise reason
- source mode: GSC, public crawl, GEO, or mixed
- priority score or rank
- risk/confidence
- next user action

Decision queue is the user's "what do I approve now?" panel. It should show only active decisions and use action-specific CTAs:

| Finding / action type | Primary CTA |
|---|---|
| new asset or missing content | `Create content task` |
| content decay or near page-one refresh | `Create refresh task` |
| low CTR title/meta work | `Create refresh task` or `Create metadata task` |
| immediately verifiable indexing, sitemap, link integrity, or schema issue | `Open in Doctor` |
| measured internal-link growth strategy | `Create Growth Action` |
| insufficient data but worth monitoring | `Watch` |
| not relevant | `Dismiss` |

No default Opportunities CTA should imply that every accepted item is a new content-generation job.

### 6.4 Finding And Opportunity Taxonomy

The 2026-07-12 owner contract applies before a work record is created: immediately verifiable technical candidates become Doctor findings, while delayed-growth candidates may become AI-generated Opportunities. The table keeps the older evidence taxonomy but no longer treats every row as an Opportunity source.

| Candidate family | Primary evidence | Recommended action | Destination |
|---|---|---|---|
| `low_ctr_page` | impressions high, CTR below expected range | Draft title/meta update | Accepted AI Opportunity -> Content Plan (`metadata_rewrite`) or Review |
| `near_page_one_query` | average position 8-20 with meaningful impressions | Create a material content refresh brief or a measured internal-link Growth Action | Accepted AI Opportunity -> Content Plan only for material content refresh; pure link-only strategy -> Growth Action |
| `content_decay` | clicks/impressions/position decline over comparison window | Refresh existing page | Accepted AI Opportunity -> Content Plan |
| `missing_content_asset` | query cluster has impressions but no dedicated asset | Create new asset brief | Accepted AI Opportunity -> Content Plan |
| `internal_link_gap` | source pages can support target page | Fix broken/zero links immediately, or propose a measured cluster strategy | Doctor -> Site Fix for immediately verifiable link patch; accepted AI Opportunity -> Growth Action for delayed-growth link strategy; pure link-only work never becomes a Content Brief |
| `indexing_issue` | URL absent, excluded, or sitemap mismatch | Create immediately verifiable technical SEO fix | Doctor -> Site Fixes |
| `geo_citation_gap` | competitor cited in AI answer, project absent | Create citation-ready asset | Accepted AI Opportunity -> Content Plan |
| `backlink_or_mention_gap` | public market data or backlink provider signal | Create outreach or evidence asset | Accepted AI Opportunity -> external handoff, or Content Plan when it creates content |

### 6.4.1 Existing type mapping

`seo_opportunities.type` is stored as text, not a database enum, but existing analyzers already emit specific names. Implementation should map product taxonomy to existing types where possible and only introduce new type strings with explicit tests and migration notes.

| Product taxonomy | Existing or proposed `seo_opportunities.type` | Migration/API impact |
|---|---|---|
| `indexing_issue` | `indexing_anomaly` | Legacy type; new immediately verifiable work is owned by Doctor and retained here only for compatibility/migration mapping. |
| `geo_citation_gap` | `geo_competitor_cited_project_absent`, `geo_project_mentioned_without_citation` | Existing GEO analyzer types; group in UI. |
| crawler access issue | `geo_crawler_access_blocked` | Existing type; group under technical / GEO blockers. |
| public cold-start content | `cold_start_context_plan`, `cold_start_competitive_gap`, `cold_start_evidence_page` | Existing types; keep in public-only Opportunities analysis. |
| `low_ctr_page` | `low_ctr_page` | New generator/type if not already emitted. Requires tests and copy mapping. |
| `near_page_one_query` | `near_page_one_query` | New generator/type if not already emitted. Requires tests and copy mapping. |
| `content_decay` | `content_decay` | New generator/type if not already emitted. Requires tests and copy mapping. |
| `internal_link_gap` | `internal_link_gap` | New generator/type if not already emitted. Requires tests and action routing. |
| `backlink_or_mention_gap` | `backlink_or_mention_gap` | Future provider-dependent type; not Phase 1. |

### 6.5 Action routing

The analysis pipeline must not assume every candidate creates an Opportunity or that every accepted Opportunity creates a new article.

| Recommended action | Route |
|---|---|
| `create_new_asset` | Accepted AI Opportunity -> Content Plan |
| `refresh_existing_page` | Accepted AI Opportunity -> Content Plan |
| `draft_title_meta_update` | Accepted AI Opportunity -> Content Plan or Review, depending on publisher capability |
| `add_internal_links` | Doctor -> Site Fix when the patch is immediately verifiable; accepted AI Opportunity -> Growth Action when success requires a growth window; never Content Plan unless the accepted scope materially creates or refreshes content |
| `create_schema_update` | Doctor -> Site Fixes |
| `fix_indexing_issue` | Doctor -> Site Fixes |
| `wait_and_measure` | Results watchlist |
| `dismiss` | Opportunities archive |

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
4. New AI-generated Opportunities that need acceptance.
5. Publisher/CMS connection missing, if accepted work needs CMS draft or publish.
6. Content Plan / Review / Publish actions.
7. Results waiting for measurement.

## 8. Content Plan Isolation

Every newly created Content Plan item must come from an accepted AI-generated Opportunity whose scope materially creates or refreshes content.

Allowed Content Plan items:

- accepted AI-generated Opportunity for a new content asset
- accepted AI-generated Opportunity for a content refresh
- accepted AI-generated Opportunity for an editorial metadata rewrite
- accepted AI-generated Opportunity for a GEO evidence asset
- scheduled or generated draft derived from one of those accepted Opportunities

Not allowed in Content Plan:

- raw GSC rows
- unreviewed opportunities
- rejected opportunities
- search data connection prompts
- provider diagnostics
- broad visibility score explanations
- generated intermediate artifacts that do not require review
- completed items unless the user opens Completed or History
- a newly created item without an accepted source Opportunity ID and AI provenance
- a user-authored Topic, Opportunity, or Content Brief intake record
- any pure link-only action, whether owned by Doctor or Opportunities

Content Plan default view should prioritize:

- work that needs approval
- work scheduled next
- blocked work

Completed work, in-progress automation details, and generated supporting artifacts should be summarized, not laid out flat.

Historical source-less Topics are not allowed-source exceptions. Per the Content Plan Section 15.4 matrix, existing records retain view/edit, cancel/reschedule, draft/generate, and archive/dismiss in a labeled compatibility surface. They remain excluded from the current queue and source-integrity metrics and cannot be created, cloned, backfilled, migrated, or used to seed new work.

When a material-content AI-generated Opportunity is accepted into Content Plan, Content Plan should show the reason in user language:

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

Results should not be the place where users decide whether to accept raw opportunities. That belongs in Opportunities.

Results default view should show outcome summaries and exceptions. Long per-URL measurement details, historical rows, and inconclusive low-signal records should be collapsed behind measurement detail views.

## 10. Publisher and CMS Integration Layer

ChatSEO's pricing page makes the Content value layer concrete: AI SEO writer plus CMS publishing, editing, and updating. CiteLoop should borrow that product boundary, but implement it through the existing Publisher / `publisher_connections` layer rather than creating a separate CMS workflow.

### 10.1 Product goal

The Content side of CiteLoop should support three jobs:

- create a new asset from an accepted AI-generated Opportunity
- update or optimize an existing CMS page
- publish or stage the approved change in the user's destination system

Content Generation should not stop at "draft created". The complete loop is:

```text
Opportunities -> accepted action -> draft/update -> user approval -> CMS publish/update -> URL verification -> Results measurement
```

### 10.2 CMS priority

CMS integrations should be sequenced by reach and implementation risk:

| Priority | Platform | Product status | Rationale |
|---|---|---|---|
| 1 | WordPress | MVP target | Broadest customer fit; supports posts/pages, drafts, updates, slugs, and metadata through mature APIs. |
| 2 | Webflow | Next connector | Strong fit for SaaS marketing sites, but content model/collection mapping adds setup complexity. |
| 3 | Shopify | Later connector | Useful for ecommerce SEO, but product/blog/page permissions and templates are more specialized. |
| 4 | Wix | Later connector | Useful for SMBs, but API and content model constraints should be validated after WordPress/Webflow. |

### 10.3 WordPress MVP scope

WordPress should launch as a gated publish/update flow:

- connect WordPress from Settings or onboarding checklist
- select site and default content destination
- create draft post or page
- update existing post or page
- set title, slug, excerpt/meta description when supported
- preserve canonical URL and published URL mapping
- require user approval before publishing or updating live content
- verify the published/updated URL after publish
- route measurement back to Results

WordPress MVP should not include:

- default auto-publish without approval
- theme/template editing
- plugin installation
- bulk publishing
- destructive delete
- rollback unless explicitly implemented and tested

### 10.4 CMS capability matrix

The product should show readiness by capability, not by vague "connected" status.

| Capability | WordPress MVP | Webflow later | Shopify later | Wix later |
|---|---|---|---|---|
| Create article/post | Yes | Later | Blog only, later | Later |
| Create page | Yes | Later | Limited, later | Later |
| Update existing content | Yes | Later | Limited, later | Later |
| Metadata update | Yes, when supported | Later | Product/blog meta, later | Later |
| Slug update | Yes | Later | Limited, later | Later |
| Draft mode | Yes | Later | Later | Later |
| Gated publish | Yes | Later | Later | Later |
| Media upload | Later | Later | Later | Later |
| Delete | Out of scope | Out of scope | Out of scope | Out of scope |
| Rollback | Later | Later | Later | Later |

### 10.5 User-facing CMS states

Default user-facing states should stay simple:

| State | User-facing copy | Behavior |
|---|---|---|
| `publisher_not_connected` | Connect WordPress to publish approved work | Show connect CTA and keep work draft-only |
| `publisher_connected_draft_only` | WordPress connected for drafts | Create drafts, require manual publish or approval |
| `publisher_ready_for_gated_publish` | WordPress ready for approved publishing | Allow approved publish/update actions |
| `publisher_needs_reconnect` | Reconnect WordPress | Keep drafts and measurements; block publish/update |
| `publisher_publish_failed` | Publish needs attention | Show one recovery action and collapse raw error detail |
| `publisher_verifying_url` | Verifying published URL | Wait for URL/canonical/indexability checks |
| `publisher_published_measuring` | Published and measuring | Move default visibility to Results |

Technical details such as WordPress post IDs, REST responses, credential references, API errors, and sync logs should be hidden behind `View publish details`.

### 10.6 Content actions supported by CMS

CMS integration should support more than new article creation:

| Action | WordPress behavior | Default user gate |
|---|---|---|
| `create_new_asset` | Create draft post/page | Approval before publish |
| `refresh_existing_page` | Update existing post/page draft or staged revision | Approval before live update |
| `draft_title_meta_update` | Update title/excerpt/meta fields when available | Approval before live update |
| `add_internal_links` | Stage content update with proposed links | Approval before live update |
| `create_schema_update` | Out of scope for WordPress MVP unless supported safely | Doctor -> Site Fixes |
| `fix_indexing_issue` | Out of scope for direct CMS write unless it is content/meta-related | Doctor -> Site Fixes |

### 10.7 Settings ownership

Settings owns CMS connection management:

- connect / reconnect / disconnect WordPress
- choose default content destination
- configure draft vs gated publish behavior
- inspect capability readiness
- view diagnostics

Content Plan owns accepted material-content production work. Growth Actions remain outside Content Plan. Publish owns approval, publish/update execution, retry, and URL verification. Results owns measurement after the CMS change is live.

## 11. Settings and Admin Placement

### 11.1 Settings

Settings moves to the left utility area under Docs.

Settings owns:

- project configuration
- publisher connections
- WordPress / CMS connection management
- Search Console OAuth connection management
- GA4 connection management
- notification channels
- crawl settings
- advanced diagnostics

Settings should not be a primary workflow step.

Search data connection management should be available to project owners/admins through a self-serve Google OAuth flow. Opportunities may show the CTA to everyone, but members without integration-management permission should receive an explanation and an admin handoff path. Internal operators may still configure service-account access through Admin/diagnostics, but that path should be labeled as internal-managed and should not replace the default OAuth onboarding.

### 11.2 Admin

Admin remains in the left utility area. It should not appear in the main workflow navigation.

Admin owns:

- internal-only tools
- privileged maintenance
- raw diagnostics
- operator utilities

### 11.3 Sidebar grouping

The `SYSTEM` section is removed from the primary nav. Utility links are visually separated at the bottom and should not compete with daily workflow pages.

## 12. Empty States

### 12.1 Opportunities empty state before GSC

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

### 12.2 Opportunities empty state after GSC but no opportunity

Primary message:

```text
No search actions need review
CiteLoop will keep watching query, page, CTR, position, and content decay signals.
```

Secondary content:

- last sync time
- number of pages observed
- next scheduled analysis

### 12.3 Content Plan empty state

Primary message:

```text
No accepted work yet
Review AI-generated Opportunities and accept one to create the next production item.
```

Primary action:

```text
Open Opportunities
```

## 13. Data and API Implications

This PRD does not require final schema design, but implementation should preserve these product boundaries.

### 13.1 Existing concepts to preserve

- `content_actions` remains the action lifecycle owner.
- `articles` remains generated content output.
- `seo_opportunities` or equivalent remains the raw/accepted opportunity source.
- `publisher_connections` remains publishing capability state.
- `publisher_credentials` remains publishing credential storage.
- `seo_properties` / integration records should express data readiness.

Implementation must not create replacement tables for these concepts during Phase 1-3. If Opportunities needs a new UI shape, adapt API serializers or view models over the existing tables first.

### 13.2 OAuth connection model

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

### 13.3 Required product states

Opportunities needs enough API surface to express:

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

Publisher states should be derived from `publisher_connections`, `publisher_credentials`, capability readiness, credential health, last publish/update error, and URL verification status. They should not require users to understand provider-specific IDs or API responses.

### 13.4 Downstream contract

When Opportunities routes accepted work downstream, it must create or update a durable Content Action with:

- action type
- source Opportunity ID
- AI provenance: discovery/generation run ID, provider/model, prompt ID/version, and evidence snapshot/reference
- one acceptance timestamp and approval source (the accepting user or policy identity when available)
- evidence summary
- target page or new asset type
- risk level
- status
- expected measurement dates
- publisher destination and capability requirements when the action needs CMS create/update/publish

For a new Content Brief, the complete logical provenance chain is the source Opportunity ID, resulting source Content Action ID, AI run/model/prompt/evidence provenance, one acceptance timestamp and approval source, and internal Topic linkage when one exists. These are logical relationships, not promises that every value is a physical column on one table; serializers may resolve them through the linked Opportunity, Content Action, AI call/run records, evidence snapshots, and internal Topic.

## 14. Rollout Plan

### Phase 1: Information architecture only

- Update sidebar grouping.
- Move Settings to bottom utility area.
- Keep Admin bottom utility only.
- Keep `Opportunities` as the canonical user-visible growth-work source.
- Keep `/projects/[id]/opportunities` as the canonical route; any existing `/analysis` route is a compatibility alias to Opportunities.
- Add Doctor as the other user-visible work source.
- Move opportunity review entry points out of `Visibility` and into `Opportunities`.
- Keep existing backend APIs.

### Phase 2: Opportunities page productization

- Add compact Search Console status control derived from existing `seo_properties` / `seo_integrations` / SEO run state.
- Add GSC connection gate.
- Add search performance snapshot.
- Add Growth findings list.
- Add compact Decision queue.
- Add Measurement snapshot.
- Keep weekly analysis brief and evidence inspector collapsed by default.
- Ensure accepted material-content opportunities route to Content Plan and measured pure link strategies create Growth Actions instead.

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

### Phase 5: OAuth-powered GSC analysis in Opportunities

- Use the selected OAuth-backed GSC property as the default source for first-party search data.
- Backfill and sync GSC search performance.
- Generate opportunity types from real query/page metrics.
- Add stale, backfilling, and error states.

### Phase 6: WordPress gated publish and update

- Add WordPress as the first self-serve CMS connector.
- Support draft creation for accepted new content actions.
- Support gated updates for existing WordPress posts/pages.
- Support title, slug, excerpt/meta description updates when available.
- Add publisher readiness states to Home, Content Plan, Publish, and Settings.
- Keep raw WordPress IDs, REST responses, and sync logs hidden behind publish details.
- Verify published/updated URLs and route outcomes to Results.
- Keep auto-publish disabled by default.

### Phase 7: GA4 and business-value prioritization

- Use existing GA4 storage fields in `page_performance_daily` and add missing ingestion/UI only as needed.
- Use engagement/conversion as prioritization signal.
- Mark recommendations without GA4 as missing business-value signal, not as failed.

### Phase 8: Additional CMS connectors

- Add Webflow after WordPress if the target customer segment needs SaaS marketing-site publishing.
- Add Shopify after Webflow if ecommerce SEO becomes a priority.
- Add Wix only after validating API constraints and customer demand.
- Each connector must expose capability readiness before it appears as a publish/update destination.

## 15. Acceptance Criteria

### Phase 1 IA

1. Sidebar no longer has a primary `SYSTEM` section.
2. Settings appears in the left bottom utility area under Docs.
3. Admin remains in the left bottom utility area.
4. Doctor and Opportunities appear as distinct work-source areas, separate from Content Plan.
5. `/projects/[id]/opportunities` remains canonical; `/analysis`, if present, is a compatibility alias.
6. UI copy uses user-facing language and avoids provider or internal job terminology in default user surfaces.
7. Primary workflow pages default to action-first summaries, not flat lists of every generated artifact or diagnostic.
8. Each primary workflow page follows the Page UX contract: status, next action, approval/recovery items, summary context, then collapsed details.

### Phase 2 Opportunities analysis surface

1. Content Plan does not show raw, unaccepted opportunities.
2. Opportunities owns opportunity acceptance and dismissal.
3. Accepted opportunities carry evidence and reason into downstream production work.
4. Empty states route users to one next action, not a grid of future modules.
5. Opportunities can explain public-only, OAuth not connected, property missing, property selected, connected, stale, revoked, mismatch, and no-opportunity states through its analysis capabilities.
6. Opportunities does not show a large connection status hero or a flat feed of large recommendation cards by default.
7. Opportunities shows compact Search Console status in the header, with green connected and red disconnected/unhealthy states.
8. The Search Console status dropdown shows connection detail and links to Settings -> Search Console connection.
9. Opportunities defaults to search performance snapshot, growth findings, compact decision queue, and measurement snapshot.
10. Raw evidence and full signal tables are collapsed behind explicit disclosure controls.
11. Completed, dismissed, or snoozed opportunities are not visible in the default queue unless they become relevant again.
12. Every newly created Content Plan item resolves the complete logical provenance chain: source Opportunity ID, source Content Action ID, AI run/model/prompt/evidence provenance, one acceptance timestamp and approval source, and internal Topic linkage when one exists.
13. Content Plan exposes no manual Topic, Opportunity, or Content Brief intake; its empty state links only to Opportunities.

### Phase 3 Results surface

1. Results / Visibility no longer owns opportunity acceptance.
2. Results shows published action outcomes, measurement windows, and waiting/inconclusive/positive/negative states.
3. Home does not become a full analytics page after GSC is connected.
4. Results summarizes outcomes and exceptions by default; long per-URL or per-query detail is hidden behind measurement detail views.

### Phase 4 Self-serve GSC OAuth onboarding

1. Home displays a clear Connect Search Console gate when first-party search data is missing.
2. Project owners/admins can start Google OAuth from Home, Opportunities, or Settings.
3. Users who deny consent, lack a matching property, or lose token access receive a recoverable state.
4. CiteLoop lists authorized GSC properties and recommends the best match for the project domain.
5. Users must confirm the selected property before the first backfill begins.
6. Non-admin users do not hit an admin-only dead end when clicking the Opportunities search-data CTA.
7. Service-account connection remains available only as internal/admin fallback.

### Phase 5 GSC search data activation

1. Backfilling, stale, connected, revoked, and mismatch states are derived from real OAuth/integration/run state.
2. OAuth-backed GSC data writes into the existing SEO operations data model or an explicitly compatible extension.
3. The analysis pipeline can generate low CTR, near page-one, and content decay Opportunities from real query/page metrics, while immediately verifiable indexing evidence creates Doctor findings.
4. Accepted OAuth-backed opportunities carry property, query/page evidence, baseline window, and measurement window downstream.

### Phase 6 WordPress gated publish and update

1. Project owners/admins can connect, reconnect, and disconnect WordPress from Settings.
2. WordPress readiness is shown by capability: draft creation, existing content update, metadata update, gated publish, and URL verification.
3. Accepted new-asset actions can create WordPress drafts.
4. Accepted refresh or metadata actions can stage updates to existing WordPress posts/pages.
5. No WordPress live publish/update happens without explicit user approval in MVP.
6. Publish failures show one recovery action by default and hide raw provider details behind `View publish details`.
7. Published/updated WordPress URLs flow into Results measurement.

### Phase 7 GA4 business-value prioritization

1. GA4 engagement/conversion fields in `page_performance_daily` are reused or extended compatibly.
2. Opportunities can mark recommendations as missing business-value signal when GA4 is absent.
3. GA4 signals influence priority only when the signal is available and fresh enough to trust.

### Phase 8 additional CMS connectors

1. Webflow, Shopify, and Wix are not presented as available destinations until their connector capability readiness is implemented.
2. Each connector uses the same Publisher contract and user-facing states as WordPress where possible.
3. Connector-specific limitations are shown as unavailable capabilities, not as generic failure states.

### Cross-phase end-to-end workflow

The complete workflow is not accepted only because each phase passes in isolation. After all implemented phases for a release candidate are complete, CiteLoop must pass an end-to-end user journey from the perspective of a new customer.

Required happy path:

1. User signs up or signs in.
2. User creates or selects a project.
3. User enters a product domain.
4. CiteLoop completes public discovery and shows the project-level next action.
5. User either connects GSC through OAuth or intentionally continues in public-only mode.
6. If GSC is connected, user selects or confirms the matching property and sees backfill or connected state.
7. Opportunities presents decision-ready opportunities, not raw signal tables.
8. User accepts at least one opportunity and dismisses or snoozes at least one other opportunity.
9. Accepted material-content work appears in Content Plan with reason, evidence summary, target, and measurement window; pure link-only work does not.
10. User generates or prepares the work, reviews it, and approves or requests changes.
11. If a CMS connector is in scope for the release, user connects WordPress or the relevant publisher, creates/stages the draft or update, approves the gated publish/update, and verifies the live URL.
12. Results receives the published action and shows the correct waiting, inconclusive, positive, or negative measurement state.

Required recovery and edge paths:

1. Domain-only project without GSC still has a usable public-only Opportunities path and clear Search Console setup CTA.
2. Missing GSC property opens the setup assistant instead of a dead end.
3. Non-admin user gets an admin handoff path for GSC and CMS connections.
4. OAuth denied, revoked, stale, or mismatched property states are visible and recoverable.
5. Backfill in progress does not show fake precision.
6. Publisher disconnected or publish failed states show one recovery action by default.
7. Completed, dismissed, diagnostic, raw evidence, and provider-detail content remains hidden unless the user explicitly opens it.

End-to-end acceptance must also include a UX pass. In every first viewport, the verifier should be able to answer:

```text
What do I need to do now?
```

If a real user would need to inspect raw tables, understand provider terminology, hunt through completed automation output, or guess the next action, the release candidate fails the E2E UX check even if the underlying data and APIs work.

## 16. Verification Protocol

Acceptance requires product verification, not only code review. Every implemented phase must have:

- automated checks for the changed backend/frontend contract
- Chrome verification against the deployed web app
- evidence recorded with URL, project, user role, timestamp, and pass/fail result
- screenshots or short notes for key UI states
- explicit blocked status if an external provider, OAuth verification, or permission issue prevents completion

For PRD-only changes, local document validation is enough. For product implementation, the default expectation is real browser verification in Chrome.

### 16.1 Chrome verification rules

Chrome verification should use the real deployed app, not mocked local screenshots, for release acceptance.

The verifier should:

1. Open the deployed CiteLoop URL in Chrome.
2. Sign in with the intended test/customer account.
3. Select or create the test project.
4. Exercise the feature through the user-facing UI.
5. Confirm that default pages are action-first and do not expose raw provider details.
6. Confirm downstream state changes in the UI.
7. Inspect backend/API state only as supporting evidence, not as a substitute for the user flow.
8. Confirm the first viewport answers what the user needs to do now.

If a feature is behind a preview deployment, the same Chrome flow should run on preview before merge and on production after release.

### 16.2 OAuth-assisted verification

When verification requires Google Search Console, Google Analytics, WordPress, or another external account:

- CiteLoop should navigate to the connect flow.
- The assistant can drive Chrome up to the external OAuth or login screen.
- The user completes provider login, consent, MFA, or account selection.
- The assistant resumes after the OAuth callback returns to CiteLoop.
- The user should never paste tokens, secrets, cookies, or credentials into chat.
- The verification record should mention that the user completed OAuth manually and that CiteLoop received the expected connected state.

For GSC, the expected test path is:

```text
Create/select project -> input domain -> Connect Search Console -> user completes Google OAuth -> select matching property -> first backfill starts -> Opportunities shows connected/backfilling state
```

### 16.3 Phase verification matrix

| Phase | Feature area | Chrome verification | Supporting checks |
|---|---|---|---|
| Phase 1 | IA / sidebar | Verify sidebar has Home, Context, Doctor, Opportunities, Content Plan, Review, Publish, Results; Settings is bottom utility; no primary SYSTEM group; each primary page's first viewport exposes the page's next action. | Canonical `/opportunities` and optional `/analysis` compatibility redirect; navigation tests. |
| Phase 2 | Opportunities analysis surface | Verify Opportunities shows compact top-right GSC status, search performance snapshot, growth findings, decision queue, measurement snapshot, action-specific CTAs, and collapsed evidence. | API returns connection state, opportunities, evidence summaries; raw tables and large recommendation-card feeds are hidden by default. |
| Phase 3 | Results surface | Verify Results shows published action outcomes and measurement states, not raw opportunity acceptance. | Measurement API returns waiting/inconclusive/positive/negative states. |
| Phase 4 | GSC OAuth onboarding | Verify Connect Search Console from Home/Opportunities/Settings, user OAuth, property list, property selection, denied/revoked/missing-property states. | OAuth state/CSRF tests, encrypted token storage, integration records, no token leakage in logs. |
| Phase 5 | GSC-backed Opportunities analysis | Verify connected project backfills data, shows real query/page opportunity cards, carries evidence downstream after acceptance. | GSC sync tests, opportunity generator tests, `seo_opportunities` / `content_actions` records. |
| Phase 6 | WordPress gated publish/update | Verify WordPress connect, capability readiness, create draft, stage update, approval gate, publish/update, URL verification, Results measurement. | Publisher contract tests, credential health tests, WordPress API client tests, publish failure recovery tests. |
| Phase 7 | GA4 prioritization | Verify GA4-connected project shows engagement/conversion influence; non-GA4 project shows missing business-value signal without failing. | GA4 ingestion/storage tests using `page_performance_daily` fields. |
| Phase 8 | Additional CMS connectors | Verify unimplemented connectors are not selectable as available destinations; implemented connectors expose capability readiness. | Connector capability tests and provider-specific integration tests. |

### 16.4 Feature-level verification requirements

Search data connection:

- Connect CTA appears in onboarding, Home, Opportunities, and Settings when missing.
- Non-admin users see an admin handoff instead of a dead-end Settings page.
- OAuth success, cancel, denied consent, revoked token, stale sync, and property mismatch states are visible and recoverable.

Opportunity analysis recommendations:

- Default view shows a compact findings and decision queue, not a recommendation-card feed.
- Each decision has reason, expected impact, confidence/risk, and an action-specific create/watch/dismiss path.
- `View evidence` reveals raw supporting details only after user action.

Page UX:

- Home answers the project-wide next-step question in the first viewport.
- Context asks for confirmation or correction, not passive reading.
- Opportunities asks for accept/dismiss/snooze/investigate; analysis is the capability behind those decisions, not a separate work source.
- Content Plan shows accepted material-content work that is ready, scheduled, blocked, or needs approval.
- Review shows approval decisions only.
- Publish shows publish/update/retry/verify work only.
- Results shows outcome summaries and exceptions only.
- Settings groups connection and project configuration tasks.

Content Plan:

- Only accepted work appears by default.
- Raw unaccepted opportunities, generated intermediates, and completed history remain hidden unless explicitly opened.
- Accepted work retains source opportunity, evidence summary, target URL/asset, risk, and measurement window.

Publish / CMS:

- Publisher readiness is capability-based.
- WordPress creates drafts before live publishing.
- Live publish/update requires explicit user approval in MVP.
- Provider errors show one recovery action by default.
- Raw provider responses are hidden behind publish details.

Results:

- Published or updated URLs move into measurement.
- Waiting, inconclusive, positive, and negative states are visible.
- Long per-query/per-URL detail is collapsed by default.

### 16.5 Release acceptance rule

A phase is not done until the responsible implementer can provide:

- passing automated checks
- Chrome verification notes for the relevant user flow
- provider/OAuth verification result when the feature depends on GSC, GA4, or CMS
- known gaps or blocked external-provider issues
- screenshots or URLs for the key accepted states

The complete multi-phase release is not done until the cross-phase end-to-end workflow in Section 15 also passes.

### 16.6 End-to-end verification and self-repair loop

End-to-end verification is the final release gate for this PRD. It should be run after all implemented phases in the release candidate have passed their phase-level checks.

The verifier should start from a clean customer perspective:

1. Use a fresh or reset test account where possible.
2. Create a fresh project or reset the prior test project state.
3. Enter the product domain through the normal user flow.
4. Use real deployed Chrome verification, not mocked screenshots.
5. Complete external OAuth, CMS login, consent, or MFA manually with the user when required.
6. Record evidence with URLs, project/domain, account role, connected property or publisher label, screenshots or notes, and pass/fail status.

The E2E script must cover the happy path and the recovery paths listed in Section 15. It should verify the full chain:

```text
Signup / project -> domain -> public discovery -> GSC gate or public-only mode -> GSC OAuth/property selection/backfill -> Opportunities decision -> Content Plan -> Review -> Publish/CMS -> URL verification -> Results measurement
```

Stop-and-fix rule:

- If any step cannot be completed, the verifier stops and records the blocker.
- The team fixes product behavior, copy, data state, permissions, routing, or implementation before claiming release readiness.
- After a fix, the verifier reruns the failed step and every downstream step.
- If the failed step may have contaminated project state, the verifier reruns the journey from a clean account or project.
- A release cannot pass with "known issue" on the critical path unless the PRD explicitly defers that path from the release scope.

UX self-repair rule:

- After the functional E2E pass, the verifier reviews the experience as a customer, not as an implementer.
- Every page must lead with the current status and next action.
- Raw data, diagnostics, generated intermediates, old completed work, and provider details must stay hidden by default.
- Confusing copy, missing CTAs, duplicated decisions, or overloaded default pages should be fixed and reverified before release acceptance.

The final verification packet should include:

- release candidate URL
- account role used for the test
- project/domain used for the test
- whether GSC was connected, public-only, or missing-property
- whether CMS publishing was connected, draft-only, or out of scope
- evidence for Opportunities, Content Plan, Review, Publish, and Results handoffs
- recovery states tested
- final pass/fail status and any explicitly deferred non-critical gaps

## 17. Risks

1. **Opportunities analysis becomes another overloaded dashboard.**
   Mitigation: keep default view to compact GSC status, search performance snapshot, growth findings, decision queue, and measurement snapshot; keep brief, raw evidence, completed work, and diagnostics collapsed.

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

12. **WordPress connector expands Content scope too quickly.**
   Mitigation: limit MVP to draft creation, gated updates, metadata updates when supported, URL verification, and measurement. Keep theme editing, plugin installs, bulk publishing, destructive delete, and rollback out of MVP.

13. **CMS provider details leak into the user workflow.**
   Mitigation: expose capability readiness and simple recovery actions by default. Hide post IDs, API responses, credential refs, and sync logs behind publish details or admin diagnostics.

14. **Phase-level acceptance hides broken cross-phase handoffs.**
   Mitigation: make the end-to-end user journey the final release gate, and require stop-and-fix behavior when any handoff or recovery path fails.

## 18. Product Success Metrics

These measure whether the IA change works without promising SEO rankings:

1. Opportunity acceptance rate: percentage of open Opportunities records accepted or dismissed within 7 days.
2. Time from signal to action: median time from opportunity creation to accepted `content_actions` record.
3. Self-serve connection completion: percentage of eligible projects that complete OAuth, select a property, and start first backfill.
4. Search data connection readiness: percentage of active projects in `gsc_connected`, `gsc_backfilling`, or an explicitly understood public-only state.
5. Publisher readiness: percentage of active projects with a connected CMS or an explicitly understood draft-only/manual publishing state.
6. Time from accepted action to staged CMS draft/update.
7. Content Plan source integrity: percentage of non-legacy Content Plan items with an accepted AI-generated source Opportunity; target is 100%, and source-less legacy Topics are excluded from the denominator.
8. Results coverage: percentage of published actions with a measurement window and at least one recorded outcome state.
9. End-to-end workflow completion: percentage of release-candidate E2E runs that complete from domain setup to Results measurement without verifier intervention beyond expected OAuth/CMS consent.
10. Time to first measured action: median time from project creation to the first action entering Results measurement.

## 19. Product Decisions

1. The user-visible growth-work source and sidebar label are `Opportunities`.
   Search Intelligence or analysis may describe capabilities inside that page, but neither creates a third work source.

2. `Opportunities` remains in the primary sidebar beside Doctor.
   Growth findings and the compact decision queue are the primary working area inside Opportunities.

3. `Visibility` becomes `Results` in the visible product IA.
   The implementation may keep the old `/visibility` route as a compatibility alias during rollout.

4. Opportunities owns the connect/reconnect CTA for Search Console.
   Settings owns detailed connection management, revocation, diagnostics, and advanced configuration.

5. Immediately verifiable technical SEO work belongs to Doctor as Site Fixes, not Content Plan.
   Measured pure link strategy becomes an Opportunities Growth Action, while Content Plan receives only accepted AI-generated Opportunity work that materially creates or refreshes content. A separate Tasks page remains out of scope.

6. Default Search data connection model is self-serve end-user Google OAuth.
   Service account remains supported only as internal/admin fallback or migration bridge.

7. The first customer path is OAuth-first.
   A user should be able to create a project with a domain, connect their own GSC property, and reach Opportunities without operator setup.

8. Default surfaces are collapsed by design.
   User-facing pages should show what needs attention now. Evidence, diagnostics, completed work, raw records, and generated intermediate content should be available but hidden by default.

9. WordPress is the first self-serve CMS integration.
   Webflow, Shopify, and Wix are connector roadmap items and should not block WordPress MVP.

10. CMS publishing is gated by default.
   CiteLoop may create drafts and stage updates, but live publish/update requires explicit approval until auto-publish policy, rollback, and recovery paths are proven.

11. Publisher capability readiness is user-facing.
   The product should show what a connected CMS can safely do instead of treating all publisher connections as equivalent.

12. Content Plan is not a new-work source.
   Doctor and Opportunities are the only user-visible new-work sources. Content Plan has no manual Opportunity/Topic/Brief intake; historical source-less Topics retain only the existing-record operations in the Content Plan Section 15.4 matrix and cannot create, clone, backfill, migrate, or seed work.
