#!/usr/bin/env node

const fs = require("node:fs");
const { execFileSync } = require("node:child_process");

const marker = "<!-- claude-review-fix-loop -->";
const repo = process.env.GITHUB_REPOSITORY;
const eventPath = process.env.GITHUB_EVENT_PATH;
const requestedMaxLoops = Number(process.env.MAX_FIX_LOOPS || "2");
const maxLoops = Number.isFinite(requestedMaxLoops) && requestedMaxLoops > 0
  ? Math.floor(requestedMaxLoops)
  : 2;

function setOutput(name, value) {
  const outputPath = process.env.GITHUB_OUTPUT;
  if (!outputPath) {
    return;
  }
  fs.appendFileSync(outputPath, `${name}=${String(value)}\n`);
}

function setOutputs(values) {
  for (const [name, value] of Object.entries(values)) {
    setOutput(name, value);
  }
}

function ghApi(args) {
  const env = {
    ...process.env,
    GH_TOKEN: process.env.GH_TOKEN || process.env.GITHUB_TOKEN,
  };
  const output = execFileSync("gh", ["api", ...args], {
    encoding: "utf8",
    env,
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
  return output ? JSON.parse(output) : null;
}

function resolvePullRequestNumber(event) {
  const inputPr = (process.env.INPUT_PR_NUMBER || "").trim();
  if (inputPr) {
    return Number(inputPr);
  }

  const workflowRun = event.workflow_run;
  const workflowRunPr = workflowRun?.pull_requests?.find((pullRequest) => {
    return pullRequest?.base?.repo?.full_name === repo || pullRequest?.head?.repo?.full_name === repo;
  }) || workflowRun?.pull_requests?.[0];

  if (workflowRunPr?.number) {
    return Number(workflowRunPr.number);
  }

  if (workflowRun?.head_sha) {
    const pulls = ghApi([
      `repos/${repo}/commits/${workflowRun.head_sha}/pulls`,
      "-H",
      "Accept: application/vnd.github+json",
    ]);
    const sameRepoPull = pulls.find((pullRequest) => pullRequest?.head?.repo?.full_name === repo) || pulls[0];
    if (sameRepoPull?.number) {
      return Number(sameRepoPull.number);
    }
  }

  return 0;
}

function main() {
  if (!repo) {
    throw new Error("GITHUB_REPOSITORY is required");
  }
  if (!eventPath) {
    throw new Error("GITHUB_EVENT_PATH is required");
  }

  const event = JSON.parse(fs.readFileSync(eventPath, "utf8"));
  const prNumber = resolvePullRequestNumber(event);

  if (!prNumber) {
    setOutputs({
      should_fix: "false",
      reason_code: "no_pr",
      reason: "Could not resolve a pull request for this workflow run.",
    });
    return;
  }

  const pullRequest = ghApi([`repos/${repo}/pulls/${prNumber}`]);
  const comments = ghApi([`repos/${repo}/issues/${prNumber}/comments?per_page=100`]);
  const files = ghApi([`repos/${repo}/pulls/${prNumber}/files?per_page=100`]);

  const loopCount = comments.filter((comment) => (comment.body || "").includes(marker)).length;
  const prdChanged = files.some((file) => /^docs\/.*prd.*\.md$/i.test(file.filename));

  const baseOutputs = {
    pr_number: prNumber,
    head_ref: pullRequest.head.ref,
    head_sha: pullRequest.head.sha,
    head_repo: pullRequest.head.repo.full_name,
    base_ref: pullRequest.base.ref,
    loop_count: loopCount,
    next_loop: loopCount + 1,
    max_loops: maxLoops,
    prd_changed: prdChanged ? "true" : "false",
    changed_files_count: files.length,
  };

  if (pullRequest.state !== "open") {
    setOutputs({
      ...baseOutputs,
      should_fix: "false",
      reason_code: "closed",
      reason: `PR #${prNumber} is not open.`,
    });
    return;
  }

  if (pullRequest.draft) {
    setOutputs({
      ...baseOutputs,
      should_fix: "false",
      reason_code: "draft",
      reason: `PR #${prNumber} is a draft.`,
    });
    return;
  }

  if (pullRequest.head.repo.full_name !== repo) {
    setOutputs({
      ...baseOutputs,
      should_fix: "false",
      reason_code: "external_branch",
      reason: `PR #${prNumber} comes from ${pullRequest.head.repo.full_name}; skipping write automation.`,
    });
    return;
  }

  if (loopCount >= maxLoops) {
    setOutputs({
      ...baseOutputs,
      should_fix: "false",
      reason_code: "max_loops",
      reason: `PR #${prNumber} already reached the ${maxLoops}-pass fixer limit.`,
    });
    return;
  }

  setOutputs({
    ...baseOutputs,
    should_fix: "true",
    reason_code: "ready",
    reason: `PR #${prNumber} is eligible for fixer pass ${loopCount + 1} of ${maxLoops}.`,
  });
}

try {
  main();
} catch (error) {
  console.error(error);
  process.exit(1);
}
