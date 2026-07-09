import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");

test("Analysis Recently sent exit rule is event-driven, not time- or count-based", async () => {
  const source = await read("projects/[id]/seo/seo-client.tsx");

  assert.match(
    source,
    /return activeHandoffStages\.has\(deriveVisibilityLifecycleStage\(action\)\);/,
    "isRecentlySentAction must exit only when the downstream item advances past the handoff stages",
  );
  assert.doesNotMatch(source, /7 \* 24 \* 60 \* 60 \* 1000/, "handoff exit must not use a time window");

  const sentBlock = source.slice(source.indexOf("const sentOpportunityLinks"), source.indexOf("const selectedDirectAction"));
  assert.ok(sentBlock.length > 0, "sentOpportunityLinks derivation must exist");
  assert.doesNotMatch(sentBlock, /slice\(0,\s*\d+\)/, "sent handoff cards must not be silently capped; overflow scrolls in-group");
});

test("Analysis loop and sent cards exclude actions hidden by their destination queues", async () => {
  const source = await read("projects/[id]/seo/seo-client.tsx");
  const loopBlock = source.slice(source.indexOf("const activeOpportunities"), source.indexOf("const selectedDirectAction"));
  const helperBlock = source.slice(source.indexOf("const inactiveLoopActionStatuses"), source.indexOf("function isRecentlySentAction"));

  assert.ok(loopBlock.length > 0, "loop action derivation block must exist");
  assert.ok(helperBlock.length > 0, "visible loop helper must exist before sent handoff filtering");

  for (const marker of [
    "function isVisibleLoopAction",
    "hasDismissedSourceOpportunity(action)",
    "!hasResultsExecutionEvidence(action)",
    "const visibleLoopActions = loopActions.filter(isVisibleLoopAction)",
    "visibilityLifecycleCounts(visibleLoopActions)",
    "visibleLoopActions.filter((action) => deriveVisibilityLifecycleStage(action) === selectedLoopStage)",
    "const directReviewActionsAll = visibleLoopActions",
    "const sentOpportunityLinks = visibleLoopActions",
  ]) {
    assert.equal(source.includes(marker), true, `seo-client.tsx missing ${marker}`);
  }

  assert.match(helperBlock, /dismissed/, "visible loop helper must recognize dismissed source states");
  assert.match(helperBlock, /archived/, "visible loop helper must recognize archived source states");
});

test("Analysis Recently sent cards use current-surface routing instead of stale destination routing", async () => {
  const source = await read("projects/[id]/seo/seo-client.tsx");
  const sentSectionStart = source.indexOf("{sentOpportunityLinks.map((action) => {");
  const sentSectionEnd = source.indexOf("{watchingOpportunityLinks.map", sentSectionStart);
  const sentSection = source.slice(sentSectionStart, sentSectionEnd);

  assert.ok(sentSection.length > 0, "recently sent card render block must exist");
  assert.match(sentSection, /loopActionCurrentHref\(projectId, action as LoopAction\)/);
  assert.match(sentSection, /loopActionCurrentLabel\(action as LoopAction\)/);
  assert.doesNotMatch(sentSection, /actionHandoffHref\(projectId, action\)/);
  assert.doesNotMatch(sentSection, /destinationForAction\(action\)/);
});

test("Analysis handoff cards expose accessible names with title and destination", async () => {
  const source = await read("projects/[id]/seo/seo-client.tsx");

  assert.match(source, /aria-label=\{`Open "\$\{loopActionTitle\(action as any\)\}" in \$\{label\}`\}/);
  assert.match(source, /if \(label === "Site Fixes"\)/);
});

test("Analysis Recently sent starts collapsed by default", async () => {
  const source = await read("projects/[id]/seo/seo-client.tsx");
  const marker = "Recently sent ({sentOpportunityLinks.length + watchingOpportunityLinks.length})";
  const markerIndex = source.indexOf(marker);
  const detailsStart = source.lastIndexOf("<details", markerIndex);
  const section = source.slice(detailsStart, source.indexOf("</details>", markerIndex));
  assert.ok(section.length > 0, "analysis recently sent section must exist");

  assert.doesNotMatch(section, /<details[^>]*\sopen(?:=|\s|>)/, "Analysis Recently sent should not default open");
});

test("Content Plan keeps Sent to Review handoff link cards for drafted content briefs", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");

  for (const marker of [
    "data-content-plan-recently-sent",
    "data-content-plan-sent-card",
    "Recently sent ({sentToReviewActions.length + sentToReviewTopics.length})",
    "Sent to Review",
    "sentToReviewActions",
    "hasReviewableDraft(action)",
    "reviewArticleByTopic",
    'topic.status === "drafted" && reviewArticleByTopic[topic.id]',
  ]) {
    assert.equal(source.includes(marker), true, `topics-client.tsx missing ${marker}`);
  }

  assert.match(
    source,
    /review\?article=\$\{reviewArticleByTopic\[topic\.id\]\}/,
    "legacy sent topic card must deep-link to the draft article in Review",
  );
  assert.match(source, /href=\{reviewHrefForAction\(projectId, action\)\}/);
  assert.match(source, /aria-label=\{`Open "\$\{contentPlanActionTitle\(action\)\}" in Review`\}/);
  assert.match(source, /aria-label=\{`Open "\$\{topic\.title\}" in Review`\}/);
  assert.match(source, /<a[\s\S]{0,200}data-content-plan-sent-card/, "sent topic card must be a link, not a button or details");
});

test("Content Plan Recently sent starts collapsed by default", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");
  const sectionStart = source.indexOf("data-content-plan-recently-sent");
  const section = source.slice(sectionStart, source.indexOf("</details>", sectionStart));
  assert.ok(section.length > 0, "recently sent section must exist");

  assert.doesNotMatch(section, /<details[^>]*\sopen(?:=|\s|>)/, "Recently sent should not default open");
});

test("Sent to Review cards only expose review links and pre-publish reconsideration actions", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");
  const sectionStart = source.indexOf("data-content-plan-recently-sent");
  const section = source.slice(sectionStart, source.indexOf("</details>", sectionStart));
  assert.ok(section.length > 0, "recently sent section must exist");

  assert.match(section, /setPendingContentPlanConfirmation\(\{ kind: "return", action \}\)/);
  assert.match(section, /setPendingContentPlanConfirmation\(\{ kind: "dismiss", action \}\)/);
  assert.match(section, /href=\{reviewHrefForAction\(projectId, action\)\}/);

  for (const forbidden of ["Schedule", "Archive", "Draft Content", "aria-expanded", "RightDrawer"]) {
    assert.equal(section.includes(forbidden), false, `sent card section must not contain ${forbidden}`);
  }
});
