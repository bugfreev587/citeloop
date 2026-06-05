# PRD：CiteLoop — SEO + GEO 自动内容引擎（MVP / V2 草案）

> 本版基于对 v1 的 review 修订。Claude Code 可直接执行：每模块附验收 checklist，末尾附 PR 拆分与工时。

### v1 → v2 变更摘要（修订依据 review）

1. **发布闸门去矛盾**：app 内的 approve 是唯一人工闸门;到点 Publisher **自动 commit / 自动 merge** = 真自动发布,去掉"等你 merge"。(§1.5 / §4 / §5.6 / §8)
2. **爬虫加边界**：博客发现/抓取补 same-origin、max pages/depth、超时、限速、robots、canonical 归一化、sitemap 上限、"全部"的明确定义。(§5.1)
3. **排期单一真相**：新增 `articles.scheduled_at`,状态机显式化。(§3 / §5.5 / §5.6)
4. **QA 改为证据映射**:每条产品声明必须映射到 profile/inventory 证据,映射不上即 blocking。(§5.3)
5. **新增 `SearchProvider`** 接口 + 配额 + 结果落库 + 失败降级。(§4 / §5.2)
6. **补全 DDL 约束** + cron 幂等/加锁。(§3 / §5.4)
7. **收紧 syndication 解锁**:`ready_to_distribute` 必须等 canonical 真实 URL 回填后。(§5.6)
8. **拆 PR1**(发现/抓取 vs profile/inventory+UI),工时上调。(§9)
9. **新增成本熔断**:per-project 月度预算上限。(§3 config / §5.4)
10. **canonical 适用范围修正**:论坛/聚合(Reddit/HN)不支持 rel=canonical,只在正文放源链接;仅 Medium/Dev.to/Hashnode/LinkedIn 设 canonical 标签。(§4 / §5.3 / §5.6)

---

## 0. 背景与定位

- **短期目标**：为 UniPost 做 SEO + GEO 推广 —— 自动产出内容、发到 UniPost 博客（SEO 正本）、并站点分发到第三方平台（GEO 触达）。
- **长期目标**：沉淀为独立 SaaS（多租户），为任意产品做 SEO + GEO 内容运营。
- **设计原则**：MVP 即按 **「单租户运行、多租户就绪」** 设计。所有业务实体 `project` 作用域；发布走 `Publisher` 接口；LLM 走 `LLMProvider`；搜索走 `SearchProvider`。
- **部署形态**：独立服务 —— 独立 Railway project + 独立 Postgres + 独立 Go API。仅在「社交分发」子集上消费 UniPost 公共 API（V1 不实现自动发，见非目标）。

---

## 1. 目标（Goals）

1. **仅输入 landing page URL**，服务在**受控边界内**自动发现并抓取站内博客文章（见 §5.1 上限定义），产出结构化「产品认知」与「存量内容清单」，且**可人工编辑**。
2. 基于产品认知 + 存量内容做缺口分析，产出带内链建议与排期的**选题 backlog**，区分 SEO（博客）与 GEO（站点分发）。
3. 按排期自动生成内容：博客 canonical 正本 + 各分发平台的改写版本，写作时套用 GEO 优化策略。
4. **发布前强制人工审核**：自动备货 → 审核队列 → **app 内 approve（唯一人工闸门）** → 到点自动发布。
5. **blog lane 真·自动发布**（approve 后到点由 Publisher 自动落地，无需二次人工 merge）；站点分发 lane V1 半自动（生成改写稿 + 复制/跳转，且仅在 canonical 发布后解锁）。

---

## 2. 非目标（Non-Goals）— V1 明确不做

- ❌ 多租户注册 / 计费 / 团队管理（schema 预留，但不做注册流与 billing）。
- ❌ Analytics / 反馈闭环（排名、AI 引用、SoV 追踪）—— 留 V2。
- ❌ 第三方平台（Dev.to / Hashnode / Reddit）的**自动发布** —— V1 仅生成改写稿，人工发。
- ❌ 经 UniPost API 的社交**自动发布** —— fast-follow，不在 V1。
- ❌ **通用任意站点的高鲁棒爬虫**——V1 只针对 UniPost 自有博客,用保守上限(§5.1);面向任意站点的完整爬虫鲁棒性是 **SaaS 阶段的 P0**,不在本版。
- ❌ 配图 / 图像生成、A/B 测试、多语言。
- ❌ 付费 SEO 数据源采购（V1 用 LLM + 搜索估算即可）。

---

## 3. 核心概念与数据模型

### 概念

| 概念 | 说明 |
|---|---|
| Project | 一个被推广的产品。MVP = UniPost 一条。所有表挂 `project_id`。 |
| Product Profile | AI 抽取的产品认知，版本化、可编辑、单 project 仅一个 active。 |
| Content Inventory | 存量 + 已生成文章清单，供去重、内链、语气一致性、**QA 证据映射**使用。 |
| Topic | 选题，带 `channel`、目标关键词/AI prompt、内链建议、排期。 |
| Article | 产出物，`kind`=canonical 或 syndication_variant（带 `platform`）。 |

### Postgres DDL（sqlc 友好；含约束）

```sql
create table projects (
  id          uuid primary key default gen_random_uuid(),
  owner_id    text not null,
  name        text not null,
  slug        text not null unique,
  config      jsonb not null default '{}',   -- 见下方 config schema
  created_at  timestamptz not null default now()
);

create table product_profiles (
  id          uuid primary key default gen_random_uuid(),
  project_id  uuid not null references projects(id),
  source_urls jsonb not null default '[]',
  profile     jsonb not null,
  version     int not null default 1,
  is_active   boolean not null default true,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now()
);
-- 单 project 仅一个 active profile
create unique index one_active_profile_per_project
  on product_profiles (project_id) where is_active;

create table content_inventory (
  id             uuid primary key default gen_random_uuid(),
  project_id     uuid not null references projects(id),
  url            text not null,            -- 已 canonical 归一化（见 §5.1）
  title          text,
  target_keyword text,
  topics         jsonb not null default '[]',
  summary        text,
  source         text not null default 'existing'
                 check (source in ('existing','generated')),
  captured_at    timestamptz not null default now(),
  unique (project_id, url)                 -- 去重靠归一化后的 url
);

create table topics (
  id              uuid primary key default gen_random_uuid(),
  project_id      uuid not null references projects(id),
  channel         text not null check (channel in ('blog','syndication','both')),
  title           text not null,
  target_keyword  text,
  target_prompt   text,
  angle           text,
  format          text,
  priority        int  not null default 0,
  internal_links  jsonb not null default '[]',
  status          text not null default 'backlog'
                 check (status in ('backlog','scheduled','generating','drafted','done','archived')),
  scheduled_at    timestamptz,             -- 计划发布时间（排期意图）
  created_at      timestamptz not null default now()
);

create table articles (
  id             uuid primary key default gen_random_uuid(),
  project_id     uuid not null references projects(id),
  topic_id       uuid not null references topics(id),
  kind           text not null check (kind in ('canonical','syndication_variant')),
  platform       text,                     -- canonical 时为 null
  content_md     text not null,
  seo_meta       jsonb not null default '{}',
  geo_score      numeric,
  seo_score      numeric,
  qa_issues      jsonb not null default '[]',
  qa_blocking    boolean not null default false,  -- 有未解决的 blocking 证据问题
  canonical_url  text,                     -- 真实 URL，仅在 canonical 发布后回填
  status         text not null default 'generating'
                 check (status in ('generating','pending_review','approved',
                                    'scheduled','published',
                                    'ready_to_distribute','distributed','rejected')),
  scheduled_at   timestamptz,              -- 本产出物的发布时间（真相来源，见状态机）
  reviewed_by    text,
  reviewed_at    timestamptz,
  published_at   timestamptz,
  publish_result jsonb,
  created_at     timestamptz not null default now(),
  -- canonical 必须无 platform；variant 必须有 platform
  check ((kind='canonical' and platform is null)
      or (kind='syndication_variant' and platform is not null))
);
-- 同一 topic 下，每种 (kind, platform) 至多一篇（平台无关 canonical 用空串占位）
create unique index uniq_article_topic_kind_platform
  on articles (topic_id, kind, coalesce(platform,''));

create table generation_runs (   -- 可观测性 + 成本审计（熔断依据）
  id          uuid primary key default gen_random_uuid(),
  project_id  uuid not null references projects(id),
  agent       text not null check (agent in ('insight','strategist','writer','qa')),
  input       jsonb,
  output      jsonb,        -- 含搜索结果快照（见 §5.2）
  model       text,
  tokens      int,
  cost_usd    numeric,
  status      text not null check (status in ('ok','error')),
  error       text,
  created_at  timestamptz not null default now()
);
```

### `projects.config` schema

```jsonc
{
  "cadence_per_week": 3,
  "buffer_days": 5,
  "channel_mix": { "blog": 0.6, "syndication": 0.4 },
  "brand_voice": "…",
  "monthly_budget_usd": 50,          // 成本熔断上限（§5.4）
  "crawl": {                          // 爬虫边界（§5.1）
    "same_origin_only": true,
    "max_pages": 200,
    "max_depth": 3,
    "request_timeout_ms": 8000,
    "rate_limit_rps": 1,
    "respect_robots": true,
    "sitemap_url_cap": 2000
  }
}
```

### 状态机（显式）

- `topics.status`：`backlog → scheduled → generating → drafted → done | archived`
- `articles.status`（**canonical**）：`generating → pending_review → approved →(写入 scheduled_at)→ scheduled → published | rejected`
- `articles.status`（**syndication_variant**）：`generating → pending_review → approved →（等所属 topic 的 canonical 进入 published 且 canonical_url 回填）→ ready_to_distribute → distributed | rejected`

**scheduled_at 单一真相**：`scheduled_at` 以 **articles** 为准。approve canonical 时写入：若所属 `topic.scheduled_at` 非空则继承，否则取 `now() + buffer_days`。`topics.scheduled_at` 仅表达"排期意图",不直接驱动发布。

---

## 4. 系统架构 / Agent 流水线

```
landing URL（仅此一项输入）
   → Insight Agent      → 受控爬取（同源/限深/限页/限速/robots）→ 全部文章
                        → Product Profile + Content Inventory（可编辑）
   → Strategist Agent   → Topic Backlog（带 channel / 内链 / 排期）
   → [Scheduler 提前 buffer_days 触发缺内容的槽位；带锁+预算熔断]
   → Writer Agent       → canonical 正本 + 各平台改写版
   → QA（证据映射 + 评分）→ blocking → 进审核队列；非 blocking → 自动修正一次
   → 人工审核闸门（唯一）  → approve / edit / reject
   → Publisher
        ├─ blog lane         （自动）到点自动 commit/merge → UniPost 博客（canonical）
        └─ syndication lane  （半自动）canonical 发布并回填 URL 后才解锁改写稿
```

**canonical 适用范围(重要修正)**:仅 Medium / Dev.to / Hashnode / LinkedIn 文章支持设 `rel=canonical`。**Reddit / Hacker News 等论坛聚合类不支持**,只能在正文放指向正本的源链接。spec 按平台区分处理。

**Agent 定性**：只有 Insight 是真正 agentic（自主抓取决策）;Strategist 需搜索;Writer/QA 是确定性 LLM 步骤,不做自主循环 agent。

**接口抽象**

```go
type LLMProvider interface {
    Complete(ctx context.Context, req CompletionReq) (CompletionResp, error)
}

type SearchProvider interface {                 // 新增
    Search(ctx context.Context, q SearchQuery) ([]SearchResult, error)
}
// SearchResult: { Title, URL, Snippet, Source, FetchedAt }

type Publisher interface {
    Platform() string                            // "blog" | "dev_to" | "reddit" | ...
    Mode() PublishMode                           // Auto | SemiManual
    SupportsCanonical() bool                     // 区分能否设 canonical 标签
    Publish(ctx context.Context, a *Article) (PublishResult, error)
}
```

V1 实现 `BlogPublisher`（Auto, SupportsCanonical=true）;分发平台用 `SemiManualPublisher`（仅返回改写稿与跳转链接）。

---

## 5. 功能需求（按模块）+ 验收标准

### 5.1 Insight Agent（认知）

**输入**：**仅 landing page URL**。
**博客发现 + 抓取边界（全部受 `config.crawl` 约束）**：
1. **发现**：`robots.txt → sitemap.xml`（递归 sitemap index，**总 URL 数 ≤ `sitemap_url_cap`**，超限按 `lastmod` 倒序取前 N 并记 `truncated=true`）;无 sitemap 时回退爬 `/blog`、`/posts`、`/articles`、`/changelog` 等索引并跟随分页(**受 `max_pages` / `max_depth` 限**)。
2. **边界**：**仅同源**（same_origin_only）、`max_pages`、`max_depth`、`request_timeout_ms`、`rate_limit_rps`、遵守 `robots`。
3. **"全部"的定义**：= **去重归一化后、判定为"文章"页、且落在上述上限内的集合**。URL 归一化:去 query/fragment(保留必要参数)、去末尾斜杠、强制小写 host、跟随 `<link rel=canonical>`。分页/标签/作者页判为非文章,排除。
4. **抓取 + 抽取**：抓 landing + 关键页 + 文章页 → LLM 产出 `profile` 与逐篇 `inventory`。

**Profile schema**：`positioning, value_props[], features[], icp[], tone, key_terms[], competitors[], differentiators[]`
**Inventory（每篇）**：`{url, title, target_keyword, topics[], summary, evidence_snippets[]}`（`evidence_snippets` 供 QA 证据映射）

验收：
- [ ] 仅给定 landing URL 即自动发现并抓取站内文章，inventory 条数 = 实际文章数（在上限内）。
- [ ] 所有 crawl 边界生效:越界不抓;命中 `sitemap_url_cap` 时标 `truncated`。
- [ ] 同源校验生效;robots 禁止的路径不抓。
- [ ] URL 归一化 + 去重正确;标签/作者/分页索引被排除。
- [ ] 单篇失败跳过并记日志,不阻塞整体;landing 抓取失败才中止报错。
- [ ] 重跑生成新 profile version、旧版保留、新版 active（受 `one_active_profile_per_project` 约束）。
- [ ] 写入 `generation_runs`（agent=insight）。

### 5.2 Strategist Agent（选题）

**输入**：active Profile + Inventory。
**行为**：缺口分析 + 关键词/AI-prompt 研究（经 `SearchProvider`）+ 内链规划 → `topics`。
**搜索数据契约**：
- 走 `SearchProvider` 接口;V1 用一个具体实现(可复用现有搜索能力)。
- **配额**:单次选题 run 的搜索调用数上限(默认 ≤ 10),计入成本。
- **结果落库**:本次用到的 `SearchResult[]` 快照写入 `generation_runs.output.search`。
- **失败语义**:搜索失败 → **降级为纯 LLM 选题**并在 run 上标 `degraded=true`(不整步失败);连续失败才告警。

验收：
- [ ] 一次 run 产出 ≥ 10 条 topic，字段齐全（channel / 目标词或 prompt / angle / format / priority）。
- [ ] 与 inventory 全部已有文章去重通过。
- [ ] 部分 topic 带 `internal_links`。
- [ ] 搜索结果快照落入 `generation_runs.output`;搜索失败时降级且标记 `degraded`。

### 5.3 Writer + QA（生成 → 证据映射 + 评分）

**Writer**：
- canonical：完整文章 + SEO on-page（title/meta/slug/H/内链/schema）+ GEO 策略（自包含块、统计、引用、Q&A、权威语气）。
- syndication_variant：按 `platform` 改写。**canonical 字段处理按 `Publisher.SupportsCanonical()`**:支持的平台带 `canonical_url`(发布前为待回填占位);**Reddit/HN 等不支持的,改为正文内放源链接,不写 canonical 标签**。

**QA（双层,证据映射是真闸门）**：
1. **证据映射(blocking 闸门)**:抽取文中所有**关于产品的事实性声明**,每条必须能映射到 `profile.features` / `source_urls` / `inventory.evidence_snippets` 之一。**映射不上的声明 → 写入 `qa_issues` 且置 `qa_blocking=true`**,不可自动通过,强制进人工审核。
2. **LLM 评分(润色,非闸门)**:GEO 分 / SEO checklist / 品牌语气 / 原创度 → `geo_score` / `seo_score`;低于阈值自动修正一次。

验收：
- [ ] blog topic → 1 篇 canonical（完整 `seo_meta`）。
- [ ] syndication topic → canonical + 每平台一条 variant;variant 的 canonical 处理按平台能力区分（支持则带占位 canonical_url,不支持则正文源链接）。
- [ ] **证据映射生效**:故意让 Writer 编造一个 UniPost 不存在的功能 → 该声明被标 `qa_blocking=true`,文章不自动通过。
- [ ] 每篇产出 `geo_score` / `seo_score`,审核界面可见。
- [ ] 写入 `generation_runs`（writer / qa 各一条）。

### 5.4 Scheduler（自动运营核心）

**机制**：Go cron 每日 tick。对每个 project，检查未来 `buffer_days` 内排期槽是否已有 approved 内容;缺则触发生成 → QA → 入审核队列并通知。
**幂等 + 加锁**：
- 每次 tick 对 project 取 `pg_advisory_xact_lock(project_id)`,防并发重复触发。
- 取候选 topic 用 `SELECT … FOR UPDATE SKIP LOCKED`;生成前检查该 topic 是否已存在非 rejected 的 article,有则跳过(配合 `uniq_article_topic_kind_platform` 兜底)。
**成本熔断**：生成前累计本月 `generation_runs.cost_usd`;若 ≥ `config.monthly_budget_usd` → **跳过生成 + 告警**,不再调用 LLM。
**关键约束**：生成与发布解耦,留 buffer;**未审核内容绝不自动发布**;审核超时 → hold + 再通知。

验收：
- [ ] cadence / buffer_days 可配置;缺内容槽自动备货,已 approved 槽不重复生成。
- [ ] 并发 tick 不产生重复 article（advisory lock + skip locked + 唯一索引共同保证）。
- [ ] 月度成本达上限后停止生成并告警;次月恢复。
- [ ] 生成失败可重试,连续失败告警,不阻塞其他槽。

### 5.5 审核队列 UI（唯一人工闸门）

验收：
- [ ] 列出 `pending_review` 文章:标题、channel/platform、geo/seo 分、`qa_issues`(blocking 高亮)。
- [ ] **approve**：canonical → `approved` 并**写入 `articles.scheduled_at`**(继承 topic 或 `now()+buffer`);variant → `approved`(等 canonical 发布后才解锁)。
- [ ] **edit**：可改 `content_md` / `seo_meta` 后保存;**有 `qa_blocking` 的必须先解决才能 approve**。
- [ ] **reject**：→ `rejected`,topic 回 backlog 或 archive。
- [ ] canonical 与其 variants 同视图分组;审核动作写 `reviewed_by` / `reviewed_at`。

### 5.6 Publisher

**blog lane（Auto，真自动）**：到达 `articles.scheduled_at` 且 `status=approved` 的 canonical → `BlogPublisher.Publish()`：
- 实现按 §8 选型(MDX-in-repo 则**自动 commit 到发布分支 / 自动 merge**,或调 CMS/API)。**不需要人再去 GitHub merge**。
- 成功 → `published`,回填 `published_at` / `publish_result` / **真实 `canonical_url`**,并把该文加入 inventory(source=generated)。

**syndication lane（SemiManual / V1）**：
- variant 在 `approved` 后**保持等待**;**仅当所属 canonical 进入 `published` 且真实 `canonical_url` 回填后**,variant 才转 `ready_to_distribute`。
- 此时回填 variant 的 canonical（支持的平台写标签值,不支持的把正文源链接替换为真实 URL）。
- UI 提供"复制改写稿 + 打开目标平台撰写页";用户手动发后标 `distributed`。

验收：
- [ ] canonical 到点**自动发布**(无二次人工 merge),真实 URL 回填。
- [ ] **变体绝不会在 canonical 发布前进入 `ready_to_distribute`**(防止拿着失效 canonical 去发)。
- [ ] 不支持 canonical 的平台(Reddit/HN)产出物正文带真实源链接,不含 canonical 标签。
- [ ] 发布失败有重试与报错,不误标 published。
- [ ] 已发布 canonical 自动进 inventory。

---

## 6. 可扩展性 / SaaS 预留设计

| 预留点 | MVP 做法 | 转 SaaS 时 |
|---|---|---|
| 多租户 | 全表 `project_id` + `owner_id`，单 project 运行 | 加注册/onboarding/billing，无需改 schema |
| 发布平台 | `Publisher` 接口 + BlogPublisher | 加 Dev.to / Hashnode / UniPostSocial 适配器 |
| LLM / 搜索 | `LLMProvider` / `SearchProvider` 接口 | 多模型/多搜索路由 |
| Channel/Platform | text + **CHECK 约束**（应用层集合，非用户自定义） | 改 CHECK + 迁移即可加平台 |
| 配置 | `projects.config` jsonb | 直接成为租户级设置 |
| 爬虫 | 保守上限的同源爬虫 | **补全任意站点鲁棒性（SaaS P0）** |
| 成本 | per-project 月度预算熔断 | 接入 billing 配额 |

---

## 7. 技术栈

- 后端：Go + Chi，Railway，PostgreSQL + sqlc，cron（robfig/cron 或现有方案），`pg_advisory_xact_lock`。
- 前端：Next.js 14+ App Router + TypeScript + Tailwind v4 + shadcn/ui + Clerk，Vercel。
- LLM / 搜索：经 `LLMProvider` / `SearchProvider`，默认 Claude + 现有搜索能力。
- 抓取：HTTP fetch + 正文抽取（readability 类库），受 `config.crawl` 约束。
- 通知：MVP 用邮件（或站内 badge）。

---

## 8. 开放决策（需你拍板）

**UniPost 博客内容当前如何存储？** 决定 `BlogPublisher` 的实现(注意:**两种都做成"自动发布",app 内 approve 已是唯一人工闸门**):

- **A. Next.js 仓库内 MDX** → Publisher 自动写 MDX 并 **commit 到发布分支 / 自动 merge PR**（GitHub API + 受限 token）。
- **B. Headless CMS / DB + API** → Publisher 调对应发布接口。

默认 **A**,但**自动落地**(不再"等你 merge")。若你担心自动改仓库的风险,可让 Publisher 提交到一个**受保护的 `content/` 路径 + 单独发布分支并自动合并**,审计留在 git 历史里。接口已抽象,选型不影响其余模块。

---

## 9. 工时与 PR 拆分（已按 review 拆 PR1）

| PR | 范围 | 估时 |
|---|---|---|
| PR1a | 服务脚手架 + schema/sqlc（含全部约束）+ **博客发现/抓取**（边界 + 归一化 + robots） | ~2 天 |
| PR1b | **Insight 抽取**（profile + inventory + evidence_snippets）+ project/profile/inventory CRUD UI | ~1.5 天 |
| PR2 | **Strategist** + `SearchProvider`（配额 + 落库 + 降级）+ Topic Backlog/排期 UI | ~2 天 |
| PR3 | **Writer + QA**（证据映射闸门 + 评分 + 按平台 canonical 处理）+ article 存储 | ~2.5 天 |
| PR4 | **Scheduler**（cron + 锁 + 幂等 + 预算熔断）+ **审核队列 UI** + 通知 | ~2 天 |
| PR5 | **Publisher**（BlogPublisher 自动发布 + syndication 解锁门控 + URL 回填 + 回灌 inventory） | ~1.5 天 |

**合计 ≈ 11.5 天**（强单人 + Claude Code 节奏）。比 v1 的 9 天上调,主要来自爬虫边界、证据映射、约束与锁、`SearchProvider` 这些被 review 点出的"做扎实"成本。建议顺序 PR1a→PR5,每个 PR 自带可演示切片。

---

## 10. V2+ 预告（不在本 PRD 范围）

- Analytics Agent：博客排名 + AI 引用 / SoV 追踪 → 回流重排选题、强化 Writer。
- 站点分发自动化：UniPost API（社交子集）+ Dev.to / Hashnode connector。
- 通用站点爬虫鲁棒性 + 多租户 onboarding + billing（SaaS 化）。
