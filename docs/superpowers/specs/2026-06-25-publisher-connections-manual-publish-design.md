# Publisher Connections Manual Publish Design

## Goal

Tighten publisher access so GitHub/Next.js publishing is managed only from project Settings, and approved content is not automatically scheduled by default.

## Scope

This change covers the existing GitHub/Next.js canonical publishing path only. External syndication connectors remain manual distribution surfaces and do not gain real scheduling in this work.

## Product Behavior

- Project publisher connections are managed from `/projects/:id/settings#publisher`.
- Publish no longer owns connection setup. Its platforms drawer is removed.
- Publish shows a compact connected-account selector for enabled, connected publisher accounts.
- The selector includes a `Manage connections` link to `/projects/:id/settings#publisher`.
- Each publisher connection has an Enable or Disable action in Settings.
- Only enabled and connected publisher connections are eligible for Publish scheduling.
- If no enabled publisher exists, Publish explains that publishing is blocked until a connection is enabled.
- New projects default to manual publishing.
- Existing projects without an explicit `publish_mode` are parsed as manual publishing.
- Auto-advance can still generate, repair, and approve content, but it must not schedule canonical publication unless the project explicitly uses scheduled or auto publish mode.

## Architecture

Add an `enabled boolean not null default false` column to `publisher_connections`. Existing records start disabled so production does not keep publishing through old implicit configuration. The API returns `enabled`, exposes a project-scoped toggle endpoint, and keeps `status` focused on connection health.

Scheduler publisher resolution uses the default GitHub/Next.js connection only when it is `enabled` and `connected`. The env-backed `BlogPublisher` remains available for explicit `env:GITHUB_TOKEN` credential refs and dry-run/local paths, but missing project connections no longer create an eligible production publisher.

Frontend connection management moves to Settings. Publish consumes the connection list as read-only state, renders enabled connected accounts in a simple select, and links users to Settings for changes.

## Testing

- Backend config tests cover manual publish defaults and explicit scheduled/auto modes.
- DB contract tests cover the new `enabled` column and toggle queries.
- Scheduler tests cover disabled/missing connections not being eligible and enabled connected connections being eligible.
- API tests cover `enabled` in responses and toggle routes.
- Web contract tests cover Publish drawer removal, `Manage connections` copy, Settings enable/disable controls, and default manual UI behavior.
