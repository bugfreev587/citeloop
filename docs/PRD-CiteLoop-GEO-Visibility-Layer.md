# PRD: CiteLoop GEO Visibility Layer

> 阶段：SEO Operations Loop foundation 之后，SEO Autopilot 自动执行之前。
> 目标：让 CiteLoop 从“能生成和运营 SEO 内容”升级为“能发现、衡量并改善 AI answer / generative search visibility 的运营系统”。
> 术语：本文中的 GEO 指 Generative Engine Optimization，即 ChatGPT Search、Perplexity、Claude Search、Google AI Overviews / AI Mode、Bing/Copilot 等 answer engine 中的品牌提及、引用、链接和答案吸收能力。

## 0. 当前实现盘点

本盘点基于当前代码库，而不是 PRD 设想。

| 能力 | 当前状态 | 已有实现 | 主要缺口 |
|---|---:|---|---|
| 技术 SEO / 可抓取性 | 部分实现 | `internal/crawl` 支持 sitemap、robots、same-origin、限深限页；`internal/seo.Service.Sync` 对已发布 canonical URL 做 HTTP/title/meta/H1/canonical/robots/JSON-LD/link count 轻量检查；`technical_checks` 已建表。 | 缺少 domain-wide discovery audit、sitemap inclusion 校验、AI crawler/WAF access audit、结构化数据解析验证、技术问题到 opportunity 的细分生成。 |
| GSC / GA4 数据闭环 | 部分实现 | `googledata.Client` 可拉 Search Console 和 GA4；`page_performance_daily`、`search_performance_daily`、`search_appearance_daily` 已建表；SEO overview/brief UI 已有。 | Analyzer 目前主要生成 `indexing_anomaly`，未实现 CTR/position/query decay、内容刷新、内链、合并/裁剪、结构化数据机会。 |
| SEO opportunity / action queue | 部分实现 | `seo_opportunities`、`content_actions`、brief、queue/dismiss/create action API 和 UI 已有。 | `generate-draft` / `publish` 仍只是状态更新；没有真正把 opportunity 转成内容改写、页面生成或技术修复。 |
| Autopilot observe-only | 部分实现 | objective、policy、risk classifier、plan、safe mode 的 schema/API/UI 已有。 | 仍是 observe/plan foundation，未执行真实变更，也未连接 GEO 指标。 |
| AI crawler 可见性 | 未实现 | 无专门代码。 | 未检查 `OAI-SearchBot`、`PerplexityBot`、`Claude-SearchBot`、`Claude-User`、Bing/Copilot 相关访问；未检查 Cloudflare/WAF/CAPTCHA/403/429；未记录 crawler access evidence。 |
| AI 搜索/引用监控 | 未实现 | `seo_objectives.primary_metric` 预留了 `ai_visibility`。 | 无 prompt set、answer engine run、citation observation、brand mention、competitor citation、AI visibility score。 |
| 产品型页面 / 工具页 / 模板页 | 未实现 | Topic/Writer 主要产出 canonical article 和 syndication variant。 | 无 comparison/use-case/template/tool/report/glossary 等页面资产类型；无非 blog surface 的 Publisher contract。 |
| 引用资产 | 部分实现 | Writer prompt 要求 GEO 策略，包括自包含块、统计、引用、Q&A、权威语气。 | 没有“可被引用资产”的产品化：数据小报告、benchmark、definition block、comparison table、template/checklist、source bundle、事实更新机制。 |
| 第三方分发 | 部分实现 | Semi-manual lane 支持 Dev.to、Hashnode、Reddit；能回填 canonical/source link 和 compose URL。 | 无自动发布 connector；无外部分发 URL 回收、backlink/canonical 验证、外部页面是否被 AI 引用的追踪。 |
| 实体/权威信号 | 未实现 | 无专门代码。 | 未维护品牌实体画像、founder/company profile、官方社媒、GitHub、目录站、review site、第三方提及一致性。 |

## 1. 背景与机会

传统 SEO 依赖页面排名和点击。GEO 多了一层：answer engine 会把多个来源检索、筛选、合成，并可能只引用少量页面。对 CiteLoop 来说，继续只发 blog 会遇到两个问题：

1. 内容可能被抓取和索引，但没有被 answer engine 选中为引用来源。
2. answer engine 可能引用竞品、社区讨论或第三方目录，而不是引用产品官网。

Google 官方说明 AI Overviews / AI Mode 仍使用基础 SEO 规则，不需要特殊 AI 文件或特殊 schema，但要求页面可抓取、可索引、内容文本可理解、内链清晰、结构化数据与可见内容一致。OpenAI、Perplexity、Anthropic 也分别公开了搜索/用户访问 crawler 的 robots 与 WAF 配置建议。因此 CiteLoop 的高价值新增层不应该是“玄学 AI 文案”，而应该是一个可观测的 GEO loop：

domain access -> prompt monitoring -> citation gap -> asset/action queue -> publishing/distribution -> measurement.

## 2. 官方事实边界

- Google Search Central 的 AI features guidance 明确：AI Overviews / AI Mode 沿用基础 SEO 最佳实践；页面需要满足 Search technical requirements、可抓取、可索引、内容以文本形式可理解、内链清晰、结构化数据匹配可见内容；不需要额外的 AI text files 或特殊 schema 才能出现于这些功能中。Source: https://developers.google.com/search/docs/appearance/ai-features
- OpenAI Publisher FAQ 明确：公开网站可出现在 ChatGPT search；要让内容被发现、展示、清楚引用和链接，不能阻止 `OAI-SearchBot`；允许访问后，ChatGPT search referral 会带 `utm_source=chatgpt.com` 便于分析。Source: https://help.openai.com/en/articles/12627856-publishers-and-developers-faq
- Perplexity crawler docs 区分 `PerplexityBot` 和 `Perplexity-User`；前者用于在 Perplexity search results 中 surface/link websites，建议通过 robots.txt 与 WAF allowlist 放行；后者用于用户请求触发的访问。Source: https://docs.perplexity.ai/docs/resources/perplexity-crawlers
- Anthropic Help Center 区分 `ClaudeBot`、`Claude-User`、`Claude-SearchBot`；`Claude-SearchBot` 用于改善搜索结果质量，禁用它可能降低搜索结果中的可见性和准确性；Anthropic 表示其 bots 尊重 robots.txt。Source: https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler

## 3. 目标

1. 自动判断一个项目是否能被主要 AI 搜索 crawler 和用户触发 fetcher 访问。
2. 为每个项目生成并维护一组可审核的 GEO prompt set，覆盖 category、problem、comparison、alternative、how-to、integration、buyer-intent 等查询。
3. 定期运行 answer engine observation，记录品牌是否被提及、是否被引用、引用 URL、竞品引用、答案中的来源排序和证据缺口。
4. 把 GEO gap 转成现有 `seo_opportunities` 和 `content_actions` 可处理的运营任务。
5. 生成高引用价值资产 brief：comparison page、alternative page、template/checklist、benchmark/report、glossary/definition、integration/docs page。
6. 将第三方分发 URL 纳入监控，判断外部分发是否形成引用、反链或品牌实体信号。

## 4. 非目标

- 不保证 ChatGPT、Google、Perplexity、Claude 或 Copilot 一定引用某个页面。
- 不做黑帽 SEO、伪造 mention、批量 forum spam、PBN、虚假评论、自动刷引用。
- 不试图绕过 robots.txt、WAF、CAPTCHA 或平台 ToS。
- 不伪造 OpenAI、Perplexity、Anthropic、Google 或 Bing 的 crawler User-Agent 做 HTTP 抓取；实际 HTTP 请求必须使用 CiteLoop 自己的诚实 User-Agent。
- 不在本 PRD 中做第三方平台全自动发帖；自动发布属于后续 connector PRD。
- 不把 LLM 生成的回答当作绝对事实；每条 observation 必须保留 engine、prompt、时间、来源和置信度。
- 不采集用户私有 ChatGPT/Google 账号数据；需要登录态的观察必须走人工导入或明确授权的 provider。

## 5. 产品原则

1. **可观测优先**：先知道为什么没被引用，再决定写什么。
2. **证据优先**：所有 GEO opportunity 必须有 prompt、answer、citation、competitor 或 crawler evidence。
3. **沿用 SEO loop**：GEO 不另起一套任务系统，最终进入 `seo_opportunities`、brief、action queue 和 review gate。
4. **区分 search crawler 与 training crawler**：CiteLoop 只建议与搜索/用户可见性相关的 crawler access；训练 opt-out 由用户策略决定。
5. **不牺牲事实安全**：引用资产里的产品声明继续受现有 QA evidence mapping gate 约束。

## 6. 用户场景

### 6.1 Internal Operator

1. 打开项目 SEO/GEO 页面。
2. 看到 “AI crawler access: blocked on PerplexityBot by Cloudflare WAF”。
3. 点击查看 evidence：robots rule、HTTP 状态、sample URL、User-Agent、时间。
4. 生成修复任务，交给工程或平台配置。
5. 修复后 rerun audit，状态变为 pass。

### 6.2 Growth Operator

1. CiteLoop 基于产品 profile 生成 30 到 80 个 GEO prompts。
2. 用户审核并锁定 prompt set。
3. 每周运行 observation。
4. Brief 显示：`"social media scheduling API"` prompt 中竞品被引用 6 次，UniPost 未出现。
5. 系统建议生成 `UniPost vs Buffer API automation` comparison page，并附上需要补充的证据块。

### 6.3 SaaS Customer

1. 用户只输入 domain。
2. CiteLoop 在 public-only 模式下完成 crawler/access audit 和公开 SERP/GEO prompt fixture。
3. 用户授权 GSC/GA4 或连接 publishing access 后，GEO opportunity 可转成内容动作。

## 7. 功能范围

### 7.1 AI Crawler Access Audit

对项目的 canonical homepage、blog root、docs root、最近 canonical articles、sitemap 中样本 URL 进行 crawler 可见性审计。审计结果必须区分权威事实和启发式推断：

1. **Robots 判定是权威事实**：只下载并静态解析 `robots.txt`，判断各目标 user-agent 是否被 Allow/Disallow。
2. **HTTP/WAF 判定是启发式信号**：实际请求使用 CiteLoop 自己的 User-Agent，不能冒充 `OAI-SearchBot`、`PerplexityBot`、`Claude-SearchBot` 等第三方 bot。`403`、challenge、captcha、rate limit 只能说明 CiteLoop probe 遇到阻断，不能证明真实 bot 一定被阻断。
3. **UI 必须展示置信等级**：`robots_disallowed` 是 high confidence；`suspected_waf_block` 是 inferred，需要人工或平台侧确认；`manual_required` 不得自动生成高置信结论。

需要检查：

- `robots.txt` 对以下默认 user-agent 的 Allow/Disallow：`Googlebot`、`Bingbot`、`OAI-SearchBot`、`PerplexityBot`、`Perplexity-User`、`Claude-SearchBot`、`Claude-User`。具体列表必须配置化，便于随官方文档变化更新。
- 诚实 CiteLoop probe 的 HTTP 访问状态：`2xx`、`3xx`、`403`、`404`、`429`、timeout。
- 诚实 CiteLoop probe 观察到的 CDN/WAF 启发式信号：Cloudflare challenge、captcha、bot block、rate limit、JS challenge。
- meta robots / x-robots-tag：`noindex`、`nosnippet`、`max-snippet`。
- sitemap 是否暴露目标 URL。
- 页面正文是否可在无 JS 或基础 HTML 中抽取；PR1 应复用或抽取现有 `internal/seo.Service` 的 fetch/HTML 检查逻辑，不新写一套不可对齐的解析器。

输出：

- project-level crawler access health。
- per-agent/per-url evidence，包含 `evidence_type`: `robots_static`, `honest_probe`, `manual_confirmation`。
- 可转换为 `technical SEO fix task` 或 `geo_crawler_access_blocked` opportunity；只有 `robots_disallowed` 或人工确认的 WAF block 可以生成 high-confidence blocker，启发式 WAF 信号只能生成 medium/low-confidence review task。

### 7.2 GEO Prompt Sets

Prompt set 由系统生成，用户可编辑、暂停、锁定。

Prompt 类型：

- category recommendation：`best tools for ...`
- problem-solution：`how to ...`
- comparison：`A vs B`
- alternative：`alternatives to ...`
- workflow：`how to automate ...`
- integration：`tools that integrate with ...`
- buyer-intent：`which product should I use for ...`
- definition/entity：`what is ...`

每个 prompt 必须保存：

- `prompt_text`
- `intent_type`
- `target_persona`
- `target_topic`
- `locale`
- `target_engines`
- `priority`
- `status`
- `source`: profile, topic, competitor, manual, search result

竞品和品牌实体输入：

- 初始竞品来自 Product Profile 中的 competitors、Strategist 搜索结果、用户手动添加项。
- 用户必须能编辑、暂停或删除竞品；Observer 只能基于当前 active competitor list 判定 `competitor_mentions` 和 `competitor_citations`。
- 品牌实体应包含 project-owned domains、known product names、company/founder aliases、GitHub org、docs domain、以及 `geo_external_surfaces.owner_type='project'` 的外部 URL。

### 7.3 Answer Engine Observation

Observation run 读取 prompt set，对可用 provider 执行查询并记录结果。

MVP provider 模式：

1. `manual_fixture`：用户或测试把 engine answer/citations 导入，用于验证数据模型和 UI。
2. `search_provider_probe`：通过现有 `SearchProvider` 或可替换 provider 获取公开 SERP snippets、source candidates 和传统搜索 evidence。
3. `perplexity_api`：如项目配置 API key，可作为第一批自动 answer engine。
4. `browser_assisted_manual`：需要登录或界面访问的 engine 只生成人工检查任务，不自动抓用户私有数据。

数据真实性边界：

- PR2/PR3 的真实 answer-engine citation 数据主要来自 `manual_fixture` 和明确可用的 answer provider，例如 `perplexity_api`。
- `search_provider_probe` 不能被当成 ChatGPT、Perplexity、Google AI Mode 或 Claude 的 answer citation；它只能作为传统 SERP / discovery / source candidate evidence。
- 如果 prompt 的 `target_engines` 包含当前无法自动观测的 engine，该 prompt 进入 `manual_required` 或 `unobserved` 状态，不得在 score 分母中直接当作“未被引用”。

预算与降级：

- 每次 observation run 必须有 project-level 和 run-level budget 上限，默认按 prompt priority、engine availability、最近 observation freshness 采样。
- 预算触顶时 run 状态为 `degraded`，系统跳过低优先 prompt 或不可用 engine，并在 `geo_runs.output` 中记录 skipped prompts、skipped engines 和原因。
- 单次 run 不因预算触顶整体失败，除非没有任何 prompt 被成功观测。

每条 observation 记录：

- engine、prompt、run time、locale。
- source type：`answer_engine`, `serp_probe`, `manual_fixture`, `manual_required`。
- answer summary 或 raw response reference。
- cited URLs。
- project-owned citation count。
- project-owned cited surface IDs；项目自有 external surfaces、GitHub/docs/Dev.to/Hashnode 等 `owner_type='project'` URL 被引用时也计入 project citation。
- brand mentioned boolean。
- brand mention position。
- competitor mentions/citations。
- citation rank / prominence estimate。
- evidence snippets。
- confidence 和 error state。

### 7.4 GEO Opportunity Engine

系统将 observations 转为 `seo_opportunities`，类型包括：

- `geo_crawler_access_blocked`
- `geo_not_mentioned_for_priority_prompt`
- `geo_competitor_cited_project_absent`
- `geo_project_mentioned_without_citation`
- `geo_source_gap`
- `geo_evidence_gap`
- `geo_comparison_page_gap`
- `geo_template_asset_gap`
- `geo_external_surface_untracked`

类型定义：

- `geo_source_gap`：answer engine 或 SERP repeatedly 引用第三方来源，但项目缺少可被引用的权威外部/原始来源或 source bundle。
- `geo_evidence_gap`：项目已有相关页面，但页面缺少自包含证据块、定义、数据、对比表、步骤或可抽取引用片段。

幂等规则：

- Analyzer rerun 不得为同一问题重复制造 open opportunity。
- 默认 opportunity key 为 `project_id + type + prompt_id/intent_type + engine + normalized_project_or_competitor_url + normalized_target_topic`；已有 open/accepted/converted opportunity 时追加新 observation evidence 并更新 priority/confidence，不新增重复行。
- 如果原机会已 dismissed，只有新 evidence 显著变化或用户明确允许 reopen，才可创建新机会。

推荐动作包括：

- fix AI crawler access
- create comparison page
- create alternative page
- create source-backed definition section
- create template/checklist asset
- create data-backed mini report
- refresh canonical with evidence block
- add internal links from supporting pages
- distribute canonical variant to external platform
- add external surface URL for monitoring

### 7.5 Citation-Ready Asset Briefs

GEO opportunity 可以生成 asset brief，而不是直接写完整文章。

资产类型：

- comparison page
- alternative page
- use-case page
- integration page
- glossary/definition page
- template/checklist
- benchmark / mini-report
- dataset or stats page
- “how it works” technical explainer

每个 asset brief 必须包含：

- target prompts。
- target engine。
- competitor/source gap。
- required evidence blocks。
- product claims allowed by profile。
- pages that should internally link to it。
- expected citation mechanism：definition、statistic、comparison, process, checklist, original data。
- publication surface：blog, docs, landing, hosted asset, external distribution.

### 7.6 External Surface Inventory

记录并监控第三方页面：

- Dev.to / Hashnode / Medium / LinkedIn article / Reddit / HN。
- Product Hunt、GitHub README、docs、directory/review pages。
- 用户手动添加的 mention/review/partner pages。

每个 surface 记录：

- URL、platform、canonical/source-link status。
- owner：project, user, third_party。
- backlink/canonical evidence。
- last crawled status。
- whether cited in observations。

## 8. 数据模型

新增表建议：

### 8.1 `geo_prompt_sets`

- `id`
- `project_id`
- `name`
- `status`: `draft`, `active`, `paused`, `archived`
- `locale`
- `created_by_run_id`
- `created_at`
- `updated_at`

### 8.2 `geo_competitors`

- `id`
- `project_id`
- `name`
- `domains`
- `aliases`
- `source`: `profile`, `search_result`, `manual`
- `status`: `active`, `paused`, `archived`
- `created_at`
- `updated_at`

### 8.3 `geo_prompts`

- `id`
- `project_id`
- `prompt_set_id`
- `prompt_text`
- `intent_type`
- `target_persona`
- `target_topic`
- `target_engines`
- `priority`
- `source`
- `status`
- `created_at`
- `updated_at`

### 8.4 `geo_runs`

- `id`
- `project_id`
- `agent`: `geo_crawler_audit`, `geo_prompt_builder`, `geo_observer`, `geo_analyzer`, `geo_asset_brief`
- `status`: `ok`, `degraded`, `error`
- `provider`
- `started_at`
- `finished_at`
- `input`
- `output`
- `error`
- `cost_usd`

### 8.5 `ai_crawler_access_snapshots`

- `id`
- `project_id`
- `run_id`
- `page_url`
- `normalized_page_url`
- `target_user_agent`
- `probe_user_agent`
- `evidence_type`: `robots_static`, `honest_probe`, `manual_confirmation`
- `robots_state`: `allowed`, `disallowed`, `unknown`
- `http_status`
- `access_state`: `ok`, `blocked`, `challenge`, `rate_limited`, `timeout`, `error`
- `confidence`: `high`, `medium`, `low`
- `inferred`: boolean
- `meta_robots_state`
- `sitemap_state`
- `body_extractable`
- `raw_details`
- `checked_at`

### 8.6 `geo_observations`

- `id`
- `project_id`
- `run_id`
- `prompt_id`
- `engine`
- `locale`
- `source_type`
- `brand_mentioned`
- `brand_position`
- `project_citation_count`
- `project_citation_rank_best`
- `project_cited_surface_ids`
- `cited_urls`
- `competitor_mentions`
- `competitor_citations`
- `observation_state`: `observed`, `manual_required`, `provider_unavailable`, `budget_skipped`, `error`
- `answer_summary`
- `evidence_snippets`
- `confidence`
- `observed_at`

### 8.7 `geo_visibility_scores`

- `id`
- `project_id`
- `run_id`
- `score`
- `coverage`
- `confidence`
- `breakdown`
- `prompt_count_total`
- `prompt_count_observed`
- `engine_count_observed`
- `computed_at`

### 8.8 `geo_external_surfaces`

- `id`
- `project_id`
- `url`
- `normalized_url`
- `platform`
- `surface_type`
- `owner_type`
- `canonical_target_url`
- `backlink_state`
- `last_http_status`
- `last_cited_at`
- `created_at`
- `updated_at`

### 8.9 `geo_asset_briefs`

- `id`
- `project_id`
- `opportunity_id`
- `asset_type`
- `status`: `draft`, `ready_for_review`, `accepted`, `converted`, `dismissed`
- `target_prompts`
- `required_evidence`
- `recommended_outline`
- `internal_link_plan`
- `publication_surface`
- `created_by_run_id`
- `created_at`
- `updated_at`

复用：

- GEO gaps 进入现有 `seo_opportunities`。
- 已接受机会进入现有 `content_actions`。
- 生成内容继续走 Writer + QA evidence mapping gate。
- 发布和分发继续走 Publisher / SemiManual lane。
- `geo_visibility_scores` 应能回填或关联 `seo_objectives.primary_metric='ai_visibility'` 的 measurement，让 GEO 目标可以进入 Autopilot objective 和 experiment 视角。

## 9. API

新增 route group：`/projects/{projectID}/geo`

- `GET /overview`
- `POST /crawler-audit`
- `GET /crawler-audit/latest`
- `POST /prompt-sets/generate`
- `GET /prompt-sets`
- `PUT /prompt-sets/{promptSetID}`
- `POST /runs/observe`
- `GET /runs`
- `GET /observations`
- `GET /external-surfaces`
- `POST /external-surfaces`
- `POST /asset-briefs/{briefID}/accept`
- `POST /opportunities/analyze`

SEO integration:

- SEO Overview 增加 `geo_visibility` summary。
- SEO Brief 增加 GEO blockers 和 top GEO opportunities。
- Opportunity list 支持 `type=geo_*` filter。

## 10. Scoring

### 10.1 GEO Visibility Score

项目级 0 到 100：

- 20: AI crawler access health。
- 20: priority prompts brand mention rate。
- 25: priority prompts project citation rate。
- 15: citation rank / prominence。
- 10: competitor gap severity inverse。
- 10: external surface coverage。

Score 必须随每次 scoring run 写入 `geo_visibility_scores` 时间序列，而不是只做实时计算。UI 默认展示最新 score、7/28/90 天趋势和分项 breakdown。

Score 还必须同时展示 coverage/confidence：

- `coverage`：observed priority prompts / eligible priority prompts，以及 observed target engines / eligible target engines。
- `confidence`：综合样本量、provider 类型、manual vs automatic 比例、prompt freshness。
- coverage 低于最小阈值时，UI 显示 `insufficient_data`，不渲染看似精确的 0-100 主分数。
- `manual_required`、`provider_unavailable`、`budget_skipped` 的 prompts 不作为“未被引用”直接拉低分数，只降低 coverage/confidence。

### 10.2 Opportunity Priority

`priority_score` 输入：

- prompt priority。
- target persona / buyer intent。
- competitor cited count。
- project absent or project mentioned without citation。
- crawler block severity。
- required effort。
- evidence confidence。

风险等级：

- low：新增 asset brief、补 definition/Q&A、增加内链。
- medium：改写已有 canonical section、comparison page、external distribution。
- high：robots/canonical/noindex/redirect、删除/合并页面、修改 pricing/legal/homepage。

## 11. UI

在现有 SEO 页面下新增 GEO tab，或将页面改名为 “SEO + GEO”。

核心区块：

1. GEO visibility score。
2. AI crawler access matrix：agent x URL sample。
3. Prompt set manager。
4. Observation table：prompt、engine、brand mention、project citations、competitor citations。
5. Top GEO opportunities。
6. Asset briefs。
7. External surfaces。

UI 原则：

- 不显示“保证进入 ChatGPT”这类承诺。
- 对每条结论展示 evidence。
- 对需要人工或授权的 engine 明确显示 `manual_required`。
- 对 crawler audit 区分 `robots_static` high-confidence blocker 与 `honest_probe` inferred warning。
- 对 GEO score 始终展示 coverage/confidence；数据不足时显示 `insufficient_data`。

## 12. Agent 与 Workflow

### 12.1 GEO Crawler Auditor

输入：project domain、sitemap sample、published canonical URLs、target user agents。

输出：`ai_crawler_access_snapshots`、crawler blockers、technical opportunities。

### 12.2 GEO Prompt Builder

输入：product profile、topics、inventory、competitors、search results。

输出：draft prompt set。

### 12.3 GEO Observer

输入：active prompt set、provider config。

输出：`geo_observations`。

调度：默认由现有 scheduler 每周触发，也支持手动运行；当 project safe mode、budget breaker 或 provider quota 触发时，run 降级或跳过，并写入 `geo_runs`。

### 12.4 GEO Analyzer

输入：observations、crawler snapshots、external surfaces、SEO metrics。

输出：`seo_opportunities` 和 `geo_asset_briefs`。

Analyzer 必须对 open opportunities 幂等更新，不能每周为同一 prompt/engine/competitor gap 创建重复任务。

### 12.5 Asset Brief Converter

输入：accepted `geo_asset_brief`。

输出：topic 或 content action，并进入现有 Writer + QA + Review gate。

## 13. 分阶段交付

### 13.1 PR1: AI Crawler Access Audit

价值最高且工程边界最清晰。

交付：

- 新增 crawler access 数据表和 run。
- 静态解析 robots 对 `OAI-SearchBot`、`PerplexityBot`、`Claude-SearchBot`、`Claude-User`、`Googlebot`、`Bingbot` 的规则；HTTP probe 使用 CiteLoop 自己的 User-Agent。
- UI 展示 access matrix。
- 生成 `geo_crawler_access_blocked` opportunity，并区分 high-confidence robots block 与 inferred WAF warning。

### 13.2 PR2: GEO Prompt Sets + Fixture Observation

交付：

- Prompt set 生成、编辑、启用。
- Competitor list 和 project-owned surface/domain mapping。
- `manual_fixture` observation provider。
- Observation table、GEO score v1、coverage/confidence、`geo_visibility_scores` 时间序列。

### 13.3 PR3: GEO Analyzer + Asset Briefs

交付：

- 从 observations 生成 GEO opportunities。
- 生成 citation-ready asset brief。
- Accepted brief 转 topic/content action。

### 13.4 PR4: Provider Integrations + External Surfaces

交付：

- Perplexity API 或其他合法 answer provider adapter。
- External surface inventory。
- 外部分发 URL 的 backlink/citation monitoring。

## 14. 验收标准

1. 对一个只有 domain 的项目，可以运行 crawler audit，并输出每个目标 user-agent 的 allow/block/challenge 状态。
2. 如果 `OAI-SearchBot` 或 `PerplexityBot` 被 robots 明确禁止，系统生成 high-confidence `geo_crawler_access_blocked` opportunity；如果只是 CiteLoop honest probe 遇到 WAF/challenge，系统生成 inferred warning 或人工确认任务，且 evidence 可追溯。
3. 系统能基于产品 profile 生成至少 30 个 GEO prompts，用户可编辑和启用。
4. `manual_fixture` provider 能导入至少 10 条 observation，并计算 brand mention rate、project citation rate、competitor citation count、coverage 和 confidence。
5. 当 active competitor 被引用但项目未出现时，系统生成或更新同一个 `geo_competitor_cited_project_absent` opportunity，不重复污染 queue。
6. Accepted GEO opportunity 可以生成 asset brief，并转为 topic 或 content action。
7. 由 GEO asset brief 生成的内容仍经过现有 QA evidence mapping gate；未被 profile/source 支持的产品声明不可自动通过。
8. SEO Brief 能显示 GEO blockers 和 top GEO opportunities。
9. 所有 GEO run 都记录成本、状态、输入、输出、skipped prompts/engines 和 error，不绕过现有 budget / audit 思路。
10. `geo_visibility_scores` 保留 score 时间序列；当样本量不足或 target engine 无法自动观测时，UI 显示 `insufficient_data` 或低 coverage，而不是误报确定性分数。

## 15. 主要风险

- AI search provider 行为变化快，provider adapter 必须可替换。
- ChatGPT / Google AI Mode 自动化可能受 ToS、登录态和个性化影响；MVP 不依赖私有账号抓取。
- LLM observation 结果可能不稳定；必须用多次运行和 confidence 表示趋势，不把单次回答当确定事实。
- Crawler allow 并不等于会被引用；UI 必须避免过度承诺。
- 生成 comparison / alternative 页面存在品牌和法律风险，需要 review gate。

## 16. 与现有 PRD 的关系

- 继承 `PRD-CiteLoop-SEO-Operations-Loop.md` 的 opportunity/action/brief/measurement 思路。
- 不替代 `PRD-CiteLoop-SEO-Autopilot.md`；GEO Visibility Layer 先提供观测和高价值机会，Autopilot 后续再决定哪些动作可自动执行。
- 补齐 `PRD-CiteLoop-Frontend-Dashboard.md` 中明确未做的 AI 引用、share of voice、闭环分析能力。

做到这里，CiteLoop 的 GEO 不再只是“写带 Q&A 的文章”，而是能回答：AI 搜索能不能读到我、哪些问题里没提到我、竞品为什么被引用、下一步最值得生产哪种可引用资产。
