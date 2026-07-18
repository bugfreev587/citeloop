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

  const sentBlock = source.slice(source.indexOf("const sentOpportunityLinks"), source.indexOf("const opportunityRecentCount"));
  assert.ok(sentBlock.length > 0, "sentOpportunityLinks derivation must exist");
  assert.doesNotMatch(sentBlock, /slice\(0,\s*\d+\)/, "sent handoff cards must not be silently capped; overflow scrolls in-group");

  assert.match(source, /const activeHandoffStages = new Set\(\["added_to_plan", "planned"\]\)/);
  assert.doesNotMatch(
    source,
    /const activeHandoffStages = new Set\(\[[^\]]*"drafting"/,
    "Analysis Recently Decided must not keep grandparent cards after work moves into Review",
  );
  assert.doesNotMatch(
    source,
    /const activeHandoffStages = new Set\(\[[^\]]*"ready_for_review"/,
    "Analysis Recently Decided must not keep grandparent cards after work moves into Review",
  );
});

test("Analysis loop and sent cards exclude actions hidden by their destination queues", async () => {
  const source = await read("projects/[id]/seo/seo-client.tsx");
  const loopBlock = source.slice(source.indexOf("const activeOpportunities"), source.indexOf("const opportunityRecentCount"));
  const helperBlock = source.slice(source.indexOf("const inactiveLoopActionStatuses"), source.indexOf("function isRecentlySentAction"));

  assert.ok(loopBlock.length > 0, "loop action derivation block must exist");
  assert.ok(helperBlock.length > 0, "visible loop helper must exist before sent handoff filtering");

  // Site Fixes moved to their own page, so seo-client no longer derives
  // `directReviewActionsAll`; the shared visible-loop filtering still stands.
  for (const marker of [
    "function isVisibleLoopAction",
    "hasDismissedSourceOpportunity(action)",
    "!hasResultsExecutionEvidence(action)",
    "const visibleLoopActions = loopActions.filter(isVisibleLoopAction)",
    "visibilityLifecycleCounts(visibleLoopActions)",
    "visibleLoopActions.filter((action) => deriveVisibilityLifecycleStage(action) === selectedLoopStage)",
    "const sentOpportunityLinks = visibleLoopActions",
  ]) {
    assert.equal(source.includes(marker), true, `seo-client.tsx missing ${marker}`);
  }

  assert.match(helperBlock, /dismissed/, "visible loop helper must recognize dismissed source states");
  assert.match(helperBlock, /archived/, "visible loop helper must recognize archived source states");
});

test("Analysis Recently Decided cards use current-surface routing instead of stale destination routing", async () => {
  const source = await read("projects/[id]/seo/seo-client.tsx");
  const sentSectionStart = source.indexOf("{sentOpportunityLinks.map((action) => {");
  const sentSectionEnd = source.indexOf("{watchingOpportunityLinks.map", sentSectionStart);
  const sentSection = source.slice(sentSectionStart, sentSectionEnd);

  assert.ok(sentSection.length > 0, "recently sent card render block must exist");
  assert.match(source, /if \(stage === "approved" && action\.draft_article_id\) return "Publish"/);
  assert.match(source, /if \(surface === "Publish"\) return `\/projects\/\$\{projectId\}\/publish\?article=\$\{action\.draft_article_id\}`/);
  assert.match(sentSection, /loopActionCurrentHref\(projectId, action as LoopAction\)/);
  assert.match(sentSection, /loopActionCurrentLabel\(action as LoopAction\)/);
  assert.doesNotMatch(sentSection, /actionHandoffHref\(projectId, action\)/);
  assert.doesNotMatch(sentSection, /destinationForAction\(action\)/);
});

test("Analysis handoff cards expose accessible names with title and destination", async () => {
  const source = await read("projects/[id]/seo/seo-client.tsx");

  assert.match(source, /aria-label=\{`Open "\$\{loopActionTitle\(action as any\)\}" in \$\{label\}`\}/);
  // Site Fixes handoff is no longer a special-cased on-page focus button; every
  // handoff card is a plain <Link> that routes to its current surface.
  assert.match(source, /<Link[\s\S]{0,240}data-opportunity-handoff-card[\s\S]{0,240}href=\{href\}/);
  assert.equal(source.includes("focusSiteFixCard"), false, "Site Fixes handoff should not focus an on-page card");
});

test("Analysis Recently Decided lives in a right drawer opened from the section header", async () => {
  const source = await read("projects/[id]/seo/seo-client.tsx");

  for (const expected of [
    "data-opportunity-recent-drawer-trigger",
    'dataAttribute="opportunity-recent-drawer"',
    "Recently Decided",
    "setAnalysisRecentDrawer(\"opportunities\")",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  const queueStart = source.indexOf("data-analysis-growth-findings-section");
  const siteFixesStart = source.indexOf("data-site-fixes-queue");
  const queueSection = source.slice(queueStart, siteFixesStart);
  assert.doesNotMatch(queueSection, /<details[\s\S]*Recently sent/, "Opportunity Queue should not render inline Recently sent details");
});

test("Content Plan keeps Sent to Review handoff link cards for drafted content briefs", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");

  for (const marker of [
    "data-content-plan-recently-sent",
    "data-content-plan-recent-drawer-trigger",
    "data-content-plan-sent-card",
    'dataAttribute="content-plan-recent-drawer"',
    "Recently Drafted",
    "Sent to Review",
    "sentToReviewActions",
    "reviewArticleIDForAction(action",
    "hasAdvancedDraftHandoff(action)",
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
  assert.match(source, /const reviewHref = reviewArticleID \? reviewHrefForAction\(projectId, reviewArticleID\)/);
  assert.match(source, /href=\{reviewHref\}/);
  assert.match(source, /aria-label=\{`Open "\$\{contentPlanActionTitle\(action\)\}" in Review`\}/);
  assert.match(source, /aria-label=\{`Open "\$\{topic\.title\}" in Review`\}/);
  assert.match(source, /<a[\s\S]{0,200}data-content-plan-sent-card/, "sent topic card must be a link, not a button or details");
});

test("Analysis Recently Decided handoff cards use the shared responsive square-card grid", async () => {
  const source = await read("projects/[id]/seo/seo-client.tsx");
  const sectionStart = source.indexOf('dataAttribute="opportunity-recent-drawer"');
  const section = source.slice(sectionStart, source.indexOf("</RightDrawer>", sectionStart));

  assert.ok(section.length > 0, "analysis recently decided drawer must exist");
  assert.match(section, /data-opportunity-handoff-grid/);
  assert.match(section, /md:grid-cols-2/);
  assert.match(section, /xl:grid-cols-3/);
  assert.match(section, /min-h-\[220px\]/);
  assert.match(section, /flex h-full/);
  assert.doesNotMatch(section, /sm:flex-row/, "handoff cards should not keep the old full-row internal layout");
});

test("Content Plan Recently sent cards use the shared responsive square-card grid", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");
  const sectionStart = source.indexOf('dataAttribute="content-plan-recent-drawer"');
  const section = source.slice(sectionStart, source.indexOf("</RightDrawer>", sectionStart));

  assert.ok(section.length > 0, "content plan recently drafted drawer must exist");
  assert.match(section, /data-content-plan-sent-grid/);
  assert.match(section, /md:grid-cols-2/);
  assert.match(section, /xl:grid-cols-3/);
  assert.match(section, /min-h-\[220px\]/);
  assert.match(section, /flex h-full/);
  assert.doesNotMatch(section, /sm:flex-row/, "sent-to-review cards should not keep the old full-row internal layout");
});

test("Content Plan Recently Drafted lives in a right drawer opened from the section header", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");

  for (const expected of [
    "data-content-plan-recent-drawer-trigger",
    'dataAttribute="content-plan-recent-drawer"',
    "Recently Drafted",
    "setRecentDraftsDrawerOpen(true)",
  ]) {
    assert.equal(source.includes(expected), true, `topics-client.tsx missing ${expected}`);
  }

  const planStart = source.indexOf("data-content-plan-handoff-section");
  const legacyStart = source.indexOf("Legacy content briefs", planStart);
  const planSection = source.slice(planStart, legacyStart);
  assert.doesNotMatch(planSection, /<details[\s\S]*Recently sent/, "Content Plan should not render inline Recently sent details");
});

test("Sent to Review cards are link-only history cards, not action panels", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");
  const sectionStart = source.indexOf('dataAttribute="content-plan-recent-drawer"');
  const section = source.slice(sectionStart, source.indexOf("</RightDrawer>", sectionStart));
  assert.ok(section.length > 0, "recently sent section must exist");

  assert.match(section, /const reviewHref = reviewArticleID \? reviewHrefForAction\(projectId, reviewArticleID\)/);
  assert.match(section, /href=\{reviewHref\}/);
  assert.match(section, /data-content-plan-sent-card/);

  for (const forbidden of [
    "Move back to Opportunities",
    "Dismiss",
    'kind: "return"',
    'kind: "dismiss"',
    "<Button",
    "Schedule",
    "Archive",
    "Draft Content",
    "aria-expanded",
    "RightDrawer",
  ]) {
    assert.equal(section.includes(forbidden), false, `sent card section must not contain ${forbidden}`);
  }
});

test("Content Plan Recently Drafted uses a parent-only Review handoff filter", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");
  const sentStart = source.indexOf("const sentToReviewActions = useMemo");
  const acceptedStart = source.indexOf("const acceptedPlanActions = useMemo", sentStart);
  const sentBlock = source.slice(sentStart, acceptedStart);

  assert.match(sentBlock, /hasActiveReviewHandoff\(action, reviewArticleID\)/);
  assert.doesNotMatch(sentBlock, /hasDraftStartedHandoff\(action, reviewArticleID\)/);
});

test("Recently drawers display when each item entered that recent bucket", async () => {
  const seo = await read("projects/[id]/seo/seo-client.tsx");
  const topics = await read("projects/[id]/topics/topics-client.tsx");
  const review = await read("projects/[id]/review/review-client.tsx");
  const publishing = await read("projects/[id]/publishing/publishing-client.tsx");

  assert.match(seo, /handoffTimestampLabel\("Moved"/, "Recently Decided cards must show moved time");
  assert.match(seo, /handoffTimestampLabel\("Watching since"/, "Recently watched cards must show watch time");
  assert.match(topics, /handoffTimestampLabel\("Drafted"/, "Recently Drafted action cards must show draft time");
  assert.match(topics, /handoffTimestampLabel\("Drafted"/, "Recently Drafted topic cards must show draft time");
  assert.match(review, /handoffTimestampLabel\("Reviewed"/, "Recently Reviewed cards must show review time");
  assert.match(publishing, /Published \$\{formatDate\(row\.publishedAt\)\}/, "Recently Published cards must show publish time");
});

test("Review Recently Reviewed mirrors Publish ready canonical cards", async () => {
  const review = await read("projects/[id]/review/review-client.tsx");

  assert.match(review, /isPublishReadyCanonicalArticle/);
  assert.match(review, /approvedArticles\.filter\(\(article\) => isPublishReadyCanonicalArticle\(article\)\)/);
  assert.doesNotMatch(
    review,
    /setSentToPublish\(approvedArticles\)/,
    "Recently Reviewed must not count approved syndication variants that have no Ready to post card",
  );
});

test("Publish disables Move back to Opportunities while actively publishing", async () => {
  const source = await read("projects/[id]/publishing/publishing-client.tsx");
  const readyStart = source.indexOf("function ReadyNowStrip");
  const readyEnd = source.indexOf("function SEODetailTile", readyStart);
  const readyBlock = source.slice(readyStart, readyEnd);
  const moveAriaLabelIndex = readyBlock.indexOf('aria-label={`Move "${item.title}" back to Opportunities`}');
  const moveButtonStart = readyBlock.lastIndexOf("<Button", moveAriaLabelIndex);
  const moveButtonEnd = readyBlock.indexOf("</Button>", moveAriaLabelIndex);
  const moveButton = readyBlock.slice(moveButtonStart, moveButtonEnd);

  assert.ok(readyBlock.length > 0, "ReadyNowStrip must exist");
  assert.ok(moveAriaLabelIndex >= 0, "ReadyNowStrip must render the move-back control");
  assert.match(
    moveButton,
    /disabled=\{Boolean\(busy\) \|\| item\.action === "publishing"\}/,
    "publishing cards must not allow a workflow rollback while the publisher is active",
  );
  assert.match(moveButton, /onClick=\{\(\) => onMoveBack\(item\.article\)\}/);
});

test("Content Plan draft success closes the drawer and moves started drafts out of active plan", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");

  for (const marker of [
    "hasDraftStartedHandoff(action",
    "markActionDraftStarted",
    "setSelectedContentPlanActionID(null)",
    "reviewArticleID ? reviewHrefForAction(projectId, reviewArticleID) :",
  ]) {
    assert.equal(source.includes(marker), true, `topics-client.tsx missing ${marker}`);
  }

  const recentSectionStart = source.indexOf('dataAttribute="content-plan-recent-drawer"');
  const recentSection = source.slice(recentSectionStart, source.indexOf("</RightDrawer>", recentSectionStart));
  assert.ok(recentSection.length > 0, "content plan recently drafted drawer must exist");
  assert.doesNotMatch(recentSection, /if \(!reviewArticleID\) return null/, "draft-started actions must stay visible even before a review article id is available");
});

test("Workflow cards display a stable short lineage ID across stages and recent drawers", async () => {
  const files = [
    "projects/[id]/seo/seo-client.tsx",
    "projects/[id]/topics/topics-client.tsx",
    "projects/[id]/review/review-client.tsx",
    "projects/[id]/publishing/publishing-client.tsx",
  ];
  for (const file of files) {
    const source = await read(file);
    assert.match(source, /workflowTraceLabel/, `${file} should render the shared short workflow ID`);
  }
});

test("Content Plan platform selector uses connected platform readiness instead of the legacy Both label", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");

  for (const marker of [
    "!capability.connection_ready",
    "platformBlockReason",
    "summarizeTargetSelectionPlatforms",
    "Selected platforms:",
  ]) {
    assert.equal(source.includes(marker), true, `topics-client.tsx missing ${marker}`);
  }

  assert.equal(source.includes("Blog / Syndication / Both is now a read-only summary"), false);
  assert.equal(source.includes("Recommended: {publishStrategyLabel(selectedActionRecommendedStrategy)}"), false);
});
