# Site Fix Grounding Recovery Design

## Goal

Make canonical Site Fix preparation recover safely when the independent grounding verifier rejects a generated repository patch. A rejected patch must feed a bounded, structured explanation back into generation, and GitHub PR creation must remain fail closed until a corrected patch passes verification.

The production acceptance target is the existing Site Fix for `https://unipost.dev/blog/evidence-led-social-publishing-api-planning-brief`: after the CiteLoop change deploys, retrying that fix must create a real repair PR in `bugfreev587/unipost` and expose its `Open PR` action. CiteLoop will not merge that generated UniPost PR.

## Confirmed Production Failure

The Jul 13 production retry did not fail in GitHub:

1. repository source selection succeeded;
2. repository patch generation succeeded;
3. the grounding verifier returned a complete model response and recorded `grounding_rejected`;
4. canonical application finalization did not run; and
5. GitHub branch, commit, and PR creation were never reached.

The current implementation corrects only `invalid_response` and `invalid_repository_patch` generation failures. Grounding verification runs once after that loop. When it rejects a patch, its parsed `reason` and decision fields are discarded, the request fails immediately, and a later manual retry starts over without feedback.

The API has no mapping for `ErrPatchGroundingRejected`, so this expected domain rejection falls through to HTTP 500 with `Doctor Site Fix service is temporarily unavailable`. The drawer also displays the deployment-verification retry budget (`0/3`) beside a preparation failure and labels the action as PR creation even though no application has been finalized.

## Chosen Approach

Use one bounded generation budget spanning both repository-patch correction and grounding correction.

This keeps the independent verifier as a fail-closed safety boundary while giving the generator actionable feedback when a candidate patch violates that boundary. It was chosen over:

1. enriching one finding family's evidence while retaining one-shot verification, which would leave the systemic retry and observability gap in place; and
2. relaxing or bypassing the verifier, which could allow unrelated edits, intent drift, or unsupported claims into a real repository PR.

## Control Flow

Each apply request performs source selection and loads one immutable repository snapshot. It then receives at most three total generation attempts: the initial attempt plus two correction attempts.

For each attempt:

1. Start a generation AI-call record caused by the preceding source-selection, generation, or verification call.
2. Generate a repository patch using optional typed correction feedback.
3. Apply the patch deterministically and validate the resulting canonical application plan.
4. If generation fails with `invalid_response` or `invalid_repository_patch`, finish the generation call, derive typed generation feedback, and continue only if budget remains.
5. If generation succeeds, start an independent grounding-verification call caused by that generation call.
6. Parse and deterministically validate the verifier decision.
7. If verification succeeds, finalize the application and continue into existing GitHub PR creation.
8. If verification returns `grounding_rejected`, finish and audit the verifier call, derive typed grounding feedback, and continue only if budget remains.
9. If either provider fails, an invariant is broken, persistence fails, or the error is otherwise not explicitly correctable, stop immediately.

There is no nested generation-times-verification retry loop. Three generator calls is the absolute request budget. Verification occurs only after a generator produces a deterministically valid patch, so the number of verifier calls is at most three.

The expected causal ledger is:

`source selection -> generation -> grounding verification -> corrective generation -> grounding verification`

On exhaustion, the final error still wraps `ErrPatchGroundingRejected`; preparation remains fail closed and no application or GitHub mutation is created.

## Typed Correction Feedback

Replace the generator's untyped rejection string with a bounded internal feedback value containing:

- failure kind: `repository_patch` or `grounding`;
- safe error code;
- concise explanation;
- for grounding only, the verifier's approval and intent flags;
- normalized added, removed, and unsupported proposition lists.

The value is internal and must be length- and count-bounded before it enters a new model prompt or log record.

Repository-patch feedback keeps the current exact-match guidance: copy `old_text` byte-for-byte from the supplied source and ensure it occurs exactly once.

Grounding feedback receives separate guidance: remove unrelated files or replacements, preserve the approved primary intent and proposition set, avoid unsupported claims or source-association changes, and address the verifier's bounded explanation. It must not append the unrelated `old_text` advice unless the failure also came from deterministic repository-patch validation.

The next generation descriptor and request fingerprint include only the semantic typed feedback. AI-call UUIDs remain in the ledger's `caused_by_call_id` relationship and do not enter the model prompt or request fingerprint, so equivalent semantic requests retain equivalent fingerprints while every physical call remains independently auditable.

## Verifier Decision and Error Type

The verifier returns its parsed `PatchVerification` even when deterministic validation rejects it. The apply service wraps the sentinel in a typed grounding-rejection error that retains the bounded decision for correction and logging while preserving `errors.Is(err, ErrPatchGroundingRejected)` behavior.

Provider errors and incomplete verifier responses remain distinct. They never enter the grounding correction path because there is no trustworthy decision to correct against.

The deterministic verifier predicate remains unchanged: approval and primary-intent preservation are required, intent drift is forbidden, no propositions or unsupported claims may be added or removed, and the preserved proposition set must equal the approved set.

## Audit Persistence

Add one nullable JSONB outcome field to `ai_call_records`. It stores only a bounded, structured verifier outcome:

- schema version;
- correction round;
- generator call ID;
- approval and intent flags;
- normalized added, removed, and unsupported lists;
- bounded verifier reason;
- touched source paths; and
- SHA-256 fingerprints of the patch and actual diff.

It does not store prompts, complete repository source, model raw responses, credentials, or tokens. Non-verifier calls leave the field null.

Finishing a grounding-verification record persists status, accounting, error code, and this outcome atomically. A rejected decision must be auditable even when a later correction succeeds. Existing append-only call rows and `caused_by_call_id` remain the source of attempt ordering.

The backend also emits a structured log for each grounding rejection with the Site Fix ID, verifier call ID, round, flags, bounded reason, and artifact fingerprints. This permits immediate deployment investigation without exposing the detail through the public API.

## API and Drawer Semantics

If all bounded grounding corrections are exhausted, `writeDoctorSiteFixError` maps the sentinel to HTTP 422 and a stable public error code. The message explains that generated changes did not satisfy the approved Site Fix constraints. It does not claim the service is unavailable and does not expose the model's private reason.

Infrastructure failures retain their existing mappings. In particular, provider, database, repository, readiness, and GitHub failures must not be converted into grounding rejection.

For a `preparing` fix with a preparation failure:

- the footer action is `Retry patch preparation`;
- the progress message says the repository patch could not be prepared safely;
- `grounding_rejected` receives a concise approved-evidence explanation; and
- the drawer does not display the post-deployment `retry_count / max_retries` as a preparation-attempt counter.

Once an application is ready and actual PR creation fails, the existing PR-specific retry wording remains appropriate.

## Concurrency and Idempotency

This change does not broaden mutation authority or change the GitHub idempotency design. The existing lifecycle and PR claim fences remain authoritative.

All corrective calls operate on the same immutable base commit and selected source blobs for one request. A source conflict discovered later in the publisher path continues to trigger existing fresh-source recovery rather than reusing a stale patch. Application finalization remains atomic and happens exactly once after a verified plan.

## Testing Strategy

Implementation follows test-driven development.

### Apply service

- First verifier rejects with a structured reason, the second generator receives grounding-specific feedback, the second verifier approves, and finalization occurs once.
- Generation and verification call IDs form the expected causal chain.
- Repeated grounding rejection consumes at most three generation attempts, persists every verifier outcome, returns the sentinel, and never finalizes.
- A verifier provider error is terminal and does not trigger corrective generation.
- Existing invalid-response and invalid-repository-patch correction behavior stays within the shared budget.
- A mixed sequence of repository-patch rejection followed by grounding rejection still uses only three total generator calls.

### Generator and verifier

- Grounding feedback produces grounding-specific prompt guidance and a distinct request fingerprint.
- Repository-patch feedback retains exact-replacement guidance.
- Parsed rejected decisions preserve the bounded reason and fields.
- Invalid or incomplete verifier output remains `invalid_response`, not `grounding_rejected`.

### Persistence

- Migration and query contract tests cover the nullable JSONB outcome field.
- Successful and rejected verifier outcomes persist bounded structured metadata and artifact fingerprints.
- Other AI-call stages leave the field null.

### API and Web

- Exhausted grounding rejection returns the stable HTTP 422 response and safe message.
- Unexpected errors remain redacted HTTP 500 responses.
- Preparing failures use preparation-specific action and status copy.
- Deployment verification retry counts remain visible only in deployment/verification states.
- Existing ready-for-PR and GitHub PR retry behavior is unchanged.

## Verification and Release

Before publication, run the full Go test suite, Web test suite, Web type checking, and production Web build. Review the branch independently against this design, then create a CiteLoop PR, wait for required checks, merge it into `origin/main`, and verify both Railway and Vercel deploy the merged SHA.

Production verification then uses the existing Site Fix:

1. confirm the deployed frontend and API are healthy and report the merged revision;
2. open the existing Site Fix in `citeloop.app`;
3. choose `Retry patch preparation` once;
4. confirm the request progresses through a successful grounding decision;
5. confirm a real PR is created in `bugfreev587/unipost`;
6. confirm the drawer exposes the PR URL and no generic service-unavailable toast appears; and
7. inspect the generated PR to ensure the branch, base, touched files, and diff match the approved Site Fix.

The generated UniPost PR remains open for human review and is not merged automatically.

If production differs from the expected flow, retain the failed call IDs and structured outcomes, diagnose the new boundary, implement a follow-up fix from the latest `origin/main`, and repeat deployment and production verification.

## Non-Goals

- Bypassing or weakening grounding invariants.
- Retrying provider, authentication, database, or GitHub failures as if they were patch corrections.
- Increasing scheduled Doctor authority.
- Writing directly to a configured repository base branch.
- Automatically merging the generated UniPost repair PR.
- Exposing verifier reasoning or repository source through the public Site Fix API.
