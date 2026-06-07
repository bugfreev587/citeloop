# PRD：CiteLoop 持续 SEO 运营闭环

> 阶段：第二阶段，内部 MVP 可用之后的 SEO 运营产品化。
> 日期：2026-06-07
> 依赖：`docs/PRD-CiteLoop-MVP-Closure.md`
> 目标：让 CiteLoop 不只是发布内容，而是持续读取搜索表现、发现机会、生成可执行运营动作，并把每周最该做的 SEO 工作排给内部单人运营者；同时为真实用户保留 domain-only 接入路径。

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
- `searchAppearance` 不能作为主事实表的普通组合维度；实现必须把它作为单独聚合流，或作为 filter 查询具体 appearance 下的明细。
  - Source: https://developers.google.com/webmaster-tools/v1/how-tos/all-your-data
- Search Analytics 按 query/page 等维度切分的数据可能因匿名化和隐私保护缺失部分行；page 级总量必须单独按 page 维度拉取，不能由 query 行求和推导。
  - Source: https://developers.google.com/webmaster-tools/v1/how-tos/all-your-data
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
- 不把 Google Search Console / GA4 私有数据读取伪装成“只靠 domain 即可”；真实用户可以只输入产品 domain，但系统必须通过托管发布、站点所有权验证、OAuth 授权或公开数据降级来合法获得数据。
- 不直接自动删除或 noindex 页面；高风险动作只生成建议和草稿。
- 不把关键词难度、搜索量等第三方 SEO 数据作为 MVP 必需依赖；可作为增强 provider。

## 5. 用户与使用场景

### 5.1 用户

- 内部单人运营者：目标是运营 UniPost / CiteLoop 相关内容资产，需要系统每周告诉自己“做什么最有效”，而不是自己分析 GSC 表格。
- 真实 SaaS 用户：只应输入自己的产品 domain，例如 `example.com`，不应被要求理解或填写 `gsc_site_url`、GA4 property id、service account、credential ref、Search Console property type 等内部字段。

### 5.2 真实用户 domain-only 原则

真实用户 onboarding 的第一屏只收一个产品 domain。CiteLoop 后台必须把这个 domain 转换成可运营的 SEO property，并根据站点控制权选择数据接入模式：

1. **公开数据模式**：只依赖公开页面、sitemap、robots、metadata、链接图和可选 SERP provider。该模式无需用户额外配置，但没有 GSC/GA4 私有指标，系统只能生成冷启动机会和技术 SEO 建议。
2. **CiteLoop 托管内容模式**：当 CiteLoop 生成并托管 blog/docs/landing content surface 时，CiteLoop 可以自动注入 GA4/GTM/Search Console verification tag，使用平台级 service account 读取该托管 surface 的数据。用户仍然只输入产品 domain。
3. **客户自有站点模式**：如果要运营客户已经存在、且不由 CiteLoop 控制发布的站点，仅凭 domain 不能合法读取 GSC/GA4。CiteLoop 可以把验证流程托管成后台任务，但必须通过 Google OAuth、Site Verification token、DNS/HTML/meta 验证、GitHub/repo 写入或用户已有 Search Console 授权之一完成所有权证明。

因此，“用户只输入 domain”是产品体验约束，不是安全模型的豁免。UI 不暴露 Google 细节；系统状态必须清楚区分 `public_only`、`managed_content_connected`、`customer_site_pending_verification`、`customer_site_connected`。

### 5.3 核心场景

1. 周一打开 CiteLoop，看 SEO operating brief。
2. 系统展示本周 top opportunities。
3. 运营者接受其中 3 到 5 个动作。
4. CiteLoop 自动生成内容修改草稿或任务。
5. 运营者 review 后 approve。
6. 系统发布、验证、记录 baseline。
7. 7/14/28 天后系统回看 outcome，并把结论纳入后续 prioritization。

## 6. 数据源

### 6.0 数据接入分层

真实用户只输入产品 domain 后，CiteLoop 必须先完成 domain discovery：

- 规范化 root domain、canonical homepage、www/non-www、http/https。
- 发现 sitemap、robots、RSS、blog/docs path、canonical tags、主要语言和 country hint。
- 创建 `seo_properties`，并写入 onboarding mode 和 ownership state。
- 若用户选择 CiteLoop 托管内容 surface，自动创建可监测的 managed site URL。
- 若用户要求运营客户自有站点，进入 verification required 状态，不把缺失的 GSC/GA4 当成系统错误。

数据能力按层级开启：

| 层级 | 用户输入 | 数据能力 | 可生成动作 | 自动驾驶上限 |
|---|---|---|---|---|
| `public_only` | domain | crawl/sitemap/robots/page metadata/link graph/公开 SERP | 技术 SEO、内容库存、冷启动主题、发布节奏 | Level 1 |
| `managed_content_connected` | domain | public data + CiteLoop 托管 surface 的 GSC/GA4 | 完整 Operations Loop | Level 2 |
| `customer_site_connected` | domain + 后台完成验证/OAuth | public data + 客户自有站点 GSC/GA4 | 完整 Operations Loop | Level 2 |

没有 GSC/GA4 时，系统不得展示 CTR/position/conversion 作为事实指标；只能显示 `unavailable_until_verified` 或公开数据估算。

### 6.1 Google Search Console

必需。

同步流拆成三类，避免把不同粒度误写到同一事实表：

1. Query/page 主事实流：
   - `date`
   - `page`
   - `query`
   - `country`
   - `device`
2. Page total 事实流：
   - `date`
   - `page`
   - `country`
   - `device`
3. Search appearance 低频聚合流：
   - `date`
   - `searchAppearance`
   - 可选：对具体 `searchAppearance` 使用 filter 后再拉 page/query 明细，但不能把 `searchAppearance` 写入主表 unique key。

数据约束：

- `search_performance_daily` 只存 query/page 主事实流，`search_appearance` 固定为空或不存。
- `page_performance_daily` 的 clicks/impressions/CTR/position 来自 page total 事实流。
- GSC 会匿名化部分低频 query，因此 page total 不等于 query 行求和；Query gap 和 CTR baseline 必须把 `query_data_partial=true` 当作置信度折扣。

同步指标：

- `clicks`
- `impressions`
- `ctr`
- `position`

同步策略：

- 首次接入默认 backfill 最近 90 天；允许配置为最多 16 个月，但内部 MVP 不默认拉满。
- 每日拉取最近 28 天数据，覆盖 GSC 延迟、补数和验收所需窗口。
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

### 6.2.1 URL normalization

所有 GSC / GA4 / technical crawl / `articles.canonical_url` 写入前必须经过同一个 `normalize_seo_url(raw_url, site_url)` 函数，并同时保留原始 URL。

规则：

- scheme 和 host 小写；默认把同站 `http` 归一到 `https`，除非 property 配置明确保留 `http`。
- 移除 fragment。
- 移除 query string；允许 project config 白名单保留真正区分内容的 query key，默认移除 UTM、ref、fbclid、gclid 等 tracking 参数。
- path 做 percent-decoding 后再标准化重复斜杠。
- trailing slash 默认去除，根路径 `/` 除外。
- path 大小写默认保持；如目标站点确认大小写不敏感，可配置 lower-case path。
- GA4 `pagePath` 必须用 `site_url` 补全后再归一化。

所有 join 使用 `normalized_page_url`，展示层可显示原始 `page_url`。任何写入造成同一个 `normalized_page_url` 多行冲突时，后写入必须 upsert，而不是生成重复页面。

### 6.3 CiteLoop / UniPost 内部数据

必需。

- article id
- topic id
- slug
- canonical URL
- publish path
- content hash，当前表结构没有，Phase 1 必须迁移新增 `articles.content_hash`，并在内容或 metadata 变更时更新
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
- `input_domain`
- `managed_site_url`
- `onboarding_mode`：`public_only`, `managed_content`, `customer_site`
- `ownership_status`：`unverified`, `pending`, `verified`, `not_required`
- `gsc_site_url`
- `ga4_property_id`
- `url_normalization_config`
- `default_country`
- `default_language`
- `created_at`
- `updated_at`

### 7.2 `seo_integrations`

记录授权状态，不暴露 raw secret。内部单人 MVP 可以使用 platform service account + Search Console 站点授权；真实用户版必须把 integration 作为系统托管能力，普通用户不直接填写 credential ref。

字段：

- `id`
- `project_id`
- `provider`：`google_search_console`, `google_analytics`, `bigquery`, `serp_provider`
- `status`：`missing`, `connected`, `expired`, `error`
- `mode`：`platform_managed`, `user_oauth`, `site_verification`, `public_provider`
- `credential_ref`：指向 Railway/Vercel/local secret name；MVP 不把 Google credential payload 存进数据库
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
- `normalized_page_url`
- `query`
- `country`
- `device`
- `clicks`
- `impressions`
- `ctr`
- `position`
- `query_data_partial`：默认 `true`，提醒下游不要把 query 行求和当 page total
- `source`：`gsc_api`, `gsc_bigquery`
- unique key：`project_id + property_id + date + normalized_page_url + query + country + device`

### 7.4 `page_performance_daily`

page/day 聚合，用于趋势和 dashboard。

字段：

- `project_id`
- `property_id`
- `date`
- `page_url`
- `normalized_page_url`
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
- `data_source_notes`
- unique key：`project_id + property_id + date + normalized_page_url`

### 7.5 `search_appearance_daily`

Search appearance 聚合数据，低频同步，不与 query/page 主表共用 unique key。

字段：

- `project_id`
- `property_id`
- `date`
- `search_appearance`
- `clicks`
- `impressions`
- `ctr`
- `position`
- `source`：`gsc_api`, `gsc_bigquery`
- unique key：`project_id + property_id + date + search_appearance`

### 7.6 `url_index_snapshots`

重点 URL 的 index 状态快照。

字段：

- `id`
- `project_id`
- `run_id`
- `page_url`
- `normalized_page_url`
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
- unique key：`project_id + run_id + normalized_page_url`

### 7.7 `technical_checks`

技术 SEO 本地检查的原始结果。`page_performance_daily.technical_status` 只存摘要，来源必须可追溯到本表。

字段：

- `id`
- `project_id`
- `run_id`
- `page_url`
- `normalized_page_url`
- `article_id`
- `http_status`
- `canonical_status`
- `robots_status`
- `title_status`
- `meta_description_status`
- `h1_status`
- `structured_data_status`
- `sitemap_status`
- `internal_link_count`
- `outbound_link_count`
- `content_hash`
- `unsafe_mdx_detected`
- `raw_details`
- `checked_at`
- unique key：`project_id + run_id + normalized_page_url`

### 7.8 `seo_opportunities`

系统生成的机会。

字段：

- `id`
- `project_id`
- `type`
- `status`：`open`, `accepted`, `dismissed`, `converted`, `done`, `stale`
- `priority_score`
- `confidence`
- `page_url`
- `normalized_page_url`
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
- unique key：`project_id + type + normalized_page_url + coalesce(query, '') + created_by_run_id`

### 7.9 `content_actions`

运营者接受后的可执行动作。

字段：

- `id`
- `project_id`
- `opportunity_id`
- `action_type`
- `status`：`drafting`, `ready_for_review`, `approved`, `published`, `measuring`, `completed`, `failed`
- `target_article_id`
- `target_url`
- `normalized_target_url`
- `target_content_hash_before`
- `target_content_hash_after`
- `draft_article_id`
- `baseline_window`
- `measurement_window`
- `published_at`
- `outcome_summary`
- `created_at`
- `updated_at`
- unique key：`project_id + opportunity_id + action_type`

### 7.10 `internal_link_edges`

记录内部链接图。

字段：

- `project_id`
- `source_url`
- `normalized_source_url`
- `target_url`
- `normalized_target_url`
- `anchor_text`
- `link_context`
- `source_article_id`
- `target_article_id`
- `first_seen_at`
- `last_seen_at`
- unique key：`project_id + normalized_source_url + normalized_target_url + anchor_text`

### 7.11 `seo_runs`

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

关系：

- SEO 专用任务写 `seo_runs`，同时把 LLM 成本汇总写入或关联现有 `generation_runs`，避免绕过 MVP 月度 cost breaker。
- `seo_runs.cost_usd` 是 SEO run 内部明细；项目级硬上限仍以现有 `generation_runs` 月度预算为最终 breaker。

## 8. Opportunity 类型

### 8.1 Striking distance

定义：

- query/page 平均 position 在 4 到 20。
- impressions 高于项目 P60，且项目已有至少 50 个有效 query/page/day 样本；否则使用固定门槛 `impressions >= 50`。
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
- 每个 position bucket 至少有 20 个有效 query/page/day 样本；低于门槛时不生成 CTR rewrite opportunity。

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
- page 跌幅必须显著超过站点同期整体跌幅：`page_delta < site_delta - 0.15`，且 absolute drop 超过最小门槛。
- 如果全站同期下降超过 20%，默认降级为观察，不生成 decay action，除非该 page 额外低于全站 15 个百分点以上。

建议动作：

- refresh outdated claims
- update examples/screenshots
- add missing competitor/product changes
- republish with updated date only when 内容实质更新

### 8.4 Query gap

定义：

- GSC 出现相关 query，但没有明确 targeted article。
- 或 existing page 获得曝光但 intent 不完全匹配。
- 因 GSC 会匿名化低频 query，本类型只代表“可见 query gap”，不能声明覆盖全部长尾需求。
- page 级 impressions 明显高于 query 行 impressions 之和时，系统必须显示 `anonymous_query_gap` 提示，并降低 confidence。

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
- add noindex only after 人工确认。
- archive draft，不自动删除。

### 8.9 数据不足与冷启动行为

新站或新内容前 1 到 3 个月数据稀疏时，系统必须降级，而不是制造噪声。

项目级最小数据门槛：

- GSC 有效天数少于 14 天：只展示 indexing health、发布节奏、technical checks，不生成 query/CTR/decay 类 opportunity。
- 最近 28 天总 impressions 少于 500：禁用 P60/P80 相对阈值，改用固定低门槛并降低 confidence。
- 最近 28 天总 clicks 少于 30：不生成 CTR rewrite 和 content decay。
- 单个 page 最近 28 天 impressions 少于 50：不生成该 page 的 CTR rewrite。

Weekly brief 降级：

- 数据不足时 brief 标记为 `cold_start`。
- 内容从“top opportunities”改为“indexing health + publishing cadence + technical hygiene + next measurement milestones”。
- 验收“生成 5 类 opportunity”只适用于 seeded fixture 或真实数据达到上述门槛的项目；冷启动项目的正确行为是少生成或不生成。

## 9. Priority score

每个 opportunity 计算 `priority_score`。所有因子必须先归一化到 0 到 100，避免 impressions、decay percentage、affected URL count 等不同量纲不可比。

建议公式：

```text
priority_score =
  normalized_size * business_value * confidence
  / max(effort_points, 1)
  * risk_adjustment
```

### 9.1 Opportunity size

按 opportunity 类型计算 `normalized_size`：

- striking distance：`min(100, impressions_pctl * position_gain_factor)`，position 4 到 10 高于 11 到 20。
- CTR rewrite：`min(100, impressions_pctl * ctr_gap_ratio)`，`ctr_gap_ratio = max(0, baseline_ctr - actual_ctr) / max(baseline_ctr, 0.01)`。
- content decay：`min(100, abs(page_delta - site_delta) * 100)`，但必须满足 §8.3 的站点趋势排除规则。
- internal link：`min(100, target_business_value * source_relevance * source_traffic_pctl)`。
- indexing anomaly：高价值新 URL 固定 80，普通 URL 固定 40，并根据 technical confidence 调整。
- cannibalization / prune：按 affected clicks/impressions percentile 计算，但 risk adjustment 会显著折扣。

### 9.2 Business value

归一化到 0 到 100，来源：

- project config 中的 product priority
- query intent classification
- GA4 conversion 或 key event
- manual override

### 9.3 Confidence

归一化到 0 到 100，来源：

- 数据量是否足够
- trend 是否稳定
- URL/query mapping 是否明确
- 是否能定位具体动作

### 9.4 Effort

`effort_points` 枚举：

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

- top 5 到 10 actions
- why these actions
- expected effort
- previous week completed actions
- measurement updates
- blockers
- alerts

Brief 同时写入 app 和 notification。

### 10.6 Settings

内部管理员视图新增：

- Google Search Console service account connection
- GA4 connection
- optional BigQuery export config
- default country/language
- business value keywords
- ignored URLs / URL patterns
- opportunity thresholds
- weekly brief schedule

真实用户视图只展示：

- Product domain
- Crawl / sitemap / managed content / Google data connection health
- Connection status and blockers，用人能理解的语言解释，例如 “CiteLoop can analyze public pages now; Search Console data requires managed publishing or ownership verification.”
- Advanced diagnostics 可折叠展示 provider details，但不要求用户手动填写 `gsc_site_url`、GA4 property id 或 credential ref。

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
- 调用 writer 或 `reviser` 生成 draft。
- 绑定 evidence 和 diff。
- 生成前读取 `articles.content_hash`；发布前若 hash 变化，action 必须 abort 并转人工 review。

### 11.4 `reviser`

新 agent，专门处理 existing article 的受控修改；不是现有 writer 的隐式模式。

输入：

- existing article content + SEO metadata
- target opportunity
- required diff objective
- allowed evidence set
- current content hash

输出：

- patch 或完整 revised draft
- changed sections summary
- evidence mapping
- title/meta diff
- risk notes

约束：

- 不新增无 evidence 的产品事实。
- 不把 refresh 伪装成只更新日期。
- 不直接发布，必须进入 existing review gate。

### 11.5 `outcome_measurer`

职责：

- 在 7/14/28 天窗口评估动作结果。
- 标记 improved / neutral / worsened / inconclusive。
- 输出学习信号。

## 12. API endpoints

后端新增：

- `GET /api/projects/{projectID}/seo/overview`
- `POST /api/projects/{projectID}/seo/sync`
- `GET /api/projects/{projectID}/seo/runs?agent=&status=&limit=&cursor=`
- `GET /api/projects/{projectID}/seo/opportunities?type=&status=&limit=&cursor=`
- `GET /api/projects/{projectID}/seo/opportunities/{opportunityID}`
- `POST /api/projects/{projectID}/seo/opportunities/{opportunityID}/accept`
- `POST /api/projects/{projectID}/seo/opportunities/{opportunityID}/dismiss`
- `POST /api/projects/{projectID}/seo/opportunities/{opportunityID}/actions`
- `GET /api/projects/{projectID}/seo/actions?status=&limit=&cursor=`
- `GET /api/projects/{projectID}/seo/actions/{actionID}`
- `POST /api/projects/{projectID}/seo/actions/{actionID}/generate-draft`
- `POST /api/projects/{projectID}/seo/actions/{actionID}/approve`
- `POST /api/projects/{projectID}/seo/actions/{actionID}/publish`
- `GET /api/projects/{projectID}/seo/briefs/latest`
- `GET /api/projects/{projectID}/seo/settings`
- `PUT /api/projects/{projectID}/seo/settings`

约束：

- 所有 response 使用 normalized URL 和 display URL 双字段。
- 所有 mutation 写 `seo_runs` 或 audit record。
- 任何返回 Google/GA4 配置的 endpoint 只返回 connection status，不返回 secret。

## 13. 内容动作规则

### 13.1 Refresh

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

### 13.2 Metadata rewrite

输出：

- title candidates
- meta candidates
- chosen candidate
- reason

约束：

- 不承诺排名。
- 不 clickbait。
- 保持品牌语气。

### 13.3 Internal link patch

输出：

- source article patch
- anchor text
- target URL
- context sentence

约束：

- anchor 必须自然。
- 每篇 source article 单次最多新增 3 个内链。

### 13.4 Merge recommendation

输出：

- pages to merge
- winning canonical
- sections to preserve
- redirect/noindex recommendation
- risk explanation

约束：

- 只生成计划，不自动执行。

## 14. 通知事件

新增 notification event：

- `seo.sync.failed`
- `seo.auth.expired`
- `seo.opportunity.ready`
- `seo.brief.ready`
- `seo.action.measurement_ready`
- `seo.indexing.anomaly`

每个事件必须有 stable `event_id`，避免 spam。

## 15. 安全与合规

- 内部 MVP 使用 service account + GSC property 授权；真实用户版不得要求用户手动配置 service account。
- 仅凭 domain 不能读取客户 GSC/GA4 私有数据；CiteLoop 必须通过公开数据降级、平台托管内容 surface、Google OAuth、Site Verification 或用户显式授权获得合法访问。
- Google service account JSON 只保存为 secret reference；当前代码库没有 secret encryption 基础设施，因此不把 credential payload 写入数据库。
- API response 不返回 raw token。
- Logs 不输出 token、refresh token、authorization header 或 credential JSON。
- 只请求最小必要 scopes。
- 普通 blog URL 不使用 Google Indexing API。
- SERP 数据若接第三方 provider，必须使用 provider API，不抓取违反服务条款的 Google 页面。
- 所有自动生成内容仍复用 MVP fact-safety guard。
- 如果未来必须把 encrypted credential payload 存入 DB，该工作必须作为独立迁移和密钥轮换 PRD 处理。

## 16. 验收清单

1. 内部 MVP 可通过 service account 连接 Google Search Console property，并验证授权状态。
2. 首次接入默认 backfill 最近 90 天；每日可按天同步最近 28 天 Search Analytics 数据。
3. Query/page、page total、search appearance 三类同步流分表写入，且重复跑不会重复写。
4. URL/page/query 可映射到 CiteLoop article/topic。
5. SEO Overview 展示真实 clicks、impressions、CTR、position。
6. URL normalization 对 GSC URL、GA4 path、article canonical、technical crawl 使用同一函数，同页不会出现多行 page 聚合。
7. 达到数据门槛的项目至少生成以下 opportunity：striking distance、CTR rewrite、content decay、internal link、indexing anomaly。
8. 冷启动或数据不足项目不会生成噪声 opportunity，brief 会降级为 indexing health + publishing cadence。
9. Opportunity detail 展示 evidence、推荐理由和数据完整性提示。
10. 运营者可 accept/dismiss opportunity。
11. Accepted opportunity 可生成 content action。
12. Refresh action 能通过 `reviser` 生成文章 diff，并进入 existing review gate。
13. Metadata rewrite action 能更新 title/meta draft。
14. 真实用户 onboarding 首屏只要求输入 product domain。
15. 只输入 domain 的项目能进入 `public_only` 模式，并生成 crawl/sitemap/technical 冷启动 brief。
16. UI 不要求真实用户填写 `gsc_site_url`、GA4 property id 或 credential ref。
17. 需要 GSC/GA4 私有数据时，系统展示托管发布或所有权验证路径，而不是把缺失配置作为用户错误。
18. Internal link action 能生成 source article patch。
19. 目标文章 content hash 变化时，action_generator abort 并转人工 review。
20. Weekly brief 可生成 5 到 10 个 action，并在 app 内查看。
21. Weekly brief 可通过 verified notification channel 投递。
22. Action 发布后进入 measurement 状态。
23. 7/14/28 天 outcome measurement 可写回。
24. Google auth 失效会显示配置错误并通知。
25. GSC 数据延迟和匿名 query 缺口不会误报为 0 表现。
26. URL Inspection 只用于重点 URL，不做无界全站扫描。
27. 普通 blog 不调用 Google Indexing API。
28. 第二阶段交接给 Autopilot 的前置状态可被机器判断：连续 14 天 sync 成功、最近 28 天 GSC 数据可用、URL normalization 无重复冲突、verified notification channel 存在。
29. `go test ./...` 通过。
30. `web npm run build` 通过。

## 17. Rollout plan

### Phase 1：Data foundation

- Service account connection。
- `articles.content_hash` migration。
- URL normalization。
- GSC Search Analytics ingestion。
- page total / query-page / search appearance 分流。
- technical_checks。
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

## 18. Definition of Done

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
