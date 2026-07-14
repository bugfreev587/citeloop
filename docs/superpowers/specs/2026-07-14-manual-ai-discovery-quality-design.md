# Manual AI Discovery Quality and Progress Design

## Status

Approved on July 14, 2026. This specification amends the stage-aware Opportunity Discovery design. Its bounded-output rule supersedes the earlier blanket non-goal against an Opportunity quota for explicit manual runs only.

## Production Finding

The UniPost manual run at 18:11 UTC executed every Opportunity Finding checkpoint but made no physical AI call. Evidence refresh selected eight prompts, then reused the current weekly `ai_answer` evidence run. The analyzer deterministically rescored 18 historical candidates. Three passed Foundation scoring but merged into already-converted work; ten were rejected by an over-broad internal-term regex and five remained watchlisted for single-provider GEO demand. The user therefore saw zero new Opportunities even though the UI described the run as AI discovery.

## Product Contract

An explicit **Run finding** request is a request for fresh, stage-aware discovery:

1. It must perform a new audited TokenGate call when Opportunities AI is enabled. Scheduled runs may continue to reuse evidence.
2. It must seek candidates appropriate to the pinned Growth Stage, not merely rescore the same historical observation set.
3. It targets at least one new decision-ready Opportunity when the project has confirmed public capabilities and the provider succeeds.
4. If the first candidate set yields no new Opportunity because of duplicates or correctable quality failures, one bounded repair pass receives the rejection reasons and proposes replacements.
5. It may still return zero when no confirmed publishable capability exists, the AI provider fails, or every proposal violates a non-bypassable safety or evidence rule. The terminal status must explain that outcome.

The minimum-output target is not permission to invent evidence, expose private implementation details, reopen completed work, or bypass canonical conflict and duplicate rules.

## Discovery Architecture

Manual runs add a tracked `opportunity_discovery` planner before deterministic materialization. The planner receives only confirmed public project context, the pinned stage profile, current coverage, recent SEO/GEO evidence summaries, supported platform targets, and hashes/summaries of already handled work. It returns structured candidate hypotheses and targeted answer-engine prompts.

The planner output is model-assisted input, not evidence. Each proposal is mapped back to a confirmed capability and validated against supported intent and asset taxonomies. Targeted prompts are observed through the answer provider and combined with search or first-party evidence before the existing deterministic scorer may create an Opportunity.

One repair call is allowed when the first pass creates no new work. Its input includes stable rejection codes such as duplicate identity, already handled, unsupported claim, insufficient evidence, or missing target. The repair pass must change the topic/intent/action identity rather than paraphrase a rejected candidate.

Every planner and repair call is recorded in `ai_call_records`, including physical-provider status, tokens, cost, prompt version, model, and linked workflow run. The run summary reports fresh calls, proposals, accepted candidates, repair attempts, and the final zero-result reason.

## Fresh Evidence

Manual evidence collection uses a fresh request identity and does not reuse the weekly answer-evidence result. It remains persisted and auditable and can be consumed by later scheduled work. Scheduled and event-triggered discovery keep the weekly reuse policy.

Repeated observations from the same underlying provider remain one independent GEO provider. Freshness does not manufacture provider diversity.

## Context Safety

The existing regex incorrectly treats public subject matter such as databases, API keys, deployment, and encryption as inherently private. The replacement classifier distinguishes:

- public educational/product topics, which may be discovered and published;
- secret-shaped values, credentials, private URLs/repository identifiers, environment-specific infrastructure, and explicit internal diagnostics, which remain blocked.

This change applies both when selecting active prompts and when scoring candidate topics. It does not place raw secrets in an AI prompt.

## Progress and Parallelism

The Opportunity page polls the existing checkpoint API while a run is queued or active and displays:

- a determinate progress bar from `progress_percent`;
- a human label for the active checkpoint;
- the six checkpoint states;
- an explicit “Calling AI” state during planner, answer observation, or repair work;
- the terminal number of new Opportunities and an actionable zero-result reason.

Within evidence refresh, crawler audit, search-result collection, answer-provider observation, and external-surface monitoring may run concurrently after prompt selection. Each operation writes to independent records and returns its own error. Prompt creation/selection precedes them; analysis, repair, deterministic scoring, arbitration, and materialization remain ordered.

## Growth Stage UI

Replace the native select with an accessible custom listbox. The closed trigger displays only the `Growth Stage` label and selected stage name. Each open option displays a prominent stage name with its explanation below in smaller muted text. Keyboard navigation, Escape, outside-click dismissal, focus state, and disabled/busy behavior are required.

The unconfirmed-default notice has a close button. Dismissal is stored in local storage using project ID and stage-setting version, so it remains closed for that unchanged default but reappears if the relevant setting changes. Explicitly choosing a stage still removes the notice server-side.

## Verification

Automated tests must cover manual fresh evidence, a physical tracked planner call, one bounded repair, correct zero-result reasons, scheduled evidence reuse, public-vs-secret context classification, parallel evidence aggregation, checkpoint progress rendering, accessible stage selection, and notice dismissal persistence.

Production verification uses UniPost: trigger Run finding, observe live progress, confirm at least one new `ai_call_records` row with `provider_called=true`, confirm fresh TokenGate usage, and verify either a new open Opportunity or a precise non-correctable blocking reason. UI behavior is verified at desktop and narrow widths.
