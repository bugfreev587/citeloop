# PRD：CiteLoop Product Docs

> 范围：全局 Docs、Dashboard 内文档入口、Docs 首页、Overview 内容结构与后续文档信息架构  
> 参考：UniPost docs（`https://www.unipost.dev/docs`）的清爽文档壳、左侧目录、右侧 On This Page、路径导向式首页  
> 目标：让第一次进入 CiteLoop 的用户在 30 秒内明白“CiteLoop 如何运行、我需要做什么、系统会自动做什么”。
> 实现基线：对齐 PR #10 后的信息架构，即 `Context`、`Content Plan`、`Publish`、`Visibility`、`Settings > Activity Log`。不得按旧一级导航 `Knowledge / Topics / Publishing / SEO / Runs` 作为主文案落地。

## 0. 背景

CiteLoop 的 dashboard 信息架构正在从内部任务控制台收敛为用户工作流：Home、Context、Content Plan、Review、Publish、Visibility、Settings。用户第一次进入时仍需要自己推断：

- CiteLoop 的完整工作流是什么。
- 哪些步骤是自动化，哪些步骤需要人工确认。
- Review queue 为什么是唯一人工闸门。
- SEO/GEO、canonical、variants、evidence、Activity Log 这些概念之间是什么关系。
- 下一步应该先看 Context、生成 Content Plan，还是直接 Review。

Docs 的任务不是做开发者 API 文档，也不是营销页，而是在产品内提供一套“产品使用说明 + 工作流地图”。首页必须用 Overview 解释 CiteLoop 的运行方式，让用户一眼就知道整个 loop。

Docs 必须在创建项目之前可达。原因是最需要理解 CiteLoop 的用户，往往还没有 project id。项目内的 Docs 入口可以携带当前 project context，但通用说明本身必须由项目无关的 `/docs` 承载。

## 1. 目标

1. 提供项目无关的 `/docs`，让零项目用户也能在创建 project 之前理解 CiteLoop。
2. 在 dashboard 左下角提供稳定的 Docs 入口，位置在现有 Help 下方。
3. 提供一个 docs 首页，首屏必须包含 `Overview`，并清楚解释 CiteLoop 如何运行。
4. 让用户理解 CiteLoop 的核心循环：read domain → build context → plan content → generate drafts → evidence check → review → publish → distribute → observe visibility → feed back into Content Plan。
5. 用产品语言解释关键概念，避免暴露过多内部 agent、token、cron、DB、migration 等工程信息。
6. 复用现有 dashboard 的视觉体系，同时借鉴 UniPost docs 的文档布局纪律：左侧文档目录、中间正文、右侧 On This Page、简洁路径卡片。

## 2. 非目标

- V1 不做完整 API Reference。
- V1 不做全文搜索、`⌘K` 搜索或 Algolia 类文档搜索。
- V1 不做多语言切换；页面文案使用英文，以匹配当前 dashboard UI。PRD 本身使用中文。
- V1 不做 CMS 驱动的文档系统；可以先用静态 TSX/MDX 风格内容实现。
- V1 不替代 Help/support；Help 仍保留为支持入口，Docs 是产品说明入口。
- V1 不展示敏感工程调试信息，例如 provider secrets、token 明细、数据库状态。

## 3. 信息架构基线

Docs 的页面文案与链接必须按 PR #10 后的新 IA 命名：

| 用户可见主名 | Project route | 说明 |
|---|---|---|
| Home | `/projects/[id]` | 每日 next action 与 loop momentum |
| Context | `/projects/[id]/context` | 产品认知、证据、crawl/context health |
| Content Plan | `/projects/[id]/plan` | 选题、机会、排期和生成意图 |
| Review | `/projects/[id]/review` | 唯一人工审核闸门 |
| Publish | `/projects/[id]/publish` | canonical 发布与 variants 分发 |
| Visibility | `/projects/[id]/visibility` | SEO + GEO 用户结果层 |
| Settings | `/projects/[id]/settings` | cadence、budget、publisher、automation 配置 |
| Activity Log | `/projects/[id]/settings/activity` | 原 Runs 的高级审计与排障入口 |

旧名只可作为兼容说明出现，例如 `Context, formerly Knowledge`。Docs 不应主导用户去找 `Knowledge`、`Topics`、`Publishing`、`SEO` 或一级 `Runs`。

落地顺序：

1. 先确保实现分支已同步 PR #10 的新 IA。
2. 再落地 Docs。
3. 如果必须在旧工作树上开发 Docs，PRD 仍以新 IA 为准，旧 route 只能作为临时兼容 alias。

## 4. 入口与路由

### 4.1 Dashboard sidebar 入口

PR #10 后的 dashboard sidebar 主导航：

1. Home
2. Context
3. Content Plan
4. Review
5. Publish
6. Visibility
7. Settings

Sidebar 底部结构：

1. Budget
2. Help
3. Project/account card

新增 Docs 后结构为：

1. Budget
2. Help
3. Docs
4. Project/account card

Docs item 规格：

- Label：`Docs`
- Icon：`BookOpen`（lucide-react）
- Route：优先指向 `/docs`；若实现 project-scoped docs wrapper，可指向 `/projects/[id]/docs`，但内容源必须与 `/docs` 共享。
- Active state：当 pathname 是 `/docs`、以 `/docs/` 开头，或以 `/projects/[id]/docs` 开头时高亮。
- 桌面端必须位于 Help 下方，account card 上方。
- 移动端应在 horizontal nav 中出现 `Docs` chip，避免移动端无法访问。
- Docs 与 Settings 不应竞争一级主导航位置。Settings 保持主导航；Docs 保持 sidebar footer 的学习入口。

### 4.2 Docs 路由

P0 路由：

| Route | 用途 |
|---|---|
| `/docs` | 项目无关 Docs 首页 + Overview，零项目用户可达 |
| `/projects/[id]/docs` | 可选 project-scoped wrapper，共用 `/docs` 内容，但 CTA 深链到当前 project |

P1 可扩展路由：

| Route | 用途 |
|---|---|
| `/docs/getting-started` | 新项目启动与首次设置 |
| `/docs/context` | Context / Evidence 说明 |
| `/docs/content-plan` | Content Plan、机会、排期、选题策略说明 |
| `/docs/review` | Review gate 与 QA blocking 说明 |
| `/docs/publish` | Canonical 自动发布与 variants / syndication 分发说明 |
| `/docs/visibility` | SEO/GEO visibility、AI crawler、observations 说明 |
| `/docs/settings` | Cadence、budget、automation、crawl、publisher settings |
| `/docs/activity-log` | Activity Log / degraded / failure audit 说明 |

V1 可以先实现单页 docs 首页，通过 anchor sections 覆盖 P0 内容，并保留未来拆页的信息架构。

### 4.3 零项目可达性

`/docs` 是 canonical docs route。它不依赖 project id。

零项目状态下：

- `/` 的项目列表页应提供 `Read the docs` 或等价入口。
- `/docs` 的 CTA 使用通用动作，例如 `Create your first project`。
- 没有 project id 时，Docs 不显示 project-scoped links，或将它们降级为解释性文本。

有项目状态下：

- Project sidebar 的 Docs 入口打开 docs。
- Docs CTA 可深链到当前 project 的 Context、Content Plan、Review、Publish、Visibility、Settings。
- 如果实现 `/projects/[id]/docs`，它应复用同一个 `DocsContent`，只替换 CTA link builder。

## 5. Docs 首页内容

Docs 首页第一屏必须是 `Overview`。标题不应是营销式 headline，而应直接告诉用户这是产品工作流说明。

### 5.1 首屏结构

首屏内容建议：

- Eyebrow：`CiteLoop Docs`
- H1：`Overview`
- Subtitle：`How CiteLoop turns your domain into evidence-backed SEO and GEO content.`
- 一个显式闭环图，而不是线性结束的 flow strip：
  1. `Read your domain`
  2. `Build context`
  3. `Plan content`
  4. `Generate drafts`
  5. `Check evidence`
  6. `Review once`
  7. `Publish and distribute`
  8. `Measure visibility`
  9. `Feed opportunities back into the plan`
- 首屏右侧或下方显示 “Your role / CiteLoop role” 对照：
  - You provide the domain, confirm context, and approve drafts.
  - CiteLoop crawls, plans, writes, checks evidence, publishes canonical content, and prepares distribution variants.

验收标准：

- 用户不滚动或少量滚动即可看到 Overview 和完整 flow。
- 用户能立刻理解 CiteLoop 不是单纯写作工具，而是一个带证据、审核、发布、可见性反馈的内容运营 loop。
- `Measure visibility` 必须通过箭头或 cycle layout 回到 `Plan content` / `Content Plan`，让 loop 被看见。
- 首屏不能是营销 hero，不能使用过大的装饰视觉。

### 5.2 Overview 正文必须回答的问题

Overview 必须回答以下问题：

1. **CiteLoop 需要用户输入什么？**  
   一个 landing page 或 domain 起步；用户后续确认产品事实、审核内容、处理 publish/distribution 事项。

2. **CiteLoop 自动做什么？**  
   读取公开页面，建立 Product Profile、Content Inventory 和 Evidence library；生成 Content Plan；写 canonical article 与 syndication variants；运行 QA evidence check；到点自动发布 canonical；解锁 variants；记录 SEO/GEO visibility。

3. **为什么有 Review？**  
   Review 是唯一人工闸门。CiteLoop 可以自动准备内容，但可发布内容必须通过用户 approval。QA blocking 的内容必须先解决证据问题，不能直接 approve。

4. **什么是 canonical 与 variants？**  
   Canonical 是发布到主博客或主站的正本。Variants 是给 Dev.to、Hashnode、LinkedIn、Reddit/HN 等渠道的改写稿。Variants 只有在 canonical 真实 URL 回填后才解锁。

5. **SEO/GEO 在 CiteLoop 里怎么闭环？**  
   SEO/GEO 不是单独的报告页，而是观察哪些内容、prompt、surface 正在被搜索引擎和 answer engine 发现，再把机会回流到 Content Plan。

6. **系统状态如何判断？**  
   Home 是“此刻该做什么”的唯一真源；Context 告诉用户系统理解了什么；Review 告诉用户哪些草稿需要确认；Publish 告诉用户哪些内容已发布或待分发；Visibility 告诉用户 SEO/GEO 信号；Settings > Activity Log 只用于查失败、降级和成本审计。

### 5.3 Docs 首页 section 列表

P0 首页包含以下 sections：

1. `Overview`
   - 解释完整运行流程。
   - 包含 loop diagram。
   - 包含 user role vs CiteLoop role。

2. `Start here`
   - 三个路径卡片，参考 UniPost docs 的 “Choose the fastest path”：
     - `Set up context`：输入 domain，确认产品认知与 evidence。
     - `Create a content plan`：生成 topic backlog 和 schedule。
     - `Review and publish`：审核 drafts，等待 canonical 自动发布，分发 variants。

3. `Core concepts`
   - Project
   - Context / Product Profile
   - Evidence
   - Content Plan / Topic
   - Canonical
   - Variant
   - Distribution / Syndication
   - Review gate
   - Visibility
   - Budget / Automation
   - Activity Log

4. `Workflow model`
   - Docs 只解释工作流类别，不复述 Home 的 next-action 优先级规则。
   - Home 是“现在该做什么”的唯一真源。
   - Docs 解释常见 action 会落在哪些页面：Context、Content Plan、Review、Publish、Visibility、Settings。
   - 任何动态优先级、blocked reason 或 project-specific recommendation 都应由 Home / 页面空状态实时生成。

5. `Pages in the dashboard`
   - Home：next action 和 momentum。
   - Context：产品认知与证据。
   - Content Plan：内容计划、机会和排期。
   - Review：唯一人工闸门。
   - Publish：canonical 发布与 variants 分发。
   - Visibility：SEO/GEO visibility 与 crawler / answer-engine 信号。
   - Settings：cadence、budget、publisher、crawl boundaries。
   - Settings > Activity Log：失败、降级、成本审计。

6. `Common states and signals`
   - 解释稳定概念，不手抄易变的内部 status 字符串。
   - 推荐分组：needs review、evidence blocked、scheduled/published、ready to distribute、visibility degraded、human decision needed、activity log entry。
   - 如果页面必须展示精确 status code，文案应从共享常量或现有状态映射生成，不在 docs 中维护第二份清单。
   - 示例可以出现 `provider unavailable`、`degraded` 等用户可见概念，但必须标明它们不是负面 visibility 结论。

7. `Limits and expectations`
   - CiteLoop 读取公开页面，不绕过登录或 robots 限制。
   - 没有 Search Console / GA4 时，不伪造 CTR、position、conversion。
   - Answer-engine observations 可能降级；provider unavailable 不等于 visibility 失败。
   - V1 的 third-party syndication 是半自动。

8. `Next steps`
   - 链接到 dashboard 内对应页面。
   - 有 project id 时使用具体动作：Open Context、Open Content Plan、Open Review、Open Publish、Open Visibility。
   - 没有 project id 时使用通用动作：Create your first project。
   - 不做营销 CTA。

## 6. 文档布局与视觉风格

### 6.1 总体风格

参考 UniPost docs 的结构，不复制其品牌视觉。CiteLoop docs 应保持 dashboard 的克制工作台风格：

- 背景沿用 `bg-stone-100`。
- 内容面使用白色、细边框、轻量分隔线。
- 卡片 radius 不超过现有体系，建议 `rounded-lg` 或 `rounded-xl`。
- 不使用装饰性渐变 hero。
- 不做大面积 marketing copy。
- 文档正文密度适中，适合快速扫读。

### 6.2 Docs page layout

桌面端：

- 全局 `/docs`：使用轻量 docs chrome，包含 CiteLoop 标识、返回 dashboard / create project 的入口，不需要 ProjectShell。
- Project-scoped `/projects/[id]/docs`：可使用 dashboard sidebar（既有）。
- Docs 内容内部再分三列：
  - Docs nav：约 190px，列出 Overview、Start here、Core concepts 等 anchors。
  - Main content：约 680px 到 760px，承载文档正文。
  - On This Page：约 180px，sticky，显示当前页 anchors。
- 总宽应适配现有 `max-w-5xl` 或略放宽，但不能变成全宽 dashboard。

移动端：

- Dashboard mobile nav 中显示 Docs；全局 `/docs` 移动端提供返回首页的入口。
- Docs 内部左侧目录和 On This Page 收起。
- Main content 单列。
- Loop diagram 可横向滚动、改成纵向 steps，或用 compact cycle；不得造成页面级横向溢出。

### 6.3 组件

P0 可使用本地轻量组件：

- `DocsLayout`
- `DocsNav`
- `DocsOnThisPage`
- `DocsSection`
- `WorkflowStep`
- `ConceptCard`
- `StatusRow`
- `PathCard`

如能保持简单，也可以先在 `/docs/page.tsx` 内局部实现，后续拆组件。

## 7. 内容语气

Docs 文案应符合以下原则：

- 使用用户语言，不使用内部任务名作为主要标题。
- 可以解释 Insight、Strategist、Writer、QA，但要把它们翻译成用户能理解的能力：read, plan, write, check evidence。
- 避免承诺无法验证的增长结果。
- 对 unavailable / degraded / no data 状态保持诚实。
- 强调 evidence-backed：CiteLoop 不是凭空生成内容，而是使用 domain facts 和 evidence。
- 不把 Home next-action 规则复制进 docs。Home 是动态行动建议的唯一真源，Docs 解释为什么会出现这些行动。
- 不维护内部 status 字符串清单。Docs 解释稳定的用户可见概念；精确状态标签由代码常量或页面状态映射提供。

建议用词：

- `Context` 作为主名；旧 `Knowledge` 只作为兼容备注。
- `Content Plan` 作为主名；旧 `Topics` 只作为兼容备注。
- `Publish` 作为主名；旧 `Publishing` 只作为兼容备注。
- `Visibility` 作为主名，解释它覆盖 SEO + GEO。
- `Review gate` 用来解释唯一人工闸门。
- `Activity Log` 用来解释旧 Runs 的高级排障和审计能力。

## 8. 实现范围

### P0

1. 确保实现分支已同步 PR #10 的新 IA，或至少以新 route/name 作为 Docs 文案基线。
2. 新增项目无关 `/docs` 页面。
3. 在 root `/` 的零项目状态或项目创建区域提供 docs 入口，让首次用户创建 project 前可达。
4. 在 project sidebar footer 的 Help 下方新增 Docs 入口。
5. 在 mobile dashboard nav 中新增 Docs。
6. 可选新增 `/projects/[id]/docs` wrapper；如新增，必须复用 `/docs` 内容。
7. 页面首屏包含 Overview 和完整 CiteLoop loop diagram。
8. 页面包含 Start here、Core concepts、Workflow model、Dashboard pages、Common states and signals、Limits and expectations、Next steps。
9. 页面有 desktop docs nav 与 On This Page，mobile 单列可读。
10. CTA 在有 project id 时指向当前 project route；无 project id 时指向 create project 或通用说明。

### P1

1. 拆分多页 docs。
2. 增加搜索。
3. 增加 in-product contextual docs，例如 Review 页面链接到 Review docs anchor。
4. 增加空状态里的 docs deep link。
5. 增加 API / webhook / publisher setup reference（如果 CiteLoop 后续开放开发者集成）。
6. 从共享状态映射生成 status glossary，避免手写状态清单漂移。

## 9. 测试与验收

### 9.1 自动验证

- TypeScript typecheck 通过。
- Next.js build 通过。
- 若新增 route/component contract test，应覆盖：
  - `/docs` 对零项目用户可访问，不需要 project id。
  - Root `/` 提供 docs 入口。
  - ProjectShell 包含 Help 下方的 Docs link。
  - Docs route 的主要 section 文案存在。
  - Docs 使用 PR #10 后的新 IA 文案：Context、Content Plan、Publish、Visibility、Settings > Activity Log。
  - Docs CTA 在 project-scoped route 使用当前 project id 拼 route；在 global route 使用通用 CTA。
  - Docs 不把旧 IA 作为主导航文案。

### 9.2 浏览器验收

桌面端：

- 左下角 Help 下方可见 Docs。
- 点击 Docs 后进入 docs 页面。
- 从没有项目的 `/` 页面也能进入 `/docs`。
- Overview 在首屏可见。
- Loop diagram 不换行挤压、不遮挡，并能看出 Visibility 回流到 Content Plan。
- Docs nav 与 On This Page 在桌面可见且不与正文重叠。

移动端：

- Mobile nav 可进入 Docs。
- Overview、loop diagram、路径卡片单列可读。
- 不出现水平页面溢出；仅 flow strip 如设计为横滑时允许局部横滑。

内容验收：

- 用户能在 30 秒内复述 CiteLoop 的运行方式。
- 用户能分辨自己负责什么、CiteLoop 自动做什么。
- 用户能理解 Review 是唯一人工闸门。
- 用户能理解 canonical 和 variants 的关系。
- 用户能理解 SEO/GEO visibility 如何回流到 content plan。
- 用户不会被引导去找旧的一级 Runs 页。
- 用户不会把 docs 当成“此刻 next action”的动态真源；这件事由 Home 负责。

## 10. Decisions And Open Questions

已决策：

1. Docs V1 页面文案使用英文，跟随 dashboard UI。
2. Docs 主 IA 对齐 PR #10：Context、Content Plan、Publish、Visibility、Settings > Activity Log。
3. `/docs` 是 canonical docs route，必须支持零项目访问。
4. Overview 必须画成 loop，而不是线性流程。
5. Home 是动态 next action 的唯一真源；Docs 不复制 Home 的优先级规则。

仍开放：

1. Help 入口是否仍链接到 `/`，还是后续要改成 support/contact？
2. 是否需要把 docs 入口也放到 Home 顶部的 learning resources 折叠条中？P0 可不做，但这是自然的 P1。
3. `/projects/[id]/docs` 是否作为真实 route 存在，还是 project sidebar 直接链接到 `/docs`？两者都可以，但不能牺牲 `/docs` 的零项目可达性。
