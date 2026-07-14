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

默认值为 verification_only，除非系统生成了完整、可信的增长假设和测量计划。

### 3.2 impact_mode

新增影响模式：

| impact_mode | 典型变更 | 默认策略 |
|---|---|---|
| presentation_only | 标题更易读、描述更清晰但不改变意图 | verification_only |
| technical_reliability | canonical、robots、sitemap、HTTP、redirect | verification_only |
| search_visibility | 关键词 title/meta、内链、搜索结构优化 | required 或 optional |
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
| security_or_config_repair | 权限、构建、配置或安全问题恢复 |

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

## 6. 生命周期

### 6.1 验证型 fix

    Finding → Approved → Applied → Deployed → Verified

Verified 是终点，展示：

    Verified — implementation complete

### 6.2 增长型 fix

    Finding → Approved → Applied → Deployed → Verified → Measuring → Learned | Inconclusive | Negative

- Verified：实现正确，开始测量。
- Measuring：measurement window 尚未结束。
- Learned：达到预设成功条件。
- Inconclusive：数据不足或无法归因。
- Negative：指标恶化或未达到 guardrail。

### 6.3 可选测量

measurement_optional 默认走验证型路径。用户点击 Track impact in Results 后才生成 measurement；这不会改变已完成的 Site Fix Verified 结果。

## 7. 数据模型建议

在 site_fixes 增加：

    impact_mode text not null default 'technical_reliability'
    measurement_policy text not null default 'verification_only'
    growth_hypothesis text
    primary_metric text
    secondary_metrics jsonb
    measurement_window jsonb
    measurement_opted_in boolean not null default false

建议约束：

1. measurement_required 必须有 primary_metric。
2. measurement_required 必须有合法 measurement_window。
3. verification_only 不创建 Results attribution row。
4. measurement_optional 只有 opted in 后才创建 Results attribution row。
5. measurement row 必须引用 site_fix_id。

Results attribution 应保留 source_type = site_fix、source_id = site_fix_id、impact_mode、growth_hypothesis、primary_metric 和 measurement_window。

## 8. 后端与 API 行为

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
| measurement_required | 更新为 verified/measuring，幂等创建 Results measurement |
| measurement_optional | opted in 才创建 measurement，否则仅更新为 verified |

### Results 查询

Results 只返回 measurement_required，或 measurement_optional 且 measurement_opted_in = true 的 Site Fix。

verification_only 不得出现在 measurement queue、Results waiting count、Results exception count 或 impact attribution rows。

## 9. 前端行为

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
    Impact measurement started in Results.

并提供 View impact in Results 链接。

measurement_optional 显示 Track impact in Results。点击后校验 measurement fields、创建 Results attribution、将状态改为 measuring，并跳转到对应 Results action。

## 10. 迁移与兼容

旧 Site Fix 默认迁移为 measurement_policy = verification_only，避免历史数据突然污染 Results。

只有旧 proposed_fix、acceptance data 或关联 action 中明确包含 CTR、impressions、clicks、position、ranking、citations、brand mentions、referral sessions 或 conversion 等信号时，才考虑升级为 measurement_required。无法确定时保持 verification_only，并允许用户手动 opt-in。

Site Fix 重试、PR 重开或重新部署不能重复创建 Results attribution。唯一键建议为 project_id、site_fix_id、measurement_policy。

## 11. 验收标准

### 验证型 fix

- presentation_only fix 完成 Applied、Deployed、Verified。
- Results 不出现该 fix。
- Results waiting、completed、exceptions 计数不受影响。
- Site Fix 详情显示 Verification only。

### 增长型 fix

- 带 growth_hypothesis、primary_metric 和 measurement_window 的 fix 在 Verified 后自动创建 Results measurement。
- Results 显示正确的 source、metric 和 checkpoint。
- Site Fix 显示 View impact in Results。
- 重试、刷新和重复 webhook 不会产生重复 attribution。

### 可选型 fix

- 未 opt in 前不出现在 Results。
- opt in 后创建 measurement。
- 取消或失败不会改变已经完成的 Site Fix Verified 结果。

### 回归保护

- Broken / Optimization badge 仍正常显示。
- Site Fix 的 PR、部署、验证闭环不受影响。
- Results 现有 content action attribution 不受影响。
- 老数据加载时没有空字段导致前端崩溃。

## 12. 推荐实施顺序

### Phase 1：策略和数据基础

1. 增加 measurement_policy。
2. 增加 impact_mode。
3. 默认旧数据和新 Site Fix 为 verification_only。
4. 增加 policy validation 和 API 字段。

### Phase 2：Site Fix UI

1. 显示 Outcome type。
2. 显示 Growth hypothesis、primary metric 和 measurement window。
3. 区分 Verified — implementation complete 和 Verified — measurement started。

### Phase 3：Results 集成

1. 只为 measurement_required 创建 Results attribution。
2. 支持 measurement_optional 的用户 opt-in。
3. Results 查询过滤 verification_only。
4. 添加幂等 upsert 和 source traceability。

### Phase 4：策略细化

1. 为常见 fix type 建立默认规则。
2. 增加 growth hypothesis 完整性检查。
3. 根据历史数据和用户行为调整默认 policy。
4. 补充 GEO、CTR、内链和内容类 measurement adapter。

## 13. 最终产品规则

> Site Fix 负责证明“改对了”；Results 负责证明“产生了增长”。只有存在明确增长假设、指标和测量窗口的 fix，才从 Site Fix 进入 Results。

