# PRD：CiteLoop SEO 自动驾驶

> 阶段：第三阶段，建立在 `PRD-CiteLoop-SEO-Operations-Loop.md` 之上。
> 日期：2026-06-07
> 目标：让 CiteLoop 在明确策略、预算、风险边界和人工审计机制下，自动规划、执行、验证和复盘 SEO 内容运营动作。

## 1. 第一性原理

SEO 自动驾驶不是“让 AI 随便改网站”，而是一个带约束的闭环控制系统。

系统必须同时满足：

1. **目标函数明确。** 优化什么必须可配置：搜索曝光、点击、转化、产品教育、品牌覆盖、AI answer visibility。
2. **动作边界明确。** 哪些动作能自动执行，哪些必须人工 review，哪些永远禁止。
3. **证据链完整。** 每个动作都必须能追溯到数据、假设、diff、发布、结果。
4. **可回滚。** 自动改动必须可定位、可撤销、可降级。
5. **不牺牲事实安全。** SEO 增长不能用 hallucinated product claim 换取。
6. **承认搜索延迟与不确定性。** 不把短期排名波动当作因果证明。

本 PRD 的目标是“guarded autonomy”，不是无人监管的黑箱。

## 2. 与第二阶段的关系

第二阶段提供：

- GSC/GA4 数据同步。
- Opportunity engine。
- Content action queue。
- Weekly brief。
- Outcome measurement。

第三阶段在此基础上新增：

- SEO objective manager。
- Autopilot policy。
- Portfolio planning。
- Autonomous action execution。
- Experiment and rollout framework。
- Guardrail verifier。
- Outcome learning loop。
- Rollback and audit。

如果第二阶段数据不稳定，本阶段不得启用自动执行。

## 3. 目标

1. 允许内部运营者定义 SEO objectives 和 risk policy。
2. 系统每周自动生成 SEO action portfolio。
3. 系统按 policy 自动执行低风险动作。
4. 中高风险动作自动生成 draft/PR，但必须人工 approve。
5. 每个动作进入 experiment / measurement loop。
6. 系统能根据 outcome 调整 future prioritization。
7. 所有自动动作可审计、可回滚、可暂停。
8. 系统能在异常时自动进入 safe mode。

## 4. 非目标

- 不承诺排名或流量。
- 不做黑帽、灰帽、伪外链、自动 spam。
- 不自动删除高价值页面。
- 不绕过 Google 的普通索引机制。
- 不抓取违反条款的 SERP 页面。
- 不用单一 LLM 判断长期 SEO 成败。
- 不让 agent 获得无限制 repo 写权限。

## 5. 自动驾驶等级

### Level 0：Manual

- 系统只展示数据。
- 所有动作人工完成。

### Level 1：Assistive

- 系统生成 opportunity 和 draft。
- 人工选择、人工 approve、人工 publish。

### Level 2：Guarded execution

- 低风险动作可自动执行。
- 中高风险动作必须人工 approve。
- 默认推荐阶段。

### Level 3：Portfolio autopilot

- 系统按周自动选择动作组合。
- 低风险自动发布。
- 中风险批量 review。
- 高风险只生成 plan。

### Level 4：Full autopilot

- 系统可自动创建、刷新、内链、metadata、sitemap、measurement。
- 仍保留 kill switch、预算上限、风险边界和 audit。
- 本 PRD 只定义通向 Level 4 的架构，不把 Level 4 作为首个交付目标。

## 6. 自动执行动作矩阵

| 动作 | 风险 | Level 2 | Level 3 | Level 4 |
|---|---:|---|---|---|
| submit sitemap | low | auto | auto | auto |
| metadata rewrite for low-traffic page | low | auto with rollback | auto | auto |
| add internal link from approved source | low/medium | review required | auto if policy allows | auto |
| refresh paragraph with existing evidence | medium | review required | review required | auto if confidence high |
| create supporting article | medium | review required | review required | auto with sampling review |
| update canonical article substantially | high | review required | review required | review required by default |
| merge pages | high | plan only | plan only | review required |
| noindex/prune/delete | high | plan only | plan only | review required |
| change robots/canonical rules | high | plan only | review required | review required |

默认产品策略：

- Level 2 起步。
- 删除、noindex、merge、redirect 永远不全自动。
- 所有 product claim 修改必须过 fact-safety QA。

## 7. 数据模型

### 7.1 `seo_objectives`

字段：

- `id`
- `project_id`
- `name`
- `status`
- `primary_metric`：`clicks`, `impressions`, `conversions`, `qualified_sessions`, `ai_visibility`
- `secondary_metrics`
- `target_pages`
- `target_topics`
- `target_queries`
- `time_horizon_days`
- `budget_usd`
- `created_at`
- `updated_at`

### 7.2 `seo_policies`

字段：

- `id`
- `project_id`
- `autopilot_level`
- `weekly_action_limit`
- `monthly_budget_limit`
- `allowed_action_types`
- `blocked_url_patterns`
- `requires_review_action_types`
- `max_auto_changes_per_page_per_month`
- `min_confidence_for_auto_publish`
- `quiet_hours`
- `kill_switch_enabled`
- `created_at`
- `updated_at`

### 7.3 `autopilot_runs`

字段：

- `id`
- `project_id`
- `objective_id`
- `status`
- `mode`：`observe`, `draft`, `guarded`, `portfolio`
- `started_at`
- `finished_at`
- `input_snapshot`
- `selected_actions`
- `rejected_actions`
- `guardrail_results`
- `published_changes`
- `cost_usd`
- `error`

### 7.4 `seo_action_plans`

字段：

- `id`
- `project_id`
- `autopilot_run_id`
- `objective_id`
- `plan_window_start`
- `plan_window_end`
- `status`
- `actions`
- `expected_impact`
- `expected_effort`
- `aggregate_risk`
- `approval_required`
- `approved_by`
- `approved_at`

### 7.5 `seo_experiments`

字段：

- `id`
- `project_id`
- `action_id`
- `hypothesis`
- `baseline_start`
- `baseline_end`
- `measurement_start`
- `measurement_end`
- `primary_metric`
- `secondary_metrics`
- `control_pages`
- `result`
- `confidence`
- `notes`

### 7.6 `guardrail_checks`

字段：

- `id`
- `project_id`
- `action_id`
- `check_type`
- `status`
- `severity`
- `details`
- `created_at`

### 7.7 `rollback_records`

字段：

- `id`
- `project_id`
- `action_id`
- `rollback_type`
- `source_commit_sha`
- `rollback_commit_sha`
- `reason`
- `performed_by`
- `created_at`

## 8. Planner / Executor / Verifier 架构

### 8.1 Planner

输入：

- SEO objectives
- current opportunities
- historical outcomes
- budget
- policy
- content calendar
- risk limits

输出：

- weekly action portfolio
- selected / rejected reason
- expected effort
- expected impact
- risk summary

Planner 必须解释：

- 为什么选这些动作。
- 为什么没有选其他高分 opportunity。
- 这些动作是否互相冲突。
- 本周计划是否超过运营 capacity。

### 8.2 Executor

输入：

- approved action plan
- action-specific draft generator
- repo publisher
- UniPost verification

输出：

- draft
- diff
- commit
- publish result
- measurement schedule

约束：

- 低风险动作可自动执行。
- 需要 review 的动作必须停在 review gate。
- 所有代码/content 写入必须走 branch/path guard。

### 8.3 Verifier

检查：

- fact safety
- SEO metadata
- canonical consistency
- no unsafe MDX
- no duplicate title
- internal link validity
- target URL 2xx
- sitemap inclusion
- no policy violation

Verifier 失败时：

- action 标为 `blocked`。
- 发 notification。
- 不 publish。

### 8.4 Outcome learner

输入：

- experiment outcome
- action metadata
- baseline
- page/query trends

输出：

- improved / neutral / worsened / inconclusive
- reason
- future scoring adjustment

限制：

- 不用单个动作的短期结果大幅改动全局策略。
- 小样本 outcome 只能弱信号。

## 9. Guardrails

### 9.1 Fact guard

任何产品事实变化必须映射到：

- product profile
- source URL
- inventory evidence
- user-approved fact

否则 blocking。

### 9.2 Brand guard

检查：

- title 是否 clickbait
- tone 是否偏离 brand
- competitor mention 是否安全
- claims 是否过度承诺

### 9.3 Technical SEO guard

检查：

- canonical URL
- robots meta
- noindex
- sitemap
- structured data parse
- slug consistency
- redirect chain

### 9.4 Cannibalization guard

任何新文章或大 refresh 必须检查：

- 是否已有同 intent 页面。
- 是否会抢同一 query。
- 是否应改成 section 而不是 new article。

### 9.5 Risk guard

如果动作满足任一条件，必须人工 review：

- high traffic page
- homepage / pricing / docs critical page
- merge / prune / noindex
- major rewrite
- claim confidence below threshold
- opportunity confidence below threshold
- policy explicit block

## 10. 用户体验

### 10.1 Autopilot Overview

展示：

- current mode
- active objectives
- weekly budget
- actions planned
- actions executed
- actions waiting review
- actions measuring
- guardrail failures
- kill switch

### 10.2 Policy Editor

运营者可设置：

- autopilot level
- allowed actions
- review-required actions
- weekly action limit
- budget limit
- URL allow/deny patterns
- minimum confidence
- quiet hours
- notification preferences

### 10.3 Weekly Plan Review

展示：

- portfolio summary
- selected actions
- rejected actions
- risk notes
- expected measurement date
- approve all low-risk
- approve individually
- pause plan

### 10.4 Action Audit Detail

必须展示：

- original opportunity
- plan reasoning
- generated diff
- guardrail results
- commit/deploy verification
- measurement result
- rollback button if applicable

### 10.5 Outcome Dashboard

展示：

- actions by result
- improved / neutral / worsened / inconclusive
- metric deltas
- confidence
- lessons learned
- model/scoring adjustments

## 11. Autopilot workflow

### 11.1 Weekly planning

1. `seo_sync` 完成。
2. `seo_analyzer` 更新 opportunities。
3. Planner 读取 objective/policy。
4. Planner 生成 weekly action plan。
5. Verifier 预检查 plan-level risk。
6. 根据 autopilot level：
   - Level 1：只展示。
   - Level 2：低风险进入执行，其余进入 review。
   - Level 3：生成 portfolio review。

### 11.2 Execution

1. Executor claim action。
2. Generate draft/diff。
3. Run guardrails。
4. If allowed auto publish：commit/publish。
5. Else：send to review queue。
6. Verify URL。
7. Create measurement schedule。

### 11.3 Measurement

1. 等待 observation window。
2. Pull GSC/GA4 data。
3. Compare baseline。
4. Mark outcome。
5. Update planner memory。
6. Include in weekly brief。

### 11.4 Safe mode

进入条件：

- publish failures exceed threshold。
- verified notification channel missing。
- Google auth expired。
- guardrail false positive/negative manually reported。
- traffic drop anomaly after recent autopilot changes。
- budget exceeded。

Safe mode 行为：

- 停止 auto publish。
- 允许 observe/draft。
- 发 critical notification。
- UI 显示恢复条件。

## 12. 实验与评估

### 12.1 Baseline

每个动作必须记录：

- baseline period：默认 action 前 28 天。
- comparable previous period。
- seasonality note。
- affected query/page set。

### 12.2 Measurement windows

默认：

- metadata：7/14/28 天。
- internal links：14/28/56 天。
- refresh：14/28/56 天。
- new article：28/56/90 天。
- merge/prune：28/56/90 天。

### 12.3 Outcome labels

- `improved`
- `neutral`
- `worsened`
- `inconclusive`

判断标准必须考虑：

- site-wide trend
- query volume changes
- index status
- page availability
- baseline noise

### 12.4 Learning

Planner 可学习：

- 哪类 action 在本项目上更有效。
- 哪些 query intent 转化更高。
- 哪些 content formats 表现更好。
- 哪些风险信号导致失败。

Planner 不得：

- 因一个小样本结果永久屏蔽某类动作。
- 把 correlation 当作确定 causation。

## 13. Notifications

新增事件：

- `autopilot.plan.ready`
- `autopilot.action.published`
- `autopilot.action.blocked`
- `autopilot.safe_mode.enabled`
- `autopilot.measurement.ready`
- `autopilot.rollback.completed`

所有事件必须：

- stable event id
- anti-spam window
- dashboard URL
- project/action/run ids

## 14. 安全、权限与审计

- 所有自动写入必须绑定 actor：`autopilot`。
- 所有 commit message 包含 action id。
- 所有 generated changes 可追溯到 input snapshot。
- Kill switch 必须在 UI 第一屏可见。
- 高风险动作永远不能绕过 review。
- Google credentials 和 repo credentials 不进 logs。
- Policy 变更要写 audit log。
- Autopilot mode 提升必须二次确认。

## 15. 失败处理

### 15.1 Plan failure

- 不执行任何动作。
- 写 run failure。
- 发 notification。

### 15.2 Guardrail failure

- action blocked。
- 显示 failed check。
- 允许人工 override，但 override 要写 reason。

### 15.3 Publish failure

- 复用 MVP publish_failed/backoff。
- Autopilot 暂停同类型动作。

### 15.4 Measurement failure

- action 保持 measuring。
- 重试。
- 超过窗口后标 inconclusive。

### 15.5 Outcome worsened

- 标记 negative result。
- Planner 降低类似 action confidence。
- 如果 drop 超过 critical threshold，进入 safe mode。

## 16. Rollout plan

### Phase 1：Observe-only autopilot

- Objective manager。
- Policy editor。
- Weekly plan generation。
- No auto execution。

### Phase 2：Draft autopilot

- Accepted plan 自动生成 draft。
- 所有动作仍需人工 publish。
- Guardrail dashboard。

### Phase 3：Guarded low-risk execution

- metadata and sitemap low-risk auto execution。
- internal link auto execution only when policy allows。
- kill switch。
- rollback records。

### Phase 4：Portfolio autopilot

- Weekly portfolio approval。
- Limited autonomous publish。
- Measurement learning loop。
- safe mode。

### Phase 5：Expanded autonomy

- High-confidence refresh auto execution。
- Sampling review。
- advanced experiment analysis。
- Still no automatic prune/delete.

## 17. 验收清单

1. 可创建 SEO objective。
2. 可配置 autopilot policy。
3. Observe-only 模式能生成 weekly action plan。
4. Plan 每个 action 都有 reason、evidence、risk、expected impact。
5. Policy 能阻止不允许的 action。
6. Low-risk action 可自动生成 draft。
7. Guardrail checks 全部落库并可查看。
8. Fact guard 能阻止无 evidence 产品事实。
9. Technical guard 能阻止 canonical/noindex/unsafe MDX 问题。
10. Level 2 下 low-risk metadata rewrite 可自动发布。
11. 中高风险动作不会自动发布。
12. Publish 后写 measurement schedule。
13. Outcome measurer 能给出 improved/neutral/worsened/inconclusive。
14. Planner 能读取 historical outcome 调整 future scoring。
15. Safe mode 可由 kill switch 手动开启。
16. Safe mode 可由连续 publish failure 自动开启。
17. Safe mode 开启后不再 auto publish。
18. Action audit detail 可追溯到 opportunity、plan、diff、commit、measurement。
19. Rollback record 可创建。
20. Notification 事件可投递。
21. No raw secret in logs/API/UI。
22. `go test ./...` 通过。
23. `web npm run build` 通过。

## 18. Definition of Done

CiteLoop 达到“接近自动驾驶 SEO”时，内部运营者可以这样工作：

1. 设置 SEO objective 和 policy。
2. 每周收到 autopilot plan。
3. 系统自动执行低风险动作。
4. 系统把中高风险动作送入 review。
5. 系统自动发布已允许的动作。
6. 系统自动测量 outcome。
7. 系统根据结果调整下周计划。
8. 出现异常时系统自动进入 safe mode。
9. 运营者随时能审计和回滚任何自动动作。

做到这里，CiteLoop 才从“持续 SEO 运营助手”进入“受控自动驾驶 SEO 系统”。
