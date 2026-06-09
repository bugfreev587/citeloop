# PRD：CiteLoop Dashboard UX 整改

> 日期：2026-06-09
> 状态：Draft
> 范围：Dashboard 产品设计、信息架构、页面体验、信息披露策略
> 参考：SuperX app context 与公开产品叙事

## 0. 摘要

CiteLoop 当前 dashboard 的主要问题不是功能缺失，而是信息架构从系统实现出发，混合了用户工作流、后台自动化、成本审计、SEO/GEO 调试和开发者排障信息。用户进入 dashboard 后，会同时看到 `Knowledge`、`Topics`、`Review`、`Publishing`、`SEO`、`Runs` 等同级入口，但不知道下一步应该做什么，也无法快速判断系统是否正确理解自己的 domain。

本次整改的目标是把 CiteLoop 从“自动化任务控制台”改成“domain 内容运营工作台”。Dashboard 应优先回答用户关心的五个问题：

1. CiteLoop 是否正确理解了我的 domain？
2. 现在最需要我做什么？
3. 哪些内容正在排期、待审核、待发布或待分发？
4. 哪些地方被阻塞了，阻塞原因是什么？
5. 自动化是否健康，是否有需要我介入的异常？

核心改动：

- `Knowledge` 改为 `Context`，作为 CiteLoop 对用户 domain 的产品认知中心。
- `Evidence library` 提升为 Context 的核心主角之一，让“每条声明都有出处”成为 CiteLoop 的差异化体验。
- `Runs` 从一级导航移除，降级为 `Settings > Activity Log` 或高级排障入口。
- Home 重做为 next-action + momentum 工作台，同时回答“现在做什么”和“是否在赢”。
- SEO/GEO 信息统一抽象成 `Visibility`，默认展示机会、风险、结果和置信度，调试细节折叠。
- Visibility opportunity 回流到 Content Plan 的闭环必须被看见、被追踪、被庆祝，而不是隐藏在页面跳转里。
- 所有页面按照“用户结果 -> 可执行动作 -> 证据/细节 -> 高级调试”的顺序组织。

## 1. 背景

CiteLoop 的核心流程是：

```text
Landing URL
  -> Insight: 抓取 domain，生成 Product Profile 和 Content Inventory
  -> Strategist: 生成 topic backlog
  -> Writer + QA: 生成 canonical 与 syndication variants
  -> Review: 人工审核
  -> Publish: canonical 自动发布，variants 人工分发
  -> Visibility: SEO/GEO 监控与机会识别
```

现有前端已经覆盖了很多流程，但页面表达仍然偏工程化：

- Home 里直接出现 `Run Insight`、`Run Strategist`、`Publish tick`。
- Knowledge 页默认展示 `Profile JSON`。
- Runs 页展示 agent、status、cost、model、tokens、error。
- SEO 页同时承载 setup、Search Console、crawler audit、GEO prompts、provider observation、asset briefs 等多个心智模型。

这些信息并非没有价值，但很多不应该在普通用户的默认视野里出现。用户买的是结果，不是后台作业列表。

## 2. 对标 SuperX 的启发

SuperX 和 CiteLoop 的相似点：两者都是 AI 内容/增长工作台，都需要先建立“上下文”，再用上下文驱动生成、排期和自动化。

关键差异：

| 产品 | 绑定对象 | Context 的来源 | 用户最终任务 |
|---|---|---|---|
| SuperX | 用户的 X account | profile、tweets、writing voice、interests、favorite creators、rules | 持续写出符合账号定位的 X 内容并增长 |
| CiteLoop | 用户的 domain | landing page、site pages、product profile、inventory、evidence、competitors、SEO/GEO signals | 持续产出可信、可发布、能提升 SEO/GEO 可见性的内容 |

SuperX 公开产品叙事强调的是“知道今天该发什么”“用自己的 voice 生成”“自动排期”“不 bloated”。它的公开站点把用户任务包装为 Discover、Ready to Post、Create & Schedule、Engage & Grow，而不是把底层任务名暴露给用户。参考：https://superx.so/

SuperX 的 `/context` 页面可访问的 HTML/JS 显示，它把 context 定义为影响 AI inspirations and generations 的设置，并按 About You、Writing Voice、Rules、Products、Interests 等模块组织。参考：https://app.superx.so/context

SuperX Playbook 还强调 context 是保持内容在清晰主题簇里的关键，并把 weekly routine 写成用户能执行的运营习惯。参考：https://superx.so/blog/superx-playbook

CiteLoop 应借鉴的是这种“用户任务语言”和“渐进披露”，不是照搬它的业务内容或视觉风格。

### 2.1 SuperX 的粘性机制

SuperX 不只是把菜单命名得更友好。它真正让用户每天回来的，是把增长变成一个可重复循环：

```text
Inspiration -> Remix -> Publish -> Engage -> Learn -> Repurpose
```

这个循环持续回答两个问题：

1. 今天该做什么？
2. 我是否在赢？

CiteLoop 当前整改已经解决第一个问题，即通过 Home 的 next action 降低迷路感。但如果 Home 只展示待办、待审、待修，用户长期感受到的是负担，而不是产品价值。CiteLoop 必须同样设计一个可感知的循环：

```text
Context -> Content Plan -> Review -> Publish -> Visibility -> New Opportunity -> Content Plan
```

这个 loop 是产品命名 CiteLoop 的核心。Visibility 发现机会、机会进入 Content Plan、内容发布、Visibility 再观察改善，这一闭环必须在界面中被显式呈现。

## 3. 当前问题诊断

### 3.1 信息架构从系统模块出发

当前一级导航把 `Runs` 与 `Review`、`Publishing` 放在同一层级。`Review` 是用户每天需要处理的工作流入口，`Runs` 是后台审计入口。两者同级会让用户误以为每个 run 都需要关心。

问题：

- 用户无法判断哪些页面是日常工作流，哪些是高级调试。
- 导航里缺少“我现在该去哪”的优先级。
- 技术名词占据过多主入口。

### 3.2 Home 不是用户首页，而是任务控制台

Home 当前包含 Pipeline 操作区，以及 `Run Insight`、`Run Strategist`、`Publish tick`。这些是系统内部动作名，不是用户心智里的工作。

问题：

- 新用户不知道为什么要手动点这些按钮。
- `Publish tick` 暴露了 scheduler 概念，用户不应该理解 tick。
- Home 没有明确 next action。

### 3.3 Knowledge 页把内部结构直接暴露给用户

用户需要判断的是“系统是否正确理解我的产品”，而不是编辑 JSON。

当前 `Profile JSON` 的问题：

- 不符合非技术用户心智。
- 无法快速扫描定位、ICP、价值主张、语气、竞品等关键字段。
- JSON 错误会造成保存失败，增加不必要的焦虑。
- 证据 snippets、crawl summary、inventory 分散，缺少“可信上下文”的整体叙事。

### 3.4 Runs 页价值真实，但默认可见性过高

Runs 可以用于审计、排障、成本控制，但普通用户不关心每一次 insight 用了多少 tokens。

应该默认隐藏的信息：

- tokens
- model
- raw agent name
- per-run cost
- raw output
- 成功 run 的完整列表

应该默认显示的信息：

- 自动化是否健康
- 最近是否失败
- 是否被预算或 provider quota 限制
- 是否有用户需要处理的阻塞

### 3.5 SEO/GEO 页面混合过多心智模型

SEO 页目前同时像：

- 设置页
- 数据看板
- 爬虫调试台
- GEO prompt 管理器
- provider observation 控制台
- autopilot 操作台

问题：

- 首屏不知道核心结果是什么。
- 冷启动、缺少 GSC、provider unavailable 等状态容易显得像错误。
- 对普通用户来说，prompts、provider、crawler snapshots 是次级解释，不是主页面内容。

## 4. 产品定位

CiteLoop dashboard 应定位为：

> 一个围绕用户 domain 的 AI 内容运营工作台，帮助用户确认产品上下文、规划内容、审核可信内容、发布与分发，并持续发现 SEO/GEO 可见性机会。

不是：

- 后台 job monitor
- 大屏 analytics dashboard
- crawler debug workbench
- LLM cost explorer
- SEO 专家工具箱

## 5. 目标

### 5.1 用户目标

1. 用户首次进入项目后，能理解如何从 domain URL 建立 Context。
2. 用户能快速确认 CiteLoop 对产品、受众、证据和语气的理解是否正确。
3. 用户每天进入 Home，能直接看到最重要的下一步动作。
4. 用户每天进入 Home，也能看到已经交付的结果和增长势头。
5. 用户能审核内容，并知道为什么某些内容不能 approve。
6. 用户能看到哪些内容已经发布，哪些 variants 等待分发。
7. 用户能理解 SEO/GEO 可见性机会，并能把机会加入内容计划。
8. 用户能看到机会从发现、计划、发布到效果观察的闭环进度。
9. 用户只在异常、预算风险、降级时看到自动化状态。

### 5.2 业务目标

1. 降低 dashboard 首次使用困惑。
2. 提高 Context 完成率。
3. 提高 review queue 处理率。
4. 降低用户对正常后台 run 的误解和焦虑。
5. 提高 Visibility opportunity 转化为 Content Plan 的比例。
6. 提高用户对“CiteLoop 正在交付价值”的感知，降低只看到待办带来的疲劳感。
7. 让产品更接近可销售的 SaaS 工作台，而不是内部 MVP 控制台。

### 5.3 设计目标

1. 信息密度克制，优先使用单列或窄内容流。
2. 所有页面有明确默认决策或推荐动作。
3. 同一页面内不要混合超过两个主要用户心智模型。
4. 系统内部状态必须映射成用户可理解的结果语言。
5. 技术细节通过 progressive disclosure 展示。
6. Loop 进度和结果必须可见，不能只存在于后台数据流。

## 6. 非目标

- 本 PRD 不要求重做后端数据模型。
- 本 PRD 不设计完整计费、团队权限、多租户管理后台。
- 本 PRD 不要求把 SEO/GEO 所有高级能力做成专家级分析工具。
- 本 PRD 不要求视觉品牌完整重塑；本次优先解决 IA、文案和信息披露。视觉精修、品牌系统和更强的 micro-interaction 是后续 follow-up。
- 本 PRD 不要求移除 run 数据，只要求改变默认入口和展示层级。

## 7. 目标用户与 Jobs To Be Done

### 7.1 用户类型

**Founder / Operator**

- 不想看技术日志。
- 想知道 CiteLoop 是否理解产品。
- 想稳定产出内容。
- 关心内容是否可信、是否能发布。

**Content / Growth Marketer**

- 关心选题、排期、审核、分发。
- 需要知道内容与 positioning、ICP、关键词和证据是否一致。
- 需要可解释的 SEO/GEO 机会。

**Technical Admin**

- 关心 integrations、publisher config、notifications、runs、cost、errors。
- 可以进入高级区域，但不应影响普通用户主流程。

### 7.2 核心 JTBD

1. 当我刚创建项目时，我想输入 domain 并确认系统理解了我的产品，这样后续生成不会跑偏。
2. 当我每天打开 dashboard 时，我想知道现在最重要的动作是什么，这样不用扫完整个系统。
3. 当内容生成后，我想快速判断是否能 approve，不能 approve 时知道要修哪里。
4. 当内容发布后，我想知道哪些 variant 可以分发到其他平台。
5. 当系统运行异常时，我想看到影响和处理建议，而不是原始日志。

## 8. 设计原则

### 8.1 Outcome-first

页面先展示用户结果，再展示系统过程。

示例：

- 用 `3 drafts need review`，不要用 `writer run ok`。
- 用 `Context needs refresh`，不要用 `insight run missing`。
- 用 `Publishing is blocked by missing canonical URL`，不要用 `publisher failed with pending_url_verification`。

### 8.2 Default calm, disclose on demand

默认界面应安静。只有失败、阻塞、预算风险、数据不足时才提高视觉优先级。

### 8.3 Default decision, not single command

每个页面首屏必须有一个明确的默认决策或推荐动作，但不能牺牲批处理效率。

说明：

- Home、Context、Publish、Visibility 首屏应有一个主推荐动作。
- Review 和 Content Plan 是队列型/批处理界面，允许多选、批量操作和多个 item-level actions。
- Next action 是建议，不是命令。界面必须解释为什么推荐它，并保留用户改走其他队列的 agency。

### 8.4 Context before generation

没有 Context 或 Context 不健康时，生成动作应降级，引导用户先修 Context。

### 8.5 Human-readable evidence

QA、blocking、claim safety 必须关联到用户能读懂的 evidence，而不是只显示 status code 或 agent error。

### 8.6 Evidence is the product moat

SuperX 的 Context 强调 voice 和 interests。CiteLoop 的 Context 必须更强调 evidence。用户应该持续感受到：

> CiteLoop 不是泛泛生成内容，而是用我的 domain 事实和可追溯证据生成内容。

因此 Evidence library 不是 Source pages 的附属字段，而是 Context 页的一等 section，并且要能被 Review、QA blocking、Visibility opportunities 复用。

### 8.7 Loop is visible

Visibility opportunity 不能只是一个列表项。它应有生命周期：

```text
Detected -> Added to plan -> Drafted -> Published -> Measuring -> Learned
```

每个阶段都要有用户可见状态。完成闭环时，界面应以克制但明确的方式反馈价值，例如：

- `Opportunity converted into 1 published article`
- `AI crawler access issue resolved after robots update`
- `Coverage improved after publishing the comparison page`

### 8.8 Advanced is available, not ambient

高级信息保留，但不能常驻在主流程里。

## 9. 新信息架构

### 9.1 一级导航

建议一级导航：

1. `Home`
2. `Context`
3. `Content Plan`
4. `Review`
5. `Publish`
6. `Visibility`
7. `Settings`

说明：

- `Knowledge` 改为 `Context`。
- `Topics` 改为 `Content Plan`，更符合用户心智。
- `Publishing` 改为 `Publish`，更短，更动作化。
- `SEO` 改为 `Visibility`，承载 SEO + GEO 的用户结果。
- `Runs` 移出一级导航。

### 9.2 二级与高级入口

`Settings` 下包含：

- Project settings
- Publishing connection
- Notifications
- Automation
- Budget
- Activity Log
- Advanced

`Activity Log` 承载原 Runs 页：

- failed/degraded run 默认展开
- successful run 默认折叠
- tokens/model/per-run cost 放到详情抽屉
- 支持按日期、agent、status 筛选

### 9.3 路由迁移

| 当前路由 | 新路由/名称 | 默认可见性 | 说明 |
|---|---|---:|---|
| `/projects/[id]` | Home | 一级 | next-action 工作台 |
| `/projects/[id]/knowledge` | `/projects/[id]/context` | 一级 | Context 中心 |
| `/projects/[id]/topics` | `/projects/[id]/plan` | 一级 | 内容计划与排期 |
| `/projects/[id]/review` | Review | 一级 | 人工审核闸门 |
| `/projects/[id]/publishing` | `/projects/[id]/publish` | 一级 | 发布与分发 |
| `/projects/[id]/seo` | `/projects/[id]/visibility` | 一级 | SEO/GEO 用户结果 |
| `/projects/[id]/runs` | `/projects/[id]/settings/activity` | 高级 | 审计与排障 |
| `/projects/[id]/settings` | Settings | 一级 | 项目配置 |

旧路由应保留 redirect，避免现有链接失效。

## 10. 页面 PRD

## 10.1 App Shell

### 目标

让用户一眼知道哪些入口是日常工作流，哪些入口是设置/高级信息。

### 需求

- Sidebar 使用固定宽度，保持现有 SuperX-inspired 窄工作台方向。
- 一级导航最多 7 个入口。
- `Review` 可以作为 primary CTA，但 CTA 文案应根据状态变化：
  - 有待审核：`Review 3 drafts`
  - 无待审核但 Context 缺失：`Set up Context`
  - 有发布失败：`Fix publishing`
  - 正常：`Open review`
- Sidebar 底部预算只显示状态，不显示复杂费用：
  - `Budget healthy`
  - `Near monthly limit`
  - `Automation paused by budget`
- Advanced 或 Activity Log 入口放在 Settings 或 sidebar footer 的 muted 区域。

### 验收

- 普通用户不会在一级导航看到 `Runs`。
- Sidebar 中没有 tokens、model、deployment、DB migration 等工程信息。
- 移动端导航保留同样的信息层级，不横向堆满所有高级入口。

## 10.2 Home

### 定位

Home 是用户每天进入 CiteLoop 的工作台，不是数据大屏，也不是任务控制台。

### 首屏结构

1. `Next action`
2. `Results / Momentum`
3. `Loop progress`
4. `This week`
5. `Needs review`
6. `Ready to publish/distribute`
7. `Context health`
8. `Automation health`

### Next action

页面顶部展示推荐动作，但不把其他队列藏起来。该模块由三部分组成：

- 推荐动作：系统认为现在最值得处理的一件事。
- Why this：解释为什么推荐它。
- Also waiting：紧凑列出其他等待用户处理的事项。

优先级规则：

1. Context missing 或 stale：引导 `Refresh Context`。
2. Publish failed：引导 `Fix publishing`。
3. QA blocking：引导 `Review blocked drafts`。
4. Pending review：引导 `Review drafts`。
5. Ready to distribute：引导 `Distribute variants`。
6. No content plan：引导 `Generate content plan`。
7. Everything healthy：展示 `Next content slot` 或 `All set for now`。

Next action 可以由后端统一计算，避免前端分散实现。该计算应是可测试的纯规则函数，输出推荐动作、原因和备选等待项。用户可以忽略推荐动作并进入任意队列，界面不应把推荐动作表现成唯一正确路径。

示例文案：

- `Your domain context needs a refresh before generating new content.`
- `3 drafts are waiting for approval.`
- `2 variants are ready to distribute after the canonical article went live.`
- `Publishing is blocked. The GitHub/Next.js connection needs attention.`

Also waiting 示例：

- `2 blocked drafts`
- `1 publish retry`
- `4 open visibility opportunities`

### Results / Momentum

Home 必须回答“我是否在赢”。该区块不做大屏 KPI，而是用克制的结果摘要展示 CiteLoop 已交付的价值。

默认指标：

- published this month
- drafts approved
- variants distributed
- opportunities converted into content
- active loop items

有 verified SEO/GEO 数据时可展示：

- visibility trend
- new AI mentions / citations
- resolved crawler blockers
- Search Console clicks/impressions trend

无 verified 数据时不伪造指标。用 capability-aware 文案：

```text
Search Console is not connected yet. CiteLoop is tracking public crawl and content progress only.
```

Momentum 区块的文案要强调已完成的价值，而不是制造 vanity metrics：

- `5 articles published from domain-backed evidence this month`
- `3 visibility opportunities turned into planned content`
- `2 crawler blockers resolved`

### Loop progress

Home 应展示 CiteLoop loop 的当前状态，让用户看到机会如何回流为内容。

建议显示 3 到 5 个最近 loop items：

| Stage | 用户可见文案 |
|---|---|
| Detected | `Opportunity detected: comparison page missing` |
| Added to plan | `Added to Content Plan` |
| Drafted | `Draft waiting for review` |
| Published | `Canonical published` |
| Measuring | `Visibility impact being measured` |
| Learned | `Coverage improved` 或 `No movement yet` |

Loop item 点击后应进入对应页面或详情：

- Detected -> Visibility opportunity
- Added to plan / Drafted -> Content Plan or Review
- Published -> Publish
- Measuring / Learned -> Visibility

### This week

展示未来 7 天内容节奏：

- scheduled canonical
- empty slot
- pending review items that would fill slots
- published items

不展示：

- run ID
- agent
- tokens
- model

### Needs review

展示最需要用户判断的内容。

卡片字段：

- title
- format/kind
- target keyword 或 target prompt
- QA status
- evidence confidence
- short preview
- primary action: `Review`

QA blocking 的展示要具体：

- `2 claims need evidence`
- `Missing source for pricing claim`
- `Product positioning changed since this draft was generated`

### Ready to publish/distribute

分为两个 lane：

- `Ready to publish`: canonical approved and due
- `Ready to distribute`: variant unlocked after canonical URL exists

如果系统是自动 publish，Home 不展示 `Publish tick`，只展示状态：

- `Scheduled for Jun 12`
- `Publishing now`
- `Published`
- `Needs manual distribution`

### Context health

用轻量 summary 展示：

- last refreshed
- source pages scanned
- evidence coverage
- stale warning
- blocked crawl warning

点击进入 Context。

### Automation health

默认折叠，只在异常时提高优先级。

状态：

- `Healthy`
- `Degraded`
- `Paused by budget`
- `Action required`

展示用户影响：

- `Search provider unavailable. New content plans may be less competitive.`
- `Monthly budget reached. Generation is paused until next cycle.`

不展示：

- 每条成功 run
- tokens
- model
- raw JSON output

### Home 验收

- 用户不需要点击其他页面，也能知道下一步动作和当前势头。
- Home 同时回答“现在做什么”和“是否在赢”。
- Next action 有原因，并展示 also waiting。
- Visibility opportunity 到 Content Plan 的 loop 进度可见。
- Home 首屏没有内部任务名：`Insight`、`Strategist`、`tick`、`run`。
- 成功 run 不占据 Home 的视觉空间。
- 用户只在异常时看到自动化细节。

## 10.3 Context

### 定位

Context 是 CiteLoop 对用户 domain 的产品认知中心。它回答：

> CiteLoop 用什么事实、证据、语气和边界来生成我的内容？

Context 页面可以借鉴 SuperX 的命名，但不能只镜像 SuperX。SuperX 的核心是 voice/context，CiteLoop 的核心是 evidence-backed domain cognition。页面必须让用户感到“系统读了我的站，并且每条可发布声明都有出处”。

### 页面结构

1. Header
   - title: `Context`
   - subtitle: `This is how CiteLoop understands your domain and writes for it.`
   - primary action: `Refresh context`
   - secondary action: `View crawl details`

2. First-run Context confirmation
   - only shown after first successful crawl until user confirms
   - guided review of key facts
   - clear accept/edit actions
   - progress indicator across sections

3. Context health card
   - status: healthy / stale / incomplete / blocked
   - last refreshed
   - source pages
   - evidence coverage
   - recommendations

4. Evidence library
   - supported claim
   - evidence snippet
   - source page
   - confidence
   - last seen
   - used by drafts

5. Domain profile
   - positioning
   - ICP
   - value props
   - product features
   - differentiators
   - competitors
   - key terms

6. Voice & rules
   - tone
   - preferred vocabulary
   - banned claims
   - style instructions
   - content guardrails

7. Source pages
   - landing page
   - crawled pages
   - skipped pages
   - generated content included in inventory

8. Advanced crawl details
   - discovered/fetched/inventory/errors
   - truncation reason
   - robots/rate limit
   - raw crawl warnings

### First-run Context confirmation

这是 CiteLoop 相比 SuperX 的结构性优势：SuperX 需要用户手动描述自己，CiteLoop 可以从 domain 自动生成初始理解。首次成功抓取后，Context 页不应只是静态卡片，而应进入一个引导式确认流程：

```text
We read your domain. Confirm what CiteLoop should use when planning and reviewing content.
```

确认流程包含：

1. `Positioning`
   - 展示系统抽取的定位。
   - 用户可以 accept 或 edit。

2. `Audience / ICP`
   - 展示目标用户、人群和使用场景。
   - 用户可以删除不准确项或新增人群。

3. `Evidence-backed claims`
   - 展示可安全使用的 claims。
   - 每条 claim 必须有 source URL 和 snippet。
   - 用户可以 mark as approved、edit、remove。

4. `Banned / risky claims`
   - 展示系统检测到的不应随意使用的 claims。
   - 用户可以新增 banned claim。
   - 首版必须支持编辑，因为这是信任与合规护栏。

5. `Competitors and alternatives`
   - 展示系统识别的竞品或替代方案。
   - 用户可以新增、删除、改名。

6. `Voice & rules`
   - 展示语气和内容规则。
   - 用户可以编辑。

确认完成后，Home 的 Context health 从 `needs confirmation` 变为 `ready`。如果用户跳过确认，生成动作仍可执行，但必须带 warning：

```text
Context has not been confirmed. Generated drafts may use incorrect positioning or unsupported claims.
```

### Domain profile 编辑体验

不要默认展示 JSON。改为结构化 field groups：

- Positioning: textarea
- ICP: list editor
- Value props: list editor
- Features: list editor
- Differentiators: list editor
- Competitors: tag/list editor
- Key terms: tags
- Tone: textarea
- Banned claims: list editor
- Content rules: list editor

高级 JSON 只在 `Advanced` 折叠里显示。

### Evidence library

证据是 Context 页的核心，不是 inventory 或 Source pages 的附属字段。Evidence library 必须作为独立 section，排序高于 Domain profile 或至少与 Domain profile 平级。

每条 evidence 应展示：

- `Claim`
- `Evidence`
- `Source URL`
- `Used by drafts`
- `Confidence`
- `Approved by user`

支持操作：

- edit evidence
- open source
- mark as outdated
- add note
- approve claim
- remove claim from safe use

### Source pages

Source pages 不是 crawler debug，而是“CiteLoop 读过哪些页面”。

默认展示：

- page title
- URL
- summary
- last crawled
- evidence count

失败和 skipped 页面只在异常时展开。

### Empty states

无 Context：

```text
Start by connecting your domain.
CiteLoop will read your public pages, extract product facts, and build the context used for content planning and review.
```

Context stale：

```text
Your context is older than the latest site changes. Refresh before generating new content.
```

Crawl blocked：

```text
CiteLoop could not read key pages. Generated content may miss important product facts.
```

### Context 验收

- 页面名不再是 `Knowledge`。
- 默认视图没有 `Profile JSON`。
- 首次抓取后进入 Context confirmation flow，而不是只显示静态数据。
- 用户可以在 30 秒内判断产品定位、受众、证据是否正确。
- Evidence library 是独立 section，不内嵌在 Source pages。
- Competitors、banned claims、positioning 首版可编辑。
- Crawl summary 不再独立漂浮，而是服务于 Context health。
- Advanced JSON 和 raw crawl details 默认折叠。

## 10.4 Content Plan

### 定位

Content Plan 管理选题、内容节奏和生成意图。它不是 agent 输出列表。

### 结构

1. Plan health
   - backlog count
   - scheduled this week
   - missing slots
   - stale because Context changed

2. Generate plan
   - primary action: `Generate content plan`
   - disabled or warned when Context incomplete

3. Schedule
   - week grouped rows
   - empty slots visible
   - drag/drop optional future enhancement

4. Backlog
   - grouped by intent or priority
   - filters: status, channel, priority, scheduled/unscheduled

5. Topic detail drawer
   - title
   - angle
   - target keyword/prompt
   - evidence sources
   - related inventory
   - generate draft

6. Visibility-sourced opportunities
   - opportunities accepted from Visibility
   - source signal
   - expected impact
   - loop stage
   - linked measurement after publishing

### Naming

- `Run Strategist` -> `Generate content plan`
- `Generate selected topic` -> `Draft this topic`
- `Archive topic` -> `Remove from plan`

### 验收

- 用户知道每个 topic 为什么存在。
- Topic 与 Context/evidence 有可见关联。
- 从 Visibility 加入的 opportunity 必须带来源和 loop stage。
- 生成前有 Context health 提示。

## 10.5 Review

### 定位

Review 是唯一人工审核闸门。它应该让用户快速判断“能不能发布，不能发布要修哪里”。

### 结构

1. Review summary
   - pending
   - blocked
   - ready to approve
   - edited recently

2. Queue grouped by content bundle
   - canonical first
   - variants below
   - same topic grouped

3. Article review panel
   - content preview/editor
   - SEO metadata
   - QA status
   - evidence checklist
   - approve/reject actions

### QA blocking 表达

不要只显示 `qa_blocking=true`。

应该显示：

- blocking reason
- affected claim
- missing or weak evidence
- suggested fix

示例：

```text
Cannot approve yet
The draft claims "SOC 2 ready" but Context has no supporting source.
Add evidence in Context or edit the claim.
```

### 验收

- Blocking 状态不能被误解成系统错误。
- Approve 禁用态旁边有原因和下一步。
- 用户能从 blocking issue 跳到 Context evidence。

## 10.6 Publish

### 定位

Publish 展示 canonical 和 syndication variants 的发布/分发状态。

### 结构

1. Publish status
   - connected / needs setup / failed
   - next scheduled publish

2. Published canonical
   - title
   - URL
   - published date
   - status

3. Ready to distribute
   - platform
   - content preview
   - copy
   - open compose
   - mark distributed

4. Waiting on canonical
   - variants that cannot unlock yet
   - reason

5. Failures
   - impact
   - retry action
   - connection/settings link

### Naming

- `Reconcile` -> `Check publish status`
- `Retry` -> `Retry publishing`
- `Mark distributed` 保留，但可加确认反馈

### 验收

- 用户知道 canonical 与 variants 的关系。
- 缺少 canonical URL 时，原因显示清楚。
- Publish failure 不需要用户去 Runs 页找原因。

## 10.7 Visibility

### 定位

Visibility 是 SEO + GEO 的用户结果层。它回答：

> 我的 domain 在搜索和 AI answer surfaces 上是否更容易被发现？下一步应该做什么？

页面命名为 `Visibility`，但首屏必须清楚标明它覆盖 `SEO + GEO visibility`，避免用户改名后找不到既有 SEO 能力。

### 结构

1. Visibility overview
   - mode: public-only / connected / limited
   - confidence
   - top opportunities
   - major blockers

2. Search visibility
   - clicks/impressions only when verified
   - unavailable metrics 不伪造
   - setup guidance if GSC missing

3. AI visibility
   - crawler access status
   - answer coverage
   - surfaced opportunities
   - confidence

4. Opportunities
   - issue
   - impact
   - recommended content action
   - accept/dismiss

5. Loop closure
   - detected opportunities
   - opportunities added to Content Plan
   - published content linked to opportunity
   - measurement status
   - result / no movement / needs more data

6. Advanced diagnostics
   - crawler snapshots
   - prompts
   - provider runs
   - raw observations

### 数据可见性

默认显示：

- `AI crawlers can access 8/10 key pages`
- `3 content gaps found`
- `Search Console not connected. Using public crawl only.`

折叠显示：

- prompt sets
- provider observations
- raw robots results
- HTTP status details

### Opportunity lifecycle

Opportunity 是 Visibility 页最重要的对象，不只是建议列表。每个 opportunity 都要有 lifecycle：

```text
Detected -> Added to Content Plan -> Drafted -> Published -> Measuring -> Learned
```

默认字段：

- opportunity title
- source signal: SEO / GEO / crawler / competitor / evidence gap
- user impact
- recommended action
- status in loop
- linked topic/article
- measurement state

主动作：

- `Add to Content Plan`
- `Dismiss`
- `Mark not relevant`

加入 Content Plan 后，Visibility 页面不应只显示“accepted”，而要继续显示后续状态：

- `Planned`
- `Draft waiting for review`
- `Published`
- `Measuring impact`
- `Improved`
- `No movement yet`

### Loop celebration

当 opportunity 完成闭环时，界面需要有克制但可见的正反馈。不是大型庆祝动画，而是让用户感到 CiteLoop 交付了结果。

示例：

```text
Loop closed
The "AI crawler blocked on docs pages" opportunity was resolved. 8/8 key pages are now accessible.
```

```text
Opportunity converted
"Comparison page missing" became a published canonical article and is now being measured in Visibility.
```

### Capability-aware SEO/GEO labels

Visibility 比 `SEO` 更抽象，因此页面必须在标题、副标题和 tab label 上持续解释范围：

- title: `Visibility`
- subtitle: `SEO and AI-answer visibility for your domain`
- tabs: `Search`, `AI Answers`, `Opportunities`, `Diagnostics`

无第一方数据时，不显示虚假趋势。应显示 capability mode：

- `Public crawl only`
- `Search Console not connected`
- `Provider unavailable`
- `Insufficient answer coverage`

### 验收

- 用户不会在首屏看到 provider/debug 概念。
- 用户能识别这是 SEO + GEO 的用户结果页。
- 无第一方数据时，页面解释能力限制，而不是展示 404 或空指标。
- Opportunity 能直接转化为 Content Plan 或 Context 修正。
- Opportunity 的后续状态能回到 Visibility 被追踪。
- Loop closed / opportunity converted 这类结果有可见反馈。

## 10.8 Settings

### 定位

Settings 是配置，不是运营首页。

### 信息分组

1. Project
   - domain
   - cadence
   - budget
   - crawl bounds

2. Publishing
   - GitHub/Next.js connection
   - base URL
   - content directory
   - publish mode

3. Notifications
   - Slack/Discord webhook
   - event subscriptions

4. Automation
   - safe mode
   - schedule
   - manual pause/resume

5. Activity Log
   - runs
   - webhook deliveries
   - errors

6. Advanced
   - raw config
   - API/debug IDs

### 验收

- 日常用户不需要进入 Settings 才能完成核心工作。
- Settings 不默认展示 notification delivery logs。
- Activity Log 是高级入口。

## 11. Runs / Activity Log 设计

### 定位

Activity Log 是审计与排障，不是用户日常页面。

### 默认视图

- `Needs attention`
  - failed
  - degraded
  - budget stopped
  - provider unavailable
- `Recent successful activity`
  - 折叠

### 列表字段

默认字段：

- user-facing event
- status
- impact
- time
- action

高级详情：

- agent
- run ID
- model
- tokens
- cost
- raw error
- raw input/output

### Event 映射

| 系统 run | 用户事件 |
|---|---|
| `insight` ok | Context refreshed |
| `strategist` ok | Content plan updated |
| `writer` ok | Draft created |
| `qa` failed | Draft needs evidence |
| `publisher` failed | Publishing needs attention |
| budget stopped | Automation paused by budget |
| provider unavailable | Visibility check degraded |

### 验收

- Tokens 不在默认列表出现。
- Cost 只在 monthly budget 或 advanced detail 中出现。
- 成功 run 不制造噪音。
- 每个失败都有用户影响和下一步。

## 12. 信息披露策略

### 12.1 默认显示

- next action
- why this action
- also waiting
- results / momentum
- loop progress
- pending review count
- blocked content reason
- publish/distribution status
- context health
- evidence coverage
- visibility opportunities
- automation warnings

### 12.2 条件显示

- monthly spend: 仅当接近预算或触顶
- degraded state: 仅当影响结果质量或自动化继续运行
- crawl errors: 仅当关键页面失败或 Context 不完整
- Search Console missing: 仅当用户查看 Visibility 或相关指标

### 12.3 高级显示

- tokens
- model
- raw run output
- raw crawl details
- prompt sets
- provider observations
- deployment/build/database migration

## 13. 页面文案规范

### 13.1 避免内部词

避免：

- Run Insight
- Run Strategist
- Publish tick
- generation_runs
- agent
- degraded output
- tokens

替换：

- Refresh context
- Generate content plan
- Check publish status
- Activity
- Automation
- Limited quality
- Budget usage

### 13.2 异常文案格式

异常必须包含：

1. 发生了什么
2. 对用户有什么影响
3. 用户下一步能做什么

示例：

```text
Publishing needs attention
CiteLoop could not confirm the article URL after publishing. Variants are waiting until the canonical URL is available.
Check publish status or update the publishing connection.
```

### 13.3 Empty state 格式

Empty state 必须指向下一步动作。

错误示例：

```text
No runs.
```

正确示例：

```text
No content plan yet.
Refresh Context first, then generate a plan for this domain.
```

## 14. 视觉与布局要求

### 14.1 总体布局

- 继续使用 fixed sidebar + centered content column。
- 默认内容列宽约 `960px`。
- Review 这类编辑页面可放宽到约 `1320px`。
- 避免全宽 dashboard。
- 避免多个同级 KPI cards 堆满首屏。

### 14.2 密度

- Home：中等密度，突出队列和下一步。
- Context：单列模块化，允许阅读和编辑。
- Review：更高密度，支持比较和编辑。
- Activity Log：表格密度较高，但默认不在主流程。

### 14.3 卡片使用

卡片只用于：

- 可操作工作项
- 模块边界
- 异常或状态摘要

不要把每个小数字都做成 KPI 卡片。

### 14.4 状态颜色

- Green：healthy, ready, published
- Amber：needs review, limited, stale, waiting
- Red：blocked, failed, cannot approve
- Neutral：empty, inactive, advanced

颜色只表达状态，不做装饰。

Amber 承载的语义较多，必须配合明确文字标签，避免页面变成一片黄色但用户不知道优先级：

- `Needs review`
- `Waiting on canonical`
- `Context stale`
- `Limited data`
- `Measuring`

同为 amber 时，排序由用户影响决定，而不是颜色深浅决定。

### 14.5 移动端策略

移动端优先用于只读分诊和轻量动作，不承载复杂编辑。

Home 移动端：

- 首屏显示 Next action、Also waiting、Results/Momentum 的紧凑摘要。
- This week、Needs review、Ready to distribute 使用单列列表。
- 不展示宽表格、debug 信息、raw details。

Context 移动端：

- First-run confirmation 支持逐步确认。
- 长文本编辑可进入独立 full-screen editor。
- Evidence library 默认只展示 claim、source、confidence，详情点开。

Review 移动端：

- 支持阅读、approve/reject、查看 blocking reason。
- 大段 markdown 编辑和 SEO metadata 编辑可以提示用户使用桌面获得完整体验。

Activity Log 移动端：

- 默认只展示 failed/degraded events。
- advanced run detail 可读，但不优先优化复杂筛选。

## 15. 数据与 API 影响

本 PRD 优先是前端信息架构整改，但为了支持用户视角，可能需要新增或聚合若干 DTO。

### 15.1 Home summary DTO

建议后端或前端 client 聚合：

```ts
type HomeSummary = {
  next_action: NextAction;
  also_waiting: WaitingItem[];
  momentum: MomentumSummary;
  loop_items: LoopItem[];
  context_health: ContextHealth;
  review_summary: ReviewSummary;
  publish_summary: PublishSummary;
  plan_summary: PlanSummary;
  automation_health: AutomationHealth;
};
```

`NextAction` 应包含可解释原因：

```ts
type NextAction = {
  kind: string;
  label: string;
  reason: string;
  href: string;
  severity: "neutral" | "warning" | "critical";
  dismissible: boolean;
};

type WaitingItem = {
  label: string;
  count?: number;
  href: string;
  severity: "neutral" | "warning" | "critical";
};
```

### 15.1.1 Momentum summary DTO

```ts
type MomentumSummary = {
  published_this_month: number;
  drafts_approved: number;
  variants_distributed: number;
  opportunities_converted: number;
  active_loop_items: number;
  visibility_trend?: {
    label: string;
    value: string;
    direction: "up" | "flat" | "down" | "unknown";
    capability_mode: "verified" | "public_only" | "unavailable";
  };
};
```

### 15.1.2 Loop item DTO

```ts
type LoopItem = {
  id: string;
  title: string;
  source: "seo" | "geo" | "crawler" | "competitor" | "evidence_gap";
  stage: "detected" | "planned" | "drafted" | "published" | "measuring" | "learned";
  status_label: string;
  href: string;
  linked_topic_id?: string;
  linked_article_id?: string;
  result?: {
    label: string;
    tone: "neutral" | "positive" | "warning";
  };
};
```

### 15.2 Context health DTO

```ts
type ContextHealth = {
  status: "healthy" | "needs_confirmation" | "stale" | "incomplete" | "blocked";
  last_refreshed_at: string | null;
  source_page_count: number;
  evidence_count: number;
  unsupported_claim_count: number;
  crawl_warning_count: number;
  confirmed_at: string | null;
  recommendations: string[];
};
```

### 15.3 Activity event DTO

把 run 映射为用户事件：

```ts
type ActivityEvent = {
  id: string;
  label: string;
  status: "ok" | "warning" | "failed";
  impact: string | null;
  next_action: string | null;
  created_at: string;
  advanced?: {
    run_id: string;
    agent: string;
    model?: string;
    tokens?: number;
    cost_usd?: number;
    raw_error?: string;
  };
};
```

### 15.4 Route compatibility

旧 route 应 redirect 到新 route：

- `/knowledge` -> `/context`
- `/topics` -> `/plan`
- `/publishing` -> `/publish`
- `/seo` -> `/visibility`
- `/runs` -> `/settings/activity`

## 16. 成功指标

### 16.1 产品指标

- 新项目 Context completion rate 提升。
- 首次生成 content plan 的时间下降。
- Review queue item approval/rejection rate 提升。
- 因理解错误导致的 reject 或 edit 下降。
- 用户访问 Runs/Activity Log 的比例下降，但异常处理完成率上升。

### 16.2 UX 指标

- 5 秒测试：用户能说出当前项目下一步动作。
- 30 秒测试：用户能说出 CiteLoop 是否正确理解 domain。
- 用户不需要查看 Activity Log 就能处理常见 publish/review 问题。
- 用户不会把正常 successful runs 误认为需要处理的任务。

### 16.3 质量指标

- 无 Context 时不会鼓励生成内容。
- 无第一方 SEO 数据时不展示虚假 CTR/position。
- QA blocking 必须有可读原因。
- Advanced-only 信息不会出现在 Home 首屏。

## 17. 分阶段落地

### Phase 1：信息架构与命名整改

目标：最快降低混乱，并让用户立刻知道下一步。

范围：

- `Knowledge` 改名 `Context`。
- `Topics` 改名 `Content Plan`。
- `Publishing` 改名 `Publish`。
- `SEO` 改名 `Visibility`。
- `Runs` 从一级导航移出。
- Home 增加最小版 `Next action` 横幅，包含推荐动作、原因和 also waiting。
- Home 删除 tokens/model/per-run cost 默认展示。
- `Publish tick` 从 Home 移除或改为 `Check publish status` 并放到 Publish 页。

验收：

- 一级导航只保留用户日常工作流。
- Home 首屏出现 next action，而不是只换标签。
- Home 不再展示 tokens。
- 普通用户不会被引导去 Runs。

### Phase 2：Home 重做为 Next Action + Momentum 工作台

目标：让用户知道下一步做什么，也知道是否在赢。

范围：

- 完整化 `Next action` 区块。
- 新增 `Results / Momentum` 区块。
- 新增 `Loop progress` 区块。
- 重排 Home section。
- Automation health 只显示异常。
- Context health 加入 Home。
- Recent runs 改为 Activity warning summary。

验收：

- Home 首屏回答“现在该做什么”。
- Home 首屏回答“已经交付了什么价值”。
- Visibility opportunity 到 Content Plan 的 loop item 在 Home 可见。
- 成功 run 不占据 Home。

### Phase 3：Context 页重做

目标：让 Context 成为用户可审核的产品认知中心。

范围：

- 去掉默认 `Profile JSON`。
- 新增 First-run Context confirmation flow。
- 新增结构化 Domain profile。
- 新增独立 Evidence library，并提升到页面核心区域。
- Competitors、positioning、banned claims 首版可编辑。
- Source pages 与 crawl summary 合并进 Context health。
- Advanced JSON 折叠。

验收：

- 用户能在 Context 页审核 positioning、ICP、value props、evidence。
- 首次抓取后用户能确认或修正系统理解。
- Evidence 与 QA blocking 可关联。

### Phase 4：Visibility 降噪

目标：把 SEO/GEO 从调试台改成结果页，并让 loop 闭合被感知。

范围：

- SEO/GEO 合并为 Visibility 页面叙事。
- 首屏展示 overview、opportunities、blockers。
- Opportunity 增加 lifecycle：detected/planned/drafted/published/measuring/learned。
- `Add to Content Plan` 后继续在 Visibility 追踪状态。
- Loop closed / Opportunity converted 的结果反馈。
- crawler snapshots、prompts、provider observations 放到 Advanced diagnostics。

验收：

- 用户能理解 visibility 状态，不需要理解 provider。
- 缺失数据被解释为 capability mode，不被表现成系统错误。
- Opportunity 闭环状态可见，并能回流到 Home momentum。

### Phase 5：Activity Log 高级化

目标：保留审计能力，但不打扰普通用户。

范围：

- Runs 页改为 Activity Log。
- 默认只展开 failed/degraded/budget events。
- tokens/model/cost 进入详情。
- 每个失败映射用户影响和下一步。

验收：

- Activity Log 对技术 admin 有用。
- 普通用户日常不需要进入 Activity Log。

## 18. 关键验收清单

- [ ] 一级导航不包含 `Runs`。
- [ ] `Knowledge` 全部改为 `Context`。
- [ ] Home 首屏有 next action、why this、also waiting。
- [ ] Home 首屏有 Results / Momentum。
- [ ] Home 可见 loop progress。
- [ ] Home 不展示 tokens、model、per-run cost。
- [ ] Context 默认不展示 JSON。
- [ ] Context 首次抓取后有 confirmation flow。
- [ ] Context 包含独立 Evidence library、Domain profile、Source pages、Voice & rules。
- [ ] Evidence library 不内嵌在 Source pages。
- [ ] Competitors、positioning、banned claims 可编辑。
- [ ] QA blocking 展示可读原因和下一步。
- [ ] Publish failure 可在 Publish 页解决，不要求去 Runs。
- [ ] Visibility 首屏不展示 provider/prompt/debug 表。
- [ ] Visibility 首屏能让用户识别这是 SEO + GEO visibility。
- [ ] Visibility opportunity 能加入 Content Plan，并持续追踪 lifecycle。
- [ ] Loop closed / opportunity converted 有可见反馈。
- [ ] Activity Log 只在高级入口展示。
- [ ] 所有 empty state 指向下一步。
- [ ] 所有异常文案说明用户影响。

## 19. 已定设计决策

1. `Context` 支持用户手动编辑 competitors、positioning、banned claims。尤其 banned claims 是品牌和法律护栏，不能只读。
2. Evidence library 是独立 section，不内嵌在 Source pages。
3. `Visibility` 在 Phase 1 先完成命名、导航和 redirect；内容重构放到 Phase 4。
4. Home 的 `Next action` 由统一规则计算，最好放在后端或共享纯函数中，并提供 `why this` 和 `also waiting`。
5. Activity Log 全员可见，但作为次级/高级入口；限制的是显眼程度，不是可见性。
6. Home 不新增 `Results` 一级导航。Results / Momentum 并入 Home，守住导航不 bloated。

## 20. 结论

CiteLoop 的 dashboard 应从“展示系统做过什么”转向“告诉用户现在该做什么、系统已经交付了什么价值、下一轮机会从哪里来”。SuperX 的启发不是具体 UI 组件，而是它把复杂内容增长系统包装成清晰的日常工作流和可重复循环：context、ideas、writing、schedule、automation、growth。CiteLoop 的 domain 版本应该是：context、evidence、plan、review、publish、visibility、learn。

只要把低价值内部信息降级，把 Evidence-backed Context 做成人能确认的产品认知中心，把 Home 改成 next-action + momentum 工作台，并把 Visibility opportunity 回流到 Content Plan 的 loop 做成可感知体验，CiteLoop 的 dashboard 就会从 MVP 控制台转向真正的用户产品。
