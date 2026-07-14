# Site Fix Grounding Recovery Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Safely correct repository patches rejected by the independent grounding verifier, preserve structured audit evidence, report truthful API/UI errors, and prove the existing production Site Fix can create a real UniPost PR.

**Architecture:** One immutable repository snapshot feeds a maximum of three total patch-generation attempts. Deterministic generation failures and complete grounding rejections produce distinct typed semantic feedback. Each valid patch receives an independent verification call; only an approved decision can finalize an application. Parsed verifier outcomes are stored as bounded JSONB metadata on the append-only AI-call ledger, and causal UUIDs remain only in ledger relationships.

**Tech Stack:** Go 1.24, pgx/sqlc, PostgreSQL migrations, log/slog, Next.js/TypeScript, Node test runner.

---

## Task 1: Introduce typed correction feedback and grounding rejection details

**Files:**

- Modify: internal/sitefix/apply.go
- Modify: internal/sitefix/grounding_verifier.go
- Modify: internal/sitefix/apply_test.go
- Modify: internal/sitefix/repository_apply_test.go

### Step 1: Write the failing feedback and rejection tests

Add tests that prove:

- repository-patch feedback adds exact old_text matching guidance;
- grounding feedback adds intent/proposition/unrelated-edit guidance and does not add exact-match-only guidance;
- equivalent semantic feedback creates equivalent descriptors/fingerprints regardless of call UUIDs;
- a complete rejected verifier response returns a typed error retaining its bounded PatchVerification decision; and
- malformed or incomplete verifier output remains invalid_response and has no trusted grounding decision.

Update the test generator interfaces to accept a GenerationFeedback value instead of a string so the test initially fails to compile against production code.

### Step 2: Run the focused tests and confirm RED

    go test ./internal/sitefix -run 'TestLLMApplicationGeneratorUsesFailureSpecificFeedback|TestGroundingVerifierPreservesRejectedDecision|TestGroundingVerifierRejectsIncompleteDecision'

Expected: FAIL because typed feedback and the grounding rejection error do not exist.

### Step 3: Implement the minimal typed contracts

In internal/sitefix/apply.go:

- define private feedback kinds repository_patch and grounding;
- define GenerationFeedback with safe code, bounded explanation, grounding flags, and bounded normalized added/removed/unsupported lists;
- keep AI-call UUIDs out of the feedback type;
- replace the FixGenerator string parameter with the typed value;
- add bounded text/list helpers with explicit item and rune limits;
- define PatchGroundingRejectionError with Unwrap returning ErrPatchGroundingRejected and no private reason in Error; and
- make deterministic verifier validation return the typed error with its decision.

In LLMApplicationGenerator:

- branch prompt guidance by feedback kind;
- serialize only bounded semantic feedback into the prompt;
- retain exact-replacement advice only for repository-patch failures; and
- bump the prompt version to doctor-repository-patch-generation-v2.

In LLMPatchGroundingVerifier, return the parsed decision plus the typed rejection error when the complete decision fails deterministic validation.

### Step 4: Run focused and package tests and confirm GREEN

    gofmt -w internal/sitefix/apply.go internal/sitefix/grounding_verifier.go internal/sitefix/apply_test.go internal/sitefix/repository_apply_test.go
    go test ./internal/sitefix

Expected: PASS.

### Step 5: Commit

    git add internal/sitefix/apply.go internal/sitefix/grounding_verifier.go internal/sitefix/apply_test.go internal/sitefix/repository_apply_test.go
    git commit -m "refactor: type Site Fix generation feedback"

## Task 2: Unify generation and grounding correction into one bounded loop

**Files:**

- Modify: internal/sitefix/apply.go
- Modify: internal/sitefix/apply_test.go
- Modify: internal/sitefix/repository_apply_test.go

### Step 1: Write failing orchestration tests

Extend applyStoreStub to record verifier call IDs, verifier causes, and finalize count. Extend patchVerifierStub to return a sequence of decisions/results/errors. Structured outcome capture is introduced with the persistence contract in Task 3.

Add tests for:

1. first verifier rejection -> grounding feedback -> second generation -> second verifier approval -> exactly one finalization;
2. causal order selection -> generation 1 -> verifier 1 -> generation 2 -> verifier 2;
3. three grounding rejections -> exactly three generator/verifier calls -> no finalization -> persisted grounding_rejected;
4. verifier provider error -> one generation/verifier call and no corrective generation;
5. repository-patch failure followed by grounding rejection shares the same three-generator budget; and
6. existing generation-only correction behavior remains bounded.

### Step 2: Run the focused tests and confirm RED

    go test ./internal/sitefix -run 'TestCanonicalApplyCorrectsGroundingRejection|TestCanonicalApplyBoundsGroundingCorrection|TestCanonicalApplyDoesNotCorrectVerifierProviderFailure|TestCanonicalApplySharesCorrectionBudget'

Expected: FAIL because grounding still returns outside the generation loop.

### Step 3: Implement the unified loop

Refactor ApplyService.Apply to:

- select and load repository source once;
- run at most 1 + maxGenerationCorrectionRounds generator attempts;
- create repository feedback after explicitly correctable generation errors;
- verify each deterministically valid plan in the same loop;
- create grounding feedback only from a complete grounding_rejected decision;
- set the next generation's causedByCallID to the rejected verifier call ID;
- return provider, persistence, lifecycle, and invariant errors immediately;
- finalize once, after an approved verification; and
- return a wrapped grounding sentinel on exhaustion.

Do not create nested retry loops or reload repository source inside a correction attempt.

### Step 4: Run focused and package tests and confirm GREEN

    gofmt -w internal/sitefix/apply.go internal/sitefix/apply_test.go internal/sitefix/repository_apply_test.go
    go test ./internal/sitefix

Expected: PASS, including all pre-existing source-selection and fail-closed tests.

### Step 5: Commit

    git add internal/sitefix/apply.go internal/sitefix/apply_test.go internal/sitefix/repository_apply_test.go
    git commit -m "fix: correct rejected Site Fix patches"

## Task 3: Persist bounded verifier outcomes atomically

**Files:**

- Create: internal/migrations/0085_ai_call_verifier_outcome_add.sql
- Create: internal/migrations/0086_ai_call_verifier_outcome_validate.sql
- Create: internal/db/ai_call_verifier_outcome_contract_test.go
- Modify: internal/db/queries/ai_calls.sql
- Modify: internal/db/ai_call_reclaim_contract_test.go
- Regenerate: internal/db/models.go
- Regenerate: internal/db/ai_calls.sql.go
- Modify: internal/sitefix/apply.go
- Modify: internal/sitefix/apply_test.go

### Step 1: Write failing migration and query contract tests

Assert that:

- verifier_outcome is nullable JSONB with no default and no backfill;
- a NOT VALID check permits non-null outcomes only on fix_grounding_verification rows and only for JSON objects;
- validation is isolated in migration 0086;
- FinishCanonicalAICallFenced writes the outcome in the same update as terminal status/accounting;
- first non-null outcome wins, so cleanup or late duplicate finishes cannot overwrite it; and
- non-verifier finish calls leave it null.

Extend the reclaim contract test to require outcome preservation.

### Step 2: Run DB tests and confirm RED

    go test ./internal/db -run 'TestAICallVerifierOutcome|TestFinishCanonicalAICallFenced'

Expected: FAIL because the migrations, column, and query assignment are absent.

### Step 3: Add migrations and source SQL

0085 must use the repository's online-migration pattern:

- 5-second lock timeout and 30-second statement timeout;
- add nullable verifier_outcome jsonb without a default;
- add the stage/object check NOT VALID; and
- reset timeouts.

0086 validates only that constraint under the same timeouts.

Extend FinishCanonicalAICallFenced with a nullable outcome argument and a verifier-stage, first-write-preserving assignment.

### Step 4: Regenerate sqlc and inspect generated scope

    make sqlc
    git diff -- internal/db/models.go internal/db/ai_calls.sql.go internal/db/querier.go

Expected: AiCallRecord and generated query scan/parameter lists include VerifierOutcome; the querier method signature remains compatible. Do not hand-edit generated files.

### Step 5: Add the bounded outcome domain object and store wiring

Define GroundingVerificationOutcome with:

- schema version;
- correction round;
- generator call ID;
- decision flags and bounded lists/reason;
- normalized, bounded touched paths; and
- SHA-256 patch/diff fingerprints.

Change ApplyStore.FinishGroundingVerification to receive an optional outcome. Build it for every complete approved or rejected decision and pass nil for provider/incomplete responses. Marshal it only in PostgresApplyStore; source-selection and generation finishes pass nil to the generic fenced finish.

Update stubs to assert rejected outcomes survive even when a later correction succeeds.

### Step 6: Run focused tests and confirm GREEN

    gofmt -w internal/db/ai_call_verifier_outcome_contract_test.go internal/db/ai_call_reclaim_contract_test.go internal/sitefix/apply.go internal/sitefix/apply_test.go
    go test ./internal/db ./internal/sitefix

If a local Postgres integration target is available, additionally run the fenced-ledger integration case and prove duplicate finish cannot overwrite the first outcome.

### Step 7: Commit

    git add internal/migrations/0085_ai_call_verifier_outcome_add.sql internal/migrations/0086_ai_call_verifier_outcome_validate.sql internal/db/queries/ai_calls.sql internal/db/ai_call_verifier_outcome_contract_test.go internal/db/ai_call_reclaim_contract_test.go internal/db/models.go internal/db/ai_calls.sql.go internal/db/querier.go internal/sitefix/apply.go internal/sitefix/apply_test.go
    git commit -m "feat: audit Site Fix verifier outcomes"

## Task 4: Add structured rejection logs and truthful API errors

**Files:**

- Modify: internal/sitefix/apply.go
- Modify: internal/sitefix/apply_test.go
- Modify: internal/api/handlers_site_fixes.go
- Modify: internal/api/site_fixes_api_test.go

### Step 1: Write failing logging and API tests

Add tests that prove:

- every complete grounding rejection emits one structured warning containing Site Fix ID, verifier call ID, correction round, decision flags, bounded reason, and artifact fingerprints;
- raw patch/diff and unbounded model response never appear in the log;
- a direct or wrapped ErrPatchGroundingRejected returns HTTP 422 with code=site_fix_grounding_rejected and the safe public message;
- the private verifier reason is absent from the API body; and
- unexpected errors still return the existing redacted HTTP 500 response.

### Step 2: Run focused tests and confirm RED

    go test ./internal/sitefix ./internal/api -run 'TestCanonicalApplyLogsGroundingRejection|TestDoctorSiteFixGroundingRejection'

Expected: FAIL because the service has no logger and the handler has no grounding case.

### Step 3: Implement logging and error mapping

- Add an optional slog.Logger to ApplyService and wire Server.Log through both production constructors.
- Log after the verifier outcome has been finished in the ledger.
- Add a coded JSON error helper local to the Site Fix handler or a small reusable response helper.
- Map the grounding sentinel before the generic error branch to HTTP 422 and the stable code/message.
- Keep all other error mappings unchanged.

### Step 4: Run focused tests and confirm GREEN

    gofmt -w internal/sitefix/apply.go internal/sitefix/apply_test.go internal/api/handlers_site_fixes.go internal/api/site_fixes_api_test.go
    go test ./internal/sitefix ./internal/api

### Step 5: Commit

    git add internal/sitefix/apply.go internal/sitefix/apply_test.go internal/api/handlers_site_fixes.go internal/api/site_fixes_api_test.go
    git commit -m "fix: report Site Fix grounding failures truthfully"

## Task 5: Distinguish patch preparation from PR and deployment retries in the drawer

**Files:**

- Modify: web/app/lib/site-fix-pr-progress.ts
- Modify: web/app/lib/site-fix-pr-progress.test.mjs
- Modify: web/app/projects/[id]/site-fixes/site-fixes-client.tsx
- Modify: web/app/lib/action-portfolio-contract.test.mjs
- Modify: web/app/lib/seo-client-contract.test.mjs

### Step 1: Write failing Web behavior tests

Assert that:

- preparing plus site_fix.failure_reason produces Retry patch preparation / Retrying patch preparation...;
- preparing plus only application.failure_reason retains Retry PR creation;
- grounding_rejected maps to safe approved-evidence copy rather than exposing the raw code alone;
- preparation failure progress text says patch preparation is incomplete;
- the preparation failure drawer does not render deployment retry counts; and
- deployment/verification states still show their retry budget.

Update string-contract tests so they distinguish the two retry paths.

### Step 2: Run focused Web tests and confirm RED

    cd web
    node --test app/lib/site-fix-pr-progress.test.mjs app/lib/action-portfolio-contract.test.mjs app/lib/seo-client-contract.test.mjs

Expected: FAIL because every preparing failure currently uses PR wording and always renders 0/3.

### Step 3: Implement the UI semantics

- Add small pure helpers for preparation-failure detection and safe failure copy.
- Give canonical Site Fix preparation failures precedence over application PR failures only when the former is present.
- Update success/error toasts in applyFix to use preparation wording for Site Fix preparation retries.
- Render the retry-budget card and detail only outside preparation-failure state.
- Preserve existing PR/open/deploy/verify actions and polling behavior.

### Step 4: Run Web tests and type checking and confirm GREEN

    cd web
    npm test
    npm run typecheck

Expected: all tests and TypeScript checks pass.

### Step 5: Commit

    git add web/app/lib/site-fix-pr-progress.ts web/app/lib/site-fix-pr-progress.test.mjs 'web/app/projects/[id]/site-fixes/site-fixes-client.tsx' web/app/lib/action-portfolio-contract.test.mjs web/app/lib/seo-client-contract.test.mjs
    git commit -m "fix: clarify Site Fix preparation retries"

## Task 6: Full local verification and independent review

**Files:**

- Review all branch changes against docs/superpowers/specs/2026-07-13-site-fix-grounding-recovery-design.md

### Step 1: Run formatting and generated-file checks

    gofmt -w internal/sitefix/apply.go internal/sitefix/grounding_verifier.go internal/sitefix/apply_test.go internal/sitefix/repository_apply_test.go internal/api/handlers_site_fixes.go internal/api/site_fixes_api_test.go internal/db/ai_call_verifier_outcome_contract_test.go internal/db/ai_call_reclaim_contract_test.go
    make sqlc
    git diff --check

Expected: no diff drift after regeneration and no whitespace errors.

### Step 2: Run full verification from fresh commands

    go test ./...
    go vet ./...
    go build ./...
    cd web && npm test
    cd web && npm run typecheck
    cd web && npm run build

Expected: every command exits 0.

### Step 3: Request independent review

Have an independent reviewer compare the branch against the design and inspect:

- shared retry budget and all terminal paths;
- fail-closed finalization/GitHub boundary;
- outcome redaction and immutable first-write behavior;
- error taxonomy/API leakage;
- migration rolling-deploy safety; and
- Web distinction between preparation, PR, and deployment retries.

Address actionable findings test-first, then rerun the affected package and full verification.

### Step 4: Commit verification-only corrections, if any

Use a focused commit message describing only the reviewed correction. Do not create an empty commit.

## Task 7: Publish, merge, deploy, and prove production behavior

### Step 1: Synchronize with latest origin/main

Fetch, inspect new upstream changes, rebase or merge only if needed, and rerun the complete verification suite after resolving any overlap.

### Step 2: Push and create a ready CiteLoop PR

Push codex/fix-pr-creation-root-cause, open a ready PR against origin/main, and include:

- the confirmed production call sequence and root cause;
- the bounded correction/audit design;
- RED/GREEN tests and full verification commands; and
- the explicit production acceptance procedure.

### Step 3: Wait for checks and merge

Do not merge on a failing required check. Diagnose any failure from logs, fix it on the branch, push, and wait again. Merge the CiteLoop PR only after required checks and review are acceptable.

### Step 4: Wait for both production deployments

Record the merge SHA and wait until:

- Railway citeloop-api reports a successful deployment of that SHA; and
- Vercel citeloop reports READY and the production alias serves that SHA.

Confirm API and Web health before mutation.

### Step 5: Execute the authorized production acceptance test

In citeloop.app, open Site Fix d327f8c5-74ea-4215-a0b0-a2002a69c489 and retry patch preparation once.

Pass criteria:

- no generic service-unavailable toast;
- any initial grounding rejection is visible in the internal structured ledger/log and drives a bounded correction;
- a verifier attempt eventually succeeds;
- an application finalizes and GitHub is reached;
- exactly one real repair PR exists in bugfreev587/unipost;
- the drawer exposes the canonical Open PR URL; and
- the generated branch, base, files, and diff match the approved Site Fix.

Do not merge the generated UniPost PR.

### Step 6: Loop on any production gap

If any criterion fails, preserve the exact Site Fix/call/deployment IDs, diagnose the next boundary, create a clean follow-up from the then-current origin/main, test, merge, redeploy, and repeat until all pass criteria are met.

Only report completion after the production acceptance test passes, and include the merged CiteLoop PR URL plus the generated UniPost PR URL.
