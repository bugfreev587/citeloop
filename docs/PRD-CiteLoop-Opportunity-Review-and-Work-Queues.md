# PRD: CiteLoop Opportunity Review and Work Queues

> 日期: 2026-07-05
> 状态: Draft
> 范围: Analysis 页面信息架构、Opportunity Queue、Site Fixes、Content Plan 分流、Results Watchlist、用户可见文案
> 上游文档:
> - `docs/PRD-CiteLoop-Analysis-Workflow.md`
> - `docs/PRD-CiteLoop-Analysis-Review-Decision-Drawer-Redesign.md`
> - `docs/PRD-CiteLoop-Visibility-Analysis-to-Content-Loop.md`

## 0. 摘要

当前 Analysis 页面把“待用户决策的机会”和“已经进入执行的工作”放在同一个视图里，而且顶部先展示 metrics board，再展示 Direct Action queue，最后才是 Opportunity Queue。这会让用户产生错误心智: 好像系统已经把一部分工作推进执行了，用户只是事后检查。

本 PRD 将 Analysis 的用户心智简化为:

```text
Opportunity Queue = 待用户决策的机会
Content Plan = 已批准的内容和页面更新工作
Site Fixes = 已批准的网站修复工作
Results = 已执行工作的效果和复盘
```

核心 IA 调整:

```text
Analysis 页面
1. Opportunity Queue
2. Site Fixes
3. Loop status / automation / diagnostics 等低优先级状态
```

顶部 `What needs review next` metrics board 应移除。Opportunity Queue 放到最顶上，成为 Analysis 的首要工作区。原 Direct Action queue 改名为 Site Fixes，并放在 Opportunity Queue 下方，因为它是 approval 之后的执行队列，不是用户第一步要看的决策入口。

核心产品规则:

```text
系统发现的工作，在进入 Content Plan、Site Fixes 或 Results Watchlist 前，必须有明确 approval source。
```

approval source 可以来自用户手动 review、明确配置过的 Autopilot policy、用户手动创建任务、或已批准工作的 retry / recovery。

另一个关键 UX 规则:

```text
上一阶段的 card 完成后不立刻消失，而是先变成指向下一阶段的 link card。
```

这样用户不会在 approve、create、submit 之后丢失上下文。

## 1. 背景和问题

### 1.1 当前用户为什么会晕

当前页面要求用户理解太多内部概念:

- opportunity
- finding
- content action
- direct action
- topic
- reviewable output
- loop stage
- measurement queue

这些概念对工程有用，但对用户过载。用户看到的体验是:

1. 在 Opportunity Queue 里点击一个 finding。
2. CTA 可能叫 `Create Content` 或类似创建动作。
3. 点击后却不一定出现在 Content Plan。
4. 它可能进入 Direct Action queue。
5. Direct Action queue 还出现在 Opportunity Queue 上方，视觉顺序与实际流程相反。

这违反了最简单的用户流程:

```text
先决定要不要做
再把批准后的工作放进正确队列
最后执行并衡量结果
```

### 1.2 Direct Action 是工程词，不是产品词

`Direct Action` 描述的是实现方式，不是用户目的。用户不是来“review direct action”的。用户想做的是:

- 创建内容
- 改进页面
- 修网站问题
- 观察结果

因此 Direct Action 不应作为用户可见的一等产品概念。用户可见名称应改为 `Site Fixes`。

### 1.3 Create Content 作为通用 CTA 会误导

不是所有 Opportunity 都会创建内容。有些是:

- schema fix
- internal link fix
- crawler / robots / canonical fix
- sitemap fix
- metadata update
- watchlist item

如果 CTA 叫 `Create Content`，用户会自然预期它出现在 Content Plan。实际没有出现时，用户会认为系统丢数据或流程坏了。

## 2. 产品目标

1. Opportunity Queue 成为 Analysis 页面首个主要工作区。
2. 移除顶部 `What needs review next` metrics board。
3. 用户可见文案中将 Direct Action 改为 Site Fixes。
4. Site Fixes 放在 Opportunity Queue 下方。
5. 每一个系统发现的 execution item 都必须有 approval source。
6. 每张 Opportunity 卡片在用户点击前必须显示 work type、destination 和 next step。
7. 用 destination-specific CTA 取代通用 `Create Content`。
8. 每个阶段完成后，当前阶段 card 先变成 link card，指向下一阶段的对应 item。
9. 保留现有后端模型和数据表，先修正用户心智和页面分流。
10. 将用户可见 Opportunity 类型压缩为少量 outcome-oriented work types。

## 3. 非目标

- 不在本 PRD 中重命名内部 `content_actions` 数据表。
- 不要求先重构所有后端模型才能改善 UX。
- 不把 Analysis 做成完整 SEO analytics dashboard。
- 不在 Opportunity Queue 中展示完整执行历史、发布历史或 measurement history。
- 不允许系统自动执行高风险站点改动，除非用户显式授权 policy。
- 不要求用户理解 `metadata_rewrite`、`schema_patch`、`technical_fix` 等内部 asset type。
- 不继续把 Direct Action 作为用户可见产品词。

## 4. 用户心智模型

### 4.1 Opportunity Queue

Opportunity Queue 是决策 inbox。

它回答:

- CiteLoop 发现了什么?
- 为什么重要?
- 证据来自哪里?
- 这会创建哪类工作?
- approve 后会去哪里?

Opportunity Queue 不应该把已批准执行项作为首要内容展示。

### 4.2 Content Plan

Content Plan 是已批准的内容和页面更新队列。

它包含:

- 新内容资产。
- 现有页面 refresh。
- evidence expansion。
- comparison page。
- alternative page。
- GEO / answer-ready owned asset。
- 需要 editorial context 的 metadata / page update。

Content Plan 不展示未 review 的原始 opportunities。

### 4.3 Site Fixes

Site Fixes 是已批准的网站修复队列。

它包含:

- schema fix。
- internal link fix。
- sitemap fix。
- robots / crawler access fix。
- canonical fix。
- indexability blocker。
- technical visibility blocker。

Site Fixes 取代用户可见的 Direct Action queue。

### 4.4 Results

Results 是结果和复盘页面。

它包含:

- 已发布内容的 outcome。
- 已应用 site fix 的 outcome。
- watchlist item。
- measurement window。
- positive / negative / inconclusive / waiting 状态。

Results 不负责审批新的 opportunities。

## 5. Approval Source 模型

### 5.1 核心规则

所有系统发现的工作，在进入执行队列之前必须有 approval source。

```text
System finding
-> Opportunity Queue
-> Approval source
-> Content Plan / Site Fixes / Results Watchlist
```

### 5.2 Approval Source 类型

| Source | 含义 | 是否必须经过 Opportunity Queue | 说明 |
|---|---|---:|---|
| Human opportunity approval | 用户 review 并 approve 系统 finding | 是 | 默认路径 |
| Autopilot policy approval | 用户显式配置 policy，允许某类低风险工作自动批准 | 单条不需要，但 policy 本身需要批准 | UI 必须展示 policy source |
| Manual task creation | 用户手动创建 content item 或 site fix | 否 | 用户创建动作本身就是 approval |
| Retry / recovery | 系统重试之前已批准的工作 | 否 | 必须保留原始 approval source |
| Admin / imported work | staff 或 admin tooling 创建的工作 | 否 | 如果对用户可见，需显示来源 |

### 5.3 用户可见 approval 文案

Opportunity detail 必须显示下列含义之一:

- `Approve to send this to Content Plan.`
- `Approve to create a Site Fix.`
- `Approve to watch this in Results.`
- `Approved by Autopilot policy: low-risk site fixes.`
- `Created manually by user.`

产品不能在没有 approval source 的情况下，把系统发现的 finding 静默推进 Content Plan、Site Fixes 或 Results Watchlist。

## 6. 用户可见 Work Types

用户不应该看到完整内部 taxonomy。用户只需要看到四类 work type。

| Work type | 用户含义 | Primary CTA | Destination |
|---|---|---|---|
| Create Content | 创建新资产、comparison page、alternative page 或 answer-ready page | Add to Content Plan | Content Plan |
| Improve Page | 更新现有页面、refresh 内容、补强 evidence、改 title/meta | Create Page Update | Content Plan |
| Fix Site Issue | 修 schema、internal links、crawler access、canonical、robots、sitemap、indexing | Create Site Fix | Site Fixes |
| Watch Result | 暂不改动，只观察信号或结果 | Watch in Results | Results Watchlist |

### 6.1 内部类型映射

| Existing internal signal | User work type | Destination |
|---|---|---|
| `gsc_low_ctr_query` | Improve Page | Content Plan |
| `gsc_query_gap` | Improve Page 或 Create Content | Content Plan |
| `gsc_striking_distance_query` | Improve Page | Content Plan |
| `gsc_content_decay` | Improve Page | Content Plan |
| `thin_evidence_page` | Improve Page | Content Plan |
| `cold_start_context_plan` | Create Content | Content Plan |
| `cold_start_competitive_gap` | Create Content | Content Plan |
| `cold_start_evidence_page` | Improve Page 或 Create Content | Content Plan |
| `geo_competitor_cited_project_absent` | Create Content | Content Plan |
| `geo_project_mentioned_without_citation` | Create Content | Content Plan |
| `internal_link_gap` | Fix Site Issue | Site Fixes |
| `schema_gap` | Fix Site Issue | Site Fixes |
| `technical_visibility_issue` | Fix Site Issue | Site Fixes |
| `gsc_query_cannibalization` | Improve Page | Content Plan |
| `geo_crawler_access_blocked` | Fix Site Issue | Site Fixes |

当内部类型可能对应多种 work type 时，由 recommended action 和 evidence 决定最终 route。用户只看到最终 work type 和 destination。

## 7. Analysis 页面 IA

### 7.1 必须采用的页面顺序

Analysis 页面顺序:

```text
1. Page header + compact data status
2. Opportunity Queue
3. Site Fixes
4. Loop status / automation / diagnostics
```

### 7.2 移除顶部 Metrics Board

移除当前顶部 board:

```text
Start here
What needs review next
2 direct actions
1 opportunity available
```

原因:

- 它重复了下方真实队列的信息。
- 它把 execution signal 放到 decision queue 上方。
- 它增加了用户学习成本。
- 它让用户以为 Direct Action 是第一优先级。

用户应该直接从 Opportunity Queue 开始，而不是从 summary cards 再跳转。

### 7.3 Opportunity Queue 要求

Opportunity Queue 卡片必须展示:

- Work type: Create Content、Improve Page、Fix Site Issue、Watch Result。
- Priority。
- Evidence source。
- Human-readable reason。
- Destination line。
- Primary CTA。
- Secondary actions: dismiss、snooze、view evidence。

示例:

```text
Work type: Fix Site Issue
Finding: Structured data is missing on the product page
Why now: The latest crawl found no JSON-LD on a page that should expose product facts.
Approve sends this to: Site Fixes
CTA: Create Site Fix
```

### 7.4 Site Fixes 要求

Site Fixes 放在 Opportunity Queue 下方。

Site Fixes 只展示已批准或 policy-approved 的修复工作，不展示未 review 的 findings。

Site Fixes 卡片必须展示:

- Source opportunity 或 approval source。
- Fix type。
- Target URL。
- Status。
- Risk level。
- Next review / execution step。

示例:

```text
Site Fix
Schema update for /product
Source: Approved opportunity
Status: Waiting for review
Next step: Review suggested schema patch
```

### 7.5 Empty States

Opportunity Queue empty state:

```text
No opportunities need review
CiteLoop will add new opportunities here after the next analysis run.
```

Site Fixes empty state:

```text
No site fixes waiting
Approved technical fixes will appear here after you approve a site issue.
```

## 8. Stage-to-Stage Handoff Links

### 8.1 核心交互原则

用户完成一个阶段动作后，当前 card 不应立刻消失。它应该先从 action card 变成 link card，明确告诉用户工作已经被送到哪里，并允许用户点击跳到下一阶段的对应 item。

```text
上一阶段不是垃圾桶。
上一阶段是追踪入口。
```

这解决两个 UX 问题:

- 用户知道刚刚 approve / create / submit 的东西去了哪里。
- 用户可以从上游页面回到当前工作的最新位置。

### 8.2 Opportunity Queue Handoff

Opportunity Queue 中的 card 有两种主要交互状态:

| State | Visual treatment | Click behavior | Allowed actions |
|---|---|---|---|
| Needs decision | 明显的待处理状态，可使用红色或高优先级边框，但不能只依赖颜色 | 打开 opportunity review drawer | approve、dismiss、snooze、view evidence |
| Sent downstream | 成功态，可使用绿色边框和 destination badge | 跳转到下一阶段的对应 item | view linked item，不允许再次 approve |

颜色只能作为辅助。卡片必须同时显示文字状态，例如:

- `Needs decision`
- `Sent to Content Plan`
- `Sent to Site Fixes`
- `Watching in Results`

### 8.3 Opportunity Queue 移除时机

一个 approved opportunity 不应在用户点击 approve 后立刻从 Opportunity Queue 消失。

推荐规则:

```text
Opportunity approve
-> Opportunity card 变成 link card
-> 指向 Content Plan / Site Fixes / Results Watchlist 的对应 item
-> 当下一阶段 item 被用户进一步处理并进入再下一阶段后，Opportunity Queue 中的 card 才从默认队列移除
```

示例:

```text
Opportunity Queue
-> Add to Content Plan
-> Opportunity card 变成 "View in Content Plan"
-> Content Plan item 被 Create / Generate 后进入 Review
-> Opportunity card 从默认 Opportunity Queue 移除，进入 reviewed/history
```

Site Fixes 和 Results Watchlist 也遵循同样原则: Opportunity card 先变成 link，再在下游 item 明确接手后从默认 decision queue 移除。

### 8.4 Content Plan Handoff

Content Plan item 被 create / generate / submit 后，不应立刻消失。它应先变成 link card，指向 Review 页面中的对应 draft 或 review item。

示例:

```text
Content Plan
-> Create draft
-> Content Plan card 变成 "View in Review"
-> 点击跳到 Review 页面对应 draft
```

当 Review 明确接手后，Content Plan 可以将该 item 从 active planning list 移到 completed / sent-forward / history。

### 8.5 Review 和 Publish Handoff

Review 和 Publish 也应遵循相同模式:

```text
Review
-> Approve
-> Review card 变成 "View in Publish"

Publish
-> Publish / Apply
-> Publish card 变成 "View Results"
```

完成动作后的 card 不再打开原 drawer，也不再允许重复执行原动作。它只承担追踪和跳转作用。

### 8.6 链路示例

完整链路:

```text
Opportunity Queue
-> approve
-> Opportunity card links to Content Plan

Content Plan
-> create / generate
-> Content Plan card links to Review
-> Opportunity card may leave default queue

Review
-> approve
-> Review card links to Publish

Publish
-> publish / apply
-> Publish card links to Results
```

### 8.7 Link Card Requirements

Link card 必须展示:

- Current stage status。
- Destination label。
- Destination item title。
- Last completed action。
- Timestamp 或 relative time。
- Clear CTA，例如 `View in Content Plan`、`View in Review`、`View in Publish`、`View Results`。

Link card 不允许展示已经失效的 primary action。例如:

- 已 sent to Content Plan 的 opportunity 不再显示 `Add to Content Plan`。
- 已 sent to Review 的 Content Plan item 不再显示 `Create draft`。
- 已 sent to Publish 的 Review item 不再显示 `Approve`。

### 8.8 Same-Page Linked Item Focus Behavior

当 link card 指向同一页面内的下游区域时，点击后不能只改变 URL 或轻微滚动。系统必须明确指出目标 card。

典型场景:

```text
Opportunity Queue card
-> View in Site Fixes
-> Site Fixes 区域中的对应 Site Fix card
```

点击同页链接后的行为:

```text
1. Smooth scroll 到目标区域。
2. 如果目标区域折叠，则自动展开。
3. 将目标 card 滚到可视区域顶部附近。
4. 在目标区域内临时将目标 card 置顶，或保持排序但确保目标 card 完整可见。
5. 目标 card 获得 focus ring。
6. 目标 card 背景或边框柔和 pulse 两次。
7. 2-3 秒后恢复正常视觉状态。
```

注意: 不应把整个 Site Fixes 区域移动到 Opportunity Queue 上方。页面 IA 仍然保持 Opportunity Queue 在上、Site Fixes 在下。被强调的是目标 Site Fix card，而不是整个 section 的位置。

可接受的视觉表达:

- 柔和背景高亮两次。
- 边框 pulse 两次。
- 短暂 focus ring。
- 短暂 `Linked from Opportunity` label。

不可接受的视觉表达:

- 强烈闪屏。
- 页面大幅跳动。
- 长时间改变排序导致用户以为队列真实优先级变了。
- 只依靠颜色，不提供文字或 focus 状态。

Accessibility requirements:

- 如果用户开启 `prefers-reduced-motion`，不做 pulse 动画，改用静态高亮和 focus ring。
- 点击 link 后焦点必须移动到目标 card 或目标 card 内的 heading。
- 目标 card 必须有稳定 anchor / id，便于深链接和浏览器返回。
- 如果目标 card 被 filter、tab、pagination 隐藏，系统应自动切换到可见状态，或显示明确 fallback message。
- 如果目标 item 已不存在，link card 应显示 stale state，例如 `This item moved or was completed`，并提供进入目标页面的安全入口。

## 9. CTA 规则

Opportunity Queue 不允许使用 generic creation CTA。

不要使用:

- Create Content
- Create Action
- Review Direct Action
- Add to loop

改用:

| Destination | CTA |
|---|---|
| Content Plan, new content | Add to Content Plan |
| Content Plan, existing page update | Create Page Update |
| Site Fixes | Create Site Fix |
| Results Watchlist | Watch in Results |
| Archive | Dismiss |

每个 CTA 必须和 route 一致。

## 10. Status 语言

### 10.1 Opportunity Status

Opportunity Queue 使用:

- Needs decision
- Approved
- Sent to Content Plan
- Sent to Site Fixes
- Watching in Results
- Dismissed
- Snoozed

### 10.2 Work Queue Status

Content Plan 和 Site Fixes 使用:

- Planned
- Sent to Review
- Waiting for review
- Approved
- Sent to Publish
- In progress
- Applied
- Sent to Results
- Measuring
- Blocked

### 10.3 Results Status

Results 使用:

- Waiting for data
- Measuring
- Positive signal
- Negative signal
- Inconclusive
- Learned

## 11. Data 和 API Implications

本 PRD 不要求立即重命名已有数据库表。

建议实现方式:

- 保留 `seo_opportunities` 作为系统发现 findings 的来源。
- 保留 `content_actions` 作为内部 execution bridge。
- 对外暴露用户可理解的 route: Content Plan、Site Fixes、Results Watchlist。
- 对每个 execution item 增加或推导 approval source。
- 对每个已送出的 item 增加或推导 next destination link。
- 将 Direct Action 作为 internal-only concept。
- 用 Site Fixes 作为 approved technical / site work 的用户可见 surface。

如果 approval source 无法从现有字段可靠推导，则应添加显式字段或 API DTO 属性，而不是只靠前端文案猜测。

建议 API-facing concepts:

```text
work_type: create_content | improve_page | fix_site_issue | watch_result
destination: content_plan | site_fixes | results_watchlist
approval_source: human_review | autopilot_policy | manual | retry_recovery | admin_import
approval_source_label: user-facing sentence
current_stage: opportunity | content_plan | review | publish | results
next_destination: content_plan | site_fixes | review | publish | results | null
next_entity_id: uuid | null
next_entity_label: user-facing sentence
next_entity_anchor: stable same-page anchor | null
same_page_focus_behavior: scroll_and_highlight | page_navigation | none
```

## 12. Autopilot 规则

Autopilot 只有在用户显式批准 policy 后，才能跳过逐条 Opportunity Queue review。

Autopilot UI 必须展示:

- 它可以批准哪类 work。
- risk limit。
- destination。
- 最近由 policy 批准的 items。
- kill switch 或 pause control。

示例:

```text
Approved by Autopilot policy
Policy: Low-risk site fixes
Destination: Site Fixes
```

Autopilot 不允许让高风险站点改动对用户不可见。

## 13. Acceptance Criteria

### 13.1 Analysis Page

1. 顶部 `What needs review next` metrics board 被移除。
2. Opportunity Queue 是 Analysis 页面第一个主要 work surface。
3. Site Fixes 位于 Opportunity Queue 下方。
4. 用户可见文案不再出现 Direct Action 或 Direct Action Queue。
5. 非内容工作不再使用 generic `Create Content` CTA。
6. Opportunity card 在 approve 前显示 destination。
7. Site Fixes 只展示 approved 或 policy-approved site-fix work。

### 13.2 Routing

1. Create Content opportunities route to Content Plan。
2. Improve Page opportunities route to Content Plan。
3. Fix Site Issue opportunities route to Site Fixes。
4. Watch Result opportunities route to Results Watchlist。
5. Dismissed opportunities 不进入执行队列。
6. 每个 execution item 都有 approval source。

### 13.3 Handoff Links

1. Opportunity approve 后，Opportunity card 不立刻消失。
2. Approved opportunity card 变成 link card，指向 Content Plan、Site Fixes 或 Results Watchlist 的对应 item。
3. Approved opportunity card 不允许再次 approve。
4. 当对应 Content Plan item 进入 Review 后，Opportunity card 可以从默认 Opportunity Queue 移除。
5. Content Plan item 进入 Review 后，Content Plan card 变成 `View in Review` link。
6. Review item 进入 Publish 后，Review card 变成 `View in Publish` link。
7. Publish item 完成 publish / apply 后，Publish card 变成 `View Results` link。
8. 所有 link card 都必须显示 destination label 和 next entity label。

### 13.4 Same-Page Linked Focus

1. Opportunity card 指向同页 Site Fixes item 时，点击后 smooth scroll 到 Site Fixes。
2. 目标 Site Fix card 必须完整进入可视区域。
3. 目标 Site Fix card 必须获得 focus ring。
4. 目标 Site Fix card 背景或边框柔和 pulse 两次。
5. `prefers-reduced-motion` 下禁用 pulse，改用静态高亮和 focus ring。
6. 如果目标 card 被 filter、tab、pagination 隐藏，系统必须让目标 card 可见或显示明确 fallback。
7. 同页聚焦不能改变 Opportunity Queue 和 Site Fixes 的整体 IA 顺序。

### 13.5 Product Language

1. 用户不需要理解 internal type string 就能看懂 card。
2. 每张 card 可以通过 work type、evidence、destination、CTA 被理解。
3. Site Fixes 被描述为 approved site work，不是另一个 discovery queue。
4. Results 被描述为 measurement surface，不是 opportunity approval surface。

## 14. Migration Plan

### Phase 1: Copy and IA

- 移除 Analysis 顶部 metrics board。
- 将 Opportunity Queue 放到页面顶部。
- 将 Direct Action queue 改名为 Site Fixes。
- 将 Site Fixes 放在 Opportunity Queue 下方。
- 将 generic CTA 替换为 destination-specific CTA。
- 将 approved Opportunity card 从立即移除改为 link card。

### Phase 2: Routing Clarity

- 在 Opportunity presentation 中增加 work type 和 destination。
- 确保 approved fix-site work 进入 Site Fixes。
- 确保 approved content / page-update work 进入 Content Plan。
- 确保 watch-only decision 进入 Results Watchlist。
- 为 Content Plan、Review、Publish 增加 sent-forward link card 行为。
- 为同页 link 增加 scroll、focus、target-card pulse 行为。

### Phase 3: Approval Source

- 为 execution items 增加或推导 approval source。
- 在 Content Plan、Site Fixes、Results 中显示 approval source。
- 显式展示 Autopilot policy approval。
- 在上游 card 中显示 approval source 和 downstream link。

### Phase 4: Measurement Feedback

- 将 Site Fix outcome 接入 Results。
- 已应用 technical / site fixes 与已发布 content outcomes 一起进入结果复盘。
- Results 不承接 raw opportunity approval。

## 15. UX Review Summary

最终用户路径应压缩为:

```text
Review opportunity
Approve destination
Execute in the right queue
Measure result
```

这个设计把 Analysis 聚焦在决策，把 Content Plan 聚焦在内容和页面更新，把 Site Fixes 聚焦在站点修复，把 Results 聚焦在结果。

同时，上一阶段 card 不应在完成后立即消失，而应先变成下一阶段的 link:

```text
Action card -> Link card -> History
```

这是降低用户迷失感的关键交互原则。

最重要的视觉顺序:

```text
Opportunity Queue first
Site Fixes second
Metrics and diagnostics later
```

产品必须在用户点击前说明下一步会发生什么。
