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

如果第二阶段数据不稳定，本阶段不得启用自动执行。Level 2 自动执行的硬前置：

- Operations Loop 达到 Definition of Done，并通过机器可读 health check。
- `seo_sync` 连续 14 天成功，且最近 28 天 GSC page total 数据可用。
- URL normalization 最近 14 天无同页重复冲突。
- verified notification channel 存在并通过测试投递。
- permission readiness 通过：`search_read + publisher_write + notification_write + autopilot_policy_confirmed` 全部 connected；`public_only` 项目不得启用 Level 2 自动执行，只能停留在 observe/draft。
- outcome measurement 至少能读取 baseline 并写入 measurement schedule。
- kill switch 和 safe mode 状态可在 UI 第一屏看到。

## 3. 目标

1. 允许内部运营者定义 SEO objectives 和 risk policy。
2. 系统每周自动生成 SEO action portfolio。
3. 系统按 policy 自动执行低风险动作。
4. 中高风险动作自动生成 draft/PR，但必须人工 approve。
5. 每个动作进入 experiment / measurement loop。
6. 系统能根据 outcome 调整 future prioritization。
7. 所有自动动作可审计、可回滚、可暂停。
8. 系统能在异常时自动进入 safe mode。
9. 真实用户只输入 product domain 的项目，也能进入自动驾驶前的冷启动观察状态；系统不得要求用户手动配置 GSC/GA4/service account。
10. 注册后的 guided permission onboarding 能一次性拿齐自动运营需要的最小权限，后续用户只参与高风险 approval、异常恢复和策略变更。

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
- `public_only` 项目最高只能到 Level 1，除非接入 CiteLoop 托管内容数据或完成客户站点所有权验证。

### Level 2：Guarded execution

- 低风险动作可自动执行。
- 中高风险动作必须人工 approve。
- 默认推荐阶段。
- 要求 `search_read`、`publisher_write`、`notification_write`、`autopilot_policy_confirmed` 和 `dry_run_passed` 全部有效。

### Level 3：Portfolio autopilot

- 系统按周自动选择动作组合。
- 低风险自动发布。
- 中风险批量 review。
- 高风险只生成 plan。

### Level 4：Full autopilot

- 系统可自动创建、刷新、内链、metadata、sitemap、measurement。
- 仍保留 kill switch、预算上限、风险边界和 audit。
- 本 PRD 只定义通向 Level 4 的架构，不把 Level 4 作为首个交付目标。

Run mode 由 `autopilot_level` 派生，不单独配置：

| `autopilot_level` | derived mode | 自动发布能力 |
|---:|---|---|
| 0 | `observe` | 无 |
| 1 | `draft` | 无 |
| 2 | `guarded` | 仅低风险且 policy 允许 |
| 3 | `portfolio` | 低风险自动，中风险批量 review |
| 4 | `expanded` | 本 PRD 只定义架构，不作为首交付 |

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

### 6.1 Risk classification

风险分级必须是确定性规则，不由 LLM 自由判断。Classifier 输入：

- `action_type`
- 最近 28 天 page clicks / impressions 及项目内分位
- page type：`blog`, `docs`, `pricing`, `homepage`, `legal`, `unknown`
- diff 规模：metadata-only、link-only、paragraph、section、major rewrite
- 是否涉及 product claim、canonical、robots、redirect、merge/noindex/delete
- policy allow/deny patterns

输出：

- `risk_level`：`low`, `medium`, `high`
- `risk_reasons`
- `classifier_version`

默认规则：

- `high`：homepage/pricing/docs critical/legal；merge、redirect、noindex、delete；canonical/robots 改动；major rewrite；最近 28 天 clicks 或 impressions 位于项目 P80 以上。
- `medium`：new supporting article；paragraph/section refresh；internal link patch on non-low-traffic page；product claim diff；confidence 低于 policy 阈值。
- `low`：sitemap submit；metadata-only rewrite on low-traffic blog page；internal link patch from approved source to approved target 且 diff 小于 3 个链接。

Classifier 规则必须版本化并写入 audit，action 执行时使用的版本不可被事后覆盖。

### 6.2 Low-traffic definition

默认 `low_traffic` 判定：

- 最近 28 天 clicks `< 10`，且
- 最近 28 天 impressions `< 500`，且
- page 不属于 homepage/pricing/docs critical/legal，且
- page traffic 分位低于项目 P60。

阈值可在 `seo_policies` 配置。任何一个条件不满足都不得按 low-traffic 自动发布。

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
- `budget_usd`：planner allocation，不是硬上限；不能超过 `seo_policies.monthly_budget_limit`
- `created_at`
- `updated_at`

### 7.2 `seo_policies`

字段：

- `id`
- `project_id`
- `autopilot_level`
- `weekly_action_limit`
- `monthly_budget_limit`：SEO Autopilot 子系统硬上限
- `allowed_action_types`
- `blocked_url_patterns`
- `requires_review_action_types`
- `max_auto_changes_per_page_per_month`
- `low_traffic_clicks_28d_threshold`：默认 10
- `low_traffic_impressions_28d_threshold`：默认 500
- `min_confidence_for_auto_publish`
- `quiet_hours_start`
- `quiet_hours_end`
- `quiet_hours_timezone`
- `quiet_hours_behavior`：`defer_to_next_window` 或 `skip_cycle`，默认 `defer_to_next_window`
- `kill_switch_enabled`
- `safe_mode_enabled`
- `risk_classifier_version`
- `created_at`
- `updated_at`

预算关系：

- 现有 `generation_runs` 月度 budget breaker 是项目级最终硬上限。
- `seo_policies.monthly_budget_limit` 是 SEO Autopilot 子系统硬上限。
- `seo_objectives.budget_usd` 是某个 objective 的计划分配；多 objective 共享 `seo_policies.monthly_budget_limit`，Planner 不得让分配总和超过 policy。
- 任一预算触顶时，Planner 只能 observe/draft，不得 auto publish 或调用额外 LLM。

### 7.3 `autopilot_runs`

字段：

- `id`
- `project_id`
- `objective_id`
- `status`
- `autopilot_level_snapshot`
- `derived_mode`：由 `autopilot_level_snapshot` 推导，不允许 UI 独立设置
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
- `risk_classifier_version`
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
- `evidence_level`：`matched_control`, `site_trend_normalized`, `no_control`
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
- `reported_false_positive_at`
- `reported_false_negative_at`
- `reported_by`
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

### 7.8 `risk_classification_rules`

字段：

- `id`
- `project_id`
- `version`
- `rules`
- `created_by`
- `created_at`
- `retired_at`

约束：

- action 记录只保存使用时的 `classifier_version` 和 `risk_reasons`。
- 已执行 action 不随规则更新重算历史风险。

### 7.9 `safe_mode_events`

字段：

- `id`
- `project_id`
- `reason`
- `trigger_source`：`manual`, `publish_failure`, `auth`, `notification`, `guardrail_report`, `traffic_anomaly`, `budget`
- `entered_at`
- `entered_by`
- `exited_at`
- `exited_by`
- `exit_reason`
- `related_run_id`
- `related_action_id`

约束：

- 同一 project 同时只能有一个 open safe mode event。
- 退出一律需要人工确认；系统可以显示恢复建议，但不得自动退出 safe mode。
- safe mode open 时，Executor 不得 auto publish。

### 7.10 `autopilot_audit_events`

字段：

- `id`
- `project_id`
- `actor`：`autopilot`, `human`, `system`
- `event_type`
- `entity_type`
- `entity_id`
- `before_snapshot`
- `after_snapshot`
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
- 每个 action 使用的 `risk_classifier_version`、risk reasons 和 low-traffic 判断结果。

同页冲突规则：

- 同一 plan 内两个 action 命中同一 `target_article_id` 或 `normalized_target_url` 时，Planner 必须合并为一个复合 action 或只保留最高优先级 action。
- 不能合并的冲突 action 标为 `rejected_due_to_conflict`，并写 rejected reason。
- 同一 article 每月自动发布次数不得超过 `max_auto_changes_per_page_per_month`。

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
- Executor claim action 时必须记录 `content_hash_before`；发布前再次读取目标 article hash。
- 如果执行中发现 article 被人工编辑或 hash 与 plan snapshot 不一致，Executor 必须 abort，并把 action 转为 review。
- quiet hours 命中时，默认延后到下一个允许窗口；如果延后超过 plan window，则跳过本周期并写 audit。

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

Risk guard 调用 §6.1 的 deterministic classifier；LLM 只能补充解释，不能覆盖 classifier 输出。如果动作满足任一条件，必须人工 review：

- high traffic page
- homepage / pricing / docs critical page
- merge / prune / noindex
- major rewrite
- claim confidence below threshold
- opportunity confidence below threshold
- policy explicit block
- classifier version missing 或 policy threshold missing

## 10. 用户体验

Autopilot UI 必须继承 Operations Loop 的 domain-only onboarding 约束：

- 真实用户只看到 product domain、launch checklist、connection health、autopilot readiness。
- 不展示 `gsc_site_url`、GA4 property id、service account、credential ref 作为必填项。
- 如果项目仍是 `public_only`，Level 2+ 控件禁用，并解释为 “CiteLoop can draft recommendations from public data; automatic execution requires managed content data or verified first-party search data.”
- 内部管理员可以查看 provider-level diagnostics，但该视图不作为真实用户主路径。

### 10.0 Permission readiness gate

Autopilot Overview 第一屏必须展示 readiness gate：

| gate | Level 2 必需 | 失败时行为 |
|---|---:|---|
| `search_read` | yes | 禁用 CTR/position/decay 自动执行，只允许 draft |
| `analytics_read` | no | 降低 priority confidence，标注 no conversion signal |
| `publisher_write` | yes | 禁用 auto publish，只生成 diff/draft |
| `notification_write` | yes | 禁用 Level 2+，因为异常无法可靠通知 |
| `autopilot_policy_confirmed` | yes | 保持 Level 1，提示完成策略确认 |
| `dry_run_passed` | yes | 不执行真实写入 |

用户完成 launch checklist 后，不再需要日常处理权限。系统只在 grant expired/revoked、scope 不足、publish failure、safe mode 或 policy 提升时打扰用户。

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
- low-traffic thresholds
- quiet hours
- quiet hours timezone and behavior
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
- report guardrail false positive / false negative

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
- 写入 `safe_mode_events` open event。
- UI 显示进入原因、相关 action/run、恢复建议和人工恢复按钮。

退出机制：

- 一律需要人工确认，不自动退出。
- 恢复前必须重新验证：notification channel、Google integration、budget、最近一次 publish health。
- 退出写 `safe_mode_events.exited_at/exited_by/exit_reason` 和 audit event。

## 12. 实验与评估

### 12.1 Baseline

每个动作必须记录：

- baseline period：默认 action 前 28 天。
- comparable previous period。
- seasonality note。
- affected query/page set。

### 12.1.1 Control page selection

优先选择 control pages：

- 同 topic cluster。
- 同 page type。
- 最近 28 天 clicks/impressions 分位接近。
- 未在本 measurement window 内被其他 action 修改。

如果找不到至少 3 个合格 control pages：

- 降级为 site-trend normalized comparison。
- `seo_experiments.evidence_level` 标为 `site_trend_normalized`。
- 如果连站点趋势也不可用，标为 `no_control`，result 只能是 `inconclusive` 或弱信号。

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

### 12.5 Rollback semantics

不同 action 的 rollback 含义：

| action type | rollback_type | 可恢复内容 | 局限 |
|---|---|---|---|
| metadata rewrite | `git_revert` 或 `content_snapshot_restore` | title/meta/frontmatter | Google SERP 可能已抓取中间版本，不能保证 SERP 立即恢复 |
| internal link patch | `git_revert` 或 `content_snapshot_restore` | source article link diff | 已被 crawl 的 link graph 变化不可即时撤销 |
| paragraph/section refresh | `content_snapshot_restore` | article body diff | 事实更新若已被后续人工编辑覆盖，需人工 merge |
| new supporting article | `unpublish_or_redirect_plan` | 只能生成下架/redirect 计划 | 不自动 delete/noindex |
| sitemap submit | `not_reversible` | 无 | sitemap submit 无法真正回滚，只能提交新版 sitemap |
| merge/redirect/noindex/delete | `manual_plan_required` | 需人工制定反向 redirect/restore plan | 搜索状态和 canonical 选择不可保证恢复 |

Rollback 是内容和配置层面的恢复，不等于恢复 Google 排名、抓取状态或 SERP 展示。

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
- Phase 1 必须先迁移自动写入链路的 actor 字段或 audit event；现有 `articles` / publish 链路没有 actor 概念，不能在缺失 actor 的情况下启用自动执行。
- 所有 commit message 包含 action id。
- 所有 generated changes 可追溯到 input snapshot。
- Kill switch 必须在 UI 第一屏可见。
- 高风险动作永远不能绕过 review。
- Google credentials 和 repo credentials 不进 logs。
- Executor 每次执行前必须验证 permission grant 未过期、未撤销、scope 足够、resource 与 action target 匹配。
- Grant revoke/expire 后必须自动降级：search grant 失效停止数据驱动 action；publisher grant 失效停止 auto publish；notification grant 失效进入 safe mode。
- Policy 变更要写 audit log。
- Autopilot mode 提升必须二次确认。
- Risk classifier rule 变更必须写 audit log，并从新 action 开始生效。

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

### Phase 1：Observe-only autopilot（目标 Level 0/1）

- Objective manager。
- Policy editor。
- Weekly plan generation。
- No auto execution。
- Permission readiness gate。
- actor/audit migration。
- risk classification rules v1。
- safe mode event storage。

### Phase 2：Draft autopilot（目标 Level 1）

- Accepted plan 自动生成 draft。
- 所有动作仍需人工 publish。
- Guardrail dashboard。
- content hash conflict abort。

### Phase 3：Guarded low-risk execution（目标 Level 2）

- metadata and sitemap low-risk auto execution。
- internal link 仍默认 review required；只有 policy 显式允许且 classifier 判定 low risk 时才可 auto。
- kill switch。
- rollback records。

### Phase 4：Portfolio autopilot（目标 Level 3）

- Weekly portfolio approval。
- Limited autonomous publish。
- Measurement learning loop。
- safe mode。

### Phase 5：Expanded autonomy（目标 Level 4 架构，不作为首交付）

- High-confidence refresh auto execution。
- Sampling review。
- advanced experiment analysis。
- Still no automatic prune/delete.

## 17. 验收清单

1. 可创建 SEO objective。
2. 可配置 autopilot policy。
3. Observe-only 模式能生成 weekly action plan。
4. Plan 每个 action 都有 reason、evidence、risk、expected impact。
5. Risk classifier 用 deterministic rules 输出 risk level、risk reasons、classifier version。
6. Low-traffic 阈值可配置，默认 clicks < 10 且 impressions < 500 且非 critical page。
7. Policy 能阻止不允许的 action。
8. `autopilot_level` 到 derived mode 的映射固定，UI 不允许独立设置 mode。
9. `seo_policies.monthly_budget_limit` 和现有 `generation_runs` 月度 breaker 任一触顶时，不会 auto publish。
10. Low-risk action 可自动生成 draft。
11. Guardrail checks 全部落库并可查看。
12. Guardrail false positive/negative 可人工上报，并触发 safe mode 条件。
13. Fact guard 能阻止无 evidence 产品事实。
14. Technical guard 能阻止 canonical/noindex/unsafe MDX 问题。
15. Level 2 启用前必须确认 permission readiness：`search_read + publisher_write + notification_write + autopilot_policy_confirmed + dry_run_passed`。
16. Executor 每次执行前重新验证 permission grant 状态和 resource scope。
17. 任一必需 grant 过期、撤销或 scope 不足时，Level 2 自动执行被禁用或进入 safe mode。
18. Level 2 下 low-risk metadata rewrite 可自动发布。
19. 中高风险动作不会自动发布。
20. 同一 plan 内同页 action 会合并或拒绝；不会并发修改同一 article。
21. Executor 发现 content hash 被人工改动时 abort 并转 review。
22. Quiet hours 命中时按 policy 延后或跳过，并写 audit。
23. Publish 后写 measurement schedule。
24. Control page 不足时 measurement 降级为 site-trend normalized，并写 evidence level。
25. Outcome measurer 能给出 improved/neutral/worsened/inconclusive。
26. Planner 能读取 historical outcome 调整 future scoring。
27. Safe mode 可由 kill switch 手动开启。
28. Safe mode 可由连续 publish failure 自动开启。
29. Safe mode 开启后不再 auto publish。
30. Safe mode open event 落库，退出必须人工确认并写 exited fields。
31. Action audit detail 可追溯到 opportunity、plan、diff、commit、measurement。
32. Rollback record 可按 action type 创建，并展示 rollback 局限。
33. 所有自动写入绑定 actor 或 audit event。
34. Notification 事件可投递。
35. No raw secret in logs/API/UI。
36. `go test ./...` 通过。
37. `web npm run build` 通过。

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
