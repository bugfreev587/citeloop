# PRD: Runtime Model Routing And AI Site Fix GitHub PRs

> Date: 2026-07-09
> Scope: Project Admin Platform Runtime, Site Fixes, TokenGate model routing, GitHub PR generation
> Status: Draft for PM review

## 1. Summary

CiteLoop needs two connected upgrades:

- The project Admin Platform Runtime page should let operators configure which model powers each major CiteLoop AI role.
- Site Fixes should use the existing AI fixing JSON plus the configured Site Fix model to generate a reviewable GitHub PR when the user has connected GitHub.

The current Admin runtime UI exposes `Default model`, `Writer model`, and `QA model`. That is no longer specific enough. `Default model` is doing real planning work, including user context extraction and opportunity finding, while Site Fix PR generation is a distinct high-risk workflow that needs its own model route.

The target model is a four-role runtime matrix:

| Runtime role | CiteLoop usage | OpenAI model | Anthropic model | Primary provider |
| --- | --- | --- | --- | --- |
| Default / Planning | User context extraction, site summarization, opportunity finding, classification, routing recommendations | Configurable | Configurable | Configurable |
| AI Writer | Blog drafts, content refreshes, content repair | Configurable | Configurable | Configurable |
| QA | Evidence validation, hallucination checks, publish readiness, generated patch validation | Configurable | Configurable | Configurable |
| Site Fix | AI fixing JSON to repo patch proposal and GitHub PR | Configurable | Configurable | Configurable |

CiteLoop should still call TokenGate only. OpenAI and Anthropic model fields are TokenGate model IDs or aliases. CiteLoop stores TokenGate credentials and model routing preferences, not native OpenAI or Anthropic keys.

## 2. Background

Recent Site Fix work exposed a product gap:

1. A user connected GitHub for a project.
2. Site Fixes correctly showed a `Create GitHub PR` path for source-backed direct patches.
3. A metadata rewrite Site Fix failed because the action did not contain exact proposed title and description copy.
4. The UI still had an AI fixing JSON payload that could explain the issue, target URL, likely surfaces, constraints, and validation checks.
5. The user reasonably expected CiteLoop to feed that JSON to a strong AI model and create a PR.

Separately, the Admin Platform Runtime page currently makes model ownership ambiguous:

- `Default model` is not only a fallback. It powers planning behavior such as context extraction and opportunity discovery.
- `Writer model` and `QA model` do not cover Site Fix PR generation.
- There is no per-role OpenAI vs Anthropic selection.
- Site Fix work needs stronger code and repository reasoning than ordinary content generation.

The new design should make model routing visible, configurable, and auditable.

## 3. Problem Statement

CiteLoop cannot reliably explain or control which model is used for each internal AI workflow.

This creates four product problems:

- Operators cannot separately tune planning, writing, QA, and Site Fix behavior.
- Site Fix PR generation has no dedicated model route even though it can change production code.
- Connected-GitHub users still fall back to `Copy JSON Fix` or deterministic failures when a Site Fix lacks an exact patch.
- The system cannot audit a Site Fix PR with enough detail to answer: which model generated this patch, from which AI fixing JSON, for which action, and under which validator.

## 4. Goals

- Replace the current three-field runtime UI with a role-based model routing matrix.
- Treat `Default / Planning` as a first-class role, not merely a fallback.
- Let every runtime role configure one OpenAI model and one Anthropic model.
- Let every runtime role choose a primary provider: `OpenAI`, `Anthropic`, or `Auto`.
- Add a dedicated `Site Fix` role for AI fixing JSON to GitHub PR generation.
- Use TokenGate for all model calls.
- Add Site Fix AI PR generation for GitHub-connected projects.
- Keep deterministic validators and patch application in CiteLoop; AI proposes patches, CiteLoop creates PRs.
- Preserve `Copy JSON Fix` for projects without GitHub or when AI PR creation cannot safely proceed.
- Record role, provider, model, action ID, AI fixing JSON hash, and validation result on every AI-created Site Fix PR.

## 5. Non-Goals

- Do not store native OpenAI or Anthropic API keys in CiteLoop.
- Do not let AI directly push arbitrary repository changes without CiteLoop validation.
- Do not auto-merge Site Fix PRs.
- Do not move content-producing work out of Content Plan.
- Do not use Site Fix AI PR generation to create new blog posts.
- Do not replace human GitHub review.
- Do not require every Site Fix to use AI; exact deterministic patches can still create PRs directly.

## 6. Product Decisions

### 6.1 Runtime Roles

Admin should expose these roles:

| Role key | User-facing label | Responsibilities |
| --- | --- | --- |
| `planning` | Default / Planning | User context extraction, crawl summary interpretation, opportunity finding, opportunity classification, routing recommendations, low-risk planning copy. |
| `writer` | AI Writer | Blog drafts, article refreshes, evidence block rewrites, AI repair for content drafts. |
| `qa` | QA | Evidence validation, citation checks, hallucination checks, publish readiness, Site Fix patch sanity review. |
| `site_fix` | Site Fix | Converts AI fixing JSON and repo context into a structured patch proposal for existing pages, metadata, schema, routing, sitemap, robots, canonical, and similar direct site work. |

`Default / Planning` is the role used when CiteLoop is analyzing the project and creating opportunities. It is not the fallback model. Fallback behavior is configured separately.

### 6.2 Provider And Model Selection

Each role has:

- `primary_provider`: `openai`, `anthropic`, or `auto`
- `openai_model`: TokenGate model ID or alias for OpenAI-backed calls
- `anthropic_model`: TokenGate model ID or alias for Anthropic-backed calls
- `fallback_enabled`: boolean; when enabled, the non-primary configured provider can be used after a visible primary-route failure policy allows it

When `primary_provider` is:

- `openai`: use the role's OpenAI model first.
- `anthropic`: use the role's Anthropic model first.
- `auto`: let CiteLoop choose based on workflow defaults, availability, and cost or quality policy.

All calls still go through the TokenGate OpenAI-compatible API endpoint. The provider choice decides which configured TokenGate model alias CiteLoop sends.

### 6.3 Recommended Defaults

V1 defaults should be conservative:

| Role | Recommended primary | Example model intent |
| --- | --- | --- |
| Default / Planning | OpenAI or Auto | Fast, strong general reasoning for extraction and classification. |
| AI Writer | OpenAI or Auto | Strong long-form writing and repair. |
| QA | OpenAI or Anthropic | Strong factual checking and instruction following. |
| Site Fix | Anthropic | Strong code/repository reasoning, for example an Opus-class TokenGate alias. |

The product should not hardcode `opus 4.8` into business logic. Admin can configure a TokenGate model alias such as `opus-4.8` or any future equivalent for the Site Fix Anthropic model.

## 7. Admin Platform Runtime UX

### 7.1 Page Structure

The Project Admin `Platform runtime` tab should contain:

1. Runtime credentials
2. Model routing
3. Runtime test actions
4. Safe secrets note

### 7.2 Runtime Credentials

Keep the existing credential controls:

- TokenGate API key
- TokenGate base URL
- TokenGate configured status
- Save credentials
- Test connection
- Refresh
- Delete key

The page should continue returning only base URL, model IDs, provider preferences, and key tail to the browser. Raw keys remain server-side.

### 7.3 Model Routing Matrix

Replace the three current model inputs with a routing matrix.

Each row is a role:

- Default / Planning
- AI Writer
- QA
- Site Fix

Each row includes:

- Primary provider segmented control: `Auto`, `OpenAI`, `Anthropic`
- OpenAI model input
- Anthropic model input
- Fallback toggle or compact fallback status
- Short helper text naming the CiteLoop features that use this role

Example row copy:

```text
Site Fix
Uses AI fixing JSON and repository context to generate validated GitHub PRs for existing pages.
Primary: Anthropic
OpenAI model: gpt-5.1
Anthropic model: opus-4.8
Fallback: On
```

### 7.4 Validation

The Admin page should validate before save:

- TokenGate base URL must be non-empty and should include `/v1`.
- At least one model must be configured for each role.
- A role cannot use `openai` primary without an OpenAI model.
- A role cannot use `anthropic` primary without an Anthropic model.
- `auto` requires at least one model and should warn when only one provider is available.

### 7.5 Test Connection

`Test connection` must test every primary runtime route, not just the base TokenGate key.

V1 behavior:

- Run lightweight test prompts for all four roles:
  - Default / Planning
  - AI Writer
  - QA
  - Site Fix
- For each role, use that role's configured `primary_provider` and the corresponding model.
- If a role uses `auto`, resolve the primary model exactly as runtime execution would.
- Show a per-role result row with provider, model, status, latency, and error message when applicable.
- Treat the overall test as failed if any role's primary route fails.
- Do not silently test a fallback provider when the primary provider fails. Fallback can be tested separately, but the primary route must be visibly healthy or visibly broken.

Example result:

| Role | Provider | Model | Result |
| --- | --- | --- | --- |
| Default / Planning | OpenAI | `gpt-5.1` | Passed |
| AI Writer | OpenAI | `gpt-5.1` | Passed |
| QA | OpenAI | `gpt-5.5` | Passed |
| Site Fix | Anthropic | `opus-4.8` | Passed |

Optional V2 behavior:

- Add a `Test role` action per row for faster iteration.
- Add a `Test fallback` action per row when fallback is enabled.
- Show last successful test time per role.

## 8. Backend Model Routing

### 8.1 Data Model

The backend should persist role routes in a normalized structure. Implementation can choose either typed columns or a JSON column, but the API contract should normalize to this shape:

```json
{
  "base_url": "https://api.tokengate.to/v1",
  "configured": true,
  "key_tail": "b97a",
  "routes": {
    "planning": {
      "primary_provider": "openai",
      "openai_model": "gpt-5.1",
      "anthropic_model": "claude-sonnet-4-6",
      "fallback_enabled": true
    },
    "writer": {
      "primary_provider": "openai",
      "openai_model": "gpt-5.1",
      "anthropic_model": "claude-sonnet-4-6",
      "fallback_enabled": true
    },
    "qa": {
      "primary_provider": "openai",
      "openai_model": "gpt-5.5",
      "anthropic_model": "claude-sonnet-4-6",
      "fallback_enabled": true
    },
    "site_fix": {
      "primary_provider": "anthropic",
      "openai_model": "gpt-5.1",
      "anthropic_model": "opus-4.8",
      "fallback_enabled": false
    }
  }
}
```

### 8.2 Migration

Existing fields should migrate as follows:

| Existing field | New role route |
| --- | --- |
| `default_model` | `planning.openai_model` or provider-neutral default route |
| `writer_model` | `writer.openai_model` or provider-neutral writer route |
| `qa_model` | `qa.openai_model` or provider-neutral QA route |

If provider is unknown during migration:

- Preserve existing values as OpenAI route values for compatibility.
- Populate Anthropic route values from environment defaults when available.
- Set Site Fix to a safe default only when the configured model exists; otherwise leave it unset and require Admin attention before AI PR generation.

### 8.3 Runtime Purpose Mapping

Backend LLM calls should declare a purpose:

| Purpose | Route |
| --- | --- |
| `planning` | Default / Planning |
| `writer` | AI Writer |
| `qa` | QA |
| `site_fix` | Site Fix |

Existing calls without a purpose should use `planning`.

### 8.4 Runtime Route Test API

The runtime test endpoint should return per-role test results:

```json
{
  "ok": false,
  "results": [
    {
      "role": "planning",
      "provider": "openai",
      "model": "gpt-5.1",
      "ok": true,
      "latency_ms": 842
    },
    {
      "role": "site_fix",
      "provider": "anthropic",
      "model": "opus-4.8",
      "ok": false,
      "error": "TokenGate rejected model opus-4.8 for this key"
    }
  ]
}
```

The API should use each role's primary provider route. It should not collapse the result into a single success flag without returning role-level detail.

## 9. Site Fix AI Create PR

### 9.1 User-Facing Behavior

Site Fix detail actions should depend on GitHub connection state and fix readiness.

| State | Primary action |
| --- | --- |
| GitHub not connected | `Copy JSON Fix` |
| GitHub connected and exact deterministic patch is available | `Create GitHub PR` |
| GitHub connected and AI fixing JSON exists but exact patch is missing | `AI Create PR` |
| GitHub connected but Site Fix route is not configured | Disabled action with Admin guidance |
| Site Fix is already applied or PR exists | `Open PR` or applied status |

`AI Create PR` means:

- CiteLoop calls TokenGate using the `site_fix` runtime role.
- The model receives the AI fixing JSON plus constrained repository context.
- The model returns a structured patch proposal.
- CiteLoop validates and applies the patch to a GitHub branch.
- CiteLoop opens a GitHub PR for human review.

### 9.2 AI Input

The Site Fix AI call should receive:

- AI fixing JSON
- Project ID and action ID
- Target URL and canonical URL
- Site fix category and issue type
- Existing observed values from production when available
- Desired outcome and constraints
- Likely repository surfaces from the fixing JSON
- Source file excerpts fetched from GitHub
- Publisher connection config such as repo, branch, content path, and base URL
- Brand or product profile summary when useful

The model must not receive raw TokenGate or GitHub credentials.

### 9.3 AI Output Contract

The model must return strict JSON, not prose:

```json
{
  "summary": "Rewrite homepage metadata for query relevance.",
  "target_url": "https://unipost.dev/",
  "files": [
    {
      "path": "dashboard/src/app/marketing/page.tsx",
      "operations": [
        {
          "type": "replace",
          "selector": "const HOMEPAGE_TITLE",
          "before": "Current title",
          "after": "Proposed SEO title"
        }
      ]
    }
  ],
  "generated_copy": {
    "title": "Proposed SEO title",
    "meta_description": "Proposed meta description"
  },
  "validation_checks": [
    "Title is not copied from the issue title.",
    "Description references reviewed production copy.",
    "No new page or post is created."
  ],
  "risk_flags": []
}
```

V1 may support metadata rewrite operations first, then expand to schema, robots, sitemap, canonical, redirects, internal links, and content-section edits.

### 9.4 CiteLoop Validation

CiteLoop must validate the AI patch proposal before creating a PR:

- Output is valid JSON and matches the schema.
- Target URL matches the Site Fix action.
- Files are in allowed repository surfaces.
- The patch applies cleanly to the configured GitHub branch.
- The patch updates an existing page or existing site config; it does not create a new blog post unless the Site Fix type explicitly allows it.
- Metadata title is not identical to the Site Fix title.
- Generated copy contains no placeholders, staging URLs, localhost URLs, or generic issue text.
- Metadata copy is within configured length guidance.
- The patch preserves existing framework conventions.
- The patch can be summarized in a small, reviewable diff.

If validation fails, no PR is created. The drawer should show a specific fixable error and keep `Copy JSON Fix` available.

### 9.5 Optional QA Gate

For high-risk Site Fixes, CiteLoop should run a second validation pass using the `qa` role before opening the PR.

QA receives:

- AI fixing JSON
- Generated patch proposal
- Final diff summary
- Validator results

QA returns:

- approve
- reject with reason
- needs human review

V1 can require QA for metadata and content-copy changes and skip QA for deterministic low-risk technical patches.

### 9.6 GitHub PR Creation

After validation:

1. Create or reuse a CiteLoop branch.
2. Apply the validated patch.
3. Commit with a Site Fix-specific message.
4. Open a GitHub PR.
5. Store PR URL, PR number, branch, commit SHA, model route, and validation snapshot.

PR title format:

```text
Fix: <site fix title>
```

PR body should include:

- Target URL
- Site Fix action ID
- AI fixing JSON hash
- Runtime role: `site_fix`
- Provider and model used
- Validation checks run
- Human review warning
- Production verification checklist

## 10. Loop Prevention

AI-created Site Fix PRs must not create an infinite SEO/GEO improvement loop.

CiteLoop should:

- Mark the Site Fix action as `pr_created` or equivalent after PR creation.
- Suppress identical findings for the same target URL and issue hash while PR is open.
- After merge and deployment, re-crawl or re-check production before marking applied.
- If the same finding appears again, attach it to the prior Site Fix and require a new reason before creating another PR.
- Store before/after observed values so the next opportunity detector can distinguish unchanged pages from failed fixes.

## 11. Error Handling

| Failure | User-facing behavior |
| --- | --- |
| GitHub not connected | Show `Copy JSON Fix` and link to publisher connection settings. |
| Site Fix route not configured | Show Admin guidance: configure Site Fix model route. |
| TokenGate call fails | Show retryable error with provider/model used. |
| AI output is invalid | Show validation error; keep `Copy JSON Fix`. |
| Patch does not apply | Show source mapping error; keep `Copy JSON Fix`. |
| QA rejects patch | Show QA reason; do not create PR. |
| GitHub branch or PR conflict | Reuse existing branch safely or show existing PR. |
| GitHub permission missing | Show GitHub App permission guidance. |

Errors should be precise enough that a user can understand whether to configure Admin, reconnect GitHub, retry, or copy JSON to a coding agent manually.

## 12. Permissions And Security

- TokenGate key remains server-side.
- GitHub App token remains server-side.
- Browser receives only redacted key status, model IDs, route preferences, and PR metadata.
- AI never receives secrets.
- AI cannot choose arbitrary files outside allowed surfaces.
- AI output cannot bypass CiteLoop validation.
- PR creation requires GitHub connection and repository permission.

## 13. Analytics And Audit

Record events for:

- Admin model route saved
- Admin route test succeeded or failed
- Site Fix AI PR generation started
- TokenGate model selected
- AI patch proposal generated
- Patch validation passed or failed
- QA gate passed or failed
- GitHub PR created
- GitHub PR creation failed
- Production verification passed or failed

Every Site Fix AI PR should be traceable from:

- Project
- Action
- Opportunity
- AI fixing JSON
- Model route
- GitHub PR
- Post-deploy verification

## 14. Rollout Plan

### Phase 1: Admin Runtime Routing

- Add role route data model and API.
- Migrate existing default, writer, and QA values.
- Add Site Fix role fields.
- Replace Admin Platform Runtime UI with the routing matrix.
- Keep old API compatibility until frontend and backend are deployed together.

### Phase 2: Site Fix AI Patch Proposal

- Add Site Fix `site_fix` LLM purpose.
- Add structured AI output schema.
- Add metadata rewrite AI proposal support first.
- Add validators and tests.
- Add drawer state for `AI Create PR`.

### Phase 3: GitHub PR Creation

- Feed validated patch proposal into existing GitHub PR publisher.
- Store PR metadata and model audit data.
- Add duplicate-loop suppression for open PRs.

### Phase 4: Production Verification Loop

- Re-check production after merge and deployment.
- Mark Site Fix applied only when observed production state satisfies the fix.
- Surface failed verification as a follow-up, not a duplicate opportunity.

## 15. Acceptance Criteria

### Admin Runtime

- Admin Platform Runtime shows four roles: Default / Planning, AI Writer, QA, and Site Fix.
- Each role can configure OpenAI model, Anthropic model, and primary provider.
- Saving the page persists all role routes.
- Refreshing the page returns the same routes.
- Test connection tests all four primary role routes: Default / Planning, AI Writer, QA, and Site Fix.
- Test connection shows provider, model, status, latency, and error for each role.
- Test connection fails overall when any primary role route fails.
- Existing projects retain their current writer and QA behavior after migration.
- No native provider API keys are added to CiteLoop.

### Site Fix AI PR

- GitHub-disconnected projects still show `Copy JSON Fix`.
- GitHub-connected projects show `AI Create PR` when a Site Fix has AI fixing JSON but lacks an exact deterministic patch.
- Clicking `AI Create PR` calls TokenGate with the `site_fix` role.
- AI output must be strict structured JSON.
- Invalid AI output does not create a PR.
- Unsafe metadata copy, including issue-title-as-title, does not create a PR.
- Valid output creates a GitHub branch and PR.
- PR body records action ID, AI fixing JSON hash, provider, model, and validation checks.
- Open PRs suppress duplicate Site Fix loops for the same issue hash.

## 16. Test Plan

Backend tests:

- Admin credentials migration preserves existing default, writer, and QA values.
- Admin route save/load handles all four roles.
- Runtime provider selects model by purpose and primary provider.
- Runtime provider falls back only when fallback is enabled.
- Runtime test endpoint tests planning, writer, QA, and Site Fix primary routes.
- Runtime test endpoint reports per-role provider, model, status, latency, and error.
- Runtime test endpoint fails overall when any primary route fails.
- Site Fix AI PR route uses `site_fix` purpose.
- AI output schema parser rejects malformed output.
- Metadata validator rejects issue-title-as-title.
- Patch validator rejects new blog post creation for metadata Site Fix.
- GitHub PR publisher stores model audit metadata.
- Duplicate finding suppression works while a PR is open.

Frontend tests:

- Admin Platform Runtime renders four role rows.
- Primary provider controls validate missing model fields.
- Save payload includes OpenAI and Anthropic model IDs for each role.
- Site Fix drawer shows `Copy JSON Fix` when GitHub is disconnected.
- Site Fix drawer shows `AI Create PR` when GitHub is connected and deterministic patch is missing.
- Drawer shows precise validation errors and retry states.

Production verification:

- Configure Site Fix role on the project Admin page.
- Trigger a metadata rewrite Site Fix with AI fixing JSON.
- Confirm GitHub PR is created with safe generated metadata, not the issue title.
- Confirm PR body includes model audit data.
- Merge a safe test PR in a controlled project.
- Confirm production re-check marks the Site Fix applied or reports a specific failed check.

## 17. Open Questions

- Should Site Fix fallback be enabled by default, or should code-changing workflows require the primary provider only?
- Should every AI-generated Site Fix PR require QA, or only metadata and content-copy changes?
- Should the Admin UI expose per-role cost/latency hints from TokenGate when available?
- Should `AI Create PR` first show a diff preview inside CiteLoop before opening GitHub PR, or is GitHub review the first preview surface for V1?
