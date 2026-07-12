# CiteLoop Doctor / Opportunities 双线验收审计

- 审计日期：2026-07-12
- 验收对象：`PRD-CiteLoop-Doctor-Opportunities-Two-Line-Optimization.md` AC1–47
- 生产项目：`1459b054-cdc3-4d9b-9dd4-18e12458c61a`（unipost.dev）
- 生产基线：PR #311–#338，`main` 至 `2a5af290f77202003e600c1d21832f8bd31fde30`

## 结论

AC1–47 均已有可执行实现和自动化契约。能安全在生产触发的主路径已完成生产验证；需要真实 measurement window、directional outcome 或并发碰撞才能出现的数据状态，以代码、数据库约束和自动化测试验收，不伪造生产数据。

状态说明：

- `生产通过`：已在 citeloop.app、生产 API 或生产数据库验证。
- `契约通过`：实现、数据库约束与自动化测试通过；当前生产数据没有自然触发该分支。

## AC1–47 矩阵

| AC | 状态 | 验收证据 |
|---:|---|---|
| 1 | 生产通过 | Home 只呈现 Doctor / Opportunities 两个 control center；主导航无第三条 discovery 产品线。`visible-closed-loops-contract.test.mjs`。 |
| 2 | 生产通过 | Opportunities 仅显示 `Evidence matching`、`AI assistance` 等阶段；Signal Scan / AI Discovery 无独立 queue。`opportunities-loop-contract.test.mjs`。 |
| 3 | 生产通过 | Doctor 页面与 `TestDoctorReportsBrokenOptimizationHealthy` 覆盖 Broken、Optimization、Healthy。 |
| 4 | 生产通过 | Doctor 只创建 canonical Site Fix；PR #336 后 Opportunities 同时在数据和 UI 层排除 direct repair。 |
| 5 | 生产通过 | Opportunities 只展示 Content Plan → publish → measurement → learning；生产页面不再出现 Site Fix work。`TestOpportunityFindingExcludesImmediateRepairsAndKeepsDelayedGrowth`。 |
| 6 | 契约通过 | `TestArbitrationCreateCannotReassignCandidateOwner` 与 owner-neutral signature registry 保证单 owner。 |
| 7 | 契约通过 | `TestProjectEquivalentDoctorAndOpportunitySchemaWork` 将等价 schema work 合并到唯一 canonical owner。 |
| 8 | 契约通过 | immediate repair 过滤覆盖 `zero_internal_links`；统一 target/change signature 阻止第二条修复。 |
| 9 | 契约通过 | `TestProjectLowCTRQueryDoesNotDuplicateSameTitleMutation` 保留不同 success contract，并建立跨线 dependency。 |
| 10 | 契约通过 | thin evidence / AI citation 投影测试与 semantic arbitration 按 proposed mutation 合并等价 work。 |
| 11 | 契约通过 | `identity_test.go` 和 `semantic_test.go` 的 signature 不依赖 action wording、query 或 prompt 文案。 |
| 12 | 契约通过 | `reservation_test.go`、`discovery_reservation_contract_test.go` 在同一事务锁 bucket、reserve signature、检查 active registry。 |
| 13 | 生产通过 | Results 将 Applied / deploying 与 Verified 分开；Site Fix applied 不会直接标记 verified。 |
| 14 | 契约通过 | `sitefix_ai_verify_test.go` 重新抓取 evidence，并执行 typed acceptance tests，拒绝 substring false positive。 |
| 15 | 契约通过 | canonical Site Fix 状态机保留 evidence，支持 retryable/reopened，耗尽或终止后才 terminal。 |
| 16 | 生产通过 | 真实 Doctor finding 带 GSC/GA4 priority context，completion contract 为 `immediate_evidence_only`。`TestDoctorGSCAndGA4ContextPrioritizesWithoutChangingCompletionContract`。 |
| 17 | 生产通过 | PR #337 后 grounding 由批准证据确定性生成，再由独立 AI verifier 检查实际 patch；生产 generation 与 verifier 均成功。 |
| 18 | 契约通过 | `TestWithGrowthSpecificationProducesDecisionReadyWriterParams` 强制 hypothesis、baseline、primary metric、measurement window。 |
| 19 | 契约通过 | `content_action_trace_contract_test.go` 与 measurement queries 保留 source evidence / Opportunity linkage。 |
| 20 | 契约通过 | measurement evaluator 与 scheduler 生成 positive / negative / mixed / inconclusive / insufficient_data。 |
| 21 | 契约通过 | `TestRecordTerminalOutcomeSeparatesLearningFromMeasurementQuality`：四类 directional outcome 生成 learning，insufficient_data 只生成 quality record。 |
| 22 | 契约通过 | `learning_scoring_test.go` 与 `growth_learning_contract_test.go` 保存 scoring provenance；生产尚无自然 directional learning，不伪造样本。 |
| 23 | 生产通过 | Opportunities loop stage 可点击真实 action/content/result；生产 Measuring 与 Learned stage 可展开。 |
| 24 | 契约通过 | shared evidence 唯一键包含 source / target / window / collection-spec fingerprint，重复 refresh 复用 attempt。 |
| 25 | 生产通过 | standalone weekly GEO authority 已移除；Opportunity Finding 是唯一 durable scheduler authority。 |
| 26 | 契约通过 | provider unavailable 保留 deterministic evidence，不写虚假 zero findings。`TestDoctorDiagnosisProviderUnavailablePreservesDeterministicFindings`。 |
| 27 | 契约通过 | partial/missing crawl 标为 skipped/partial，不计 Healthy。`TestDoctorCoverageMarksMissingOrFailedGEOAuditSkipped`。 |
| 28 | 生产通过 | canonical `ai_call_records` 覆盖 diagnosis、arbitration、generation、verification 等 stage，并支持 aggregate 重算。 |
| 29 | 生产通过 | 迁移保留 legacy aliases、source snapshots、决策与 execution provenance；生产无静默删除。 |
| 30 | 契约通过 | migration dry-run 测试列出 active collision、planned collision、duplicate 与 ambiguous owner。 |
| 31 | 生产通过 | 生产所有 migration batch 满足 `source_count = migrated + archived_duplicate + review`，不守恒批次为 0。 |
| 32 | 契约通过 | deprecated Doctor convert route 复用 canonical creation；legacy alias 返回 canonical Site Fix linkage。 |
| 33 | 契约通过 | review-memory migration 保留 dismissed/snoozed/watching，并跨 signature alias/version 生效。 |
| 34 | 生产通过 | AI authority migration fail-closed；现有项目不会自动开启 Doctor AI。生产开关来自明确用户操作。 |
| 35 | 契约通过 | `site_fixes.doctor_finding_id` 是权威关系；不存在 finding current-fix pointer。 |
| 36 | 生产通过 | Settings 独立展示、保存、撤销 Doctor / Opportunities AI authority，并显示 partial state。 |
| 37 | 契约通过 | legacy technical action 先生成 provenance-complete migration finding，再创建 Site Fix。 |
| 38 | 契约通过 | collection-spec fingerprint 包含 user agent、dimensions、prompt/provider version，不错误合并。 |
| 39 | 契约通过 | semantic provider 在 Phase A 锁外执行；Site Fix preparation lease 不持有 DB transaction/row/advisory lock。 |
| 40 | 契约通过 | Phase B 校验 candidate/evidence/bucket versions；drift 时 rollback 并重新 prepare。`TestDoctorSiteFixCreationReloadsAndRecomparesAfterSnapshotStale`。 |
| 41 | 生产通过 | PR #338：所有 internal hold 均有 internal owner、age/SLA、admin API 与 resolution audit；migration review 使用 `migration_ops` + 7 天 SLA，仍不暴露给用户。 |
| 42 | 契约通过 | `max_measuring_duration` 有界，deadline 到期 terminalize。`growth_measurement_policy_contract_test.go`。 |
| 43 | 生产通过 | 每个 derived mutation 有 versioned ledger/inverse；生产 one-of-source violation=0，batch conservation violation=0。 |
| 44 | 契约通过 | Doctor citation optimization 强制 `added_propositions=[]`；新增事实 candidate fail-closed 到 Opportunities。 |
| 45 | 契约通过 | `TestDoctorVerificationStopsAtVerified` 移除 legacy verified → measuring transition。 |
| 46 | 契约通过 | 所有 arbitration snapshot writer 共享 bucket locks，并在事务内递增 bucket version；contract test 阻止 unversioned writer。 |
| 47 | 契约通过 | `absolute_terminal_at` 首次 Measuring 时持久化；retry/policy upgrade 不可延后，grace period 有限。 |

## 生产不变量

最终生产检查：

- Doctor findings：7；canonical Site Fixes：4。
- Opportunities：7；Growth Actions：6。
- active enforced signatures：5；active exact-signature duplicates：0。
- Doctor / Opportunities writer authority：均为 `canonical`，均未 fenced。
- application one-of-source violations：0。
- migration batch conservation violations：0。
- pending migration reviews：0 / 7；全部 owner=`migration_ops`，已审计 dismissed。
- retired legacy discovery config rows：0。
- Opportunities 页面旧 `Content Plan and Site Fixes` copy：0；Growth-only loop copy 已上线；console error：0。

真实 Doctor AI 修复 `d327f8c5-74ea-4215-a0b0-a2002a69c489`：

- 修复前 4 次 generation 因模型自报 grounding 不稳定而 `invalid_output`，共 22,084 tokens / $0.1657。
- PR #337 上线后 generation `ok`：5,485 tokens / $0.041365。
- 独立 grounding verification `ok`：3,930 tokens / $0.02547。
- application 成功进入 `manual_apply_required`；没有伪造 PR、部署或 verified 状态。

## 非代码阻塞与后续建议

以下不构成 AC 失败，但会影响真实闭环的自动化程度：

1. 当前 GA4 connection 显示 `Needs attention`，原因是生产账号/property access；已有历史 GA4 evidence 仍被 Doctor 使用。需在 Google 侧恢复 property 权限后重新 Sync。
2. 上述真实 Site Fix 只有 `page metadata / structured data / sitemap / routing` 四个泛化 surface，没有唯一 repository file path，因此安全降级为 manual apply。下一步可增加 publisher-aware URL → source-file resolver；在高置信唯一映射时自动 PR，否则继续 fail closed。
3. 当前生产没有 scoring-eligible directional learning；AC21/22 已由契约覆盖。需等待真实 Growth measurement window 产生 outcome 后再做一次生产 scoring provenance 复核。

## 关联修复

- PR #336：Opportunities 排除 Doctor Site Fix 工作与 copy。
- PR #337：Doctor grounding 改为 canonical evidence 派生，并保留独立 AI verifier。
- PR #338：migration review 增加 internal owner、SLA、age filters 和审计 API。
