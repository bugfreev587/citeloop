# PRD：CiteLoop 竞品差距补齐路线图

> 日期：2026-06-08
> 阶段：MVP 收口之后的产品化路线图
> 参考对象：Outrank.so 公开产品与文档
> 依赖：`docs/PRD-CiteLoop-MVP-Closure.md`、`docs/PRD-CiteLoop-SEO-Operations-Loop.md`、`docs/PRD-CiteLoop-SEO-Autopilot.md`、`docs/PRD-CiteLoop-GEO-Visibility-Layer.md`

## 1. 背景

CiteLoop 已经具备内容生成、人工 review gate、GitHub/MDX 自动发布、GSC/GA4 数据接入、SEO opportunity、autopilot policy、通知和运行记录等基础能力。但从用户视角看，CiteLoop 仍更像“内部运营控制台”，而不是一个真实用户可以自助上手、持续看到收益、低成本授权和自动运营的网站增长产品。

Outrank.so 的公开页面和文档体现了几个更成熟的产品化能力：

- 用网站/产品作为入口，生成内容计划和自动发布承诺。
- 把多 CMS 发布包装成 one-click integrations，包括 WordPress、Ghost、Webflow、Notion、Wix、Shopify、WordPress.com、Webhook、Framer、Next.js Blog。
- 用 Google Search Console 驱动 Article Improvements，持续刷新已发布内容，并允许 review 或 auto-push。
- 提供 Free Tools Builder，把免费工具作为 SEO/GEO 增长资产。
- 提供 REST API 和 CLI，让外部系统或 coding agent 能程序化操作产品。
- 把 backlinks / ChatGPT mentions / articles created / traffic outcome 作为可感知成果。

参考链接：

- Outrank homepage: https://www.outrank.so/
- Integrations: https://www.outrank.so/integrations
- Article Improvements: https://www.outrank.so/docs/improvements
- REST API: https://www.outrank.so/docs/api
- Tools Builder: https://www.outrank.so/tools-builder

本 PRD 的目标不是复制 Outrank，而是把 CiteLoop 当前能力补齐成“真实用户可自助使用、能持续自动运营 SEO/GEO、并能解释收益”的产品。

## 2. 第一性原理

SEO/GEO 自动运营产品必须解决六件事：

1. **进入成本低**：用户应该从 domain 开始，而不是理解 GSC property、credential ref、repo token、deploy hook、crawl config。
2. **计划可信**：系统不能只生成文章列表，必须解释为什么这些主题、页面或工具值得做。
3. **执行省心**：生成、改稿、发布、验证、失败告警应该串成闭环，而不是散落在多个按钮里。
4. **风险可控**：低风险动作可以自动，高风险动作进入人工决策；系统必须避免无限循环、误发布和黑帽策略。
5. **收益可见**：每个动作必须有 baseline、执行记录、观察窗口和 outcome。
6. **可扩展到外部系统**：真实客户会使用不同 CMS、不同站点结构、不同运营习惯；CiteLoop 需要 integrations、API 和 CLI，而不是只支持内部 UniPost。

## 3. 当前 CiteLoop 基线

### 3.1 已有能力

- **内容生成链路**：Insight、Strategist、Writer、QA，支持 topics、review queue、approve/reject。
- **Review gate**：已有 review queue、人工 approve/reject、文章 preview、SEO contribution、人工编辑后 QA requalify。自动的 `QA feedback -> AI editor -> QA rerun` 修复循环、editor/reviser agent、以及 QA 修复 attempt cap 不作为本 PRD 的当前基线；它们属于 Phase 2 必须新增的能力。
- **发布能力**：GitHub Contents API 写 Next.js repo 内 MDX 文件，支持 canonical URL 和发布验证。
- **SEO operations**：已有 GSC/GA4 ingest、overview、opportunities、content actions、brief、autopilot policy、safe mode 的数据结构和部分 UI。
- **通知**：支持 Slack/Discord webhook channel、subscriptions、retry/dead delivery。
- **运行记录**：generation runs 和 SEO runs 能记录 agent、status、cost、error。
- **认证和项目隔离**：Clerk auth、project owner guard 已经在 API 层存在。

### 3.2 主要差距

| 差距 | 当前状态 | 产品影响 | 优先级 |
|---|---|---|---|
| Domain-first onboarding | 创建项目只收 name/slug，SEO 设置暴露内部字段 | 真实用户无法自助接入 | P0 |
| Growth plan / content calendar | 有 topic backlog，但不是业务目标驱动计划 | 用户不理解为什么要写这些内容 | P0 |
| Article Improvements | 有 SEO opportunity/action，但没有真实 rewrite diff inbox，也没有可复用的 editor/reviser agent | 无法持续优化已发布内容 | P0 |
| 多 CMS 发布 | 主要支持 env-var 驱动的 GitHub MDX；syndication 仍半手动；缺 per-project publisher connection | 客户站点覆盖面太窄，auto-push 无法安全落地 | P0/P1 |
| Outcome attribution | 有指标和 runs，但缺 action-level before/after 故事 | 用户无法感知 ROI | P1 |
| Free Tools Builder | 没有工具页生成能力 | 少一个高杠杆 SEO/GEO 增长面 | P1 |
| Public API / CLI | 内部 API 已有，但没有 API key、外部文档、agent CLI | 难以成为可编排平台 | P1/P2 |
| Partner link/citation | 没有外链或 citation opportunity | 缺少 off-page 增长能力 | P2 |

## 4. 产品目标

### 4.1 短期目标

时间范围：0 到 8 周；如果同一小团队同时推进 Webhook/WordPress publisher，则顺延到 10 周更现实。

目标：让一个真实用户只输入 domain，就能完成可理解 onboarding，获得一份可执行的 SEO/GEO growth plan，并通过 review queue 处理新内容和已发布文章改进。

必须补齐：

- Domain-first project creation。
- Setup checklist 和 connection health。
- Growth Plan / Calendar。
- Article Improvements MVP，包括 editor/reviser agent 和 repair state machine。
- Publisher connection foundation：至少 GitHub/Next.js per-project 配置、capability schema、credential reference。
- Webhook publisher 和 WordPress publisher 的第一版。

### 4.2 中期目标

时间范围：1 到 3 个月。

目标：让 CiteLoop 从“内容生成工具”升级为“持续运营系统”，自动发现机会、生成改稿、发布到主流 CMS，并能解释每个动作的效果。

必须补齐：

- Outcome attribution。
- Improvements auto-push policy。
- Free Tools Builder MVP。
- API key、REST API 文档、CLI。
- 多站点/多 publisher 配置能力。

### 4.3 长期目标

时间范围：3 到 6 个月。

目标：让 CiteLoop 成为可扩展的 SEO/GEO autopilot 平台，支持多租户、团队、外部 agent、合作伙伴 citation/link opportunity，并能在安全边界内自动执行低风险增长动作。

必须补齐：

- Agent/CLI ecosystem。
- Partner citation/link opportunity marketplace。
- Billing/usage/limits。
- Team roles 和 agency/multi-client workflow。
- GEO visibility layer 与 SEO actions 的闭环融合。

## 5. 非目标

- 不做开放式 backlink exchange 或 PBN。
- 不承诺排名、流量或 ChatGPT mention 一定增长。
- 不使用 Google Indexing API 批量提交普通 blog 文章。
- 不允许用户只输入 domain 后绕过合法授权读取 GSC/GA4 私有数据。
- 不在第一阶段支持所有 CMS；优先选择 Webhook、WordPress、GitHub/Next.js。
- 不自动发布高风险动作，例如 homepage/pricing rewrite、canonical/noindex/robots 改动、删除页面、redirect。

## 6. 总体产品形态

目标用户进入 CiteLoop 后看到的不是“项目控制台”，而是一个运营 cockpit：

1. **Setup**：告诉用户还缺哪些权限，以及缺权限时系统能做到什么。
2. **Plan**：展示 30 天内容和改稿计划。
3. **Review**：只展示需要人类决策的 draft、improvement、工具页、发布风险。
4. **Autopilot**：展示自动执行等级、预算、低风险动作边界、safe mode。
5. **Outcomes**：展示每个动作的 before/after 和整体趋势。

## 7. Phase 0：Domain-first Onboarding 与 Connection Health

### 7.1 目标

把 CiteLoop 的入口从“创建一个项目壳”改为“输入产品 domain，系统自动发现站点并引导授权”。

实现说明：当前分支已有 guided SEO permission onboarding / tenant slug 相关改造痕迹。Phase 0 不是 greenfield 重写；第一步必须盘点现有 onboarding、project tenant、authz、project creation 代码，把已存在的字段和 UI 与本 PRD 的 checklist/state 统一，避免重复 schema 和重复流程。

### 7.2 用户故事

作为一个 SaaS founder，我只想输入 `example.com`，然后看到 CiteLoop 已经发现了哪些页面、还需要我授权什么、授权后能自动做什么。

### 7.3 功能需求

1. Project creation 第一屏只要求：
   - product domain
   - optional project name
2. 后端自动执行 domain discovery：
   - canonical host
   - www/non-www
   - https/http redirect
   - sitemap
   - robots
   - likely blog/docs path
   - RSS/Atom
   - public page count sample
3. 生成 setup checklist：
   - `public_crawl`
   - `search_data`
   - `analytics_data`
   - `publisher_write`
   - `site_verification`
   - `notification`
   - `policy`
   - `dry_run`
4. UI 不暴露 `credential_ref`、service account、GSC property type 等内部字段。
5. 缺权限时进入明确降级模式：
   - `public_only`
   - `managed_content_connected`
   - `customer_site_pending_verification`
   - `customer_site_connected`
6. 每个 checklist item 必须说明：
   - 当前状态
   - 为什么需要
   - 用户下一步动作
   - 跳过后的能力限制

### 7.4 验收标准

1. 新用户可以只输入 domain 创建 project。
2. 创建后 60 秒内出现 public crawl summary。
3. 没有 GSC/GA4 时不显示 CTR、position、conversion 作为事实数据。
4. SEO settings 页面不再要求普通用户填写 `credential_ref`。
5. 至少能展示三种模式：`public_only`、`pending_verification`、`connected`。
6. Product owner 可以在 UI 上清楚看到“现在 CiteLoop 能自动做什么，不能做什么”。

## 8. Phase 0.5：Publisher Connection Foundation

### 8.1 目标

在 Article Improvements 和 auto-push 之前，先把发布目标从全局 env var 配置升级为 per-project publisher connection。没有这层能力，Phase 2 的 low-risk auto-push、Phase 3 的多 CMS、Phase 5 的工具页发布都会缺少安全落点。

### 8.2 用户故事

作为用户，我希望每个 project 都能连接自己的发布目标，并清楚知道 CiteLoop 可以在该目标上创建、更新、保存草稿、发布和回滚哪些内容。

### 8.3 功能需求

1. 新增或复用 publisher connection 配置层，至少覆盖 GitHub/Next.js：
   - repo
   - branch
   - content path
   - base URL
   - publish mode
   - credential reference
2. 所有 publisher 声明同一份 capability schema：
   - `create_article`
   - `update_article`
   - `metadata_update`
   - `canonical`
   - `media_upload`
   - `draft_mode`
   - `publish_mode`
   - `delete`
   - `rollback`
3. `publisher_write` checklist item 读取 publisher connection health，而不是读取全局 env var。
4. 现有 GitHub MDX publisher 必须迁移为 capability-driven adapter；env var 只能作为内部 fallback。
5. 所有 publisher credentials 使用统一 secret store 方案，不把 raw token/webhook URL 返回给前端。

### 8.4 安全要求

1. 第三方 CMS token、webhook secret、OAuth refresh token 必须 encrypted at rest。
2. Credential 作用域必须绑定 project 和 publisher connection。
3. OAuth provider 必须记录 access token expiry、refresh status、last verified time 和 revoke 状态。
4. Webhook publisher 必须支持 signed payload、idempotency key 和 replay 防护。
5. Connection test 不得把 secret 写入 logs、runs output、notification payload 或 API response。

### 8.5 验收标准

1. 一个 project 可以在 UI/API 中配置 GitHub/Next.js publisher，不需要改 env。
2. Publisher health 能出现在 setup checklist 中。
3. Review/Publish 页面只展示该 publisher capability 支持的动作。
4. GitHub/Next.js publisher 仍能发布现有 MDX 内容。
5. raw credential 不会出现在浏览器、日志、runs、notifications。

## 9. Phase 1：Growth Plan / Content Calendar

### 9.1 目标

把 topics backlog 升级成业务目标驱动的 30 天 growth plan，让用户理解每篇文章、每次改稿、每个工具页的原因和预期收益。

### 9.2 用户故事

作为单人运营者，我希望每周打开 CiteLoop 就看到“本周最该做的 5 件事”，而不是自己筛 topic、看 GSC 表格和猜优先级。

### 9.3 功能需求

1. Growth plan 由三类动作组成：
   - new article
   - article improvement
   - free tool idea
2. 每个 plan item 包含：
   - target keyword 或 target prompt
   - user intent
   - business value
   - evidence
   - expected contribution
   - risk level
   - required permission
   - scheduled date
3. Calendar 支持：
   - weekly view
   - 30-day view
   - backlog drag/schedule
   - cadence control
4. 系统必须解释为什么选这个主题：
   - GSC opportunity
   - SERP gap
   - competitor coverage
   - product feature priority
   - GEO visibility gap
5. 与 budget 和 autopilot policy 联动：
   - 超预算不自动生成
   - high-risk item 必须 review
   - low-risk item 可进入 auto queue

### 9.4 验收标准

1. 用户可以生成 30 天 growth plan。
2. 每个 plan item 至少有一个 evidence source 和一个 expected contribution。
3. Calendar 能展示 article、improvement、tool 三类 item。
4. 用户能 approve、reschedule、dismiss 任意 item。
5. 被 dismiss 的 item 记录原因，并影响下一次 planner。
6. 生成内容前，用户能看懂“为什么这项值得做”。

## 10. Phase 2：Article Improvements Inbox

### 10.1 目标

让 CiteLoop 持续优化已发布内容，而不是只生成新文章。

### 10.2 用户故事

作为用户，我希望 CiteLoop 根据 Search Console 和内容质量自动发现需要刷新、改标题、补内链、补事实、修结构的旧文章，并先交给 AI 修复，再让我只处理真正需要判断的部分。

### 10.3 功能需求

1. Opportunity analyzer 生成 improvement candidates：
   - striking distance
   - CTR rewrite
   - content decay
   - cannibalization
   - missing internal links
   - outdated facts
   - thin content
   - indexing anomaly
2. Improvement draft 必须是 diff-first：
   - original article
   - proposed article
   - structured diff
   - changed sections
   - metadata diff
   - internal link diff
3. QA 先自动修复：
   - 新增 editor/reviser agent，不能假设现有 Writer/QA 已包含此能力
   - QA feedback 给 AI editor
   - AI editor 修改后再次 QA
   - 最多 2 次自动修复
   - repair state 必须持久化，防止页面刷新或 worker 重试造成无限循环
   - 超过上限进入 human decision
4. Human decision 只处理：
   - product positioning conflict
   - insufficient evidence
   - high-risk page
   - legal/compliance issue
   - destructive SEO change
5. Improvement 可进入两种发布策略：
   - review required
   - auto-push low-risk
6. Auto-push 依赖 Phase 0.5 的 publisher capability 和 secret isolation。没有 `update_article` / `metadata_update` capability 的 publisher，只能生成 review draft，不能自动发布。

### 10.4 验收标准

1. 至少能从 GSC/page data 生成 5 类 improvement candidate。
2. 每个 candidate 都能创建 improvement draft。
3. Review 页面展示 old/new diff、SEO reason、expected impact、risk level。
4. QA blocking draft 会自动进入 AI repair loop，不要求用户手动点击 AI fix。
5. 自动修复超过上限后展示 2 到 4 个明确人工选择。
6. Low-risk metadata rewrite 可以在 policy 允许时自动发布。
7. 发布后写入 baseline 和 measurement window。
8. 修复循环有持久化 attempt cap，QA 和 editor 不会无限往返。

## 11. Phase 3：Publisher Integrations

### 11.1 目标

把 CiteLoop 从 Next.js/GitHub-first 扩展为多发布目标系统，覆盖真实用户最常见的 CMS。

### 11.2 集成优先级

1. **Webhook publisher**：最通用，允许用户把内容推到任意系统。
2. **WordPress publisher**：覆盖最大 CMS 用户群。
3. **GitHub/Next.js publisher**：强化现有能力，支持 GitHub App/OAuth。
4. **Webflow / Ghost / Shopify**：中期补齐。
5. **Notion / Wix / Framer**：长期或按客户需求补齐。

### 11.3 功能需求

1. Publisher connection 不要求普通用户手填 raw token，优先 OAuth/App/integration。需要 raw token/webhook URL 的场景只能作为 advanced fallback，并仍走统一 secret store。
2. 每个 publisher 必须使用 Phase 0.5 的 capability schema；UI、planner、review、auto-push 都读取同一份 capability，不另行维护一套能力判断。
3. 每个 publisher 必须声明 capabilities：
   - create article
   - update article
   - metadata update
   - canonical support
   - media upload
   - draft mode
   - publish mode
   - delete support
4. Webhook publisher 必须支持：
   - signed payload
   - retry
   - idempotency key
   - delivery log
   - preview payload
5. WordPress publisher 必须支持：
   - post create/update
   - category/tag mapping
   - featured image optional
   - status draft/publish
   - canonical/frontmatter fallback
6. Publisher health 必须进入 setup checklist。

### 11.4 验收标准

1. 用户可以连接 Webhook publisher，并收到 test payload。
2. 用户可以连接 WordPress publisher，并发布 draft post。
3. 每次 publish 有 delivery log、error、retry、public URL。
4. Publisher 不支持的能力不会在 UI 上伪装为可用。
5. GitHub/Next.js publisher 支持通过 UI 配置 repo、branch、content path，不要求改 env。
6. Publish failure 进入 notification 和 Needs attention。

## 12. Phase 4：Outcome Attribution

### 12.1 目标

让用户看到 CiteLoop 的每个动作是否与结果变化相关，同时避免把单一 before/after 窗口包装成确定因果。

### 12.2 用户故事

作为用户，我希望每篇文章、每次改稿、每个工具页都有“做之前、做之后、结果如何、置信度如何”的记录。

### 12.3 功能需求

1. 每个 action 记录：
   - baseline window
   - execution timestamp
   - measurement window
   - target metric
   - expected impact
   - observed impact
   - outcome label
   - confidence level
   - attribution method
2. Outcome label：
   - `improved`
   - `neutral`
   - `worsened`
   - `inconclusive`
3. 置信标准：
   - `improved` / `worsened` 必须满足最小 delta、方向一致、数据量门槛。
   - 站点整体趋势剧烈波动时，优先标记 low confidence 或 `inconclusive`。
   - 可行时使用 site trend normalization、同类页面对照或 holdout。
   - 不满足数据充分度时不输出强结论。
4. 指标来源：
   - GSC clicks/impressions/CTR/position
   - GA4 engagement/conversion
   - crawl/indexing status
   - GEO visibility observation
5. UI 需要展示：
   - action-level before/after
   - weekly summary
   - month-to-date impact
   - confidence 和 inconclusive 原因
   - “相关性，不保证因果”的解释
6. Outcome 进入 planner memory：
   - 高置信有效动作提高权重
   - 高置信无效或负向动作降低权重
   - low-confidence / inconclusive 只能作为弱信号

### 12.4 验收标准

1. 每个 published action 自动创建 baseline。
2. 7/14/28 天 checkpoint 自动计算 outcome。
3. 用户能在 action detail 看到 before/after、confidence、method。
4. 如果 GSC 数据不足，显示 `inconclusive`，不伪造收益。
5. Planner 引用历史 outcome 时必须带 confidence。
6. 产品文案不承诺单个动作一定导致排名或流量变化。

## 13. Phase 5：Free Tools Builder MVP

### 13.1 目标

让 CiteLoop 生成可发布的小工具页面，作为比普通文章更强的 SEO/GEO 资产。

### 13.2 用户故事

作为 SaaS 用户，我希望 CiteLoop 根据我的产品和搜索机会，自动建议并生成一些免费工具，比如 calculator、checker、generator，让这些工具带来搜索流量、引用和潜在转化。

### 13.3 MVP 范围

第一版只支持模板化、deterministic、可静态发布的工具，不支持任意代码生成，也不支持面向终端访客的 LLM-backed public endpoint。

工具类型：

1. Calculator
2. Checker
3. Generator
4. Comparison table
5. Template library

每个工具由以下结构组成：

- landing copy
- input schema
- deterministic output rules
- result explanation
- CTA
- FAQ
- metadata
- structured data
- sitemap entry
- usage counter

LLM-backed 工具推迟到后续版本，前置条件包括 serving runtime、per-IP/per-visitor 限流、abuse monitoring、成本上限、日志审计和 publisher/runtime 能力声明。

### 13.4 功能需求

1. Tool idea 来自：
   - search opportunity
   - competitor free tools
   - product feature
   - support questions
   - GEO prompt gaps
2. Tool draft 必须进入 review queue。
3. Tool 发布必须通过 publisher capability check。
4. 静态工具的输入、输出和边界条件必须可 preview。
5. Tool outcome 纳入 attribution。

### 13.5 验收标准

1. 系统能生成至少 5 个 tool ideas。
2. 用户能选择一个 idea 生成 deterministic tool draft。
3. Tool draft 可以在 preview 中交互。
4. Tool 可以发布到 Next.js/GitHub 或 Webhook publisher。
5. Published tool 进入 sitemap。
6. Tool usage 和 SEO outcome 可追踪。
7. MVP 中没有公开 LLM-backed 工具接口，也不会产生按终端访客调用计费的后端调用。

## 14. Phase 6：Public API、API Key 与 CLI

### 14.1 目标

让外部系统、用户自己的 agent、coding agent 能安全调用 CiteLoop。

### 14.2 用户故事

作为高级用户或 agency，我希望可以用 API/CLI 创建项目、触发 crawl、读取 plan、approve draft、发布内容、导出 metrics，而不是只能点 UI。

### 14.3 功能需求

1. API key 管理：
   - create
   - revoke
   - scope
   - last used
   - rate limit
2. API scopes：
   - `project:read`
   - `project:write`
   - `plan:read`
   - `draft:review`
   - `publish:write`
   - `metrics:read`
   - `admin:billing`
3. Clerk browser session 和 API key 是两条认证入口，但必须进入同一套 authz 层：
   - Clerk session resolve user -> allowed projects -> project role/scopes。
   - API key resolve project/user/service principal -> key scopes -> project guard。
   - 任何写操作同时检查 project 权限、token scope、policy 和 idempotency。
4. REST API 文档：
   - authentication
   - pagination
   - idempotency
   - errors
   - webhooks
5. CLI 支持：
   - `citeloop projects create --domain`
   - `citeloop plan generate`
   - `citeloop review list`
   - `citeloop review approve`
   - `citeloop publish`
   - `citeloop metrics`
6. Agent usage 必须可审计，并写入 API audit events。

### 14.4 验收标准

1. 用户可以创建 API key。
2. API key 只允许访问授权 project。
3. CLI 能完成从 domain create 到 plan list 的只读流程。
4. Approve/publish 类命令要求明确确认或 scoped token。
5. 所有 API 调用写 audit log，包含 subject、project、scope、endpoint、status、request id。
6. API 文档可以被外部 agent 直接使用。

## 15. Phase 7：Partner Citation / Safe Link Opportunities

### 15.1 目标

提供 off-page 增长建议，但不做低质量 backlink exchange。

### 15.2 原则

Outrank 把 backlink exchange 作为主要卖点，但 CiteLoop 不应做开放式互链池。CiteLoop 应做更安全的 partner citation/link opportunity：

- 只推荐真实相关站点。
- 必须有人工或 verified partner 关系。
- 不自动把用户链接塞进无关文章。
- 记录 anchor、目标页、上下文、风险。
- 对低质量来源直接过滤。

### 15.3 功能需求

1. Citation opportunity 来源：
   - customer partner pages
   - directories
   - integration marketplace
   - comparison pages
   - guest post targets
   - community/resource pages
2. 每个 opportunity 包含：
   - source domain
   - relevance reason
   - suggested target page
   - anchor/context
   - risk level
   - outreach draft
3. 不自动发布外链，只生成任务或 draft。

### 15.4 验收标准

1. 系统能为一个项目生成至少 10 个 safe citation opportunities。
2. 每个 opportunity 有 relevance reason 和 risk level。
3. 用户可以 mark contacted、accepted、rejected。
4. 被拒绝来源进入 suppression list。
5. 不允许自动互链发布。

## 16. Phase 排期总览

以下时间是乐观估计，适用于小团队连续投入；如果同一人同时承担产品、实现、验证和上线，Phase 2 到 Phase 6 需要顺延。

| Phase | 时间 | 名称 | 核心产出 | 依赖 |
|---|---:|---|---|---|
| 0 | 0-2 周 | Domain-first onboarding | domain 创建、setup checklist、connection health | 现有 auth/project/crawl |
| 0.5 | 1-3 周 | Publisher Connection Foundation | per-project GitHub/Next.js publisher、capability schema、secret isolation | Phase 0、现有 publisher |
| 1 | 2-5 周 | Growth Plan / Calendar | 30 天 plan、calendar、item evidence | Phase 0、topics、SEO brief |
| 2 | 3-8 周 | Article Improvements Inbox | improvement candidates、diff、editor/reviser repair loop、review/auto-push | Phase 0.5、GSC、content_actions |
| 3 | 5-10 周 | Publisher Integrations | Webhook、WordPress、GitHub UI config | Phase 0.5、notification |
| 4 | 7-12 周 | Outcome Attribution | action before/after、confidence、measurement checkpoints | GSC/GA4 sync、seo_experiments、content_actions |
| 5 | 10-16 周 | Free Tools Builder MVP | deterministic tool ideas、interactive preview、static publish | Phase 0.5、publisher capability |
| 6 | 12-18 周 | API/CLI | API keys、audit events、docs、CLI | authz/audit/rate limits |
| 7 | 16-28 周 | Safe Citation Opportunities | partner citation tasks | crawler/search/GEO data |

## 17. 数据模型原则与补充

### 17.1 原则：不另起一套任务系统

本路线图必须沿用 SEO loop，不新增平行的 `growth_plan_items`、`content_improvements`、`growth_outcomes` silo。

| 产品概念 | 复用或扩展对象 | 说明 |
|---|---|---|
| Growth Plan / Calendar | `seo_action_plans` + topics backlog + `seo_opportunities` | Plan 是已有 opportunity/topic 的编排视图。只有当现有表无法表达 scheduling 时，才加轻量 join/metadata，不复制 status/outcome。 |
| Article Improvements | `content_actions` | 用 action type / payload / diff 字段承载 improvement，不新建 `content_improvements`。 |
| Outcome Attribution | `seo_experiments` + `content_actions.outcome_summary` | baseline、measurement window、confidence 和 observed impact 复用实验/动作记录。 |
| GEO visibility actions | `seo_opportunities` + `content_actions` | 与 GEO PRD 保持一致，GEO 是 evidence/source，不是第二套 task。 |

### 17.2 `project_onboarding_state`

如果当前 onboarding 迁移和 project 字段不足以表达 checklist 状态，再新增该模型；否则优先扩展现有 onboarding 状态。

- `project_id`
- `input_domain`
- `canonical_domain`
- `mode`
- `current_step`
- `checklist`
- `discovery_summary`
- `last_checked_at`

### 17.3 `publisher_connections`

- `project_id`
- `kind`
- `status`
- `capabilities`
- `capability_schema_version`
- `credential_ref`
- `config`
- `oauth_access_expires_at`
- `oauth_refresh_status`
- `revoked_at`
- `last_verified_at`
- `last_error`

Credential 要求：

- raw token、webhook secret、OAuth refresh token encrypted at rest。
- credential scope 绑定 project 和 publisher connection。
- 前端、logs、runs、notifications、API response 不返回 secret。
- Webhook publisher 使用 signed payload、idempotency key 和 replay 防护。

### 17.4 `content_actions` 扩展

- `action_type`
- `source_opportunity_id`
- `target_url`
- `original_snapshot`
- `proposed_snapshot`
- `structured_diff`
- `qa_state`
- `repair_state`
- `repair_attempts`
- `repair_max_attempts`
- `human_decision_options`
- `publisher_connection_id`
- `publish_policy`

### 17.5 `seo_experiments` / outcome 扩展

- `baseline_window`
- `measurement_window`
- `metrics_before`
- `metrics_after`
- `outcome_label`
- `confidence`
- `attribution_method`
- `inconclusive_reason`
- `measured_at`

### 17.6 `tool_assets`

- `project_id`
- `source_action_id`
- `type`
- `input_schema`
- `deterministic_rules`
- `content`
- `published_url`
- `status`
- `usage_counter`

MVP 不存储 LLM-backed runtime 配置。后续如果支持公开 LLM 工具，需要新增 runtime connection、visitor rate limit 和 abuse audit。

### 17.7 `api_keys`

- `project_id`
- `subject_type`
- `subject_id`
- `name`
- `hash`
- `scopes`
- `rate_limit`
- `last_used_at`
- `revoked_at`
- `created_by`

### 17.8 `api_audit_events`

- `project_id`
- `subject_type`
- `subject_id`
- `api_key_id`
- `scope`
- `method`
- `path`
- `status_code`
- `request_id`
- `idempotency_key`
- `ip_hash`
- `user_agent`
- `created_at`

## 18. 产品指标

### 18.1 Activation

- 用户从 domain 创建到完成 public crawl 的比例。
- 用户完成至少一个 publisher connection 的比例。
- 用户生成第一份 growth plan 的时间。

### 18.2 Execution

- 每周 generated plan items。
- 每周 approved/published actions。
- 自动修复成功率。
- Human decision rate。
- Publish failure rate。

### 18.3 Outcome

- Actions with measured outcome ratio。
- Improved / neutral / worsened / inconclusive 分布。
- High-confidence outcome ratio。
- Search clicks/impressions trend。
- GEO mention/citation trend。
- Tool usage 和 tool-driven conversion。

### 18.4 Trust

- 用户手动 reject rate。
- Auto-push rollback rate。
- Safe mode triggers。
- Notification delivery success rate。

## 19. 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| 用户误以为只输入 domain 就能读取私有 GSC 数据 | 信任和合规风险 | 明确 public-only 降级和授权流程 |
| 客户 CMS 密钥泄漏或跨租户混用 | 严重安全风险 | encrypted at rest、project-scoped credential、secret redaction、OAuth revoke |
| AI improvement 改坏已发布文章 | 搜索和品牌风险 | diff review、risk policy、snapshot rollback |
| 多 CMS 发布适配复杂 | 交付变慢 | 先做 Webhook 和 WordPress，capability-driven abstraction |
| Outcome 被误读成确定因果 | 信任风险 | confidence threshold、trend normalization、inconclusive 默认保守 |
| Free Tools 生成不可靠或被滥用 | 用户体验和成本风险 | 第一版 deterministic/static，不做公开 LLM endpoint |
| API/CLI 滥用 | 成本和安全风险 | scopes、rate limit、audit、project isolation |
| Citation/link 变成低质互链 | SEO 风险 | 不做开放 exchange，只做 verified opportunity 和人工任务 |

## 20. 各 Phase 总验收门槛

### 短期完成门槛

1. 一个新用户只输入 domain 可创建 project。
2. 用户能看到 setup checklist 和当前 capability mode。
3. 至少 GitHub/Next.js publisher 可以 per-project 配置，且 capability/secret 不依赖全局 env。
4. 用户能生成 30 天 growth plan。
5. 用户能 review 新文章和 article improvement。
6. QA blocking draft 自动进入有上限的 AI repair loop。
7. Webhook 或 WordPress 至少一个 publisher 可以真实发布。
8. 所有高风险动作进入人工 review。

### 中期完成门槛

1. 系统能自动发现、生成、发布、测量至少 3 类 SEO actions。
2. 每个 published action 有 before/after outcome、confidence 和 inconclusive 解释。
3. Free Tools Builder 能生成并发布 deterministic 模板化工具页。
4. 用户可以通过 API key 和 CLI 完成只读运营流程。
5. Low-risk auto-push 有 policy、audit 和 rollback。

### 长期完成门槛

1. CiteLoop 支持多 project、多 publisher、多用户角色。
2. Agent/CLI 可以安全编排运营流程。
3. GEO visibility observations 能进入 planner 和 outcome attribution。
4. Safe citation opportunities 能作为 off-page 任务进入 plan。
5. Billing/usage/limits 能约束真实 SaaS 使用。

## 21. 推荐执行顺序

最优路径不是先做 Free Tools 或 API，而是先把当前 CiteLoop 已有能力产品化：

1. **先做 Domain-first onboarding**，降低接入摩擦。
2. **马上做 Publisher Connection Foundation**，否则 review、auto-push 和工具发布都没有安全落点。
3. **再做 Growth Plan**，让用户理解系统在帮他做什么。
4. **接 Article Improvements**，补上 editor/reviser agent 和有上限的 QA repair loop。
5. **并行扩展 Publisher integrations**，让自动运营能落地到真实客户站点。
6. **随后做 Outcome Attribution**，用保守、有置信度的结果反哺 planner。
7. **再做 Free Tools Builder**，先做 deterministic/static 工具，扩展增长面。
8. **最后做 API/CLI 和 safe citation**，把平台能力开放出去。

这条路线最大化复用现有代码，同时最早解决真实用户“我能不能上手、能不能发布、能不能看到收益”的核心疑问。
