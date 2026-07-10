# PRD：CiteLoop Doctor 与 Opportunities 双线优化

> 日期：2026-07-10
> 状态：Draft for implementation planning
> Owner：Product
> 范围：Doctor、Opportunities、Signal Scan、AI Discovery、Site Fixes、Content Growth Loop、共享证据层、调度与迁移
> Supersedes：
> - `docs/PRD-CiteLoop-SEO-Doctor.md` 中“Doctor 只读、不得拥有修复闭环”的产品边界
> - `docs/PRD-CiteLoop-Opportunity-Review-and-Work-Queues.md` 中由 Opportunities 分发 Site Fixes 的边界
> - `docs/plans/2026-07-06-opportunity-finding-settings.md` 中把 Signal Scan 与 AI Discovery 暴露为用户需要配置和理解的独立产品模式
> Related docs：
> - `docs/PRD-CiteLoop-SEO-Operations-Loop.md`
> - `docs/PRD-CiteLoop-GEO-Visibility-Layer.md`
> - `docs/PRD-CiteLoop-Visibility-Analysis-to-Content-Loop.md`
> - `docs/PRD-CiteLoop-Event-Triggered-Growth-Loop.md`

## 0. Executive Summary

CiteLoop 当前使用 Doctor、Signal Scan 和 AI Discovery 三条路径发现 SEO/GEO 问题与机会。三条路径可以读取相同的 crawl、GSC、GA4、Context 和 AI-answer evidence，但当前会重复分类同一页面问题、产生相似 Opportunity、生成相似 Site Fix 或 Content Action，并通过多套 scheduler 重复访问相同页面和 provider。

这增加了三类成本：

1. 用户需要理解 Doctor、Signal Scan、AI Discovery、Opportunity、Site Fix 等过多概念。
2. 同一问题可能在多个页面、队列和生命周期中重复出现，dismiss、approve、verify 状态不能同步。
3. 工程团队需要维护多套 detection、identity、deduplication、routing 和 scheduling 逻辑。

本 PRD 将产品收敛为两条互相独立的用户产品线：

```text
Doctor
发现网站当前的缺陷和可立即验证的站内优化
-> Site Fix
-> Apply
-> Immediate Verification
-> Healthy / Improved

Opportunities
发现需要通过时间窗口验证的增长假设
-> Growth Action / Content
-> Review
-> Publish
-> Measurement
-> Learning
-> Next Opportunity
```

两条线共享一层 evidence infrastructure，但不共享用户工作对象：

```text
Crawl + GSC + GA4 + AI Answers + Context + Publisher State
                         |
              Shared Evidence Layer
                 /               \
         Doctor Decision      Growth Decision
              |                    |
          Site Fixes          Opportunities
```

核心产品合同：

> 如果成功可以在修改后立即通过 crawl、render、schema、link、tracking 或配置检查验收，工作归 Doctor。

> 如果成功必须等待 rankings、CTR、traffic、AI citations、engagement 或 conversions 的测量窗口，工作归 Opportunities。

Signal Scan 与 AI Discovery 不再是用户可见的独立产品线。它们成为内部 searching capabilities，由 Doctor 和 Opportunities 按各自目标调用。Token 成本不是首要优化目标；只要 evidence grounded、结果质量和闭环效果更好，产品允许积极使用 AI。

## 1. Background And Problem

### 1.1 当前存在三套 discovery 心智

当前产品要求用户理解：

- Doctor：技术健康 finding。
- Signal Scan：GSC、crawl、inventory、Context 驱动的 opportunity。
- AI Discovery：prompt、answer engine observation、citation gap 驱动的 opportunity。
- Site Fixes：部分 Doctor 或 Opportunity 的执行目的地。
- Content Plan：另一部分 Opportunity 的执行目的地。

这些名称描述了系统实现方式，而不是用户任务。用户真正想回答的只有两个问题：

1. 我的网站现在有什么需要修复或优化？
2. 我下一步应该做什么，才能获得可测量的增长？

### 1.2 同一事实被重复解释

Doctor 与 Signal Scan 当前都读取 `technical_checks`。同一 URL 可能同时产生：

| 实际情况 | Doctor | Signal Scan |
|---|---|---|
| HTTP 错误 | `broken_url` | `technical_visibility_issue` |
| noindex | `noindex` | `technical_visibility_issue` |
| canonical 缺失 | `canonical_missing` | `technical_visibility_issue` |
| JSON-LD 缺失 | `structured_data_missing` | `schema_gap` |
| 内部链接不足 | `internal_link_gap` | `internal_link_gap` |

Doctor findings 与 SEO opportunities 使用不同表、状态和 identity。结果是用户可以在 Doctor dismiss 一个问题，却仍在 Opportunities 看到相同问题；也可能从两个入口创建两个相似 Site Fix。

### 1.3 不同 type 仍会产生相同工作

Signal Scan 与 AI Discovery 即使 opportunity type 不同，也可能要求同样的内容：

| Signal Scan | AI Discovery | 可能产生的相同工作 |
|---|---|---|
| `thin_evidence_page` | `geo_project_mentioned_without_citation` | 强化现有页面证据块 |
| `cold_start_competitive_gap` | `geo_competitor_cited_project_absent` | 创建 comparison / alternative page |
| `gsc_query_gap` | AI prompt coverage gap | 扩写同一 URL 或创建同一主题页面 |
| `schema_gap` | citation readiness gap | 增加实体、schema 和可抽取事实 |

当前去重主要依赖 `type + normalized URL + query + intent + engine`。它无法识别“不同 type、相同目标、相同修改、相同成功指标”的语义重复。

### 1.4 Site Fix 身份不稳定

Doctor 的多个 finding 会被映射为通用的 `technical_visibility_issue`。同一 URL 的 canonical、metadata、HTTP、H1 等不同问题可能：

- 在 Opportunity 层错误合并；
- 后一次 evidence 覆盖前一次；
- 又因为 `action_type` 文案不同而创建多个 Site Fix actions。

这同时产生上游错误合并和下游重复执行。

### 1.5 调度和网络调用重复

当前可能同时存在：

- daily Signal Scan；
- daily automatic AI Discovery；
- weekly GEO loop 再运行 AI Discovery；
- weekly Doctor；
- manual Opportunity Finding；
- manual Doctor。

在 `all` 模式下，一次 Opportunity Finding 可能先做 SEO technical GET，再做 AI crawler audit，再 monitor external surfaces。多个 scheduler 也可能重复运行同一 AI prompt 或 HTTP probe。

### 1.6 闭环对用户不可见

产品内部已有 opportunity、action、topic、article、publish、measurement 等对象，但用户很难回答：

- 这个工作最初由什么 evidence 发现？
- 为什么归 Doctor 或 Opportunities？
- 系统现在在执行哪一步？
- 修复是否已部署并验证？
- 内容是否真的改善了 ranking、citation 或 conversion？
- 本轮结果如何影响下一轮决策？

如果闭环不可见，自动化越强，用户越难建立信任。

## 2. Product Thesis

### 2.1 两条产品线

#### Doctor

Doctor 是持续运行的网站质量与站内优化系统。

它回答：

- 当前网站哪里坏了？
- 哪些页面虽然没有坏，但存在可明确验证的质量提升？
- 哪些问题会阻塞 search、AI discovery、measurement 或 publishing？
- CiteLoop 能否安全地产生并应用一个 Site Fix？
- 修复后，原问题是否已消失？

Doctor 不仅报告缺陷，也主动发现不改变页面需求目标的站内优化。它可以优化已有信息的结构、表达、机器可读性、链接、metadata、measurement readiness 和 crawler access，但不能凭空增加未经 Context 支持的产品 claim。

#### Opportunities

Opportunities 是闭环增长系统。

它回答：

- 搜索用户和 AI answer engines 正在表达什么需求？
- 哪个增长假设值得优先执行？
- 应创建、刷新、合并或重新定位什么内容？
- 内容发布后是否改善 impressions、CTR、traffic、AI citations、engagement 或 conversion？
- 本轮结果应该如何改变下一轮 strategy？

### 2.2 共享 searching，不共享工作对象

Doctor 与 Opportunities 可以同时读取：

- public crawl；
- sitemap 与 robots；
- GSC page/query/search appearance；
- GA4 landing page、engagement、key events；
- AI answer provider outputs、citations 与 competitor mentions；
- Context、inventory、topics、articles；
- publisher/deployment/verification state。

共享 evidence 不代表共享 conclusion。每个 candidate 必须经过 owner arbitration，最终只能进入一条产品线。

### 2.3 Token policy

本产品不以最小化 token 使用为主要目标。

AI 使用原则：

1. 优先提高 evidence coverage、判断质量、修复质量和增长效果。
2. 允许多阶段或多模型分析，只要每一步有明确价值。
3. 所有结论必须保留 evidence provenance、model/provider、prompt version、cost 和 confidence。
4. AI 不得把推测包装成 observed fact。
5. 成本是可观测 guardrail，不是阻止高质量分析的默认限制。

## 3. Goals

1. 将三条用户 discovery 心智收敛为 Doctor 与 Opportunities 两条线。
2. 允许两条线充分使用 AI、GSC、GA4、crawl 和 Context。
3. 让 Doctor 同时发现缺陷和可立即验证的主动站内优化。
4. 让 Opportunities 专注于需要延迟测量的增长假设与内容闭环。
5. 同一个实际工作只能由一条线拥有。
6. 消除重复 Opportunity、Site Fix、content brief、page update 和 publication work。
7. 让用户清楚看到每条闭环的当前步骤、evidence、owner、结果与下一步。
8. 合并重复调度、crawl、provider observation 和 analysis passes。
9. 保留历史 provenance，使迁移后仍能解释旧 finding/action 的来源。
10. 为 AI-first discovery 提供质量优先而非 token-minimal 的运行策略。

## 4. Non-Goals

- 不要求 Doctor 与 Opportunities 使用不同 provider 或完全不同 evidence storage。
- 不禁止同一 evidence 被两条线引用。
- 不把所有内部 analyzer 合并成一个巨大服务。
- 不允许 Doctor 以“优化”为名创建新的 keyword target、persona、unsupported claim 或全新内容主题。
- 不允许 Opportunities 承担 broken URL、canonical、robots、schema validity 等即时技术验收工作。
- 不以 opportunity type 字符串作为最终 owner 判断标准。
- 不在本 PRD 中定义具体 token 单价或供应商采购策略。
- 不承诺 SEO ranking、traffic、conversion 或 AI citation 一定提升。
- 不通过删除 evidence 或降低扫描覆盖来实现去重。

## 5. Core Ownership Rule

### 5.1 Primary rule

每个 candidate 必须声明一个 primary success contract：

| Success contract | Owner |
|---|---|
| 修改后可以立即通过 crawl、render、schema、link、tracking、deployment 或配置检查验证 | Doctor |
| 必须经过观察窗口，通过 ranking、CTR、traffic、citation、engagement 或 conversion 判断 | Opportunities |

### 5.2 Secondary rules

当 primary rule 仍不明确时，依次使用：

1. **Intent preservation**：不引入新需求目标、persona、topic 或 claim，只修复/重组已有信息，优先 Doctor。
2. **Demand hypothesis**：工作由 query、prompt、competitor demand、traffic decay 或 conversion gap 驱动，归 Opportunities。
3. **Verification latency**：秒/分钟级确定性检查归 Doctor；天/周级统计变化归 Opportunities。
4. **One owner**：混合 candidate 不能同时生成两份工作；必须选择 primary owner，另一条线只附加 evidence 或 blocker。
5. **Blocker relationship**：Doctor blocker 可以阻塞 Opportunity 执行，但不会复制该 Opportunity。

### 5.3 Examples

| 情况 | Owner | 原因 |
|---|---|---|
| title tag 缺失 | Doctor | HTML 可立即验证 |
| title 长度、重复或模板变量错误 | Doctor | metadata contract 可立即验证 |
| 为已有高曝光 query 提升 CTR 而重写 title | Opportunities | 成功依赖 CTR 测量窗口 |
| canonical 缺失或错误 | Doctor | HTML 与 URL contract 可立即验证 |
| JSON-LD 缺失、无效或与页面事实不一致 | Doctor | schema parser 可立即验证 |
| 为提高某 AI prompt citation rate 增加证据内容 | Opportunities | 成功依赖 AI observation |
| 已有证据没有清晰、可抽取的事实结构 | Doctor | 页面结构与事实支持可立即验证 |
| 创建新的 comparison page | Opportunities | 新内容与增长假设 |
| 内部链接为 0、broken 或指向 redirect chain | Doctor | link graph 可立即验证 |
| 为提升特定 cluster 排名设计新 internal-link strategy | Opportunities | 目标是延迟排名结果 |
| GA4 tag 或 key event 未正确触发 | Doctor | instrumentation 可立即验证 |
| landing page conversion rate 低，需要新 offer/copy | Opportunities | 成功依赖 conversion measurement |
| 页面内容衰退 | Opportunities | 由历史表现变化驱动 |
| robots 阻止 search/AI crawler | Doctor | robots contract 可立即验证 |
| AI 没引用项目但引用竞品 | Opportunities | AI demand/citation gap |

## 6. Target Product Architecture

```text
Source Connectors
  |- Public crawler
  |- Sitemap / robots
  |- Google Search Console
  |- Google Analytics 4
  |- AI answer providers
  |- Context and content inventory
  |- Publisher and deployment state
  v
Evidence Collection And Normalization
  |- normalized URL / entity / query / prompt
  |- observation window
  |- source confidence
  |- raw evidence snapshot
  v
Candidate Generation
  |- deterministic rules
  |- statistical detection
  |- AI reasoning
  |- multi-source synthesis
  v
Ownership Arbitration
  |- immediate quality contract -> Doctor
  |- delayed growth contract -> Opportunities
  |- duplicate/conflict suppression
  v
+--------------------------+    +---------------------------+
| Doctor                   |    | Opportunities             |
| Finding -> Site Fix      |    | Hypothesis -> Growth Work |
| -> Apply -> Verify       |    | -> Publish -> Measure     |
| -> Healthy / Improved    |    | -> Learn -> Next Loop     |
+--------------------------+    +---------------------------+
```

### 6.1 Required isolation

- Doctor queue 不读取 `seo_opportunities` 作为工作来源。
- Opportunities queue 不读取 Doctor findings 作为 growth opportunity。
- Doctor Site Fix 不写入 Content Plan。
- Opportunity Content/Growth Action 不写入 Doctor Site Fix queue。
- 一条线可以引用另一条线的 object ID 作为 dependency/blocker，但不能复制其 action。
- 用户对一条线的 dismiss/watch/approve 不会隐式改变另一条线的独立对象；共享 candidate 已在 arbitration 前完成去重，因此不应存在需要同步 dismiss 的重复工作。

## 7. Shared Evidence Layer

### 7.1 Evidence object

所有 searching stage 输出统一 observation envelope：

```json
{
  "project_id": "uuid",
  "source": "crawl|gsc|ga4|ai_answer|context|publisher",
  "source_run_id": "uuid",
  "observed_at": "timestamp",
  "window_start": "date|null",
  "window_end": "date|null",
  "normalized_target": "url|query|prompt|entity",
  "target_kind": "page|query|prompt|entity|site|integration",
  "facts": {},
  "raw_snapshot": {},
  "confidence": 0.0,
  "completeness": 0.0,
  "provider": "string|null",
  "model": "string|null",
  "prompt_version": "string|null",
  "cost_usd": 0.0
}
```

### 7.2 Shared collection rules

1. 同一 project、source、target、observation window 的 active collection job 必须幂等。
2. Doctor 与 Opportunities 请求相同 freshness 时复用 evidence snapshot，不重复 crawl/provider call。
3. 需要不同 user agent、query dimensions 或 AI prompt 时可以产生不同 observation，但必须共享 normalized target。
4. Evidence 必须区分：observed、inferred、model_assisted、missing、provider_unavailable。
5. “没有数据”不能等同于“发现问题”。
6. GSC/GA4 隐私和权限状态必须随 evidence 保存。
7. AI answer 原文、citations、competitor mentions 与 project surface matches 必须可追溯。

### 7.3 Freshness targets

| Evidence | Default freshness |
|---|---|
| HTML / technical crawl | 24 hours after relevant deployment；otherwise 7 days |
| robots / sitemap | 24 hours after change；otherwise 7 days |
| GSC page/query | daily，排除未稳定日期 |
| GA4 page/key event | daily |
| AI answer observations | weekly，或 content publish/major update 后触发 |
| publisher/deployment | event-driven + reconciliation fallback |
| Context/inventory | source change or user confirmation event |

## 8. Doctor Product Line

### 8.1 User promise

Doctor 持续回答：

> 站点现在是否健康？还有哪些可以安全优化？修复是否真的生效？

Doctor 既包含 `Broken` findings，也包含 `Optimization` findings：

- `Broken`：违反明确技术或内容质量 contract。
- `Optimization`：页面可用，但存在可以即时验证、不会改变页面需求目标的改进。
- `Healthy`：通过检查的页面与 contract，必须可见，避免 Doctor 只展示负面问题。

### 8.2 Doctor searching sources

#### Public crawl

- HTTP status、redirect chain、final URL。
- canonical、robots meta、X-Robots-Tag。
- title、meta description、H1、heading hierarchy。
- JSON-LD presence、validity、page-fact consistency。
- internal/outbound links、broken links、orphan risk。
- sitemap inclusion、lastmod consistency、URL variants。
- content extractability、rendered body、template leakage。
- supported product facts、source links、author/entity signals。
- measurement tags and consent-safe instrumentation presence。

#### GSC

Doctor 使用 GSC 来：

- 选择高价值页面优先检查；
- 判断 technical finding 的 affected demand；
- 识别 GSC-discovered URL 与 crawl/canonical/sitemap 的不一致；
- 标记 indexing/coverage evidence；
- 在修复后确认 Google 是否重新看到正确 URL state。

Doctor 不把 CTR、position 或 clicks uplift 当成自身 success contract。

#### GA4

Doctor 使用 GA4 来：

- 选择高流量 landing pages 优先检查；
- 检查 measurement readiness、page_view、key event 和 attribution plumbing；
- 识别 public page 与 tracked landing path 的不一致；
- 为 severity 提供 impact context。

Doctor 不以 conversion uplift 作为 Site Fix 的完成条件。

#### AI

Doctor 使用 AI 来：

- 识别页面类型、实体、事实和 intended contract；
- 对比 rendered page 与 Context，发现 unsupported、inconsistent 或不可抽取表达；
- 分析 schema 与页面事实是否一致；
- 评估 citation-ready structure，而不是预测 citation 一定增加；
- 归纳多 URL pattern，避免每页重复 finding；
- 生成最小、安全、evidence-backed 的修复方案和 acceptance tests；
- 对修复后的页面做第二次 structured review。

### 8.3 Doctor finding families

#### A. Crawl and availability

- broken URL；
- redirect loop/chain；
- soft 404；
- timeout、challenge、rate limit；
- unexpected content type；
- non-extractable body；
- important page not discovered。

#### B. Indexability and URL integrity

- noindex conflict；
- canonical missing/mismatch/multiple；
- HTTP canonical disagreement；
- sitemap missing/stale/variant mismatch；
- robots conflict；
- duplicate URL variants；
- GSC-discovered orphan/alternate URL inconsistency。

#### C. Metadata and rendering

- title missing、duplicate、template leakage、length/readability issue；
- description missing、duplicate、unsupported promise；
- H1 missing/multiple/inconsistent；
- OG metadata conflict；
- hydration/render mismatch；
- unsafe MDX/script rendering。

#### D. Structured data and entity clarity

- JSON-LD missing/invalid；
- schema type inconsistent with page role；
- canonical/entity URL mismatch；
- organization/product/software entity inconsistency；
- missing supported properties；
- schema contains unsupported claims；
- visible content and structured data disagreement。

#### E. Link and information architecture

- zero/broken internal links；
- orphan page；
- redirecting internal links；
- anchor/target mismatch；
- navigation/sitemap discovery gaps；
- obvious cluster relationship not represented without creating a new content target。

#### F. GEO and citation readiness

- AI/search crawler robots block；
- content not extractable；
- supported facts exist but lack a self-contained answer block；
- claims lack visible source/evidence association；
- entity names and product facts conflict across owned pages；
- citation target points to unstable/noncanonical surface。

#### G. Measurement readiness

- GA4/page tracking absent or duplicated；
- key event configured but not observed where contract requires it；
- canonical URL and analytics page path cannot be reconciled；
- publish/update verification missing；
- measurement baseline cannot be trusted because instrumentation is broken。

### 8.4 Doctor optimization guardrails

Doctor 可以修改 existing page，但必须同时满足：

1. 不创建新的 keyword/prompt target。
2. 不改变页面 primary intent 或 target persona。
3. 不引入 Context 未支持的 claim、comparison 或 competitor statement。
4. 修改可以用确定性或 bounded AI acceptance test 立即验证。
5. 修改不会替代需要 measurement window 的 growth hypothesis。

允许的修改示例：

- canonical、robots、sitemap、redirect；
- metadata contract correction；
- JSON-LD；
- 修复 broken/zero internal links；
- 将已有 supported facts 重排为可抽取结构；
- 修复实体命名不一致；
- 增加已有 evidence 的 visible source association；
- 修复 GA4 instrumentation；
- 修复渲染、模板和 unsafe output。

不允许的修改示例：

- 新建 comparison/alternative/use-case page；
- 为新 query 扩写正文；
- 改变 offer、positioning 或 CTA 以追求 conversion uplift；
- 基于 content decay 重写整篇文章；
- 为提高 AI citation rate 创建新证据内容；
- 合并/拆分页面以测试 ranking hypothesis。

### 8.5 Doctor lifecycle

```text
Evidence refreshed
-> Finding detected
-> AI diagnosis and proposed fix
-> Human/policy approval
-> Site Fix prepared
-> Apply via PR/CMS/manual handoff
-> Deployment observed
-> Immediate verification
-> Verified / Failed / Reopened
-> Site health baseline updated
```

Required states：

```text
finding: active | dismissed | resolved | converted
site_fix: proposed | approved | preparing | ready_to_apply | applying |
          awaiting_deploy | verifying | verified | failed | superseded
```

Doctor completion requires verification evidence. `PR merged`、`CMS returned 200` 或 `user clicked done` 只能表示 applied，不能表示 verified。

### 8.6 Doctor output

每条 Doctor finding 必须包含：

- finding family 与 issue identity；
- Broken 或 Optimization；
- affected/healthy URLs；
- observed evidence 与 freshness；
- GSC/GA4 impact context；
- AI reasoning summary 与 confidence；
- why it matters；
- proposed minimal fix；
- exact acceptance tests；
- risk、review requirement、rollback plan；
- current owner 与 lifecycle state；
- verification result。

## 9. Opportunities Product Line

### 9.1 User promise

Opportunities 持续回答：

> 基于真实需求和历史结果，下一项最值得执行的增长工作是什么？执行后产生了什么结果？系统学到了什么？

### 9.2 Internal searching capabilities

Signal Scan 和 AI Discovery 保留为内部 capability 名称，可出现在 diagnostics/run detail 中，但不再作为用户选择产品线。

#### Search performance reasoning

- GSC impressions、clicks、CTR、position、query/page relationship；
- striking distance；
- low CTR with demand；
- query gap；
- content decay；
- cannibalization；
- search appearance changes；
- new/rising demand。

#### Analytics reasoning

- GA4 landing sessions、engagement、key events；
- high-traffic/low-conversion gap；
- query-to-landing-to-conversion relationship；
- content-assisted conversion；
- published content cohorts；
- statistically honest low-volume handling。

#### AI discovery reasoning

- prompt generation from Context、topics、queries、competitors；
- multi-provider answer observation；
- brand mention、project citation、competitor citation；
- citation surface and rank；
- prompt coverage；
- answer/source changes over time；
- evidence and entity gaps tied to observed prompts。

#### Context and market reasoning

- ICP、positioning、features、differentiators；
- competitor and alternative language；
- existing inventory coverage；
- supported claims and evidence；
- content format and publication surface。

### 9.3 Growth opportunity families

#### A. Search demand capture

- striking-distance page improvement；
- query gap；
- low-CTR snippet hypothesis；
- new/rising query；
- search appearance opportunity。

#### B. Content portfolio

- content decay refresh；
- cannibalization consolidation；
- missing use case；
- comparison/alternative page；
- supporting cluster asset；
- new landing/content surface。

#### C. AI visibility growth

- competitor cited、project absent；
- brand mentioned、owned source not cited；
- prompt category uncovered；
- weak citation surface；
- answer-ready evidence asset；
- external/public surface opportunity。

#### D. Conversion and engagement

- high-demand landing page with weak conversion；
- engaged traffic without clear next step；
- content-to-offer mismatch；
- assisted conversion opportunity；
- content cohort expansion based on measured winners。

### 9.4 Opportunities guardrails

每个 Opportunity 必须具备：

- explicit growth hypothesis；
- target audience/query/prompt；
- baseline window；
- primary metric；
- expected direction and plausible range；
- target page/content surface；
- required evidence and Context constraints；
- execution type；
- measurement window；
- attribution confidence plan；
- stop/rollback/reconsider rule。

没有 baseline 或可定义的 success metric 时，只能进入 `needs_evidence`，不能伪装成 decision-ready opportunity。

### 9.5 Opportunities lifecycle

```text
Evidence refreshed
-> Candidate generated
-> AI multi-source hypothesis
-> Ownership and duplicate arbitration
-> Opportunity review
-> Growth action / content brief
-> Draft or page update
-> Review
-> Publish
-> Baseline and measurement window
-> Positive / Negative / Inconclusive
-> Learning extracted
-> Strategy and next discovery updated
```

Required user-visible stages：

```text
Found
-> Needs decision
-> Planned
-> Creating
-> Needs review
-> Publishing / Applying
-> Measuring
-> Learned
```

### 9.6 Closed-loop learning

每个完成 measurement window 的 action 必须生成 learning record：

- hypothesis；
- executed change；
- baseline and after windows；
- search metrics；
- GA4 metrics；
- AI citation metrics；
- confounders；
- attribution confidence；
- outcome；
- reusable lesson；
- affected future candidate scores。

未来 Opportunity scoring 必须读取 learning，而不是每轮从零开始。

## 10. Ownership Arbitration And Deduplication

### 10.1 Candidate before product object

所有 detectors 先产生 internal `candidate`, 不得直接写 Doctor finding、SEO opportunity、Site Fix 或 content action。

Candidate 至少包含：

```text
candidate_id
project_id
target_kind
normalized_target
issue_or_hypothesis_family
change_family
primary_success_metric
verification_mode
evidence_ids
suggested_owner
confidence
```

### 10.2 Canonical work identity

工作 identity 不再依赖 recommendation 文案。建议逻辑键：

```text
project_id
+ normalized target/entity
+ change family
+ primary success contract
+ target intent
```

示例：

- `page:/pricing + schema.organization + immediate.schema_validity`
- `page:/pricing + metadata.title + delayed.gsc_ctr + query:pricing software`
- `topic:competitor-x + content.comparison + delayed.ai_citation`

同一 identity 只能有一个 active work item。

### 10.3 Semantic conflict resolver

Arbitration 必须执行：

1. exact identity dedupe；
2. same target + overlapping change-family detection；
3. AI semantic comparison of proposed changes；
4. existing active Doctor Site Fix lookup；
5. existing active Opportunity/content action lookup；
6. ownership rule；
7. suppress、merge evidence、attach dependency 或 create。

AI semantic comparison可以消耗 token，但必须输出 structured decision：

```json
{
  "decision": "create|merge_evidence|suppress|block_on_other_line",
  "owner": "doctor|opportunities",
  "canonical_work_id": "string",
  "overlaps": ["work-id"],
  "reason": "string",
  "confidence": 0.0
}
```

### 10.4 Cross-line relationship

允许以下关系：

- Opportunity `blocked_by_doctor_finding`；
- Opportunity `blocked_by_site_fix`；
- Doctor finding `provides_evidence_to_opportunity`；
- Doctor verification `unblocks_opportunity`。

禁止以下关系：

- Doctor finding 自动复制成 growth opportunity；
- Growth opportunity 自动复制成 Doctor Site Fix；
- 两条线对相同 target/change 同时执行；
- 一条线通过不同 action wording 绕过 identity。

### 10.5 Conflict examples

#### Schema missing + AI citation gap

- Doctor 创建 schema Site Fix。
- AI citation Opportunity 保留为独立增长 hypothesis，但在执行 evidence expansion 前可以依赖 schema fix。
- AI Opportunity 不再创建第二个 schema patch。

#### Missing title + low CTR

- title missing 先归 Doctor，修复并验证。
- low CTR Opportunity 标记 `blocked_by_site_fix`。
- Doctor 修复后重新建立 CTR baseline，再决定是否仍需要 growth title experiment。

#### Thin evidence + brand mentioned without citation

- 如果页面已有 supported evidence，只是结构不可抽取：Doctor。
- 如果需要新增证据内容或新的 source-backed asset：Opportunities。
- 两者不能同时创建“增强 evidence block”的相似 action。

#### Internal link zero + ranking cluster opportunity

- 修复 zero/broken inlinks：Doctor。
- 基于排名 hypothesis 的 cluster expansion：Opportunities。
- Opportunity 可以消费 Doctor 修复后的 link graph，但不重复修链接。

## 11. AI Operating Model

### 11.1 AI roles in Doctor

- page role classification；
- evidence-grounded diagnosis；
- multi-page pattern grouping；
- schema/page consistency review；
- citation-readiness structure review；
- minimal patch planning；
- risk and rollback generation；
- verification interpretation。

### 11.2 AI roles in Opportunities

- query/prompt clustering；
- demand and competitor synthesis；
- multi-source growth hypothesis；
- expected impact reasoning；
- content brief and outline；
- draft/page update generation；
- editorial QA；
- outcome and confounder explanation；
- learning extraction and next-action recommendation。

### 11.3 Quality-first execution

允许：

- 多 provider AI observations；
- 高价值页面多模型 review；
- analyzer 与 verifier 使用不同 model；
- 重要 candidate 的 critique/revision pass；
- 在 evidence conflict 时追加 targeted calls。

要求：

- provider/model routing 可审计；
- per-run token/cost 可见；
- failure 可降级但不能伪造结果；
- low-confidence AI candidate 不直接进入自动执行；
- AI content/fix 必须 grounded in Context 和 observed evidence。

## 12. User Experience And Visible Loops

### 12.1 Navigation

用户主导航只暴露：

```text
Home
Doctor
Opportunities
Content
Results
Context
Settings
```

Signal Scan 与 AI Discovery 只出现在：

- run details；
- evidence/source badges；
- admin/provider diagnostics；
- cost and coverage reports。

用户不需要选择“我现在使用哪条 discovery 产品线”。

### 12.2 Doctor page

Doctor 首屏必须展示：

- current health and optimization score；
- last evidence refresh；
- Broken / Optimization / Healthy coverage；
- top issues；
- active Site Fix progress；
- next recommended Doctor action。

Doctor finding drawer 必须展示：

- evidence；
- source freshness；
- GSC/GA4 impact context；
- AI explanation；
- proposed fix and acceptance tests；
- risk/approval；
- exact lifecycle；
- verification evidence。

Doctor loop visualization：

```text
Found -> Fix prepared -> Approved -> Applied -> Verifying -> Verified
```

### 12.3 Opportunities page

Opportunities 首屏必须展示：

- opportunity queue；
- last growth discovery run；
- source coverage summary；
- growth loop in motion；
- measuring and learned counts；
- next user decision。

Opportunity drawer 必须展示：

- hypothesis；
- GSC/GA4/AI evidence；
- why now；
- target audience/query/prompt；
- planned content/action；
- baseline and success metric；
- measurement window；
- dependencies；
- owner and destination。

Growth loop visualization：

```text
Found -> Decided -> Planned -> Created -> Published -> Measuring -> Learned
```

点击任一步必须打开对应真实对象，不能只展示聚合数字。

### 12.4 Home

Home 分别展示两张独立控制卡：

- `Site health`：Doctor status、blocker、active fix、verification。
- `Growth loop`：opportunities、content in motion、measurement、learning。

Doctor blocker 可以出现在 Growth 卡的 dependency 文案中，但两个指标和 CTA 不合并。

### 12.5 Results

Results 分两类 outcome：

- Doctor verification results：即时、确定性、是否通过 acceptance tests。
- Growth outcomes：延迟、统计性、positive/negative/inconclusive。

两类结果可以在同一 Results 页面分区展示，也可以由 Doctor/Opportunities 各自展示详情；数据语义不得混合。

## 13. Data Model Direction

### 13.1 Recommended logical domains

```text
evidence_runs
evidence_observations
discovery_candidates
work_identity_registry

doctor_runs
doctor_findings
site_fixes
site_fix_verifications

growth_opportunities
growth_actions
topics
articles
action_measurements
growth_learnings

work_relationships
```

### 13.2 Physical migration guidance

现有表可阶段性复用，但必须建立严格 owner：

- `seo_doctor_findings` 迁移为 Doctor canonical findings。
- 现有 technical `seo_opportunities` 转回 Doctor findings 或标记 legacy，不再生成新技术 opportunity。
- `content_actions` 中 Site Fix 类 action 迁入 dedicated `site_fixes`，或在过渡期增加不可为空的 `product_line` 并用约束隔离。
- content/growth actions 保留在 Growth domain。
- `geo_observations`、GSC/GA4 daily tables、technical checks 作为 shared evidence source。
- `geo_asset_briefs` 只保留真正的 growth content brief；crawler/schema repair 不进入 asset brief。

长期推荐 dedicated `site_fixes` 表，而不是继续让技术修复和内容 action 共用一个宽泛状态机。

### 13.3 Required invariants

1. `canonical_work_identity` 在 active work 范围内唯一。
2. 一个 work item 只能有 `doctor` 或 `opportunities` owner。
3. Doctor Site Fix 不得关联 topic/article creation，除非只是验证目标，不代表内容所有权。
4. Growth Action 不得把 immediate technical acceptance 当作最终 outcome。
5. 每个 action 必须引用 candidate 和 evidence snapshot。
6. 每个 verified/learned outcome 必须引用实际 applied/published artifact。

## 14. API Direction

### 14.1 Doctor

```text
POST /projects/{id}/doctor/runs
GET  /projects/{id}/doctor
GET  /projects/{id}/doctor/runs/{runID}
GET  /projects/{id}/doctor/findings/{findingID}
POST /projects/{id}/doctor/findings/{findingID}/dismiss
POST /projects/{id}/doctor/findings/{findingID}/site-fixes
GET  /projects/{id}/doctor/site-fixes
POST /projects/{id}/doctor/site-fixes/{fixID}/approve
POST /projects/{id}/doctor/site-fixes/{fixID}/apply
POST /projects/{id}/doctor/site-fixes/{fixID}/verify
```

Doctor 不再通过 `seo_opportunities` 中转 Site Fix。

### 14.2 Opportunities

```text
POST /projects/{id}/opportunities/runs
GET  /projects/{id}/opportunities/status
GET  /projects/{id}/opportunities
GET  /projects/{id}/opportunities/{opportunityID}
POST /projects/{id}/opportunities/{opportunityID}/dismiss
POST /projects/{id}/opportunities/{opportunityID}/watch
POST /projects/{id}/opportunities/{opportunityID}/approve
GET  /projects/{id}/growth-actions
GET  /projects/{id}/growth-actions/{actionID}/measurement
GET  /projects/{id}/growth-learnings
```

### 14.3 Evidence and diagnostics

Internal/admin APIs 可展示：

- source runs；
- Signal Scan stages；
- AI Discovery stages；
- provider/model/cost；
- source freshness；
- arbitration decisions；
- suppressed duplicate candidates。

这些不是默认用户 queue。

## 15. Scheduling And Orchestration

### 15.1 One scheduler per product line

目标调度：

- Shared evidence refresh：按 source freshness 和事件触发。
- Doctor evaluation：weekly、deployment 后、manual。
- Opportunities evaluation：daily、Context change、content outcome、manual。
- AI answer observation：按 Opportunities cadence 或 high-value trigger，不再由独立 weekly GEO scheduler 绕过设置。

### 15.2 Required changes

1. 移除重复的 standalone weekly AI Discovery orchestration，或让它只产生 shared evidence refresh，不直接生成 opportunities。
2. `Opportunity Finding` run 只创建 Growth candidates，不创建 Doctor work。
3. Doctor run 只创建 Doctor candidates/findings，不创建 Growth work。
4. 同一 evidence job 使用 durable run、idempotency key、retry 和 partial failure。
5. Scheduler 不因一个 provider/URL 失败中断所有其他 source。
6. Provider unavailable 必须显示 coverage degraded，不生成零值假 finding。

### 15.3 Events

建议事件：

```text
context.confirmed
evidence.refresh.requested
evidence.refresh.completed
doctor.evaluation.requested
doctor.finding.ready
site_fix.applied
site_fix.verification.requested
site_fix.verified
growth.discovery.requested
growth.opportunity.ready
growth.action.published
growth.measurement.due
growth.learning.ready
```

## 16. Error Handling And Trust

### 16.1 Partial evidence

- GSC unavailable：Doctor 仍可 public scan；Opportunities 不生成 query-derived claims。
- GA4 unavailable：不生成 conversion-derived opportunity；显示 measurement limitation。
- AI provider unavailable：保留 deterministic candidates，标记 AI review skipped。
- Crawl partial：报告 checked/skipped/failed URLs，不把未检查页面当作 healthy。
- Deployment unknown：Site Fix 保持 awaiting deploy，不进入 verified。

### 16.2 Confidence

每个 finding/opportunity 必须分别展示：

- evidence confidence；
- model confidence；
- coverage completeness；
- attribution confidence；
- missing sources。

### 16.3 High-risk actions

以下默认需要人工 approval：

- robots/canonical/indexability changes；
- redirect and URL consolidation；
- schema with legal/commercial claims；
- deleting/merging content；
- changing offer, pricing, positioning or conversion path；
- publishing competitor comparisons。

## 17. Product Metrics

### 17.1 Simplification metrics

- 用户可见 discovery product lines 从 3 降到 2。
- Signal Scan / AI Discovery 不再作为一级导航或 queue。
- 同一 target/change 的 active duplicate work rate < 1%。
- Doctor 与 Opportunities 的 cross-line duplicate action rate = 0。
- 用户从 finding 到正确目的地的 routing correction rate < 2%。

### 17.2 Doctor metrics

- finding precision；
- false-positive dismissal rate；
- Site Fix approval rate；
- apply success rate；
- immediate verification pass rate；
- reopen rate；
- time to verified；
- healthy coverage improvement；
- proactive optimization adoption。

### 17.3 Opportunities metrics

- decision-ready opportunity precision；
- approve/dismiss/watch distribution；
- time from approval to publish；
- measurement coverage；
- positive/negative/inconclusive rate；
- attribution confidence；
- percentage of future scoring influenced by learning；
- search、AI citation、engagement、conversion outcomes。

### 17.4 AI metrics

- grounded-output rate；
- unsupported-claim rate；
- AI candidate acceptance rate；
- AI verification agreement rate；
- provider coverage；
- token/cost per verified Site Fix；
- token/cost per measured Growth Action；
- incremental lift from AI-assisted vs deterministic-only decisions。

Cost 指标用于优化 routing 和质量，不设置为优先于 outcome 的单一目标。

## 18. Migration Plan

### Phase 0：Instrumentation And Inventory

- 列出现有 Doctor findings、SEO opportunities、content actions、Site Fixes、GEO briefs。
- 给每条 active work 计算 provisional canonical identity。
- 记录 duplicate/collision report，不改变用户数据。
- 增加 source、owner、verification mode diagnostics。

### Phase 1：Shared Candidate And Arbitration

- 引入 internal candidate envelope。
- Signal Scan、AI Discovery、Doctor detectors 先写 candidate。
- 引入 owner arbitration 和 semantic dedupe。
- Shadow mode 比较新旧输出，不改变 queue。

### Phase 2：Doctor Owns Site Fixes

- Doctor 创建独立 Site Fix，不再经 `seo_opportunities`。
- 迁移 active technical actions。
- Doctor 增加 proactive Optimization 与 Healthy coverage。
- Site Fix 增加 apply -> deploy -> verify 闭环。
- Opportunities 停止生成新的 schema/canonical/robots/metadata/basic-link repair。

### Phase 3：Opportunities Owns Growth Work

- Signal Scan 与 AI Discovery 合并为 internal searching stages。
- Opportunity 必须包含 hypothesis、baseline、metric、window。
- 增加 GA4、AI citation 与 learning input。
- 清理用户可见 source-mix 心智。

### Phase 4：Visible Closed Loops

- Doctor 展示 Site Fix lifecycle。
- Opportunities 展示 publish/measurement/learning lifecycle。
- Home 展示两张独立 control cards。
- Results 分开 immediate verification 与 delayed growth outcome。

### Phase 5：Legacy Cleanup

- 停止旧 technical opportunity generators。
- 删除或封存重复 weekly GEO orchestration。
- 迁移/归档 legacy duplicate work。
- 收紧 DB constraints 和 API contracts。
- 删除不再使用的 routing heuristics 与 source-mode settings。

## 19. Backward Compatibility

1. 已批准、执行中或 measuring 的工作不得静默删除。
2. 已有 Site Fix 优先迁入 Doctor，保留 original opportunity/action provenance。
3. 已有 content/GEO briefs 优先迁入 Opportunities。
4. 无法自动判定 owner 的 active item 进入 migration review，不自动复制。
5. 旧 URL/API 在迁移期可提供 deprecated alias，但返回 canonical object ID。
6. 旧 Activity Log 保留原 agent/source 名称。
7. 历史 Results 不重算，只标记 legacy attribution model。

## 20. Acceptance Criteria

### 20.1 Product boundary

1. 用户只需要理解 Doctor 和 Opportunities 两条 discovery 产品线。
2. Signal Scan 与 AI Discovery 不再拥有用户可见独立 queue。
3. Doctor 能展示 Broken、Optimization 和 Healthy coverage。
4. Doctor 只产生可立即验证的 Site Fix。
5. Opportunities 只产生需要 measurement window 的 Growth Work。
6. 每个 candidate 只能有一个 owner。

### 20.2 No duplicate work

7. 同一 URL 的 schema missing 不会同时生成 Doctor Site Fix 和 Opportunity Site Fix。
8. 同一 URL 的 zero internal links 不会产生两条修复工作。
9. title missing 与 low CTR title hypothesis 被识别为两个不同 success contract，并按 dependency 顺序执行。
10. thin evidence 与 AI citation gap 在 proposed changes 相同的情况下只生成一项工作。
11. 不同 action wording 不能绕过 canonical identity。
12. 创建 work 前必须检查两条线的 active work registry。

### 20.3 Doctor loop

13. Site Fix applied 后不会直接显示 verified。
14. Verification 必须重新读取对应 evidence 并执行 acceptance tests。
15. Verification failure 会保留 evidence、进入 failed/reopened，并给出下一步。
16. GSC/GA4 可以影响 Doctor coverage、priority 和 context，但不能把 delayed uplift 当作 Doctor completion。
17. Doctor AI output grounded in Context 和 observed page evidence。

### 20.4 Opportunities loop

18. 每个 decision-ready Opportunity 有 hypothesis、baseline、primary metric 和 measurement window。
19. 每个 published/applied Growth Action 能回到其 source evidence 和 Opportunity。
20. Measurement 到期后生成 positive、negative 或 inconclusive outcome。
21. 每个 outcome 生成 learning record。
22. 下一轮 scoring 能展示使用了哪些历史 learning。
23. 用户可以从 loop stage 点击到真实 action/content/result。

### 20.5 Scheduling and evidence

24. 相同 source/target/window 的 evidence refresh 不重复执行。
25. AI Discovery automation 不再被 standalone weekly scheduler 绕过项目设置。
26. Provider unavailable 不产生虚假零 citation/zero mention findings。
27. Crawl partial failure 不把未检查页面标为 healthy。
28. 所有 AI calls 记录 provider、model、prompt version、token/cost 和 status。

### 20.6 Migration

29. Existing in-flight work 保留 provenance 和用户决策。
30. Migration dry run 能列出所有 collision、duplicate 和 ambiguous owner。
31. 没有 active work 因迁移被静默丢失或重复执行。
32. Legacy routes 在迁移窗口返回 canonical object linkage。

## 21. Implementation Sequence Recommendation

本 PRD 范围较大，实施必须拆成独立 PRD/plan slices：

1. **Shared Candidate + Work Identity**：只建内部模型、shadow arbitration 和 duplicate report。
2. **Doctor Site Fix Ownership**：从 Opportunity routing 中移出技术修复，建立独立 verification lifecycle。
3. **Opportunity Growth Contract**：强制 hypothesis、baseline、metric、window，收敛 Signal Scan + AI Discovery。
4. **Scheduler Consolidation**：共享 evidence refresh，删除重复 AI/GEO runs。
5. **Visible Loops**：Doctor、Opportunities、Home、Results UI。
6. **Legacy Migration And Cleanup**：数据迁移、约束和旧 API 清理。

每个 slice 必须独立上线、可回滚，并在生产数据上报告 duplicate rate、routing differences 和 lifecycle integrity。

## 22. Final Product Contract

```text
Shared evidence may overlap.
Searching methods may overlap.
User work must not overlap.

Doctor owns immediate site quality.
Opportunities owns delayed growth outcomes.

Doctor closes the loop with verification.
Opportunities closes the loop with measurement and learning.
```
