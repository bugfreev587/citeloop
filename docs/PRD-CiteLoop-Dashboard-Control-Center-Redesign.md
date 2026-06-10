# PRD：CiteLoop Dashboard 控制中心重构

> 日期：2026-06-10
> 状态：Draft
> 范围：Dashboard 信息架构、页面长度、首屏优先级、空状态、文案、渐进披露
> 上游文档：`docs/PRD-CiteLoop-Dashboard-UX-Remediation.md`
> 代码基线：`main@fbfa677`，包含 PR #14–#20 已合并的 Review blocking、Publish reconcile、Visibility/Context polish、Settings gate 等改动

## 0. 摘要

CiteLoop dashboard 当前已经从早期的后台功能列表，走向了 `Home / Context / Content Plan / Review / Publish / Visibility / Settings` 的产品化信息架构。但实际页面仍然偏长，尤其是 Home、Visibility、Settings。即使在空数据状态下，页面也会展示大量 section、卡片、空状态和高级信息，导致用户打开 dashboard 后仍然需要滚动、扫描和判断。

本 PRD 的目标是把 CiteLoop dashboard 从“内容堆叠的运营后台”进一步重构成“用户的控制中心”：

```text
Status
+ Actions
+ Timeline
+ Minimalism
```

用户打开 dashboard 的第一秒，应能回答三个问题：

1. 我的项目现在是否正常？
2. 最值得我处理的下一步是什么？
3. CiteLoop 最近交付了什么，下一步会发生什么？

不应该默认让用户处理：

- 内部 agent 名称
- run、tick、sync、reconcile 等系统动作
- raw JSON、tokens、model、prompt、crawler table
- 每个页面所有空 section

本次重构不要求重写后端数据模型，但要求前端默认展示策略、页面分组和文案彻底从用户结果出发。

## 1. 当前审计结论

### 1.1 客观页面长度

基于本地 dev server 的空状态页面测量。该测量用于说明默认信息量问题，不替代实施前的最新浏览器验收；每个 phase 开始前必须在当时的 `main` 上重新测量一次。

| 页面 | 空状态高度 | 首屏 section 数 | 问题 |
|---|---:|---:|---|
| Home | 约 1.76 屏 | 7 | 空状态也展示 Momentum、Context、Loop、This week、Needs attention、Needs review、Ready to distribute |
| Context | 约 1.11 屏 | 5 | 未连接 domain 时仍展示 Evidence、Domain profile、Voice、Source pages 多个空区块 |
| Content Plan | 约 1 屏 | 3 | 文案仍使用 `Run Strategist`，缺少 plan health |
| Review | 约 1 屏空状态 | 1 | 方向正确，但有数据时单篇内容默认过重 |
| Publish | 约 1 屏空状态 | 4 | 四个 lane 都默认展示空状态 |
| Visibility | 约 1.68 屏 | 12 | SEO、GEO、Loop、Brief、Opportunities、Diagnostics 多个心智模型串行展示 |
| Settings | 超长表单 | 5+ | General、Publisher、Crawl、Notifications、Delivery、Activity 混在同一页 |

注意：PR #14–#20 已经解决了一部分诚实性和解释性问题，例如 Review inline blocking reason、Publish reconcile summary、Visibility cold-start metric honesty、Settings 非管理员入口隐藏。本 PRD 后续只把仍然存在的默认信息量、首屏优先级和长期稳态问题纳入 scope，避免重复 specification 已完成的工作。

### 1.2 主要问题

1. **页面默认展示过多空状态。**
   空状态应该帮助用户进入下一步，而不是把未来所有模块都摊开。

2. **页面没有足够强的“一个页面只做一件事”。**
   Visibility 同时像 analytics、SEO setup、GEO debug、opportunity list、autopilot settings。Settings 同时像项目设置、发布集成、crawl config、通知中心和事件日志。

3. **首屏信息优先级仍然松散。**
   Home 的 `Next action` 是正确方向，但其后立刻展示过多模块，稀释了控制中心感。

4. **工程语言还没有完全翻译成用户语言。**
   `Run Strategist`、`Reconcile`、`Sync`、`service console`、`canonical_url` 等词仍然暴露底层实现。

5. **高级信息虽然有折叠，但默认页仍被高级心智影响。**
   Visibility 的高级 diagnostics 已折叠，但主页面仍展示 brief、opportunities、content actions 三套相邻列表。

## 2. 产品目标

### 2.1 用户目标

1. 新用户能在一个清晰 onboarding flow 中连接 domain，而不是看到多个空模块。
2. 日常用户能在 Home 首屏知道系统状态、下一步动作和近期成果。
3. 内容运营者能进入对应页面后只处理该页面的主要工作，不需要理解系统内部任务。
4. 技术管理员仍能找到高级调试和设置，但它们不影响普通用户默认工作流。
5. 用户能看到 CiteLoop 的 loop：Context -> Content Plan -> Review -> Publish -> Visibility -> New Opportunity。

### 2.2 业务目标

1. 降低 dashboard 首次使用困惑。
2. 提高 context 完成率。
3. 提高 review queue 处理率。
4. 提高 visibility opportunity 转化为 content plan 的比例。
5. 降低用户对正常后台自动化的误解。
6. 让 dashboard 更接近可销售 SaaS 产品，而不是内部 MVP 控制台。

### 2.3 设计目标

1. 每个页面空状态默认不超过 1.2 屏。
2. Home 默认首屏必须回答 status、next action、momentum。
3. 每个页面默认可见 section 不超过 3 个，除非用户已经有真实数据需要处理。
4. 高级信息通过 tabs、details、drawer 或独立子路由展示。
5. 所有主动作使用用户语言，不使用 agent 或 job 语言。
6. 用户结果优先于系统过程。

## 3. 非目标

- 不重做完整视觉品牌系统。
- 不要求新增计费、团队权限或多项目管理后台。
- 不要求删除 runs、prompts、crawler diagnostics、tokens、model 等高级信息。
- 不要求重写后端 domain model。
- 不要求一次性完成所有页面的最终视觉精修。
- 不做营销 landing page。

## 4. 核心原则

### 4.1 Dashboard 是控制中心，不是功能列表

Dashboard 默认展示：

- 系统状态
- 关键动作
- 最近进度
- 阻塞与解决路径

Dashboard 默认不展示：

- 每个功能入口的空列表
- 所有高级设置
- 后台任务流水账
- 原始调试数据

### 4.2 首屏必须回答三个问题

每个核心页面首屏必须回答：

1. 当前状态是什么？
2. 用户现在能做什么？
3. 为什么这是下一步？

### 4.3 用户结果优先，系统过程后置

示例：

| 不要 | 要 |
|---|---|
| `Run Strategist` | `Generate content plan` |
| `Reconcile` | `Check publish status` |
| `canonical_url missing` | `Live article URL is not confirmed yet` |
| `qa_blocking=true` | `Cannot approve: 2 claims need evidence` |
| `Sync` | `Refresh visibility data` |

### 4.4 空状态是 onboarding，不是占位符

没有数据时，页面只展示一个推荐路径。不要展示所有未来模块的空盒子。

### 4.5 渐进披露优先

默认视图面向 Founder / Operator / Growth Marketer。高级视图面向 Technical Admin。

高级信息保留，但通过以下方式进入：

- Settings 子导航
- Advanced details
- Diagnostic drawer
- Activity Log
- Article detail
- Visibility diagnostics tab

### 4.6 页面只做一件事

| 页面 | 只做什么 |
|---|---|
| Home | 判断状态并进入下一步 |
| Context | 确认 CiteLoop 对 domain 的理解和证据 |
| Content Plan | 管理选题、计划和生成意图 |
| Review | 判断内容是否能发布 |
| Publish | 处理发布和分发状态 |
| Visibility | 发现可见性机会并追踪 loop |
| Settings | 配置项目和高级工具 |

### 4.7 Action over data

任何默认可见数字都必须回答“用户能做什么”。指标不能只是统计值。

示例：

| 不要 | 要 |
|---|---|
| `Drafts approved: 2` | `2 approved drafts` + `Open Publish` |
| `Failed publishes: 1` | `1 publish issue` + `Retry publishing` |
| `Opportunities converted: 0` | 隐藏该 tile，或显示 `No opportunity loop yet` + `Open Visibility` |

新项目不要默认展示四个 `0`。零值指标应被隐藏、合并成 onboarding 状态，或改写成定性状态。

### 4.8 屏高是护栏，不是北极星

页面高度用于防止失控滚动，但不是最终体验目标。真正的目标是：

1. 用户 3 秒内知道状态和下一步。
2. 首屏只有一个主决策。
3. 数据块能直接进入动作。
4. 稳态项目不会因为多个 conditional module 同时命中而重新变成长摘要页。

### 4.9 F-pattern 视觉层级

不仅要规定 section 顺序，还要规定视觉权重。Home 的 Next action 是首屏视觉主体；Momentum、Event stream、Health chips 是次级信息。不能让所有 section 看起来等权。

## 5. 页面长度与信息预算

### 5.1 全局预算

屏高预算是 guardrail。若为了压缩到预算而牺牲留白、可读性或动作清晰度，应优先保留清晰度，并通过折叠、tabs、条件显示解决。

| 场景 | 目标 |
|---|---:|
| 空状态核心页面 | <= 1.2 屏 |
| Home 正常状态 | <= 1.6 屏 |
| 队列页面有 1 到 5 条数据 | <= 1.8 屏 |
| Settings 默认页 | <= 1.2 屏，其他内容放 tabs |
| Visibility 默认页 | <= 1.4 屏 |
| 高级诊断页 | 可长，但必须用户主动进入 |

### 5.2 默认可见 section 上限

| 页面 | 空状态默认 section 上限 | 正常状态默认 section 上限 |
|---|---:|---:|
| Home | 3 | 5 |
| Context | 1 onboarding + 1 preview | 4 |
| Content Plan | 2 | 4 |
| Review | 1 | 2 pane layout |
| Publish | 2 | 4, only show non-empty lanes |
| Visibility | 3 | 4 |
| Settings | 2 | tabbed |

### 5.3 KPI 上限

Home 和 Visibility 首屏主指标均不得超过 4 个。需要更多指标时使用 secondary detail、drawer 或 dedicated analytics view。

### 5.4 首屏决策预算

每个核心页面首屏最多：

- 1 个 primary decision
- 2 个 secondary actions
- 4 个可点击状态入口

超过预算的信息进入 `More`、tabs、details 或下级页面。

### 5.5 稳态模块预算

空状态需要克制，稳态也需要克制。Home 中如果多个模块同时有数据，默认只展开与 Next action 优先级最相关的前 2 个模块，其余折叠到 `More waiting`。

优先级沿用 Next action：

1. Context missing / unconfirmed / stale
2. Publish failed
3. QA blocking
4. Pending review
5. Ready to distribute
6. No content plan
7. Scheduled content
8. Visibility opportunities
9. Healthy history / completed work

## 6. 信息架构

### 6.1 一级导航

保持当前方向：

1. Home
2. Context
3. Content Plan
4. Review
5. Publish
6. Visibility
7. Settings

### 6.2 移除或降级入口

- `Runs` 不出现在一级导航。
- `SEO` redirect 到 `Visibility`。
- `Knowledge` redirect 到 `Context`。
- `Topics` redirect 到 `Content Plan`。
- `Publishing` redirect 到 `Publish`。

### 6.3 Settings 子导航

Settings 内部拆分：

1. General
2. Publishing
3. Crawl
4. Notifications
5. Activity Log
6. Advanced

移动端使用 segmented tabs 或 select。桌面端可使用左侧二级导航。

## 7. App Shell

### 7.1 目标

App Shell 让用户知道：

- 当前项目是什么
- 哪些入口是日常工作流
- 当前最重要动作是什么
- 系统是否健康

### 7.2 主 CTA

当前固定 `Review queue` CTA 需要改为动态 `Primary action`。

优先级：

1. Context 缺失：`Set up Context`
2. Context 未确认：`Confirm Context`
3. Publish failed：`Fix publishing`
4. QA blocking：`Review blocked drafts`
5. Pending review：`Review N drafts`
6. Ready to distribute：`Distribute N variants`
7. No content plan：`Generate content plan`
8. Healthy：`Open Home`

### 7.3 Budget

Sidebar 底部不默认强调金额。展示状态：

- `Budget healthy`
- `Near monthly limit`
- `Automation paused`

金额放 tooltip 或 Settings > General。

### 7.4 验收

- 一级导航最多 7 项。
- 普通用户不看到 Runs。
- Sidebar 不展示 tokens、model、run ID、raw cost。
- 主 CTA 随项目状态变化。

## 8. Root / Project List

### 8.1 当前问题

首页标题 `CiteLoop service console` 偏内部。右侧能力说明是静态功能解释，不如 onboarding progress 有价值。

### 8.2 目标

Root page 是项目入口，不是营销页，也不是后台 console。

### 8.3 首屏结构

1. Product label：`SEO + AI visibility content engine`
2. H1：`CiteLoop content operations`
3. Project list
4. Create project card

### 8.4 Create project

未创建项目时，右侧表单只做一件事：

```text
Connect your domain
CiteLoop will read public pages, extract product facts, and create the context used for planning and review.
```

提交后展示进度：

1. Project created
2. Context refresh started
3. Visibility baseline queued

避免 `product profile job`、`SEO baseline job` 这类内部语言。

### 8.5 验收

- Root page 空状态不超过一屏。
- 用户知道第一步是 connect domain。
- 不使用 `service console`。

## 9. Home

### 9.1 定位

Home 是日常控制中心。它不是所有模块的摘要页。

### 9.2 首屏结构

默认首屏只保留：

1. Next action
2. Actionable momentum
3. Event stream

可选紧凑区域：

- Context health chip
- Automation health chip

视觉权重：

- Next action 占据首屏最大面积和最高对比度，是用户第一眼看到的主体。
- Actionable momentum 是次级状态入口，不和主动作争抢视觉优先级。
- Event stream 是细线型信息，不做大卡片堆叠。

不在空状态默认展示：

- Needs attention 空列表
- Needs review 空列表
- Ready to distribute 空列表
- Waiting on canonical 空列表
- This week 的多个空 slot

### 9.3 Next action

Next action 组件包括：

- Action title
- Why this
- Primary action
- Secondary action
- Also waiting

示例：

```text
Refresh context
Confirm product facts, evidence, and positioning before generating a content plan.

[Refresh context] [Open Context]

Also waiting: No content plan yet
```

如果没有任何待办：

```text
All set for now
Your content loop is healthy. The next scheduled slot is Friday.

[Open Content Plan]
```

### 9.4 Momentum

Momentum 不是纯数据区，而是 action-bearing status。最多 4 个 tile，每个 tile 必须有 destination 或 action。

默认候选：

| Tile | 显示条件 | 动作 |
|---|---|---|
| Published this month | count > 0 | Open Publish history |
| Drafts approved | count > 0 | Open Publish |
| Ready to distribute | count > 0 | Distribute variants |
| Opportunities converted | count > 0 | Open Visibility pipeline |
| Active loop items | count > 0 | Open event stream / loop detail |
| Context ready | context ready 且其他 count 为 0 | Open Content Plan |

零值规则：

- 不默认展示四个 `0`。
- 如果所有 momentum 数字为 0，显示一个 qualitative state：`Context is ready. Generate your first content plan.` 或 `Set up Context to start the loop.`
- 数字 tile 的 value、label、detail 和 action 必须在同一个可点击区域内，体现“状态即入口”。

无 verified SEO/GEO 数据时不展示假指标。使用 capability-aware 文案：

```text
Search Console is not connected. CiteLoop is tracking public crawl and content progress only.
```

### 9.5 Event stream

Home 需要的是按时间理解的事件流，不是 lifecycle pipeline。它回答：

```text
刚发生了什么？
现在正在发生什么？
下一步什么时候发生？
```

默认展示 3 到 5 行：

- Recent done events：`Draft approved`、`Canonical published`、`Context refreshed`
- Live events：`Reading your site`、`Generating draft`、`Publishing now`
- Next event：`Next content slot: Jun 14`

空时只显示一行 onboarding：

```text
No activity yet. Connect your domain to start the first context refresh.
```

不要在空状态占据一整张大卡。

Lifecycle pipeline 仍然重要，但它属于 Visibility 的 Opportunity pipeline，不属于 Home 默认首屏。

### 9.6 Live status

如果后台正在执行用户可理解的工作，Home 首屏必须显示一条 live status。

示例：

```text
Reading your site
CiteLoop is scanning public pages. Evidence and source pages will appear in Context automatically.
```

```text
Generating draft
CiteLoop is drafting one approved topic. It will move to Review when QA finishes.
```

Live status 只展示用户影响，不展示 run ID、agent、model、tokens。

### 9.7 Conditional modules

以下模块只有满足条件才显示：

| 模块 | 显示条件 |
|---|---|
| Needs attention | 有 publish failure、automation warning、budget pause |
| Needs review | review count > 0 |
| Ready to distribute | ready count > 0 |
| Waiting on canonical | waiting variants > 0 |
| This week | 有 scheduled content，或用户已经完成 context + content plan |

稳态展开规则：

- 默认最多展开 2 个 conditional modules。
- 与 Next action 同类型的模块排第一。
- 其他模块进入 `More waiting`，显示 compact links 和 counts。
- 用户展开过的模块可在当前 session 内保持展开。

### 9.8 Healthy / inbox-zero state

全部处理完时，Home 不是“没有内容”的缺省状态，而是一个安静的完成状态：

```text
All set for now
No drafts, publish issues, or distribution tasks need your attention.
Next scheduled check: Jun 14.

[Open Content Plan] [Open Visibility]
```

这个状态应让用户确信系统正常，而不是误以为没有数据或坏了。

### 9.9 验收

- Home 空状态 <= 1.2 屏。
- Home 正常状态 <= 1.6 屏。
- 首屏必须同时出现 next action、actionable momentum、event stream。
- Next action 是首屏视觉主体，其他模块为次级信息。
- 空状态不展示 3 个以上大 empty cards。
- 稳态最多展开 2 个 conditional modules，其余进入 `More waiting`。
- Momentum tile 必须可点击或带明确动作。
- 新项目不得展示四个 0-value KPI tile。
- Home event stream 展示 recent / live / next，不把 lifecycle pipeline 当时间轴。
- 没有 `run`、`strategist`、`tick` 等内部词。

## 10. Context

### 10.1 定位

Context 是 CiteLoop 对 domain 的产品认知和证据中心。它回答：

```text
CiteLoop 是否正确理解我的产品、受众、证据和内容边界？
```

### 10.2 未连接 domain 状态

未连接或无 profile 时，只展示一个 setup panel：

```text
Set up Context
Connect your domain so CiteLoop can read public pages, extract product facts, and build evidence-backed context.

[Refresh context]
```

下面可以展示一个小 preview，说明 context 会包含：

- Product positioning
- Audience / ICP
- Evidence-backed claims
- Voice and rules

不要显示 Evidence library、Domain profile、Voice、Source pages 四个空 section。

### 10.3 成功抓取但未确认

进入 first-run confirmation flow：

1. Positioning
2. Audience / ICP
3. Evidence-backed claims
4. Banned / risky claims
5. Competitors and alternatives
6. Voice and rules

每一步显示：

- Extracted value
- Source / evidence when available
- Accept
- Edit
- Remove

### 10.4 正常状态结构

默认展示：

1. Context health
2. Evidence library
3. Domain profile
4. Voice and rules

Source pages 默认折叠或 tab 内展示。Advanced JSON 继续折叠。

### 10.5 Evidence library

Evidence library 是核心，不是 source pages 的附属信息。

每条 evidence 展示：

- Claim
- Evidence snippet
- Source URL
- Confidence
- Used by drafts
- User approval status

### 10.6 验收

- 无 context 时只有一个主要 onboarding action。
- 用户 30 秒内能判断定位、受众、证据是否正确。
- Evidence library 独立且优先级高。
- Raw JSON 默认不可见。
- Crawl warnings 只在 warning > 0 时显示。

## 11. Content Plan

### 11.1 定位

Content Plan 管理选题、计划和生成意图。它不是 strategist 输出列表。

### 11.2 当前问题

页面标题仍是 `Topics`，主按钮仍是 `Run Strategist`。Backlog item 展示了 channel、status、priority、format、angle、internal links、scheduled 等字段，但没有清楚解释“为什么这个选题值得做”。

### 11.3 首屏结构

1. Plan health
2. Generate content plan
3. Schedule / Backlog tabs

Plan health 展示：

- Backlog count
- Scheduled this week
- Missing slots
- Context state

### 11.4 文案

| 当前 | 改为 |
|---|---|
| `Topics` | `Content Plan` |
| `Run Strategist` | `Generate content plan` |
| `Running strategist` | `Generating plan` |
| `Generate` | `Draft this topic` |
| `Archive` | `Remove from plan` |

### 11.5 Topic card

每个 topic 默认展示：

- Title
- Why this exists
- Target keyword or prompt
- Priority reason
- Evidence sources
- Next action

次级字段如 internal links、raw status、scheduled_at 放进 details。

### 11.6 Visibility-sourced opportunities

从 Visibility 加入的机会必须标注：

- Source signal
- Expected impact
- Loop stage
- Linked opportunity

### 11.7 验收

- 页面标题和按钮不出现 `Strategist`。
- 用户知道每个 topic 为什么存在。
- 无 context 时生成按钮带 warning 或 disabled。
- 空状态只展示一个生成计划动作。

## 12. Review

### 12.1 定位

Review 是唯一人工审核闸门。它回答：

```text
这篇内容能不能发布？不能的话要修哪里？
```

### 12.2 当前方向

Review 是目前最接近“一个页面做一件事”的页面。它已展示 QA blocking、repair、SEO contribution 和 web preview。但有数据时，每篇文章默认展开 Markdown、QA、SEO、Preview，会导致队列很长。

### 12.3 新布局

桌面端使用两栏：

- 左栏：Review queue
- 右栏：Selected article review panel

移动端：

- Queue list
- 点击进入 article review screen

### 12.4 Queue row

每行展示：

- Title
- Content type / platform
- QA state
- Evidence state
- Primary issue
- Action

不要默认展示完整 markdown。

### 12.5 Review panel

默认展示：

1. Decision bar：Approve / Request changes / Reject
2. Blocking reason
3. Evidence checklist
4. Web preview

Markdown editor 默认折叠，点击 `Edit content` 后展开。

### 12.6 QA blocking copy

示例：

```text
Cannot approve yet
The draft makes a pricing claim, but Context has no supporting evidence.

[Fix evidence in Context] [Edit draft]
```

### 12.7 验收

- 队列有多篇文章时页面长度主要由 queue 控制，不由每篇文章全文控制。
- Approve 禁用态旁边有明确原因。
- Blocking issue 能跳到 Context evidence。
- 默认不展示 raw QA issue 字符串，raw detail 放 Advanced。

## 13. Publish

### 13.1 定位

Publish 展示 canonical 和 syndication variants 的发布/分发状态。

### 13.2 当前问题

页面默认展示 4 个 lane 的空状态：Publish failures、Published canonical、Ready to distribute、Waiting on canonical。空状态用户需要扫完整页才知道其实没事。

### 13.3 首屏结构

1. Publish status
2. Primary lane
3. Issues if any

Publish status 展示：

- Connection state
- Next scheduled publish
- Last publish
- Manual distribution count

### 13.4 Conditional lanes

| Lane | 显示条件 |
|---|---|
| Failures | failed.length > 0 |
| Ready to distribute | ready.length > 0 |
| Waiting on canonical | waiting.length > 0 |
| Published canonical | published.length > 0，或用户主动打开 history |

空状态只显示：

```text
Nothing to publish right now
Approved canonical articles will publish automatically when due. Variants unlock after the live article URL is confirmed.
```

### 13.5 文案

| 当前 | 改为 |
|---|---|
| `Publishing` | `Publish` |
| `Reconcile` | `Check publish status` |
| `Retry` | `Retry publishing` |
| `missing canonical_url` | `Live article URL not confirmed` |
| `canonical_url backfill` | `live article URL confirmation` |

### 13.6 验收

- 空状态 <= 1 屏。
- 有失败时 failure 显示在首屏。
- 用户知道 canonical 和 variant 的依赖关系。
- 用户不需要去 Activity Log 找 publish failure 原因。

## 14. Visibility

### 14.1 定位

Visibility 是 SEO + AI answer visibility 的用户结果层。它回答：

```text
我的 domain 是否更容易被搜索和 AI answer surfaces 发现？下一步应该做什么？
```

### 14.2 当前问题

当前 Visibility 默认串行展示：

- Visibility overview
- Search visibility
- AI visibility
- Loop closure
- Advanced diagnostics
- Visibility brief
- Opportunities
- Content actions

它仍像多个工具页拼在一起。

### 14.3 新默认结构

默认只展示：

1. Visibility status
2. Top opportunities
3. Loop closure

可选 compact status row：

- Search data mode
- AI crawler access
- Open blockers
- Opportunities in loop

### 14.4 Tabs

Visibility 使用 tabs 或 segmented control：

1. Overview
2. Opportunities
3. Search
4. AI Answers
5. Diagnostics

默认进入 Overview。

### 14.5 Overview tab

展示：

- Capability mode：public crawl only / connected / limited
- Top 3 opportunities
- Major blockers
- Loop closure

不展示：

- placeholder clicks/impressions cards
- prompt tables
- provider observations
- asset brief tables
- autopilot settings

### 14.6 Opportunities tab

合并 `Visibility brief`、`Opportunities`、`Content actions` 为一个 opportunity pipeline。

这是 lifecycle pipeline 的主场。Home 只展示 recent / live / next 事件流；Visibility 才展示 opportunity 从 detected 到 learned 的生命周期。

每个 opportunity 展示：

- Title
- Source signal
- Expected impact
- Recommended content action
- Status in loop
- Linked topic/article if any
- Actions：Add to Content Plan / Dismiss / Mark not relevant

状态：

```text
Detected -> Added to Content Plan -> Drafted -> Published -> Measuring -> Learned
```

### 14.7 Search tab

只有 connected GSC 时展示 clicks、impressions、CTR、position。

当前 `main@fbfa677` 已经把未连接 GSC 的指标做成置灰并标注 placeholder，这解决了诚实性问题。本 PRD 的下一步目标是进一步降低默认认知负担：未连接时 Overview 不展示 placeholder metric cards，Search tab 内优先展示 setup guidance。

```text
Search Console is not connected.
CiteLoop is using public crawl and content progress until first-party search data is connected.
```

如果需要保留置灰指标作为教育性 preview，只能放在 Search tab 的 secondary area，不能进入 Visibility Overview。

### 14.8 AI Answers tab

展示：

- AI crawler access summary
- Answer coverage when confidence is sufficient
- Citation-ready briefs
- Competitor / alternative mentions only when available

Prompt sets、prompts、provider observations 放 Diagnostics。

### 14.9 Diagnostics tab

包含：

- Setup checklist
- Site URL settings
- GEO crawler access table
- Prompt sets
- Prompts
- Provider observations
- Competitors
- External surfaces
- Asset briefs
- Autopilot

Diagnostics 可以长，但用户必须主动进入。

### 14.10 验收

- Visibility 默认页 <= 1.4 屏。
- 默认页最多 4 个主 section。
- 未连接 GSC 时不展示 placeholder metric cards。
- Brief、opportunities、content actions 不再是三个连续列表。
- Advanced diagnostics 不影响 Overview 的页面长度。

## 15. Settings

### 15.1 定位

Settings 是配置和高级管理区，不是日常工作台。

### 15.2 当前问题

Settings 当前把 Activity Log、项目节奏、发布连接、crawl config、notifications、subscriptions、deliveries 全部串行展示。

### 15.3 新结构

使用二级导航：

1. General
2. Publishing
3. Crawl
4. Notifications
5. Activity Log
6. Advanced

### 15.4 General

展示：

- Cadence per week
- Buffer days
- Monthly budget
- Channel mix
- Brand voice

保存按钮固定在表单底部或 sticky footer。

### 15.5 Publishing

展示：

- Connection status
- Repository
- Branch
- Content path
- Base URL
- Token state
- Test connection

危险动作如 revoke token 放 danger zone。

### 15.6 Crawl

默认展示：

- Max pages
- Respect robots
- Same origin only

高级参数默认折叠：

- Max depth
- Request timeout
- Rate limit
- Sitemap URL cap

### 15.7 Notifications

分为：

- Channels
- Subscriptions
- Delivery history

Delivery history 默认折叠或独立 tab，只有失败 delivery 才在默认区显示。

### 15.8 Activity Log

承载原 Runs：

- Needs attention 默认展开
- Successful activity 默认折叠
- Advanced details 内展示 run ID、model、tokens、cost、raw error

### 15.9 验收

- Settings 默认页 <= 1.2 屏。
- 用户进入 Settings 后先看到 General，不看到所有高级表格。
- Crawl timeout、rate limit、delivery history 不默认占据主页面。

### 15.10 访问模型

当前 `main@fbfa677` 中 Settings 是 admin-gated，非管理员不会在 sidebar 看到入口，直接访问会 404。重构 Settings 前必须先确认访问模型：

| 模型 | 说明 | 影响 |
|---|---|---|
| Internal-only | Settings 继续只给内部运营/管理员使用 | 可以保留更多技术字段，但不应计入普通用户 dashboard IA |
| Customer self-serve | 客户可配置 cadence、publisher、notifications | 必须做普通用户文案、权限和危险动作保护 |
| Hybrid | General/Publishing 给客户，Activity/Advanced 给 admin | 推荐方向，但需要更细权限分区 |

在访问模型未确认前，Phase 3 只应先做信息拆分和 admin-gated 体验，不假设所有用户都能打开 Settings。

## 16. Article Detail 与 Admin

### 16.1 Article Detail

Article detail 是 drill-down 页面，不进入主导航。

默认展示：

- Article state
- Publish state
- Content preview
- QA state

Raw SEO metadata JSON 放 Advanced。

### 16.2 Admin

Admin 保持内部工具定位，不出现在普通 sidebar。仅 internal users 可见。

## 17. 数据与前端契约

### 17.1 可先用前端纯函数实现

以下规则可以先保留在前端纯函数中，并配合同步测试：

- Next action priority
- Sidebar primary CTA
- Home conditional modules
- Actionable momentum visibility and destinations
- Home event stream recent/live/next mapping
- Visibility lifecycle label
- Context health label
- Publish status label

### 17.2 推荐后端聚合端点

后续可新增聚合端点，减少页面内并发请求和重复推导：

```text
GET /projects/:id/dashboard-summary
GET /projects/:id/settings-summary
GET /projects/:id/visibility-summary
```

`dashboard-summary` 返回：

- next_action
- momentum
- context_health
- publish_health
- review_counts
- distribution_counts
- loop_items
- automation_health

### 17.3 前端组件建议

新增或抽取：

- `PrimaryActionPanel`
- `StatusSummary`
- `ActionableMomentum`
- `HomeEventStream`
- `ConditionalSection`
- `SettingsTabs`
- `VisibilityTabs`
- `OpportunityPipeline`
- `ReviewQueueLayout`
- `EmptyOnboardingPanel`

这些组件应降低页面文件长度，避免继续在 `workspace.tsx`、`seo-client.tsx`、`settings-client.tsx` 中堆叠。

## 18. 文案迁移表

| 当前文案 | 新文案 |
|---|---|
| CiteLoop service console | CiteLoop content operations |
| Connect service | Connect domain |
| Start product profile job | Build Context |
| Start SEO baseline job | Start visibility baseline |
| Run Strategist | Generate content plan |
| Running strategist | Generating plan |
| Generate | Draft this topic |
| Reconcile | Check publish status |
| Sync | Refresh visibility data |
| missing canonical_url | Live article URL not confirmed |
| canonical_url backfill | live article URL confirmation |
| qa blocking | Cannot approve: `<specific reason>` |
| Run ID | Automation record |

## 19. 分阶段交付

### Phase 1：Dashboard 空状态和 Home 控制中心

目标：

- Home 空状态压到 <= 1.2 屏。
- 空模块按条件显示。
- 稳态最多展开 2 个 active modules，其余进入 `More waiting`。
- Momentum 数字必须挂动作，零值 KPI 不默认展示。
- Home timeline 改为 recent / live / next event stream。
- Root page 和 App Shell 文案调整。
- Sidebar primary CTA 动态化。

交付：

- `Home` conditional rendering
- `PrimaryActionPanel`
- `EmptyOnboardingPanel`
- `ActionableMomentum`
- `HomeEventStream`
- `sidebarPrimaryAction` 纯函数和测试

### Phase 2：Visibility 重组

目标：

- Visibility 默认 Overview <= 1.4 屏。
- Search、AI Answers、Diagnostics 拆成 tabs。
- Brief、Opportunities、Content actions 合并成 opportunity pipeline。

交付：

- `VisibilityTabs`
- `OpportunityPipeline`
- Diagnostics tab
- 未连接 GSC 不展示 placeholder metrics

### Phase 3：Settings 拆分

目标：

- Settings 默认页只展示 General。
- Publishing、Crawl、Notifications、Activity Log 独立 tab 或子路由。

交付：

- `SettingsTabs`
- General / Publishing / Crawl / Notifications / Activity components
- Delivery history 默认折叠

### Phase 4：Content Plan、Publish、Review 精简

目标：

- Content Plan 改名和 plan health。
- Publish conditional lanes。
- Review queue/detail 两栏布局。

交付：

- Plan health
- Publish status card
- Review queue layout
- 文案迁移测试

## 20. 验收标准

### 20.1 全局验收

- 所有核心页面空状态不超过 1.2 屏，Visibility 允许 1.4 屏。该项是防失控护栏，不是唯一成功指标。
- 普通用户不需要滚动很久才能理解页面状态。
- 每个页面首屏有一个明确推荐动作或主工作区。
- 每个页面首屏最多 1 个 primary decision。
- 每个页面默认可见 section 不超过 3 个，真实数据队列除外。
- Home 稳态最多展开 2 个 conditional modules，其余收进 `More waiting`。
- 默认可见指标必须是动作入口；纯数据指标不能独立占据首屏。
- 高级信息必须可访问，但不能默认 ambient 展示。

### 20.2 自动化验收建议

新增 browser-level smoke checks。优先使用 Browser MCP；如果当前 session 没有 browser/devtools MCP，则使用项目本地 Playwright 脚本跑同等检查。

1. 在 mock empty data 下访问 Home，断言 scroll ratio <= 1.2。
2. 在 mock active data 下访问 Home，断言默认展开的 conditional modules <= 2。
3. 在 mock empty data 下访问 Home，断言没有四个 0-value KPI cards。
4. 在 mock active data 下访问 Home，断言 Momentum tile 是 link/button 或包含明确 action。
5. 在 mock active data 下访问 Home，断言 Event stream 至少能渲染 recent/live/next 三类中的两类。
6. 在 mock empty data 下访问 Context，断言只出现 setup onboarding，不出现 4 个大 empty sections。
7. 在 mock empty data 下访问 Visibility，断言 Overview 默认不出现 prompt/provider/autopilot 表格。
8. 在 mock empty data 下访问 Publish，断言只出现一个 empty action panel。
9. 在 mock settings data 下访问 Settings，断言默认只出现 General tab，或在 admin-gated 模式下入口不对普通用户可见。

### 20.3 体验验收

- 用户打开 Home 3 秒内能指出下一步动作。
- 首屏除 navigation 外只有一个主决策。
- 健康空状态表达“系统正常”，而不是“没有数据”。
- 页面通过折叠和优先级减少默认认知负担，而不是通过压缩间距牺牲留白。

### 20.4 文案验收

主流程页面不得出现：

- `Run Strategist`
- `tick`
- `canonical_url`
- `Profile JSON`
- `tokens`
- `model`
- `Run ID`

例外：

- Activity Log 的 Advanced details
- Article detail Advanced
- Visibility Diagnostics
- Admin internal pages

## 21. 埋点与成功指标

### 21.0 北极星

本轮 redesign 的北极星不是 scroll ratio，而是：

```text
用户打开 Home 3 秒内知道系统状态和下一步动作。
```

Scroll ratio、section count、KPI count 都是护栏，用于防止 dashboard 退回内容堆叠。

### 21.1 产品指标

- Context setup completion rate
- Context confirmation rate
- Review queue action rate
- Publish failure resolution rate
- Visibility opportunity accepted rate
- Opportunity to Content Plan conversion rate
- Time to first meaningful action after opening Home

### 21.2 UX 指标

- Home first meaningful action click within 10 seconds
- First-screen primary decision count
- Action-bearing metric click-through rate
- Healthy empty-state comprehension rate
- Empty-state page scroll depth
- Number of opened advanced panels per session
- Review queue completion time

### 21.3 质量指标

- No regression in existing API contract tests.
- No hidden routes broken by redirects.
- No mobile horizontal overflow.
- No button text overflow at common breakpoints.

## 22. 风险与处理

### 22.1 风险：隐藏太多导致用户找不到功能

处理：

- 用 tabs、secondary links 和 clear labels 保留入口。
- Home `Also waiting` 保留其他队列入口。
- Settings 二级导航始终可见。

### 22.2 风险：前端推导状态和后端真实状态不一致

处理：

- Phase 1 先使用前端纯函数和契约测试。
- Phase 2 或后续引入 summary endpoints。

### 22.3 风险：页面变短但信息不足

处理：

- 不是删信息，而是条件显示。
- 有异常、有待办、有真实数据时提高可见性。
- 空状态和健康状态保持克制。

### 22.4 风险：Visibility tabs 让用户找不到 SEO 能力

处理：

- 页面 subtitle 保留 `SEO + AI-answer visibility`。
- Overview status row 展示 Search data mode。
- Search tab label 使用 `Search`，不是内部 `GSC`。

## 23. 开放问题

1. Settings 访问模型是什么？Internal-only、Customer self-serve、还是 Hybrid？建议先按 Hybrid 设计权限边界，但 Phase 3 实施前必须确认。
2. Settings 使用 tab 还是子路由？建议先用 tabs，后续可映射到子路由。
3. Review 两栏布局是否需要保存用户选中的 article？建议先不持久化。
4. Home 是否需要 “This week” 始终可见？建议只有 context 和 plan 完成后显示，并且不进入首屏主结构。
5. Visibility opportunity lifecycle 是否由后端统一返回？建议先前端推导，后端聚合端点后续补齐。
6. Home live status 的数据来源是否直接读取 runs，还是由 dashboard summary 聚合？建议先前端从 recent runs 推导用户语言，后续迁移到 summary endpoint。

## 24. 最终判断

CiteLoop dashboard 的下一轮优化重点不是增加组件，而是减少默认认知负担。页面应该从“所有功能都在这里”变成“现在最重要的是这个”。当用户需要深入时，细节必须存在；但用户不需要深入时，dashboard 应保持安静。

本 PRD 的成功标准是：用户打开 CiteLoop 3 秒内知道项目状态、下一步动作和 CiteLoop 正在推进的 loop，而不是开始滚动寻找答案。
