# PRD: CiteLoop Workflow Handoff Link Cards

> 日期：2026-07-05（修订版，含 PM review 决议）  
> 范围：Project workflow pages 中 Content 被送到下一层后的去向可见性  
> 目标：当一个 Content 进入下一层 workflow 后，上一层页面必须保留一个不可展开的 link card，指向该 Content 当前所在的下一层位置，让用户知道“它去了哪里”。  
> 规范地位：本文档是 handoff link card 机制的**唯一规范**。`docs/PRD-CiteLoop-Opportunity-Review-and-Work-Queues.md` §8 与《Loop Lifecycle and Content Plan UX》PRD 中与 handoff link / deep link 重叠的条款，凡与本文档冲突，以本文档为准。

## 1. 背景

CiteLoop 的主工作流已经收敛为：

1. Opportunities
2. Content Plan
3. Review
4. Publish
5. Results

当前体验里，用户在一个页面完成动作后，Content 往往从原列表消失，只通过 toast、汇总数字、页面文案或下游页面列表体现状态变化。对用户来说，这会造成两个问题：

- 用户知道自己刚刚点了 `Add to plan`、`Draft now`、`Approve` 或 `Publish now`，但不一定知道这条 Content 当前在哪里。
- 用户想回看刚刚送走的 Content 时，需要在下一层重新搜索或推断状态。

CiteLoop 需要一个统一规则：**Content 被送到下一层后，上一层必须保留去向卡片。** 这张卡片复用原 card 的核心信息，但变成更轻、更明确的 link card。它不是新的可操作实体，也不能继续展开；它只负责解释状态和把用户带到下一层对应 Content。

## 2. 核心规则

### 2.1 Handoff Link Card Rule

在一个页面上的一个 Content，如果它在当前 workflow 链路里已经被送到下一层，则当前页面必须展示一个上一层 handoff link card：

- 这张 card 必须链接到下一层中对应的 Content。
- 这张 card 不可展开，不打开 drawer，不进入 inline edit。
- 这张 card 应复用原 card 的核心识别信息：title、type/status chip、目标 URL 或关键词、最近状态。
- 优先把原 card 转换为 sent-state link card；如果当前列表只展示 active items，则在 Recently Sent 区域复用同一 card 的 compact variant。
- 这张 card 可以比原 card 更小、更轻。
- UI designer 决定视觉样式，但必须让它与“尚未送到下一层、仍需当前页操作的 Content”明显区分。
- 如果下一层暂时还没有实体 id，card 必须链接到下一层页面的可解释状态，例如 pending/generating queue，并显示 pending 文案。

### 2.2 Exit Rule（退场规则）

Handoff link card 的移除**只由下游事件驱动，不使用时间窗口或数量上限**：

```text
Content 被送走
-> 原 card 变为 handoff link card，进入当前层的 "Recently sent" 折叠分组
-> 当下游 item 被进一步处理、进入再下一阶段后
-> 该 link card 从 Recently sent 分组移入 history，不再默认展示
```

规则说明：

- 只要下游 item 还停留在“刚被送达”的阶段，link card 就一直保留。用户 approve 后一周不去下游处理，card 依然在——这是本规则的本意：不丢上下文。
- 淤积靠**展示层折叠**化解，而不是靠移除：已送出的 card 归入默认折叠的 `Recently sent (n)` 分组；待处理 card 永远排在分组之上。批量操作（例如一次 approve 20 条 draft）产生的 card 全部进入该分组，由分组消化。
- 折叠分组展开时默认显示最近 3–6 条，其余通过组内滚动或 `View all in [destination]` 进入下游查看。
- 本退场规则与 `PRD-CiteLoop-Opportunity-Review-and-Work-Queues.md` §8.3 一致；该文档中如有表述差异，以本节为准。

### 2.3 Root Exception

如果 Content 没有上一层，不要求 handoff link card。

例子：

- Opportunities 页面上新发现的 opportunity finding 是 root content。
- Context/source evidence 是 setup/root content，不由上一层 workflow 送入。

Root content 自身可以使用 drawer、details 或 decision card；本 PRD 只约束它被送往下一层后的去向展示。

## 3. 目标

1. 用户完成一次推进动作后，在原页面能看到该 Content 已进入哪一层。
2. 用户能从原页面直接点击进入下一层对应 Content，而不是只进入下一层首页。
3. 推进后的 card 不再保留原操作 affordance，避免用户误以为它仍可在上一层继续处理。
4. 尽量复用原 card 的结构、文案和数据，不为 handoff 状态新建一套复杂 UI。
5. 统一主链路、direct action 链路、canonical/variant 链路的去向表达。

## 4. 非目标

- 不要求重做所有 workflow 页面布局。
- 不要求新增独立 detail page；可以通过 query/hash/focus state 深链到下一层页面内的对应 card。
- 不要求 handoff card 支持所有操作；它只负责导航和解释状态。
- 不要求保留无限历史。可以限制 Recently Sent / Recently Moved 数量。
- 不把 toast 当作满足本规则的 UI。toast 可以保留，但不能替代 handoff link card。
- 不把顶部汇总数字、metric card 或泛化 `View results` link 当作满足本规则的 UI。

## 5. 名词定义

| 名词 | 定义 |
|---|---|
| Content | 用户可识别、可跟踪的工作项，例如 opportunity、topic、draft article、publish item、direct action、measurement action。 |
| 当前层 | 用户刚刚处理 Content 的 workflow 页面。 |
| 下一层 | Content 被送往的后续 workflow 页面。 |
| 原 card | Content 仍在当前层等待处理时使用的 card。 |
| Handoff link card | Content 被送走后，在当前层留下的 compact link card。 |
| Recently Sent | 当前层展示已送往下一层 Content 的区域名。实际文案可由 UI designer 调整，例如 `Sent to Review`、`Recently moved`、`Now in Publish`。 |

## 6. Workflow 覆盖范围

### 6.1 Opportunities -> Content Plan

状态：应满足本规则。

当用户把 opportunity finding 加入 Content Plan 后：

- 原 opportunity card 不需要继续保留为可展开 decision card。
- 当前层应在 Recently Sent / Loop in motion / equivalent 区域展示 handoff link card。
- Card 链接到 Content Plan 中对应 topic/action content。
- Card 文案清楚表达该 item 已进入 Content Plan，例如 `Sent to Content Plan`、`Planning`、`Generating topic`。
- 如果 topic 尚未生成，只能链接到 Content Plan 的 pending handoff state，并显示 `Generating plan item`。

验收标准：

- 添加 opportunity 后，用户不离开 Opportunities 页面也能看到它去了 Content Plan。
- Handoff card 是 link，不打开 opportunity drawer。
- Handoff card 与 open opportunity decision cards 视觉上有明显区别。

### 6.2 Content Plan -> Review

当前缺口：topic draft 后通常从 backlog 消失，只留下 toast 或计数。

当用户点击 `Draft now`，或系统按 cadence 自动把 topic 送入 Review：

- Content Plan 必须保留对应 topic 的 handoff link card。
- Card 链接到 Review 中对应 draft article。
- 如果 writer/QA 仍在运行，card 链接到 Review 页面中该 pending/recovering draft 的位置，或链接到 Review 并带 pending state query。
- Card 不可展开，不显示 schedule/edit/archive 操作。
- Card 应复用 topic title、channel、format、target keyword/prompt 等识别信息。

建议区域：

- `Sent to Review`
- `Recently drafted`
- `Now in Review`

验收标准：

- Draft 生成或进入后台生成后，原 topic 不会只从 Plan 消失。
- 用户能从 Content Plan 点击进入 Review 中的对应 draft。
- 已送出的 topic card 不再显示 `Schedule`、`Edit`、`Draft now`、`Archive`。

### 6.3 Review -> Publish

当前缺口：draft approve 后离开 Review queue，只留下 toast；Review 页面没有 sent-to-publish card。

当用户 approve draft，或批量 approve ready drafts 后：

- Review 必须保留对应 draft 的 handoff link card。
- Card 链接到 Publish 中对应 canonical/variant publish item。
- Card 显示 `Approved for Publish`、`Scheduled`、`Ready to publish` 或类似下一层状态。
- Card 不可展开，不打开 review drawer。
- Card 不显示 approve/reject/edit 操作。
- 批量 approve 时，每个被送出的 draft 都应有独立 card，数量可以受 Recently Sent limit 限制。

建议区域：

- `Sent to Publish`
- `Recently approved`
- `Now in Publish`

验收标准：

- Approve 后，用户仍能在 Review 页面看到这条 draft 去了 Publish。
- Handoff card 点击后，Publish 页面能定位到对应 article。
- Review queue card 与 sent card 在视觉上明显不同。

### 6.4 Publish -> Results

当前缺口：Published lane 主要链接 live URL，而不是 Results 中的 measurement/attribution content。

当 canonical article 发布成功、URL verification 完成，或 direct publish item 进入 measurement：

- Publish 必须保留对应 publish item 的 handoff link card。
- Card 链接到 Results 中对应 action/article measurement content。
- Card 可以同时保留 external live URL link，但 external URL 不能替代 Results handoff link。
- Card 显示 measurement 状态，例如 `Measuring impact`、`Waiting for checkpoint`、`Published and measuring`。
- Card 不打开 publish drawer，不提供 retry/publish-now 操作。

建议区域：

- `Sent to Results`
- `Measuring impact`
- `Recently published`

验收标准：

- 发布完成后，用户在 Publish 页面能看到该 Content 已进入 Results。
- 点击 handoff card 进入 Results 并定位到对应 measurement/action card。
- Live article link 与 Results link 的用途清晰区分。

### 6.5 Direct Action -> Results

Direct action 包括 metadata rewrite、internal link patch、schema patch、technical fix、sitemap update 等不经过 topic/draft/publish 的动作。

当用户在 Analysis 中 mark applied 或 action 被系统验证通过后：

- Analysis 必须保留对应 direct action 的 handoff link card。
- Card 链接到 Results 中对应 action-level attribution content。
- Card 不打开 direct action drawer。
- Card 显示 `Applied`、`Verified`、`Measuring` 或 checkpoint 状态。

建议区域：

- `Sent to Results`
- `Applied actions`
- `Measuring direct actions`

验收标准：

- Mark applied 后，用户能在 Analysis 页面看到 action 进入 Results。
- Existing `Loop in motion` 可以承担这个区域，但 preview items 必须变成 per-item link cards，而不是普通 div 或单个泛化 `View results` link。

### 6.6 Results -> New Opportunities / Content Plan

当 Results 学到 negative/mixed/positive/inconclusive outcome，并生成下一轮 opportunity 或 plan adjustment：

- **默认路径**：系统把结果反馈成新的 opportunity，进入 Opportunity Queue 等待用户 review；Results 显示 handoff link card 到 Opportunities 中对应 finding。
- **受限路径**：Results 直接生成 plan item 会绕过 Opportunity Queue，必须满足 approval source 规则（见 `PRD-CiteLoop-Opportunity-Review-and-Work-Queues.md` §5）——仅当该类工作在用户显式批准的 Autopilot policy 覆盖范围内，或由用户在 Results 页手动触发时才允许。此时 handoff link card 必须显示 approval source（例如 `Approved by Autopilot policy: low-risk refresh`）。
- 系统不得在无 approval source 的情况下静默把 Results 产出推进 Content Plan。
- 如果只是展示结果，没有生成下一层 Content，不要求 handoff card。

验收标准：

- 用户能看到结果如何回流到下一轮工作。
- 没有产生下一层 Content 时，不制造空的 handoff card。

## 7. UX 要求

### 7.1 Card 复用原则

Handoff card 应复用原 card 的主要信息：

- title
- entity type / asset type
- key status
- target keyword / URL / platform
- moved time 或 destination status

实现上优先采用同一 workflow card 的 sent-state variant，而不是为 handoff 重新设计一套完全不同的信息结构。UI 可以缩小尺寸、减少正文、弱化视觉重量，但用户应能认出这是刚刚被送走的同一个 Content。

但必须移除或禁用原 card 的当前层操作：

- 不保留 expand drawer affordance。
- 不保留 inline editor。
- 不保留 destructive action。
- 不保留当前层的 primary action，例如 `Draft now`、`Approve`、`Publish now`、`Mark applied`。

### 7.2 Visual Differentiation

UI designer 决定最终视觉方案，但必须满足：

- 当前层待处理 card 和已送出 handoff card 一眼可区分。
- Handoff card 应更轻、更紧凑，避免抢占当前层主要决策队列。
- Handoff card 应明确显示 destination，例如 `In Review`、`In Publish`、`Measuring in Results`。
- Handoff card 的点击目标应清晰；整张 card 可以是 link。
- 不使用展开箭头、drawer chevron 或 `Open details` 文案。

必选结构（配合 §2.2 退场规则）：

- 每个页面的已送出 card 统一归入默认折叠的 `Recently sent (n)` 分组；待处理/需决策 card 永远排在该分组之上。
- 分组展开时默认显示最近 3–6 条，其余通过组内滚动或 `View all in [destination]`。

可选设计方向（分组内单张 card 的样式）：

1. 使用 compact card row，左侧保留 title，右侧显示 destination chip 和 arrow。
2. 使用 muted/tinted card，把 destination chip 作为主视觉。

### 7.3 Link Behavior

Handoff card link 必须尽量定位到下一层对应 Content：

- Content Plan: `/projects/[id]/plan?topic=[topicId]` 或 hash/query equivalent。
- Review: `/projects/[id]/review?article=[articleId]`。
- Publish: `/projects/[id]/publish?article=[articleId]`。
- Results: `/projects/[id]/results?action=[contentActionId]` 或 `/projects/[id]/results?article=[articleId]`。
- Opportunities: `/projects/[id]/analysis?opportunity=[opportunityId]`。

如果当前没有 query/hash deep-link infrastructure，V1 可以先链接到下一层页面，并在 PRD implementation 中补充定位机制。最终验收必须支持定位对应 Content。

Pending 目标态的链接格式：当下一层实体 id 尚不存在（例如 draft 仍在生成）时，统一使用：

```text
[destination route]?pending=[source entity id]
```

目标页面必须渲染可解释的 pending 状态区（例如 generating/recovering queue），并在实体 id 产生后可被相同 source id 解析到最终实体。禁止各页面自造 pending 链接格式。

### 7.4 Recency And Retention

Handoff card 的保留与移除遵循 §2.2 Exit Rule：**事件驱动，不设时间窗口或数量上限。**

- Link card 保留在 `Recently sent` 折叠分组中，直到下游 item 进入再下一阶段，随后移入 history。
- 页面重量靠折叠控制：分组默认折叠、展开时默认显示最近 3–6 条、其余走 `View all in [destination]`。
- History 不在上一层页面默认展示，但数据不删除，供审计与回溯。

### 7.5 Stale Links

下游实体被 dismiss、删除或合并后，handoff link card 不得指向 404 或空聚焦：

- Card 转为 stale 态，显示 `This item moved or was completed`。
- 保留进入目标页面的安全入口（不带失效的实体定位参数）。
- Stale 态视同“下游已接手”，card 按 §2.2 移入 history。
- 该行为与 `PRD-CiteLoop-Opportunity-Review-and-Work-Queues.md` §8.8 的 stale 规则一致。

## 8. Data Requirements

每条 handoff card 至少需要：

- source entity id。
- destination route。
- destination entity id（**nullable**：pending 时为空，此时必须携带 source entity id 并按 §7.3 的 pending 链接格式渲染）。
- destination workflow stage。
- display title。
- display subtitle。
- status label。
- moved_at 或 updated_at。

`moved_at` 的数据来源说明：现有表未必记录“送出时刻”（`updated_at` 会被后续状态变更覆盖）。V1 允许用状态推导的 `updated_at` 近似，并在 UI 上显示为 relative time；如果近似造成明显误导（例如下游多次状态变更后时间被刷新），应在后端补充显式 `moved_at` 字段。**这可能是后端工作量，不应默认按零成本估计。**

可复用现有数据：

- `visibilitySummary.actions_in_loop`
- `content_actions.draft_article_id`
- `topics.source_content_action_id`
- `articles.topic_id`
- article status such as `approved`, `published`, `pending_url_verification`
- action status such as `ready_for_review`, `approved`, `measuring`, `completed`

如果现有 API 无法返回 destination id，应扩展现有 response，而不是新建平行 workflow 表。

## 9. Page Requirements

### 9.1 Opportunities / Analysis

Required handoff areas:

- `Sent to Content Plan` for accepted opportunities that became plan content.
- `Sent to Results` for direct actions that were applied/verified.

Existing `Loop in motion` may be reused if:

- Each preview item becomes a link card.
- Each item has a destination route.
- The card does not open an Analysis drawer.

### 9.2 Content Plan

Required handoff areas:

- `Sent to Review` for topics that generated draft articles.

Content Plan backlog should keep active plan topics separate from sent topics.

### 9.3 Review

Required handoff areas:

- `Sent to Publish` for approved drafts.

Review should keep current decision cards separate from approved/sent cards.

### 9.4 Publish

Required handoff areas:

- `Sent to Results` for published or verified canonical articles.
- `Sent to Results` or `Distributed` for variants whose distribution is recorded and measurable.

Published/live URL cards must not only point outward; they also need a CiteLoop Results handoff when measurement exists.

### 9.5 Results

Required handoff areas:

- `Sent to Opportunities` when a result generates a new opportunity.
- `Sent to Content Plan` when a result directly creates or updates a plan item.

No handoff card is needed when Results only displays measurement.

## 10. Accessibility Requirements

- Handoff card must be keyboard-focusable as a link.
- Link accessible name must include title and destination, for example `Open "OAuth guide refresh" in Review`.
- Card must not use `aria-expanded`.
- Card must not trap focus or open modal/drawer.
- Destination chip text must not be the only cue; visual state should not depend only on color.

## 11. Analytics And Observability

Track:

- `workflow_handoff_card_shown`
- `workflow_handoff_card_clicked`
- source workflow
- destination workflow
- entity type
- entity id
- destination entity id
- `destination_state: ready | pending | stale`

Use these events to answer:

- Do users successfully follow moved Content?
- Which handoff transitions cause confusion?
- Are users clicking generic nav instead of handoff cards?

## 12. Acceptance Criteria

1. Every non-root workflow transition has a handoff link card on the previous page until the downstream item advances to its next stage (§2.2).
2. Handoff cards are not expandable and do not open drawers.
3. Handoff cards link to the destination workflow and identify the corresponding Content.
4. Active/current-layer cards remain visually distinct from sent/handoff cards.
5. Toast-only, metric-only, and generic destination-page links do not count as satisfying the rule.
6. Opportunity finding root cards remain allowed to open decision drawers before they are accepted.
7. Existing cards are reused where practical, with compact sent-state variants.
8. Automated transitions, not only user-click transitions, produce handoff visibility.
9. Sent cards live in a collapsed `Recently sent (n)` group; needs-action cards always render above the group.
10. Handoff card removal is triggered only by downstream advancement or stale detection, never by time windows or count caps.
11. Pending destinations use the `?pending=[source entity id]` link format and render an explanatory pending state (§7.3).
12. Stale destinations render the stale state defined in §7.5 and never produce a 404 or empty focus.
13. Results-to-Content-Plan direct creation displays an approval source and never occurs without one (§6.6).

## 13. Implementation Phases

按“深链基础设施现成度”排期：

### Phase 1: Opportunities -> Content Plan 与 Direct Action -> Results

- 复用 `Loop in motion`（per-item link card 与彩色 destination badge 已在 #217/#222 落地）。
- 落地 `Recently sent` 折叠分组与 §2.2 事件驱动退场规则。
- 契约测试覆盖：sent card 为 link 而非 button/details。

### Phase 2: Content Plan -> Review

- `/review?article=` 深链已存在，主要工作是 Content Plan 侧的 sent-state card 与折叠分组。
- 落地 pending 链接格式（draft 生成中场景最常见于此层）。

### Phase 3: Review -> Publish 与 Publish -> Results

- 新增 `/publish?article=` 深链与 Publish 页定位/高亮。
- 新增 Results 页 `?action=` / `?article=` 定位（如尚未支持）。
- Publish 卡片区分 live URL link 与 Results handoff link。

### Phase 4: Results 回流与全量收尾

- Results -> Opportunities / Content Plan handoff（依赖 §6.6 的 approval source 约束落地）。
- Stale 态全量覆盖与 analytics `destination_state` 维度上线。

## 14. Open Questions

1. ~~Should handoff retention be time-based, count-based, or both?~~
   **已决议（2026-07-05）：事件驱动退场，不使用时间或数量上限；见 §2.2。**
2. Should direct action handoff cards live in Analysis `Loop in motion`, or a separate `Sent to Results` section?
3. Should Review approved cards link to Publish item cards or to article detail when Publish cannot deep-link yet?
4. Should published canonical cards in Publish prioritize `View in Results` over `Live article`, or show both?
5. Should Home include a cross-workflow version of Recently Sent, or should handoff cards stay only on previous workflow pages?

## 15. Implementation Notes

These notes are guidance, not mandatory UI design:

- Prefer a shared `HandoffLinkCard` primitive or a sent-state variant of existing workflow cards.
- Add destination builders per workflow instead of hardcoding routes inside every card.
- Add query/hash support to destination pages so handoff links can focus the relevant item.
- Keep the old card body compact; reuse title/chips/subtitle but remove action rows.
- Extend current API responses only where destination ids are missing.
- Add contract tests that search for handoff card sections and verify sent cards use links rather than buttons/details.

## 16. Out Of Scope For This PRD

- Reworking drawer contents.
- Rewriting SEO/GEO scoring.
- Changing workflow state machines.
- Adding new automation behavior.
- Adding long-term audit history beyond recent handoff visibility.
