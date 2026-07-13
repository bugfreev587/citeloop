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

### 0.1 2026-07-12 新工作来源边界

Doctor 与 Opportunities 是唯二用户可见的新工作来源；Content Plan、Site Fixes、Review、Publish 和 Results 都是下游执行或结果 surface，不是 intake source。

- Doctor finding 是新 Site Fix 的唯一产品来源，且该 Site Fix 必须有可立即执行的 acceptance tests。
- 已接受、且 materially creates or refreshes content 的 AI-generated Opportunity 是新 Content Brief 的唯一产品来源；每个 Content Brief 必须保留 source Opportunity ID 与 AI provenance。
- 产品不提供 manual Opportunity 或 manual Content Brief intake，也不允许 Content Plan 通过 Topic、Brief 或其他 ad hoc form 创建新工作。
- 用户仍可显式触发 AI Opportunity Finding。该操作只触发 AI searching/generation run；输出仍是 AI-generated candidate，必须经过 ownership arbitration 和 Opportunities acceptance，不能直接创建 Content Brief。
- 历史 source-less Topics 只保留 Content Plan Section 15.4 定义的 existing-record operations，不创建、不 clone、不迁移、不 backfill，也不能被复用为新 Brief 来源。
- Owner 与 artifact 分开判断：pure link-only patch 若可立即验证则为 Doctor Site Fix；若成功必须等待 growth window 则为 Opportunities Growth Action。两者都不是 Content Brief；只有 materially creates or refreshes content 的 scope 才进入 Content Plan。

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
- user-triggered AI Opportunity Finding；
- user-triggered Doctor evaluation。

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

Dependency 必须进一步区分：

- `hard_blocker`：两项工作会修改相同 field/selector/URL contract，或 Doctor 问题会让 Growth baseline、发布、归因不可用。必须等待 Doctor verified；用户不能绕过，否则会产生重复 mutation 或不可解释的 measurement。
- `soft_dependency`：两项工作 mutation 不重叠，且当前 baseline/measurement 仍可信。允许并行执行，但 Opportunity 必须记录 Doctor finding/Site Fix、可能 confounder 与 attribution risk。
- resolver 必须保存 blocker classification、reason、overlapping mutation fields 和 reassessment trigger；不能把“同一页面存在 Doctor finding”一律解释为 hard blocker。

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
| 把当前页面已经陈述的事实重排成 answer block，不增加新事实命题 | Doctor | 仅改变结构，可做 proposition-preservation 检查 |
| Context 中存在但当前页面未陈述的新事实，需要加入正文以争取 citation | Opportunities | 新增 evidence proposition，成功依赖观察窗口 |
| 创建新的 comparison page | Opportunities | 新内容与增长假设 |
| 内部链接为 0、broken 或指向 redirect chain | Doctor | link graph 可立即验证 |
| 为提升特定 cluster 排名设计 pure link-only internal-link strategy | Opportunities | 目标是延迟排名结果；创建 Growth Action，不创建 Content Brief |
| GA4 tag 或 key event 未正确触发 | Doctor | instrumentation 可立即验证 |
| landing page conversion rate 低，需要新 offer/copy | Opportunities | 成功依赖 conversion measurement |
| 页面内容衰退 | Opportunities | 由历史表现变化驱动 |
| robots 阻止 search/AI crawler | Doctor | robots contract 可立即验证 |
| AI 没引用项目但引用竞品 | Opportunities | AI demand/citation gap |

Artifact rule：owner 由 success contract 决定，destination 由实际交付物决定。Pure link-only work 无论归 Doctor 或 Opportunities，都不能进入 Content Plan；只有 materially creates or refreshes content 的 accepted Opportunity 才产生 Content Brief。

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
- 新 Site Fix 必须引用 source Doctor finding；新 Content Brief 必须引用已接受的 AI-generated Opportunity。
- Content Plan、Site Fixes 或其他下游 surface 不得提供 manual Opportunity/Brief intake 或建立第三种 source record。
- 用户显式触发 AI run 不改变 record ownership：AI Opportunity Finding 的结果仍先进入 Opportunities，Doctor evaluation 的结果仍先进入 Doctor。

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
  "call_status": "ok|partial|failed|skipped|null",
  "prompt_tokens": 0,
  "completion_tokens": 0,
  "total_tokens": 0,
  "cost_usd": 0.0
}
```

如果一个 observation 由多次 AI call 合成，token、cost、status 必须保存在 linked call records，并在 observation 上保存 aggregate。不得只保存 cost 而丢失 token 与失败状态。

### 7.2 Shared collection rules

1. 同一 project、source、target、observation window 与 normalized collection-spec fingerprint 的 active collection job 必须幂等。
2. Doctor 与 Opportunities 请求相同 freshness 时复用 evidence snapshot，不重复 crawl/provider call。
3. Collection-spec fingerprint 必须覆盖所有会改变请求或证据语义的输入，包括 user agent、HTTP method/render mode、query dimensions/filters、country/device/locale、provider/model、prompt ID/version、sampling policy 和 normalization version。不同 fingerprint 可以产生不同 observation，但必须共享 normalized target；相同 fingerprint 才能复用 job。
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

Citation-readiness 的硬判据是 **proposition preservation**：

1. Doctor 输出中的每个事实命题必须已经存在于修改前的 live page；Context 只用于验证支持性，不能成为向页面新增事实的来源。
2. Doctor 可以重排、拆分、标记或为现有命题补充 Context 中已有的 source association，但不能新增 claim、comparison、proof point、topic/entity target 或 demand target。
3. 如果 proposed output 需要把 Context/source 中的新事实写入页面、创建新 evidence asset，或用 citation/ranking uplift 才能判断成功，candidate 必须归 Opportunities。
4. AI acceptance test 必须输出 `preserved_propositions`、`added_propositions`、`removed_propositions` 和 `source_association_changes`；`added_propositions` 非空时 Doctor fail closed，并重新路由。

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
          awaiting_deploy | verifying | verified | failed_retryable |
          reopened | failed_terminal | superseded
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

Opportunity 必须由 searching/generation pipeline 产生并保留 AI provenance。用户可以触发该 pipeline，但不能绕过 candidate generation、ownership arbitration 或 acceptance 直接写入 Opportunity；只有 accepted 且 materially creates or refreshes content 的 Opportunity 才能创建 Content Brief。

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
-> Positive / Negative / Mixed / Inconclusive / Insufficient data
-> [Positive / Negative / Mixed / Inconclusive] Learning extracted
-> Strategy and next discovery updated

or

-> [Final Insufficient data] Closed — insufficient data
-> Measurement-quality record only
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
-> Learned | Closed — insufficient data
```

### 9.6 Closed-loop learning

每个 action 可以有多个 measurement checkpoints：

```text
baseline -> early signal -> primary checkpoint -> follow-up checkpoint
```

- `primary checkpoint` 是默认闭环判断点。
- primary checkpoint 数据完整时，产生 `positive | negative | mixed | inconclusive` terminal outcome。
- primary checkpoint 为 `insufficient_data` 时不关闭增长闭环；系统进入 follow-up checkpoint，最多按 policy 重试两次。
- 最后一次 follow-up 仍为 `insufficient_data` 时，action 以 `insufficient_data` 关闭，但不得生成可影响 growth scoring 的方向性 learning。
- `mixed` 表示不同 primary/guardrail metrics 方向不一致，必须保留各指标结果与解释。
- `inconclusive` 表示数据量足够，但观察到的变化无法可靠归因或未超过 decision threshold。

每个 Growth Action 在进入 `measuring` 前必须绑定 versioned measurement policy：

```text
policy_version
early_signal_offset
primary_checkpoint_offset
follow_up_offsets[]
max_follow_up_attempts
max_measuring_duration
minimum_sample/evidence requirements
terminalization_grace_period
absolute_terminal_at
```

- 所有 offsets、`max_measuring_duration` 和 `terminalization_grace_period` 必须是有限值；不得创建无限期 Measuring action。
- `max_measuring_duration` 不得早于最后一个允许的 follow-up checkpoint，并由 action family policy 参数化，而不是散落在 scheduler 代码中。
- action 首次进入 Measuring 时必须计算并持久化 immutable `absolute_terminal_at = measuring_started_at + max_measuring_duration + terminalization_grace_period`。普通 policy upgrade、checkpoint retry、provider outage 或 scheduler delay 不能向后移动该时间；需要继续观察时必须先 terminalize 当前 action，再以有审计的新 hypothesis/action 开启新窗口。
- 到达 `max_measuring_duration + terminalization_grace_period` 后仍无足够数据时，action 必须以 `insufficient_data` 关闭并生成 measurement-quality record。
- checkpoint 延迟、跳过、重试或 policy upgrade 都必须留下原因和原 policy version；不能静默延长 Measuring。

每个 `positive | negative | mixed | inconclusive` terminal outcome 必须生成 learning record：

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

`insufficient_data` 只生成 measurement-quality record，用于改善 coverage、instrumentation 或 window policy；它不能被当成“没有效果”的 growth learning。

现有 legacy outcomes 按原值迁移：`positive`、`negative`、`mixed`、`inconclusive`、`insufficient_data` 不折叠。旧记录没有 checkpoint role 时，最新 completed checkpoint 映射为 primary，其他记录保留为 historical checkpoints。这里的 completed checkpoint 必须是已有 persisted `action_measurements` row，具有 `computed_at`、非空 terminal `outcome_label`，且不再是 measurement policy 中的 `scheduled` placeholder；只有计划日但没有 measurement row 的 checkpoint 不算 completed。

未来 Opportunity scoring 必须读取 learning，而不是每轮从零开始。

## 10. Ownership Arbitration And Deduplication

### 10.1 Candidate before product object

所有 detectors 先产生 internal `candidate`, 不得直接写 Doctor finding、SEO opportunity、Site Fix 或 content action。

Candidate 至少包含：

```text
candidate_id
project_id
target_kind
normalized_target_set
issue_or_hypothesis_family
change_family
proposed_mutations
artifact_intent
intended_slug_or_canonical
topic_entity_identity
audience_identity
primary_success_metric
verification_mode
evidence_ids
suggested_owner
confidence
candidate_schema_version
```

字段语义：

- `normalized_target_set`：排序、去重后的所有受影响 URL/entity/surface；多 URL 合并、redirect、internal-link change 不能只保存一个代表 URL。
- `proposed_mutations`：结构化描述 add/remove/update/move/redirect/link/schema/metadata/content operations，以及 field/selector/source/target。
- `artifact_intent`：`repair_existing_surface | update_existing_content | create_new_asset | consolidate_assets | measurement_only`。
- `intended_slug_or_canonical`：新资产或 URL 变更的目标地址；未知时必须进入 `needs_specification`，不得创建 work。
- `topic_entity_identity`：归一化 topic、product entity、competitor entity 和 target prompt/query cluster。
- `audience_identity`：归一化 persona/market/locale；同一内容面向相同 audience 时参与重复判断。

没有足够 mutation/artifact specification 的 candidate 可以保存 evidence，但不能进入 owner queue。

### 10.2 Owner-neutral work signature

工作去重必须先于 owner 判断。`work_signature` 不包含 Doctor/Opportunities owner、success contract、recommendation 文案或 detector type，否则同一修改会因两条线使用不同成功指标而绕过去重。

确定性 signature input：

```text
project_id
+ normalized target set
+ change family
+ normalized proposed mutation set
+ artifact intent
+ intended slug/canonical
+ topic/entity identity
+ audience/locale identity
+ signature schema version
```

示例：

- `page:/pricing + schema.organization + update:jsonld.organization`
- `page:/pricing + metadata.title + update:title + topic:pricing-software`
- `new:/compare/competitor-x + content.comparison + entity:competitor-x + audience:smb`

系统同时保存：

- `exact_signature_hash`：规范化字段的 deterministic hash；
- `semantic_fingerprint`：对 proposed mutation、artifact spec、topic/entity/audience 生成的 versioned embedding/model fingerprint；
- `conflict_bucket_keys`：按 project + overlapping target/topic/slug + coarse change family 生成的一组稳定 bucket keys；
- `signature_version`：允许未来算法升级和 shadow comparison。

同一 exact signature 只能有一个 active reservation/work item。不同 exact signature 但 semantic overlap 超过阈值时，必须进入 conflict resolution，不能直接创建。

### 10.3 Semantic conflict resolver

Arbitration 必须执行：

1. exact signature dedupe；
2. same target + overlapping change-family detection；
3. intended slug/canonical、topic/entity、audience collision detection；
4. AI semantic comparison of normalized proposed mutations and artifact specs；
5. review-memory suppression check；
6. existing active Doctor Site Fix lookup；
7. existing active Opportunity/content action lookup；
8. ownership rule；
9. transactionally reserve、merge evidence、suppress、attach dependency 或 create。

AI semantic comparison可以消耗 token，但必须输出 structured decision：

```json
{
  "decision": "create|merge_evidence|suppress|block_on_other_line",
  "owner": "doctor|opportunities",
  "work_signature": "string",
  "overlaps": ["work-id"],
  "reason": "string",
  "confidence": 0.0
}
```

AI 只提供 advisory semantic judgment，不能代替唯一约束和事务 reservation。Provider/embedding/LLM 调用 **不得发生在持有 database transaction、row lock 或 advisory lock 时**。Arbitration 必须采用两阶段 compare-and-reserve：

#### Phase A：lock 外 semantic preparation

1. 每个 deterministic `conflict_bucket_key` 必须有唯一 registry row 和单调递增 `bucket_version`；不存在时先用独立、短小、idempotent insert materialize version `0`，不得把“row 不存在”当作不可版本化的空 snapshot。
2. 读取当前 conflict buckets 的 immutable snapshot：`bucket_key + bucket_version + active_work_ids + signature/fingerprint versions`。
3. 在无数据库事务/锁的情况下计算 candidate semantic fingerprint，并只对 snapshot 中可能 overlap 的 work/review-memory 执行 AI comparison。
4. 保存 versioned structured decision、compared work IDs、snapshot bucket versions、provider/model/prompt version 与 AI call record。
5. 任何 provider failure、低 confidence 或 evidence/specification 缺失都进入对应 hold state，不开启 reserve transaction。若 deterministic policy 证明 overlap set 为空、不需要 provider call，则记录 `deterministic_safe` disposition。

#### Phase B：短事务 deterministic recheck + reserve

1. 开启短事务，按稳定顺序取得全部 `conflict_bucket_keys` 的 row/advisory locks。
2. 重新读取 bucket versions、active exact/semantic registry rows、slug/topic/audience collision 和 review-memory aliases。
3. 如果 bucket version、overlap set、signature version 或 relevant evidence fingerprint 与 Phase A snapshot 不一致，立即 rollback；释放锁后使用新 snapshot 重跑 Phase A。锁内不得调用 AI。
4. 只有 snapshot 仍然有效、deterministic checks 无冲突且 Phase A decision 达标时，才能在同一事务中 reserve 并创建 Doctor finding（及 policy 已批准的 Site Fix）或带 AI provenance、状态为 `needs_decision` 的 Opportunity。此事务不得创建 Growth Action 或 Content Brief。
5. reserve/merge/suppress/dependency 写入后递增所有相关 bucket versions，再 commit。
6. Opportunity 被接受后，下游创建事务必须重新取得相关 bucket locks、验证 accepted source Opportunity 与 reservation，之后才可创建 Growth Action 或 Content Brief。

Snapshot mutation discipline：任何能够改变 Phase A snapshot input 的 writer，包括 active-work membership/status、signature/fingerprint version、relevant evidence fingerprint、review-memory alias/decision、target/topic/slug identity 或 bucket membership，都必须按相同稳定顺序取得相关 bucket locks，并在同一事务中更新数据与递增所有相关 `bucket_version`。不得存在绕过 bucket lock/version 的后台 backfill、status transition、migration 或 admin writer；否则 Phase B recheck 不构成并发保证。

Operational requirements：

- confidence `< 0.80`、模型意见冲突或 evidence 不完整时，candidate 进入 `needs_arbitration_review`，fail closed，不创建 work。
- AI/provider unavailable 时，exact signature、target/change-family、slug/topic/audience deterministic checks 仍执行；存在可能 overlap 时 fail closed。
- 只有不存在 deterministic conflict，且任何 required semantic comparison confidence 达标时才能 reserve；deterministic policy 证明无 possible overlap 时可以不调用 AI，但必须记录规则版本和 disposition。
- reservation 与 source record 创建必须在同一数据库事务中完成：Doctor 路径创建 finding（以及 policy 已批准的 Site Fix），Opportunities 路径只创建 `needs_decision` Opportunity。Growth Action 或 Content Brief 必须等待 acceptance，并在独立的 accepted-source transaction 中创建。
- unique conflict 时读取现有 reservation 并 merge evidence，不重试创建第二条 work。
- 事务内只允许 deterministic recheck、snapshot validity check 和持久化；semantic provider call 永远在锁外。仅对 exact hash 加 unique constraint 仍不足以防止两个不同 hash 的语义等价工作并发创建，因此 bucket version compare-and-reserve 是强制要求。
- 多 target candidate 必须一次取得全部 bucket locks；取得失败时 retry 整个 arbitration，不能只创建部分 work。
- transaction/lock duration、snapshot-invalid retry rate、provider latency 和 bucket contention 必须分别监控，不能把 AI latency 算作 lock duration。

Active reservation statuses：

```text
reserved | proposed | approved | preparing | executing | awaiting_deploy |
verifying | measuring | blocked | watching | snoozed | failed_retryable | reopened
```

`verified | learned | dismissed | superseded | cancelled | failed_terminal` 退出 active uniqueness，但 review memory 继续生效。

### 10.4 Review memory

用户决策绑定 owner-neutral work signature，而不是旧 Doctor/Opportunity object ID：

```text
work_signature
exact_signature_hash_at_decision
semantic_fingerprint_at_decision
conflict_bucket_keys
signature_version
signature_aliases
decision = dismissed | snoozed | watching
decision_scope
evidence_fingerprint_at_decision
snoozed_until
material_change_policy_version
decided_by
decided_at
```

- `dismissed`：相同 evidence fingerprint 永久 suppress，直到发生 material evidence change。
- `snoozed`：到期前 suppress；到期后重新评估，但不自动 approve。
- `watching`：允许刷新 evidence 和 measurement，不重新进入 decision queue，除非发生 material change。
- material change 必须由 versioned deterministic fields 判定；AI 可以解释变化，但不能单独触发 reopen。
- review-memory lookup 必须使用与 work creation 相同的两阶段 compare-and-reserve、exact alias lookup 和 semantic resolver；AI comparison 在锁外完成，短事务内只验证 bucket snapshot/alias 状态并持久化 decision。不得只按当前 exact signature 查询。
- signature version 升级时，旧 signature 保存为 alias；同一 mutation 的新版本必须继承旧 review decision。
- semantic variant 命中 dismissed/snoozed/watching memory 且 confidence >= 0.80 时继承 decision；低于阈值时进入 `needs_arbitration_review`，不能当作全新 work。
- 旧 Doctor/Opportunity 的 dismiss、snooze、watch 状态必须迁移到该 registry，避免换 owner 后复活。

### 10.5 Cross-line relationship

允许以下关系：

- Opportunity `blocked_by_doctor_finding`；
- Opportunity `blocked_by_site_fix`；
- Doctor finding `provides_evidence_to_opportunity`；
- Doctor verification `unblocks_opportunity`。

每个 relationship 必须标记 `hard_blocker | soft_dependency`。Hard blocker 只能由 overlapping mutation、invalid baseline/measurement、publish precondition 或安全/合规 gate 触发；Soft dependency 可以并行，但必须进入 Growth attribution confounders。用户可以重新评估 blocker 证据，但不能直接 override hard blocker 的唯一性或 measurement-integrity guardrail。

禁止以下关系：

- Doctor finding 自动复制成 growth opportunity；
- Growth opportunity 自动复制成 Doctor Site Fix；
- 两条线对相同 target/change 同时执行；
- 一条线通过不同 action wording 绕过 identity。

### 10.6 Conflict examples

#### Schema missing + AI citation gap

- Doctor 创建 schema Site Fix。
- AI citation Opportunity 保留为独立增长 hypothesis，但在执行 evidence expansion 前可以依赖 schema fix。
- AI Opportunity 不再创建第二个 schema patch。
- 如果 Growth work 只新增不重叠的 evidence content 且当前 baseline 可信，该关系是 soft dependency，可并行；如果两者都修改同一 JSON-LD/answer block，则是 hard blocker。

#### Missing title + low CTR

- title missing 先归 Doctor，修复并验证。
- low CTR Opportunity 标记 `blocked_by_site_fix`。
- Doctor 修复后重新建立 CTR baseline，再决定是否仍需要 growth title experiment。
- 因为两项工作修改同一 title 且缺失 title 会破坏 CTR baseline，这里是不可 override 的 hard blocker。

#### Thin evidence + brand mentioned without citation

- 如果页面已有 supported evidence，只是结构不可抽取：Doctor。
- 如果需要新增证据内容或新的 source-backed asset：Opportunities。
- 两者不能同时创建“增强 evidence block”的相似 action。

#### Internal link zero + ranking cluster opportunity

- 修复 zero/broken inlinks：Doctor -> Site Fix，完成后立即验证 link graph。
- 基于 ranking hypothesis、只改变 links 的 cluster strategy：Opportunities -> Growth Action，以 growth window 验收。
- 两项 pure link-only work 都不创建 Content Brief；只有 separate scope materially creates or refreshes content 时才进入 Content Plan。
- Growth Action 可以消费 Doctor 修复后的 link graph，但不重复修链接。

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

`needs_arbitration_review`、`needs_specification`、`needs_evidence` 和 `migration_review` 也不是新的用户产品线或一级导航：

- `needs_specification`：由 detector/enrichment worker 补齐 target、mutation 或 artifact spec；超时后进入 internal triage。
- `needs_evidence`：由 shared evidence scheduler 获取缺失 baseline/source；只有 evidence 足够后才重新进入 arbitration。
- `needs_arbitration_review`：由内部 SEO/GEO Ops 在统一 `Discovery Ops Review` 面处理 semantic ambiguity、merge/suppress/owner/dependency decision。
- `migration_review`：只在 migration console 中由迁移 operator 处理 legacy ambiguity，迁移结束后不成为常驻产品 queue。

默认用户只能看到“仍在收集证据”或“不确定、未创建工作”的解释性状态；内部 queue 不得泄漏为第三条 discovery 心智。Ops surface 必须展示 queue owner、age、reason、source evidence、candidate diff、suggested decision 和审计记录。

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
Found -> Decided -> Planned -> Created -> Published -> Measuring
      -> Learned | Closed — insufficient data
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
- Growth outcomes：延迟、统计性、positive/negative/mixed/inconclusive/insufficient_data，并展示 checkpoint role。

两类结果可以在同一 Results 页面分区展示，也可以由 Doctor/Opportunities 各自展示详情；数据语义不得混合。

## 13. Data Model Direction

### 13.1 Recommended logical domains

```text
evidence_runs
evidence_observations
ai_call_records
discovery_candidates
work_signature_registry
discovery_review_items

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
migration_batches
migration_ledger
```

`ai_call_records` 是所有 AI 使用的 canonical ledger，不限于 AI-answer observations。每条 provider call 必须记录：

```text
id
project_id
run_id
stage = evidence|doctor_diagnosis|arbitration|fix_generation|verification|
        growth_hypothesis|brief|content_generation|qa|outcome_learning
linked_object_type
linked_object_id
provider
model
prompt_version
request_fingerprint
status = queued|running|ok|partial|failed|skipped
error_code
prompt_tokens
completion_tokens
total_tokens
cost_usd
started_at
finished_at
```

- Observation/candidate/finding/action 上的 token/cost 是 ledger aggregate，不是第二个 truth source。
- Aggregate 必须可由 linked `ai_call_records` 重算；CI/integration tests 校验 aggregate consistency。
- Retry 每次创建独立 call record，并通过 request fingerprint / parent call 关联，不能覆盖失败记录。
- Provider 未调用时写 `skipped` 和原因，不写伪造的零结果 observation。

### 13.2 Physical migration guidance

Canonical target model 确定为 dedicated `site_fixes`。不得把“继续使用 `content_actions` 并增加 product_line”作为最终或并列方案，因为现有 `content_actions.opportunity_id` 非空会让 Doctor 继续依赖 Opportunities。

迁移要求：

1. `seo_doctor_findings` 保留为 Doctor canonical findings；不新增反向 `site_fix_id`。旧 `linked_opportunity_id` / `linked_content_action_id` 只保留 legacy provenance，不作为 canonical relationship。
2. 新建 `site_fixes`，由 `site_fixes.doctor_finding_id` 单向、权威地引用 finding，同时引用 `candidate_id`、`work_signature_id` 和 evidence snapshot，不引用 `seo_opportunities`。一个 Site Fix 恰好属于一个 finding；一个 finding 可以保留 sequential fix attempts/revisions，但同一 work signature 同时只能有一个 active Site Fix。后续 revision 使用 `supersedes_site_fix_id`，不通过 Doctor finding 上的 current pointer 表达。
3. 现有 technical `seo_opportunities` 经过 dry-run owner classification：
   - 未执行且与 Doctor finding 等价：迁为 Doctor finding/review memory，legacy opportunity 标记 `migrated`；
   - 已批准或执行中：先确保存在 canonical Doctor finding，再创建对应 `site_fixes`，保留原 ID mapping；
   - 无法自动判定：进入 migration review，不创建副本。
4. `content_actions` 中 Site Fix 类 row 迁入 `site_fixes`。原 row 在 cutover 前保持只读并写 `canonical_site_fix_id`，API 返回 canonical resource；验证完成后才允许归档。
5. `site_change_applications` 增加 nullable `site_fix_id`，将现有 `content_action_id` 改为 nullable，并增加 one-of-source constraint：恰好一个 `site_fix_id` 或 `content_action_id` 非空。
6. application、PR reconcile、deployment observation、rollback 和 verification 对 Doctor work 统一引用 `site_fix_id`。
7. content/growth actions 保留在 Growth domain；`content_actions` 不再接受 Site Fix asset types。
8. `geo_observations`、GSC/GA4 daily tables、technical checks 作为 shared evidence source。
9. `geo_asset_briefs` 只保留真正的 growth content brief；crawler/schema repair 不进入 asset brief。

Legacy technical action 可能早于 Doctor、没有 `seo_doctor_findings`。此时迁移必须：

1. 创建 trigger=`migration` 的 immutable Doctor migration run；
2. 从 legacy opportunity/action/evidence snapshot 生成 normalized migration finding，保留 `legacy_opportunity_id`、`legacy_content_action_id`、original timestamps、approval source 和 evidence provenance；
3. 使用当前 work-signature algorithm 做 exact/semantic collision check；
4. finding 成功落库后才能创建 `site_fixes.doctor_finding_id`；
5. evidence 不足、target 无法归一化或多个 legacy rows 无法安全合并时，整组进入 `migration_review`，不得创建 orphan Site Fix 或猜测 finding。

Status mapping：

| Legacy content action | Canonical site fix |
|---|---|
| `ready_for_review` | `proposed` |
| `approved` | `approved` |
| `drafting` / repair preparation | `preparing` |
| PR/draft ready | `ready_to_apply` |
| apply started | `applying` |
| applied / PR merged | `awaiting_deploy` |
| deployment observed | `verifying` |
| verified/completed | `verified` |
| failed with retry available | `failed_retryable` |
| failed with retry exhausted/user terminated | `failed_terminal` |

Site Fix 与 registry 必须在同一事务中转换：`failed_retryable` / `reopened` 对应 active registry status 并继续阻止重复 work；`failed_terminal` 才释放 active reservation，同时保留 historical alias 和 review memory。

Migration invariants：

- `legacy technical actions = migrated site_fixes + explicitly archived duplicates + migration_review`，逐项目与全局 row counts 必须守恒。
- 每个 legacy application 必须恰好映射到一个 canonical application source。
- 每个 legacy ID 必须能通过 alias 查询到 canonical Site Fix。
- `legacy dismissed/snoozed/watching decisions = migrated review-memory rows + explicitly resolved duplicate decisions + migration_review`，逐项目与全局 counts 必须守恒。
- rollback 在 cutover window 内恢复 legacy reads/writes，但不得同时允许两套 writer。
- dual-read 可以存在，dual-write 禁止；每个 phase 只有一个 canonical writer。

#### Migration ledger and rollback contract

所有迁移写入必须绑定 immutable `migration_batch_id`，并在 migration ledger 记录：

```text
migration_batch_id
phase/cutover_point
legacy_object_type/id
canonical_object_type/id
operation = create|map|archive|repoint|decision_migrate
before_snapshot_hash
after_snapshot_hash
inverse_operation
rollback_eligibility
created_at
rolled_back_at
```

不得依靠“按时间范围猜测迁移行”回滚，也不得 hard-delete 已产生审计/provenance 的 canonical rows。各 cutover point 的回滚动作：

| Cutover point / 新造状态 | Forward action | Rollback action | Rollback gate |
|---|---|---|---|
| provisional signatures / collision report | 写 shadow registry/report | 删除 batch-scoped shadow rows | 尚未成为 active reservation |
| migration Doctor run/finding | 创建 immutable run/finding 与 legacy mapping | 将 finding 标记 `migration_rolled_back`，释放未使用 reservation，保留 ledger/tombstone | finding 尚无 cutover 后人工审批或执行 |
| canonical `site_fixes` | 从 legacy action 创建 Site Fix | 标记 Site Fix `migration_rolled_back`，恢复 legacy canonical status/pointer | Site Fix 尚无无法逆投影的新 application/revision |
| `site_change_applications` source repoint | `content_action_id -> site_fix_id` one-of-source | 用 ledger 恢复原 `content_action_id` 与 snapshot | application 未产生只存在于新模型的执行状态，或 inverse projector 已验证 |
| review-memory migration | 创建 signature/alias decision row | 停用 batch row并恢复 legacy decision authority | 没有 cutover 后 material-change/user decision；否则必须迁回最新 decision，不得丢失 |
| legacy row archive/canonical mapping | legacy writer off，canonical writer on | 在 write fence 内恢复 legacy reads/writes，撤销 archive/canonical routing | 满足 row conservation，且只有一个 writer |

Rollback procedure：

1. 停止对应 product writer/scheduler，建立 project/batch write fence；等待 in-flight transaction 完成。
2. 验证 ledger 完整、before snapshots 可读、所有 cutover 后写入都可由 versioned inverse projector 无损映射。
3. 若出现无法逆投影的新 Site Fix revision、application、review decision 或 user action，则该 batch 标记 `rollback_blocked_forward_fix_required`；不得以丢数据为代价强制回滚。
4. 在单一 migration transaction/可恢复分片中执行 inverse operations，恢复 legacy authority 后才解除 write fence。
5. 重跑 exact counts、relationship、application one-of-source、review-memory 与 row-conservation checks；任何不变量失败都保持 writer fenced。
6. 回滚后的 canonical rows保留 tombstone/ledger provenance，不能在下一次 migration 被重复计数为新的 legacy work。

Cutover window 内仍然禁止 dual-write。Rollback 可用性必须通过演练证明，不是仅由 feature flag 声明。

### 13.3 Required invariants

1. `work_signature` reservation 在 active work 范围内唯一，并由数据库约束和事务保证。
2. 一个 work item 只能有 `doctor` 或 `opportunities` owner。
3. Doctor Site Fix 不得关联 topic/article creation，除非只是验证目标，不代表内容所有权。
4. Growth Action 不得把 immediate technical acceptance 当作最终 outcome。
5. 每个 action 必须引用 candidate 和 evidence snapshot。
6. 每个 verified/learned outcome 必须引用实际 applied/published artifact。
7. `site_fixes` 必须引用 Doctor finding；Growth Action 必须引用 Growth Opportunity。
8. `site_change_applications` 必须满足 one-of-source constraint。
9. review memory 不因 object owner 或 legacy ID 变化而丢失。
10. review memory 必须跨 signature version、alias 和高置信 semantic equivalent 生效。

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
- pending `needs_specification` / `needs_evidence` enrichment；
- `needs_arbitration_review` 与 `migration_review` triage、age、assignment 和 audited disposition。

Minimum internal/admin endpoints：

```text
GET  /internal/projects/{id}/discovery-review?state=&age=&assignee=
GET  /internal/projects/{id}/discovery-review/{candidateID}
POST /internal/projects/{id}/discovery-review/{candidateID}/resolve
GET  /internal/projects/{id}/migration-review?batch_id=&state=
POST /internal/projects/{id}/migration-review/{itemID}/resolve
```

Resolve 必须要求 expected candidate/bucket version；过期 decision 返回 conflict 并重新 arbitration，不能覆盖更新后的 evidence。

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

### 15.4 Existing discovery settings migration

用户不再看到 `Signal Scan / AI Discovery` 产品模式，但迁移不能静默扩大 provider calls、token spend 或自动化 authority。旧设置迁入内部 capability policy：

| Legacy source mix | `growth_signal_enabled` | `growth_ai_enabled` | `doctor_ai_enabled` | 说明 |
|---|---:|---:|---:|---|
| `all` | true | true | false | 保持旧 Growth AI authority；Doctor AI 是新 authority，等待用户确认 |
| `signal_scan` | true | false | false | 保留用户显式关闭 AI 的意图；用户可在新的 AI assistance 设置中重新开启 |
| `ai_discovery` | false | true | false | 不静默开启 deterministic Signal Scan 或新增 Doctor AI authority |
| missing/default legacy config | true | true | false | Growth 与旧 default `all` 一致；Doctor AI 等待用户确认 |

旧 `ai_discovery_automation` 只映射到 `growth_ai_run_policy`，不得同时赋权 Doctor：

| Legacy value | New `growth_ai_run_policy` | Provider call authority |
|---|---|---|
| `automatic` | `scheduled_only` | 只保留旧 Opportunity Finding scheduled trigger；不新增 Context/content outcome events |
| `semi_automatic` | `on_demand_recommended` | 系统可提示，用户触发后调用；不自动调用 |
| `manual` | `manual_only` | 只有显式用户操作可调用 |
| missing | `on_demand_recommended` | 与旧 default semi-automatic 一致 |

新用户可见配置必须能表达两条线不对称的 authority，不能用一个 Boolean 合并：

- `Doctor AI assistance: on/off`，持久化为 `doctor_ai_enabled`；
- `Doctor AI run policy: automatic/on demand/manual only`，只控制 Doctor diagnosis/fix/verification calls；
- `Opportunities AI assistance: on/off`，持久化为 `growth_ai_enabled`；
- `Opportunities AI run policy: automatic/on demand/manual only`，只控制 growth discovery/generation/learning calls；
- provider budget/cost visibility；
- GSC/GA4 connection and evidence freshness。

Settings summary 可以显示 `AI assistance: Doctor off · Opportunities on` 等 partial state，但每条线必须独立 consent、保存和撤销。关闭某条线不得影响另一条线已授权的 calls；共享 provider credential 不等于共享 execution authority。

迁移前已存在的项目一律不自动获得 Doctor AI provider-call authority。用户在新版设置或 onboarding confirmation 中明确开启 `AI assistance for Doctor` 后，`doctor_ai_enabled` 才能变为 true。迁移完成后创建的新项目可以在清晰披露 provider use、automation policy 和 cost visibility 的 onboarding consent 下默认同时开启两条线的 AI assistance。

Existing projects 的 `doctor_ai_run_policy` 初始为 `manual_only` 且因 `doctor_ai_enabled=false` 不可执行；用户开启 Doctor AI 时必须同时选择或确认其 run policy。

Existing projects 的 legacy `automatic` 只迁移旧 scheduler 实际拥有的 trigger set，例如 daily Opportunity Finding。`context.confirmed`、`content.published`、`measurement.completed` 等 event-driven AI calls 是新增 authority，必须由用户在新版设置中将 Opportunities AI policy 从 `scheduled_only` 明确升级为 `scheduled_and_event`。新项目可以在 onboarding consent 中直接选择该 policy。

Signal Scan 不再是产品模式，但 internal `growth_signal_enabled` 在迁移期保留，以尊重 legacy opt-out。Doctor/Opportunities 的事件触发 precedence：显式 manual request > approved event policy > scheduled policy；任何低 authority trigger 都不能覆盖高限制设置。

迁移 acceptance：同一项目在迁移后允许的 provider call triggers 必须是旧设置允许集合的子集或相等集合；只有用户再次确认设置后才允许扩大。

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
- Exact duplicate active work rate = `active work sharing exact_signature_hash / all active work`，按 rolling 30 days，目标 0。它是 database invariant health alert，不作为证明产品价值的独立 success KPI；任何非零值按 correctness incident 处理。
- Semantic duplicate active work rate = `human-labeled equivalent active work pairs missed by resolver / all labeled equivalent pairs`，按每月 stratified sample，目标 < 1%。
- Cross-line duplicate action rate = `equivalent Doctor/Opportunity action pairs / all cross-line work pairs reviewed`，按 rolling 30 days，目标 0。
- Automatic suppression precision = `correctly suppressed candidates / all automatically suppressed candidates`，provisional launch target >= 95%。
- Duplicate-safety recall = `(correctly suppressed equivalents + correctly fail-closed equivalents) / all labeled equivalent candidates`，provisional launch target >= 95%；进入人工复核不是 suppression 命中，必须单独计算 hold cost。
- Automated disposition coverage = `不需要人工仲裁即可安全 create/merge/suppress/block 的 eligible candidates / all eligible candidates`；目标必须由 historical spike 校准，不能通过扩大 suppression 降低 precision。
- 用户从 finding 到正确目的地的 routing correction rate provisional target < 2%。
- `needs_arbitration_review` hold rate、weekly inflow、open backlog、p50/p95 age、resolution SLA breach 和 re-arbitration rate 必须成为一级 operational metrics。
- `needs_specification` / `needs_evidence` 分别统计 enrichment success、age 和 terminal drop reason，不与 semantic review backlog 混成一个指标。
- Internal hold state 泄漏成新的用户 queue/product line 的数量目标为 0。

`>=95%` 和 `<2%` 是待验证的 provisional targets，不是未经数据支撑的承诺。Phase 0/1 必须用历史 Doctor/opportunity/content-action 真实样本完成 semantic-dedupe spike：

1. 建立带 legitimate dependency negative examples 的人工 gold set；
2. 分别报告 exact、embedding、AI judge、combined resolver 和 fail-closed policy 的 precision/recall/coverage；
3. 报告不同 threshold 下的 review backlog 与 Ops capacity；
4. 冻结 launch threshold、queue SLO 和每周 capacity，并将 dataset/version 记录进 metric definition；
5. 如果无法同时满足 safety target 与可运营 backlog，Phase 1 不得开启 automatic suppression，只能 shadow/hold，并重新设计 resolver。

Semantic duplicate evaluation set 每月必须覆盖：

- single-URL metadata/schema/link changes；
- multi-URL redirect/consolidation；
- 不同 query/prompt 生成相同 proposed page；
- 相同 topic、slug、audience 的新 content；
- immediate 与 delayed success contract 但 proposed mutation 相同；
- 中英文或不同 action wording 的等价工作；
- legitimate dependency pairs，防止错误 suppress。

所有 suppressed candidates 进入 audit sample denominator，不能通过不记录 suppression 来美化 duplicate rate。

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
- hard-blocked Opportunity rate、time to Doctor verified/unblock、blocker 带来的 approval-to-publish delay；
- soft dependency parallel execution rate 与 related attribution-confounder rate；
- measurement coverage；
- positive/negative/mixed/inconclusive/insufficient_data rate；
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
- 给每条 active work 计算 provisional owner-neutral work signature。
- 记录 duplicate/collision report，不改变用户数据。
- 增加 source、owner、verification mode diagnostics。
- 导出 dismissed/snoozed/watching decision memory 与 evidence fingerprints。
- 导出 legacy discovery settings 和实际 provider-call authority。
- 用历史真实 work 构建 semantic-dedupe gold set，运行 precision/recall/coverage/backlog spike，并冻结 launch threshold 与 Ops capacity。

### Phase 1：Shared Candidate And Arbitration

- 引入 internal candidate envelope。
- Signal Scan、AI Discovery、Doctor detectors 先写 candidate。
- 引入 owner arbitration 和 semantic dedupe；AI comparison 在锁外执行。
- 引入 bucket-version compare-and-reserve、短事务 work-signature reservation 和 review-memory registry。
- 建立 internal Discovery Ops Review/API，为 arbitration/specification/evidence hold state 定义 owner、SLO、容量与审计。
- Shadow mode 比较新旧输出，不改变 queue。

### Phase 2：Doctor Owns Site Fixes

- Doctor 创建独立 Site Fix，不再经 `seo_opportunities`。
- 迁移 active technical actions。
- Doctor 增加 proactive Optimization 与 Healthy coverage。
- Site Fix 增加 apply -> deploy -> verify 闭环。
- 显式替换当前 `MarkSiteChangeApplicationAndContentActionVerified` / `sitefix_verify.go` 的 legacy 行为：technical Site Fix verification 不再把 parent `content_action` 推进 `measuring`；canonical Doctor `site_fix` 止于 `verified`，只有真正的 Growth Action 才进入 measurement scheduler。
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
- 按明确 mapping 迁移 legacy discovery settings，不扩大自动化 authority。
- 收紧 DB constraints 和 API contracts。
- 删除不再使用的 routing heuristics 与 source-mode settings。
- 在每个 cutover point 执行 migration-ledger rollback drill；无法无损 inverse-project 的 batch 不允许宣称可回滚。

## 19. Backward Compatibility

1. 已批准、执行中或 measuring 的工作不得静默删除。
2. 已有 Site Fix 优先迁入 Doctor，保留 original opportunity/action provenance。
3. 已有 content/GEO briefs 优先迁入 Opportunities。
4. 无法自动判定 owner 的 active item 进入 migration review，不自动复制。
5. 旧 URL/API 在迁移期可提供 deprecated alias，但返回 canonical object ID。
6. 旧 Activity Log 保留原 agent/source 名称。
7. 历史 Results 不重算，只标记 legacy attribution model。
8. `dismissed`、`snoozed`、`watching` 迁入 owner-neutral review memory；迁移本身不触发 reopen。
9. Snooze expiry、material-change fingerprint 和 previously reviewed evidence 必须保留。
10. Legacy `mixed`、`insufficient_data` measurement outcome 原值保留，不折叠为 positive/negative/inconclusive。
11. Legacy discovery settings 按 15.4 映射，迁移后 provider call authority 不扩大。
12. 历史 source-less Topics 按 Content Plan Section 15.4 继续允许现有记录的 view/edit、cancel/reschedule、draft/generate 和 archive/dismiss；不得 create、clone、backfill、migrate 或 seed 新工作，也不参与新工作 source counts。

## 20. Acceptance Criteria

最终执行与生产验收记录见 [`PRD-CiteLoop-Doctor-Opportunities-Two-Line-Acceptance-Audit.md`](./PRD-CiteLoop-Doctor-Opportunities-Two-Line-Acceptance-Audit.md)。

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
11. 不同 action wording、query 或 prompt 不能绕过 owner-neutral work signature。
12. 创建 work 前必须在同一事务中 reserve signature 并检查两条线的 active registry。

### 20.3 Doctor loop

13. Site Fix applied 后不会直接显示 verified。
14. Verification 必须重新读取对应 evidence 并执行 acceptance tests。
15. Verification failure 会保留 evidence；可重试时进入 `failed_retryable` 或 `reopened` 并继续占用 work signature，只有 retry policy 耗尽或用户终止后才进入 `failed_terminal`。
16. GSC/GA4 可以影响 Doctor coverage、priority 和 context，但不能把 delayed uplift 当作 Doctor completion。
17. Doctor AI output grounded in Context 和 observed page evidence。

### 20.4 Opportunities loop

18. 每个 decision-ready Opportunity 有 hypothesis、baseline、primary metric 和 measurement window。
19. 每个 published/applied Growth Action 能回到其 source evidence 和 Opportunity。
20. Primary/follow-up measurement 到期后生成 positive、negative、mixed、inconclusive 或 insufficient_data outcome。
21. Positive、negative、mixed、inconclusive 生成 growth learning；insufficient_data 只生成 measurement-quality record。
22. 下一轮 scoring 能展示使用了哪些历史 learning。
23. 用户可以从 loop stage 点击到真实 action/content/result。

### 20.5 Scheduling and evidence

24. 相同 source/target/window 的 evidence refresh 不重复执行。
25. AI Discovery automation 不再被 standalone weekly scheduler 绕过项目设置。
26. Provider unavailable 不产生虚假零 citation/zero mention findings。
27. Crawl partial failure 不把未检查页面标为 healthy。
28. 所有 AI calls（包括 diagnosis、arbitration、generation、QA、verification、learning）写入 canonical `ai_call_records`，并可重算 object aggregates。

### 20.6 Migration

29. Existing in-flight work 保留 provenance 和用户决策。
30. Migration dry run 能列出所有 collision、duplicate 和 ambiguous owner。
31. 没有 active work 因迁移被静默丢失或重复执行。
32. Legacy routes 在迁移窗口返回 canonical object linkage。
33. Legacy dismissed/snoozed/watching counts 在 review-memory migration 中守恒，且跨 signature version/alias 生效。
34. 迁移不为任何 existing project 自动开启 Doctor AI provider-call authority。
35. `site_fixes.doctor_finding_id` 是唯一权威关系，不存在可冲突的 Doctor finding current-fix pointer。
36. Doctor 与 Opportunities AI controls 独立展示、持久化和撤销，并正确显示 partial state。
37. 没有 Doctor finding 的 legacy technical action 必须先创建 provenance-complete migration finding，之后才能创建 Site Fix。
38. Evidence job idempotency 包含 collection-spec fingerprint；不同 user agent/dimensions/prompt/provider version 不会被错误合并。
39. AI/embedding/provider semantic comparison 不在 database transaction、row lock 或 advisory lock 内运行；lock duration telemetry 不包含 provider latency。
40. Phase A 后 bucket version/overlap set 变化时，Phase B 必须 rollback 并在锁外重新 comparison，不能使用 stale AI decision reserve。
41. `needs_arbitration_review`、`needs_specification`、`needs_evidence` 和 `migration_review` 都有 internal owner/API/age/SLA/audit，且不成为用户第三条 discovery queue。
42. 每个 Growth Action 有 finite `max_measuring_duration`；到期后必须 terminalize，不能无限停在 Measuring。
43. 每个 migration-derived row 有 batch ledger 与 inverse operation；rollback 后 row conservation、one-of-source、review-memory counts 和 single-writer invariant 仍通过。
44. Doctor citation-readiness optimization 的 `added_propositions` 必须为空；需要把新事实写入页面的 candidate 归 Opportunities。
45. 当前 technical Site Fix 的 legacy `verified -> content_action.measuring` transition 在 Phase 2 被移除；canonical Doctor Site Fix 止于 `verified`。
46. 所有会改变 arbitration snapshot input 的 writer 使用相同 bucket locks，并在同一事务中递增相关 bucket versions；不存在 unversioned snapshot mutation path。
47. `terminalization_grace_period` 有限，首次进入 Measuring 时持久化的 `absolute_terminal_at` 不可被 retry 或 policy upgrade 延后。

### 20.7 New-work source boundary

48. Doctor 与 Opportunities 是唯二用户可见的新工作来源。
49. 每个新 Site Fix 都能追溯到 Doctor finding，并以 immediate verification 作为 success contract。
50. 每个新 Content Brief 都能解析完整 logical provenance chain：source Opportunity ID、source Content Action ID、AI run/model/prompt/evidence provenance、唯一 acceptance timestamp 与 approval source，以及存在时的 internal Topic linkage；Content Plan 不提供 manual Opportunity、Topic 或 Content Brief intake。
51. 用户触发 AI Opportunity Finding 时，系统创建带 AI provenance 的 candidate，并在 acceptance 前不创建 Content Brief。
52. 历史 source-less Topics 按 Content Plan Section 15.4 只允许现有记录的 view/edit、cancel/reschedule、draft/generate 和 archive/dismiss；不得 create、clone、backfill、migrate、seed 新工作或成为新 Brief source。
53. Pure link-only work 依 success contract 路由为 Doctor Site Fix 或 Opportunities Growth Action；两者都不创建 Content Brief，除非另有 materially create/refresh content 的 accepted scope。

### 20.8 Executable Given/When/Then scenarios

#### Concurrent cross-line creation

```text
Given Doctor 和 Opportunities 同时提出相同 URL、相同 schema mutation
When 两个 worker 并发 reserve work signature
Then 数据库只允许一个 active reservation
And 另一个 candidate merge evidence 或进入 dependency
And 不产生第二个 Site Fix/Growth Action
```

```text
Given 两个 worker 同时提出 exact hash 不同、但 target/change bucket 相同的语义等价修改
When 两者并发 arbitration
Then 两者都只在锁外执行 semantic comparison
And 第一个 worker 在短事务中验证 bucket snapshot 后 reserve 并递增 bucket version
And 第二个 worker 在锁内发现 stale bucket version 后 rollback
And 第二个 worker 释放锁、读取新 reservation 并在锁外重新 comparison
And 最终只有一个 active work
```

#### Same content from different prompts

```text
Given 两个 AI providers 从不同 prompts 提议相同 topic、audience 和 intended slug 的 comparison page
When exact hashes 不同但 semantic fingerprint 超过 threshold
Then resolver 只创建一个 Opportunity
And 保留两个 prompts/providers 的 evidence
```

#### Multi-URL mutation

```text
Given 一个 Doctor candidate 修复 A -> B redirect 和一个 Opportunity candidate 合并 A/B 内容
When normalized target sets overlap and both mutate canonical B
Then conflict resolver fail closed
And 用户只能批准一个 canonical work plan
```

#### Low-confidence arbitration

```text
Given AI semantic comparison confidence < 0.80 或 providers disagree
When candidate 与 active work 存在 possible overlap
Then candidate 进入 needs_arbitration_review
And 不创建、发布或自动批准任何 work
```

#### Provider latency does not hold locks

```text
Given semantic provider 延迟 90 秒或 timeout
When candidate 执行 Phase A comparison
Then 不存在持有该 candidate conflict bucket lock 的开放数据库事务
And provider failure 进入 needs_arbitration_review 或 deterministic-safe disposition
And 其他不冲突 candidate 可以继续 reserve
```

#### Hard blocker versus soft dependency

```text
Given Doctor 与 Growth candidate 指向同一页面
When proposed mutation fields 不重叠且当前 Growth baseline/measurement 可信
Then relationship 是 soft_dependency
And 两项工作可以并行
And Doctor change 被记录为 Growth attribution confounder
```

```text
Given Doctor 与 Growth candidate 都修改 title，且缺失 title 使 CTR baseline 无效
When resolver classification dependency
Then relationship 是 hard_blocker
And Growth action 在 Doctor verified 与 baseline 重建前不能执行
And 用户不能 override uniqueness/measurement-integrity guardrail
```

#### Finite measurement terminality

```text
Given Growth Action 已用完 primary 与最多两次 follow-up checkpoint
And 当前时间超过 immutable absolute_terminal_at
When measurement scheduler reconcile
Then action 以 insufficient_data terminalize
And 生成 measurement-quality record
And 不生成方向性 growth learning
```

#### Migration rollback conservation

```text
Given migration batch 创建 Doctor migration findings、site_fixes、application repoints 和 review-memory aliases
When cutover rollback 在无 canonical-only unmappable writes 的 write fence 内执行
Then 每条 ledger operation 执行其 versioned inverse operation
And legacy writer 恢复前不存在 canonical writer
And legacy row counts、application one-of-source 与 user decision counts 仍守恒
And rolled-back canonical rows保留 tombstone/provenance，不能被再次迁移计数
```

#### Review memory preservation

```text
Given 用户曾 dismiss technical Opportunity，evidence fingerprint 未变化
When 该 detector 迁到 Doctor 并再次命中
Then owner-neutral review memory suppress finding from decision queue
And migration 不触发 reopen
```

```text
Given watched/snoozed work 发生 versioned material evidence change
When 重新评估
Then snoozed 未到期仍保持 suppress
And watching 只在 material-change policy 允许时重新进入 decision queue
And 保留 previous decision provenance
```

#### Site Fix row conservation

```text
Given N 条 legacy technical content_actions
When migration dry run/classification 完成
Then N = migrated site_fixes + archived exact duplicates + migration_review
And 每条 site_change_application 恰好有一个 canonical source
And 每个 legacy ID 可解析到 canonical Site Fix
```

#### Settings authority

```text
Given legacy source_mix=signal_scan and AI automation=manual
When 迁移到新 capability policy
Then scheduled/event/manual provider-call authority 不扩大
And Doctor/Opportunities 不会自动调用 AI
```

```text
Given legacy AI automation=automatic 只授权 scheduled Opportunity Finding
When 迁移到 growth_ai_run_policy
Then policy=scheduled_only
And Context/content/measurement events 不得调用 provider
And 只有用户再次确认 scheduled_and_event 后才能新增 event triggers
```

```text
Given 任意 migration 前已存在项目，包括 legacy source_mix=all 或 ai_discovery
When 迁移到新 capability policy
Then doctor_ai_enabled=false
And 只有用户明确确认新版 Doctor AI assistance 后才能开启 provider calls
```

```text
Given migrated project 的 Doctor AI=false、Opportunities AI=true
When 用户打开并保存 Settings
Then UI 显示 partial state 且两个 persisted flags 保持不变
And 用户可以只撤销 Opportunities AI 或只开启 Doctor AI
```

#### Evidence collection identity

```text
Given 同一 URL/window 有 OAI-SearchBot probe、Googlebot probe 和两个不同 GSC dimensions 请求
When scheduler 并发请求 evidence
Then 四个不同 collection-spec fingerprints 分别执行
And 相同 fingerprint 的重复请求只执行一次
And observations 共享 normalized target 但不互相覆盖
```

#### Measurement terminality

```text
Given primary checkpoint outcome=insufficient_data
When follow-up retry quota 未耗尽
Then action 保持 Measuring 且不生成 directional growth learning
When 最后 follow-up 仍 insufficient_data
Then action 以 insufficient_data 关闭并只生成 measurement-quality record
```

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
