# PRD：CiteLoop AI Discovery 后续优化路线图

> 日期：2026-07-15  
> 阶段：Competitive SEO Discovery 后续优化  
> 基线：已完成自动 competitive seed recall、answer observation citation/mention 召回、site discovery、path probes、probe provenance、brief provenance，并已在生产验证 PR #414。  
> 核心方向：让 AI Discovery 主动发现、深挖、排序和学习竞品机会，而不是要求用户手动输入竞品或某个竞品页面 URL。

## 1. 背景

PostSyncer `https://postsyncer.com/tools` 案例暴露的根本问题不是“用户没有输入 URL”，而是 CiteLoop 的 AI Discovery 还不应该把竞品发现责任交给用户。用户真正想要的是：

- 输入自己的产品或 domain；
- CiteLoop 自动理解市场、竞品、邻近工具和 SEO/GEO surface；
- 自动找出像 `/tools`、`/alternatives`、`/compare`、templates、glossary、integration hub 这类高杠杆页面系统；
- 解释为什么这些页面对当前项目有意义；
- 生成 fit 当前项目的 opportunity，而不是机械复制竞品。

截至 2026-07-15，CiteLoop 已经补上了第一层 recall 能力：系统可以从 search evidence、answer observation 的 `CompetitorCitations` / `CompetitorMentions`、已知竞品 domain、未知竞品名称搜索、site discovery 和 path probes 中找到更多 competitive seed URL。这已经显著降低了用户必须手动输入竞品 URL 的概率。

但当前能力仍偏“run 内召回”：它能在一次 discovery 中找到更多候选页面，但还没有形成长期记忆、跨 run 学习、结构化 page intelligence、机会排序和 outcome feedback。因此下一阶段目标不是继续堆更多 query，而是把 AI Discovery 升级成一个真正的 competitive discovery system。

## 2. 产品判断

不应该让用户自己输入竞品或竞品页面 URL，除非这是高级调试或强制覆盖场景。

原因：

1. **竞品发现是 AI Discovery 的职责**：用户通常不知道哪些邻近站点在 SEO/GEO 上表现好，也不知道某个竞品是否有 programmatic SEO 结构。
2. **用户输入会缩小系统视野**：如果只围绕用户已知竞品跑，系统会漏掉 SERP neighbor、adjacent tools、category directories 和 emerging competitors。
3. **高价值机会来自 pattern，不来自单个 URL**：`/tools` 的价值不是一个页面，而是“free tools hub + 多 leaf page + intent capture + internal links”的页面系统。
4. **低 effort 是产品壁垒**：CiteLoop 应该从 domain-first 走向 autopilot-first，用户参与越少、发现质量越高，产品价值越明显。

因此后续优化的 UX 原则是：

- 默认不要求用户输入 competitor URL。
- 允许用户在 advanced/debug 区域添加 seed URL，但它只能是加速器，不是主路径。
- 每次 AI Discovery 都要展示“系统自己发现了什么”，而不是“用户提供了什么”。
- 对没有发现的情况，系统要解释 miss reason，而不是静默返回空 opportunity。

## 3. 目标

### 3.1 产品目标

1. 建立 project-level competitive graph，持续记住竞品、邻近站点、页面簇、archetype 和来源。
2. 主动从 SERP、answer observations、known competitors、topic vocabulary、GSC query clusters 和 prior outcomes 中扩展发现面。
3. 对竞品页面做 deep-dive，抽取页面结构、SEO intent、内链模式、CTA、schema、工具/模板/比较对象和可复刻资产。
4. 将竞品 pattern 转成 project-fit opportunity，并为每条 opportunity 提供 evidence、fit rationale、risk、effort 和 expected output。
5. 建立 feedback loop：执行后的 outcome 会反向影响后续 recall、scoring 和 archetype prioritization。

### 3.2 用户目标

1. 用户不需要知道 PostSyncer、Buffer、Hootsuite 或其他竞品具体有哪些页面。
2. 用户能看到 CiteLoop 自动发现的竞品页面系统。
3. 用户能理解“为什么这个竞品 pattern 对我的产品有意义”。
4. 用户能直接 review 可执行的 opportunity，例如“为 UniPost 做 social post formatter tools hub”，而不是“研究 PostSyncer”。

### 3.3 业务目标

1. 提高每次 AI Discovery 生成高质量 opportunity 的概率。
2. 降低空跑、重复跑和低价值 opportunity。
3. 让 CiteLoop 在 cold-start 项目中也能找到初始增长方向。
4. 形成可 dogfood 的 competitive intelligence 能力，支撑 CiteLoop 自身增长和对外定位。

## 4. 非目标

1. 不复制竞品内容、文案、代码、工具实现或品牌资产。
2. 不绕过 robots、登录墙、WAF 或付费数据源限制。
3. 不承诺流量、排名或 LLM mention 一定增长。
4. 不做 Ahrefs/Semrush 级关键词数据库。
5. 不自动发布竞品启发的新页面；发布仍走现有 review / publish / policy gate。
6. 不让用户承担必须输入竞品 URL 的责任。

## 5. 当前基线

当前已具备：

- 从 active prompts 和 public terms 生成 competitive recall queries。
- 从 search results 中识别 high-signal competitive paths。
- 对非 seed homepage 进行 `/tools`、`/alternatives`、`/compare`、`/templates`、`/resources` 等 path probe。
- 从 answer observation 的 competitor citation URL 直接 promotion。
- 从 answer observation 的 competitor citation/mention 名称匹配已知 competitor domain。
- 对未知 competitor citation/mention 名称发起搜索召回。
- 通过 site discovery 从 homepage 或其他页面发现 tools hub 等 seed page。
- 在 evidence、seed report 和 brief 中记录 source/provenance/probe intent。

主要欠缺：

1. 没有持久化 competitive graph，run 与 run 之间缺少记忆。
2. 没有系统级 domain/page/entity 置信度和状态。
3. 没有深挖页面结构，只能粗略识别 archetype。
4. opportunity scoring 仍偏 evidence count/archetype count，缺少 project-fit、risk、effort、novelty 和 outcome history。
5. 缺少“为什么没发现/为什么过滤”的 miss diagnostics。
6. 缺少基于执行 outcome 的学习闭环。

## 6. 核心产品原则

1. **AI first, user optional**  
   竞品发现和深挖默认由 AI Discovery 完成。用户输入 seed URL 是 advanced override。

2. **Recall memory before more recall volume**  
   不只是多搜几个 query，而是把每次发现沉淀成 graph，避免重复、遗漏和短期记忆丢失。

3. **Pattern over page**  
   系统要理解页面系统，例如 tools hub、comparison cluster、templates library，而不是只看一个 URL。

4. **Fit before imitation**  
   竞品做 video downloader，不代表 UniPost 应该做 downloader；系统要抽象 intent，再映射到项目能力。

5. **Evidence and miss transparency**  
   成功发现要有 evidence；被过滤也要有 reason code。

6. **Safe competitive intelligence**  
   只使用公开页面，只保存必要 facts 和 derived intelligence，不保存不必要的页面全文。

## 7. 用户体验要求

### 7.1 默认体验

用户点击 Run AI Discovery 后，不需要输入竞品。

系统自动展示：

- 新发现的 competitor / adjacent domains；
- 新发现的 high-signal URLs；
- 检测到的 page archetypes；
- 生成的 opportunities；
- 被过滤的候选及原因摘要；
- 当前 run 与历史 graph 的关系，例如“新发现”“已知但新页面”“已知页面重新验证”。

### 7.2 Advanced 输入

高级区域允许：

- 添加 seed URL；
- 标记某 domain 为 competitor / not competitor；
- 合并重复 competitor entity；
- 屏蔽不相关 domain；
- 触发 repair run。

但 UI 文案必须明确：这些不是必填项，只是帮助系统更快收敛。

### 7.3 Opportunity 详情

每条 competitive opportunity 必须包含：

- 触发来源：search、answer observation、known competitor、site discovery、path probe、graph refresh；
- 竞品页面 URL 和 canonical URL；
- archetype，例如 tools hub、alternatives page、comparison page、templates library；
- system-observed facts；
- project-fit explanation；
- recommended output；
- risk flags；
- why now；
- evidence freshness；
- 可执行下一步。

## 8. Phase 1：Competitive Graph Memory

### 8.1 目标

把 AI Discovery 从单次 run 的候选 URL 列表升级为 project-level competitive graph。

### 8.2 功能需求

系统需要持久化以下实体：

1. **Competitive Entity**
   - name
   - aliases
   - normalized domain list
   - entity type：direct competitor、adjacent tool、directory、publisher、unknown
   - confidence
   - first seen / last seen
   - source counters
   - user override state

2. **Competitive Domain**
   - host
   - canonical host
   - robots status
   - sitemap status
   - crawlability
   - classification
   - confidence
   - blocked/dismissed state

3. **Competitive Page**
   - canonical URL
   - title/meta
   - archetype candidates
   - discovered from URL/query/entity
   - first seen / last checked
   - crawl status
   - normalized evidence snapshot

4. **Competitive Relationship**
   - entity → domain
   - domain → page
   - page → discovered page
   - entity/page → project topic
   - page → opportunity

### 8.3 状态模型

Competitive entities 和 pages 至少支持：

- `candidate`
- `confirmed`
- `rejected`
- `ignored`
- `stale`

用户 override 优先级高于 AI confidence，但系统仍可记录 ignored candidate 的新 evidence，用于以后解释。

### 8.4 验收标准

1. 同一竞品在多次 run 中不会重复生成多个独立 entity。
2. PostSyncer 第一次被发现后，后续 run 可以从 graph 中继续刷新它，而不依赖再次搜索命中。
3. Run detail 可以展示“本次新增 / 已知刷新 / 被过滤”的 competitive graph diff。
4. 用户可将某 domain 标记为 not competitor，后续不会再生成该 domain 的 competitive opportunity。

## 9. Phase 2：SERP + LLM Surface Cross-check

### 9.1 目标

让 discovery 不只依赖现有 prompts 或 answer observation，而是主动用 SERP 和 LLM surface 交叉验证市场邻居。

### 9.2 输入来源

- product profile terms；
- GSC query clusters；
- active topics；
- existing opportunities；
- known competitor names/domains；
- answer observation competitor mentions/citations；
- prior competitive graph；
- successful outcome patterns；
- category seed query families。

### 9.3 Query families

每个项目根据 stage 和 vertical 生成少量高价值 query：

- `best <category> tools`
- `<category> alternatives`
- `<competitor> alternatives`
- `<competitor> vs <competitor>`
- `free <workflow> tool`
- `<persona> <workflow> template`
- `<platform> <job-to-be-done> integration`
- `<category> API`
- `<category> automation`
- `<workflow> checklist`

Query 必须记录：

- source term；
- intent；
- expected archetype；
- budget group；
- generated reason；
- sensitive-term filter status。

### 9.4 LLM surface cross-check

对 selected prompts，系统继续观察 answer providers，但输出要进入 graph：

- competitor mentions；
- competitor citations；
- citation URL domains；
- recurring entities；
- project absent but competitor present patterns。

### 9.5 Budget 要求

1. competitive discovery 使用独立 budget bucket，不能无声挤占普通 search evidence。
2. 每个 run 有 hard cap。
3. graph refresh 使用低频 budget，不必每次 full crawl。
4. Cold-start 项目优先 recall；已有丰富 graph 的项目优先 refresh stale/high-score nodes。

### 9.6 验收标准

1. 对 UniPost cold-start run，不输入竞品 URL 也能生成包含 social media tools / alternatives / API / templates 的 recall queries。
2. 如果 PostSyncer 在 answer observation 只以 mention 出现，系统能搜索并加入 graph。
3. 如果某 query 多次只返回 directories/media，系统会降低该 query family 优先级。
4. Run detail 显示 budget 消耗和 query reason。

## 10. Phase 3：Auto Deep-dive Page Intelligence

### 10.1 目标

对 high-confidence competitive pages 做结构化 deep-dive，把页面从“URL”升级成“可理解的 SEO/GEO pattern”。

### 10.2 抽取内容

对每个被选中的 competitive page，抽取：

- page title/meta/canonical；
- headings outline；
- internal links grouped by path pattern；
- CTA placement 和 CTA type；
- structured data types；
- visible module pattern；
- list/table/card count；
- related tools/templates/comparison entities；
- topical terms；
- freshness signal；
- monetization signal；
- risk flags。

### 10.3 Archetype 分类

支持以下 archetypes：

- tools hub；
- tool leaf page；
- alternatives page；
- comparison page；
- templates library；
- checklist/resource page；
- glossary cluster；
- integration hub；
- API docs / developer landing；
- scheduler/workflow page；
- directory/media article。

### 10.4 Pattern summary

系统需要生成 derived pattern，而不是保存整页内容。例如：

```text
PostSyncer /tools:
- archetype: tools_hub
- structure: hero + category grid + 100+ tool leaf internal links
- intent: free social media utility search
- reusable pattern for UniPost: developer/marketer-friendly social content utilities
- avoid: downloader-style tools that do not fit UniPost positioning
```

### 10.5 验收标准

1. `https://postsyncer.com/tools` 被 deep-dive 后，系统能识别 tools hub 和 leaf-page cluster。
2. 系统能区分“可复刻 pattern”和“不适合复制的具体工具类型”。
3. Deep-dive result 可被 opportunity materializer 使用，不需要重新抓取页面。
4. 被 robots 禁止或抓取失败的页面会记录 miss reason。

## 11. Phase 4：Opportunity Fit Scoring

### 11.1 目标

让 competitive opportunity 不只是“发现了一个竞品页面”，而是“这是一个适合当前项目做的增长动作”。

### 11.2 Scoring 维度

每个候选 opportunity 至少计算：

1. **Recall confidence**
   - 来源数量；
   - source diversity；
   - 是否来自 answer observation；
   - 是否来自 graph repeat observation。

2. **Archetype confidence**
   - URL path；
   - page structure；
   - internal link pattern；
   - schema/meta/heading evidence。

3. **Project fit**
   - 与 product profile 的 capability match；
   - 与 active topics 的 semantic overlap；
   - 与 target audience 的匹配；
   - 与 existing content gap 的关系。

4. **Effort**
   - 需要新工具实现；
   - 只需内容页面；
   - 需要 design/engineering；
   - 是否能由现有 generator/publisher 支持。

5. **Risk**
   - trademark risk；
   - thin-copy risk；
   - irrelevant intent；
   - low trust / spam pattern；
   - claims unsupported。

6. **Novelty**
   - 是否已有相似 topic/page；
   - 是否近期已生成类似 opportunity；
   - 是否用户 dismissed 过同类建议。

7. **Outcome prior**
   - 过去类似 archetype 是否带来 impressions、clicks、mentions 或 accepted rate。

### 11.3 Opportunity 输出类型

支持输出：

- build tools hub；
- create tool leaf page；
- create alternatives page；
- create comparison page；
- create templates library；
- create integration page；
- update existing page；
- add internal link cluster；
- monitor competitor only；
- dismiss/no action。

### 11.4 验收标准

1. PostSyncer `/tools` 不会直接生成“复制 PostSyncer downloader tools”的机会。
2. 对 UniPost，应生成更 fit 的机会，例如 social post formatter、caption helper、UTM builder、blog-to-social generator、API use-case tools hub。
3. 每个 opportunity 都有可解释 score breakdown。
4. 低 fit 或高 risk 的 candidate 会进入 monitor/dismiss，而不是污染 review queue。

## 12. Phase 5：Closed-loop Learning

### 12.1 目标

让 AI Discovery 从已执行动作中学习，逐步知道哪些 competitive patterns 对某类项目真正有用。

### 12.2 Feedback sources

- 用户 accept/dismiss；
- opportunity 被转成 content action；
- draft 是否通过 review；
- publish 是否成功；
- GSC impressions/clicks；
- LLM/answer citation observations；
- internal link/crawl/indexing status；
- manual override。

### 12.3 学习行为

系统根据 feedback 调整：

- query family priority；
- domain confidence；
- archetype priority；
- opportunity type ranking；
- duplicate suppression；
- risk thresholds；
- project-fit mapping。

### 12.4 验收标准

1. 用户连续 dismiss 某类 competitor pattern 后，系统降低同类机会出现频率。
2. 某类 tools hub 机会执行后表现好，后续相似项目会更优先考虑该 archetype。
3. outcome 不足时系统只降低信心，不把缺数据当成失败事实。
4. Run detail 可以解释“为什么这次推荐不同于上次”。

## 13. Diagnostics：Miss Reason 和 Repair Run

### 13.1 目标

当系统没有生成预期 opportunity 时，能解释卡在哪一步。

### 13.2 Miss reason 分类

至少支持：

- no relevant query generated；
- search budget exhausted；
- search provider unavailable；
- candidate domain classified irrelevant；
- robots blocked；
- fetch failed；
- no sitemap；
- no high-signal path discovered；
- archetype confidence too low；
- project fit too low；
- duplicate existing opportunity；
- user ignored domain；
- risk too high；
- materializer rejected。

### 13.3 Repair Run

Advanced 用户可以触发 repair run，但 repair run 的目标不是手动输入替代 AI，而是定位系统为什么漏。

Repair run 支持：

- 输入 expected domain 或 URL；
- 显示该 URL 是否已在 graph；
- 重新执行有限 budget 的 fetch/deep-dive；
- 输出 miss reason；
- 如果通过校验，加入 graph 并重新 materialize。

### 13.4 验收标准

1. 如果用户问“为什么没有发现 PostSyncer /tools”，系统能显示具体阶段。
2. Repair run 的结果会进入 graph，不只是一次性调试输出。
3. Repair run 不绕过 safety/risk gate。

## 14. 数据与隐私要求

1. 只抓取公开可访问页面。
2. 尊重 robots 和 rate limit。
3. 默认保存 derived facts、metadata、hash 和 snippets，不保存整页 HTML。
4. 用户 override、dismiss 和 ignored domain 是 project-scoped。
5. Competitive graph 不跨客户泄漏具体项目数据。
6. 跨项目学习只能使用 anonymized archetype-level aggregate。

## 15. Observability

每个 AI Discovery run 需要记录：

- query count；
- search result count；
- new entity count；
- refreshed entity count；
- new page count；
- deep-dive count；
- filtered candidate count by reason；
- generated opportunity count；
- accepted/dismissed downstream outcome；
- budget consumed；
- provider/crawler failures。

Run detail UI 至少展示：

- Discovery funnel；
- Top new competitive entities；
- Top pages by score；
- Miss reason summary；
- Opportunity score explanation；
- Graph diff。

## 16. Success Metrics

### 16.1 Recall metrics

- cold-start run 中发现至少 3 个 relevant competitive/adjacent domains 的比例；
- high-signal competitive page discovery rate；
- duplicate candidate collapse rate；
- miss reason coverage。

### 16.2 Quality metrics

- competitive opportunity accept rate；
- dismissed as irrelevant rate；
- generated opportunity to content action conversion；
- review pass rate；
- risk rejection rate。

### 16.3 Outcome metrics

- published competitive-inspired assets count；
- GSC impressions/clicks after observation window；
- answer-provider citation/mention lift；
- internal link/indexing success；
- user-reported usefulness。

## 17. Rollout Plan

### Phase 1A：Graph schema and persistence

建立 competitive entity/domain/page/relationship 的最小数据模型，把现有 run 内 evidence 写入 graph。

### Phase 1B：Graph-backed recall

让 AI Discovery 从 graph 读取 known entities/pages，优先刷新 stale/high-confidence nodes。

### Phase 2A：Cross-check query planner

将 query families、budget group、query reason 和 source term 结构化。

### Phase 2B：LLM observation to graph

把 answer observation mentions/citations/cited domains 全部汇入 graph，并做 entity merge。

### Phase 3A：Page deep-dive extractor

对 selected competitive pages 抽取结构化 pattern summary。

### Phase 3B：Archetype classifier

将 tools hub、alternatives、comparison、templates、resources 等 archetype 统一分类。

### Phase 4A：Fit scoring

加入 project-fit、risk、effort、novelty score，并在 opportunity brief 中展示 score breakdown。

### Phase 4B：Opportunity materializer upgrade

把 page pattern 转成具体 output type 和 action brief。

### Phase 5A：Feedback ingestion

接入 accept/dismiss/publish/outcome 信号。

### Phase 5B：Learning loop

用 outcome history 调整 recall/scoring/archetype priority。

### Phase 6：Diagnostics and repair run

补齐 miss reason、repair run 和 advanced debug UI。

## 18. Dogfood Scenario：UniPost / PostSyncer

目标行为：

1. 用户不输入 PostSyncer。
2. AI Discovery 通过 SERP、answer observation mention/citation 或 adjacent query 发现 PostSyncer。
3. 系统将 `postsyncer.com` 加入 competitive graph。
4. 系统发现 `https://postsyncer.com/tools`。
5. Deep-dive 识别 tools hub 和 tool leaf cluster。
6. Fit scorer 判断 UniPost 不应复制 downloader 类工具。
7. Materializer 生成更适合 UniPost 的 opportunity：
   - social post formatter；
   - caption generator；
   - blog-to-social repurposer；
   - UTM/social link builder；
   - API payload/example generator；
   - social media calendar/template hub。
8. Opportunity detail 展示 PostSyncer evidence、抽象出的 pattern、UniPost fit rationale 和 risk guard。
9. 如果没有生成 opportunity，run detail 明确显示 miss reason。

## 19. 验收总标准

1. 新项目不输入竞品 URL，也能发现 relevant competitor/adjacent domains。
2. 已发现 competitor 会被记入 graph，并能跨 run 刷新。
3. `CompetitorMentions`、`CompetitorCitations`、search result、site discovery、path probe 都能归入同一 graph。
4. 高价值页面被 deep-dive 后能产出结构化 pattern summary。
5. Opportunity 不复制竞品，而是映射到项目能力和 audience。
6. 用户可以看到 discovery funnel、score breakdown 和 miss reasons。
7. 用户 dismiss/accept/outcome 会影响后续推荐。
8. PostSyncer `/tools` 是固定 regression scenario。

## 20. Implementation Notes

后续执行应坚持小 PR 切片，每个 phase 都必须能独立测试和生产验证。

建议顺序：

1. 先做 graph persistence，不急着加更多 crawler。
2. 再把现有 evidence 写入 graph，保持行为不变。
3. 再做 graph-backed recall，降低重复搜索。
4. 再做 deep-dive extractor。
5. 再做 fit scoring 和 opportunity materializer。
6. 最后做 learning loop 和 diagnostics UI。

每个 PR 都应包含：

- 红测；
- targeted tests；
- `go test ./internal/opportunityfinding -count=1`；
- 相关 package tests；
- production smoke；
- PostSyncer/UniPost fixture 或等价 regression。

