# PRD：CiteLoop 生产 QA 问题修复计划

> 日期：2026-06-08
> 阶段：MVP 收口后的生产可用性修复
> 事实来源：2026-06-08 生产环境 E2E QA，测试项目 `staging.unipost.dev`
> 目标：把 CiteLoop 从“链路局部可跑”修到“真实用户可以自助接入、系统能解释状态、QA/SEO/GEO/发布闭环不中断”的状态。

## 1. 背景

本轮 QA 从真实生产环境出发，覆盖了以下路径：

1. 已登录用户访问 Home。
2. 输入 domain 创建项目。
3. 后台 Insight 抓取产品页面并生成 Knowledge。
4. 手动运行 Strategist 生成 topics。
5. 从 Topic 点击 Generate，触发 Writer 和 QA。
6. 查看 Review、Publishing、SEO、Runs、Settings。
7. 访问未登录 `/`、`/sign-in`、`/sign-up`。
8. 点击关键按钮：Run Strategist、Generate、Archive、Schedule、Reconcile、SEO Sync/Prompts/Analyze/Plan/Safe mode、Settings Save/Add 等。

QA 结论：`domain -> insight -> strategist -> writer` 基本能跑，但 `QA -> review -> SEO/GEO -> publish` 闭环仍然会断。最严重的问题不是 UI 细节，而是失败状态没有被产品化，用户无法知道系统卡在哪里，也无法把失败交给 AI 自动修复。

### 1.1 版本错位风险

这份 PRD 的初版基于生产环境 E2E。后续 review 对照当前代码发现，部分“404 / 缺失”类问题可能来自生产部署落后于当前分支，或来自更深层的 handler/runtime/migration 失败，而不是代码里完全没有 route 或完全没有持久化。

因此本 PRD 增加 P0-0：任何修复动工前，必须先确认 QA 所测部署的 git commit，并在当前目标 commit 上重跑端到端验收，把每个问题重新标记为：

- `still_reproduces`
- `already_fixed`
- `root_cause_changed`
- `cannot_reproduce`

没有完成 P0-0 前，不允许把生产 404 直接等同于“代码缺路由”，也不允许把 Review 空态直接等同于“Writer 没持久化 article”。

## 2. 已确认非问题 / 非目标

### 2.1 Clerk development mode

当前生产域名还未正式购买和上线，因此 Clerk 使用 development key 符合当前阶段预期。正式上线前需要切到 Clerk production instance，但不纳入本 PRD 的修复项。

本 PRD 仍保留未登录根路径 404 的修复，因为这不是“必须切 production key”问题，而是路由和登录入口体验问题。若根路径 404 的真实根因是 Clerk development instance 的 redirect / fallback URL 配置错误，本 PRD 只修 redirect 配置和路由策略，不要求当前阶段切到 Clerk production key。

### 2.2 本 PRD 不做的事

- 不重写完整 onboarding 产品。
- 不重新设计全部 billing/team/agency 权限。
- 不新增新的 CMS 大矩阵。
- 不解决所有移动端视觉细节，只要求本轮涉及页面不再出现明显拥挤、遮挡、不可读。
- 不把 QA 自动修复做成无限循环；必须有 attempt cap 和人工决策出口。

## 3. 第一性原理

CiteLoop 的产品价值不是“按钮能触发 agent”，而是：

1. 用户只输入 domain 后，系统能合法、清楚地拿到运营所需权限。
2. 系统必须持续展示自己正在做什么、做完了什么、哪里失败了。
3. AI 生成和 QA 必须形成闭环；失败先交给 AI 修复，只有 AI 无法决策时才打扰用户。
4. SEO/GEO 的建议必须来自可解释状态，而不是 404、空表和内部错误。
5. 发布前后必须有状态机和验证，不允许用户以为已经闭环，实际只是日志里失败。

因此本 PRD 的核心是“状态产品化 + 自动修复闭环 + 用户可理解的权限/发布设置”。

## 4. 目标

1. 新用户或已登录用户访问根路径时不会看到 404。
2. 项目创建后 Dashboard 能展示后台 onboarding job 的状态、进度、结果和失败入口。
3. Writer 成功但 QA 失败时，Review queue 必须出现可操作项，而不是空。
4. QA feedback 必须先进入 AI editor/reviser 自动修复循环，达到上限后才进入人工选择。
5. SEO/GEO 页面所有核心动作不再 404；缺数据时进入明确的 cold-start/degraded 状态。
6. 发布权限不再只表现为手填 GitHub token 的内部表单；普通用户看到 guided setup。
7. Runs 里的失败能深链到相关 topic/article/action。
8. Knowledge、Topics、Publishing、Settings 的状态文案能让用户知道下一步该做什么。

## 5. 成功指标

1. 使用 `staging.unipost.dev` 创建新项目后，5 分钟内可以看到：
   - crawl summary
   - active product profile
   - topic backlog
   - job history
2. 任意 writer/QA 失败都能在 Review 或 Runs 中找到可操作入口。
3. `SEO -> Prompts` 和 `SEO -> Analyze` 不再返回 404；没有 GSC/GA4 时显示 cold-start 或 insufficient data。
4. 任何禁用按钮都能说明禁用原因。
5. 任何 destructive action，例如 Archive 或 Safe mode，都有确认、Undo 或恢复入口。
6. 生产无登录访问 `/` 不再返回 404。

## 6. 修复优先级

| 优先级 | 修复项 | 用户影响 |
|---|---|---|
| P0 | 验证环境与 commit 对齐 | 避免为陈旧部署假象写错修复 |
| P0 | 未登录根路径 404 | 新用户入口断裂 |
| P0 | QA 失败状态语义化 | 内容闭环中断 |
| P0 | QA feedback -> AI editor -> QA rerun | 人工成本过高，且失败无法恢复 |
| P0 | SEO/GEO API root cause 修复 | SEO/GEO 闭环不可用 |
| P0 | Publisher setup gate | 无法安全发布 |
| P1 | Background job progress | 用户不知道系统是否卡住 |
| P1 | Runs deep link | 失败只能看日志，不能处理 |
| P1 | Knowledge crawl summary | 用户无法判断抓取质量 |
| P1 | Topics 操作安全性 | 容易误归档、排期含义不清 |
| P1 | Settings 用户化 | 内部字段和原始错误暴露 |
| P2 | Project archive/delete | 项目列表不可维护 |
| P2 | Responsive QA matrix | 窄屏/移动端风险未系统化覆盖 |

## 6.1 P0-0：验证环境与 commit 对齐

### 当前现状

生产 E2E 发现了若干 404 和空态问题。但当前代码中已经存在以下实现痕迹：

- GEO routes 已在 API server 注册，包括 overview、prompt set、observations、external surfaces、opportunities analyze 等。
- Writer 在 QA error 后仍会创建 `pending_review` article，并把 QA error 变成 blocking issue。
- Crawl summary 已写入 `generation_runs.output.crawl_summary`，Knowledge client 也有读取逻辑。

这说明部分现象可能是生产部署 commit 落后、数据库迁移未应用、API base URL 指向旧服务，或前端/后端部署不一致。

### 预期体验

每轮 QA 报告必须能绑定到一个明确版本：

- frontend deployment URL
- frontend git commit
- API deployment id
- API git commit 或 image digest
- DB migration version

PRD 和实现计划只修当前目标 commit 上仍可复现的问题。

### 改动步骤

1. 在前端显示或隐藏暴露 `/health/version`，返回 frontend commit、API commit、build time。
2. API `/health` 增加 commit/build metadata 和 DB migration version。
3. 在 QA 验收脚本开头记录版本信息。
4. 用当前目标 commit 部署 preview 或 production。
5. 重跑本 PRD §22 端到端验收。
6. 将每个问题标注为：
   - still_reproduces
   - already_fixed
   - root_cause_changed
   - cannot_reproduce
7. 对 `already_fixed` 的条目只保留 regression test，不再实现重复功能。

### 验收标准

1. QA 报告中能看到前端/API/DB 版本。
2. P0-2、P0-4、P1-3 三个争议项都完成重新分类。
3. 代码实现计划只包含当前 commit 上仍复现或根因已确认的问题。

## 7. P0-1：未登录根路径 404

### 当前现状

未登录访问 `https://citeloop.vercel.app/` 返回 404。`/sign-in` 和 `/sign-up` 可以返回页面，但根路径没有把用户引导到登录或公开入口。

### 预期体验

未登录用户访问 `/` 时看到以下之一：

1. 公开 landing/onboarding 入口。
2. 自动 redirect 到 `/sign-in`。
3. 显示带有 `Sign in` / `Create account` 的产品入口页。

不能显示 Next.js 默认 404。

### 改动步骤

1. 用 P0-0 的目标 commit 复现未登录 `/`。
2. 记录完整跳转链和 response headers，区分：
   - Next.js app route 缺失或 not-found。
   - Clerk `auth.protect()` redirect 链路失败。
   - middleware matcher 误保护或 rewrite 到 404。
   - sign-in fallback redirect 指回 `/` 后循环/404。
3. 明确根路径策略：
   - 当前阶段推荐：未登录 redirect 到 `/sign-in`。
   - 已登录继续显示 project console。
4. 按真实根因修复 middleware、Clerk redirect URL 或 root route。
5. 增加根路径未登录集成测试。
6. 增加 smoke test：`curl -I /` 未登录返回 302 或 200，不允许 404。

### 验收标准

1. 未登录 `GET /` 不返回 404。
2. 未登录浏览器访问 `/` 能进入登录或公开入口。
3. 已登录访问 `/` 仍显示项目列表。
4. `/sign-in` 和 `/sign-up` 仍可直接访问。

## 8. P0-2：QA 失败状态语义化与失败原因落库

### 当前现状

测试中 Topic 点击 Generate 后：

- `writer` run 成功。
- `qa` run 失败：`parse qa: unexpected EOF; compact fallback failed: missing claims`。
- Review queue 仍显示 `Nothing pending review`。
- 用户无法看到 writer 生成的草稿，也无法触发 AI fix、retry QA 或 reject/regenerate。

当前代码核实显示，Writer 已经会在 QA error 后创建 `pending_review` article，并把 QA error 转成 blocking issue。因此“Writer 成功后先持久化 draft”不是主要缺口。真正需要修的是：

- 当前目标 commit 上是否仍复现 Review 空态。
- QA parse failure 是否作为 article 的一等状态保存。
- QA failure reason 是否能稳定显示在 Review，而不只存在于 `generation_runs.error`。
- Review 是否能把 `qa_parse_failed` 与普通 `qa_blocking` 区分开。

### 预期体验

Writer 成功后，无论 QA pass 还是 QA fail，Review queue 都必须出现一个可操作 draft。如果当前 commit 已经能显示 draft，则本项的重点改为“失败原因和状态语义化”。

状态分为：

- `qa_passed`
- `qa_blocking`
- `qa_parse_failed`
- `ai_fixing`
- `needs_human_decision`
- `rejected`

QA parse failed 不是隐藏状态。它应该展示：

- 草稿 title
- topic
- QA failure reason
- 最新 writer output preview
- 可用动作：
  - `AI fix`
  - `Retry QA`
  - `Reject and regenerate`
  - `Open editor`

### 改动步骤

1. 用 P0-0 的目标 commit 重跑 QA parse failure fixture，确认 Review 空态是否仍复现。
2. 如果 draft 不可见，追踪 Review API 查询、article status、project ownership、topic grouping，而不是重复实现 Writer 持久化。
3. 如果 draft 可见但原因不可见，新增 article-level QA state：
   - `qa_status`
   - `qa_failure_kind`
   - `qa_failure_message`
   - `qa_failure_fingerprint`
   - `qa_attempt_count`
4. QA parse failure 时更新 article QA state，同时保留 generation run error。
5. Review query 必须返回 `qa_parse_failed`、`needs_human_decision` 和普通 `qa_blocking` 的区分字段。
6. Review card 增加 QA failure panel：
   - parse failure
   - evidence missing
   - SEO metadata missing
   - policy failure
7. Approve 按钮在 QA 未通过时 disabled，并展示原因。
8. Runs 中的 QA error 深链到对应 article detail。

### 验收标准

1. 构造 QA parse failed fixture 后，Review queue 出现 draft。
2. Review card 显示错误原因和 article preview。
3. Approve disabled，并解释为什么。
4. 用户可以点击 AI fix、Retry QA、Reject。
5. Runs 中 QA error 可点击进入该 draft。
6. QA parse error 在 article 记录和 run 记录中都可追踪。

## 9. P0-3：QA feedback -> AI editor -> QA rerun 闭环

### 当前现状

QA 失败后，需要用户自己理解错误并手动修正文稿。之前页面文案还要求用户“Use AI fix first”，但用户问得对：这不应该依赖用户主动点。QA 应该自己触发可控的修复循环。

### 预期体验

QA 失败后，系统自动进入修复状态：

1. QA 输出结构化 feedback。
2. AI editor/reviser 根据 feedback 修改 draft。
3. 系统 rerun QA。
4. 最多自动尝试 `N` 次。
5. 仍失败时进入人工决策，并给用户选择，不要求用户自己读全文修。

默认建议：

- `max_auto_fix_attempts = 3`
- parse failure 先走 normalize/repair prompt。
- evidence missing 走 claim pruning / evidence rewrite。
- SEO metadata missing 走 metadata completion。
- product ambiguity 进入 human decision。

### 系统契约

QA result schema 是 Review UI、AI editor/reviser、Runs deep link 和人工决策的共同契约，必须先冻结再实现 UI 或 agent。第一版 schema：

- `status`: `passed | blocking | parse_failed | needs_human_decision`
- `blocking_reasons[]`
- `claims[]`
- `evidence_map[]`
- `fix_instructions[]`
- `human_decisions[]`
- `failure_fingerprint`
- `confidence`

### 改动步骤

1. 冻结 QA result schema，并为 schema 增加 validation tests。
2. 定义 Editor input schema：
   - original draft
   - QA feedback
   - product profile
   - inventory evidence
   - SEO contribution target
3. 新增或复用 article revision table，记录每次 AI fix diff。
4. Pipeline 状态机增加：
   - `qa_failed`
   - `ai_fix_queued`
   - `ai_fix_running`
   - `qa_rerun_queued`
   - `needs_human_decision`
5. 避免死循环：
   - 同一 failure fingerprint 连续两次不改善，提前停止。
   - 超过 attempt cap 停止。
   - Editor 不能删除所有 claims 来逃避 QA；最低内容质量检查必须保留。
   - `max_auto_fix_attempts` 第一版固定为 3；后续如需配置，复用现有 policy/autopilot 配置层，不新建孤立设置。
6. Review UI 展示修复历史：
   - attempt count
   - 每次 QA feedback
   - AI 修改摘要
   - 当前阻塞点
7. 人工决策提供按钮：
   - accept safer wording
   - remove unsupported claim
   - choose target keyword
   - request full regeneration
   - reject topic

### 验收标准

1. QA missing claims fixture 会自动触发 AI fix。
2. AI fix 后自动 rerun QA。
3. 修复成功时 draft 进入 `qa_passed` 或 `qa_blocking=false`。
4. 连续失败不会无限循环；最多 3 次自动修复。
5. 最终需要人工时，Review 显示选择按钮，而不是要求用户自己改正文。
6. Runs 能看到 writer、qa、editor、qa rerun 的完整链路。

## 10. P0-4：SEO/GEO 404 根因确认与 cold-start 状态

### 当前现状

SEO 页面多处 404：

- 初始：`SEO data unavailable 404: 404 page not found`
- `Prompts`：`Could not generate GEO prompts 404`
- `Analyze`：`GEO analyzer failed 404`

`Sync` 返回 degraded，但页面没有解释清楚 degraded 的原因。没有 GSC/GA4 时，部分模块只是空表或 disabled。

当前代码核实显示，GEO API routes 已经注册，包括：

- `/geo/overview`
- `/geo/prompt-sets/generate`
- `/geo/prompt-sets`
- `/geo/runs/observe`
- `/geo/runs/observe-provider`
- `/geo/opportunities/analyze`
- `/geo/external-surfaces`
- `/geo/asset-briefs`

因此本项不能直接按“缺路由”处理。生产 404 的可能根因包括：

- 生产部署落后于当前分支。
- 前端部署与 API 部署 commit 不一致。
- API base URL 指到旧服务。
- route 存在但 DB migration/table 缺失，handler 以错误格式被前端呈现为 404。
- project id / auth / ownership guard 返回 404。

### 预期体验

SEO/GEO 页面必须有明确状态：

- `public_only`
- `cold_start_ready`
- `search_data_missing`
- `analytics_missing`
- `geo_prompt_ready`
- `insufficient_data`
- `connected`
- `degraded`

没有 GSC/GA4 时不应该 404。系统应使用 public crawl/profile 做 cold-start prompts 和 GEO baseline，无法做的指标标记 unavailable。

所有 cold-start 产物必须带来源标记，避免用户把推测当成 GSC/GA4 事实：

- `data_source = "cold_start"`
- `source_notes[]`
- `confidence`

UI 必须把 cold-start 与 connected GSC/GA4 数据视觉区分开。

### 改动步骤

1. 用 P0-0 确认生产 404 所属 commit。
2. 对 SEO page 所有前端 API call 做 route inventory。
3. 对后端 SEO/GEO routes 做 route inventory，并记录当前 commit 是否已注册。
4. 在目标 commit 上直接请求每个 API，分类：
   - route missing
   - auth/project guard 404
   - handler runtime error
   - migration/table missing
   - valid empty state
5. 按真实分类修复前后端 mismatch 或 runtime/migration 问题，保证：
   - summary
   - sync
   - prompt generation
   - provider observations
   - analyzer
   - plan
   - safe mode
   - objective
   都有有效 route。
6. SEO summary API 即使缺数据也返回 typed empty state，不返回 404。
7. GEO prompts 在有 active product profile 时可以 cold-start 生成，并写入 `data_source="cold_start"`。
8. Analyze 在没有 observations 时返回 `insufficient_data`，并提示先生成 prompts/provider observations。
9. SEO page 顶部增加 setup health：
   - public crawl: ready/missing
   - Search Console: missing/connected
   - GA4: missing/connected
   - publisher: missing/connected
10. Disabled buttons 加原因 tooltip 或 inline helper。

### 验收标准

1. 新项目 SEO page 初始加载不显示 404。
2. 点击 `Prompts` 不返回 404。
3. 点击 `Analyze` 在无 observations 时显示 `insufficient_data`，不显示 failed 404。
4. `Sync` degraded 时解释原因，例如 `Search Console not connected`。
5. 缺 GSC/GA4 时 CTR、position、conversion 不显示为事实数据。
6. SEO page 所有按钮都有成功、失败或 disabled reason。
7. cold-start prompt、baseline、opportunity 都带 `data_source="cold_start"` 并在 UI 中标注。

## 11. P0-5：Publisher setup gate 与真实发布前置条件

### 当前现状

Settings 页面暴露 GitHub/Next.js 配置，用户需要理解：

- repo
- branch
- content path
- base URL
- publish mode
- GitHub token

这不符合“真实用户只输入 domain，通过 guided setup 给权限”的方向。Publishing 页能 Reconcile，但没有发布权限时只显示空态。

### 预期体验

普通用户看到的是发布权限 checklist，而不是内部 GitHub 表单。

Publisher setup 应解释：

- 需要连接什么发布目标。
- CiteLoop 会写哪些路径。
- 当前权限是否足够创建文章、更新 metadata、触发 deploy、验证 URL。
- 缺权限时内容只能生成 draft，不能自动发布。

内部用户仍可有 advanced form，但需要折叠在 Advanced。

### 改动步骤

1. 把 publisher connection health 抽成统一对象：
   - `provider`
   - `mode`
   - `capabilities`
   - `credential_status`
   - `last_test_at`
   - `last_error`
2. Dashboard/SEO/Publishing 读取同一个 publisher health。
3. Settings 将 GitHub raw form 移入 Advanced section。
4. 普通 setup card 展示：
   - Connect GitHub/Next.js
   - Test connection
   - Verify base URL
   - Trigger deploy hook test
5. 没有 publisher 时：
   - Review 可以 approve draft，但显示 `Publishing blocked: publisher missing`。
   - Publishing 显示 setup CTA。
   - Publish tick disabled，并说明原因。
6. raw token 不能回显给前端。
7. 与 Runs detail 共用 redaction 规则，任何 publisher credential、deploy hook、webhook URL、API key 都不能出现在 API response、run output、client logs 或 notification body 中。

### 验收标准

1. 新项目 Publishing 页明确显示 publisher missing 和下一步。
2. Publish tick 在 publisher missing 时 disabled，并有原因。
3. Settings 不默认展示 raw token 为主要路径。
4. Save publisher 缺字段时显示用户化错误，不显示 raw JSON。
5. Test connection 失败时说明具体缺哪个权限。
6. API response 和 Runs detail 中不出现 raw token、deploy hook URL 或 webhook URL。

## 12. P1-1：后台任务进度与 Dashboard 状态

### 当前现状

项目创建后后台 Insight 会跑，但 Dashboard 初始仍是空 pipeline。Run Strategist 和 Generate 运行时只是禁用按钮，没有 job name、进度、预计耗时或 Runs 链接。

### 预期体验

每个后台动作都有可见状态：

- queued
- running
- succeeded
- degraded
- failed
- waiting_for_permission

Dashboard 顶部显示当前 job：

- 正在抓取站点
- 正在生成 profile
- 正在生成 topics
- 正在写 draft
- 正在 QA
- 正在发布

### 改动步骤

1. 定义 project activity API，聚合 recent runs + active jobs。
2. 创建项目时写 onboarding jobs：
   - public crawl
   - product profile
   - SEO cold-start sync
3. 前端增加 job status banner。
4. 所有手动按钮触发后显示 toast + status row + link to Runs。
5. 长任务按钮禁用时仅禁用相关动作，不要全页禁用无关按钮。
6. job 完成后刷新相关模块。

### 验收标准

1. 创建项目后 Dashboard 显示 onboarding running。
2. Insight 完成后显示 inventory count 和 crawl summary link。
3. Run Strategist 运行时显示 `Strategist running`。
4. Generate 运行时显示 `Writer running` 和 `QA queued/running`。
5. 失败时显示具体失败和处理入口。

## 13. P1-2：Runs deep link 与失败处理入口

### 当前现状

Runs 能显示 agent/status/cost/error，但错误没有深链到相关 topic/article/action。QA error 只能看文本。

### 预期体验

每条 run 都可以回答：

- 谁触发的？
- 针对哪个 project/topic/article/action？
- 输入是什么？
- 输出是什么？
- 失败原因是什么？
- 下一步可以做什么？

### 改动步骤

1. 为 generation runs 增加关联字段或 metadata：
   - `topic_id`
   - `article_id`
   - `seo_action_id`
   - `publisher_attempt_id`
   - `geo_run_id`
2. Runs list 增加详情入口。
3. Run detail 展示 input/output/error，敏感字段脱敏。
4. Run detail 根据 agent/status 提供动作：
   - Retry
   - Open draft
   - Run AI fix
   - Open settings
   - Open publishing
5. Dashboard/Review/SEO/Publishing 错误卡片都链接到 run detail。
6. 定义全局 redaction 规则，至少覆盖以下字段名和 URL 类型：
   - `token`
   - `api_key`
   - `authorization`
   - `secret`
   - `webhook_url`
   - `deploy_hook_url`
   - `github_token`
   - `slack_webhook`
   - `discord_webhook`
   - 任何包含 query secret 的 URL

### 验收标准

1. QA error run 可点击进入相关 draft。
2. Writer run 可看到生成的 article ID。
3. SEO failed run 可链接到 SEO page 对应模块。
4. 敏感 token/webhook 不出现在 run detail。
5. run input/output/error 中的敏感字段会被稳定替换为 `[redacted]`。

## 14. P1-3：Knowledge crawl summary 与数据一致性

### 当前现状

Insight 已完成并生成 inventory，但 Knowledge 页面仍显示 `No crawl summary`。用户看不到抓取范围、失败页面、跳过原因和是否截断。

当前代码核实显示，crawl summary 已写入 `generation_runs.output.crawl_summary`，Knowledge client 也有 `latestCrawlSummary(runs)` 读取逻辑。因此本项不应默认新增第二套持久化。优先修复方向是：

- 当前部署是否落后。
- Runs API 是否返回了 insight run output。
- 前端是否正确 parse `crawl_summary`。
- 页面是否在 inventory 已加载但 summary 尚未加载时误显示 `No crawl summary`。

### 预期体验

Knowledge 页展示 crawl summary：

- fetched pages
- discovered URLs
- skipped URLs
- failed URLs
- robots/sitemap status
- truncated
- crawl depth
- last run time

### 改动步骤

1. 用 P0-0 的目标 commit 确认问题是否仍复现。
2. 验证 Insight run output 中是否存在 `crawl_summary`。
3. 验证 Runs API 是否返回 run output，且未被 response normalization 丢弃。
4. 修复 Knowledge client 的 summary parse / loading / empty state。
5. 如果 summary 缺失但 inventory 存在，显示 `summary unavailable` 和 last run link，而不是 `No crawl summary`。
6. 只有在 run output 不能满足查询和历史回看时，才把 summary 冗余写入 active profile metadata 或独立表。
7. Inventory count 与 run log count 需要解释差异，例如 fetched、stored、filtered、generated、excluded。

### 验收标准

1. 新项目 Insight 完成后 Knowledge 显示 crawl summary。
2. 抓取失败 URL 可见。
3. truncated=true 时 UI 明确提示。
4. Inventory 数量与 summary 数量关系可解释。
5. 如果 summary 缺失，UI 显示 `summary unavailable` 和 Runs 链接，不误导为从未抓取。

## 15. P1-4：Topics 操作安全与编辑体验

### 当前现状

Topics 支持 Edit、Generate、Archive、Schedule，但存在体验问题：

- Archive 无确认、无 Undo/Restore。
- Archived topic 在 All 列表仍显示，按钮禁用，但缺少恢复入口。
- Schedule 日期为空时显示 `Schedule cleared`，但用户不一定理解发生了什么。
- Generate 长任务没有明确 writer/QA 状态。

### 预期体验

Topic 操作应是可恢复、可解释的。

### 改动步骤

1. Archive 增加确认或 snackbar Undo。
2. 增加 status filter：
   - active/backlog
   - scheduled
   - archived
   - generated
3. Archived topic 显示 Restore。
4. Schedule 输入为空时按钮文案改成 `Clear schedule`，或点击 Schedule 时要求日期。
5. Generate 后 topic card 显示 generation status。
6. Generate 使用 `topic_id + kind + platform` 作为幂等键；如果 topic 已有 non-rejected draft，再次 Generate 返回已有 draft link 或要求用户确认 regenerate。

### 验收标准

1. Archive 后 10 秒内可 Undo。
2. Archived filter 能看到归档项并 Restore。
3. 空日期不会以 `Schedule` 按钮触发模糊的 clear 行为。
4. Generate 后 topic 显示 writer/QA 状态。
5. 重复 Generate 返回已有 draft link 或 regenerate confirmation，不产生裸 DB 错误。

## 16. P1-5：Settings 用户化与错误文案

### 当前现状

Settings 暴露内部实现文案：

- `PUT /config replaces the entire config`
- `full-payload`
- `400: {"error":"repo and base_url are required"}`

通知和 publisher 配置更像开发者表单。

### 预期体验

Settings 分为：

- General
- Publishing
- Notifications
- Crawl policy
- Advanced

默认只展示用户需要理解的配置；内部 API 细节进入 Advanced 或完全隐藏。

### 改动步骤

1. 删除或隐藏 full-payload 技术说明。
2. 将 backend error 映射为用户文案：
   - repo missing
   - base URL missing
   - invalid webhook
   - token missing
   - permission denied
3. Publisher advanced fields 折叠。
4. Notification webhook 添加格式校验和测试按钮。
5. 保存成功说明哪些配置变了，而不是只说 Settings saved。
6. 对危险配置增加确认，例如 monthly budget 降低、kill switch、safe mode。

### 验收标准

1. 普通用户看不到 raw API route 说明。
2. Save publisher 缺字段时显示可行动文案。
3. Add webhook 缺 URL 时提示应填 Slack/Discord webhook URL。
4. Settings 保存成功后显示 summary。

## 17. P1-6：Publishing 页面空态和 Reconcile 解释

### 当前现状

Publishing 点击 Reconcile 后显示 `Publishing reconciled`，即使没有 publisher、没有 approved article、没有 canonical，也没有说明检查了什么。

### 预期体验

Publishing 页面应像发布控制台：

- publisher health
- approved canonical due count
- pending URL verification
- publish failures
- variants waiting canonical
- next scheduled publish

Reconcile 后展示检查结果。

### 改动步骤

1. Publishing summary API 返回 lane counts 和 blockers。
2. Reconcile 返回 structured result：
   - checked articles
   - publishable count
   - skipped reasons
   - repaired state count
3. UI 显示 reconcile result panel。
4. publisher missing 时显示 setup CTA。
5. no approved articles 时链接到 Review/Topics。

### 验收标准

1. 无内容时 Reconcile 不只显示成功，而是说明无可处理项。
2. publisher missing 时有明确 blocker。
3. QA failed draft 不会被误认为无内容。
4. Variants waiting canonical 显示等待原因。

## 18. P1-7：SEO Safe mode 与 Autopilot 安全 UX

### 当前现状

SEO 页 `Safe mode` 可以一键开启，没有确认，也没有明显退出入口。Autopilot Level、Objective、Plan 等按钮缺少上下文。

### 预期体验

Safe mode 是高影响开关，必须可解释、可恢复。

### 改动步骤

1. 点击 Safe mode 先弹确认，说明会暂停哪些动作。
2. 开启后显示 persistent banner。
3. 提供 `Exit safe mode`。
4. 记录 safe mode run/event。
5. Plan 生成 0 actions 时解释原因。
6. Objective disabled 时说明缺什么输入。

### 验收标准

1. Safe mode 开启前必须确认。
2. 开启后页面顶部持续显示状态。
3. 可以退出 Safe mode。
4. Runs 记录 safe mode event。

## 19. P2-1：Project archive/delete

### 当前现状

Home 项目列表只能进入项目，不能 archive/delete。测试项目会堆积，真实用户也无法清理错误项目。

### 预期体验

用户可以 archive project。内部管理员或 owner 可以 delete 无数据项目。

### 改动步骤

1. 增加 project status：
   - active
   - archived
2. Home 默认只显示 active。
3. Project menu 增加 Archive。
4. Archived projects 可恢复。
5. Delete 只允许无 published content 或内部管理员执行，并要求二次确认。

### 验收标准

1. Home 可 archive 项目。
2. Archived 项目默认隐藏。
3. Archived 项目可恢复。
4. Delete 不会误删已有发布历史。

## 20. P2-2：Responsive QA matrix

### 当前现状

本轮 QA 未完成严格移动端矩阵。此前 Review 页面已经出现过宽度不足、元素拥挤和 preview 显示不完整问题。

### 预期体验

核心页面在桌面、平板、移动宽度都可用：

- Home
- Dashboard
- Knowledge
- Topics
- Review
- Publishing
- SEO
- Runs
- Settings

### 改动步骤

1. 增加 Playwright viewport matrix：
   - 1440x900
   - 1024x768
   - 768x1024
   - 390x844
2. 对每页做 smoke screenshot。
3. 自动检查：
   - horizontal overflow
   - overlapping buttons
   - clipped primary content
   - disabled controls without explanation
4. Review 页面保留左原文、右 preview 的宽屏布局；窄屏改为 tabs 或 stacked layout。
5. Preview 区域支持完整文章滚动，不只显示顶部片段。

### 验收标准

1. 四个 viewport 下核心页面无横向溢出。
2. Review preview 可滚完整文章。
3. 主按钮不重叠。
4. 所有页面有可读空态。

## 21. 推荐执行顺序

### Phase A：闭环不再断

1. 完成 P0-0 验证环境与 commit 对齐，重新分类 still_reproduces / already_fixed / root_cause_changed。
2. 修复未登录根路径 404。
3. QA failed 状态语义化和失败原因落库。
4. QA auto-fix loop。
5. SEO/GEO 404 根因修复和 cold-start 状态。
6. Publisher missing gate。

Phase A 验收：从 domain 创建项目，生成一个 draft，即使 QA 失败也能在 Review 中处理；SEO 页面不 404；Publishing 能说明为什么不能发布。

### Phase B：状态可理解

1. Dashboard job progress。
2. Runs deep link。
3. Knowledge crawl summary。
4. Publishing reconcile result。
5. Settings error mapping。

Phase B 验收：用户不看 server logs 也能知道系统做了什么、失败在哪、下一步是什么。

### Phase C：操作可恢复

1. Topics archive/restore/undo。
2. Safe mode confirm/exit。
3. Project archive。
4. Responsive QA matrix。

Phase C 验收：常见误操作可恢复，窄屏可用。

## 22. 端到端验收脚本

每次修复完成后，用生产或 preview 环境执行以下验收：

0. 记录版本。
   - 预期：QA 报告包含 frontend deployment URL、frontend commit、API deployment id、API commit/image digest、DB migration version。
1. 未登录访问 `/`。
   - 预期：非 404。
2. 登录后创建 `staging.unipost.dev` 或等价测试 domain 项目。
   - 预期：Dashboard 出现 onboarding progress。
3. 等待 Insight 完成。
   - 预期：Knowledge 有 profile、inventory、crawl summary。
4. 点击 Run Strategist。
   - 预期：Dashboard/Runs 显示 strategist running，完成后 Topics >= 1。
5. 对一个 topic 点击 Generate。
   - 预期：Writer run 成功后 draft 出现在 Review。
6. 使用 QA parse failure fixture。
   - 预期：Review 出现 failed draft；article 上有 `qa_parse_failed` 状态和 failure message；自动 AI fix attempts 开始。
7. AI fix 成功 fixture。
   - 预期：QA rerun pass，Approve 可用。
8. AI fix 失败 fixture。
   - 预期：最多 3 次后进入 human decision，提供选择按钮。
9. 打开 SEO。
   - 预期：无 404；缺权限显示 cold-start/degraded。
10. 点击 Prompts。
    - 预期：生成 prompt set 或显示 insufficient prerequisites，不 404；cold-start 结果标记 `data_source="cold_start"`。
11. 点击 Analyze。
    - 预期：无 observations 时显示 insufficient_data，不 404。
12. 打开 Publishing。
    - 预期：publisher missing / no approved canonical / pending publish 状态清楚。
13. 点击 Reconcile。
    - 预期：显示 checked/skipped/blocker summary。
14. 打开 Runs。
    - 预期：每个 failed run 能 deep link 到处理页面。
15. 检查 run detail redaction。
    - 预期：token、webhook URL、deploy hook URL、API key 都被 `[redacted]` 替换。
16. 桌面和移动 viewport 截图检查。
    - 预期：无按钮重叠、无主内容截断。

## 23. 风险与约束

1. QA auto-fix 可能把文章越修越差，因此必须有 attempt cap、failure fingerprint 和最低内容质量检查。
2. SEO/GEO cold-start 不能伪装成真实 GSC/GA4 数据。
3. Publisher setup 不能把 raw token 暴露给前端或 logs。
4. Review queue 不能因为 QA parse failed 就允许 approve。
5. Safe mode 必须优先保护发布和自动化任务，不能只改 UI 状态。
6. 版本错位会制造“已修复问题仍被当成 P0”的假象，因此 P0-0 必须先执行。

## 24. Open Questions

1. QA auto-fix 的默认 attempt cap 是否固定为 3，还是项目级可配？
2. Publisher guided setup 的第一版是否只做 GitHub/Next.js，还是同时加入 Vercel deploy hook 测试？
3. Project delete 是否只给内部管理员，普通用户只给 archive？
4. SEO/GEO provider observations 第一版是否允许 mock/manual fixtures，还是必须接真实 provider？

默认建议：

1. attempt cap 第一版固定 3；如果后续需要配置，复用现有 policy/autopilot 配置层。
2. 第一版 publisher setup 只做 GitHub/Next.js + deploy hook health。
3. 普通用户只 archive，管理员可 delete。
4. SEO/GEO 先允许 fixture/cold-start，真实 provider 作为后续增强；fixture/cold-start 必须带 `data_source` 和 `confidence`。
