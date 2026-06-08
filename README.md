# CiteLoop

SEO + GEO automated content engine (PRD: `docs/PRD-CiteLoop-MVP-v2.md`).
Single-tenant run, multi-tenant ready. Everything is `project`-scoped; publishing
goes through `Publisher`, LLM through `LLMProvider`, search through `SearchProvider`.

## Architecture (maps to PRD §4)

```
landing URL
  → Insight Agent    (crawl within bounds → Product Profile + Content Inventory)
  → Strategist Agent (gap analysis + search → Topic Backlog)
  → Scheduler        (daily cron, advisory lock, budget breaker → generate)
  → Writer + QA      (canonical + per-platform variants; evidence-mapping gate)
  → Review queue     (the only human gate: approve / edit / reject)
  → Publisher        (blog lane: auto-commit MDX; syndication lane: gated unlock)
```

## Layout

| Path | PRD | What |
|---|---|---|
| `internal/migrations` | §3 | DDL with all constraints; embedded + applied at startup |
| `internal/db` | §3 | sqlc-generated queries (pgx/v5) |
| `internal/config` | §3 | process env + `projects.config` (crawl bounds, budget, cadence) |
| `internal/crawl` | §5.1 | bounded same-origin crawler (sitemap, robots, normalize, classify) |
| `internal/llm` | §4 | `LLMProvider`: TokenGate/OpenAI-compatible gateway, Claude fallback, deterministic mock |
| `internal/search` | §4/§5.2 | `SearchProvider`: real Brave + mock; quota + degrade |
| `internal/platform` | §4/§5.3 | platform canonical-capability registry |
| `internal/agents` | §5.1–5.3 | Insight / Strategist / Writer / QA |
| `internal/scheduler` | §5.4 | cron tick, `pg_advisory_xact_lock`, skip-locked, cost breaker |
| `internal/publisher` | §5.6/§8 | BlogPublisher (MDX auto-commit) + SemiManual (gated) |
| `internal/api` | §5.5 | Chi HTTP API incl. review queue |
| `cmd/api` | — | entrypoint: migrate → seed → wire providers → cron → serve |
| `web` | §5.5 | Next.js dashboard UI |

## Run locally

```bash
cp .env.example .env          # optionally add TOKENGATE_API_KEY / SEARCH_API_KEY / CLERK_SECRET_KEY
make db-up                    # throwaway Postgres on :5432
make run                      # migrates, seeds placeholder project, serves :8080

cd web && npm install
cp .env.production.example .env.local
# set NEXT_PUBLIC_API_URL=http://localhost:8080 plus Clerk frontend vars
npm run dev                   # UI on :3000
```

Without API keys the service uses **mock** LLM/search providers and the
BlogPublisher runs in **dry-run** (logs + computes URL, no commit), so the whole
pipeline runs end-to-end offline.

For real text generation through TokenGate, prefer the OpenAI-compatible chat-completions path:

```bash
TOKENGATE_API_KEY=sk-...
TOKENGATE_BASE_URL=https://tokengate-production.up.railway.app/v1
TOKENGATE_MODEL=claude-haiku-4-5-20251001
```

`TOKENGATE_BASE_URL` must be the Railway backend `/v1` API base, not the
TokenGate Vercel dashboard URL.

## Deploy frontend to Vercel

Deploy `web/` as the Vercel project root directory. The frontend only needs one
production API URL plus Clerk's publishable frontend configuration:

```bash
NEXT_PUBLIC_API_URL=https://<railway-api-domain>
NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY=pk_test_or_live_value
NEXT_PUBLIC_CLERK_SIGN_IN_URL=/sign-in
NEXT_PUBLIC_CLERK_SIGN_UP_URL=/sign-up
NEXT_PUBLIC_CLERK_SIGN_IN_FALLBACK_REDIRECT_URL=/
NEXT_PUBLIC_CLERK_SIGN_UP_FALLBACK_REDIRECT_URL=/
```

Set `CLERK_SECRET_KEY` in Vercel for Clerk middleware and in the Railway API
service for backend Bearer-token verification. Do not commit this key.

For SEO data ingestion, set `GOOGLE_SERVICE_ACCOUNT_JSON` in the Railway API
service secret store. CiteLoop stores only the credential reference in Postgres;
the service account needs read access to Search Console and GA4.

Recommended CLI flow after logging in to Vercel:

```bash
vercel link --cwd web
vercel deploy --cwd web
vercel deploy --cwd web --prod
```

For Git integration, set the Vercel Project Root Directory to `web`, Framework
Preset to Next.js, Build Command to `npm run build`, and Install Command to
`npm install`.

## Providers (decisions baked in)

- **LLM/Search:** TokenGate's OpenAI-compatible gateway is preferred when
  `TOKENGATE_API_KEY` is set (`TOKENGATE_BASE_URL` + `TOKENGATE_MODEL` control
  routing). Claude remains available through `ANTHROPIC_API_KEY` as a fallback;
  mock fallback is used when neither LLM key is set. Search defaults to Brave —
  swap the concrete in `internal/search` to use Tavily/Serper/etc. behind the
  same interface.
- **Publisher:** §8 **option A** — write MDX into the blog repo and auto-commit
  to `BLOG_BRANCH` via the GitHub Contents API (`GITHUB_TOKEN`, `BLOG_REPO`).
  Generated posts live under `BLOG_CONTENT_DIR` (`content/citeloop/blog` by
  default). If UniPost reads generated content at build time, configure
  `UNIPOST_DEPLOY_HOOK_URL` so CiteLoop can trigger the dev/prod rebuild after
  each commit; without it articles remain `pending_url_verification` until an
  external deploy makes the URL return `2xx`. The app-internal approve is the
  only human gate; there is no second merge step.

## Tests

`make test` — pure-logic units (URL normalization, article classification,
robots, config-zero handling, JSON extraction, numeric round-trip). The
end-to-end pipeline was validated against a live Postgres (see PRD acceptance).
# citeloop
