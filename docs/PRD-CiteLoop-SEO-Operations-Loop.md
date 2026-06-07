# PRD：CiteLoop 持续 SEO 运营闭环

> 阶段：第二阶段，内部 MVP 可用之后的 SEO 运营产品化。
> 日期：2026-06-07
> 依赖：`docs/PRD-CiteLoop-MVP-Closure.md`
> 目标：让 CiteLoop 不只是发布内容，而是持续读取搜索表现、发现机会、生成可执行运营动作，并把每周最该做的 SEO 工作排给内部单人运营者。

## 1. 第一性原理

SEO 运营的核心不是“写更多文章”，而是在有限产能下，把每一次内容动作投向最可能提高搜索可见度、点击、转化和产品理解的位置。

因此本阶段必须回答五个基础问题：

1. **哪些页面真的在搜索中获得曝光？** 以 Google Search Console Search Analytics 为主要外部事实来源。
2. **哪些 query/page 组合有机会？** 机会来自 impressions、position、CTR、clicks、index status、engagement、conversion 的组合，而不是单一关键词难度分。
3. **每个机会应该采取什么动作？** 新写、刷新、标题改写、合并、内链、修技术问题、等待观察，必须有可解释理由。
4. **动作是否可验证？** 每个动作必须有 baseline、执行记录、观察窗口和 outcome。
5. **单人运营能不能每周做完？** 系统必须把动作压缩成少量高杠杆任务，而不是制造一个新的数据后台。

本 PRD 的产品方向是“SEO operating loop”，不是“全自动 SEO agent”。自动执行留到第三阶段 Autopilot PRD。

## 2. 官方事实边界

本 PRD 的 Google 侧能力基于以下官方能力：

- Search Console Search Analytics API 可以按 date、query、page、country、device 等维度查询点击、曝光、CTR、position；`rowLimit` 范围为 1 到 25,000。
  - Source: https://developers.google.com/webmaster-tools/v1/searchanalytics/query
- Search Console URL Inspection API 可查询 Google index 中某个 URL 的状态，但 API 当前查看的是 Google index 版本，不做 live URL test。
  - Source: https://developers.google.com/webmaster-tools/v1/urlInspection.index/inspect
- Search Console Bulk Data Export 可以把 Search Console 数据持续导出到 BigQuery，用于更大规模或更长历史的数据分析。
  - Source: https://support.google.com/webmasters/answer/12919198
- GA4 Data API 可通过 `runReport` 查询 GA4 event/reporting 数据。
  - Source: https://developers.google.com/analytics/devguides/reporting/data/v1/basics
  - Source: https://developers.google.com/analytics/devguides/reporting/data/v1/api-schema
- Search Console Sitemaps API 可以提交 sitemap。
  - Source: https://developers.google.com/webmaster-tools/v1/sitemaps/submit
- Google Indexing API 只面向 JobPosting 或带 BroadcastEvent 的 VideoObject 页面，不作为普通 blog URL 的自动提交流程。
  - Source: https://developers.google.com/search/apis/indexing-api/v3/using-api

产品实现不得暗示或依赖“普通博客文章可通过 Google Indexing API 批量强制索引”。

## 3. 目标

1. 连接 Google Search Console，并按天同步 UniPost / CiteLoop 生成页面的搜索表现。
2. 可选连接 GA4，把 landing/page engagement 和 conversion 纳入 SEO 决策。
3. 维护 page、query、topic、article、canonical URL 的统一映射。
4. 自动生成 SEO opportunity queue，并给出清晰动作建议。
5. 支持一键从 opportunity 生成内容动作：
   - refresh canonical
   - rewrite SEO title/meta
   - add internal links
   - create follow-up topic
   - merge/prune recommendation
   - technical SEO fix task
6. 每周生成“SEO operating brief”，让内部运营者知道本周最该做的 5 到 10 件事。
7. 每个动作都能追踪 baseline、执行时间、观察窗口和 outcome。
8. 失败和异常通过现有 notification 系统投递。

## 4. 非目标

- 不保证排名提升。
- 不做黑帽 SEO、批量伪外链、PBN、自动评论、自动论坛 spam。
- 不绕过 Google Search Console / GA4 / SERP provider 的授权与配额限制。
- 不使用 Google Indexing API 批量提交普通 blog 页面。
- 不做外部 SaaS 多租户授权；仍然服务内部单人运营。
- 不直接自动删除或 noindex 页面；高风险动作只生成建议和草稿。
- 不把关键词难度、搜索量等第三方 SEO 数据作为 MVP 必需依赖；可作为增强 provider。

## 5. 用户与使用场景

### 5.1 用户

- 内部单人运营者。
- 目标是运营 UniPost / CiteLoop 相关内容资产。
- 需要系统每周告诉自己“做什么最有效”，而不是自己分析 GSC 表格。

### 5.2 核心场景

1. 周一打开 CiteLoop，看 SEO operating brief。
2. 系统展示本周 top opportunities。
3. 运营者接受其中 3 到 5 个动作。
4. CiteLoop 自动生成内容修改草稿或任务。
5. 运营者 review 后 approve。
6. 系统发布、验证、记录 baseline。
7. 7/14/28 天后系统回看 outcome，并把结论纳入后续 prioritization。

## 6. 数据源

### 6.1 Google Search Console

必需。

同步维度：

- `date`
- `page`
- `query`
- `country`
- `device`
- `searchAppearance`，如果有可用数据

同步指标：

- `clicks`
- `impressions`
- `ctr`
- `position`

同步策略：

- 每日拉取最近 7 天数据，覆盖 GSC 延迟和补数。
- 每周拉取最近 90 天聚合，修正历史 baseline。
- 对新发布 URL，在 publish 后第 1、3、7、14、28 天检查是否产生 impressions。
- 如果数据量超过 API 分页/row limit 管理复杂度，允许升级到 BigQuery Bulk Export ingestion。

### 6.2 GA4

可选但强烈建议。

同步维度：

- `date`
- `pagePath`
- `landingPage`
- `sessionSourceMedium`
- `deviceCategory`

同步指标：

- users
- sessions
- engagedSessions
- averageEngagementTime
- conversions / key events
- totalRevenue，如未来有商业转化

用途：

- 区分“有排名但无业务价值”的页面。
- 给 topic priority 加 business value。
- 识别高点击低 engagement 的内容质量问题。

### 6.3 CiteLoop / UniPost 内部数据

必需。

- article id
- topic id
- slug
- canonical URL
- publish path
- content hash
- publish date
- review history
- generated / human edited 标记
- inventory evidence mapping
- internal links
- outbound links
- title/meta/frontmatter

### 6.4 URL Inspection

用于抽样和重点 URL。

优先级：

1. 新发布 7 天后仍无 impressions 的 canonical。
2. 高价值 refresh 后需要确认 index 状态的 canonical。
3. Search Console 表现异常下降的 URL。
4. sitemap 中存在但 GSC 没表现的 URL。

限制：

- 不做全站每日 URL Inspection。
- 不把 URL Inspection 当作排名证明。
- 不把 “URL is on Google” 视为一定有排名或展示。

### 6.5 技术 SEO 本地检查

CiteLoop 自己抓取 UniPost URL，检查：

- HTTP status
- canonical tag
- robots meta
- title/meta description 长度和存在性
- H1
- structured data 是否可解析
- sitemap 是否包含 URL
- internal link count
- outbound link count
- content hash
- script tag / unsafe MDX
- page speed 不作为本阶段核心指标，只记录可选诊断

## 7. 数据模型

### 7.1 `seo_properties`

记录一个可运营站点。

字段：

- `id`
- `project_id`
- `site_url`
- `gsc_site_url`
- `ga4_property_id`
- `default_country`
- `default_language`
- `created_at`
- `updated_at`

### 7.2 `seo_integrations`

记录授权状态，不暴露 raw secret。

字段：

- `id`
- `project_id`
- `provider`：`google_search_console`, `google_analytics`, `bigquery`, `serp_provider`
- `status`：`missing`, `connected`, `expired`, `error`
- `credential_ref` 或 encrypted credential payload
- `last_verified_at`
- `last_error`
- `created_at`
- `updated_at`

### 7.3 `search_performance_daily`

page/query/day 粒度。

字段：

- `project_id`
- `property_id`
- `date`
- `page_url`
- `query`
- `country`
- `device`
- `search_appearance`
- `clicks`
- `impressions`
- `ctr`
- `position`
- `source`：`gsc_api`, `gsc_bigquery`
- unique key：`project_id + property_id + date + page_url + query + country + device + search_appearance`

### 7.4 `page_performance_daily`

page/day 聚合，用于趋势和 dashboard。

字段：

- `project_id`
- `property_id`
- `date`
- `page_url`
- `article_id`
- `topic_id`
- `clicks`
- `impressions`
- `weighted_position`
- `ctr`
- `ga4_sessions`
- `ga4_engaged_sessions`
- `ga4_conversions`
- `indexed_state`
- `technical_status`

### 7.5 `url_index_snapshots`

重点 URL 的 index 状态快照。

字段：

- `id`
- `project_id`
- `page_url`
- `article_id`
- `inspection_status`
- `coverage_state`
- `google_canonical`
- `user_canonical`
- `last_crawl_time`
- `robots_txt_state`
- `page_fetch_state`
- `raw_summary`
- `inspected_at`

### 7.6 `seo_opportunities`

系统生成的机会。

字段：

- `id`
- `project_id`
- `type`
- `status`：`open`, `accepted`, `dismissed`, `converted`, `done`, `stale`
- `priority_score`
- `confidence`
- `page_url`
- `article_id`
- `topic_id`
- `query`
- `evidence`
- `recommended_action`
- `expected_impact`
- `effort`
- `risk_level`
- `created_by_run_id`
- `created_at`
- `updated_at`

### 7.7 `content_actions`

运营者接受后的可执行动作。

字段：

- `id`
- `project_id`
- `opportunity_id`
- `action_type`
- `status`：`drafting`, `ready_for_review`, `approved`, `published`, `measuring`, `completed`, `failed`
- `target_article_id`
- `target_url`
- `draft_article_id`
- `baseline_window`
- `measurement_window`
- `published_at`
- `outcome_summary`
- `created_at`
- `updated_at`

### 7.8 `internal_link_edges`

记录内部链接图。

字段：

- `project_id`
- `source_url`
- `target_url`
- `anchor_text`
- `link_context`
- `source_article_id`
- `target_article_id`
- `first_seen_at`
- `last_seen_at`

### 7.9 `seo_runs`

记录同步、分析、brief、measurement 任务。

字段：

- `id`
- `project_id`
- `agent`
- `status`
- `started_at`
- `finished_at`
- `cost_usd`
- `input`
- `output`
- `error`

## 8. Opportunity 类型

### 8.1 Striking distance

定义：

- query/page 平均 position 在 4 到 20。
- impressions 高于项目 P60。
- 页面与 query intent 匹配。

建议动作：

- refresh section
- add FAQ
- improve title/H1
- add internal links
- create comparison section

### 8.2 CTR rewrite

定义：

- position 1 到 10。
- impressions 高。
- CTR 低于同 position bucket 的项目 baseline。

建议动作：

- SEO title rewrite
- meta description rewrite
- add rich-result eligible structured data if appropriate
- improve snippet-oriented intro

注意：

- CTR 可能受品牌、SERP layout、AI Overview、广告影响。
- 不自动承诺 CTR 提升。

### 8.3 Content decay

定义：

- page clicks 或 impressions 相对 28/56/90 天 baseline 明显下降。
- 排除季节性或站点整体下降。

建议动作：

- refresh outdated claims
- update examples/screenshots
- add missing competitor/product changes
- republish with updated date only when内容实质更新

### 8.4 Query gap

定义：

- GSC 出现相关 query，但没有明确 targeted article。
- 或 existing page 获得曝光但 intent 不完全匹配。

建议动作：

- create follow-up topic
- create glossary/supporting article
- add section to existing article

### 8.5 Cannibalization

定义：

- 同一 query 有多个 page 获得 impressions。
- position/clicks 在多个 page 间波动。
- 页面 intent 高度重叠。

建议动作：

- merge
- canonical consolidation
- internal link rewrite
- retarget one page to narrower intent

高风险：

- 不自动合并或删除。
- 只生成 recommendation 和 draft plan。

### 8.6 Internal link opportunity

定义：

- target page 高价值但 internal links 少。
- source page 有相关段落且已有流量。

建议动作：

- 给 source article 生成 anchor/context patch。
- 生成 review diff，由运营者 approve。

### 8.7 Indexing anomaly

定义：

- published URL `2xx`，sitemap 包含，但 7/14/28 天无 impressions。
- URL Inspection 显示非 indexed 或 canonical mismatch。

建议动作：

- 修 canonical/noindex/robots/sitemap。
- 增加内链。
- 重新提交 sitemap。
- 对少量关键 URL 提醒人工使用 Search Console URL Inspection UI 请求 indexing。

注意：

- Google Indexing API 不用于普通 blog。

### 8.8 Prune / merge candidate

定义：

- 90 天无 impressions 或 clicks。
- GA4 engagement 低。
- 没有明显战略价值。
- 与其他页面主题重复或质量低。

建议动作：

- merge into stronger article。
- add noindex only after人工确认。
- archive draft，不自动删除。

## 9. Priority score

每个 opportunity 计算 `priority_score`。

建议公式：

```text
priority_score =
  opportunity_size * business_value * confidence
  / max(effort, 1)
  * risk_adjustment
```

### 9.1 Opportunity size

由以下信号组成：

- impressions
- current average position
- CTR gap
- decay magnitude
- query/page count
- affected URL count

### 9.2 Business value

来自：

- project config 中的 product priority
- query intent classification
- GA4 conversion 或 key event
- manual override

### 9.3 Confidence

来自：

- 数据量是否足够
- trend 是否稳定
- URL/query mapping 是否明确
- 是否能定位具体动作

### 9.4 Effort

枚举：

- 1：metadata rewrite
- 2：small section refresh
- 3：new supporting article
- 5：major rewrite/merge
- 8：technical/site-level fix

### 9.5 Risk adjustment

- low risk：1.0
- medium risk：0.7
- high risk：0.4

## 10. 用户体验

### 10.1 SEO Overview

展示：

- last 28 days clicks/impressions/CTR/position
- trend vs previous period
- indexed / pending / anomaly URL counts
- top gaining pages
- top decaying pages
- open opportunities by type
- actions in measurement

### 10.2 Opportunities

列表字段：

- type
- priority score
- page
- query
- why this matters
- recommended action
- expected impact
- risk
- effort
- status

操作：

- accept
- dismiss
- convert to content action
- assign review date
- open evidence

### 10.3 Opportunity detail

必须展示：

- GSC trend chart
- query/page table
- baseline window
- source URLs
- current article metadata
- generated reasoning
- action proposal
- risk notes

### 10.4 Content action queue

展示：

- generated draft
- diff
- linked opportunity
- expected measurement window
- approve/reject
- publish status

### 10.5 Weekly brief

每周生成：

- top 5 actions
- why these 5
- expected effort
- previous week completed actions
- measurement updates
- blockers
- alerts

Brief 同时写入 app 和 notification。

### 10.6 Settings

新增：

- Google Search Console connection
- GA4 connection
- optional BigQuery export config
- default country/language
- business value keywords
- ignored URLs / URL patterns
- opportunity thresholds
- weekly brief schedule

## 11. Agent 与 workflow

### 11.1 `seo_sync`

职责：

- 拉取 GSC/GA4 数据。
- upsert performance tables。
- 记录 freshness、quota、errors。

触发：

- daily cron
- manual sync

失败：

- auth expired -> alert
- quota exhausted -> degraded run
- data delay -> warning，不算 failure

### 11.2 `seo_analyzer`

职责：

- 生成 opportunity。
- 更新 stale opportunity。
- 计算 priority score。

触发：

- daily after sync
- manual analyze

### 11.3 `action_generator`

职责：

- 把 accepted opportunity 转成 concrete content action。
- 调用 writer/reviser 生成 draft。
- 绑定 evidence 和 diff。

### 11.4 `outcome_measurer`

职责：

- 在 7/14/28 天窗口评估动作结果。
- 标记 improved / neutral / worsened / inconclusive。
- 输出学习信号。

## 12. 内容动作规则

### 12.1 Refresh

输入：

- existing article
- GSC query/page evidence
- product evidence
- competitor/search context if available

输出：

- edited article draft
- changed sections summary
- SEO metadata update
- QA evidence report

约束：

- 不得引入无 evidence 的产品事实。
- 不得只改日期而不改内容。

### 12.2 Metadata rewrite

输出：

- title candidates
- meta candidates
- chosen candidate
- reason

约束：

- 不承诺排名。
- 不 clickbait。
- 保持品牌语气。

### 12.3 Internal link patch

输出：

- source article patch
- anchor text
- target URL
- context sentence

约束：

- anchor 必须自然。
- 每篇 source article 单次最多新增 3 个内链。

### 12.4 Merge recommendation

输出：

- pages to merge
- winning canonical
- sections to preserve
- redirect/noindex recommendation
- risk explanation

约束：

- 只生成计划，不自动执行。

## 13. 通知事件

新增 notification event：

- `seo.sync.failed`
- `seo.auth.expired`
- `seo.opportunity.ready`
- `seo.brief.ready`
- `seo.action.measurement_ready`
- `seo.indexing.anomaly`

每个事件必须有 stable `event_id`，避免 spam。

## 14. 安全与合规

- Google OAuth tokens 必须加密保存或保存 secret reference。
- API response 不返回 raw token。
- Logs 不输出 token、refresh token、authorization header。
- 只请求最小必要 scopes。
- 普通 blog URL 不使用 Google Indexing API。
- SERP 数据若接第三方 provider，必须使用 provider API，不抓取违反服务条款的 Google 页面。
- 所有自动生成内容仍复用 MVP fact-safety guard。

## 15. 验收清单

1. 可连接 Google Search Console property。
2. 可按天同步最近 28 天 Search Analytics 数据。
3. 数据同步 idempotent，重复跑不会重复写。
4. URL/page/query 可映射到 CiteLoop article/topic。
5. SEO Overview 展示真实 clicks、impressions、CTR、position。
6. 至少生成以下 opportunity：striking distance、CTR rewrite、content decay、internal link、indexing anomaly。
7. Opportunity detail 展示 evidence 和推荐理由。
8. 运营者可 accept/dismiss opportunity。
9. Accepted opportunity 可生成 content action。
10. Refresh action 能生成文章 diff，并进入 existing review gate。
11. Metadata rewrite action 能更新 title/meta draft。
12. Internal link action 能生成 source article patch。
13. Weekly brief 可生成并在 app 内查看。
14. Weekly brief 可通过 verified notification channel 投递。
15. Action 发布后进入 measurement 状态。
16. 7/14/28 天 outcome measurement 可写回。
17. Google auth 失效会显示配置错误并通知。
18. GSC 数据延迟不会误报为 0 表现。
19. URL Inspection 只用于重点 URL，不做无界全站扫描。
20. 普通 blog 不调用 Google Indexing API。
21. `go test ./...` 通过。
22. `web npm run build` 通过。

## 16. Rollout plan

### Phase 1：Data foundation

- Google OAuth / service account connection。
- GSC Search Analytics ingestion。
- page/query/article mapping。
- SEO Overview。

### Phase 2：Opportunity engine

- opportunity schema。
- scoring。
- top opportunity types。
- opportunity list/detail UI。

### Phase 3：Action generation

- refresh draft。
- metadata rewrite。
- internal link patch。
- content action queue。

### Phase 4：Brief and measurement

- weekly brief。
- notification integration。
- outcome measurement。
- learning summary。

## 17. Definition of Done

内部运营者每周可以完成以下流程：

1. 打开 SEO Overview。
2. 查看 Weekly brief。
3. 接受 3 到 5 个 high-priority opportunity。
4. 让 CiteLoop 生成 refresh / metadata / internal link draft。
5. Review and approve。
6. 发布到 UniPost。
7. 7/14/28 天后看到 outcome。
8. 下一周 brief 自动吸收上周结果。

做到这里，CiteLoop 才从“自动发布内容”变成“能持续帮你做 SEO 运营”。
