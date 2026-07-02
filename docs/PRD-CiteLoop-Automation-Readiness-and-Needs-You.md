# PRD: CiteLoop Automation Readiness and Needs You

> 日期: 2026-07-02
> 状态: Draft, product direction approved
> 范围: Home `Needs you`, Automation readiness, Settings information architecture, Opportunities page cleanup
> 上游文档: `docs/PRD-CiteLoop-Dashboard-Control-Center-Redesign.md`, `docs/PRD-CiteLoop-SEO-Autopilot.md`, `docs/PRD-CiteLoop-SEO-GEO-Automation-Upgrade.md`
> 代码基线: `origin/main@5a6d6f5`

## 0. 摘要

CiteLoop 已经具备 Autopilot readiness API、guarded action plan、publisher connection、notification channel、safe mode 和 policy 等自动化基础能力。但当前 UI 把 `Automation readiness` 放在用户可见 Opportunities 页面主体中; 该页面当前由 `/analysis` route 实现, 导致用户在处理机会 review 时被系统 setup 问题打断。

本 PRD 的目标是重新定义 automation readiness 的产品位置:

1. Home 的 `Needs you` 成为全局人工介入队列。
2. Opportunities 页面只负责发现和 review opportunities, 并把保留的机会送入 Content Plan 或 action loop。
3. Automation readiness 迁移到 Settings 的 Automation 区域，作为一次性/低频系统配置和健康检查。
4. Readiness gate 必须是 actionable checklist, 每个 blocked gate 都提供清晰解决入口。
5. Readiness 全部通过后，不再在 Opportunities 页面占位，也不在 Home 持续提醒。

用户打开 Home 后，应能看到整个系统所有需要人工处理的 gate, 包括内容工作流、发布故障、自动化配置和系统异常。用户进入 Opportunities 时，只应处理 opportunity review, 不需要理解自动化后台配置。

### 0.1 命名和代码锚点

本 PRD 使用 `Opportunities` 作为用户可见产品术语。当前代码实现仍使用:

- route: `/projects/:id/analysis`
- compatibility redirect: `/projects/:id/opportunities -> /projects/:id/analysis`
- page: `web/app/projects/[id]/analysis/page.tsx`
- client: `web/app/projects/[id]/seo/seo-client.tsx`
- client mode: `AnalysisClient` / `mode="analysis"`

实施时不得因为 PRD 使用 `Opportunities` 而新建第二套页面。正确做法是继续复用当前 Analysis route/client, 但用户可见页面标题、主导航 item 和文案统一为 `Opportunities`。内部代码名可以在后续重构中再清理。

### 0.2 锁定决策

本 PRD 锁定以下实现决策:

1. Settings 当前已有 tabs 为 `project`, `activity`, `search-console`, `publisher`, `crawl`, `notifications`; 本项目新增 `automation` tab。
2. `project` tab 是本 PRD 中 General/Budget 的实际落点, 不使用不存在的 `#general` anchor。
3. `SEOPolicy.autopilot_level` 是当前唯一可靠的自动化启用意图信号。Phase 1 不新增 target-level 字段。
4. Home 中 Level 2 readiness gate 的 P0 判定使用 `autopilot_level >= 2 && gate.blocking === true`。
5. `AutopilotReadiness.gates` 是 Automation readiness 的 canonical source; `SEOOverview.setup_checklist` 只能作为补充/legacy diagnostic, 不能渲染重复 Home cards。
6. Settings 维持现状 admin/internal 可见。非 Settings 用户不得看到会跳 404 的 Home CTA。
7. Recovery readiness 最终需要持久化 manual recovery acknowledgement, Phase 3 增加 `seo_policies` 字段。

## 1. 背景和问题

### 1.1 当前体验问题

在当前产品中，`Automation readiness` 展示了 blocked gates:

- Publisher write
- Notifications
- Policy confirmed
- Rollback or recovery ready

但这些 gate 只显示 blocked 状态和说明，没有直接链接到解决位置。用户知道系统被 blocked, 但不知道去哪里处理。

更大的问题是信息架构不匹配。用户可见 Opportunities 页面当前由 `/projects/:id/analysis` 实现, 它的主要任务是:

1. 发现机会。
2. review opportunity 是否值得进入 Content Plan 或 action loop。
3. 追踪机会进入 loop 后的状态。

Automation readiness 是系统自动化能力是否可用的问题，属于一次性或低频配置，不属于 opportunity review 的主任务。把 readiness 放在 Opportunities product surface 的主页面，会让用户误以为这些 setup gate 是每次 review opportunities 都必须处理的任务。

### 1.2 Home `Needs you` 覆盖不完整

Home 是整个项目的控制中心，应覆盖:

- overall metrics
- next action
- global notifications
- manual gates
- system setup blockers
- operational health

当前 Home 的 `Needs you` 已经朝人工 action queue 方向演进，但仍需要明确产品规则:

- 不只展示内容生产链路里的 review/publish actions。
- 必须展示所有需要用户介入才能恢复或提升系统能力的事项。
- 不应展示已经解决、仅供参考、或不会阻塞用户目标的 setup 状态。

### 1.3 Settings 已经是更合适的承载位置

Settings 已经包含:

- Publisher connection
- Notifications
- Activity Log
- Project config
- Crawl config

这些正是解决 automation readiness blocked gates 的位置。因此 Automation readiness 应当迁入 Settings, 作为 `Automation` tab 或 section, 并链接到现有设置区域。

## 2. 产品目标

### 2.1 用户目标

1. 用户在 Home 能看到所有需要自己处理的事项，而不是只看到部分内容工作流 blockers。
2. 用户看到 blocked gate 时可以立即点击进入解决位置。
3. 用户在 Opportunities 页面只做 opportunity review, 不被一次性 setup 信息打断。
4. 用户在 Settings 能一次性完成自动化配置、策略确认、publisher、notifications 和 recovery readiness。
5. 所有 readiness gate 完成后，系统不再把 Automation readiness 当作日常任务展示。

### 2.2 业务目标

1. 提高 setup completion rate。
2. 降低用户对 Autopilot blocked 状态的困惑。
3. 提高 opportunities review 到 Content Plan/action loop 的转化。
4. 降低 Home 和 Opportunities 页面信息噪音。
5. 让 CiteLoop 的自动化能力看起来像可信赖的系统配置，而不是散落在业务页面里的 debug panel。

### 2.3 设计目标

1. Home `Needs you` 保持现有通知卡片设计，不退回简单列表。
2. Priority 通过左侧色带和轻色背景表达:
   - P0: 浅红色。
   - P1: 当前黄棕色, 保持不变。
   - P2: 淡蓝色。
3. `Needs you` 的每张卡必须包含:
   - icon
   - category/pill
   - title
   - detail
   - count, 如适用
   - primary CTA
   - left priority stripe
4. Settings 的 Automation 区域默认可见 readiness 总览, gate 详情渐进展示。
5. Opportunities 页面默认不显示 Automation readiness 主模块。

## 3. 非目标

- 不重新定义 SEO Autopilot 的完整等级模型。
- 不新增新的自动执行能力。
- 不改变 risk classifier、guardrail、publisher 或 notification 后端安全规则。
- 不移除 Settings Activity Log 或 advanced diagnostics。
- 不把所有 setup 都移到 onboarding wizard。
- 不把 Automation 作为左侧导航独立页面, 除非后续数据证明它是高频工作区。
- 不承诺 Level 2 一定可用; 本 PRD 只改善 readiness 的信息架构和可操作性。

## 4. 核心原则

### 4.1 Opportunities is for opportunity review

Opportunities product surface, 当前 `/analysis` route, 只回答:

1. 有哪些机会值得处理?
2. 哪些机会应该进入 Content Plan 或 action loop?
3. 已 review 的机会现在在哪里?

该页面不承载:

- publisher setup
- notification setup
- policy confirmation
- rollback readiness
- kill switch/safe mode controls as primary content

如果自动化不可用，但机会 review 仍可继续，页面只能显示轻量说明，例如:

```text
Automation setup is incomplete. Reviewed opportunities can still enter the plan. Finish setup in Settings when you want guarded execution.
```

该说明必须链接到 Settings Automation, 但不能替代机会列表或成为主模块。

### 4.2 Automation readiness is a system setup and health concept

Automation readiness 表示系统能否进入更高自动化等级，尤其是 Level 2 guarded execution。它是系统配置状态，不是 opportunity 状态。

因此它的长期位置是:

```text
Settings -> Automation
```

Readiness 全部通过后:

- Home 不显示 readiness action。
- Opportunities 不显示 readiness 主模块。
- Settings 继续显示健康状态和 audit 入口。

### 4.3 Home owns global user attention

Home `Needs you` 是唯一全局人工介入队列。只要某个问题需要用户处理，且会影响项目进展、安全或自动化能力，就应该进入 Home。

Home 不应该显示:

- 已通过的 readiness gate。
- optional setup。
- 纯诊断信息。
- 不需要用户处理的后台 activity。

### 4.4 Every blocked gate needs an action

任何 `blocked`, `failed`, `expired`, `revoked`, `safe mode active`, `kill switch enabled` 状态都必须能回答:

1. 为什么 blocked?
2. 这会影响什么?
3. 用户下一步要做什么?
4. 点击哪里解决?

只显示 `blocked` badge 是不合格体验。

## 5. 信息架构

### 5.1 Home

Home 保持控制中心定位:

```text
Metrics / Control Center
Needs you
Pipeline
Activity
```

`Needs you` 的职责升级为全局人工介入队列，覆盖:

- context confirmation
- opportunity review
- draft review
- QA blocked drafts
- publish failure
- distribution ready
- Search Console setup, 当它影响结果证明
- publisher setup, 当它阻止 auto-publish 或 guarded execution
- notification setup, 当它阻止 safe operations
- safe mode active
- kill switch enabled
- automation degraded/failure
- notification delivery dead
- recovery readiness missing, 当用户想启用 Level 2+

### 5.2 Opportunities product surface, current Analysis route

用户可见页面名为 `Opportunities`; 当前实现 route 仍为 `/projects/:id/analysis`。页面主结构:

```text
Header: Review opportunities
Opportunity queue
Review guidance / result summary
Reviewed action loop summary
Optional lightweight setup note
Advanced diagnostics, collapsed or separate tab
```

移除默认主模块:

- Automation readiness
- Autopilot control cards
- blocked gates list as a primary section

如果保留 diagnostic, 必须在用户主动展开后显示。

### 5.3 Settings

Settings 当前 tab id:

```text
project
activity
search-console
publisher
crawl
notifications
```

本项目新增:

```text
automation
```

目标 tab 顺序:

```text
project
automation
search-console
publisher
notifications
crawl
activity
```

`project` 是项目配置、预算和 cadence 的落点。PRD 不再使用 `#general`。所有 Settings CTA 必须指向已存在或本项目明确新增的 tab/anchor。

### 5.4 Settings -> Automation

Automation 区域包含:

1. Readiness summary
2. Blocked gates checklist
3. Policy controls
4. Safe mode / kill switch state
5. Guarded execution plan controls
6. Recovery plan status
7. Links to Publisher and Notifications setup

Automation tab 内必须提供以下子锚点:

```text
#automation
#automation-policy
#recovery-plan
```

Readiness summary 必须明确:

- current autopilot level
- derived mode
- ready_for_level_2
- safe_mode_active
- kill_switch_enabled
- failed gate count
- last generated timestamp, 如可用

## 6. Home `Needs you` priority model

### 6.1 Priority levels

| Priority | 用途 | 颜色 | 示例 |
|---|---|---|---|
| P0 | 阻断执行、安全、发布或恢复 | 浅红色 | publish failed, safe mode active, kill switch enabled, publisher write broken for Level 2, notification delivery dead |
| P1 | 需要 review 或确认的主工作流 gate | 当前黄棕色, 保持不变 | confirm context, review opportunities, review drafts, policy confirmation |
| P2 | 改善结果、提升自动化能力、非立即阻断 | 淡蓝色 | connect Search Console for metrics, finish optional analytics, add notification channel before enabling Level 2 |

### 6.2 Visual requirements

Home `Needs you` 必须继续使用现有通知卡片设计，而不是切换成纯列表。卡片结构:

```text
left priority stripe
icon
category pill
title
detail
count badge
primary CTA
arrow affordance
```

Priority 的视觉映射:

- P0: `border-l` 使用当前红色 accent 可接受, 但整体卡片必须读作浅红: `bg-red-50/55` 或等价浅红背景, icon 容器浅红, 不使用大面积深红。
- P1: 保持当前黄棕色, 不重新调色。
- P2: `border-l` 淡蓝, 背景淡蓝, icon 容器淡蓝。

卡片应支持多张并列，但视觉上仍是通知/action cards, 不能降级成 table rows。

### 6.2.1 Current implementation migration

当前 Home `HumanActionItem` 已有 numeric `priority` 和 `tone`。实施时迁移为显式业务 priority, 再映射到现有 numeric/tone:

| Business priority | Numeric range | Tone/color family | Meaning |
|---|---:|---|---|
| P0 | 10-39 | red | Blocks execution, safety, publishing, or recovery |
| P1 | 40-69 | amber | Main workflow review or confirmation |
| P2 | 70-99 | blue | Improves results or automation capability |

`tone` 只能作为视觉输出, 不能继续作为业务优先级来源。排序使用 business priority, 同 priority 内再用 numeric priority 细排。

### 6.3 Sorting

排序规则:

1. P0 before P1 before P2。
2. 同 priority 内按工作流阻断程度排序。
3. 同类 item 合并计数, 不为同类问题渲染多张重复卡。

推荐顺序:

1. Safe mode active / kill switch enabled
2. Publisher or notification broken while automation is enabled or requested
3. Publish failed
4. QA blocked drafts
5. Context confirmation
6. Opportunity review
7. Draft review
8. Distribution ready
9. Search Console / analytics setup
10. Activity warnings

### 6.4 Suppression rules

以下情况不得进入 `Needs you`:

- readiness gate 已 connected。
- optional gate 未完成但不影响当前 project goal。
- cold-start 数据不足但用户仍可 review opportunities。
- diagnostics warning 无明确用户动作。
- automation readiness 全部通过。

### 6.5 Requested automation and P0 determinism

当前代码没有独立的 target automation level 字段。Phase 1 使用 `SEOPolicy.autopilot_level` 作为确定性启用意图:

- `autopilot_level >= 2`: 用户已经请求 guarded execution。任何 `AutopilotReadinessGate.blocking === true` 的 required gate 都可以进入 Home, 且默认 P0, 除非 gate mapping 明确降级。
- `autopilot_level < 2`: 用户尚未请求 Level 2。Readiness gate gaps 默认不作为 P0; 可作为 P2 capability improvement, 或只在 Settings Automation 中展示。
- `safe_mode_active === true`: P0, 因为它代表系统处于暂停/保护状态。
- `kill_switch_enabled === true && autopilot_level >= 1`: P0; 若 `autopilot_level === 0`, 默认只在 Settings 中展示, 不打扰 Home。
- `notification delivery dead`, publish failure, QA blocking 等 operational blockers 按各自业务规则进入 Home, 不依赖 readiness level。

若未来新增 explicit target level, 它可以替代 `autopilot_level` 的启用意图判断, 但不得改变本 PRD 的 Home priority semantics。

## 7. Automation readiness gates

### 7.1 Gate list

Automation readiness 使用现有 `AutopilotReadiness.gates` 概念，前端必须为每个 gate 补足 action mapping。下表是唯一 gate mapping 来源。

| Gate key | 用户展示名 | 失败影响 | CTA | Target | Anchor status | Home priority when surfaced | Category |
|---|---|---|---|---|---|---|---|
| `search_read` | Search data | 无法用真实 query/traffic 选择低风险动作 | Connect Search Console | `/projects/:id/settings#search-console` | exists | P0 if `autopilot_level >= 2`, otherwise P2 | Blocking now / Improves results |
| `publisher_write` | Publisher write | 无法自动创建或更新内容 | Set up publisher | `/projects/:id/settings#publisher` | exists | P0 if `autopilot_level >= 2`, otherwise P2 | Blocking now / Improves results |
| `notification_write` | Notifications | 自动化异常无法通知用户 | Create notification channel | `/projects/:id/settings#notifications` | exists | P0 if `autopilot_level >= 2` or delivery is dead, otherwise P2 | Blocking now / Improves results |
| `autopilot_policy_confirmed` | Policy confirmed | Level 2 不可启用 | Review policy | `/projects/:id/settings#automation-policy` | create in `automation` tab | Settings-only until user raises level; P1 if surfaced by explicit setup flow | Needs review |
| `monthly_budget_configured` | Budget configured | 自动执行缺少预算边界 | Set budget | `/projects/:id/settings#project` | exists | P0 if `autopilot_level >= 2`, otherwise P2 | Blocking now / Improves results |
| `safe_mode_clear` | Safe mode clear | 自动执行暂停 | Review safe mode | `/projects/:id/settings#automation` | create `automation` tab | P0 when active | Blocking now |
| `kill_switch_clear` | Kill switch clear | 自动执行关闭 | Review kill switch | `/projects/:id/settings#automation` | create `automation` tab | P0 if `autopilot_level >= 1`, otherwise Settings-only | Blocking now |
| `rollback_or_recovery_ready` | Recovery ready | 不能安全执行 guarded actions | Confirm recovery plan | `/projects/:id/settings#recovery-plan` | create in `automation` tab | P0 if `autopilot_level >= 2`, otherwise P2 | Blocking now / Improves results |

`Anchor status` 为 `create` 的 target 是本项目必须新增的 Settings tab/section。实现完成前, Home 不得发出指向这些不存在 anchors 的 CTA。

### 7.2 Gate card requirements

每个 blocked gate 在 Settings Automation 中必须显示:

- label
- status
- reason
- next_action
- blocking impact
- primary CTA
- secondary link to docs/activity, 如适用

在 Home 中只显示聚合后的 actionable item。例如:

```text
Automation setup is blocked
3 gates need setup before guarded execution can run.
CTA: Finish automation setup
```

如果只有一个 gate blocked, Home 可以显示更具体标题:

```text
Set up publisher
Publisher write is required before CiteLoop can execute guarded actions.
CTA: Open Publisher settings
```

## 8. Settings Automation UX

### 8.1 Summary card

Settings Automation 顶部显示 summary:

```text
Automation readiness
Level 0/1/2/3/4
Ready for Level 2 / Blocked gates
Safe mode: clear/active
Kill switch: off/on
```

文案要求:

- 解释 Level 2 是 guarded execution, 只执行低风险动作。
- 明确中高风险动作仍需人工 review。
- 不使用恐吓式文案。

### 8.2 Actionable blocked gates

Blocked gate list 不再只是 badge 列表。每项要有 button/link。

示例:

```text
Publisher write
Blocked
CiteLoop needs a scoped publisher connection before it can create or update content automatically.
CTA: Set up publisher
```

### 8.3 Resolved state

当所有 required gates 通过:

```text
Ready for Level 2
CiteLoop can execute low-risk actions within policy, budget, safe mode, kill switch, and recovery limits.
```

此时不显示 Home action。

### 8.4 Recovery plan

Recovery plan 必须从“说明文字”升级为可确认状态:

- publisher rollback available, 或
- manual rollback required acknowledged in persistent policy state, 且
- guarded execution records recovery metadata

Phase 3 必须新增持久化字段, 不接受纯前端 acknowledgement 作为最终状态:

```text
seo_policies.recovery_plan_acknowledged_at timestamptz null
seo_policies.recovery_plan_acknowledged_by text null
```

若 publisher exposes rollback capability, `rollback_or_recovery_ready` 可通过 publisher capability 自动通过。若 publisher 只支持 manual recovery, 用户必须在 Settings Automation 的 Recovery plan section 确认 manual rollback plan, 写入 acknowledgement 字段后才通过。

Phase 2 如暂不做 migration, 可以继续展示当前后端推导状态, 但 Phase 3/最终验收必须使用持久化 acknowledgement。

## 9. Opportunities page requirements

### 9.1 Required visible content

Opportunities 默认页面必须优先展示:

1. opportunity queue count/status
2. opportunities cards
3. Add to Content Plan / dismiss actions
4. review guidance
5. reviewed/in-loop summary, 如果有数据

### 9.2 Removed from default view

以下不再默认作为主 section 出现:

- `Automation readiness`
- `Autopilot`
- `Blocked gates`
- objective/plans/safe mode summary cards
- recovery plan

### 9.3 Lightweight bridge to Settings

如果 readiness incomplete 且用户拥有权限:

```text
Guarded execution is not fully set up. You can still review opportunities and add them to the plan.
Finish automation setup in Settings.
```

该 note:

- 不超过一行标题 + 一行说明 + link。
- 不得出现在机会列表之前成为主任务。
- 当 readiness ready 后隐藏。

## 10. Data and state requirements

### 10.1 Existing data

优先复用:

- `SEOOverview.setup_checklist`
- `SEOOverview.handoff_ready_for_autopilot`
- `AutopilotReadiness`
- `AutopilotReadinessGate`
- `SEOPolicy`
- `SafeModeEvent`
- publisher connections
- notification channels
- notification deliveries
- generation/activity runs

### 10.1.1 Setup source precedence and dedupe

`SEOOverview.setup_checklist` 和 `AutopilotReadiness.gates` 都可能描述 Search Console / publisher readiness。为避免 Home 渲染重复卡片:

1. `AutopilotReadiness.gates` 是 automation readiness 的 canonical source。
2. `SEOOverview.setup_checklist` 只用于:
   - SEO overview/diagnostics。
   - Autopilot readiness API 不可用时的 fallback copy, 但不得生成第二张同类 Home card。
   - 非 Level 2 readiness 的 capability说明。
3. Home action ids 必须按 capability 去重:
   - `search_read` and `search_data` -> `search-console-setup`
   - `publisher_write` and `publisher` -> `publisher-setup`
   - `notification_write` and notification delivery failures -> `notification-setup` or `notification-health`, depending on whether the issue is missing setup or failed delivery.
4. 如果 `AutopilotReadiness` 和 `SEOOverview.setup_checklist` 对同一 capability 状态冲突, Home 使用 `AutopilotReadiness`; Settings diagnostics 可以显示 discrepancy, 但不能让用户看到两个互相竞争的 CTAs。
5. `SEOOverview.handoff_ready_for_autopilot` 只能作为 summary signal, 不能覆盖 individual gate action mapping。

### 10.2 Home aggregation input

Home 需要能读取 readiness 状态。第一阶段可直接在 Home refresh 中调用:

```text
api.getAutopilotReadiness(projectId)
```

实现要求:

- 加入现有 Home `Promise.all` refresh, 使用 `.catch(() => null)`。
- readiness 请求失败不得阻塞 Home 其它数据。
- readiness 请求失败不得显示 false ready 或 false blocked。
- 如果 Home 数据加载成功但 readiness 加载失败, 只在 Settings Automation 或 Activity 中显示 retry/error, Home 不渲染 setup blocker。

如果后续 Home 请求过多，应将 attention queue 聚合下沉到 API。

### 10.3 Frontend action model

前端应定义统一 `HumanActionItem`:

```text
id
title
detail
href
cta
priority: P0 | P1 | P2
category
count
source
```

`tone` 不应再承担业务 priority。Priority 先决定视觉色带，再映射到 tone。

### 10.4 Readiness gate action mapping

前端应提供 deterministic mapping:

```text
gate.key -> href + cta + priority + category
```

未知 gate:

- Settings Automation 中仍显示。
- Home 中可归入 P2 unless `blocking=true` and automation level >= 2, then P0。

## 11. Permissions and visibility

Settings 当前受 admin/internal gate 保护。本项目按现状设计, 不把 Automation setup 暴露给所有普通用户。若当前用户无法访问 Settings:

- Home 不应显示会跳 404 的 Settings CTA。
- Home 可以隐藏 setup action; 如果 blocker 会影响当前用户正在做的工作, 只显示一张不可点击或联系管理员的说明卡, CTA 文案为 `Ask an admin`, 不指向 404 route。
- Opportunities 页面不应要求普通用户解决无法访问的 setup gate。
- Opportunities 的 lightweight Settings bridge 仅对可访问 Settings 的用户显示。
- ProjectShell 或页面 loader 必须把 `canAccessSettings` 等价信号传给 Home/Workspace, 或提供不会跳 404 的权限判断 helper。

如果未来 Automation setup 面向普通用户开放，需要重新定义 Settings 权限边界。

## 12. Empty, loading, and error states

### 12.1 Home

- Readiness API loading 不应阻塞 Home 主内容。
- Readiness API failed 时，不显示 false blocker。
- 如果已有 activity warning, 可显示 `Check automation health`。

### 12.2 Settings Automation

- Loading: skeleton matching readiness summary and gate list。
- Error: inline notice with retry。
- Empty: `No readiness data yet`, CTA refresh or open Activity。

### 12.3 Opportunities

- Readiness API failed 不影响 opportunities review。
- Lightweight setup note only appears when readiness data is available and incomplete。

## 13. Success metrics

以下 metrics 是产品观测目标。若当前应用没有前端 analytics/event tracking 基础设施, Phase 1 不因缺埋点而阻塞 IA 搬迁; 但 implementation plan 必须把埋点前置条件标出来, 并至少通过 contract tests 验证可点击路径和 UI 状态。

### 13.1 Product metrics

- 用户点击 blocked gate 后到达有效配置区域的成功率。
- Automation readiness blocked gate 平均解决时间。
- Opportunities review completion rate。
- Home `Needs you` CTA click-through rate。
- Settings Automation tab engagement。

### 13.2 Quality metrics

- Home 中无 action 时，不能显示 stale setup reminders。
- Opportunities 页 ready/incomplete automation 状态变化不应改变 opportunity review 主路径。
- 每个 blocked gate 都有 CTA。
- Priority 色带符合 P0/P1/P2 映射。

## 14. Acceptance criteria

### 14.1 Home

1. `Needs you` 使用现有通知卡片样式。
2. P0 卡片为浅红色侧边色带。
3. P1 卡片保持当前黄棕色。
4. P2 卡片为淡蓝色侧边色带。
5. Automation readiness blockers 在需要用户处理时进入 `Needs you`。
6. Readiness ready 后, Home 不显示 automation setup action。
7. 所有 Home action CTA 都进入可解决问题的位置。
8. 非 Settings 用户不得看到会跳 404 的 Settings CTA。
9. Home readiness fetch 失败时不显示 false blocker, 也不阻塞 metrics/control center。

### 14.2 Settings Automation

1. Settings 有 Automation readiness 区域或 tab。
2. Blocked gates 每项显示 CTA link。
3. Publisher gate 链接到 Publisher connection。
4. Notification gate 链接到 Notifications。
5. Policy gate 链接到 Automation policy。
6. Recovery gate 链接到 Recovery plan。
7. Ready state 不显示 blocked gate 列表。
8. Existing anchors `#project`, `#search-console`, `#publisher`, `#notifications` remain valid。
9. New anchors `#automation`, `#automation-policy`, `#recovery-plan` exist before Home or Opportunities links point at them。
10. Recovery plan acknowledgement persists to `seo_policies.recovery_plan_acknowledged_at` and `seo_policies.recovery_plan_acknowledged_by` by final Phase 3 implementation。

### 14.3 Opportunities product surface

1. 用户可见 Opportunities 页面, 当前 `/projects/:id/analysis` route, 默认不再显示 Automation readiness 主模块。
2. Opportunity queue 和 review actions 仍是主内容。
3. Incomplete automation 只显示轻量 Settings link, 且不阻塞 review。
4. Advanced diagnostics 仍可访问, 但不是默认主任务。
5. `/projects/:id/opportunities` redirect 继续可用, 不新建第二套 opportunity page。

### 14.4 Copy

1. 不只显示 `blocked` badge。
2. 每个 blocker 都有用户语言说明。
3. CTA 使用动词:
   - Set up publisher
   - Create notification channel
   - Review policy
   - Confirm recovery plan
   - Review safe mode

## 15. Phased rollout

### Phase 1: PRD and contract tests

- Lock this PRD.
- Add/update contract tests for:
  - Home priority labels and colors.
  - Home includes automation readiness blockers.
  - Opportunities product surface, implemented by `/analysis`, no longer has default Automation readiness section.
  - Settings exposes Automation readiness CTAs.
  - Gate mapping table targets existing/new anchors exactly.
  - Non Settings users do not get dead Settings links.

### Phase 2: Home attention queue

- Fetch/read readiness state on Home by adding `api.getAutopilotReadiness(projectId).catch(() => null)` to the existing refresh `Promise.all`.
- Extend `HumanActionItem` with explicit priority.
- Preserve notification card style.
- Add P0/P1/P2 visual mapping.
- Implement source dedupe between `AutopilotReadiness.gates` and `SEOOverview.setup_checklist`.
- Use `autopilot_level >= 2` as Level 2 request signal for P0 readiness blockers.

### Phase 3: Settings Automation

- Add Automation tab/section.
- Move readiness summary and blocked gates to Settings.
- Add CTA mapping.
- Add recovery plan acknowledgement/status.
- Add `seo_policies.recovery_plan_acknowledged_at` and `seo_policies.recovery_plan_acknowledged_by`.
- Ensure `#automation`, `#automation-policy`, and `#recovery-plan` anchors exist.

### Phase 4: Opportunities cleanup

- Remove Automation readiness primary section from Opportunities product surface in current `/analysis` route/client.
- Add lightweight Settings bridge only when incomplete.
- Keep diagnostics behind advanced disclosure.
- Keep `/opportunities` redirect to `/analysis` working.

### Phase 5: Production verification

- Verify a project with blocked gates:
  - Home shows correct P0/P1/P2 cards.
  - Settings CTAs navigate to correct anchors.
  - Opportunities remains focused on review.
- Verify a project with all gates ready:
  - Home has no automation setup card.
  - Settings shows ready state.
  - Opportunities has no readiness module.

## 16. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Users miss automation setup after it moves out of Opportunities | Home shows setup blockers only when actionable; Opportunities can show lightweight bridge |
| Settings is admin-only and some users cannot resolve blockers | Hide inaccessible CTAs or show contact admin copy |
| Home becomes noisy with too many setup cards | Aggregate readiness blockers into one card unless one specific P0 gate dominates |
| P2 setup items distract from P1 review work | P2 sorts after P0/P1 and can be hidden behind limit/details if many |
| Readiness API failure creates false confidence | On failure, do not show ready or blocked; show Settings-level retry only |

## 17. Resolved review decisions

1. Automation setup remains Settings/admin-internal for this PRD. Home hides dead Settings CTAs for users without access.
2. Recovery plan confirmation persists in `seo_policies` via `recovery_plan_acknowledged_at` and `recovery_plan_acknowledged_by`.
3. Home aggregates readiness blockers client-side in Phase 1, using existing APIs. A backend `attention_queue` endpoint is deferred until request count or consistency pressure justifies it.
4. User-facing label standardizes on `Opportunities`; internal route remains `/analysis` and `/opportunities` continues redirecting for compatibility.

## 18. Product decision

Approved direction:

- Automation readiness belongs in Settings, not Opportunities.
- Home `Needs you` is the global human action queue.
- Home keeps the existing notification card design.
- Priority color system:
  - P0: light red.
  - P1: current amber/brown.
  - P2: light blue.
- Resolved automation readiness should not continue occupying Opportunities or Home.
