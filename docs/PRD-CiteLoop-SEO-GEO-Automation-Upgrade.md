# PRD: CiteLoop SEO/GEO 自动化闭环升级

> 日期：2026-06-30
> 阶段：建立在 `PRD-CiteLoop-SEO-Operations-Loop.md`、`PRD-CiteLoop-GEO-Visibility-Layer.md`、`PRD-CiteLoop-SEO-Autopilot.md` 之上。
> 目标：把 CiteLoop 从“能生成和发布内容”升级为“能自动发现 SEO/GEO 机会、执行可控内容动作、发布并验证结果的增长运营系统”。
> 文档类型：5-phase roadmap / epic PRD。它用于战略和范围对齐，不替代 Phase 1 的执行级 PRD；每个 Phase 开工前必须拆出单独 implementation PRD。

## 0. 当前实现基线

本 PRD 基于当前代码库能力和产品页面观察，而不是理想状态。

| 模块 | 当前能力 | 主要缺口 |
|---|---|---|
| Context / Insight | `internal/agents/insight.go` 能 crawl 站点，抽取 Product Profile、Content Inventory、evidence snippets。 | Context 还没有把“SEO/GEO 可引用资产缺口”显式建模，例如 entity profile、source bundle、benchmark data、comparison evidence。 |
| SEO Operations | `internal/seo.Service` 已有 GSC/GA4 ingest、technical check、cold-start opportunities、brief、setup checklist。 | Analyzer 主要覆盖 indexing anomaly 和 cold-start；缺 CTR rewrite、striking distance、content decay、query gap、internal link、schema/cannibalization 等高价值机会。 |
| Opportunity -> Action | `seo_opportunities`、`content_actions`、accept/dismiss/create action API 已有；content action 带 baseline 28 天和 7/14/28 天 measurement window。 | 机会到具体改稿 diff、页面刷新、技术修复、结果归因还不够完整。 |
| Writer / QA | Writer 生成 canonical article 和 syndication variants；QA 会把产品声明映射到 profile/evidence，阻塞 unsupported 或 banned claims。 | Writer 主要产出 article；还缺 comparison、alternative、template、glossary、benchmark/report、docs/integration 等 asset-specific writer contract。 |
| Publish | Scheduler 可推进 generation、review、publish、verification、content action measuring；已有 GitHub/MDX publishing 基础。 | 发布目标覆盖面有限；CMS/publisher capability、rollback、preview、diff 审核还需要增强。 |
| GEO | 已有 AI crawler audit、prompt set、manual/provider observation、visibility score、asset brief 数据模型和 API。 | 多 answer engine 稳定观测、citation rank/sentiment、competitor share of voice、prompt coverage、外部 surface 监控仍需产品化。 |
| Results | 已有 measurement window 和部分状态，但 Results 尚未成为 action-level before/after 归因页。 | 用户难以看到某个 opportunity/action 对 GSC、GA4、AI citation、ChatGPT referral 的实际影响。 |

## 0.1 范围判定与默认决策

本 PRD 的范围是 5 个 Phase 的产品路线图。它不直接作为 Phase 1 工程开工文档。开工前必须为当前 Phase 另写执行级 PRD，补齐 UI 结构、状态机、API contract、迁移计划、测试计划和 rollout 方案。

默认决策：

1. **P0 不包含完整 Results before/after**：P0 只交付 measurement contract、baseline capture、`insufficient_data` reason 和 action traceability。完整 action-level Results attribution 放在 Phase 4。
2. **GEO Monitoring 1.0 必须有一个自动 provider**：manual fixture 只能作为兜底和测试工具，不能作为 launch 主干。Phase 2 launch gate 是至少一个可自动运行的 answer/citation provider 可用。
3. **Results 独立成页**：Results 是留存和信任证明，不并入 Visibility 的次级 tab。
4. **Publisher 第一阶段押 GitHub/Next.js diff + preview**：CMS 扩展后置；Phase 4/5 之前先把 GitHub publisher 的 diff、preview、verification、rollback path 打磨成熟。
5. **Learning loop 第一版只调整 priority score**：不直接影响 strategist topic generation，避免早期错误结果污染内容规划。

## 1. 背景

CiteLoop 当前的差异化不是“又一个 SEO 数据后台”，而是能把产品事实、搜索表现、AI 可见性和内容生产串成一个运营闭环：

Context -> Analysis/Visibility -> Content Plan -> Review -> Publish -> Results

与主流产品相比：

- Semrush / Ahrefs 的强项是大型 SEO 数据、SERP、关键词、竞争情报和 AI visibility 报告。
- Surfer / Frase / MarketMuse 的强项是内容编辑和页面优化。
- Profound / Peec / Otterly / Scrunch 的强项是 AI answer visibility、brand mention、citation、competitor monitoring。
- CiteLoop 的机会是把这些信号转成自动化运营动作，并推进到 review、publish、measurement。

本 PRD 的核心产品赌注：

> CiteLoop should be the operating system that turns SEO/GEO visibility gaps into approved, published, measured content changes.

## 2. 第一性原理

SEO/GEO 自动化产品必须解决六个问题：

1. **知道用户是谁**：产品定位、ICP、features、differentiators、banned claims、evidence 必须先被系统理解。
2. **知道机会在哪里**：机会必须来自 GSC、GA4、public crawl、SERP/provider、answer engine observation、competitor citation，而不是凭空生成主题。
3. **知道该做什么**：每个机会必须对应明确动作：写新页面、刷新旧页面、改 title/meta、加内链、补 schema、创建 comparison/template/source asset、修 crawl blocker。
4. **能安全执行**：低风险动作可自动，中高风险动作进入 review，高风险动作只生成 plan。
5. **能验证结果**：每个 action 必须有 baseline、执行记录、publish verification、measurement window、outcome label。
6. **能越跑越准**：Results 需要回流到 future prioritization，而不是只展示历史报表。

## 3. 产品目标

### 3.1 P0 目标

1. 补强 SEO Analyzer，让 Analysis 页面成为真正的机会决策队列。
2. 将现有 cold-start opportunity 升级为 context-backed growth plan。
3. 把 opportunity -> content action -> topic -> draft -> review -> publish -> measuring 的链路做成可解释的用户体验。
4. 落地最小 measurement contract：创建 action 时保存 baseline window、measurement checkpoints、source opportunity trace，并能显示 `too_early` / `insufficient_data` reason。完整 before/after Results 留到 Phase 4。

### 3.2 P1 目标

1. 建立 GEO Monitoring 1.0，覆盖 prompt set、observation、visibility score、competitor citation gap。
2. 支持 GEO asset brief 到 topic/draft/review/publish。
3. 增加 comparison、alternative、template/checklist、glossary、benchmark/report、integration/docs 等非普通 blog asset。
4. 把 ChatGPT/Perplexity/AI search referral 和 project-owned citation 纳入 Results。

### 3.3 P2 目标

1. Guarded Autopilot 执行低风险 SEO/GEO 动作。
2. 引入 external surface monitoring，跟踪 Dev.to、Hashnode、Reddit、GitHub、docs、directory、review site 是否被 AI 引用。
3. 形成 strategy learning loop：结果好的 action type、topic、asset type、prompt intent 提升未来优先级。

## 4. 非目标

- 不承诺排名、流量、conversion 或 AI citation 一定增长。
- 不做 PBN、伪外链、自动 spam、虚假评论、黑帽或灰帽 SEO。
- 不绕过 Google Search Console、GA4、CMS、publisher、answer provider 的授权边界。
- 不伪装第三方 crawler User-Agent。
- 不把 `llms.txt`、特殊 AI schema 或玄学 prompt 文案作为 GEO 核心策略。
- 不自动执行删除、noindex、redirect、canonical、robots、pricing/homepage rewrite 等高风险动作。
- 不在第一版复制 Semrush/Ahrefs 的关键词库、外链库、SERP 历史数据库。

## 5. 用户与场景

### 5.1 SaaS Founder

用户只输入产品 domain，CiteLoop 自动理解产品、发现可抓取性和内容机会，给出第一批高意图 use-case、comparison、alternative、definition、template 机会。

### 5.2 Growth Operator

用户每周打开 Analysis/Visibility，只看到最值得处理的 SEO/GEO opportunity。接受机会后，CiteLoop 自动生成 topic、draft、QA、review，并在发布后进入 measurement。

### 5.3 Content Operator

用户主要处理 Review 队列。系统提前自动修复 QA/SEO/GEO 可修复问题，只把需要事实判断、定位判断、风险确认的内容留给人。

### 5.4 Product Marketing Lead

用户关心 AI answer visibility：哪些 prompt 提到了我们，哪些 prompt 引用了竞品，哪些答案缺少我们官网作为 source。CiteLoop 自动生成对应 asset brief。

## 6. 产品原则

1. **Evidence first**：所有机会必须展示证据来源。
2. **Action over dashboard**：分析结果默认可以进入 action，而不只是查看。
3. **Human where judgment matters**：把人类留给定位、事实、安全、品牌判断。
4. **No fake metrics**：没有 GSC/GA4/observation 数据时，明确显示 unavailable 或 insufficient data。
5. **No fake forecasts**：`priority_score`、`confidence`、`expected_impact` 必须来自可解释规则、历史数据或明确标注的 heuristic，不把 LLM 猜测包装成精确预测。
6. **One loop**：SEO 和 GEO 共享 opportunity/action/review/publish/results，不拆成两个互不相通的产品。
7. **Risk-aware automation**：自动化等级、权限、预算、风险和 kill switch 必须可见。
8. **Correlation-aware results**：Results 默认表达“与动作相关的变化”，不宣称严格因果，除非存在对照或实验设计。

## 7. 功能范围

### 7.1 SEO Analyzer 2.0

#### 7.1.1 新增 opportunity 类型

| 类型 | 数据源 | 推荐动作 |
|---|---|---|
| `striking_distance` | GSC query/page position 4-20 + impressions | refresh existing page / add evidence block / add internal links |
| `ctr_rewrite` | GSC high impressions + low CTR | rewrite SEO title/meta/H1 |
| `content_decay` | GSC clicks/impressions/position 下滑 | refresh canonical / update examples / improve freshness |
| `query_gap` | GSC query 与页面内容覆盖不匹配 | expand section / create supporting page |
| `internal_link_gap` | crawl link graph + target page priority | add internal links |
| `schema_gap` | technical check + page type | add structured data task |
| `cannibalization` | 多页面竞争同一 query | merge/prune recommendation / canonical clarification |
| `thin_evidence_page` | Context inventory + QA evidence | add source-backed evidence block |
| `technical_visibility_issue` | crawl/technical checks | fix title/meta/H1/canonical/robots/sitemap |

#### 7.1.2 Opportunity 字段要求

每条 opportunity 必须包含：

- `type`
- `status`
- `priority_score`
- `confidence`
- `scoring_method`
- `scoring_version`
- `risk_level`
- `query`
- `page_url`
- `evidence`
- `recommended_action`
- `expected_impact`
- `expected_impact_range`
- `effort`
- `source_run_id`
- `data_source_notes`

`priority_score` 和 `confidence` 不得由 LLM 直接生成。默认规则：

- GSC-driven opportunity 使用 impressions、clicks、CTR、position、trend delta、page type、recency、data completeness 计算。
- Public-only/cold-start opportunity 使用 context evidence count、source page strength、existing content gap、publisher readiness 计算。
- GEO opportunity 使用 prompt priority、engine coverage、competitor citation gap、project citation absence、observation confidence 计算。
- `expected_impact` 必须是文字假设；`expected_impact_range` 只能展示粗粒度区间，如 `low`、`medium`、`high` 或历史同类 action 的分位区间，不能展示精确排名/流量承诺。

#### 7.1.3 冷启动模式

没有 GSC/GA4 时，Analysis 只能生成：

- context-backed use-case pages
- comparison / alternative pages
- evidence-led source pages
- technical crawl issues
- public content inventory gaps
- initial GEO prompt candidates

不得展示 CTR、position、conversion 作为事实。

#### 7.1.4 验收标准

1. GSC connected 项目至少能生成 5 类非冷启动机会。
2. Public-only 项目明确展示 cold-start mode。
3. 同一个 query/page/type 不重复创建 open opportunity。
4. 每条 opportunity 都能转成 content action。
5. Brief 能展示 top 5-10 个本周最值得处理的动作。
6. Internal dogfood 阶段，surfaced opportunities 的人工 accept 或 `useful` 标记比例必须达到 40% 以上，否则不进入 autopilot。
7. Dismiss reason 中 `not relevant` / `bad evidence` 合计超过 30% 时，该 opportunity type 必须降权或暂停。

### 7.2 GEO Monitoring 1.0

#### 7.2.1 Prompt Set

系统基于 Product Profile、topics、competitors、existing content 自动生成 30-80 个 prompts。

Prompt intent 包括：

- `category_recommendation`
- `problem_solution`
- `comparison`
- `alternative`
- `workflow`
- `integration`
- `buyer_intent`
- `definition_entity`

Prompt 必须可编辑、暂停、锁定，并记录 `source`、`locale`、`target_engines`、`priority`。

#### 7.2.2 Observation Providers

MVP provider 分层：

1. `automatic_answer_provider`：Phase 2 launch 必须接入至少一个可自动运行的 answer/citation provider。默认优先级是 Perplexity API；如果不可用，则使用一个已签约或可合法调用的第三方 AI visibility provider。没有自动 provider 时，Phase 2 不得宣称 GEO Monitoring 1.0 已上线。
2. `manual_fixture`：人工导入 answer/citation，用于 QA、demo、provider outage fallback 和验证数据模型；不得作为 production 主干。
3. `search_provider_probe`：传统 SERP/source candidate，不冒充 answer engine citation，也不得计入 answer-engine citation rate。
4. `browser_assisted_manual`：需要登录态或不可自动抓取的 engine 只生成人工检查任务。
5. `future_provider`：ChatGPT Search、Gemini/AI Mode、Copilot 等，必须遵守平台授权和 ToS。

Phase 2 launch gate：

- 至少 1 个 automatic answer provider 可在后台定时运行。
- 每个 active 项目默认每周至少观测 top 10 prompts。
- Observation run 必须记录 provider、cost、coverage、skipped reason 和 engine availability。
- Manual-only 项目只能标记为 `geo_manual_mode`，不能计入 GEO automation activation。

#### 7.2.3 Observation 记录

每条 observation 必须保存：

- engine
- prompt
- locale
- source_type
- observed_at
- answer_summary
- cited_urls
- project_citation_count
- project_cited_surface_ids
- brand_mentioned
- brand_position
- competitor_mentions
- competitor_citations
- project_citation_rank_best
- sentiment
- confidence
- error_state

#### 7.2.4 GEO Score

GEO score breakdown 至少包含：

- prompt coverage
- engine coverage
- AI crawler access health
- brand mention rate
- project citation rate
- citation rank score
- competitor gap rate
- external surface coverage
- confidence / insufficient data

#### 7.2.5 GEO Opportunity 类型

- `geo_crawler_access_blocked`
- `geo_not_mentioned_for_priority_prompt`
- `geo_competitor_cited_project_absent`
- `geo_project_mentioned_without_citation`
- `geo_source_gap`
- `geo_evidence_gap`
- `geo_comparison_page_gap`
- `geo_template_asset_gap`
- `geo_external_surface_untracked`

#### 7.2.6 验收标准

1. 每个项目可生成 prompt set。
2. 每次 run 有 budget、coverage、skipped prompts、skipped engines。
3. 无可用 provider 时不把 prompt 计为失败；显示 `manual_required` 或 `unobserved`。
4. Competitor cited 且 project absent 时能生成 opportunity。
5. GEO opportunity 能进入同一 Analysis/Visibility 队列。

### 7.3 GEO Asset Briefs

#### 7.3.1 Asset Types

CiteLoop 需要支持以下 asset types：

- `comparison_page`
- `alternative_page`
- `use_case_page`
- `template_checklist`
- `benchmark_report`
- `glossary_definition`
- `integration_docs_page`
- `source_backed_evidence_page`
- `faq_answer_block`

#### 7.3.2 Brief 内容

每个 asset brief 必须包含：

- target prompts
- target audience
- target topic
- asset type
- required evidence
- competitor/citation evidence
- recommended outline
- internal link plan
- publication surface
- risk level
- QA requirements

#### 7.3.3 Writer Contract

Writer 不应把所有 brief 都写成普通 blog article。

不同 asset type 应有不同结构：

- comparison page：decision criteria、who each option is for、supported differentiators、limitations。
- alternative page：migration reason、alternative evaluation、use cases。
- glossary definition：short definition、examples、related terms、source-backed product context。
- template/checklist：actionable steps、download/use section、FAQ。
- benchmark/report：methodology、data caveats、findings、charts/tables placeholders。
- integration/docs：setup steps、API/workflow details、troubleshooting、related links。

#### 7.3.4 验收标准

1. GEO gap 可自动生成 asset brief。
2. 用户接受 asset brief 后创建 topic。
3. Topic 保留 asset type，不丢失到普通 article。
4. QA 继续阻塞 unsupported product claim。
5. Publish/Review UI 能展示 asset type 和 source evidence。

### 7.4 Results Attribution

#### 7.4.1 Measurement Lifecycle

`content_actions` 的 execution state 与 outcome label 分离。Execution state 描述动作推进到哪里，outcome label 描述观察窗口内看到的结果。

Execution state：

- `ready_for_review`
- `approved`
- `planned`
- `drafting`
- `draft_ready`
- `approved_for_publish`
- `published`
- `verified`
- `measuring`
- `measured`
- `insufficient_data`
- `failed`

允许的主要转移：

| From | To | Trigger |
|---|---|---|
| `ready_for_review` | `approved` | user accepts action |
| `approved` | `planned` | workflow creates topic or execution plan |
| `planned` | `drafting` | writer starts |
| `drafting` | `draft_ready` | draft created and QA recorded |
| `draft_ready` | `approved_for_publish` | review approved |
| `approved_for_publish` | `published` | publisher writes content |
| `published` | `verified` | canonical URL or publisher state verified |
| `verified` | `measuring` | measurement schedule created |
| `measuring` | `measured` | final checkpoint computed |
| any non-terminal state | `failed` | unrecoverable execution error |
| `measuring` | `insufficient_data` | required metric sources unavailable after checkpoint window |

No transition may clear QA blocking or publish verification failure by directly changing state; it must rerun the corresponding verifier.

#### 7.4.2 Measurement Windows

默认：

- baseline：执行前 28 天
- checkpoints：7、14、28 天
- long tail：90 天，可选

#### 7.4.3 Metrics

SEO metrics：

- clicks
- impressions
- CTR
- average position
- query count
- indexed / crawl status

GA4 metrics：

- sessions
- engaged sessions
- engagement rate
- key events / conversions

GEO metrics：

- brand mentioned
- project cited
- citation rank
- competitor cited
- prompt coverage
- answer sentiment
- ChatGPT/Perplexity referral if available

Execution metrics：

- time to draft
- time to review
- time to publish
- publish verification latency
- QA repair attempts
- human touch count

#### 7.4.4 Outcome Labels

每个 action 必须生成 outcome：

- `positive`
- `neutral`
- `negative`
- `mixed`
- `confounded`
- `insufficient_data`
- `too_early`
- `blocked`

Outcome 必须附带 reason，例如：

- "Clicks +24% vs baseline, impressions stable; correlated with the action, not proven causal."
- "No GSC data available because search_read is not connected."
- "Published less than 7 days ago."
- "AI observation provider unavailable for selected engines."
- "Confounded by overlapping site-wide update or known search volatility window."

Attribution rules：

- Default language is "correlated with this action", not "caused by this action"。
- If multiple CiteLoop actions changed the same page/query/prompt inside the measurement window, outcome must be `confounded` or `mixed` unless the measurement layer can isolate one action.
- If Google algorithm volatility, site migration, publisher outage, noindex/canonical incident, or large manual edits overlap the window, outcome must include a confounder note.
- High-traffic pages should prefer holdout/comparison logic when possible; otherwise do not auto-label as `positive` or `negative` with high confidence.

#### 7.4.5 验收标准

1. Results 页面能按 action 展示 before/after。
2. 用户能看到每个 published article 来自哪个 opportunity。
3. 没有数据时显示原因，不显示虚假的 0。
4. Outcome 能回流到 future priority score。
5. Results 文案不宣称因果，除非 action 有对照、holdout 或明确实验设计。

### 7.5 Analysis / Visibility UX

Analysis/Visibility 页面定位：**Review opportunities, not inspect reports**。

#### 7.5.1 页面结构

1. Capability status strip：
   - public crawl
   - GSC
   - GA4
   - publisher
   - GEO observation
   - autopilot level
2. Decision queue：
   - open opportunities
   - grouped by SEO / GEO / technical / content refresh
3. Evidence drawer：
   - GSC rows
   - crawled page facts
   - answer observations
   - competitor citations
   - source snippets
4. Action controls：
   - accept
   - dismiss
   - create action
   - view evidence
5. Loop status：
   - opportunities accepted
   - actions planned
   - drafts generated
   - published
   - measuring

#### 7.5.2 Copy Requirements

用户必须能看懂：

- Why now?
- What will CiteLoop do?
- What evidence supports this?
- What is the risk?
- What needs human review?
- What data is missing?

避免只展示内部状态，例如 `seo_opportunities.status = open`。UI 应转译成用户语言。

### 7.6 Guarded Autopilot Upgrade

#### 7.6.1 Autopilot Modes

- Level 0：manual reports only
- Level 1：assistive draft
- Level 2：guarded execution for low-risk actions
- Level 3：portfolio autopilot
- Level 4：expanded autopilot, not first delivery

#### 7.6.2 Low-risk Auto Actions

Level 2 可自动执行：

- metadata rewrite for low-traffic blog page
- add internal links from approved source pages
- refresh short paragraph with existing evidence
- create supporting article draft
- submit sitemap where supported
- rerun GEO observation

仍需 review：

- product claim change
- comparison against named competitor
- major rewrite
- high-traffic page
- homepage/pricing/docs/legal
- canonical/robots/noindex/redirect/merge/delete

#### 7.6.3 Autopilot Readiness

Level 2 前置：

- `search_read` connected or explicit public-only limitation accepted
- `publisher_write` connected
- `notification_write` connected
- `autopilot_policy_confirmed`
- dry run passed
- safe mode visible
- monthly budget configured

### 7.7 Publishing / CMS Capability Workstream

Publishing 是 Phase 4 Results 和 Phase 5 Autopilot 的关键路径，不能只作为 Publish 页面里的附属能力处理。第一阶段默认不扩展多个 CMS，而是把 GitHub/Next.js publisher 打磨成可信的 capability-driven adapter。

#### 7.7.1 第一版范围

- GitHub/Next.js per-project publisher connection。
- Generated diff preview：用户能看到将新增或修改的文件、frontmatter、canonical、metadata。
- Preview URL support：如 publisher/deployment 支持，Review/Publish 展示 preview link。
- Publish verification：确认文件存在、canonical URL 可访问、metadata 写入成功。
- Rollback path：优先 Git revert / PR revert；没有自动 rollback 时，必须生成人工 rollback instruction。
- Capability schema：
  - `create_article`
  - `update_article`
  - `metadata_update`
  - `canonical`
  - `preview`
  - `rollback`
  - `draft_mode`
  - `publish_mode`

#### 7.7.2 后置范围

- WordPress / Webflow / Contentful / Sanity / Framer 等 CMS。
- Multi-publisher routing。
- Media upload。
- Native CMS scheduling。

#### 7.7.3 验收标准

1. Phase 4 之前，published action 必须能从 content action 追溯到 publisher attempt、diff summary、published URL、verification result。
2. Phase 5 之前，所有 Level 2 auto actions 必须有 rollback path 或明确标注 `manual_rollback_required`。
3. Publisher capability 不支持的 action 不得进入 auto execution，只能进入 draft/review。

## 8. 数据模型建议

### 8.1 `seo_opportunities`

增强字段：

- `source_type`
- `source_run_id`
- `dedupe_key`
- `data_source_notes`
- `why_now`
- `missing_data_reasons`
- `risk_reasons`
- `estimated_impact_range`
- `scoring_method`
- `scoring_version`

### 8.2 `content_actions`

增强字段：

- `source_opportunity_type`
- `execution_plan`
- `diff_summary`
- `risk_reasons`
- `baseline_metrics`
- `checkpoint_metrics`
- `outcome_label`
- `outcome_reason`
- `measured_at`

### 8.3 `geo_observations`

增强字段：

- `sentiment`
- `answer_position`
- `citation_rank`
- `raw_response_ref`
- `provider_cost_usd`
- `manual_required_reason`

### 8.4 `geo_asset_briefs`

增强字段：

- `asset_type`
- `writer_contract_version`
- `target_surface`
- `required_evidence`
- `competitor_evidence`
- `qa_requirements`
- `accepted_topic_id`

### 8.5 `action_measurements`

新增表，建议字段：

- `id`
- `project_id`
- `content_action_id`
- `article_id`
- `checkpoint_day`
- `window_start`
- `window_end`
- `seo_metrics`
- `ga4_metrics`
- `geo_metrics`
- `execution_metrics`
- `outcome_label`
- `outcome_reason`
- `attribution_confidence`
- `confounders`
- `computed_at`

## 9. API / Backend 范围

### 9.1 SEO

- `POST /projects/{id}/seo/analyze`
- `GET /projects/{id}/seo/opportunities`
- `POST /projects/{id}/seo/opportunities/{opportunityID}/accept`
- `POST /projects/{id}/seo/opportunities/{opportunityID}/dismiss`
- `POST /projects/{id}/seo/opportunities/{opportunityID}/actions`
- `GET /projects/{id}/seo/actions/{actionID}/measurements`

### 9.2 GEO

- `POST /projects/{id}/geo/prompt-sets/generate`
- `GET /projects/{id}/geo/prompt-sets`
- `POST /projects/{id}/geo/observe`
- `GET /projects/{id}/geo/observations`
- `POST /projects/{id}/geo/analyze`
- `GET /projects/{id}/geo/asset-briefs`
- `POST /projects/{id}/geo/asset-briefs/{briefID}/accept`

### 9.3 Results

- `GET /projects/{id}/results/actions`
- `GET /projects/{id}/results/actions/{actionID}`
- `POST /projects/{id}/results/recompute`

## 10. Dependencies & Assumptions

### 10.1 External Dependencies

| Dependency | Required for | Assumption | Failure mode |
|---|---|---|---|
| Google Search Console OAuth / service account access | GSC-driven opportunities, Results SEO metrics | User can authorize property or CiteLoop can manage a hosted property | fall back to public-only mode |
| GA4 read access | engagement and conversion prioritization | Optional for Level 1/2, strongly recommended | omit conversion weighting and explain missing signal |
| Publisher write access | publish, verification, autopilot | First supported path is GitHub/Next.js connection | draft/review only |
| Notification channel | guarded autopilot, failures, approval requests | Slack/Discord/email verified before Level 2 | Level 2 disabled |
| Automatic GEO provider | GEO Monitoring 1.0 launch | At least one provider can legally return answer/citation observations | manual mode only; no automation claim |
| Secret store | OAuth tokens, publisher credentials, provider keys | Raw secrets never returned to frontend | connection blocked |
| Deployment preview or published URL verification | Review/Publish trust | GitHub/Next.js path can be verified by URL or repo file state | publish stays pending verification |

### 10.2 Product Assumptions

- GSC data may lag and may hide low-volume query rows; page-level totals and query-level rows must be treated differently.
- SEO outcome windows are noisy; before/after is directional evidence, not proof of causality.
- GEO provider availability and cost will vary; product UX must expose unavailable/manual states.
- Early customers value fewer, higher-confidence actions over large opportunity volume.
- Internal dogfood must establish opportunity precision and variable cost baseline before paid launch.

## 11. Cost Model & Packaging Assumptions

Cost is a product constraint, not only an infrastructure detail. GEO observation, LLM drafting/QA, crawling, publishing verification, and notifications all consume variable cost.

### 11.1 Unit Cost Categories

| Cost bucket | Driver | Required tracking |
|---|---|---|
| Crawl / technical checks | URLs checked per project | run count, URL count, duration |
| GSC/GA4 sync | rows and API calls | provider, date range, rows stored |
| GEO observation | prompts x engines x provider cost | provider cost, prompt count, engine count, skipped reason |
| LLM writing / QA / repair | tokens and model tier | agent run cost, repair attempts |
| Publish verification | deploy hook, URL checks, repo/API calls | attempts, failures, retry count |

### 11.2 Launch Budget Guards

These are launch hypotheses, not pricing commitments. They must be validated during dogfood:

- Internal dogfood target: variable cost per active project per month should stay under `$20` unless explicitly overridden.
- Phase 2 GEO default budget: observe top 10 prompts on 1 automatic provider weekly, with run-level budget cap recorded in `geo_runs`.
- Phase 3 asset generation: cap draft/QA/repair attempts per accepted brief.
- Phase 5 autopilot: project-level monthly budget must be configured before Level 2 is enabled.

### 11.3 Packaging Hypothesis

Paid packaging should be based on observable value and cost drivers:

- Starter: public crawl, cold-start opportunities, manual GEO mode, limited drafts.
- Growth: GSC/GA4, automated SEO opportunities, GitHub/Next.js publisher, weekly GEO provider run.
- Autopilot: guarded execution, notifications, higher prompt/action limits, measurement history.

## 12. Privacy, Permissions, and Data Retention

### 12.1 Roles

Minimum RBAC for this roadmap:

- `owner`: manage project, billing, integrations, delete data.
- `admin`: manage integrations, policy, publisher, prompts.
- `editor`: review/edit/approve drafts and opportunities.
- `viewer`: view context, opportunities, results, and runs.

Autopilot policy changes, publisher credential changes, and project deletion require `owner` or `admin`.

### 12.2 Data Handling

- Store only authorized GSC/GA4 metrics and public crawl/answer observations.
- Do not collect private ChatGPT/Google account data without explicit provider authorization.
- Raw answer responses should be referenced by `raw_response_ref`; default raw retention is 90 days, while aggregate metrics and extracted citations may be retained with the project.
- User deletion of a project must delete project-owned prompts, observations, raw refs where stored by CiteLoop, credentials, and generated content metadata.
- Competitor data must come from public pages, user-provided competitor lists, or provider-returned public answer/search evidence.

### 12.3 Secret Handling

- OAuth tokens, publisher tokens, deploy hooks, and provider keys must live in the secret store.
- API responses must return connection health and capability metadata, not raw credentials.
- Audit logs should record credential use events without exposing credential values.

## 13. 阶段计划

### Foundational Workstream：Publishing Capability

该 workstream 与 Phase 1-4 并行推进，但它是 Phase 4 Results 和 Phase 5 Autopilot 的前置条件。

范围：

- GitHub/Next.js per-project publisher connection。
- Diff preview、preview URL、publish verification。
- Rollback path 或 `manual_rollback_required` 标记。
- Publisher capability schema。

完成标准：

- 每个 published action 能追溯到 publisher attempt、diff summary、published URL、verification result。
- Capability 不支持的 action 不进入 auto execution。

### Phase 1：SEO Analyzer 2.0

范围：

- GSC-driven opportunity engine。
- Deduping 和 confidence scoring。
- Analysis 页面机会队列优化。
- Cold-start mode 明确化。

完成标准：

- GSC connected 项目能看到非冷启动机会。
- Public-only 项目不会显示私有搜索指标。
- 接受 opportunity 后能自动进入 content action。
- P0 measurement contract 可记录 baseline window、measurement checkpoints、source opportunity trace 和 insufficient data reason。

### Phase 2：GEO Monitoring 1.0

范围：

- Prompt set generation。
- At least one automatic answer/citation provider。
- Manual fixture as fallback/testing, not production主干。
- GEO visibility score。
- Competitor citation gap opportunity。

完成标准：

- 至少一个项目能完成 prompt -> observation -> score -> opportunity。
- Provider 不可用时状态可解释。
- Manual-only 项目不能计入 GEO automation activation。

### Phase 3：GEO Asset Loop

范围：

- Asset brief -> topic。
- Asset-specific writer contract。
- Review 页面展示 asset type 和 evidence。
- Publish 后进入 measurement。

完成标准：

- `geo_competitor_cited_project_absent` 可生成 comparison page brief。
- 用户接受后能生成 draft。

### Phase 4：Results Attribution

范围：

- Action measurement table。
- Results 页面 action-level before/after。
- Outcome label、reason 和 confounder notes。
- Measurement 回流 priority。

完成标准：

- Published action 在 7/14/28 天能产生 checkpoint。
- 无数据时显示 insufficient data reason。

### Phase 5：Guarded Autopilot

范围：

- Autopilot readiness UI。
- Low-risk auto action execution。
- Policy/risk classifier。
- Safe mode / kill switch / notifications。

完成标准：

- Level 2 只自动执行 policy 允许的低风险动作。
- 所有自动动作有 audit、diff、rollback 或 recovery plan。

## 14. 成功指标

### 14.1 North Star

**Weekly published-and-measured actions per active connected project**

定义：每个 active connected project 每周完成的、已发布且进入 measurement lifecycle 的 content actions 数量。`measured`、`measuring`、`insufficient_data` 均可计入，因为它们证明闭环已推进到结果层；单纯 draft、review、published 但未进入 measurement 不计入。

| Metric | Current baseline | Phase target | Basis |
|---|---:|---:|---|
| Weekly published-and-measured actions / active connected project | 0 reliable action-level measured actions; current code has measurement window metadata but no Results attribution loop | Phase 4: >= 1 per week for dogfood connected projects | 对增长闭环来说，发布且进入结果观察比“生成内容数”更接近用户价值和留存 |

### 14.2 Input Metrics

| Input metric | Current baseline | Phase target | Basis |
|---|---:|---:|---|
| Opportunity precision | Not instrumented; must start with dogfood accept/useful tracking | Phase 1: >= 40% surfaced opportunities accepted or marked useful; Phase 4: >= 55% | 机会队列质量比机会数量更重要 |
| Opportunity -> draft throughput | Partially supported by workflow; not measured as funnel metric | Phase 1: >= 60% accepted opportunities create content action; Phase 3: >= 60% accepted asset briefs create draft | 证明 automation loop 不停在 Analysis |
| Publish -> measuring reliability | Existing scheduler can mark measuring, but Results loop incomplete | Phase 4: >= 90% verified published actions enter `measuring` or `insufficient_data` | 证明 Results 不丢链 |
| GEO automation coverage | GEO data model exists; automatic provider launch gate not yet met | Phase 2: top 10 prompts weekly on >= 1 automatic provider for dogfood connected projects | 区分 automation product 和 manual-assisted workflow |
| Variable cost guardrail | Not fully instrumented by project/month | Dogfood: <= `$20` variable cost per active project/month unless explicitly overridden | 防止 GEO/LLM 成本破坏毛利 |

### 14.3 Trust and Quality Proxies

- `dismiss_reason = not_relevant` 或 `bad_evidence` 的比例低于 30%。
- Opportunity detail 中 evidence drawer 打开率和 accept/useful rate 正相关；如果打开 evidence 后 dismiss 激增，说明证据质量有问题。
- QA auto-repair 后仍需 human decision 的比例下降。
- Publish verification failure rate 下降。
- 用户对 opportunity 标记 `useful` 的比例作为“能解释 why now”的代理指标。

## 15. 风险与缓解

| 风险 | 缓解 |
|---|---|
| GSC/GA4 数据不足导致 opportunity 质量低 | 明确 cold-start mode；不展示 fake metrics；用 public crawl/context opportunities 降级。 |
| GEO provider 不稳定或 ToS 限制 | Phase 2 launch gate 要求至少一个 automatic provider；manual fixture 只做兜底；manual-only 项目不得计入 GEO automation activation。 |
| 自动发布误改高风险页面 | Deterministic risk classifier；policy；review gate；kill switch；rollback/audit。 |
| AI 内容出现 unsupported claim | 保留 evidence-aware QA；banned claims deterministic gate；writer repair loop。 |
| 用户看不懂数据 | Analysis 使用 why now / evidence / action / risk 文案，不暴露内部字段。 |
| 结果归因过度承诺 | Results 默认表达 correlated change；新增 `confounded` / `mixed` outcome；高流量页优先 holdout/comparison。 |
| Publisher 能力卡住 Results/Autopilot | GitHub/Next.js diff + preview + verification + rollback path 作为 foundational workstream；CMS 扩展后置。 |
| 成本失控 | Run-level 和 project-level budget；provider cost tracking；dogfood 变量成本上限。 |

## 16. Remaining Open Questions

1. Asset-specific writer contract 是先做 prompt 层约束，还是新增 typed renderer/schema？
2. Phase 2 automatic provider 的具体供应商选择：默认优先 Perplexity API；如果不可用，选择合法第三方 AI visibility provider。范围 gate 已确定，待 vendor evaluation。
3. Paid packaging 的具体价格和额度，需要 dogfood 的 variable cost baseline 后再定。

## 17. 参考

- Semrush AI Visibility Toolkit: https://www.semrush.com/kb/1493-ai-visibility-toolkit
- Ahrefs Brand Radar: https://help.ahrefs.com/en/articles/11064852-what-is-brand-radar-and-how-to-use-it
- Surfer Content Editor: https://surferseo.com/content-editor/
- Profound: https://www.tryprofound.com/
- Peec AI: https://peec.ai/
- Otterly AI: https://otterly.ai/
- Google AI optimization guidance: https://developers.google.com/search/docs/fundamentals/ai-optimization-guide
- OpenAI Publisher FAQ: https://help.openai.com/en/articles/12627856-publishers-and-developers-faq
- Perplexity crawler docs: https://docs.perplexity.ai/docs/resources/perplexity-crawlers
