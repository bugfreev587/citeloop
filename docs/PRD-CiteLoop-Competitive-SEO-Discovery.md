# PRD：CiteLoop Competitive SEO Discovery

> 日期：2026-07-14
> 阶段：Growth Radar / AI Discovery 升级
> 触发案例：UniPost 多次运行 AI discovery 后，仍未发现 PostSyncer 的 `https://postsyncer.com/tools` SEO hub，也没有生成对应 opportunity。
> 相关文档：`docs/PRD-CiteLoop-GEO-Visibility-Layer.md`、`docs/PRD-CiteLoop-Multi-Surface-SEO-Growth-Layer.md`、`docs/PRD-CiteLoop-SEO-GEO-Automation-Upgrade.md`、`docs/PRD-CiteLoop-SEO-Doctor.md`

## 1. 背景

CiteLoop 当前的 Opportunity Finding / Growth Radar 已经能做几件事：基于项目 profile 生成 stage-aware prompts，刷新 answer-provider observations，收集少量 search evidence，并把部分 SEO/GEO gap materialize 成 opportunity。

这套链路适合发现“已经靠近项目已知定位和已知竞品”的机会。但 PostSyncer 案例暴露了另一类机会：

- PostSyncer 的 `https://postsyncer.com/tools` 是可索引、可抓取的 tools hub。
- `robots.txt` 放行，并指向 `https://postsyncer.com/sitemap.xml`。
- sitemap 包含 `/tools`、大量 `/tools/...` leaf pages、comparison pages、scheduler pages 和 social media 相关页面。
- `/tools` 页面本身是典型 programmatic SEO hub，页面内有 100+ free social media tools 内链。
- 对 UniPost 来说，正确机会不是复制所有 downloader 工具，而是识别这个 SEO pattern，然后提出更适合 UniPost 的 free social content tools hub，例如 post formatter、caption generator、UTM builder、character counter、blog-to-social generator、social image helper。

2026-07-14 的生产诊断发现：

- UniPost active profile 里的已知竞品只有 Ayrshare、Zernio、PostForMe，没有 PostSyncer。
- `geo_competitors` 有竞品名称，但 active competitor 记录没有 domains。
- UniPost 的 `growth_search_evidence` 当前没有包含 PostSyncer 的结果。
- UniPost 的 `geo_observations` 没有非空 `cited_urls`，也没有非空 `competitor_citations`。
- 最近 AI discovery 生成的 prompts 都围绕已知 UniPost vocabulary：unified publishing API、hosted OAuth、MCP server、webhooks、SDKs、Zernio 等。

结论：这不是“PostSyncer 被扫到了但 ranking 没过”，而是 recall 阶段没有把这个页面带入系统。

## 2. 问题定义

CiteLoop 当前更像“profile-grounded prompt refresher”，还不是“competitive SEO discovery system”。

当前链路从已知 product profile、已知 competitors、已有 prompts、已有 topics/opportunities 出发，让 LLM 提出少量新 prompts，再对少数 selected prompts 收集 search evidence。search evidence 发生在 prompt planning 之后，主要作为 evidence/scoring 支撑，而不是反向驱动 competitor domain expansion、sitemap crawl、page cluster mining 或 SEO archetype opportunity。

因此，当机会具备以下特征时，系统容易漏：

- 竞品不在已知 profile competitor 列表中。
- 已知 competitor 没有绑定 domain。
- 机会来自竞品站点结构，而不是单条 answer citation。
- answer provider 没有返回 citation URL。
- search results 没有被继续 crawl、归类和扩展。
- 系统没有 tools hub、comparison cluster、alternatives cluster、scheduler cluster、integration cluster、glossary cluster 等 archetype miner。

## 3. 产品目标

1. 发现 manually confirmed competitors 之外的相关竞品、邻近工具和 SERP neighbor domains。
2. 安全抓取和 enrich 竞品公开页面，包括 robots、sitemap、canonical、metadata、structured data、internal links、可选 `llms.txt`。
3. 识别 programmatic SEO archetypes：tools hub、comparison cluster、alternatives page、integration page、scheduler page、glossary、template/checklist、benchmark/source page。
4. 把竞品 SEO pattern 转成 project-fit opportunity，附带 evidence、scoring、risk 和 recommended action。
5. 修复 citation/entity attribution，让 `cited_urls` 能被分类为 project-owned、competitor、third-party、irrelevant 等。
6. 建立可观察的 discovery funnel，解释发现了什么、抓了什么、过滤了什么、最终生成了什么。
7. 把 PostSyncer `/tools` 做成 regression fixture，防止这类漏召回再次发生。

## 4. 非目标

- 不复制竞品内容，不克隆竞品工具。
- 不抓取登录后内容，不绕过 WAF，不忽略 robots，不违反站点 ToS。
- 不在没有数据时推断竞品流量、收入、排名或 conversion。
- 不做完整 Semrush/Ahrefs 级关键词库。
- 不自动发布 competitive discovery 产生的新资产。
- 不默认生成高法律风险或弱相关机会，例如 video downloaders、商标堆砌页、spammy alternatives pages。
- 不把传统 SERP results 当作 answer-engine citation。

## 5. 产品原则

1. **Recall before ranking**：再好的 scoring 也无法评分从未进入系统的页面。
2. **Pattern over page**：最高价值 insight 往往是竞品的可复制页面系统，而不是单个 URL。
3. **Fit before imitation**：机会必须映射到客户的 positioning、capabilities、audience 和 risk tolerance。
4. **Evidence first**：每个 opportunity 必须展示 source URLs、extracted facts 和 filter decisions。
5. **Transparent misses**：如果竞品页面被过滤，系统要能解释原因。
6. **Safe competitive intelligence**：只抓公开页面，尊重 crawl policy，优先存 derived facts。

## 6. 用户故事

### 6.1 Founder / Growth Operator

作为 UniPost operator，我希望 CiteLoop 能发现 PostSyncer tools hub 这类竞品 SEO play，解释这个 pattern 为什么重要，并推荐一个适合 UniPost 的版本，而不是让我手工检查竞品站点。

### 6.2 SEO Operator

作为 SEO operator，我希望 run detail 能展示系统考虑了哪些 domains/pages、检测到哪些 archetypes、为什么最终生成或没有生成 opportunity。

### 6.3 Product Marketer

作为 product marketer，我希望 competitive opportunities 尊重产品定位。如果竞品做了 video downloaders，CiteLoop 不应该盲目建议 UniPost 也做 downloaders，而应该抽象出用户 intent，再推荐更安全、更相关的工具。

### 6.4 Internal Dogfood

作为 CiteLoop 团队，我希望 PostSyncer 成为固定 regression fixture，证明系统能发现公开竞品 tools hub、识别 page cluster，并生成或解释 UniPost-fit opportunity。

## 7. 当前基线

### 7.1 Manual AI Discovery Planner

当前 manual planner：

- 读取 active product profile。
- 读取 existing topics、SEO opportunities、GEO prompts、confirmed competitors。
- 调用 `growthradar.ClassifyContext(profile.Profile, growthradar.EvidenceIndex{})` 对 profile terms 做 classification。
- 从 confirmed capabilities/value props 生成 public vocabulary。
- 让 LLM 提出 4-6 个 prompts。
- 校验 `target_topic` 必须 map 到 public vocabulary。
- 把 accepted candidates 创建为 GEO prompts。

这个设计能减少乱跑和 unsupported claims，但也形成了 closed vocabulary。更具体地说，`ClassifyContext` 已经支持 `EvidenceIndex.PublicTerms` 扩充词表，但生产调用点仍传空 evidence index。新增 planner 因为校验更严格，让这个闭环更明显：即使 search evidence 或 competitor recall 发现了新公开词，也无法回流到 planning vocabulary。

Phase 0 必须先补通 `evidence -> vocabulary` 回路，再投入更重的 domain graph 工作。实现上应给 `AIManualDiscoveryPlanner` 增加 evidence 入口，并修复三个调用点：

- `internal/opportunityfinding/ai_planner.go`
- `internal/geo/pr2.go`
- `internal/geo/pr3.go`

### 7.1.1 Planner 并存与退役计划

当前存在两套 prompt/planning 路径：

- 新 manual planner 使用 `mapsToPublicVocabulary` 校验 `target_topic`，但该函数是双向子串匹配，不是严格集合成员判断。
- 老 `internal/geo/pr2.go` 路径仍会从 `Topic.TargetKeyword`、`Topic.TargetPrompt`、`Topic.Title` 追加 target topics，只过滤 sensitive term，不经过同样的 public vocabulary 校验。

本 PRD 不应假设所有 planning 都已经走新 planner。Phase 0 需要明确两套路径的关系：要么把老路径的 topic source 也接入同一 evidence/vocabulary gate，要么标记退役计划，避免 competitive recall 只修新路径而老路径继续产生不一致 prompt。

### 7.2 Evidence Refresh

当前 evidence refresh：

- 选择 active prompts。
- 观察 answer provider 输出。
- 跑 crawler audit。
- 监控已知 external surfaces。
- 对少量 selected prompts 收集 search evidence。

search collection 发生在 prompt planning 下游，不会在同一 run 内反过来驱动 domain expansion。

### 7.3 Citation Attribution

answer provider 可能返回 `cited_urls`，但 high-value competitor gap rule 依赖 `competitor_citations`。如果 URL 没有被归类成 competitor citation，`geo_competitor_cited_project_absent` 就不会触发。

### 7.4 Search Evidence

search evidence 已经能记录 query results，但当前链路不会：

- crawl result URLs；
- 抽取 domain-level competitors；
- 读取 sitemap；
- 检测 URL/page clusters；
- 推断 competitor archetypes；
- 从竞品页面系统创建 competitive SEO opportunities。

## 8. 方案概览

新增 Competitive SEO Discovery layer，接入 Opportunity Finding。

```text
project profile
  + known competitors
  + existing prompts/topics/opportunities
  + seed category queries
  + prior search evidence
        |
        v
competitive recall
        |
        v
domain graph + page crawl + sitemap enrichment
        |
        v
archetype miner + citation/entity classifier
        |
        v
project-fit opportunity materializer
        |
        v
Growth Radar queue + run funnel diagnostics
```

## 9. 功能需求

### 9.1 Competitive Recall

Competitive recall 在 prompt planning 完成前发现 candidate domains 和 candidate pages。

Seed sources：

- active product profile terms；
- confirmed competitors and aliases；
- existing topics and opportunities；
- active GEO prompts；
- GSC query clusters，如果可用；
- 用户提供的 competitor URLs；
- manual run seed URL；
- 根据 project positioning 生成的 SERP category queries。

初始 query families：

- `best <category> tools`
- `<category> alternatives`
- `<competitor> alternatives`
- `<competitor> vs <project>`
- `free <job-to-be-done> tool`
- `<platform> <job-to-be-done> tool`
- `<category> API`
- `<category> scheduler`
- `<category> automation tools`
- `<persona> <workflow> tool`

UniPost 示例 seed queries：

- `free social media tools`
- `social media post formatter`
- `social media caption generator`
- `social media scheduling API`
- `social media publishing API alternatives`
- `Buffer alternatives API`
- `best social media automation tools`
- `tools for scheduling social posts`

需求：

1. 每个 project/run 必须有 configurable query budget，并明确它如何消耗现有 `growth_search_evidence` SearchBudget。当前 search budget 是全表统计的硬上限，competitive recall 不能绕过或无感挤占常规 evidence refresh。
2. 按 normalized host 和 canonical URL 去重。
3. 把每个 result 分类为 likely competitor、adjacent tool、directory、media/article、docs、irrelevant 或 unknown。
4. 持久化每个 candidate 及 reason code。
5. manual run 支持 seed URL 入参，例如 `https://postsyncer.com/tools`。missed-reason report 和直接 enrichment 逻辑统一收敛到 §13.3 Repair Run。

### 9.2 Domain and Page Enrichment

Domain and Page Enrichment 不应新建一套 crawler。V1 应扩展现有 `internal/crawl` 能力，使其支持 competitive candidate domains 和跨域场景：

- `internal/crawl/robots.go` 已能读取 robots 和 sitemap 声明。
- `internal/crawl/fetch.go` 已有 sitemap 抓取和 cap。
- `internal/crawl/normalize.go` 已有 URL normalization 和 same-origin 判断。
- `internal/crawl/crawler.go` 已有 worker pool 与 rate limiting。
- `internal/config.CrawlConfig` 已有 `SameOriginOnly`、`MaxPages`、`MaxDepth`、`RequestTimeoutMs`、`RateLimitRPS`、`RespectRobots`、`SitemapURLCap`。
- `internal/api/onboarding.go` 已有按场景覆盖 crawl budget 的范式。

Phase 2 前置技术债：仓库目前有 `internal/crawl/robots.go` 和 `internal/geo/robots.go` 两套 robots 解析逻辑。Competitive discovery 必须指定复用/合并策略，不能引入第三套解析器。

对 high-confidence domains 收集公开 metadata：

- redirects 后的 final URL；
- canonical URL；
- title / meta description；
- robots access state；
- robots 中声明的 sitemap URLs；
- common sitemap locations；
- advertised `llms.txt`；
- structured data presence；
- page type hints；
- internal links；
- sitemap URL samples；
- sitemap lastmod；
- same-domain path clusters。

需求：

1. 尊重 robots 和 global crawl budgets。
2. 默认存 derived facts，不长期保存完整竞品页面快照。
3. V1 对每个 domain 设置 page cap，并通过 `CrawlConfig` 或同等场景配置表达。
4. 记录 fetch errors 和 filter reasons。
5. 除非 provider 明确支持 rank semantics，否则 search provider order 标记为 not rank。

### 9.3 Archetype Miner

Archetype miner 把 pages 和 URL clusters 转成 SEO patterns。

初始 archetypes：

| Archetype | Signals | Example opportunity |
|---|---|---|
| `tools_hub` | `/tools`、大量 tool leaf links、free tools 文案、Tool/FAQ schema | 创建 project-fit free tools hub |
| `comparison_cluster` | `/comparison/*`、`vs`、competitor names | 创建 comparison page set |
| `alternatives_cluster` | `alternatives`、`best X alternatives` | 创建 alternatives page |
| `scheduler_cluster` | platform scheduler pages | 创建 platform-specific workflow pages |
| `integration_cluster` | integrations/docs pages by platform | 创建 integration/docs assets |
| `glossary_cluster` | definition pages、alphabetical/term structure | 创建 glossary/definition pages |
| `template_cluster` | templates/checklists/calculators | 创建 reusable templates/tools |
| `benchmark_source` | stats、reports、source bundles | 创建 citation-ready source asset |

PostSyncer `/tools` 的期望分类：

- `tools_hub`：high confidence。
- supporting signals：indexable page、sitemap inclusion、100+ internal `/tools...` links、free social media tools language、fresh sitemap lastmod、related comparison/scheduler pages。
- project fit：排除 downloader tools 后，对 UniPost 为 medium/high。
- legal/risk：downloader 类工具 medium；formatter、caption、UTM、character counter、image sizing、blog-to-social、hook generator low/medium。

### 9.4 Project-Fit Opportunity Materializer

Materializer 只在 discovered archetype 能映射到 project positioning 和 supported assets 时创建 opportunity。

Opportunity fields：

- `type`：例如 `competitive_tools_hub_gap`、`competitive_comparison_cluster_gap`、`competitive_alternative_gap`；
- `query` 或 `target_topic`；
- `page_url`，如果 opportunity 针对具体页面；
- `evidence`，包含 source URLs、archetype、extracted signals、filter reasons、project-fit reasoning；
- `recommended_action`；
- `expected_impact`，只做定性假设；
- `effort`；
- `risk_level`；
- `priority_score`；
- `confidence`；
- `opportunity_identity_key`；
- `growth_spec`。

UniPost opportunity 示例：

```text
Type: competitive_tools_hub_gap
Target topic: Free social content tools for developers and operators
Recommended action: Create a UniPost Free Social Content Tools hub with 5-8 MVP tools:
  - social post formatter / line breaker
  - platform caption generator
  - social character counter
  - UTM builder for social campaigns
  - blog-to-social generator
  - social image size helper
CTA: publish or schedule through UniPost API/dashboard.
Do not include downloader tools in V1.
Evidence: PostSyncer /tools page, sitemap inclusion, internal /tools leaf count, related comparison/scheduler cluster.
```

需求：

1. 如果 archetype 无法映射到 confirmed project capabilities 或 adjacent user jobs，不创建机会。
2. 明确标记被排除的 competitor subpatterns 及原因。
3. 按 project、archetype、target topic、competitor domain、evidence window 去重。
4. rerun 时更新 existing opportunity evidence，不制造重复 open items。
5. 支持 `observe_only` mode，再逐步开启 automatic opportunity creation。

### 9.5 Citation and Entity Attribution

新增 URL-to-entity classification，用于 answer observations 和 search evidence。

Classifier outputs：

- `project_owned`
- `known_competitor`
- `new_competitor_candidate`
- `adjacent_tool`
- `directory_or_marketplace`
- `publication`
- `social_or_forum`
- `irrelevant`
- `unknown`

Inputs：

- project canonical domains；
- publisher connections 和 managed external surfaces；
- `geo_competitors.domains`；
- aliases and competitor names；
- normalized URL host/path；
- eTLD+1 / public suffix 解析，用于避免把 `*.github.io`、`*.vercel.app`、`*.notion.site` 等共享托管域误判成同一实体；
- page metadata；
- search snippets；
- deterministic checks 后的可选 LLM classification。

需求：

1. 如果 `cited_urls` 包含 known competitor domain，派生 `competitor_citations` 供 analyzer 使用。
2. 如果 `cited_urls` 包含新的 likely competitor，创建或更新 competitor candidate，但不要无确认地自动设为 active competitor。
3. classifier 逻辑变更时支持 backfill recent observations。
4. raw URL evidence 继续留在 observation；derived entity classification 单独存储，方便审计。

### 9.6 Run Funnel and Debuggability

每次 discovery run 必须暴露 funnel：

- seed queries generated；
- search results collected；
- candidate domains discovered；
- pages fetched；
- pages blocked/skipped；
- sitemaps fetched；
- sitemap URLs sampled；
- archetypes detected；
- candidates filtered by relevance；
- candidates filtered by risk；
- duplicate opportunities updated；
- new opportunities created；
- watchlist items created；
- errors and provider degradations。

用户侧 run summary 必须能回答：

1. CiteLoop 查找了什么？
2. 发现了什么？
3. 忽略了什么？
4. 生成了什么 opportunities？
5. 哪些需要人工确认？

### 9.7 Identity and Fingerprint Strategy

Competitive opportunity materialization 必须先确定 identity 和 fingerprint 策略，否则用户 dismiss 的机会会在 scheduled run 中反复复活。

当前基线：

- `seo_opportunities.opportunity_identity_key` 由 `project_id | type | normalized_page_url | query | evidence.intent_type | evidence.engine` 计算。
- `seo_opportunity_review_states` 以 `project_id + opportunity_identity_key` 记录 review 状态。
- `seo_opportunities` 上有基于 `project_id + opportunity_identity_key` 的 partial unique index。
- `evidence_fingerprint` 当前包含 `priority_score`、`confidence`、`risk_level` 等字段。
- on-conflict 逻辑只在 dismissed/snoozed/watching 且 fingerprint 不变时保持 review 状态；fingerprint 变化会 reopen。
- `growth-opportunity-v2` 另有 `growth_spec->>'dedupe_identity'` 要求，这是独立于 `opportunity_identity_key` 的概念。

Competitive discovery 的风险是：`freshness`、`demand_proxy`、SERP sample、sitemap lastmod 等会随 run 漂移，进而改变 `priority_score`，导致 fingerprint 改变，用户已 dismiss 的 opportunity 被重新打开。

要求：

1. Phase 3 前必须明确 competitive opportunities 走哪套去重主键：复用 `opportunity_identity_key`，还是以 `growth_spec.dedupe_identity` 为主并迁移 review state。
2. 如果改 `opportunity_identity_key` 公式，必须包含 review state backfill 和 partial unique index 迁移方案。
3. `competitive_*` opportunity 的 fingerprint 不得直接包含易漂移分数。可选方案是把 freshness/demand_proxy 分桶后进入 score，或为 competitive 类型定义排除易变项的 fingerprint 构成。
4. Launch criteria 中的“duplicate reruns 更新 evidence，不制造重复 open opportunities”必须同时验证 dismissed/snoozed/watching 状态不会因分数微漂移而复活。

## 10. 数据模型

本 PRD 命名的是概念表。实现时可以复用或扩展现有 `growth_search_evidence`、`geo_competitors`、`seo_opportunities`、`growth_radar_items` 和 surface tables。

### 10.1 `competitive_discovery_runs`

用途：每次 competitive recall/enrichment run 一行。

字段：

- `id`
- `project_id`
- `workflow_event_id`
- `trigger_kind`：manual、scheduled、seed_url、repair、backfill
- `mode`：observe、create
- `seed_inputs`
- `query_budget`
- `crawl_budget`
- `status`
- `funnel`
- `cost_usd`
- `started_at`
- `finished_at`
- `error`

### 10.2 `competitive_domain_candidates`

用途：SERP、citation、profile 或 manual input 发现的 candidate domains。

注意：该概念与现有 `geo_competitors` 有职责重叠。`geo_competitors` 已有 `status`、`source`、`aliases`、`domains`、`name_key` 等字段，且 `source` 已包含 search-result 类场景的预留语义。实现前必须决定是扩展 `geo_competitors` 作为 candidate/active 共用表，还是新增 `competitive_domain_candidates`。如果新增表，§12.3 review UI 必须避免出现两个互相竞争的数据源。

字段：

- `id`
- `project_id`
- `run_id`
- `host`
- `canonical_host`
- `classification`
- `confidence`
- `source`
- `source_query`
- `source_url`
- `evidence`
- `status`：candidate、active_competitor、adjacent、ignored、blocked
- `reason_codes`
- `first_seen_at`
- `last_seen_at`

### 10.3 `competitive_page_candidates`

用途：candidate domains 上被 enrich 的公开页面。

字段：

- `id`
- `project_id`
- `domain_candidate_id`
- `url`
- `canonical_url`
- `http_status`
- `indexability`
- `title`
- `meta_description`
- `schema_types`
- `internal_link_count`
- `same_archetype_link_count`
- `sitemap_included`
- `sitemap_lastmod`
- `robots_allowed`
- `llms_advertised`
- `page_type_hint`
- `fetch_state`
- `evidence`
- `created_at`
- `updated_at`

### 10.4 `seo_archetype_clusters`

用途：派生出的 page-system patterns。

字段：

- `id`
- `project_id`
- `run_id`
- `domain_candidate_id`
- `archetype`
- `confidence`
- `root_url`
- `sample_urls`
- `leaf_count_estimate`
- `signals`
- `project_fit_score`
- `risk_level`
- `excluded_subpatterns`
- `recommended_asset_type`
- `status`：detected、filtered、materialized、watchlist

### 10.5 Existing Table and Write-Path Requirements

这里不是单纯 schema 扩展。部分关键字段已经存在，但写入路径或语义尚未打通。

`geo_competitors.domains` 已存在，类型为 jsonb，默认 `[]`。当前问题是 profile 路径创建竞品时硬编码写空数组，所以 URL classifier 即使上线，也无法用这张表 join 出 known competitor citation。

顺序必须是：

1. **补 domain 写入路径**：从 profile competitor、manual competitor edit、SERP/domain recall、seed URL enrichment 中提取和写入 competitor domains。已有 `source='search_result'` 或同等来源语义时应优先复用。
2. **backfill 存量记录**：对已存在的 active competitors 补 domains，至少支持手动补录、从历史 evidence 推断 candidate、从 known official URL backfill。
3. **再上 URL classifier**：`cited_urls -> geo_competitors.domains -> competitor_citations` 的派生逻辑必须在右表有数据后启用，否则会持续空转。

其他推荐扩展：

- `geo_observations`：增加 derived URL classification output，或关联 classification records。
- `seo_opportunities.evidence`：增加 `competitive_discovery` block。
- `growth_radar_items`：记录 competitive discovery candidate ID 和 archetype ID。

## 11. Scoring

### 11.1 Domain Candidate Score

Signals：

- 出现在多个 seed queries；
- 出现在 buyer/category queries；
- 与 project category 高重叠；
- 有 comparison/scheduler/tools/integration paths；
- 命中 known competitor name/domain；
- 被 answer provider citation 引用；
- 不是 directory/publication，除非 opportunity 是 source/citation oriented。

### 11.2 Archetype Score

Signals：

- archetype confidence；
- leaf count；
- sitemap inclusion；
- freshness；
- indexability；
- internal linking depth；
- project capability fit；
- audience fit；
- existing owned coverage gap；
- risk and legal constraints；
- effort estimate。

### 11.3 Opportunity Score

默认公式必须 deterministic and explainable：

```text
priority = project_fit
  + archetype_strength
  + demand_proxy
  + owned_gap
  + freshness
  - risk_penalty
  - effort_penalty
  - duplicate_penalty
```

LLM 可以提供 qualitative reasoning，但不能作为 `priority_score` 的唯一来源。

## 12. UX 需求

### 12.1 Opportunity Card

Competitive opportunity card 必须展示：

- competitor/source domain；
- source page URL；
- detected archetype；
- 为什么重要；
- 为什么适合当前 project；
- 排除了什么；
- recommended action；
- risk and effort；
- evidence count；
- 来自哪个 discovery run。

### 12.2 Run Detail

新增 Discovery Run detail view 或 panel：

- seed queries；
- top discovered domains；
- fetched pages；
- detected archetypes；
- filter reasons；
- created/updated opportunities；
- provider/crawl errors。

### 12.3 Competitor Candidates

新增 candidate competitor review：

- accept as competitor；
- mark as adjacent；
- ignore；
- add aliases/domains；
- view source evidence。

对 UniPost，PostSyncer 可以被分类为 `adjacent_tool` 或 `new_competitor_candidate`，但 `/tools` archetype 仍然可以作为 opportunity evidence。

## 13. API 与 Job 需求

### 13.1 Manual Run

Manual Opportunity Finding 支持 optional competitive discovery inputs：

- `seed_urls`
- `seed_domains`
- `seed_queries`
- `mode`：observe 或 create
- `max_queries`
- `max_domains`
- `max_pages_per_domain`

### 13.2 Scheduled Run

Scheduled runs：

- 小批量 refresh competitive domains；
- 对 priority categories 抽样新的 SERP neighbors；
- 避免频繁重抓 unchanged sitemaps；
- 创建新 opportunity 前先更新 watchlist items。

### 13.3 Repair Run

当用户问“为什么漏了这个页面？”时，repair run 应该：

- 接收 URL；
- 直接 enrich；
- 与历史 recall inputs 对比；
- 生成 missed-reason report；
- 可选 materialize opportunity。

PostSyncer case 必须成为 first-class repair run test。

## 14. Rollout Plan

### Phase 0：Attribution and Diagnostics

目标：先打通轻量闭环，让漏召回可见，并修复 citation-derived competitor gaps。

Scope：

- 修复 `ClassifyContext` 空 `EvidenceIndex` 调用，把 qualifying public evidence 回流到 discovery vocabulary。
- 给 `AIManualDiscoveryPlanner` 增加 evidence 输入，并同步处理老 `geo/pr2`、`geo/pr3` 调用点。
- 补 `geo_competitors.domains` 写入路径。
- backfill 存量 active competitor domains。
- 为 `cited_urls` 增加 URL-to-entity classifier。
- 从 known competitor domains 派生 competitor citations。
- 存储 derived classification audit records。
- 增加 search evidence、citation URLs、competitor citations 的 run funnel counters。
- 加入 PostSyncer direct seed URL regression fixture。

Acceptance：

- EvidenceIndex 中的 public terms 能进入 accepted vocabulary，并影响 manual planner candidate validation。
- active competitor 至少能保存 domain，且 profile/manual/search_result 来源不会再永远写 `[]`。
- 如果 observation 的 `cited_urls` 包含 known competitor URL，analyzer 可以生成或更新 `geo_competitor_cited_project_absent`。
- run summary 能解释 competitive discovery 是 skipped、empty、degraded 还是 successful。

### Phase 1：SERP Domain Recall

目标：在 prompt planning 前发现新的 competitor / adjacent domains。

Scope：

- 从 product profile 生成 category seed queries。
- 按 query budget 收集 search results，并明确与现有 SearchBudget 的归并或隔离策略。
- normalize and classify hosts。
- 持久化 candidate domains、source query 和 reason codes。
- 增加 candidate competitor review state。

Acceptance：

- 对 UniPost，manual run 能通过 category queries 发现 Ayrshare/Zernio/PostForMe 之外的 domains。
- Search result domains 出现在 run funnel。
- competitive run 不会耗尽常规 evidence refresh 的 daily search budget，或者已通过独立配额清晰隔离。
- 系统能解释 PostSyncer 为什么被选中或为什么没被选中。

### Phase 2：Page and Sitemap Enrichment

目标：理解竞品站点结构。

Scope：

- 扩展 `internal/crawl` 支持 competitive candidate domains，而不是另建 crawler。
- 统一或明确复用 robots parser。
- fetch robots、sitemap、candidate pages。
- 抽取 metadata、canonical、schema hints、indexability、internal links、path clusters。
- 存储 enriched page candidates。
- 尊重 crawl budgets 和 robots policy。

Acceptance：

- 给定 `https://postsyncer.com/tools`，系统检测到它 crawlable、indexable、sitemap-included，并是 tools hub candidate。
- 系统能从 HTML 和 sitemap samples 估计 `/tools` leaf count。

### Phase 3：Archetype Mining and Opportunity Creation

目标：把竞品页面系统转成 project-fit opportunities。

Scope：

- 实现 tools hub、comparison cluster、alternatives cluster、scheduler cluster、integration cluster、template/checklist、source/report archetype miner。
- 实现 project-fit gates 和 risk gates。
- materialize `competitive_tools_hub_gap` 等 opportunity。
- 增加 opportunity card evidence blocks。

Acceptance：

- PostSyncer fixture 为 UniPost 创建 safe social content tools hub opportunity。
- opportunity 明确排除 downloader tools。
- 重复 run 更新 existing opportunity evidence，不重复创建。
- dismissed/snoozed/watching competitive opportunity 不会因为 freshness、demand proxy 或 priority score 微漂移而自动复活。

### Phase 4：Scheduling, Watchlist, and Learning

目标：让 competitive discovery 成为持续、低噪音能力。

Scope：

- Scheduled competitive refresh。
- detected-but-not-ready archetypes watchlist。
- 从 accept/dismiss feedback 学习。
- Admin controls for budgets and domains。

Acceptance：

- dismissed archetypes 根据 reason codes 降权。
- accepted competitive opportunities 提升类似 patterns 的未来优先级。
- scheduled runs 保持在 query/crawl budgets 内。

## 15. PostSyncer Regression Fixture

Regression fixture 使用 `https://postsyncer.com/tools` 作为 public seed URL。

Expected extracted facts：

- URL 返回 2xx。
- 页面 indexable。
- canonical 指向 `/tools`。
- robots allows crawling。
- sitemap includes `/tools`。
- title 提到 free social media tools。
- 页面包含大量 `/tools...` links。
- sitemap 包含相关 tools、comparison、scheduler pages。

Expected classification：

- Domain classification：adjacent competitor 或 competitor candidate。
- Page archetype：`tools_hub`。
- Project-fit for UniPost：medium/high。
- Excluded subpatterns：downloaders 和 trademark-sensitive utilities。
- Recommended asset type：`tool_hub`。

Expected opportunity：

- `type`：`competitive_tools_hub_gap`
- `recommended_action`：create a UniPost free social content tools hub。
- `risk_level`：medium。
- `confidence`：至少 medium。
- `evidence` 包含 PostSyncer URL、sitemap facts、link count 和 exclusion rationale。

## 16. Metrics

### 16.1 Activation Metrics

- 有至少一次 competitive discovery run 的 projects 占比。
- 每次 run 发现 candidate domain 的比例。
- 每次 run enrich 至少一个 competitor/adjacent page 的比例。
- 每次 run 检测到至少一个 archetype 的比例。
- detected archetypes 被 materialized 或 watchlisted 的比例。

### 16.2 Quality Metrics

- competitive opportunity accept rate。
- dismissed as irrelevant rate。
- dismissed as risky rate。
- duplicate opportunity rate。
- useful opportunity feedback rate。
- PostSyncer fixture pass rate。

### 16.3 Recall Metrics

- new domains discovered per run。
- new active competitor domains confirmed。
- SERP query coverage by category。
- pages fetched per domain。
- archetype coverage by type。
- held-out competitor domain recall@k：使用人工标注的 relevant competitor/adjacent domains 集合，衡量每次 run 在 top-k 候选中召回的比例。

### 16.4 Reliability Metrics

- crawl failure rate。
- robots-blocked rate。
- provider degraded rate。
- budget exhaustion rate。
- average run time。
- average cost per run。

## 17. 风险与缓解

| Risk | Mitigation |
|---|---|
| SERP neighbors 太多、噪音太高 | 先做 domain classification、category fit 和 watchlist，再创建 opportunity。 |
| 推荐变成 copycat | 强制 project-fit transformation 和 excluded subpatterns。 |
| 法务/商标风险 | 对 downloader tools、商标堆砌页、unsupported claims 增加 risk gates。 |
| crawl 成本失控 | per-run query/domain/page/sitemap sample budgets。 |
| 并发外网请求过多 | competitive recall 接入现有 Opportunity Finding 扇出时必须有并发上限，避免与 crawler audit、answer provider、external surfaces、search evidence 同时打满外网。 |
| robots/ToS 风险 | 尊重 robots，默认只存 derived metadata，跳过 blocked pages。 |
| duplicate opportunity 噪音 | stable identity keys；rerun 更新 existing open opportunity。 |
| dismissed opportunity 反复复活 | competitive fingerprint 排除易漂移项，或对 freshness/demand_proxy 做稳定分桶。 |
| LLM overreach | scoring deterministic；LLM 只做带证据审计的 summarization/classification。 |
| 用户不信任 | 展示 discovery funnel 和 missed-reason reports。 |

## 18. Open Questions

1. candidate competitor 是否允许在高 confidence 下自动 active，还是必须人工确认？
2. competitive discovery 的 query 消耗并入现有 SearchBudget，还是使用独立配额？如果并入，§9.1 的 query budget 必须显式受 daily/weekly/rolling 上限约束；如果独立，需要调整 SearchUsage 统计口径。
3. V1 是否允许短期保存完整竞品 HTML 方便 debugging，还是只存 derived facts？
4. competitive discovery 是否每次 manual Opportunity Finding 都运行，还是需要用户显式选择 competitive mode？
5. SERP recall 默认 search provider 用谁？quota 耗尽时如何 fallback？
6. candidate domain 是否复用 `geo_competitors` 的 status/source/domains，还是新增 `competitive_domain_candidates`？如果新增，review UI 如何避免双数据源？

## 19. Launch Criteria

Internal dogfood ready：

1. PostSyncer `/tools` direct seed URL enrichment 可用。
2. PostSyncer fixture 能为 UniPost 创建或更新 `competitive_tools_hub_gap` opportunity。
3. opportunity 包含 evidence、project-fit reasoning、excluded subpatterns。
4. citation URL classification 能从 known competitor domains 派生 competitor citations。
5. manual run detail 展示 recall、crawl、archetype、filter、materialization counters。
6. duplicate reruns 更新 evidence，不制造重复 open opportunities。
7. crawl/search budgets 可配置，并出现在 run output。
8. dismissed/snoozed/watching competitive opportunity 不会因 score/freshness 轻微变化被错误 reopen。

Customer beta ready：

1. 至少三个 archetypes 可用：tools hub、comparison cluster、alternatives cluster。
2. candidate competitor review 可用。
3. user-facing opportunity cards 能解释 source evidence 和 risk。
4. dismiss/accept feedback 影响未来 scoring。
5. 系统支持 observe-only mode，不创建 opportunities。
