# PRD: Runtime Model Routing And AI Site Fix GitHub PRs

> Date: 2026-07-09
> Scope: Platform Runtime Admin, Site Fixes, TokenGate model routing, GitHub PR generation
> Status: Draft for PM review

## 1. Summary

CiteLoop needs two connected upgrades:

- The Platform Runtime Admin surface should let operators configure which model powers each major CiteLoop AI role.
- Site Fixes should promote the current drawer-level AI coding fix JSON into a server-side AI fixing JSON contract, then use the configured Site Fix model to generate a reviewable GitHub PR when the user has connected GitHub.

The current Admin runtime UI exposes `Default model`, `Writer model`, and `QA model`. That is no longer specific enough. `Default model` is doing real planning work, including user context extraction and opportunity finding, while Site Fix PR generation is a distinct high-risk workflow that needs its own model route.

The target model is a four-role runtime matrix:

| Runtime role | CiteLoop usage | OpenAI alias | Anthropic alias | Primary provider |
| --- | --- | --- | --- | --- |
| Default / Planning | User context extraction, site summarization, opportunity finding, classification, routing recommendations | Configurable | Configurable | Configurable |
| AI Writer | Blog drafts, content refreshes, content repair | Configurable | Configurable | Configurable |
| QA | Evidence validation, hallucination checks, publish readiness, generated patch validation | Configurable | Configurable | Configurable |
| Site Fix | AI fixing JSON to repo patch proposal and GitHub PR | Configurable | Configurable | Configurable |

CiteLoop should still call TokenGate only. OpenAI and Anthropic fields are operator-labeled TokenGate model aliases, not native provider credentials. TokenGate remains the provider router. CiteLoop stores TokenGate credentials and role routing preferences, not native OpenAI or Anthropic keys.

## 2. Background

Recent Site Fix work exposed a product gap:

1. A user connected GitHub for a project.
2. Site Fixes correctly showed a `Create GitHub PR` path for source-backed direct patches.
3. A metadata rewrite Site Fix failed because the action did not contain exact proposed title and description copy.
4. The UI still had an AI coding fix JSON payload that could explain the issue, target URL, likely surfaces, constraints, and validation checks.
5. The user reasonably expected CiteLoop to feed that JSON to a strong AI model and create a PR.

Separately, the Admin Platform Runtime page currently makes model ownership ambiguous:

- `Default model` is not only a fallback. It powers planning behavior such as context extraction and opportunity discovery.
- `Writer model` and `QA model` do not cover Site Fix PR generation.
- There is no per-role OpenAI vs Anthropic selection.
- Site Fix work needs stronger code and repository reasoning than ordinary content generation.

The new design should make model routing visible, configurable, and auditable.

### 2.1 Current Implementation Baseline

This PRD builds on the current source-backed Site Fix PR apply layer:

- The frontend already exposes `AI coding fix JSON`, `Copy fix JSON`, and `Create GitHub PR` for Site Fixes.
- The backend already has a `site_change_applications` ledger, a `/site-fix-pr` endpoint, and a GitHub PR client for deterministic source-backed patches.
- The current AI coding fix JSON is derived in the frontend from action snapshots. It is useful as a human/coding-agent brief, but it is not yet a persisted server-side contract with an ID, hash, schema version, or audit trail.

This PRD extends that baseline. It does not assume that the current frontend-derived JSON is already sufficient for automated AI PR generation.

### 2.2 Global Runtime Scope

The existing runtime credential table is a singleton platform configuration. V1 keeps that global model:

- The same runtime routes apply to all projects.
- The project Admin page may surface the Platform Runtime controls for convenience, but saving those controls changes the global runtime for all projects.
- Per-project model overrides are a future feature and are out of scope for this PRD.

## 3. Problem Statement

CiteLoop cannot reliably explain or control which model is used for each internal AI workflow.

This creates four product problems:

- Operators cannot separately tune planning, writing, QA, and Site Fix behavior.
- Site Fix PR generation has no dedicated model route even though it can change production code.
- Connected-GitHub users still fall back to `Copy fix JSON` or deterministic failures when a Site Fix lacks an exact patch.
- The frontend-derived AI coding fix JSON is not yet a server-side artifact that can be hashed, versioned, audited, and safely fed into an AI patch generator.
- The system cannot audit a Site Fix PR with enough detail to answer: which model generated this patch, from which AI fixing JSON contract, for which action, and under which validator.

## 4. Goals

- Replace the current three-field runtime UI with a role-based model routing matrix.
- Treat `Default / Planning` as a first-class role, not merely a fallback.
- Let every runtime role configure one OpenAI-labeled TokenGate alias and one Anthropic-labeled TokenGate alias.
- Let every runtime role choose a primary provider label: `OpenAI` or `Anthropic`.
- Add a dedicated `Site Fix` role for AI fixing JSON contract to GitHub PR generation.
- Use TokenGate for all model calls.
- Persist AI fixing JSON contracts server-side before using them for AI PR generation.
- Add Site Fix AI PR generation for GitHub-connected projects.
- Keep deterministic validators and patch application in CiteLoop; AI proposes patches, CiteLoop creates PRs.
- Preserve `Copy fix JSON` for projects without GitHub or when AI PR creation cannot safely proceed.
- Record role, provider label, model alias, action ID, AI fixing JSON contract hash, and validation result on every AI-created Site Fix PR.

## 5. Non-Goals

- Do not store native OpenAI or Anthropic API keys in CiteLoop.
- Do not let AI directly push arbitrary repository changes without CiteLoop validation.
- Do not auto-merge Site Fix PRs.
- Do not move content-producing work out of Content Plan.
- Do not use Site Fix AI PR generation to create new blog posts.
- Do not replace human GitHub review.
- Do not require every Site Fix to use AI; exact deterministic patches can still create PRs directly.
- Do not introduce per-project runtime model overrides in V1.
- Do not implement automatic provider selection in V1; `Auto` can be evaluated after deterministic primary/fallback routing is proven.

## 6. Product Decisions

### 6.1 Runtime Roles

Admin should expose these roles:

| Role key | User-facing label | Responsibilities |
| --- | --- | --- |
| `planning` | Default / Planning | User context extraction, crawl summary interpretation, opportunity finding, opportunity classification, routing recommendations, low-risk planning copy. |
| `writer` | AI Writer | Blog drafts, article refreshes, evidence block rewrites, AI repair for content drafts. |
| `qa` | QA | Evidence validation, citation checks, hallucination checks, publish readiness, Site Fix patch sanity review. |
| `site_fix` | Site Fix | Converts an AI fixing JSON contract and repo context into a structured patch proposal for existing pages, metadata, schema, routing, sitemap, robots, canonical, and similar direct site work. |

`Default / Planning` is the role used when CiteLoop is analyzing the project and creating opportunities. It is not the fallback model. Fallback behavior is configured separately.

### 6.2 Provider Labels And Model Alias Selection

Each role has:

- `primary_provider`: `openai` or `anthropic`
- `openai_model_alias`: operator-labeled TokenGate model alias intended for OpenAI-backed calls
- `anthropic_model_alias`: operator-labeled TokenGate model alias intended for Anthropic-backed calls
- `fallback_enabled`: boolean; when enabled, the non-primary configured alias can be used after a visible primary-route failure policy allows it

When `primary_provider` is:

- `openai`: send the role's OpenAI-labeled alias to TokenGate first.
- `anthropic`: send the role's Anthropic-labeled alias to TokenGate first.

All calls still go through the TokenGate OpenAI-compatible API endpoint. The provider label decides which configured TokenGate alias CiteLoop sends; TokenGate remains responsible for mapping that alias to the actual upstream provider. CiteLoop must not infer provider identity from an opaque alias string except for known built-in defaults during migration.

### 6.3 Recommended Defaults

V1 defaults should be conservative:

| Role | Recommended primary | Example model intent |
| --- | --- | --- |
| Default / Planning | OpenAI | Fast, strong general reasoning for extraction and classification. |
| AI Writer | OpenAI | Strong long-form writing and repair. |
| QA | OpenAI or Anthropic | Strong factual checking and instruction following. |
| Site Fix | Anthropic | Strong code/repository reasoning, for example an Opus-class TokenGate alias. |

The product should not hardcode `opus 4.8` into business logic. Admin can configure a TokenGate model alias such as `opus-4.8` or any future equivalent for the Site Fix Anthropic model.

## 7. Admin Platform Runtime UX

### 7.1 Page Structure

The `Platform runtime` tab should contain:

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

The page should continue returning only base URL, model aliases, provider preferences, and key tail to the browser. Raw keys remain server-side.

### 7.3 Model Routing Matrix

Replace the three current model inputs with a routing matrix.

Each row is a role:

- Default / Planning
- AI Writer
- QA
- Site Fix

Each row includes:

- Primary provider segmented control: `OpenAI`, `Anthropic`
- OpenAI model alias input
- Anthropic model alias input
- Fallback toggle or compact fallback status
- Short helper text naming the CiteLoop features that use this role

Example row copy:

```text
Site Fix
Uses AI fixing JSON contract and repository context to generate validated GitHub PRs for existing pages.
Primary: Anthropic
OpenAI alias: gpt-5.1
Anthropic alias: opus-4.8
Fallback: Off
```

### 7.4 Validation

The Admin page should validate before save:

- TokenGate base URL must be non-empty and should include `/v1`.
- At least one model must be configured for each role.
- A role cannot use `openai` primary without an OpenAI model alias.
- A role cannot use `anthropic` primary without an Anthropic model alias.
- If fallback is enabled, the non-primary model alias must also be configured.

### 7.5 Test Connection

`Test connection` must test every primary runtime route, not just the base TokenGate key.

V1 behavior:

- Run lightweight test prompts for all four roles:
  - Default / Planning
  - AI Writer
  - QA
  - Site Fix
- For each role, use that role's configured `primary_provider` and the corresponding model.
- Show a per-role result row with provider label, model alias, status, latency, and error message when applicable.
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
  "backend_provider": "tokengate",
  "routes": {
    "planning": {
      "primary_provider": "openai",
      "openai_model_alias": "gpt-5.1",
      "anthropic_model_alias": "claude-sonnet-4-6",
      "fallback_enabled": true
    },
    "writer": {
      "primary_provider": "openai",
      "openai_model_alias": "gpt-5.1",
      "anthropic_model_alias": "claude-sonnet-4-6",
      "fallback_enabled": true
    },
    "qa": {
      "primary_provider": "openai",
      "openai_model_alias": "gpt-5.5",
      "anthropic_model_alias": "claude-sonnet-4-6",
      "fallback_enabled": true
    },
    "site_fix": {
      "primary_provider": "anthropic",
      "openai_model_alias": "gpt-5.1",
      "anthropic_model_alias": "opus-4.8",
      "fallback_enabled": false
    }
  }
}
```

`backend_provider` is the legacy runtime provider and must remain `tokengate`. It is not the same field as a role's `primary_provider`. The role field selects which operator-labeled TokenGate alias to send.

### 8.2 Migration

Existing fields should migrate as follows:

| Existing field | New role route |
| --- | --- |
| `model` / `default_model` | `planning` primary alias |
| `writer_model` | `writer` primary alias |
| `qa_model` | `qa` primary alias |

Migration rules:

- Preserve existing runtime behavior first. Existing model values should continue to be the selected primary alias for their role after migration.
- Do not blindly classify an opaque TokenGate alias as OpenAI or Anthropic. Only known CiteLoop defaults or clear built-in prefixes may be prefilled into a labeled slot.
- If an existing alias cannot be safely labeled, store it as the role's current primary alias and show an Admin warning asking the operator to assign it to the OpenAI or Anthropic slot before changing the route.
- Populate the missing complementary slot from environment defaults or leave it blank with validation guidance.
- Set Site Fix to a safe Anthropic-labeled default only when that alias is configured and testable; otherwise leave Site Fix disabled and require Admin attention before AI PR generation.

### 8.3 Runtime Purpose Mapping

Backend LLM calls should declare a purpose:

| Purpose | Route |
| --- | --- |
| `planning` | Default / Planning |
| `writer` | AI Writer |
| `qa` | QA |
| `site_fix` | Site Fix |

Existing calls without a purpose should use `planning`.

Implementation must audit current LLM call sites so opportunity finding, classification, and routing recommendations use `planning`, writer calls use `writer`, QA calls use `qa`, and Site Fix patch generation uses the new `site_fix` purpose. The existing `PurposeDefault("")` maps to `planning` for compatibility.

### 8.4 Runtime Route Test API

The runtime test endpoint should return per-role test results:

```json
{
  "ok": false,
  "results": [
    {
      "role": "planning",
      "provider": "openai",
      "model_alias": "gpt-5.1",
      "ok": true,
      "latency_ms": 842
    },
    {
      "role": "site_fix",
      "provider": "anthropic",
      "model_alias": "opus-4.8",
      "ok": false,
      "error": "TokenGate rejected model opus-4.8 for this key; fallback is disabled for this role"
    }
  ]
}
```

The API should use each role's primary provider route. It should not collapse the result into a single success flag without returning role-level detail.

This is new backend behavior. The current connection test only probes the default runtime path; implementation must add role-aware route resolution and role-specific probes.

## 9. Site Fix AI Create PR

### 9.1 User-Facing Behavior

Site Fix detail actions should depend on GitHub connection state and fix readiness.

| State | Primary action |
| --- | --- |
| GitHub not connected | `Copy fix JSON` |
| GitHub connected and exact deterministic patch is available | `Create GitHub PR` |
| GitHub connected and AI fixing JSON contract exists but exact patch is missing | `AI Create PR` |
| GitHub connected but Site Fix route is not configured | Disabled action with Admin guidance |
| Site Fix is already applied or PR exists | `Open PR` or applied status |

`AI Create PR` means:

- CiteLoop calls TokenGate using the `site_fix` runtime role.
- The model receives the server-side AI fixing JSON contract plus constrained repository context.
- The model returns a structured patch proposal.
- CiteLoop validates the patch proposal and renders an in-app diff preview.
- The user confirms the preview.
- CiteLoop applies the confirmed patch to a GitHub branch and opens a GitHub PR for human review.

### 9.2 AI Input

The Site Fix AI call should receive:

- AI fixing JSON contract ID, schema version, and payload
- Project ID and action ID
- Target URL and canonical URL
- Site fix category and issue type
- Existing observed values from production when available
- Desired outcome and constraints
- Likely repository surfaces from the fixing JSON contract
- Source file excerpts fetched from GitHub
- Publisher connection config such as repo, branch, content path, and base URL
- Brand or product profile summary when useful

The model must not receive raw TokenGate or GitHub credentials.

The server-side AI fixing JSON contract should be derived from the existing drawer payload but persisted before generation. It must include a stable ID, schema version, hash, target URL, issue type, proposed-change inputs when available, constraints, likely surfaces, acceptance tests, and source action metadata.

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

If validation fails, no PR is created. The drawer should show a specific fixable error and keep `Copy fix JSON` available.

### 9.5 QA Gate

For metadata and content-copy Site Fixes, CiteLoop must run a second validation pass using the `qa` role before showing the final PR confirmation. Deterministic low-risk technical patches may skip QA when the deterministic validator fully covers the change.

QA receives:

- AI fixing JSON contract
- Generated patch proposal
- Final diff summary
- Validator results

QA returns:

- approve
- reject with reason
- needs human review

If QA rejects the patch, no PR is created. The drawer should show the QA reason and keep `Copy fix JSON` available.

### 9.6 In-App Diff Preview

After AI generation, deterministic validation, and required QA pass, CiteLoop should render an in-app diff preview before opening a GitHub PR.

The preview should show:

- Target URL
- Source file path
- Provider label and model alias
- AI fixing JSON contract hash
- Proposed generated copy when applicable
- Unified diff or a compact file-level diff
- Validation checklist status
- Required QA result when applicable

The user must explicitly confirm before CiteLoop creates the GitHub PR. GitHub review remains the final code review surface, but it should not be the first place the user sees the AI-generated diff.

### 9.7 GitHub PR Creation

After preview confirmation:

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
- AI fixing JSON contract hash
- Runtime role: `site_fix`
- Provider label and model alias used
- Validation checks run
- QA gate result when applicable
- Human review warning
- Production verification checklist

## 10. Loop Prevention

AI-created Site Fix PRs must not create an infinite SEO/GEO improvement loop.

CiteLoop should:

- Mark the Site Fix action as `pr_created` or equivalent after PR creation.
- Suppress identical findings for the same target URL and opportunity key while PR is open, using the existing `seo_opportunities.opportunity_key` / `site_change_applications.opportunity_key` model rather than introducing a third dedupe key.
- After merge and deployment, re-crawl or re-check production before marking applied.
- If the same finding appears again, attach it to the prior Site Fix and require a new reason before creating another PR.
- Store before/after observed values so the next opportunity detector can distinguish unchanged pages from failed fixes.

## 11. Error Handling

| Failure | User-facing behavior |
| --- | --- |
| GitHub not connected | Show `Copy fix JSON` and link to publisher connection settings. |
| Site Fix route not configured | Show Admin guidance: configure Site Fix model route. |
| TokenGate call fails | Show retryable error with provider label and model alias used. If fallback is disabled, explicitly say no fallback was attempted. |
| AI output is invalid | Show validation error; keep `Copy fix JSON`. |
| Patch does not apply | Show source mapping error; keep `Copy fix JSON`. |
| QA rejects patch | Show QA reason; do not create PR. |
| User rejects preview | Store the rejected proposal snapshot and keep the Site Fix in review. |
| GitHub branch or PR conflict | Reuse existing branch safely or show existing PR. |
| GitHub permission missing | Show GitHub App permission guidance. |

Errors should be precise enough that a user can understand whether to configure Admin, reconnect GitHub, retry, or copy JSON to a coding agent manually.

## 12. Permissions And Security

- TokenGate key remains server-side.
- GitHub App token remains server-side.
- Browser receives only redacted key status, model aliases, route preferences, preview diff data, and PR metadata.
- AI never receives secrets.
- AI cannot choose arbitrary files outside allowed surfaces.
- AI output cannot bypass CiteLoop validation.
- PR creation requires GitHub connection and repository permission.

## 13. Analytics And Audit

Record events for:

- Admin model route saved
- Admin route test succeeded or failed
- AI fixing JSON contract created
- Site Fix AI PR generation started
- TokenGate model selected
- AI patch proposal generated
- Patch validation passed or failed
- QA gate passed or failed
- Diff preview accepted or rejected
- GitHub PR created
- GitHub PR creation failed
- Production verification passed or failed

Every Site Fix AI PR should be traceable from:

- Project
- Action
- Opportunity
- AI fixing JSON contract
- Model route
- Preview diff
- GitHub PR
- Post-deploy verification

## 14. Rollout Plan

### Phase 1: Admin Runtime Routing

- Add role route data model and API.
- Migrate existing default, writer, and QA values.
- Add Site Fix role fields.
- Replace Admin Platform Runtime UI with the routing matrix.
- Add role-aware route resolution for `planning`, `writer`, `qa`, and `site_fix`.
- Add per-role primary-route `Test connection` probes.
- Keep old API compatibility until frontend and backend are deployed together.

### Phase 2: AI Fixing JSON Contract

- Promote the current frontend-derived AI coding fix JSON into a server-side contract.
- Store contract ID, schema version, payload hash, action ID, target URL, issue type, constraints, likely surfaces, acceptance tests, and source action metadata.
- Keep `Copy fix JSON` available, but copy the server-side contract when present.
- Use this contract as the only allowed AI Site Fix PR generation input.

### Phase 3: Site Fix AI Patch Proposal And Preview

- Add Site Fix `site_fix` LLM purpose.
- Add structured AI output schema.
- Add metadata rewrite AI proposal support first.
- Add validators and tests.
- Require QA for metadata and content-copy Site Fix proposals.
- Add in-app diff preview and explicit user confirmation.
- Add drawer state for `AI Create PR`.

### Phase 4: GitHub PR Creation

- Feed confirmed, validated patch proposal into the existing source-backed GitHub PR publisher.
- Store PR metadata and model audit data.
- Add duplicate-loop suppression for open PRs using existing opportunity/application keys.

### Phase 5: Production Verification Loop

- Re-check production after merge and deployment.
- Mark Site Fix applied only when observed production state satisfies the fix.
- Surface failed verification as a follow-up, not a duplicate opportunity.

## 15. Acceptance Criteria

### Admin Runtime

- Admin Platform Runtime shows four roles: Default / Planning, AI Writer, QA, and Site Fix.
- The Platform Runtime page clearly states that V1 routes are global across projects.
- Each role can configure OpenAI model alias, Anthropic model alias, and primary provider label.
- Saving the page persists all role routes.
- Refreshing the page returns the same routes.
- Test connection tests all four primary role routes: Default / Planning, AI Writer, QA, and Site Fix.
- Test connection shows provider label, model alias, status, latency, and error for each role.
- Test connection fails overall when any primary role route fails.
- Test connection does not silently test fallback when a primary route fails.
- Existing projects retain their current writer and QA behavior after migration.
- No native provider API keys are added to CiteLoop.

### Site Fix AI PR

- GitHub-disconnected projects still show `Copy fix JSON`.
- GitHub-connected projects show `AI Create PR` when a Site Fix has an AI fixing JSON contract but lacks an exact deterministic patch.
- Clicking `AI Create PR` calls TokenGate with the `site_fix` role.
- AI output must be strict structured JSON.
- Invalid AI output does not create a PR.
- Unsafe metadata copy, including issue-title-as-title, does not create a PR.
- Valid output creates an in-app diff preview before opening a PR.
- User confirmation from the preview is required before GitHub PR creation.
- PR body records action ID, AI fixing JSON contract hash, provider label, model alias, QA result, and validation checks.
- Open PRs suppress duplicate Site Fix loops for the same opportunity key.

## 16. Test Plan

Backend tests:

- Admin credentials migration preserves existing default, writer, and QA values.
- Admin route save/load handles all four roles.
- Runtime provider selects model by purpose and primary provider.
- Runtime provider falls back only when fallback is enabled.
- Runtime test endpoint tests planning, writer, QA, and Site Fix primary routes.
- Runtime test endpoint reports per-role provider label, model alias, status, latency, and error.
- Runtime test endpoint fails overall when any primary route fails.
- Runtime test endpoint reports when fallback was not attempted because fallback is disabled.
- AI fixing JSON contract generation stores schema version, hash, action ID, and target URL.
- Site Fix AI PR route uses `site_fix` purpose.
- AI output schema parser rejects malformed output.
- Metadata validator rejects issue-title-as-title.
- Patch validator rejects new blog post creation for metadata Site Fix.
- Metadata and content-copy Site Fix AI proposals require QA approval.
- GitHub PR publisher stores model audit metadata.
- Duplicate finding suppression works while a PR is open.

Frontend tests:

- Admin Platform Runtime renders four role rows.
- Primary provider controls validate missing model fields.
- Save payload includes OpenAI and Anthropic model aliases for each role.
- Runtime test results render one row per role.
- Site Fix drawer shows `Copy fix JSON` when GitHub is disconnected.
- Site Fix drawer shows `AI Create PR` when GitHub is connected and deterministic patch is missing.
- Site Fix drawer shows an AI-generated diff preview before PR creation.
- Drawer shows precise validation errors and retry states.

Production verification:

- Configure Site Fix role on the Platform Runtime page.
- Run `Test connection` and confirm all four primary role routes are tested.
- Trigger a metadata rewrite Site Fix with an AI fixing JSON contract.
- Confirm an in-app diff preview is created with safe generated metadata, not the issue title.
- Confirm GitHub PR is created only after preview confirmation.
- Confirm PR body includes model audit data.
- Merge a safe test PR in a controlled project.
- Confirm production re-check marks the Site Fix applied or reports a specific failed check.

## 17. Open Questions

- Should the Admin UI expose per-role cost/latency hints from TokenGate when available?
- Should custom TokenGate aliases with no clear provider label require an explicit Admin relabel step before saving new route settings?
- When should CiteLoop add per-project runtime overrides on top of the global Platform Runtime routes?
