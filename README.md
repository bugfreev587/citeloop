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
| `internal/llm` | §4 | `LLMProvider`: real Claude + deterministic mock |
| `internal/search` | §4/§5.2 | `SearchProvider`: real Brave + mock; quota + degrade |
| `internal/platform` | §4/§5.3 | platform canonical-capability registry |
| `internal/agents` | §5.1–5.3 | Insight / Strategist / Writer / QA |
| `internal/scheduler` | §5.4 | cron tick, `pg_advisory_xact_lock`, skip-locked, cost breaker |
| `internal/publisher` | §5.6/§8 | BlogPublisher (MDX auto-commit) + SemiManual (gated) |
| `internal/api` | §5.5 | Chi HTTP API incl. review queue |
| `cmd/api` | — | entrypoint: migrate → seed → wire providers → cron → serve |
| `web` | §5.5 | Next.js review-queue UI |

## Run locally

```bash
cp .env.example .env          # optionally add ANTHROPIC_API_KEY / SEARCH_API_KEY
make db-up                    # throwaway Postgres on :5432
make run                      # migrates, seeds placeholder project, serves :8080

cd web && npm install
NEXT_PUBLIC_API_URL=http://localhost:8080 npm run dev   # UI on :3000
```

Without API keys the service uses **mock** LLM/search providers and the
BlogPublisher runs in **dry-run** (logs + computes URL, no commit), so the whole
pipeline runs end-to-end offline.

## Providers (decisions baked in)

- **LLM/Search:** real implementations wired (`ANTHROPIC_API_KEY`, `SEARCH_API_KEY`);
  mock fallback when unset. Search defaults to Brave — swap the concrete in
  `internal/search` to use Tavily/Serper/etc. behind the same interface.
- **Publisher:** §8 **option A** — write MDX into the blog repo and auto-commit
  to a publish branch via the GitHub Contents API (`GITHUB_TOKEN`, `BLOG_REPO`).
  The app-internal approve is the only human gate; there is no second merge step.

## Tests

`make test` — pure-logic units (URL normalization, article classification,
robots, config-zero handling, JSON extraction, numeric round-trip). The
end-to-end pipeline was validated against a live Postgres (see PRD acceptance).
# citeloop
