# Context Refresh Controls Design

## Goal

Let users manually update Context from their connected domain without allowing domain changes, show when Context was last crawled, prevent repeated manual crawls within 24 hours, and run a weekly lightweight automatic refresh.

## Current Behavior

The Context page renders a domain input and "Refresh context" button even after Context is confirmed. That button calls `POST /projects/{id}/insight` with an arbitrary `landing_url`. The backend starts a crawl using that value. Context freshness is inferred from profile `updated_at`, which also changes when users edit profile fields and does not strictly mean a crawl completed.

## Product Behavior

- First setup may still accept a domain when no active profile exists.
- Once a project has a configured domain, manual updates must use `projects.config.site_url`.
- Users must not be able to change the service domain from the Context page or by posting a different `landing_url` to the Context refresh endpoint.
- The Context page shows "Last updated" next to the update button using the latest completed context crawl time.
- Manual update is allowed only when no crawl is running and the latest manual crawl is at least 24 hours old.
- Weekly automatic refresh runs in the scheduler using the same project domain and a lightweight crawl budget.
- Every completed manual or automatic crawl updates the crawl timestamp.
- Refreshing must preserve `context_confirmed_at` or `confirmed_at`.

## Architecture

Add explicit crawl metadata to the active product profile rather than overloading `updated_at`. The profile JSON will store:

- `context_last_crawled_at`: RFC3339 timestamp of latest completed context crawl.
- `context_last_manual_crawled_at`: RFC3339 timestamp of latest completed manual refresh.
- `context_crawl_started_at`: RFC3339 timestamp while a refresh is in progress.
- `context_crawl_source`: `manual`, `weekly`, or `onboarding`.

Add a new project-scoped endpoint `POST /projects/{id}/context/refresh` for manual Context updates. It reads `config.site_url`, enforces the 24-hour manual cooldown, marks the crawl as started, and starts a detached crawl. The existing `/insight` endpoint remains for first-run compatibility but must reject attempts to crawl a different URL once `config.site_url` exists.

Add scheduler method `TickContextRefresh(ctx)` and register it weekly. It lists projects, skips projects without `site_url` or confirmed Context, skips projects crawled within the last week, and starts a lightweight refresh using the project domain.

## UI

On the Context page after setup:

- Remove the editable domain input from the health panel.
- Show a compact action row with `Update context` and `Last updated <date>`.
- Disable `Update context` while busy, while a crawl is running, or during the 24-hour manual cooldown.
- Keep the existing top `Refresh` button as a page data refresh.

The setup panel can keep the initial domain field because no domain has been connected yet.

## Testing

- Backend API tests cover fixed-domain refresh, 24-hour cooldown, active crawl rejection, and preserving confirmation metadata.
- Insight endpoint tests cover rejecting a mismatched `landing_url` after project domain is set.
- Scheduler tests cover weekly registration and lightweight refresh eligibility.
- Frontend contract tests cover removing the connected-domain input, using `api.refreshContext`, and displaying last crawl time next to `Update context`.
