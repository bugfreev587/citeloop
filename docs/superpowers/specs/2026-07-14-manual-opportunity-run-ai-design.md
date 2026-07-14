# Manual Opportunity Finding AI Design

## Goal

Make an explicit **Run finding** request execute AI Discovery and Growth Radar materialization whenever Opportunities AI is enabled, including projects whose stored run policy is `scheduled_only`. Keep `scheduled_only` as a restriction on automatic event triggers, not on user-authorized runs.

## Behavior

- A manual Opportunity Finding run includes Signal Scan according to `growth_signal_enabled` and includes AI Discovery according to `growth_ai_enabled`.
- `scheduled_only` continues to authorize scheduled AI runs and reject event-triggered AI runs.
- Disabling Opportunities AI continues to reject every AI trigger.
- The stored policy keys and migration behavior do not change.

## Settings placement

Opportunity Finding authority belongs on the Automation tab because that is where users look for run behavior. Move the existing Opportunities controls there without duplicating state or save actions. Map `#opportunity-finding` to the Automation tab and place the card before readiness controls so both `#automation` and the dedicated deep link expose it immediately.

The AI assistance tab retains the independent Doctor controls and an overview of both authority lines. It links users to Automation for Opportunity Finding controls.

## Copy

Present `scheduled_only` as **Scheduled + manual**. Explain that scheduled runs and explicit **Run finding** requests may call AI, while automatic context, publish, and measurement events may not. Use the same wording in Opportunity Finding run status to avoid showing “Scheduled runs only” after a successful manual AI run.

## Verification

Add backend regression tests for trigger authority and stage selection, UI contract tests for ownership/deep links/copy, then run Go tests, web tests, typecheck, and build. After merge and deployment, verify the production settings page and trigger a production run for UniPost; confirm a new Growth Radar run/materialization is recorded.
