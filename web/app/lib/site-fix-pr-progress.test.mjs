import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadProgressModule() {
  let source = "export {};";
  try {
    source = await readFile(new URL("./site-fix-pr-progress.ts", import.meta.url), "utf8");
  } catch {
    // The first TDD run deliberately exercises the missing helper module.
  }
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  return import(`data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`);
}

test("only canonical HTTPS GitHub pull-request URLs are linkable", async () => {
  const progress = await loadProgressModule();
  assert.equal(typeof progress.validSiteFixPullRequestURL, "function");

  assert.equal(
    progress.validSiteFixPullRequestURL("https://github.com/citeloop/site/pull/42"),
    "https://github.com/citeloop/site/pull/42",
  );
  for (const unsafe of [
    "http://github.com/citeloop/site/pull/42",
    "https://github.example.com/citeloop/site/pull/42",
    "https://github.com/citeloop/site/issues/42",
    "https://github.com/citeloop/site/pull/not-a-number",
    "https://github.com/citeloop/site/pull/42/files",
    "https://github.com/citeloop/site/pull/42?diff=split",
    "https://user@github.com/citeloop/site/pull/42",
  ]) {
    assert.equal(progress.validSiteFixPullRequestURL(unsafe), "", unsafe);
  }
});

test("readiness blocks every state except a stored ready result", async () => {
  const progress = await loadProgressModule();
  assert.equal(typeof progress.siteFixReadinessGate, "function");

  assert.deepEqual(progress.siteFixReadinessGate({ readiness: { status: "ready" } }), {
    allowed: true,
    tone: "green",
    title: "GitHub is ready for repair PRs",
    detail: "CiteLoop can create a branch and pull request for this Site Fix.",
  });
  for (const input of [
    { readiness: null },
    { readiness: { status: "not_connected" } },
    { readiness: { status: "not_checked" } },
    { readiness: { status: "permission_missing" } },
    { readiness: { status: "repository_unavailable" } },
    { readiness: { status: "error" } },
    { readiness: { status: "future_unknown_state" } },
    { readiness: { status: "ready" }, loading: true },
    { readiness: { status: "ready" }, fetchError: "request failed" },
  ]) {
    const gate = progress.siteFixReadinessGate(input);
    assert.equal(gate.allowed, false, JSON.stringify(input));
    assert.ok(gate.title);
    assert.ok(gate.detail);
  }
});

test("Site Fix PR actions route proposed approval and historical retries correctly", async () => {
  const progress = await loadProgressModule();
  assert.equal(typeof progress.siteFixPullRequestAction, "function");

  assert.deepEqual(progress.siteFixPullRequestAction({ status: "proposed" }), {
    kind: "approve",
    label: "Approve fix",
    busyLabel: "Approving & creating PR...",
  });
  assert.deepEqual(progress.siteFixPullRequestAction({ status: "approved" }), {
    kind: "apply",
    label: "Create PR",
    busyLabel: "Creating PR...",
  });
  assert.equal(progress.siteFixPullRequestAction({ status: "preparing" }), null);
  assert.deepEqual(progress.siteFixPullRequestAction({ status: "preparing", failure_reason: "github_pr_failed" }), {
    kind: "apply",
    label: "Retry PR creation",
    busyLabel: "Retrying PR creation...",
  });
  assert.deepEqual(
    progress.siteFixPullRequestAction({ status: "preparing", application: { failure_reason: "pr_interrupted" } }),
    { kind: "apply", label: "Retry PR creation", busyLabel: "Retrying PR creation..." },
  );
  assert.deepEqual(progress.siteFixPullRequestAction({ status: "ready_to_apply" }), {
    kind: "apply",
    label: "Create PR",
    busyLabel: "Creating PR...",
  });
  assert.equal(progress.siteFixPullRequestAction({ status: "awaiting_deploy" }), null);
});

test("a validated PR URL is always the primary action and malicious URLs never are", async () => {
  const progress = await loadProgressModule();
  const href = "https://github.com/citeloop/site/pull/42";

  for (const status of ["applying", "awaiting_deploy", "failed_retryable", "reopened", "verified", "failed_terminal"]) {
    assert.deepEqual(progress.siteFixPullRequestAction({ status, application: { github_pr_url: href } }), {
      kind: "open_pr",
      label: "Open PR",
      href,
    });
  }
  assert.deepEqual(
    progress.siteFixPullRequestAction({
      status: "proposed",
      application: { github_pr_url: "https://github.com.attacker.test/citeloop/site/pull/42" },
    }),
    { kind: "approve", label: "Approve fix", busyLabel: "Approving & creating PR..." },
  );
  assert.deepEqual(
    progress.siteFixPullRequestMutationAction({
      status: "preparing",
      failure_reason: "github_pr_failed",
      application: { github_pr_url: href },
    }),
    { kind: "apply", label: "Retry PR creation", busyLabel: "Retrying PR creation..." },
  );
});

test("lifecycle milestones require deployed and verified evidence", async () => {
  const progress = await loadProgressModule();
  assert.equal(typeof progress.canonicalSiteFixMilestones, "function");

  assert.deepEqual(progress.canonicalSiteFixMilestones({ status: "approved", approved_at: "2026-07-13T10:00:00Z" }), [
    { label: "Finding", complete: true, current: false },
    { label: "Approved", complete: true, current: false },
    { label: "Applied / deploy", complete: false, current: true },
    { label: "Verified", complete: false, current: false },
  ]);
  for (const fix of [
    { status: "awaiting_deploy", approved_at: "now", applied_at: "now" },
    { status: "awaiting_deploy", approved_at: "now", application: { github_pr_state: "open" } },
    { status: "awaiting_deploy", approved_at: "now", application: { github_pr_state: "merged", merged_at: "now" } },
  ]) {
    assert.equal(progress.canonicalSiteFixMilestones(fix)[2].complete, false, JSON.stringify(fix));
  }
  const verifying = progress.canonicalSiteFixMilestones({ status: "verifying", application: { deployed_at: "now", verified_at: "now" } });
  assert.deepEqual(verifying[2], { label: "Applied / deploy", complete: true, current: false });
  assert.deepEqual(verifying[3], { label: "Verified", complete: true, current: false });
  for (const status of ["verifying", "verified"]) {
    assert.equal(progress.canonicalSiteFixMilestones({ status })[2].complete, true, `${status} proves deployment`);
  }
  for (const status of ["failed_retryable", "reopened"]) {
    const withoutDeployment = progress.canonicalSiteFixMilestones({
      status,
      applied_at: null,
      deployed_at: null,
      application: { deployed_at: null },
    });
    assert.deepEqual(withoutDeployment[2], { label: "Applied / deploy", complete: false, current: true });
    assert.deepEqual(withoutDeployment[3], { label: "Verified", complete: false, current: false });

    const withFixDeployment = progress.canonicalSiteFixMilestones({ status, deployed_at: "now" });
    assert.deepEqual(withFixDeployment[2], { label: "Applied / deploy", complete: true, current: false });
    assert.deepEqual(withFixDeployment[3], { label: "Verified", complete: false, current: true });

    const withApplicationDeployment = progress.canonicalSiteFixMilestones({ status, application: { deployed_at: "now" } });
    assert.deepEqual(withApplicationDeployment[2], { label: "Applied / deploy", complete: true, current: false });
  }
  assert.equal(progress.canonicalSiteFixMilestones({ status: "verifying", verified_at: "now" })[3].complete, true);
  const verified = progress.canonicalSiteFixMilestones({ status: "verified" });
  assert.deepEqual(verified[3], {
    label: "Verified",
    complete: true,
    current: false,
  });
  assert.equal(verified.some((milestone) => milestone.current), false);
});

test("polling remains active only for an open drawer with in-flight lifecycle work", async () => {
  const progress = await loadProgressModule();
  assert.equal(typeof progress.shouldPollSiteFixLifecycle, "function");

  const shouldPoll = (fix, drawerOpen = true) => progress.shouldPollSiteFixLifecycle({ drawerOpen, fix });
  assert.equal(shouldPoll({ status: "preparing" }), true);
  assert.equal(shouldPoll({ status: "applying", application: { status: "creating_pr" } }), true);
  assert.equal(shouldPoll({ status: "applying", application: { github_pr_state: "open" } }), true);
  assert.equal(shouldPoll({ status: "awaiting_deploy" }), true);
  assert.equal(shouldPoll({ status: "applying", application: { status: "deployment_pending" } }), true);
  assert.equal(shouldPoll({ status: "verifying" }), true);
  assert.equal(shouldPoll({ status: "applying", application: { status: "verification_pending" } }), true);

  assert.equal(shouldPoll({ status: "preparing" }, false), false);
  assert.equal(shouldPoll(null), false);
  assert.equal(shouldPoll({ status: "proposed" }), false);
  assert.equal(shouldPoll({ status: "approved" }), false);
  assert.equal(shouldPoll({ status: "preparing", failure_reason: "github_pr_failed" }), false);
  assert.equal(shouldPoll({ status: "preparing", application: { failure_reason: "pr_interrupted" } }), false);
  assert.equal(shouldPoll({ status: "applying", application: { status: "needs_follow_up" } }), false);
  assert.equal(shouldPoll({ status: "conflict" }), false);
  assert.equal(shouldPoll({ status: "failed_retryable" }), false);
  assert.equal(shouldPoll({ status: "reopened" }), false);
  assert.equal(shouldPoll({ status: "verified" }), false);
});

test("progress copy follows the whole pull-request and verification lifecycle", async () => {
  const progress = await loadProgressModule();
  assert.equal(typeof progress.canonicalSiteFixProgressText, "function");

  const cases = [
    [{ status: "creating_pr" }, "Creating the repair PR"],
    [{ status: "applying", application: { status: "creating_pr" } }, "Creating the repair PR"],
    [{ status: "open" }, "Waiting for PR review and merge"],
    [{ status: "applying", application: { status: "github_pr_open", github_pr_state: "open" } }, "Waiting for PR review and merge"],
    [{ status: "deployment_pending" }, "PR merged - waiting for deploy"],
    [{ status: "applying", application: { github_pr_state: "merged" } }, "PR merged - waiting for deploy"],
    [{ status: "applying", application: { status: "deployment_pending" } }, "PR merged - waiting for deploy"],
    [{ status: "awaiting_deploy" }, "PR merged - waiting for deploy"],
    [{ status: "verification_pending" }, "Checking the production change"],
    [{ status: "verifying" }, "Checking the production change"],
    [{ status: "applying", application: { status: "verification_pending" } }, "Checking the production change"],
    [{ status: "verified" }, "Verified"],
  ];
  for (const [fix, expected] of cases) {
    assert.equal(progress.canonicalSiteFixProgressText(fix), expected, JSON.stringify(fix));
  }
});

test("Site Fix client refreshes stored readiness but polling stays list-only", async () => {
  const source = await readFile(new URL("../projects/[id]/site-fixes/site-fixes-client.tsx", import.meta.url), "utf8");

  assert.match(source, /api\.getGithubPRReadiness\(projectId\)/);
  assert.match(source, /Promise\.allSettled/);
  assert.match(source, /SITE_FIX_POLL_INTERVAL_MS\s*=\s*10_000/);
  const pollStart = source.indexOf("const pollSiteFixes = useCallback");
  const pollEnd = source.indexOf("useEffect(() =>", pollStart);
  const pollSource = source.slice(pollStart, pollEnd);
  assert.notEqual(pollStart, -1);
  assert.match(pollSource, /api\.listDoctorSiteFixes\(projectId\)/);
  assert.match(pollSource, /pollRequestRef/);
  assert.doesNotMatch(pollSource, /\+\+listRequestRef\.current/, "polling must not invalidate a manual refresh");
  assert.doesNotMatch(pollSource, /getGithubPRReadiness|checkGithubPRReadiness|listPublisherConnections|method:\s*["']POST/);
  assert.doesNotMatch(pollSource, /setError\(/, "background poll failures must stay silent");
  assert.match(source, /window\.setInterval/);
  assert.match(source, /window\.clearInterval/);
  const reconcileStart = source.indexOf("const reconcileAfterMutationFailure = useCallback");
  const reconcileEnd = source.indexOf("const pollSelectedFix", reconcileStart);
  const reconcileSource = source.slice(reconcileStart, reconcileEnd);
  assert.notEqual(reconcileStart, -1);
  assert.match(reconcileSource, /await refresh\(\)/, "every failed PR mutation must reload fixes and stored readiness");
  assert.doesNotMatch(reconcileSource, /err\?\.status|pollSiteFixes/, "failed PR mutations must not leave persisted readiness stale");
  assert.equal(
    (source.match(/await reconcileAfterMutationFailure\(\)/g) ?? []).length,
    2,
    "both approve and apply failure paths must run the full reconciliation",
  );
});

test("Site Fix client exposes guarded PR progress without a manual-apply success path", async () => {
  const source = await readFile(new URL("../projects/[id]/site-fixes/site-fixes-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "Open PR",
    "Repository",
    "Base branch",
    "Working branch",
    "Base commit",
    "Head commit",
    "PR number",
    "PR state",
    "/projects/${projectId}/settings#publisher",
  ]) {
    assert.equal(source.includes(expected), true, `missing ${expected}`);
  }
  assert.match(source, /busyLabel=\{primaryMutationAction\.busyLabel/);
  assert.match(source, /siteFixPullRequestAction/);
  assert.match(source, /group-hover\/readiness:/);
  assert.match(source, /group-focus-within\/readiness:/);
  assert.match(source, /aria-disabled=\{blocked \|\| undefined\}/);
  assert.doesNotMatch(source, /\sdisabled=\{blocked/);
  assert.match(source, /role="note"/);
  assert.doesNotMatch(source, /role="tooltip"/);
  assert.match(source, /href=\{`\/projects\/\$\{projectId\}\/settings#publisher`\}/);
  assert.doesNotMatch(source, /href=\{drawerApplication\.github_pr_url\}/);
  assert.doesNotMatch(source, /Open source change/);
  assert.doesNotMatch(source, /manual_apply_required|I applied this manually|Manual application required/);
});
