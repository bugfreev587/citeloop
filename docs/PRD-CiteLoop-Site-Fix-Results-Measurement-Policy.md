# PRD：Site Fix 验证闭环与 Results 增长测量策略

> 状态：Draft，待产品评审  
> 日期：2026-07-14  
> 范围：Doctor Site Fix、Results、SEO/GEO attribution

## 1. 背景与问题

Site Fix 负责承载 Doctor 发现后的修复工作，生命周期包含 Finding、Approved、Applied、Deployed 和 Verified。Results 负责跟踪 SEO/GEO action 的 measurement window、指标变化和 outcome attribution。

当前 Site Fix 只有 Broken 和 Optimization 两类 finding。但这两个标签不能决定是否需要增长测量：

- Optimization 可能只是让标题更容易阅读，不应该产生 Results 测量任务。
- Optimization 也可能明确以提升 CTR 为目标，应该进入 Results。
- Broken 可能只是恢复 canonical 或 sitemap 的正确性，通常不需要增长归因。
- Broken 也可能包含明确的 SEO/GEO visibility 假设，需要测量。

因此，Broken / Optimization 只能表达问题性质，不能承担 Results 路由决策。

## 2. 产品原则

### 2.1 Site Fix 证明“改对了”

Site Fix 需要验证变更应用到了正确的仓库和文件、部署成功、页面意图未改变、验收条件通过且没有引入回归。对验证型修复而言，Applied → Deployed → Verified 就是完整闭环。

### 2.2 Results 证明“产生了增长”

Results 只追踪有明确 outcome hypothesis 的 action，例如提升 organic CTR、impressions、clicks、ranking position、GEO citations、brand mentions、referral sessions、backlinks 或 conversion。

没有增长假设、primary metric 和 measurement window 的 Site Fix，不应自动进入 Results。

### 2.3 不混淆两类 Verified

Verified 表示实现正确且没有回归，不代表增长效果已经确认。对增长型 fix，Verified 是测量的起点；对验证型 fix，Verified 是生命周期终点。

## 3. 核心分类模型

### 3.1 measurement_policy

新增独立测量策略：

| 策略 | 行为 |
|---|---|
| verification_only | Applied、Deployed、Verified 后结束，不创建 Results attribution |
| measurement_required | Verified 后必须创建或激活 Results measurement |
| measurement_optional | 默认完成验证闭环，用户可以主动加入 Results |

默认值为 verification_only，除非系统生成了完整、可信且通过 readiness 校验的增长假设和测量计划。measurement_policy 一旦为某个 measurement generation 冻结，不因后续 classifier 或 policy 升级而静默改变。

### 3.2 impact_mode

新增影响模式：

| impact_mode | 典型变更 | 默认策略 |
|---|---|---|
| presentation_only | 标题更易读、描述更清晰但不改变意图 | verification_only |
| technical_reliability | canonical、robots、sitemap、HTTP、redirect | verification_only |
| search_visibility | 关键词 title/meta、内链、搜索结构优化 | 由 classifier 决定 |
| geo_visibility | entity、schema、citation、answer-engine 优化 | required |
| content_demand | 内容新增、重写、扩充以获取需求 | required |
| conversion_or_ctr | 明确提升 CTR、注册、转化 | required |

impact_mode 用于解释变更意义；最终是否进入 Results 由 measurement_policy 决定。

### 3.3 不用 finding_kind 替代 impact_mode

保留 finding_kind = broken | optimization，但它只用于问题分类和 UI badge，不参与 Results 路由决策。

## 4. Fix 类型与 Results 策略

### 4.1 默认 verification_only

以下类型主要恢复正确性、稳定性或可验证性：

| Fix type | 主要验收标准 |
|---|---|
| title_readability | 标题更清晰，页面意图不变 |
| metadata_format | metadata 格式、长度、可读性正确 |
| canonical_repair | canonical 指向正确 URL |
| robots_repair | robots 不阻断预期抓取 |
| sitemap_repair | sitemap 包含正确 URL，格式有效 |
| redirect_or_http_repair | HTTP 状态、redirect 链和目标正确 |
| schema_validity_repair | JSON-LD/schema 有效且无回归 |
| content_typo_or_clarity | 语义和页面意图保持不变 |
| technical_fix | 权限、构建、配置或安全问题恢复；`security_or_config_repair` 输入别名在写入时规范化为该类型 |

这些类型完成 Applied → Deployed → Verified 后结束，不应出现在 Results measurement queue。

### 4.2 默认 measurement_required

以下类型明确改变搜索、引用或需求表现：

| Fix type | 推荐 primary metric |
|---|---|
| metadata_ctr_optimization | CTR |
| search_title_keyword_optimization | impressions、CTR、position |
| internal_link_authority_optimization | clicks、impressions、position |
| schema_entity_optimization | rich result / parse coverage、clicks |
| geo_entity_clarity | project-owned citations、brand mentions |
| geo_citation_optimization | citations、competitor citation gap |
| content_rewrite_for_search | impressions、clicks、position、CTR |
| content_demand_expansion | impressions、clicks、qualified sessions |
| external_distribution | referral sessions、backlinks、brand mentions |
| conversion_or_cta_optimization | conversion rate、qualified actions |

这些类型必须具备 growth_hypothesis、primary_metric、baseline window 和 measurement checkpoints。

### 4.3 measurement_optional

| Fix type | 默认行为 |
|---|---|
| internal_link_patch | 修复断链时验证型；目标是 authority 时进入 Results |
| schema_patch | 修复无效 JSON-LD 时验证型；目标是实体/富结果时进入 Results |
| metadata_rewrite | 只改善可读性时验证型；目标是 CTR 时进入 Results |
| technical_fix | 默认验证型；有 visibility hypothesis 才测量 |
| geo_content_clarity | 提高实体表达准确性时可选；目标是 citation 时必须测量 |

## 5. 截图案例

截图中的 fix 是：

> Make the existing metadata easier to read without changing the page's intent.

建议分类为：

    impact_mode = presentation_only
    measurement_policy = verification_only

它的成功条件是 metadata 更易读、页面意图不变、PR 正确创建并合并、部署成功、页面验证通过。因此它不应该因为 finding_kind = optimization 而进入 Results。

如果目标改为“Shorten the title to improve organic CTR for the target query”，则分类应改为：

    impact_mode = conversion_or_ctr
    measurement_policy = measurement_required
    growth_hypothesis = A shorter title will improve organic CTR without reducing qualified impressions.
    primary_metric = ctr
    secondary_metrics = impressions, clicks, position

## 6. 生命周期与状态所有权

### 6.1 验证型 fix

    Finding → Approved → Applied → Deployed → Verified

Verified 是终点，展示：

    Verified — implementation complete

### 6.2 增长型 fix

    Site Fix: Finding → Approved → Applied → Deployed → Verified
    Measurement: Planned → Baseline ready → Observing → Terminal

- Site Fix 永远停在 verified；measurement 不复用 site_fixes.status，也不增加 measuring 状态。
- Planned：measurement generation 已创建，但 handoff 或 baseline 尚未完成。
- Baseline ready：变更前基线已经冻结。
- Observing：变更已经 Verified，正在等待 checkpoints。
- Terminal：完成 positive、negative、mixed、inconclusive 或 insufficient_data 之一。
- Results 拥有 measurement 状态；Site Fix 只拥有实现、部署和技术验证状态。

### 6.3 可选测量

measurement_optional 默认走验证型路径。用户点击 Track impact in Results 后创建新的 measurement generation；这不会改变已完成的 Site Fix Verified 结果。若 opt-in 发生在 Verified 之后，必须标记为 prospective observation，不得伪造变更前 baseline，并降低 attribution confidence。

## 7. Results measurement 持久化模型

### 7.1 选型

现有 action_measurements 强制引用 content_action_id，且既有 scheduler、terminal outcome 和 learning contract 都以 Opportunity/Content Action 为中心。本 PRD 不把 site_fix_id 直接塞进 action_measurements，也不引入 nullable polymorphic foreign key。

本期采用独立的 site_fix_measurements aggregate，并由 Results 提供统一的只读 action view。这样既不破坏现有 content action contract，也能保证 Site Fix 的跨项目 FK、checkpoint、baseline 和 terminal outcome 完整可审计。未来若需要统一写模型，再通过 read model 或明确 migration 合并，而不是在本期混合两套所有权。

### 7.2 聚合结构

新增三类持久化对象：

| 对象 | 职责 |
|---|---|
| site_fix_measurements | 一次不可变 measurement generation 的计划、baseline、状态、terminal outcome 和 Results deep link |
| site_fix_measurement_checkpoints | 每个 checkpoint 的数据、结果、重试和数据源可用性 |
| site_fix_measurement_learnings | positive、negative、mixed、inconclusive 的可复用 learning；insufficient_data 只写 quality record |

site_fix_measurements 至少包含：

- id、project_id、site_fix_id、measurement_generation
- target_url、normalized_target_url、target_query 或 entity identity
- impact_mode、fix_type、classifier_version、decision_origin、decision_confidence
- immutable policy_version 和完整 measurement_policy_snapshot
- growth_hypothesis、primary_metric、secondary_metrics
- baseline_window、baseline_snapshot、baseline_status
- started_at、absolute_terminal_at、status
- terminal_outcome、outcome_reason、attribution_confidence、confounders
- results_deep_link、created_at、updated_at

site_fix_measurement_checkpoints 至少包含：

- measurement_id、checkpoint_key、role（early_signal / primary / follow_up）
- scheduled_at、window_start、window_end、attempt_number
- required_data_sources、data_availability、minimum_sample
- seo_metrics、ga4_metrics、geo_metrics、execution_metrics
- guardrail_results、outcome_label、outcome_reason、attribution_confidence
- computed_at、failure_reason、retry_classification

关键约束：

1. site_fix_measurements 必须同时持有 project_id 和 site_fix_id，并通过 (project_id, site_fix_id) composite FK 引用 site_fixes；不能只用 site_fix_id 做跨项目关联。
2. unique(project_id, site_fix_id, measurement_generation)。
3. unique(measurement_id, checkpoint_key, attempt_number)。
4. measurement_generation 只递增，不复用；retry 不创建新 generation。
5. 任何 Results 页面都以实际 measurement row 为准，不以 measurement_policy 查询推导不存在的任务。

### 7.3 与现有 Results 的关系

Results API 增加 site-fix measurement read model，字段映射到现有 ResultsAction 形状，但保留 source_type = site_fix 和 source_id = site_fix_id。content action 继续使用 action_measurements；site fix 使用 site_fix_measurements。两者在 Results UI 统一展示，但各自拥有写入和状态机。Site Fix checkpoint worker 复用现有 scheduler 的 polling、for-update/skip-locked、backoff 和 dead-letter 语义，但不复用强制 content_action_id 的写模型。

## 8. 测量计划、baseline 与有限 contract

### 8.1 计划冻结时点

对 measurement_required：

1. Approved 时创建 measurement generation 和 immutable policy snapshot。
2. Apply 前收集 baseline window；baseline 不能晚于实际变更开始时间。
3. baseline 不可用时，generation 进入 baseline_blocked 或 insufficient_data，不得伪造 baseline；Site Fix 仍可完成自己的 Verified 闭环，但 Results 必须显示 measurement-quality 状态。
4. Verified 只触发 observation handoff，不重新计算 policy。

v1 的 pre-apply baseline freshness contract 固定为：`start < end <= captured_at <= cutoff`；`captured_at` 距 classifier cutoff 不超过 7 天，baseline `end` 距 cutoff 不超过 10 天，单个 baseline window 不超过 90 天。该窗口兼容 GSC 的常见数据延迟，同时拒绝未来、过早采集或陈旧 baseline；Approved 时必须用审批时 cutoff 再校验一次。

对 measurement_optional：

- 若在 Approved/Applied 前 opt-in，按正常 baseline 流程执行。
- 若在 Verified 后 opt-in，创建 prospective observation，baseline_status = unavailable，attribution_confidence 最高为 low，并在 Results 明确显示“无变更前基线”。

### 8.2 复用现有 Growth outcome taxonomy

terminal outcome 必须使用现有 taxonomy：

- positive
- negative
- mixed
- inconclusive
- insufficient_data

insufficient_data 不等于 negative，也不生成方向性 learning；它只生成 measurement-quality record。positive、negative、mixed 和 inconclusive 必须生成 learning record。

### 8.3 不可变 measurement policy contract

每个 generation 必须冻结：

- policy_version
- early_signal_offset
- primary_checkpoint_offset
- follow_up_offsets
- max_follow_up_attempts
- max_measuring_duration
- minimum_sample 或 evidence requirements
- metric decision thresholds
- guardrails
- required data sources
- terminalization_grace_period
- absolute_terminal_at

所有 offset、duration 和 retry 次数必须有限。普通 policy upgrade、provider outage、checkpoint retry 或 scheduler delay 不能向后移动 absolute_terminal_at；需要继续观察时必须 terminalize 当前 generation，再以有审计的新 generation 开启新窗口。

### 8.4 checkpoint 与 outcome

primary checkpoint 数据完整时，按 threshold 和 guardrail 计算 positive、negative、mixed 或 inconclusive。primary checkpoint 为 insufficient_data 时，只能进入有限 follow-up；超过 max_follow_up_attempts 或 absolute_terminal_at 后必须 terminalize 为 insufficient_data。

## 9. Site Fix 与 Results handoff

### 9.1 Handoff 原则

Site Fix Verified 和 Results measurement 是两个独立 aggregate。Results 只展示已持久化的 measurement row；handoff 暂时失败不能伪造 Results 状态。

### 9.2 事务与 outbox

对 measurement_required，Approved 阶段在同一事务中创建 measurement generation 和 baseline plan。Site Fix 达到 Verified 时，在同一事务中写入唯一 handoff outbox event；事务提交后由 worker 激活 measurement。

worker 必须：

- 以 (project_id, site_fix_id, measurement_generation) 幂等 upsert。
- 失败时保留 retryable / terminal error、attempts、next_attempt_at。
- 不修改 Site Fix 的 Verified 状态。
- 由 reconciliation job 定期查找“已 Verified 但没有对应 measurement”的 gap。
- 只有 measurement row 真正存在后，Results 才显示该任务。

### 9.3 Handoff 状态

建议在 measurement aggregate 中使用：

    planned
    baseline_blocked
    ready
    observing
    terminal
    failed_retryable
    failed_terminal

Site Fix 页面可以显示“Results handoff pending”或“Results handoff failed”，但不能把 Site Fix 从 Verified 改回 measuring。

## 10. 确定性 classifier

### 10.1 新增字段

Site Fix 需要保存：

- fix_type：枚举化类型
- classifier_version：规则版本
- decision_origin：system_rule、user_override、imported_policy
- decision_confidence：high、medium、low
- impact_mode
- measurement_policy

### 10.2 fix_type 枚举

至少包括：title_readability、metadata_format、metadata_ctr_optimization、canonical_repair、robots_repair、sitemap_repair、redirect_or_http_repair、schema_validity_repair、schema_entity_optimization、internal_link_patch、internal_link_authority_optimization、geo_entity_clarity、geo_citation_optimization、content_typo_or_clarity、content_rewrite_for_search、content_demand_expansion、external_distribution、conversion_or_cta_optimization、technical_fix、unknown。

### 10.3 precedence 与 fallback

分类优先级固定为：

1. 用户显式 override，且必须通过 policy validation。
2. 结构化 proposed_fix 的 fix_type。
3. mutation fields、acceptance tests 和 target surface 的 deterministic rule。
4. classifier model 仅可提供候选，不可绕过规则校验。
5. 无法确定时 fix_type = unknown、impact_mode = unclassified、measurement_policy = verification_only、decision_confidence = low。

未知数据不得默认标注为 technical_reliability，也不得自动进入 required 或 optional。任何 required 决策都必须有完整 measurement readiness。

## 11. API 与前端行为

### 创建 Site Fix

1. 读取 finding、candidate 和 proposed fix。
2. 计算或读取 impact_mode。
3. 没有完整增长假设时，默认 measurement_policy = verification_only。
4. 只有策略校验通过后，才保存 measurement fields。
5. 不能仅因为 finding_kind = optimization 创建 measurement。

### Site Fix Verified

| policy | Verified 后行为 |
|---|---|
| verification_only | 更新为 verified，不写 Results attribution |
| measurement_required | Site Fix 更新为 verified；通过 handoff outbox 激活独立 measurement |
| measurement_optional | opted in 才创建 measurement，否则仅更新为 verified |

### Results 查询

Results 只返回已经持久化的 site_fix_measurements row。measurement_policy 只能决定是否应该创建 generation，不能让查询推导出不存在的 measurement。

verification_only 不得出现在 measurement queue、Results waiting count、Results exception count 或 impact attribution rows。

### Site Fix 详情

Site Fix 详情需要显示 Outcome type：

    Verification only

或者：

    Growth measurement required
    Primary metric: CTR
    Measurement window: 28 days

验证型完成态：

    Verified
    Implementation complete; no growth measurement is required for this fix.

增长型完成态：

    Verified
    Results measurement handoff pending.

当 measurement row 成功激活后，再显示：

    Verified
    Impact measurement started in Results.

并提供 View impact in Results 链接。

measurement_optional 显示 Track impact in Results。点击后校验 measurement readiness，创建新的 measurement generation，并跳转到对应 Results action；不修改 Site Fix 的 verified 状态。

### Handoff 失败

Results 只展示实际存在的 site_fix_measurements row。Approved 或 Verified 与 measurement worker 之间出现失败时，Site Fix 保持原有状态，页面显示 Results handoff pending 或 Results handoff failed；outbox worker 和 reconciliation job 负责重试和补偿。

## 12. 迁移与兼容

旧 Site Fix 默认迁移为 measurement_policy = verification_only，避免历史数据突然污染 Results。

历史数据统一保持 verification_only。命中 CTR、impressions、clicks、position、ranking、citations、brand mentions、referral sessions 或 conversion 等关键词的记录，只进入人工 review / optional candidate 队列；只有同时满足完整 policy snapshot、baseline/data-source readiness 和 target identity 时，才允许创建新的 prospective measurement generation。不得根据关键词回溯伪造 baseline 或自动生成 required measurement。

Site Fix 重试、PR 重开或重新部署不能重复创建 measurement。唯一键为 project_id、site_fix_id、measurement_generation；measurement_policy 不参与唯一键，因为它属于已冻结的 generation snapshot。

## 13. 验收标准

### 验证型 fix

- presentation_only fix 完成 Applied、Deployed、Verified。
- Results 不出现该 fix。
- Results waiting、completed、exceptions 计数不受影响。
- Site Fix 详情显示 Verification only。

### 增长型 fix

- Approved 时创建冻结的 measurement generation 和 baseline plan；Verified 后通过 outbox 激活 Results measurement。
- Results 显示正确的 source、metric 和 checkpoint。
- Site Fix 始终保持 verified；只有实际 measurement row 存在后才显示 View impact in Results。
- 重试、刷新和重复 webhook 不会产生重复 attribution。
- handoff worker 失败时，Site Fix 仍为 verified，Results 显示 handoff pending/failed，并可由 retry/reconciliation 恢复。

### 可选型 fix

- 未 opt in 前不出现在 Results。
- opt in 后创建 measurement。
- 取消或失败不会改变已经完成的 Site Fix Verified 结果。

### 回归保护

- Broken / Optimization badge 仍正常显示。
- Site Fix 的 PR、部署、验证闭环不受影响。
- Results 现有 content action attribution 不受影响。
- 老数据加载时没有空字段导致前端崩溃。

## 14. 推荐实施顺序

### Phase 1：策略和数据基础

1. 明确 site_fix_measurements、checkpoints、learnings 三类持久化对象。
2. 增加 fix_type、classifier_version、decision_origin、decision_confidence、impact_mode 和 measurement_policy。
3. 增加 immutable policy snapshot、baseline readiness 和 measurement readiness validation。
4. 默认旧数据和新 Site Fix 为 verification_only。
5. 增加 policy validation 和 API 字段。

### Phase 2：Site Fix UI

1. 显示 Outcome type。
2. 显示 Growth hypothesis、primary metric、policy version 和 measurement window。
3. 区分 Verified — implementation complete、handoff pending 和 measurement started。

### Phase 3：Results 集成

1. 在 Approved 阶段创建 measurement generation 和 baseline plan。
2. 增加 Verified handoff outbox、retry 和 reconciliation。
3. 只为实际存在的 site_fix_measurements row 提供 Results read model。
4. 支持 measurement_optional 的 prospective opt-in。
5. 添加幂等 upsert、checkpoint scheduler 和 source traceability。

### Phase 4：策略细化

1. 为常见 fix_type 建立版本化 deterministic rules。
2. 增加 growth hypothesis、baseline、data-source 和 policy 完整性检查。
3. 实现 positive、negative、mixed、inconclusive、insufficient_data outcome contract。
4. 补充 GEO、CTR、内链和内容类 measurement adapter。
5. 根据历史数据和人工 review 结果调整规则，不通过关键词自动升级。

## 15. 最终产品规则

> Site Fix 负责证明“改对了”；Results 负责证明“产生了增长”。只有存在明确增长假设、指标和测量窗口的 fix，才从 Site Fix 进入 Results。
