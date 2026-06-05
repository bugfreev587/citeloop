# PRD：CiteLoop MVP 收口版

> 目标：把 CiteLoop MVP 从“核心链路能跑”收成“内部单人可运营、可审计、可告警、可自动发布 UniPost blog”的版本。
> 日期：2026-06-05
> 事实来源：`docs/PRD-CiteLoop-MVP-v2.md`、`docs/PRD-CiteLoop-Frontend-Dashboard.md`、当前代码实现、UniPost notification/webhook 既有实现。

## 1. 第一性原理

CiteLoop 的价值不是“生成文章”，而是把一个产品的可验证事实变成可发布、可分发、可追踪失败的内容运营流水线。

因此 MVP 收口必须回答四个最基础的问题：

1. **事实安全吗？** 任何产品事实声明都必须能映射到 profile、source URL 或 inventory evidence；不能映射就 blocking。
2. **发布闭环吗？** app 内 approve 是唯一人工闸门；canonical 到点后应自动进入 UniPost blog 内容源。
3. **分发安全吗？** syndication variants 必须等 canonical 真实 URL 存在后才能解锁。
4. **失败可见吗？** 生成失败、发布失败、预算熔断、审核超时、通知投递失败，都不能只藏在日志里。

本 PRD 不重写已有 MVP。它只定义“最后一公里”的产品契约、数据契约、UI 契约、UniPost 侧改造、告警系统和上线验收。

## 2. 已确定决策

- 使用者：内部单人运营，不做外部 beta。
- Blog canonical 自动发布：纳入本轮收口。
- UniPost 内容源：UniPost 主 Next.js repo。
- 内容写入方式：CiteLoop 写 UniPost 主 repo 的 `citeloop-content` branch。
- 内容目录：`content/citeloop/blog/`。
- UniPost 展示路径：生成文章并入正常 blog 路径 `/blog/{slug}`。
- UniPost 侧改造：纳入本轮范围。
  - 从 UniPost `dev` 开分支。
  - 完成后推到 `origin/dev`。
  - 部署后在 `https://dev.unipost.dev` 验证。
- 通知/告警：参考 UniPost 完整 notification 模型，而不是单 env webhook。
  - channels
  - subscriptions
  - deliveries
  - dispatcher
  - worker
  - retry / dead state
- MVP 通知事件只做运营关键失败：
  - `generation.failed`
  - `publish.failed`
  - `budget.stopped`
  - `review.overdue`
  - `webhook.delivery.dead`
- 基础设置/凭证：混合模式。
  - 发布链路核心凭证继续用 env/process config。
  - notification channels/subscriptions 进数据库并提供 UI。
  - LLM/Search 保持当前 env/admin 混合方向；已有 admin credentials 能复用则复用，本 PRD 不重新设计完整 secret manager。

## 3. 目标

1. 内部运营者能看清系统做了什么、失败了什么、花了多少钱、哪里降级了、下一步该处理什么。
2. 选题在生成前可编辑、可排期、可归档，重复生成不会暴露裸 DB 错误。
3. Knowledge 页能展示 crawl summary 和 evidence snippets，方便在生成前修正系统认知。
4. Review 页继续作为唯一人工闸门，并支持正文与 SEO metadata 的完整编辑。
5. Approved canonical 到点后自动写入 UniPost `citeloop-content` branch。
6. UniPost dev 能读取 `content/citeloop/blog/` 并在 `/blog/{slug}` 展示生成文章；若 UniPost 使用 build-time loader，CiteLoop 必须触发或等待 UniPost dev rebuild。
7. CiteLoop 只在 UniPost URL 真实返回 `2xx` 后回填 `canonical_url` 和 `canonical_url_verified_at`。
8. Variants 只在 canonical `published` 且 verified real URL 存在后进入 `ready_to_distribute`。
9. 关键运营失败通过 Slack/Discord webhook notification channels 投递，有 retry/dead 状态。
10. 收口完成后有一套跨 CiteLoop + UniPost 的上线验收清单。

## 4. 非目标

- 不做外部用户注册、计费、团队权限或 SaaS onboarding。
- 不做第三方平台自动发布；Dev.to / Hashnode / Reddit 等仍然半自动分发。
- 不做 UniPost API 社交自动发布。
- 不做排名、AI 引用、Share of Voice、内容表现等 analytics 反馈闭环。
- 不做任意网站高鲁棒爬虫；继续使用当前受控 crawler。
- 不做图片生成、A/B 测试、多语言。
- 不做完整 secret-management 产品。

## 5. 当前缺口

| 模块 | 当前缺口 | 收口要求 |
|---|---|---|
| Runs | `generation_runs` 只有写入和统计，没有列表/详情 API。 | 增加 run list/detail、筛选、monthly spend、degraded、failure 展示。 |
| Topics | 缺编辑、排期、归档 API；重复 generate 可能返回裸 DB 错误。 | 增加 topic patch/archive/schedule；重复生成返回已有结果或友好 `409`。 |
| Article detail | 有 `GetArticle` query，无 HTTP detail route。 | 增加 article detail route，支持 review/publishing/runs 深链。 |
| Crawl summary | crawler 有 discovered/truncated/errors，但 `/insight` 不返回也不持久化 summary。 | 返回或写入 crawl summary，Knowledge 页展示。 |
| Evidence | Inventory update 不支持 `evidence_snippets`。 | evidence snippets 可见、可编辑。 |
| Review editor | UI 主要编辑正文，SEO metadata 编辑不完整。 | 支持 SEO metadata 编辑，并明确 metadata-only 不会清 QA blocking。 |
| Publisher | CiteLoop 有 GitHub writer，但 UniPost 尚未读取专用 content branch。 | 完成 CiteLoop branch writer + UniPost branch reader。 |
| Publish failure | 发布失败主要在日志里。 | publisher run 落库，Publishing/Runs 可见，可 retry。 |
| Notifications | scheduler alert 只是 `slog.Warn`。 | 增加 notification tables、dispatcher、worker、Slack/Discord sender。 |
| Launch readiness | 没有跨两个 repo 的最终验收。 | 定义 CiteLoop + UniPost dev 端到端验收。 |

## 6. 端到端流程

### 6.1 Knowledge setup

1. 运营者输入 landing page URL。
2. Insight 按 crawl config 抓取。
3. CiteLoop 保存：
   - active product profile version
   - content inventory
   - evidence snippets
   - crawl summary
   - insight run records
4. Knowledge 页展示 profile、inventory、crawl summary、evidence snippets。
5. 运营者能在生成前修正 profile 和 evidence。

### 6.2 Topic preparation

1. 运营者运行 Strategist。
2. Strategist 写入 search snapshots。
3. 搜索失败时允许降级成 pure LLM topic generation，但必须记录 `degraded=true`。
4. Topic backlog 支持：
   - edit
   - schedule
   - archive
   - filters
   - generate selected topic
5. 重复 generate 同一 topic 时：
   - 返回已有 non-rejected articles，或
   - 返回友好 `409` + article IDs，
   - 绝不暴露 raw unique-index `500`。
6. MVP 每个 topic 最多只有一个 active canonical。Variant 与 canonical 的 sibling 关系由 `topic_id` + 唯一 canonical 确定；不引入多 canonical A/B 分支。

### 6.3 Review gate

1. Writer 为 topic 生成 canonical 和 variants。
2. QA 对每篇文章执行 evidence mapping。
3. `qa_blocking=true` 不能 approve。
4. 运营者能编辑正文并触发 re-QA。
5. 运营者能编辑 SEO metadata；metadata-only edit 不触发 re-QA，也不能清 blocking。
6. Approve canonical 时写入 `articles.scheduled_at`。
7. Approve variants 后继续等待 canonical publish 和 URL backfill。
8. 验收必须包含一个无证据事实声明 fixture：QA 必须把它标记为 `qa_blocking=true`，并且 approve API 必须返回阻塞错误。

### 6.4 Canonical publish

1. Publish tick 选择 due 且 approved 的 canonical，并拿到 project-level publisher lease/lock。
2. 同一个 project 任意时刻只允许一个 publish tick 写 UniPost content branch；lock 只保护领取任务和短 DB 写入，不允许长时间包住 GitHub/Vercel/URL 网络调用。
3. 第一次 publish attempt 分阶段完成：
   - 读取 GitHub branch 以检测 slug/path 冲突。
   - 解析 `resolved_slug` 和 `publish_path`。
   - 在短 DB transaction 中持久化 `resolved_slug`、`publish_path`、`publish_attempts` 和当前 publish phase。
   - 后续 retry 必须复用同一路径。
4. BlogPublisher 渲染 MDX 文件。
5. BlogPublisher 只写 UniPost repo：
   - branch：`citeloop-content`
   - directory：`content/citeloop/blog/`
6. GitHub commit 成功后，CiteLoop 立即持久化 `publish_result.commit_sha`、`publish_result.path`、`publish_result.url`、`publish_result.phase='pending_url_verification'`，并把 canonical 标记为 `pending_url_verification`，不能重复创建 content file。
7. 如果 UniPost loader 是 build-time fetch，CiteLoop 必须调用 `UNIPOST_DEPLOY_HOOK_URL` 或等待已知 dev deployment；没有 deploy hook 时只能进入 `pending_url_verification` 并等待人工/外部部署，不能立即标记 `publish_failed`。
8. URL verifier 按 backoff 轮询 `BLOG_BASE_URL/{resolved_slug}`：
   - 优先 `HEAD`
   - 如 UniPost/Vercel 对 `HEAD` 不稳定，则 fallback `GET`
   - 只有 `2xx` 才视为真实 URL 存在
9. URL 验证成功后，CiteLoop 在一个短 DB transaction 内：
   - 标记 article `published`
   - 写 `published_at`
   - 回填 `canonical_url`
   - 写 `canonical_url_verified_at`
   - 将文章加入 inventory，`source=generated`
   - 解锁符合条件的 variants
10. 失败后 CiteLoop：
   - 不标记 `published`
   - 标记 article `publish_failed`
   - 写 publisher run
   - Publishing/Runs 可见
   - 发 `publish.failed`
   - 按 publish retry/backoff 计划重试；超过自动重试上限后等待人工 retry

### 6.5 UniPost rendering

1. UniPost dev 分支新增 CiteLoop content loader。
2. Loader 从 `citeloop-content` branch 读取 `content/citeloop/blog/`。
3. Loader 将生成内容合并到现有 blog index 和 detail route。
4. 已有静态 UniPost blog 不能被破坏。
5. 若 loader 使用 build-time GitHub fetch，UniPost dev 必须提供 Vercel Deploy Hook 或等价 rebuild 触发方式，供 CiteLoop 在 commit 后触发。
6. 部署后用 `https://dev.unipost.dev/blog/{slug}` 验证生成文章。

### 6.6 Syndication unlock

1. Variants 在 canonical verified publish 前保持 `approved`。
2. 只有 sibling canonical 满足以下条件才 unlock：
   - `status=published`
   - `canonical_url` 非空
   - `canonical_url_verified_at` 非空
3. Unlock 时替换正文和 metadata 中的 `{{CANONICAL_URL}}`。
4. 支持 canonical tag 的平台写 `seo_meta.canonical_url`。
5. 不支持 canonical tag 的平台只在正文写 source link。
6. Publishing 页展示 copy、compose、mark distributed。

### 6.7 Notifications

1. 关键事件 publish 到 notification dispatcher。
2. Dispatcher 为每个 matching subscription 创建 delivery row。
3. Worker 轮询 pending deliveries。
4. Worker 向 Slack/Discord webhook 发送消息。
5. 失败后按 retry schedule 重试。
6. 重试耗尽后标记 `dead`，并产生 `webhook.delivery.dead`。

## 7. 数据模型

### 7.1 Runs

继续复用 `generation_runs` 作为 MVP automation audit log，不新增第二套 run 表。

变更：

- `agent` enum 增加：
  - `insight`
  - `strategist`
  - `writer`
  - `qa`
  - `publisher`
  - `notification`
- 保留现有字段：
  - `input`
  - `output`
  - `model`
  - `tokens`
  - `cost_usd`
  - `status`
  - `error`
  - `created_at`
- 新增读取 query：
  - `ListGenerationRuns(project_id, agent?, status?, limit?, cursor?)`
  - `GetGenerationRun(id)`
  - `MonthlySpend(project_id)`
- List response 使用统一信封：
  - `items`
  - `next_cursor`
  - `monthly_spend_usd?`
  - `cursor` 为 opaque string，前端不能解析内部格式。

输出约定：

- Strategist 降级：`output.degraded=true`
- Strategist 搜索快照：`output.search`
- Publisher：`output.article_id`、`output.topic_id`、`output.path`、`output.branch`、`output.commit_sha`、`output.url`、`output.retryable`
- Notification：`output.delivery_id`、`output.channel_id`、`output.event_type`

验收：

- Runs 页能显示 agent、status、degraded、model、tokens、cost、created time、error summary。
- Home 显示最近 5 条 real runs。
- budget stop、publish failure 不只存在于 process logs。

### 7.2 Topics

新增 topic 写契约：

- `PUT /api/projects/{projectID}/topics/{topicID}`
  - `title`
  - `channel`
  - `target_keyword`
  - `target_prompt`
  - `angle`
  - `format`
  - `priority`
- `POST /api/projects/{projectID}/topics/{topicID}/schedule`
  - `scheduled_at`
- `POST /api/projects/{projectID}/topics/{topicID}/archive`
  - body 为空 `{}`，或只包含可选 `reason`
- 保留 `POST /api/projects/{projectID}/topics/{topicID}/generate`，但修复 duplicate 行为。

规则：

- `topics.scheduled_at` 只表达排期意图。
- canonical approve 后写入的 `articles.scheduled_at` 仍然是发布 source of truth。
- `archived` topics 不进入 generation candidate selection。
- duplicate generation 返回已有 articles 或友好 `409`，不返回 raw DB error。

### 7.3 Articles

新增/补齐：

- `GET /api/projects/{projectID}/articles/{articleID}`
- `PUT /api/projects/{projectID}/articles/{articleID}`
  - 支持 `content_md`
  - 支持 `seo_meta`
  - content changed 时同步 re-QA
  - metadata-only changed 时不 re-QA
- `POST /api/projects/{projectID}/articles/{articleID}/approve`
- `POST /api/projects/{projectID}/articles/{articleID}/reject`
- `POST /api/projects/{projectID}/articles/{articleID}/distributed`
- `POST /api/projects/{projectID}/articles/{articleID}/retry-publish`

发布失败最小状态：

- `status='publish_failed'`
- `last_publish_error`
- `publish_attempts`
- `next_publish_retry_at`
- `publish_phase`
- `resolved_slug`
- `publish_path`
- `canonical_url_verified_at`
- `last_publish_run_id`
- `pending_review_since`（推荐；若未实现则 fallback `created_at`）
- variant unlock template backup（推荐落点：`seo_meta.citeloop_unlock_template` 或等价字段）

规则：

- `publish_failed` 是显式 article status；Home、Publishing、Runs 都必须能直接查询到失败 canonical。
- publish failed 不能标记为 `published`。
- 第一次 publish attempt 必须在写 GitHub 前持久化 `resolved_slug` 和 `publish_path`。
- retry 必须复用已持久化的 `resolved_slug` 和 `publish_path`，不能因为 slug collision suffix 改变文件路径。
- 每次 publish attempt 写 `generation_runs.agent='publisher'`。
- retry success 且 URL 验证成功后，才能回填 `canonical_url` 并 unlock variants。
- content edit re-QA 是 MVP 同步流程：
  - API 可以用请求 context timeout 控制最长等待。
  - QA LLM/parse 失败时，文章必须保持或变成 `qa_blocking=true`，`qa_issues` 写入失败摘要。
  - re-QA 完成前不能 approve；metadata-only edit 不改变 `qa_blocking`。

状态机：

Canonical article 存储状态：

- `pending_review`：生成完成，等待人工 review。
- `approved`：人工已 approve，等待 `scheduled_at` 到点或 publish tick 领取。
- `pending_url_verification`：GitHub commit 已成功，`publish_result.commit_sha/path/url` 已落库，等待 UniPost rebuild 和 URL `2xx`。
- `published`：UniPost URL 已 verified，`canonical_url` 和 `canonical_url_verified_at` 已回填。
- `publish_failed`：GitHub write、deploy hook、URL verification、DB backfill 或 reconcile 失败，需要 retry 或人工处理。
- `rejected`：人工拒绝，不再进入 publish。

Variant article 存储状态：

- `pending_review`：等待人工 review。
- `approved`：人工已 approve，但 sibling canonical 尚未 verified published。
- `ready_to_distribute`：canonical 已 verified published，`{{CANONICAL_URL}}` 已替换。
- `distributed`：运营者已完成半自动分发并标记；如果 canonical 后续 reconcile 降级，只能标记 stale，不能自动收回外部分发。
- `rejected`：人工拒绝。

UI 派生桶：

- `Waiting on canonical` = variant `approved` 且 sibling canonical 不是 `published` 或缺 `canonical_url_verified_at`。
- `Ready to distribute` = variant `ready_to_distribute`。
- `Published canonical` = canonical `published`。
- `Pending URL verification` = canonical `pending_url_verification`。
- `Publish failed` = canonical `publish_failed`。

Variant unlock / rollback 规则：

- Unlock 前必须保留可回滚模板：
  - 原始 `content_md` 中包含 `{{CANONICAL_URL}}` 的版本。
  - 原始 `seo_meta` 中包含 placeholder 的版本。
- Canonical 从 `published` 降级为 `publish_failed` 时：
  - `ready_to_distribute` variants 回到 `approved`，并恢复 placeholder 模板。
  - `distributed` variants 不回退状态，只写 stale marker，并在 Publishing/Home 显示需要人工处理。
- 因 MVP 每 topic 只有一个 active canonical，variant unlock 使用同 topic canonical 的 verified `canonical_url`；不存在多 canonical 选择问题。

### 7.4 Crawl summary

`POST /insight` 需要返回或持久化 crawl summary。

最小结构：

```json
{
  "landing_url": "https://example.com",
  "strategy": "sitemap|fallback_bfs|mixed",
  "discovered_count": 0,
  "fetched_count": 0,
  "skipped_count": 0,
  "truncated": false,
  "limits": {
    "same_origin_only": true,
    "max_pages": 200,
    "max_depth": 3,
    "request_timeout_ms": 8000,
    "rate_limit_rps": 1,
    "respect_robots": true,
    "sitemap_url_cap": 2000
  },
  "errors": []
}
```

推荐落点：

- `generation_runs.output.crawl_summary`
- 同时在 `POST /insight` response 中返回最新 summary

### 7.5 Inventory evidence

`UpdateInventoryItem` 支持 `evidence_snippets`。

规则：

- evidence snippets 是 string array。
- 可以为空，但 UI 必须提示 evidence 缺失可能导致 QA blocking。
- QA prompt 继续使用 profile + source URLs + inventory evidence。

## 8. Blog publishing

### 8.1 CiteLoop publisher config

环境变量：

- `BLOG_REPO`：UniPost repo，例如 `owner/unipost`
- `BLOG_BRANCH`：默认 `citeloop-content`
- `BLOG_CONTENT_DIR`：默认 `content/citeloop/blog`
- `BLOG_BASE_URL`：环境相关 blog base
  - dev 验证：`https://dev.unipost.dev/blog`
  - production：`https://unipost.dev/blog`
- `GITHUB_TOKEN`：最小权限 token，仅用于写 UniPost repo content branch
- `UNIPOST_DEPLOY_HOOK_URL`：可选但强烈建议；当 UniPost loader 使用 build-time GitHub fetch 时，自动发布验收必须配置该值或等价 rebuild 触发方式。

诊断要求：

- Settings/Admin 中展示 publish config diagnostic：
  - repo configured
  - branch configured
  - content dir configured
  - dry-run or live mode
  - GitHub write probe result
  - UniPost deploy hook configured / missing
  - 不展示原始 token。

### 8.2 写入与并发规则

- Publisher 只能写 `content/citeloop/blog/`。
- 每次写入前校验 path prefix。
- 文件名：`content/citeloop/blog/{slug}.mdx`
- slug 来源：
  - 优先 `seo_meta.slug`
  - 若冲突，追加短 article ID suffix
- `resolved_slug` 不变式：
  - `resolved_slug` == 文件名 stem
  - `resolved_slug` == frontmatter `slug`
  - `resolved_slug` == URL path segment
  - 冲突后缀必须原子地应用到以上三者。
- 第一次 attempt 的顺序：
  - 读取 GitHub target branch，确认 path 是否存在。
  - 如 base slug 冲突，解析短 article ID suffix。
  - 将 `resolved_slug` 和 `publish_path` 先写入 article。
  - 再调用 GitHub Contents API create/update。
- 同一 article retry 必须复用 `resolved_slug` 和 `publish_path`，写回同一路径。
- GitHub 写入使用 Contents API：
  - create：目标文件不存在时创建。
  - update：目标文件存在时必须带当前文件 `sha`。
  - 禁止 blind overwrite。
- 单写保证：
  - project-level publish tick 必须串行化。
  - 同一个 project 不允许两个 publisher worker 同时写 `citeloop-content` branch。
  - 可用 DB advisory lock、job lease 或唯一 active publisher run 实现。
  - lock/lease 不能包住 GitHub、Vercel deploy hook 或 URL verification 的慢网络 I/O；网络步骤之间使用短 DB transaction 持久化阶段结果。
- GitHub `409`、branch head 变化、file `sha` mismatch 都视为 retryable conflict：
  - 不标记 `published`
  - 记录 `generation_runs.agent='publisher'`
  - 标记 article `publish_failed`
  - 设置 `next_publish_retry_at`
- URL verification 失败不应在第一次 `404` 后立刻进入 `publish_failed`：
  - commit 已成功但 URL 未 ready 时，状态保持 `pending_url_verification`。
  - verifier 按 1 分钟、5 分钟、15 分钟、30 分钟 backoff 重试。
  - 连续超过 verification budget 后才标记 `publish_failed`，并发 `publish.failed`。
- Publish automatic retry 计划：
  - GitHub/deploy/DB 阶段失败：5 分钟、15 分钟、1 小时、6 小时 backoff。
  - 第 5 次失败后停止自动 retry，保留 `publish_failed`，`next_publish_retry_at=null`，等待人工 Retry。
  - 人工 Retry 复用 `resolved_slug`/`publish_path`，并开启新一轮 attempt。
- commit message 包含：
  - CiteLoop project ID
  - CiteLoop article ID
  - title
- `publish_result` 至少保存：
  - repo
  - branch
  - path
  - commit SHA
  - URL
  - mode
  - phase
  - deploy hook result 或 skipped reason
  - `url_verified_at`

### 8.3 MDX 文件契约

frontmatter：

```yaml
---
source: citeloop
citeloop_article_id: "<uuid>"
citeloop_topic_id: "<uuid>"
slug: "example-slug"
title: "Article title"
seo_title: "SEO title"
description: "Meta description"
excerpt: "Short index excerpt"
published_at: "2026-06-05"
updated_at: "2026-06-05"
author: "UniPost"
category: "Engineering"
keywords:
  - "keyword"
---
```

body：

- 从 `articles.content_md` 渲染。
- 允许标准 Markdown/MDX 内容。
- 不允许 script tag；发现后必须拒绝发布或清理，不能原样写入 UniPost。
- 不允许任意 import；发现后必须拒绝发布或清理，不能原样写入 UniPost。
- 不做图片生成。
- 链接使用标准 markdown link。

### 8.4 UniPost content loader

当前 UniPost blog 使用 `dashboard/src/lib/blog.ts` 中的静态 `blogPosts`。本轮改造需要新增 generated content loader，并与现有 posts 合并。

要求：

- 从 UniPost `dev` 开分支。
- 新增读取 `citeloop-content` branch 的逻辑。
- 读取目录：`content/citeloop/blog/`。
- 将 MDX/frontmatter 转成现有 `BlogPost` shape 或 route-compatible shape。
- `/blog` index 展示 generated posts。
- `/blog/{slug}` detail 展示 generated post。
- existing static posts 不回归。
- generated content 读取失败时，不影响 existing blog 渲染；错误进入 deployment logs。
- dev 部署后，至少一个 generated fixture post 可通过 `https://dev.unipost.dev/blog/{slug}` 访问。

实现建议：

- Vercel 环境优先使用 build-time GitHub API fetch。
- 只有确认 Vercel dev/prod build 稳定支持时，才使用 local git fetch/build step。
- 如果采用 build-time fetch，必须配置 Vercel Deploy Hook；否则 CiteLoop commit 后不会自动触发 UniPost dev rebuild，URL verifier 会长期停留在 `pending_url_verification`。
- 如果未来改为 runtime fetch，可以不配置 Deploy Hook，但必须证明 `/blog/{slug}` 在 commit 后无需 rebuild 即可返回 `2xx`。

### 8.5 发布一致性与对账

权威 published state 由 CiteLoop DB 决定，但必须以 GitHub commit 和 UniPost URL 验证为前置条件。发布链路是分阶段、可重启流程；禁止把 GitHub、Deploy Hook、URL verification 包进一个长 DB transaction。

发布成功定义：

1. `resolved_slug`、`publish_path` 已在 GitHub write 前持久化。
2. GitHub Contents API 写入成功并返回 commit SHA。
3. `publish_result.commit_sha`、`publish_result.path`、`publish_result.url` 已持久化。
4. UniPost deploy hook 已触发，或 runtime loader 已证明无需 rebuild。
5. `BLOG_BASE_URL/{resolved_slug}` `HEAD` 或 `GET` 返回 `2xx`。
6. URL verified 后，在一个短 DB transaction 内完成：
   - `articles.status='published'`
   - `articles.published_at`
   - `articles.canonical_url`
   - `articles.canonical_url_verified_at`
   - generated inventory item

半成功处理：

- GitHub commit 成功、DB 更新失败：
  - 因 `resolved_slug` 和 `publish_path` 已在 GitHub write 前独立提交，下一次 reconcile 根据 article ID/frontmatter 和 `publish_path` 查 content branch。
  - 如文件存在且 commit SHA 可确认，补写 `publish_result` 并重新验证 URL。
  - URL 通过后再回填 `canonical_url` 和 `canonical_url_verified_at`。
- DB 显示 `pending_url_verification`：
  - reconcile 继续检查 content branch 文件、deploy hook 状态和 URL。
  - URL 通过后转 `published`。
  - verification budget 耗尽后转 `publish_failed` 并发 `publish.failed`。
- DB 显示 `published`、content branch 文件缺失或 URL 非 `2xx`：
  - 将 article 标记为 `publish_failed`。
  - 清空 `canonical_url_verified_at`。
  - 保留 `canonical_url` 作为最后一次已知 URL，但 UI 必须显示 stale/unverified。
  - 记录 publisher run。
  - 发 `publish.failed`，phase 为 `reconcile_missing_file` 或 `reconcile_url_unverified`。
- GitHub 写入失败或 conflict：
  - 保持 `resolved_slug` 和 `publish_path` 不变。
  - 标记 `publish_failed`。
  - 下次 retry 仍写同一路径。

对账入口：

- 手动 API：`POST /api/projects/{projectID}/publishing/reconcile`
- 自动 tick：每小时扫描最近 7 天内 `pending_url_verification`、`published`、`publish_failed`、`approved due` canonical。
- 对账必须可重复执行，不能因为同一 commit 已存在而重复创建文章或重复 unlock variants。

回滚边界：

- MVP 不做自动删除已发布文章。
- 如需要回滚，内部运营者手动 revert/delete UniPost content branch 中对应文件，然后运行 reconcile。
- reconcile 发现 URL 不再可达时，CiteLoop 将 canonical 标记为 `publish_failed`；`ready_to_distribute` variants 恢复 placeholder 并回到 `approved`，`distributed` variants 保持 distributed 但标记 stale。

## 9. Notification system

### 9.1 模型

参考 UniPost notification system：

- `notification_channels`
- `notification_subscriptions`
- `notification_deliveries`
- dispatcher
- delivery worker
- retry/dead state

CiteLoop 是单人内部系统，因此不需要完整 user/workspace 模型。用 `project_id` 做 scope；如需要 `user_id`，默认 `default` 或复用当前 admin identity。

### 9.2 Tables

`notification_channels`

- `id`
- `project_id`
- `kind`：`slack_webhook` 或 `discord_webhook`
- `config` jsonb
  - 不能保存 plaintext webhook URL。
  - Slack：`{"encrypted_url":"...","redacted_url":"https://hooks.slack.com/services/.../****"}`
  - Discord：`{"encrypted_url":"...","redacted_url":"https://discord.com/api/webhooks/.../****"}`
  - 如使用外部 secret store，可保存 `{"secret_ref":"...","redacted_url":"..."}`。
- `label`
- `verified_at`
- `created_at`
- `deleted_at`

`notification_subscriptions`

- `id`
- `project_id`
- `event_type`
- `channel_id`
- `enabled`
- `filter` jsonb nullable
- `created_at`
- unique `(project_id, event_type, channel_id)`

`notification_deliveries`

- `id`
- `project_id`
- `subscription_id`
- `channel_id`
- `event_type`
- `event_id`
- `payload`
- `status`：`pending`、`sent`、`dead`
- `attempts`
- `next_retry_at`
- `last_error`
- `delivered_at`
- `created_at`
- unique `(event_id, channel_id)`

### 9.3 Events

| Event | 触发时机 | 最小 payload |
|---|---|---|
| `generation.failed` | Insight / Strategist / Writer / QA 失败，需要人工处理。 | `project_id`, `run_id`, `agent`, `topic_id?`, `article_id?`, `error`, `dashboard_url` |
| `publish.failed` | BlogPublisher 写入、deploy hook、URL verification、DB backfill 或 reconcile 降级失败。 | `project_id`, `article_id`, `title`, `slug`, `phase`, `attempt`, `error`, `dashboard_url` |
| `budget.stopped` | 月度成本达到或超过 project budget，跳过生成。 | `project_id`, `spent_usd`, `budget_usd`, `period`, `dashboard_url` |
| `review.overdue` | article 持续 `pending_review` 超过阈值。 | `project_id`, `article_id`, `title`, `age_hours`, `dashboard_url` |
| `webhook.delivery.dead` | notification delivery 重试耗尽。 | `project_id`, `delivery_id`, `channel_id`, `event_type`, `last_error`, `dashboard_url` |

默认值：

- `review_overdue_hours`: 48
- worker tick: 10 seconds
- review overdue sweeper tick: 30 minutes
- retry delays: 1 minute, 5 minutes, 30 minutes
- 第 4 次失败后标记 `dead`

Emitter 定义：

- `generation.failed`
  - 只在 `generation_runs.status='failed'` 或 terminal error 时发。
  - `agent` 必须是 `insight`、`strategist`、`writer`、`qa` 之一。
  - Strategist 搜索失败但最终生成成功时，记录 `output.degraded=true`，不发 `generation.failed`。
- `publish.failed`
  - 由 BlogPublisher 在 GitHub write、commit conflict、deploy hook failure、URL verification budget exhausted、DB backfill 或 reconcile downgrade 时发。
  - 单次 URL `404` 只表示 UniPost 可能尚未 rebuild，不直接发；只有 verification budget 耗尽或 reconcile 判定 stale 时发。
- `budget.stopped`
  - 由 scheduler / generation entrypoint 在跳过 LLM 调用前发。
- `review.overdue`
  - 由 overdue sweeper 发。
  - 查询条件：`articles.status='pending_review'` 且 `now() - pending_review_since >= review_overdue_hours`。
  - 若本轮不新增 `pending_review_since`，MVP fallback 使用 `articles.created_at`，但 PRD 验收要注明这是近似计时。
  - `qa_blocking=true` 不豁免 overdue；它仍然代表需要人工处理。
- `webhook.delivery.dead`
  - 由 notification worker 在 delivery 第 4 次失败并标记 `dead` 后发。

反 spam / 幂等：

- 每个事件必须有 stable `event_id`。
- `generation.failed`：`generation.failed:{run_id}`
- `publish.failed` 首次状态转移：`publish.failed:{article_id}:{phase}:transition:{publish_attempts}`
- `publish.failed` 同一失败的日提醒：`publish.failed:{article_id}:{phase}:daily:{YYYY-MM-DD}:{sha256(error_fingerprint)}`
- `budget.stopped`：`budget.stopped:{project_id}:{period}:{sha256(budget_usd)}`
- `review.overdue`：`review.overdue:{article_id}:{YYYY-MM-DD}`
- `webhook.delivery.dead`：`webhook.delivery.dead:{delivery_id}`
- `budget.stopped` 每 project 每 calendar month 最多发一次，除非 budget 配置变化。
- `publish.failed` 状态从 `pending_url_verification` 或 `approved` 首次转入 `publish_failed` 时必须发 transition 事件；后续同一 article + phase + error fingerprint 每 24 小时最多发一个 daily reminder。
- `review.overdue` 每 article 每 24 小时最多发一次。
- `webhook.delivery.dead` 不能递归投递到同一个 dead channel；可以投递到其他 active channel，并且必须在 Runs/Dashboard 可见。

### 9.4 Delivery

Slack：

```json
{ "text": "message" }
```

Discord：

```json
{
  "content": "message",
  "username": "CiteLoop"
}
```

要求：

- HTTP timeout 10s。
- 2xx 视为成功。
- 非 2xx 或 network error 进入 retry。
- 不记录完整 webhook URL。
- message 用纯文本 + markdown，包含 dashboard link。

### 9.5 Notification UI

Settings/Admin 至少支持：

- List channels
- Create Slack webhook channel
- Create Discord webhook channel
- Send test message
- Delete/disable channel
- List supported events
- Enable/disable subscription per channel
- Show recent deliveries
- Show dead deliveries
- Retry dead/pending delivery

校验：

- Slack URL 必须以 `https://hooks.slack.com/` 开头。
- Discord URL 必须以 `https://discord.com/api/webhooks/` 或 `https://discordapp.com/api/webhooks/` 开头。
- 新建 channel 后必须发送 test message；只有 test message 成功才写 `verified_at`。
- 未 verified channel 不能被用于关键事件 subscription 的 active delivery。
- 如果所有 channel 都未 verified 或进入 dead 状态，Home 必须显示常驻 critical alert。
- 保存后 API/UI 只显示 redacted URL preview。
- `notification_channels.config` 必须无条件做 at-rest protection：
  - 优先复用当前 admin credentials 的加密 helper。
  - 若没有 helper，本轮新增最小 app-level encryption，密钥来自 env，例如 `NOTIFICATION_SECRET_KEY`，算法使用 AEAD。
  - 也可保存 secret reference，但不能保存 plaintext URL。
- webhook URL 不进入 logs、Runs output、delivery payload、API response。

## 10. Frontend requirements

### 10.1 Home

必须展示：

- Next scheduled
- Needs review
- Ready to distribute
- Recent runs（真实数据）
- Active critical alerts：
  - budget stopped
  - publish failed
  - generation failed
  - review overdue
  - notification delivery dead
  - no verified notification channel
  - stale distributed variant after canonical rollback

### 10.2 Knowledge

新增：

- Crawl summary panel
- limits / discovered / fetched / skipped / truncated / errors
- Evidence snippets 展示
- Evidence snippets 编辑

### 10.3 Topics

新增：

- Topic edit drawer 或 inline editor
- Schedule date/time
- Archive
- status/channel/priority filters
- duplicate generation 友好提示

### 10.4 Review

新增：

- SEO metadata editor
- metadata-only edit 不清 QA blocking 的明确提示
- article detail link
- approve/reject/edit 后局部刷新

### 10.5 Publishing

新增：

- Published canonical
- Pending URL verification
- Publish failed + retry
- Waiting on canonical
- Ready to distribute
- Stale distributed variant
- Copy success feedback
- Live article link
- `publish_result` detail

### 10.6 Runs

替换 placeholder page，展示：

- Run list
- Agent/status filters
- Degraded badge
- Cost summary
- Error summary
- Strategist search snapshot preview
- Publisher attempts
- Notification delivery failures

### 10.7 Settings/Admin

补齐：

- Project config
  - cadence
  - buffer
  - channel mix
  - crawl config
  - monthly budget
  - review overdue threshold
- Notification channels/subscriptions
- Publish diagnostics
  - repo configured
  - branch configured
  - content directory configured
  - GitHub write probe result
  - UniPost deploy hook configured / missing
  - dry-run/live mode

## 11. API requirements

Project-level：

- `GET /api/projects/{projectID}/runs?agent=&status=&limit=&cursor=`
- `GET /api/projects/{projectID}/runs/{runID}`
- `GET /api/projects/{projectID}/notifications/channels`
- `POST /api/projects/{projectID}/notifications/channels`
- `DELETE /api/projects/{projectID}/notifications/channels/{channelID}`
- `GET /api/projects/{projectID}/notifications/events`
- `GET /api/projects/{projectID}/notifications/subscriptions`
- `PUT /api/projects/{projectID}/notifications/subscriptions`
  - 请求语义为 upsert 单个 subscription。
  - body：`event_type`、`channel_id`、`enabled`、`filter?`
  - response 返回 upsert 后 subscription。
- `GET /api/projects/{projectID}/notifications/deliveries?status=&limit=`
- `POST /api/projects/{projectID}/notifications/deliveries/{deliveryID}/retry`
- `GET /api/projects/{projectID}/publishing/diagnostics`
- `POST /api/projects/{projectID}/publishing/reconcile`

Topic-level：

- `PUT /api/projects/{projectID}/topics/{topicID}`
- `POST /api/projects/{projectID}/topics/{topicID}/schedule`
- `POST /api/projects/{projectID}/topics/{topicID}/archive`

Article-level：

- `GET /api/projects/{projectID}/articles/{articleID}`
- `PUT /api/projects/{projectID}/articles/{articleID}`
- `POST /api/projects/{projectID}/articles/{articleID}/approve`
- `POST /api/projects/{projectID}/articles/{articleID}/reject`
- `POST /api/projects/{projectID}/articles/{articleID}/distributed`
- `POST /api/projects/{projectID}/articles/{articleID}/retry-publish`

Scope / ownership：

- MVP 对外 article/topic mutation route 必须带 `projectID`。
- 旧 flat article/topic mutation routes 不作为 MVP 接口暴露。
- 所有 topic/article mutation query 必须同时约束 entity id 和 `project_id`。
- 不允许仅凭 ID 操作其他 project 的 topic/article。
- 新实现必须避免裸 DB error 外泄。

## 12. 错误处理

- LLM/Search：
  - 写 run。
  - Search failure 可 degraded success。
  - generation failure 发 `generation.failed`。
- Budget：
  - LLM 调用前检查。
  - 达预算后跳过生成。
  - 写 run/event。
  - 发 `budget.stopped`。
- Publish：
  - 失败不标 `published`，并显式标记 `publish_failed`。
  - 写 publisher run。
  - Publishing/Runs 可见。
  - 发 `publish.failed`。
  - 可 retry。
  - GitHub commit 成功但 UniPost URL 尚未 ready 时，进入 `pending_url_verification`，不立即算失败。
  - 如果 UniPost 使用 build-time loader，commit 后必须触发 deploy hook 或等待外部 deploy；verification budget 耗尽才转 `publish_failed`。
  - GitHub commit 成功但 DB backfill 失败时，进入 reconcile，不重复创建 content file。
  - URL verification 失败时，不回填 `canonical_url`，variants 不 unlock。
- Notification：
  - pending delivery 按 schedule retry。
  - 重试耗尽标 dead。
  - 写 notification run/delivery。
  - 发 `webhook.delivery.dead`，但不递归到同一个 dead channel。
- UniPost content load：
  - existing blog 不崩。
  - deployment logs 可见。
  - dev 验证 fixture 不存在则验收失败。

## 13. 安全与运维约束

- GitHub token 必须最小权限。
- Publisher 每次写入前校验 path prefix。
- 任何 generated file 都不能写出 `content/citeloop/blog/`。
- path prefix 校验必须拒绝：
  - `../`
  - absolute path
  - URL-encoded path traversal
  - symlink-like 或 normalize 后逃逸的路径
- Webhook URL 是敏感信息：
  - at-rest 必须加密或保存 secret reference
  - 保存后脱敏
  - 不进日志
  - 不进入 Runs output
  - 不进入 notification delivery payload
- Admin/settings 只面向内部使用。
- API/CORS 不应因为本轮收口变成公开 SaaS API。
- Generated markdown 视为内容，不视为代码：
  - 拒绝或清理 script tag
  - 不允许任意 MDX import
  - 不允许通过 HTML event handler 注入可执行代码
  - 允许标准 markdown

## 14. Rollout plan

### Phase 1：CiteLoop operability

- Runs list/detail API
- Runs page + Home recent runs
- Topic edit/schedule/archive
- Crawl summary
- Inventory evidence edit
- Article detail + metadata edit
- Duplicate generation friendly behavior

### Phase 2：Notifications

- Notification migrations/sqlc
- Dispatcher
- Delivery worker
- Slack/Discord sender
- Settings UI
- Critical event emission
- Retry/dead visibility

### Phase 3：Publisher hardening

- Publish diagnostics
- Branch/path restriction
- MDX frontmatter renderer
- Publisher runs
- Publish failed UI
- Pending URL verification UI
- Retry publish
- GitHub Contents API create/update `sha` contract
- Conflict retry handling
- UniPost deploy hook trigger / pending verification handling
- Canonical URL backfill
- Variant unlock verification
- Reconcile API/tick

### Phase 4：UniPost dev integration

- 在 UniPost 从 `dev` 开分支。
- 增加 generated content loader。
- 合并 generated posts 到 blog index/detail。
- 推到 `origin/dev`。
- 部署并验证 `https://dev.unipost.dev/blog/{slug}`。

### Phase 5：Final e2e verification

- CiteLoop 后端测试。
- CiteLoop 前端 build。
- UniPost dashboard build。
- 用 test project 跑 Insight -> Strategist -> Generate -> Review。
- Approve canonical。
- Publish 到 `citeloop-content`。
- 触发 UniPost dev deploy hook 或确认 runtime loader 无需 rebuild。
- 验证 UniPost dev article URL。
- 验证 CiteLoop 回填 canonical URL。
- 验证 variants unlock。
- 验证 Slack/Discord notification channel。

## 15. 验收清单

MVP 收口完成必须满足：

1. Runs 页显示真实 run data、degraded、cost、errors。
2. Home 不再显示 recent runs placeholder。
3. Topics 可编辑、可排期、可归档、可安全生成一次。
4. Knowledge 显示 crawl summary 和可编辑 evidence snippets。
5. Review 支持正文编辑、SEO metadata 编辑、content edit 后同步 re-QA、blocking guard；无证据事实声明 fixture 必须产生 `qa_blocking=true` 且不可 approve。
6. Approved due canonical 自动发布到 UniPost repo `citeloop-content` branch 的 `content/citeloop/blog/`。
7. Publisher 使用 GitHub Contents API create/update；update 必须带 file `sha`。
8. Project-level publisher tick 串行化；并发 worker 不会同时写同一 project content branch。
9. Publisher lock/lease 不包住 GitHub、Deploy Hook、URL verification 的慢网络 I/O。
10. GitHub `409` / `sha` mismatch 被标记为 retryable `publish_failed`，不标记 `published`。
11. 同一 article retry 复用第一次持久化的 `resolved_slug` 和 `publish_path`。
12. `resolved_slug`、文件名 stem、frontmatter slug、URL path segment 四者一致。
13. Commit 成功但 URL 尚未 ready 时进入 `pending_url_verification`，不会立刻 spam `publish.failed`。
14. Publish failure 在 Home/Publishing/Runs 可见且可 retry；自动 retry 有 backoff 和上限。
15. GitHub commit 成功但 DB backfill 失败时，reconcile 能基于已持久化 `publish_path` 补写 `publish_result` 并重新验证 URL。
16. DB 显示 published 但 content branch 缺文件或 URL 非 `2xx` 时，reconcile 清空 `canonical_url_verified_at`、标记 `publish_failed`、发 `publish.failed`。
17. 手动 revert/delete UniPost content file 后运行 reconcile，canonical 降级为 `publish_failed`；`ready_to_distribute` variants 恢复 placeholder 并回到 `approved`，`distributed` variants 保持 distributed 但显示 stale。
18. UniPost dev 在 `/blog/{slug}` 渲染 generated article，不破坏 existing posts。
19. CiteLoop 只在 GitHub commit 成功且 `BLOG_BASE_URL/{slug}` 返回 `2xx` 后回填 `canonical_url`。
20. Variants 在 canonical publish + verified real URL 前绝不进入 `ready_to_distribute`。
21. Notification channels/subscriptions/deliveries 可用。
22. Slack 或 Discord webhook channel test message 成功后才写 `verified_at`；至少一个 verified channel 可成功收到关键事件。
23. `review.overdue` sweeper 能为超时 `pending_review` article 产生事件，且 24 小时内不重复 spam；如未实现 `pending_review_since`，验收报告必须标注使用 `created_at` 近似计时。
24. `publish.failed` 首次转入失败会发 transition event，后续同一 article + phase + error fingerprint 24 小时内只发一个 daily reminder。
25. Strategist degraded success 只记录 `output.degraded=true`，不发 `generation.failed`；terminal run failure 会发。
26. `budget.stopped` 使用包含 project、period、budget hash 的 stable `event_id`。
27. Dead delivery 可见，且不会无限递归；唯一 channel dead 时 Home 有常驻 critical alert。
28. Webhook URL at-rest 加密或保存 secret reference；UI/API/logs/Runs/delivery payload 都不返回 raw secret。
29. Path traversal / absolute path / URL-encoded escape 写入会被拒绝。
30. Generated MDX 中 script tag、arbitrary import、HTML event handler 会被拒绝或清理。
31. Flat topic/article routes 不作为 MVP 接口暴露；project-scoped routes 校验 ownership，不允许跨 project 操作。
32. CiteLoop `go test ./...` 通过。
33. CiteLoop `web` 的 `npm run build` 通过。
34. UniPost dashboard build 通过。
35. `https://dev.unipost.dev/blog/{slug}` 能访问 generated test article。

## 16. 实现前必须核验的事实

这些不是产品问题，不需要再做产品决策；实现时直接核验即可：

- UniPost repo 的准确 `BLOG_REPO` 值。
- `citeloop-content` branch 是否已存在；不存在则从当前 UniPost 默认内容状态创建。
- UniPost Vercel dev deploy 是否有可用于 build-time GitHub fetch 的安全 token。
- UniPost Vercel dev 是否提供 Deploy Hook；如没有，必须确认 loader 是 runtime fetch 或接受 `pending_url_verification` 需要人工 deploy。
- 如何最小改动 `dashboard/src/lib/blog.ts` 或其相邻模块，以合并 generated content 且不回归 existing blog。
- CiteLoop 当前 admin/credential 改动是否已有加密 helper；如有则复用到 notification URL storage，如没有则新增最小 app-level encryption。

若事实与预期不一致，实现时选择最小兼容调整，但不能改变本 PRD 中已确定的产品决策。

## 17. Definition of Done

内部运营者可以完成以下流程，即视为 MVP 收口完成：

1. 配置 project settings 和 notification channels。
2. 运行 Insight，并检查 crawl/evidence。
3. 运行 Strategist，并整理 topic backlog。
4. Generate、review、edit、approve 内容。
5. 让 due canonical 自动发布到 UniPost content branch。
6. 在 `dev.unipost.dev` 确认 generated article。
7. 不看 server logs 也能看到失败，并能 retry publish。
8. 通过 webhook channel 收到关键运营告警。
9. 只在 canonical URL 真实存在后手动分发 variants。

超出以上范围的能力，进入 post-MVP iteration。
