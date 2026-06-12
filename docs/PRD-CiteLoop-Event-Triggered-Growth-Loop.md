# PRD：CiteLoop 事件驱动 Growth Loop 自动推进

> 日期：2026-06-12
> 阶段：Dashboard growth loop + automation closure
> 依赖：`docs/PRD-CiteLoop-MVP-v2.md`、`docs/PRD-CiteLoop-SEO-Operations-Loop.md`、`docs/PRD-CiteLoop-Dashboard-Control-Center-Redesign.md`
> 目标：把 CiteLoop 从“用户进入页面后手动触发下一步”改成“用户只在必要 Review gate 停下，其余步骤由事件自动推进”。

## 1. 背景与问题

CiteLoop 当前首页已经把产品体验表达成一个 Growth Loop：

1. `Context`
2. `Find opportunities`
3. `Plan content`
4. `Create drafts`
5. `Review`
6. `Publish`
7. `Measure results`

用户的心理模型很清楚：每个卡片代表一个真实工作阶段。卡片底部展示当前状态和下一步 action item。用户完成一个需要人工判断的步骤后，系统应该自动进入下一个不需要人工判断的步骤。

当前代码里，部分链路已经接近这个模型，但还有几个关键断点：

- `Context confirmed -> Find opportunities` 已经是事件触发。
- `Find opportunities reviewed -> Plan content` 还不是事件触发。
- `Plan content generated -> Create drafts` 主要依赖手动按钮或每日 cron。
- `Draft approved -> Publish` 依赖 scheduler，并且默认可能被 `buffer_days` 推迟。
- `Published -> Measure` 与 `content_actions` 的 outcome 状态没有贯通。

这会造成用户看到的矛盾：用户已经在 Opportunities 页面把 3 个 opportunities 加入 Plan，但 Home 页的 `Find opportunities` 卡片仍显示 ready to review；进入 Content Plan 页面后还需要手动点击 `Generate content plan`，用户会感觉产品架构不是一个自动 loop，而是一组分散页面。

## 2. 现状代码审计

### 2.1 Context 确认后自动发现 Opportunities

这是当前最接近目标态的产品行为，但不是后续自动推进应该复制的技术机制。

- `internal/api/handlers_projects.go`
  - `updateProfile` 在 profile 从未确认变成已确认时，调用 `startContextOpportunityDiscovery`。
- `internal/api/onboarding.go`
  - `startContextOpportunityDiscovery` 异步启动 `runContextOpportunityDiscovery`。
- `internal/seo/service.go`
  - cold-start opportunity 生成要求 profile 已确认，避免在未经用户确认的 context 上继续推进。

产品含义：`Context` 需要用户确认；确认后自动进入 `Find opportunities`。

技术问题：当前 `startContextOpportunityDiscovery` 是 fire-and-forget goroutine，进程重启会丢失任务，没有持久化、重试、dead-letter 或可见运行状态。后续实现不能复制这个机制；它也应该迁移到本文定义的 durable workflow event。

### 2.2 Opportunity review 与 Content Plan 断开

Opportunity 页面当前的 `Add to Content Plan` 并不会生成 `topics`。

- `internal/api/handlers_seo.go`
  - `createSEOContentAction` 创建 `content_actions`，状态为 `ready_for_review`。
  - 随后把对应 `seo_opportunities` 标记为 `converted`。
- `internal/db/queries/seo.sql`
  - `CreateContentAction` 只写入 `content_actions`。
  - `UpdateSEOOpportunityStatus` 只更新 opportunity 状态。
- `internal/migrations/0007_seo_operations_loop.sql`
  - `content_actions` 没有 `topic_id` 或 action-topic 关联表。
  - 但已有 `target_article_id` 和 `draft_article_id`，后续 traceability 应复用这些列回填 article 链接，而不是在 article 侧新增重复字段。

结果：用户认为 opportunity 已经进入 plan，但系统只是创建了一个独立的 SEO action，并未变成 Content Plan 页面使用的 `topics`。

### 2.3 Content Plan 主要读取 Topics

Content Plan 页面使用的是 topic backlog，而不是 `content_actions`。

- `web/app/projects/[id]/plan/page.tsx`
  - 直接渲染 Topics client。
- `web/app/projects/[id]/topics/topics-client.tsx`
  - `runStrategist` 需要用户点击按钮，调用 `api.runStrategist(projectId)`。
  - `generate(topic)` 需要用户对单个 topic 手动触发 draft generation。
- `internal/api/handlers_agents.go`
  - `runStrategist` 调用 `agents.NewStrategist(...).Run`。
- `internal/agents/strategist.go`
  - Strategist 从 profile、inventory、search context 生成 topics；没有把 reviewed `content_actions` 作为一等输入。

结果：Content Plan 不是 opportunity review 的自然后续，而是另一个手动 Strategist 入口。

### 2.4 Draft generation 不是即时事件

当前 draft generation 有两条路径：

- 手动：`internal/api/handlers_agents.go` 的 topic generate endpoint。
- 定时：`internal/scheduler/scheduler.go` 的 `TickGenerate`，由 `internal/scheduler/helpers.go` 每日 `02:00` 触发。

结果：即使 topics 已经生成，drafts 也不一定及时出现。用户如果不进入 Plan 页面手动点击，可能要等每日 cron。

### 2.5 Review approve 后 Publish 不是即时链路

- `internal/api/handlers_review.go`
  - `ApproveArticle` 会把 article 标记为 approved。
  - 如果 canonical 没有 schedule，当前逻辑会按 `buffer_days` 设置 `scheduled_at`。
  - `ApproveArticle` 没有立即触发 project-scoped publish job。
- `internal/scheduler/scheduler.go`
  - `TickPublish` 每 5 分钟自动发布 due approved canonicals。
  - retry publish endpoint 会主动调用 `TickPublish`，但 approve endpoint 没有。

结果：用户完成 Review 后，Publish 可能不是立即发生，也没有清楚告诉用户是“等待排期”还是“等待 publisher”。

### 2.6 Measurement 与 Action outcome 没有贯通

- `internal/scheduler/scheduler.go`
  - `TickSEO` 每日同步和分析 SEO 数据。
- `internal/api/handlers_autopilot.go`
  - Autopilot plan 使用 `seo_action_plans`，与 `content_actions`、`topics`、`articles` 是另一条链。
- `content_actions` 当前状态和 `articles.published`、SEO measurement outcome 没有稳定追踪关系。
- `content_actions.status` schema 中有 7 个值，但当前主路径只写入 `ready_for_review`、`approved`、`measuring`；`drafting`、`published`、`completed`、`failed` 基本是 dead enum values。

结果：`Measure results` 卡片难以实时反映某个 opportunity/action/content 的真实 outcome。把 action 更新到 `published` 不是简单补一行，而是需要新增 publish pipeline 到 `content_actions` 的真实写入路径。

### 2.7 实测项目状态暴露的问题

调试项目 `7745b021-ab33-4977-bfab-084fbc0b2840` 体现了当前断点：

- 有 `content_actions.ready_for_review = 3`。
- 有 `topics.backlog = 12`。
- 没有 `articles`。
- `content_actions` 与 `topics` 没有关联。

这说明用户可以在 UI 上完成“加入 Plan”，也可以在另一路生成 topics，但系统无法判断这 12 个 topics 是否消费了那 3 个 reviewed opportunities。

### 2.8 已有基础设施应该复用

代码库已经有几块基础设施，事件驱动 growth loop 应该复用它们的模式，而不是另起一套不相容机制。

- `internal/migrations/0005_notifications.sql`
  - `notification_deliveries` 已经是一个 delivery outbox，包含 `pending/sent/dead`、attempts、retry time 和 pending index。
- `internal/scheduler/helpers.go`
  - `TickNotifications` 每 10 秒运行一次 notification worker。
- `internal/db/queries/notifications.sql`
  - delivery worker 使用 `for update skip locked`、retry/backoff、4 次后 dead-letter。
- `internal/notification`
  - 已有 Dispatcher 和 failure event，比如 `generation.failed`、`publish.failed`、`webhook.delivery.dead`。
- `internal/topicstate`
  - 已有 topic state machine，状态为 `backlog/scheduled/generating/drafted/done/archived`，事件为 `schedule/clear_schedule/start_generation/mark_drafted/reject_draft/archive`。
- `internal/migrations`
  - 当前 migration head 是 `0014_project_hard_delete_cascade.sql`；本文新增 migration 应从 `0015` 开始。

结论：`workflow_events` 可以是新表，因为它负责 workflow advancement 而不是 notification delivery；但 worker 语义、retry/backoff/dead handling 应沿用 notification worker 的成熟模式。Topic 状态更新必须通过 `internal/topicstate` 的 transition 语义，不应散落 raw status writes。

## 3. 产品目标

1. 用户只在必须人工判断的位置停下，其余步骤自动推进。
2. Home 的每个 Growth Loop 卡片实时反映它代表页面的真实状态。
3. 每个卡片底部都展示：
   - 当前阶段状态。
   - 用户 action item。
   - 指向对应页面或具体对象的链接。
4. Opportunity review 完成后，Content Plan 自动生成，不要求用户再进入 Plan 页面手动点击。
5. Content Plan 生成后，系统自动创建 draft，直到达到 buffer、budget 或 review gate。
6. Draft review 通过后，系统自动进入 publish 或明确的 scheduled state。
7. Published 后自动进入 measurement/baseline/outcome 追踪。
8. 每条链路有 traceability：`opportunity -> action -> topic -> article -> publish -> measurement`。Phase 1 用 `topics.source_content_action_id` 建立 action -> topic，用现有 `content_actions.draft_article_id` / `target_article_id` 回填 action -> article。
9. 后台任务必须幂等、可重试、可观测，不能因为单个 URL、单次 LLM 或单篇内容失败而把整个 loop 卡死。

## 4. 非目标

- 不自动跳过 `Context` 确认。
- 不自动 approve draft。
- 不绕过 budget、publisher permission、safe mode、QA blocking issue。
- 不把第三方平台分发改成全自动发布；V1 仍可保留半自动分发。
- 不强行引入复杂工作流平台；本阶段可以用轻量 outbox/event worker 实现。
- 不用 Home 卡片伪造状态；卡片必须来自与页面一致的数据源或共享 state query。

## 5. Human Gates 与 Auto Stages

| 阶段 | 当前状态来源 | 是否需要用户停下 | 下一步触发 |
|---|---|---:|---|
| 输入 product domain | Project setup | 是 | 用户提交 domain 后自动 build context |
| Build Context | `product_profiles`、source pages、evidence | 是 | 用户确认 profile 后自动 find opportunities |
| Find opportunities | `seo_opportunities` | 是 | 用户 review/add/dismiss 完成本批 opportunities 后自动 plan content |
| Plan content | `topics` + reviewed `content_actions` | 否 | topics 创建成功后自动 create drafts |
| Create drafts | `articles.generating/pending_review` | 否 | drafts 生成成功后进入 Review |
| Review | `articles.pending_review` | 是 | 用户 approve 后自动 publish 或进入明确 schedule |
| Publish | `articles.approved/scheduled/published` | 取决于权限和策略 | publish 成功后自动 measure |
| Measure results | SEO/GSC/public crawl outcome | 否 | observation window 到期后自动更新 outcome |

规则：只有 `Context confirm`、`Opportunity review`、`Draft review`、高风险 publisher/safe-mode block 可以要求用户停下。其他过程必须由事件推进或由 cron 兜底。

## 6. 目标用户体验

### 6.1 Home Growth Loop

Home 卡片必须成为状态总览，而不是静态指标。

每个卡片底部区域统一为：

- `Status`：一句话说明系统现在处于什么状态。
- `Action item`：如果需要用户做事，展示唯一最重要动作。
- `Link`：跳到需要处理的页面或具体对象。

示例：

| 卡片 | 状态示例 | Action item 示例 |
|---|---|---|
| Context | `Needs confirmation` | `Review and confirm context` |
| Find opportunities | `3 opportunities ready to review` | `Review opportunities` |
| Find opportunities | `All opportunities reviewed` | `No action needed` |
| Plan content | `Generating plan from reviewed opportunities` | `No action needed` |
| Plan content | `12 topics in backlog` | `Review content plan` |
| Create drafts | `Draft generation running` | `No action needed` |
| Review | `4 drafts waiting for approval` | `Review drafts` |
| Publish | `Waiting for publisher connection` | `Connect publisher` |
| Measure results | `Collecting first results` | `No action needed` |

### 6.2 Opportunities 页面

Opportunities 页面只展示用户需要 review 的东西。

- 默认只显示 `open` 或 `needs_review` opportunities。
- 已加入 plan、已 dismiss 的内容进入 secondary/history 区域，不占主屏。
- 用户完成最后一个 open opportunity 后，页面应显示：
  - `All reviewed`
  - `Content plan is being generated`
  - 跳转到 Plan 的链接
- 页面不应继续展示大量解释性数据，除非用户展开详情。

### 6.3 Content Plan 页面

Content Plan 页面不应让用户觉得必须手动启动下一步。

- 如果存在 reviewed actions 但 topics 尚未生成，显示 `Generating content plan from reviewed opportunities`。
- 如果 generation 失败，展示 retry button 和错误原因。
- `Generate content plan` 可以保留为 advanced/retry 行为，不作为主 CTA。
- 每个 topic 应显示来源：
  - 来自哪个 opportunity/action。
  - 使用了哪些 evidence snippets。
  - 当前是否已经创建 draft。

### 6.4 Review 页面

Review 是明确的人类闸门。

- Draft 生成后进入 `pending_review`。
- 用户 approve 后，系统立刻判断：
  - 如果 publisher ready 且未指定未来日期：enqueue publish now。
  - 如果有 future schedule：显示 scheduled time。
  - 如果 publisher missing：卡片显示 `Connect publisher`。
  - 如果 QA blocking：不允许 approve 或 approve 后不能 publish。

## 7. 后端状态机

### 7.1 核心原则

1. 所有自动推进都必须幂等。
2. 每个 event 有 dedupe key。
3. 每个 project 同一时间只能有一个 workflow advance worker 修改关键状态。
4. Cron 只做 reconciliation，不作为用户体验主路径。
5. 任一子任务失败不能阻断同批其他子任务。
6. 用户可见状态必须从数据库事实生成，而不是前端猜测。
7. Workflow worker 复用 notification worker 的成熟模式：poll loop、`for update skip locked`、retry/backoff、dead-letter、可人工 retry。
8. `context.confirmed` 也要进入 durable workflow event；当前 fire-and-forget goroutine 只作为过渡实现。

例外：measurement outcome 依赖 observation window 和外部 SEO/GSC 数据刷新，允许继续以 SEO cron 为主触发；但发布成功后的用户可见状态必须立即进入 `Collecting first results`。

### 7.2 推荐事件模型

新增轻量 outbox 表 `workflow_events`：

```sql
create table workflow_events (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  event_type text not null,
  entity_type text,
  entity_id uuid,
  dedupe_key text not null unique,
  payload jsonb not null default '{}',
  status text not null default 'pending'
    check (status in ('pending','running','succeeded','failed','dead')),
  attempts int not null default 0,
  run_after timestamptz not null default now(),
  locked_at timestamptz,
  processed_at timestamptz,
  error text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index idx_workflow_events_pending
  on workflow_events (status, run_after, created_at)
  where status = 'pending';

create index idx_workflow_events_reclaim
  on workflow_events (status, locked_at)
  where status = 'running';
```

新增 migration 文件从 `0015` 开始。

推荐事件类型：

| event_type | 来源 | 处理动作 |
|---|---|---|
| `context.confirmed` | profile confirm handler | 生成 cold-start opportunities |
| `opportunity.reviewed` | add/dismiss opportunity | 判断本批 opportunities 是否全部 reviewed |
| `opportunity.batch_completed` | workflow service | 从 accepted actions 生成 topics |
| `content_plan.created` | strategist/action planner | 触发 draft generation |
| `drafts.requested` | workflow service | 为 backlog topics 生成 drafts |
| `draft.ready_for_review` | writer | 更新 Review 卡片 |
| `draft.approved` | review handler | 触发 publish 或 schedule |
| `article.published` | publisher | 创建 measurement baseline |
| `measurement.window_due` | SEO scheduler | 更新 outcome |

### 7.3 Worker 语义

Workflow worker 必须定义清楚以下行为：

1. Poll：默认每 10 秒扫描 `pending` 且 `run_after <= now()` 的事件，模式对齐 `TickNotifications`。
2. Immediate nudge：HTTP handler enqueue 事件后，可以通过进程内 channel poke worker，让 UI 更快进入 `Generating`；poll loop 仍是 durable fallback。
3. Lock：worker 使用 row-level lock 领取事件，并在 `AdvanceProject` 内获取 project-level advisory lock。
4. Stuck recovery：如果事件处于 `running` 且 `locked_at < now() - interval '5 minutes'`，worker 将其 reclaim 为 `pending`，并增加 attempts 或写入 recovery marker。
5. Backoff：失败后按 1m、5m、30m 退避；达到 4 次后标记 `dead`。
6. Dead visibility：`dead` workflow event 是用户可见 block reason，Home card 必须显示具体阶段和 retry action。
7. Notification fanout：重要状态变化应通过现有 notification Dispatcher 发出，例如 `draft.ready_for_review`、`workflow.failed`、`publish.failed`。
8. Topic status：任何 topic 生成相关状态更新都必须使用 `internal/topicstate` 的 transition 语义，不允许绕过状态机直接写任意 status。

### 7.4 Dedupe key 约定

| event_type | dedupe_key |
|---|---|
| `context.confirmed` | `context.confirmed:{project_id}:{profile_id}:{confirmed_at}` |
| `opportunity.reviewed` | `opportunity.reviewed:{project_id}:{opportunity_id}:{status}` |
| `opportunity.batch_completed` | `opportunity.batch_completed:{project_id}:{context_version_or_discovery_run_id}` |
| `content_plan.created` | `content_plan.created:{project_id}:{content_action_id}` |
| `drafts.requested` | `drafts.requested:{project_id}:{topic_id}` |
| `draft.ready_for_review` | `draft.ready_for_review:{project_id}:{article_id}` |
| `draft.approved` | `draft.approved:{project_id}:{article_id}:{reviewed_at}` |
| `article.published` | `article.published:{project_id}:{article_id}:{published_at}` |

`opportunity.batch_completed` 不能用“project 当前没有 open opportunities”这种瞬时事实直接作为 dedupe key。Phase 1 需要使用 active context version；如果未来引入 discovery run，则改为 discovery run id。

### 7.5 Workflow service

新增内部服务：

```go
type GrowthWorkflow interface {
  Enqueue(ctx context.Context, projectID uuid.UUID, eventType string, payload map[string]any) error
  AdvanceProject(ctx context.Context, projectID uuid.UUID, trigger string) error
}
```

`AdvanceProject` 每次执行：

1. 获取 project-level advisory lock。
2. 读取统一 state snapshot。
3. 判断当前最靠前的可推进阶段。
4. 执行不需要用户确认的下一步。
5. 写入 workflow run/event 结果。
6. 如产生后续事件，继续 enqueue。

## 8. 数据模型改动

### 8.1 连接 Action 与 Topic

当前 `content_actions` 与 `topics` 断开。Phase 1 明确采用一对一 action -> topic，避免先引入 join table 和多 topic 分裂逻辑。

新增 migration `0015_*`：

```sql
alter table topics
  add column source_content_action_id uuid references content_actions(id);

create unique index uniq_topic_source_content_action
  on topics(project_id, source_content_action_id)
  where source_content_action_id is not null;
```

Article 关联不新增 article 字段，直接复用 `content_actions.draft_article_id` 和 `content_actions.target_article_id`。Draft 创建成功后回填 `draft_article_id`；如果 action 是 refresh/update existing article，则使用 `target_article_id`。

### 8.2 Content Action 状态重命名后置

现有 `content_actions.status` 包含 `drafting`、`ready_for_review`、`approved`、`published`、`measuring`、`completed`、`failed`，但语义与机会 review、topic plan、article publish 混在一起。

Phase 1 不重命名 enum。原因：

- 这是 CHECK-constraint migration，需要重写 live write path 和 UI。
- 当前 4 个 enum 值基本未使用，先贯通链路收益更高。
- 新状态里的 `accepted` 会与 `seo_opportunities.status = 'accepted'` 概念冲突。

后续 phase 可单独做状态重命名，映射建议如下：

| 当前值 | 后续建议值 | 迁移含义 |
|---|---|---|
| `ready_for_review` | `accepted` 或 `accepted_for_planning` | 用户已接受 opportunity，等待规划 |
| `approved` | `planned` | 已转为 plan/topic 或已通过 action 审核 |
| `measuring` | `measuring` | 已发布并进入观察 |
| `drafting` | `drafting` | 保留，但接入真实 draft generation |
| `published` | `published` | 保留，但接入真实 publish pipeline |
| `completed` | `completed` | 保留，用于 observation window 完成 |
| `failed` | `failed` | 保留，用于 workflow dead/error |

短期 UI 规则：已加入 plan 的 action 不能继续显示为“仍需 review”。即使数据库仍是 `ready_for_review`，Growth Loop state builder 也必须结合 opportunity converted 状态、source topic、draft article 来判断真实阶段。

### 8.3 统一 Growth Loop State View

新增 project-scoped query 或 materialized view，用于 Home 和各页面共享状态：

```sql
create view project_growth_loop_state as
select
  p.id as project_id,
  -- context
  -- opportunities
  -- plan
  -- drafts
  -- review
  -- publish
  -- measure
  now() as computed_at
from projects p;
```

实际实现可先用 Go query composer，不必先上 SQL view。关键是 Home 卡片和页面 header 使用同一套 state builder，避免状态不一致。

## 9. Backend 需求

### 9.1 Opportunity review completion

当用户对 opportunity 执行：

- `Add to Content Plan`
- `Dismiss`
- `Mark irrelevant`

后端必须：

1. 更新 opportunity 状态。
2. 创建或更新 content action。
3. enqueue `opportunity.reviewed`。
4. workflow 在 worker 内判断当前 opportunity batch 是否完成。

Batch 完成定义：

- 当前 active context version 下，没有 `seo_opportunities.status = 'open'` 的机会。
- Phase 1 不做 review cap；未来可允许本批机会达到用户设置的 review cap 后，剩余机会进入 secondary queue。

如果用户接受了至少一个 action，自动 enqueue `opportunity.batch_completed`。如果用户全部 dismiss，Find opportunities 标记为 done，但不触发 content plan。

并发要求：batch completion check 必须在 worker 的 project advisory lock 内执行，不能在 HTTP handler 内直接判断。两个用户动作同时完成“最后一个 open opportunity”时，dedupe key 和 project lock 必须保证只生成一次 plan。

### 9.2 Reviewed actions -> Topics

新增 action-aware strategist 路径：

```go
GenerateTopicsFromActions(projectID, actions []ContentAction) ([]Topic, error)
```

要求：

- 输入必须包含 action title、type、reason、linked opportunity、evidence snippets、profile summary。
- 输出 topic 必须写入 `source_content_action_id`。
- Phase 1 一个 accepted action 生成一个 topic。
- 同一个 action 重复触发不得创建重复 topic；唯一约束是 `uniq_topic_source_content_action`。
- 生成失败的 action 标记 `failed`，其他 actions 继续。
- 如果 LLM 超时，保存 retryable error，Home/Plan 卡片展示可重试状态。

### 9.3 Topics -> Drafts

当 topics 创建成功后，自动触发 draft generation。

规则：

- 只处理 `topics.status = 'backlog'` 且没有 existing article 的 topics。
- 遵守 `monthly_budget_usd`、draft buffer、cadence、safe mode。
- 不新增 drafts-per-batch knob；复用 `TickGenerate` 已有的 budget breaker 和 cadence deficit math，即 `desired = ceil(cadence_per_week * buffer_days / 7) - current_stock`。
- 优先生成刚由 reviewed actions 创建的 topics。
- 并发生成要有 worker 上限，避免 LLM/provider overload。
- 每个 topic 独立失败，不能阻断整个 batch。
- topic 状态必须通过 `internal/topicstate` transition 推进：
  - `backlog/scheduled -> generating` 使用 `EventStartGeneration`。
  - writer 成功后使用 `EventMarkDrafted`。
  - writer 失败后使用 `GenerationFailureStatus` 回到 `backlog` 或 `scheduled`。

手动 `Generate` 按钮保留为 retry/advanced，不是主要路径。

### 9.4 Draft approve -> Publish

用户 approve draft 后：

1. 更新 article status。
2. 如果 `scheduled_at` 是未来时间，显示 scheduled。
3. 如果没有显式 future schedule，enqueue `draft.approved` 并立即尝试 publish。
4. publisher 缺失或 safe-mode block 时，写入明确 block reason。

Phase 1 产品决策：

- 用户 approve 的语义是“可以发布”。
- 没有显式 future schedule 时，默认立即 publish。
- 如果 topic/article 已有明确未来 `scheduled_at`，尊重该时间。
- `buffer_days` 继续用于生成库存和 cadence planning，不再作为 approve 后静默延迟的默认规则。
- 如果未来担心一次 approve 多篇导致发布 burst，再增加 max-publishes-per-day valve；Phase 1 不阻塞于这个新 knob。

### 9.5 Publish -> Measure

Publish 成功后：

1. 回填 canonical URL。
2. 更新 article status 为 `published`。
3. 回填对应 `content_actions.draft_article_id` / `target_article_id`。
4. 更新对应 content action 为 `published` 或 `measuring`。当前代码没有这个路径，Phase 4 需要新增 content action write path。
5. 创建 measurement baseline。
6. enqueue `article.published`。

Measurement 可以继续由日级 SEO tick 补全，但用户可见状态必须立即变成 `Collecting first results`。

### 9.6 Cron reconciliation

现有 cron 保留，但定位改为兜底：

- `TickGenerate`：补偿漏掉的 draft generation。
- `TickPublish`：补偿漏掉的 due publish。
- `TickSEO`：补偿 measurement sync。

Cron 不应是 Home 状态推进的主要依赖。

## 10. Frontend 需求

### 10.1 Sidebar CTA

左侧 sidebar 不应在进入页面后显示重复大 CTA，例如在 Visibility 页面顶部已经处于 review context 时，再显示一个 `Review opportunities` 红色按钮会造成困惑。

要求：

- 当前页面由 sidebar nav selected state 表达。
- Global CTA 只在真正跨页面、跨阶段且用户需要处理时出现。
- 如果当前页面就是 action target，则隐藏 global CTA。

### 10.2 Home card state consistency

Home 的 Growth Loop 卡片不得使用独立推导逻辑。

要求：

- Home 使用统一 `/growth-loop-state` API 或共享 loader。
- Visibility/Plan/Review 页面也读取同一套 state summary。
- 用户在页面完成操作后，Home 返回必须无延迟反映状态。
- mutation 成功后前端应执行 `router.refresh()` 或 invalidate/revalidate 共享 growth-state query。
- Home SSR 不能拥有一套独立推导逻辑；如果使用 server loader，它必须读取同一个 growth-state builder。

### 10.3 Plan 页面 CTA 降级

`Generate content plan` 不应是主路径。

状态逻辑：

- 有 pending reviewed actions：显示系统正在生成 plan。
- 生成成功：显示 topics。
- 生成失败：显示 retry。
- 无 reviewed actions：显示去 Opportunities review 的 action item。
- Advanced/manual generate：放 secondary action。

### 10.4 Visibility 页面信息减负

默认只展示用户要处理的 opportunities。

页面结构：

1. Header：`Review opportunities`
2. Summary：ready count、accepted count、dismissed count
3. Primary queue：open opportunities
4. Completed/History：折叠
5. Diagnostics：折叠在底部或 secondary tab

用户不应在主流程中先看到大量 analytics/technical information。

## 11. 容错与性能

### 11.1 LLM 与 crawler 容错

当前用户观察到进度条卡住，可能来自单个 crawl/LLM task 长时间等待。自动推进必须避免“一个任务拖死整批”。

要求：

- 每个 URL crawl 有 timeout。
- 每个 LLM call 有 timeout 和 max retries。
- 每批 source pages 允许 partial success。
- Profile/evidence/opportunity generation 不要求抓满所有发现页面。
- 达到最低质量阈值后即可继续，比如：
  - profile：homepage + 3-5 个高置信页面。
  - source crawl：最多 20 个 primary pages，失败 URL 跳过并记录 warning。
  - evidence snippets：从成功页面生成，低价值页面不阻塞。
- 进度条按真实 worker track 展示，不用单个 aggregate bar 掩盖并发状态。

### 11.2 并发策略

推荐 worker 上限：

| 任务 | 并发上限 | 备注 |
|---|---:|---|
| URL crawl | 5-8 | 同 host 限速 |
| evidence extraction | 3-5 | 受 LLM/provider 限制 |
| topic planning | 1-2 | 质量优先，batch input |
| draft generation | 2-3 | 每篇独立失败 |
| publish | 1-2 | 避免 publisher 冲突 |

### 11.3 Quality guardrail

加速不能牺牲质量：

- Profile 仍要求用户确认。
- Topic 必须带来源 action/evidence。
- Draft QA blocking issue 必须阻止发布。
- LLM partial failure 必须进入 retryable state，不生成低置信内容冒充成功。
- 如果 evidence 不足，topic/draft 应显示 `needs more context`，而不是硬生成。

## 12. Acceptance Criteria

### 12.1 Opportunity -> Plan

给定一个 project 有 3 个 open opportunities：

1. 用户把 3 个 opportunities 都加入 Content Plan。
2. 最后一次 mutation 成功后 10 秒内，至少进入可见运行态：
   - `Find opportunities` 卡片变为 `All opportunities reviewed` 或 `Plan generated`。
   - `Plan content` 卡片变为 `Generating` 或展示新 topics 数。
   - 不需要用户点击 `Generate content plan`。
3. 数据库中每个新 topic 都能追溯到 source content action。
4. 重复触发同一事件不会创建重复 topics。

### 12.2 Plan -> Drafts

给定 new topics 已创建：

1. 系统自动开始 draft generation。
2. 生成中的 topic 显示 `generating`。
3. 成功后 article 进入 `pending_review`。
4. `Review` 卡片实时显示 waiting count。
5. 单篇生成失败不影响其他 topics。

### 12.3 Review -> Publish

给定一篇 canonical draft 处于 `pending_review`：

1. 用户 approve。
2. 如果 publisher ready 且无 future schedule，系统立即 enqueue publish。
3. Publish 成功后：
   - article status 为 `published`。
   - canonical URL 已回填。
   - source content action 进入 `published` 或 `measuring`。
   - Home `Publish` 与 `Measure results` 卡片更新。

### 12.4 Blocked states

任一自动推进被阻塞时：

- 卡片必须显示 block reason。
- action item 必须指向用户可解决的位置。
- 后端必须记录 retryable/non-retryable error。
- retry 不得产生重复数据。

### 12.5 Realtime consistency

用户从 Visibility、Plan、Review 任一页面回到 Home：

- Home 卡片展示与该页面最新状态一致。
- 不允许出现“页面已完成，但 Home 仍显示旧 action”的状态。
- 前端 mutation 后必须 `router.refresh()` 或 invalidate growth-loop state。

## 13. 实施计划

### Phase 0：状态契约、Workflow Outbox 与测试基线

- 梳理 `project_growth_loop_state` response schema。
- 为现有 project fixture 写状态计算测试。
- 让 Home 和页面 header 共享同一 state builder。
- 新增 `0015_workflow_events.sql`，包含 pending/reclaim indexes。
- 新增 workflow worker skeleton，复用 notification worker 的 poll/retry/dead-letter 模式。
- 增加 per-project `auto_advance_enabled` 配置或等价 kill switch，默认开启；budget-spending 自动任务必须检查该开关。

### Phase 1：Opportunity review 自动触发 Plan

- `createSEOContentAction` 和 dismiss endpoint enqueue `opportunity.reviewed`。
- 在 worker project lock 内新增 batch completion 判断。
- 新增 action-aware topic generation。
- 新增 `topics.source_content_action_id`。
- Phase 1 固定 action -> topic 一对一。
- Plan 页面把 manual generate 降为 retry/advanced。

### Phase 2：Plan 自动触发 Drafts

- topic creation 成功后 enqueue `content_plan.created`。
- 提取 scheduler 中 project-scoped generation 方法，供 event worker 调用。
- draft generation 遵守 budget/buffer/safe mode，并复用现有 cadence deficit math。
- topic status 通过 `internal/topicstate` 推进。

### Phase 3：Review approve 自动触发 Publish

- approve endpoint enqueue `draft.approved`。
- 实现 approve 默认 immediate publish；保留显式 future schedule。
- publisher block reason 写入统一 state。

### Phase 4：Publish/Measure 贯通

- publish success 更新 action/topic/article chain。
- 创建 measurement baseline。
- Measure card 使用 action/article outcome state。

### Phase 5：UI 减负与状态统一

- Visibility 页面只保留 review queue 主流程。
- Sidebar CTA 按当前页面和 target action 隐藏/切换。
- Home 卡片统一展示 status/action/link。

### Phase 6：Content Action 状态清理

- 单独迁移 `content_actions.status` enum。
- 为历史 rows 写明确 value mapping。
- 移除 UI 对 `ready_for_review` 的误读。

## 14. Phase 1 产品决策

1. Draft approve 默认立即 publish；如果 topic/article 有显式 future schedule，则尊重 schedule。
2. `buffer_days` 用于内容库存和 cadence planning，不再用于 approve 后静默延迟。
3. Phase 1 一个 accepted opportunity 生成一个 topic。
4. Opportunity batch completion 定义为当前 active context version 下所有 open opportunities 都已 review；review cap 留到后续。
5. Draft generation 不新增 batch knob，复用现有 budget breaker 和 cadence deficit math。
6. 冷启动 opportunities 可以进入 Plan 并自动 draft，前提是 profile 已被用户确认，且后续 draft review gate 仍然保留。
7. Workflow advancement 默认开启，但需要 per-project kill switch，避免自动 draft generation 在异常情况下持续消耗预算。

## 15. 后续产品决策

1. 是否需要 max-publishes-per-day，防止一次 approve 多篇时发布 burst。
2. 是否允许一个 opportunity 生成多个 topics；如果允许，再引入 action-topic join table。
3. 是否需要 review cap，把剩余 opportunities 放入 secondary queue。
4. 是否将 `content_actions.status` 重命名为更精确的 workflow states。

## 16. 推荐结论

本阶段推荐优先做 `Opportunity reviewed -> Plan generated -> Draft generation started` 这条链路。它是用户当前最明显的断点，也是 Home Growth Loop 是否可信的关键。

实现上建议采用轻量 `workflow_events` advancement outbox + project-level workflow service，而不是把所有逻辑直接塞进 HTTP handlers。HTTP handler 只负责记录用户动作和 enqueue event；workflow service 负责幂等推进。Worker 复用 notification delivery worker 的 poll/backoff/dead-letter 模式，并在关键状态变化时继续通过现有 notification Dispatcher 通知用户。这样既能保证用户操作后马上有反馈，又能保留 cron reconciliation 作为兜底，不会因为单次 worker/LLM/crawler 失败导致整个产品状态长期卡住。
