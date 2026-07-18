import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

// PRD-CiteLoop-Opportunity-Review-and-Work-Queues 14.3.6/14.3.7 and
// PRD-CiteLoop-Workflow-Handoff-Link-Cards Phase 3: Review keeps a
// sent-to-publish link card, Publish keeps a view-results link card.

test("cross-surface handoff cards share one persistent non-animated visual", async () => {
  const source = await readFile(new URL("../globals.css", import.meta.url), "utf8");
  const selector = ".citeloop-handoff-card-selected";
  const matches = source.match(/\.citeloop-handoff-card-selected\s*\{/g) ?? [];
  const ruleStart = source.indexOf(`${selector} {`);
  const ruleEnd = source.indexOf("}", ruleStart);
  const rule = source.slice(ruleStart, ruleEnd);

  assert.equal(matches.length, 1, "globals.css must define exactly one shared persistent handoff-card rule");
  assert.match(rule, /border-color:\s*#d93820/);
  assert.match(rule, /background-color:\s*#fff4f1/);
  assert.match(rule, /box-shadow:\s*0 0 0 2px rgb\(217 56 32 \/ 18%\)/);
  assert.doesNotMatch(rule, /animation/);
});

test("Content Plan handoffs persist until the selected action card is used", async () => {
  const source = await readFile(new URL("../projects/[id]/topics/topics-client.tsx", import.meta.url), "utf8");
  const handoffStart = source.indexOf("handledContentPlanActionHandoffRef.current === requestedActionID");
  const handoffEnd = source.indexOf("useEffect(() => {", handoffStart + 1);
  const handoffEffect = source.slice(handoffStart, handoffEnd);
  const cardStart = source.indexOf("data-content-plan-action-card");
  const cardEnd = source.indexOf("</button>", cardStart);
  const card = source.slice(cardStart, cardEnd);

  assert.notEqual(handoffStart, -1, "topics-client.tsx missing requested action handoff effect");
  assert.match(source, /const handledContentPlanActionHandoffRef = useRef<string \| null>\(null\)/);
  assert.match(
    handoffEffect,
    /handledContentPlanActionHandoffRef\.current === requestedActionID/,
    "Content Plan must ignore refreshed collection identities after handling the current query ID",
  );
  assert.match(handoffEffect, /if \(!target\) return/);
  assert.ok(
    handoffEffect.indexOf("handledContentPlanActionHandoffRef.current = requestedActionID") >
      handoffEffect.indexOf("if (!target) return"),
    "Content Plan must only mark a query ID handled after its target resolves",
  );
  assert.match(handoffEffect, /scrollIntoView/);
  assert.match(handoffEffect, /target\.focus/);
  assert.match(handoffEffect, /matchMedia\?\.\("\(prefers-reduced-motion: reduce\)"\)\?\.matches \?\? false/);
  assert.match(handoffEffect, /behavior: prefersReducedMotion \? "auto" : "smooth"/);
  assert.match(handoffEffect, /setHighlightContentPlanAction\(requestedActionID\)/);
  assert.doesNotMatch(
    handoffEffect,
    /setTimeout\(\(\) => setHighlightContentPlanAction\(null\)/,
    "Content Plan handoff highlight must not auto-clear",
  );
  assert.match(card, /citeloop-handoff-card-selected/);
  assert.doesNotMatch(card, /citeloop-linked-card-pulse/);
  assert.match(card, /aria-current=\{highlighted \? "true" : undefined\}/);
  assert.match(
    card,
    /onClick=\{\(\) => \{[\s\S]*setHighlightContentPlanAction\(null\);[\s\S]*setSelectedContentPlanActionID\(action\.id\);[\s\S]*\}\}/,
    "direct card use should consume the handoff before opening details",
  );
});

test("Review keeps approved drafts as Sent to Publish link cards", async () => {
  const source = await readFile(new URL("../projects/[id]/review/review-client.tsx", import.meta.url), "utf8");
  for (const expected of [
    "data-review-sent-to-publish",
    "data-review-recent-drawer-trigger",
    'dataAttribute="review-recent-drawer"',
    "Recently Reviewed",
    "View in Publish",
    "publish?article=${article.id}",
    "onClick={() => setRecentReviewedDrawerOpen(false)}",
    "data-review-handoff-card",
  ]) {
    assert.equal(source.includes(expected), true, `review-client.tsx missing ${expected}`);
  }
  const handoffCardStart = source.indexOf("data-review-handoff-card");
  const handoffCardEnd = source.indexOf("</Link>", handoffCardStart);
  const handoffCard = source.slice(handoffCardStart, handoffCardEnd);
  assert.equal(handoffCard.includes("onApprove"), false, "sent-to-publish link cards must not re-expose approve");

  const decisionStart = source.indexOf("data-review-decision-section");
  const inlineSentStart = source.indexOf("data-review-sent-to-publish", decisionStart);
  const drawerStart = source.indexOf('dataAttribute="review-recent-drawer"');
  assert.ok(drawerStart < inlineSentStart || inlineSentStart === -1, "Review sent cards should live in the recent drawer, not below the queue");
});

test("Publish exposes separate Results and published-page buttons and focuses ?article= on the main Publish card", async () => {
  const source = await readFile(new URL("../projects/[id]/publishing/publishing-client.tsx", import.meta.url), "utf8");
  for (const expected of [
    "data-publish-published-section",
    "data-publish-recent-drawer-trigger",
    'dataAttribute="publish-recent-drawer"',
    "Recently Published",
    "data-publish-results-link",
    "data-publish-live-link",
    "data-publish-live-unavailable",
    "data-publish-recent-card",
    "View Results",
    "Open Published Page",
    "Published Page Unavailable",
    "results?article=${row.articleId}",
    "onClose={() => setDrawer(null)}",
    "data-publish-ready-article-card={item.articleId}",
    "citeloop-handoff-card-selected",
    'searchParams.get("article")',
    "highlightedPublishArticleId === item.articleId",
  ]) {
    assert.equal(source.includes(expected), true, `publishing-client.tsx missing ${expected}`);
  }
  const handoffEffectStart = source.indexOf("A review handoff link lands here with ?article=");
  const handoffEffectEnd = source.indexOf("async function saveMode", handoffEffectStart);
  assert.notEqual(handoffEffectStart, -1, "publishing-client.tsx missing Review handoff focus effect");
  assert.notEqual(handoffEffectEnd, -1, "publishing-client.tsx missing Review handoff focus effect boundary");
  const handoffEffect = source.slice(handoffEffectStart, handoffEffectEnd);
  assert.equal(handoffEffect.includes('setDrawer("view_all")'), false, "Review handoff must not open the View all drawer");
  assert.equal(handoffEffect.includes("setDrawer(null)"), true, "Review handoff should close any open Publish drawer before focusing the target card");
  assert.equal(handoffEffect.includes("scrollIntoView"), true, "Review handoff should scroll the main Publish card into view");
  assert.equal(handoffEffect.includes("target.focus"), true, "Review handoff should focus the main Publish card");
  assert.equal(
    handoffEffect.includes("setHighlightedPublishArticleId(linkedArticleId)"),
    true,
    "Review handoff should persistently highlight the main Publish card",
  );
  assert.equal(
    handoffEffect.includes("setTimeout(() => setHighlightedPublishArticleId(null)"),
    false,
    "Review handoff highlight must not auto-clear",
  );
  assert.match(source, /const handledPublishArticleHandoffRef = useRef<string \| null>\(null\)/);
  assert.match(
    handoffEffect,
    /handledPublishArticleHandoffRef\.current === linkedArticleId/,
    "Publish must ignore refreshed collection identities after handling the current query ID",
  );
  assert.match(handoffEffect, /if \(!target\) return/);
  assert.ok(
    handoffEffect.indexOf("handledPublishArticleHandoffRef.current = linkedArticleId") >
      handoffEffect.indexOf("if (!target) return"),
    "Publish must only mark a query ID handled after its target resolves",
  );
  assert.ok(
    handoffEffect.indexOf("setDrawer(null)") >
      handoffEffect.indexOf("handledPublishArticleHandoffRef.current = linkedArticleId"),
    "Publish must not close a user-opened drawer before resolving an unhandled target",
  );
  assert.match(
    handoffEffect,
    /const consumePublishHandoff = useCallback\(\(\) => \{[\s\S]*setHighlightedPublishArticleId\(null\);[\s\S]*\}, \[linkedArticleId\]\)/,
    "any Ready to post interaction must consume the current Publish handoff, including peer-card operations",
  );
  assert.doesNotMatch(
    handoffEffect,
    /setHighlightedPublishArticleId\(\(current\)/,
    "Publish consumption must not depend on the interacted article matching the highlighted ID",
  );
  assert.match(source, /const pendingPublishHandoffTimerRef = useRef<number \| null>\(null\)/);
  const consumeStart = handoffEffect.indexOf("const consumePublishHandoff");
  const consumeEnd = handoffEffect.indexOf("useEffect(() => {", consumeStart);
  const consumeBlock = handoffEffect.slice(consumeStart, consumeEnd);
  assert.match(
    consumeBlock,
    /window\.clearTimeout\(pendingPublishHandoffTimerRef\.current\)/,
    "consuming a Publish handoff must cancel its pending focus timer",
  );
  assert.match(
    consumeBlock,
    /handledPublishArticleHandoffRef\.current = linkedArticleId/,
    "consuming a Publish handoff must mark the current query ID handled before a queued timer can replay it",
  );
  assert.ok(
    consumeBlock.indexOf("handledPublishArticleHandoffRef.current = linkedArticleId") <
      consumeBlock.indexOf("setHighlightedPublishArticleId(null)"),
    "the current query ID must be consumed before clearing its visible highlight",
  );
  assert.match(
    handoffEffect,
    /pendingPublishHandoffTimerRef\.current = focusTimer/,
    "Publish must retain the pending timer so direct interaction can cancel it",
  );

  const readyNowStart = source.indexOf("function ReadyNowStrip");
  const readyNowEnd = source.indexOf("function SEODetailTile", readyNowStart);
  const readyNow = source.slice(readyNowStart, readyNowEnd);
  assert.match(readyNow, /onConsumeHandoff/);
  assert.match(readyNow, /aria-current=\{highlighted \? "true" : undefined\}/);
  assert.match(readyNow, /citeloop-handoff-card-selected/);
  assert.doesNotMatch(readyNow, /citeloop-linked-card-pulse/);
  for (const operation of ["onSeoDetails", "onMoveBack", "onDestination", "onRetry", "onPublish"]) {
    assert.match(
      readyNow,
      new RegExp(`onConsumeHandoff\\(\\)[\\s\\S]{0,180}${operation.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`),
      `${operation} should consume the highlighted Publish handoff before running`,
    );
  }
  const operationalRowsStart = source.indexOf("function OperationalRows");
  const operationalRowsEnd = source.indexOf("export function PublishingClient", operationalRowsStart);
  const operationalRows = source.slice(operationalRowsStart, operationalRowsEnd);
  assert.doesNotMatch(
    operationalRows,
    /linkedArticleId|citeloop-linked-card-pulse/,
    "the closed operational drawer must not keep a second query-driven handoff highlight",
  );

  const recentDrawerStart = source.indexOf('dataAttribute="publish-recent-drawer"');
  const recentDrawerEnd = source.indexOf("</Drawer>", recentDrawerStart);
  assert.notEqual(recentDrawerStart, -1, "publishing-client.tsx missing Recently Published drawer");
  assert.notEqual(recentDrawerEnd, -1, "publishing-client.tsx missing Recently Published drawer boundary");
  const recentDrawer = source.slice(recentDrawerStart, recentDrawerEnd);
  assert.equal(recentDrawer.includes("onClose={() => setDrawer(null)}"), true, "Recently Published link clicks should close the drawer");

  const publishedSectionStart = source.indexOf("function PublishedSection");
  const publishedSectionEnd = source.indexOf("function OperationalRows", publishedSectionStart);
  assert.notEqual(publishedSectionStart, -1, "publishing-client.tsx missing PublishedSection");
  assert.notEqual(publishedSectionEnd, -1, "publishing-client.tsx missing PublishedSection boundary");
  const publishedSection = source.slice(publishedSectionStart, publishedSectionEnd);

  const cardMarker = publishedSection.indexOf("data-publish-published-article-card");
  const cardOpeningTag = publishedSection.lastIndexOf("<", cardMarker);
  assert.equal(
    publishedSection.slice(cardOpeningTag, cardMarker).trimStart().startsWith("<div"),
    true,
    "Recently Published cards must be plain containers rather than whole-card links",
  );
  assert.equal(
    publishedSection.match(/onClick=\{onClose\}/g)?.length,
    2,
    "both active destinations must close the recent drawer",
  );

  const resultsLinkStart = publishedSection.indexOf("data-publish-results-link");
  const resultsLinkEnd = publishedSection.indexOf("</Link>", resultsLinkStart);
  const liveLinkStart = publishedSection.indexOf("data-publish-live-link");
  const liveLinkEnd = publishedSection.indexOf("</a>", liveLinkStart);
  assert.ok(resultsLinkStart !== -1 && resultsLinkEnd !== -1, "Recently Published must render a Results link button");
  assert.ok(liveLinkStart > resultsLinkEnd && liveLinkEnd > liveLinkStart, "published-page link must be a sibling after the Results link");
  assert.ok(
    publishedSection.lastIndexOf("row.publishedUrl ? (", liveLinkStart) > resultsLinkEnd,
    "published-page link must only render when the row has a published URL",
  );

  const liveLink = publishedSection.slice(liveLinkStart, liveLinkEnd);
  assert.equal(liveLink.includes("href={row.publishedUrl}"), true, "published-page button must use the stored published URL");
  assert.equal(liveLink.includes('target="_blank"'), true, "published-page button must open in a new tab");
  assert.equal(liveLink.includes('rel="noopener noreferrer"'), true, "new-tab published-page link must isolate the opener");
  assert.equal(liveLink.includes("onClick={onClose}"), true, "published-page button must close the recent drawer");

  const unavailableStart = publishedSection.indexOf("data-publish-live-unavailable");
  const unavailableEnd = publishedSection.indexOf("</button>", unavailableStart);
  const unavailableButton = publishedSection.slice(unavailableStart, unavailableEnd);
  assert.equal(unavailableButton.includes("disabled"), true, "missing published URLs must render a disabled control");
  assert.equal(unavailableButton.includes("href="), false, "missing published URLs must not render navigation");

  const newlyPublishedEffectStart = source.indexOf("const nextIds = new Set(published.map((article) => article.id));");
  const newlyPublishedEffectEnd = source.indexOf("async function saveMode", newlyPublishedEffectStart);
  const newlyPublishedEffect = source.slice(newlyPublishedEffectStart, newlyPublishedEffectEnd);
  assert.equal(
    newlyPublishedEffect.includes("target.focus"),
    false,
    "newly published cards are static containers and must not receive programmatic focus",
  );
});

test("Results content handoffs persistently highlight without opening measurement details", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  for (const expected of [
    "requestedResultArticleID",
    'searchParams.get("article")',
    "draft_article_id === requestedResultArticleID",
    "useRouter",
    "consumedResultHandoffRef",
    "router.replace(`/projects/${projectId}/results`, { scroll: false })",
    "closeResultDrawer",
    "highlightedResultActionID",
    "focusResultActionForHandoff",
    "setHighlightedResultActionID(actionID)",
    'target.scrollIntoView({ behavior: prefersReducedMotion ? "auto" : "smooth", block: "center" })',
    "consumeResultHandoff",
    'highlighted ? "citeloop-handoff-card-selected"',
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  const focusStart = source.indexOf("const focusResultActionForHandoff");
  const focusEnd = source.indexOf("const closeResultSiteFixDrawer", focusStart);
  const focusBlock = source.slice(focusStart, focusEnd);
  assert.notEqual(focusStart, -1, "seo-client.tsx missing Results handoff focus helper");
  assert.notEqual(focusEnd, -1, "seo-client.tsx missing Results handoff focus helper boundary");
  assert.match(focusBlock, /setHighlightedResultActionID\(actionID\)/);
  assert.doesNotMatch(focusBlock, /setSelectedResultActionID\(actionID\)/);
  assert.doesNotMatch(focusBlock, /openTimer|clearTimer/);
  assert.doesNotMatch(focusBlock, /setHighlightedResultActionID\(null\)/);
  assert.match(focusBlock, /prefers-reduced-motion: reduce/);
  assert.match(source, /observedResultHandoffKeyRef/);
  const lifecycleStart = source.indexOf("observedResultHandoffKeyRef.current === requestedResultHandoffKey");
  const lifecycleEnd = source.indexOf("useEffect(() => {", lifecycleStart + 1);
  const lifecycle = source.slice(lifecycleStart, lifecycleEnd);
  assert.notEqual(lifecycleStart, -1, "Results must distinguish successive handoff query IDs");
  assert.match(lifecycle, /consumedResultHandoffRef\.current = null/);
  assert.match(lifecycle, /setHighlightedResultActionID\(null\)/);

  const actionHandoffStart = source.indexOf('if (mode !== "results" || !requestedResultActionID');
  const actionHandoffEnd = source.indexOf("// Publish handoff links land here", actionHandoffStart);
  const actionHandoffBlock = source.slice(actionHandoffStart, actionHandoffEnd);
  assert.equal(
    actionHandoffBlock.includes("focusResultActionForHandoff(requestedResultActionID)"),
    true,
    "Results ?action handoff should focus the linked card",
  );
  assert.equal(
    actionHandoffBlock.includes("setSelectedResultActionID(requestedResultActionID)"),
    false,
    "Results ?action handoff must not open the drawer",
  );

  const articleHandoffStart = source.indexOf('if (mode !== "results" || !requestedResultArticleID');
  const articleHandoffEnd = source.indexOf("useEffect(() => {", articleHandoffStart + 1);
  const articleHandoffBlock = source.slice(articleHandoffStart, articleHandoffEnd);
  assert.equal(
    articleHandoffBlock.includes("focusResultActionForHandoff(match.id)"),
    true,
    "Results ?article handoff should focus the linked card",
  );
  assert.equal(
    articleHandoffBlock.includes("setSelectedResultActionID(match.id)"),
    false,
    "Results ?article handoff must not open the drawer",
  );

  const resultCardsStart = source.indexOf("<SiteFixResultsCard");
  const resultCardsEnd = source.indexOf("</section>", resultCardsStart);
  const resultCards = source.slice(resultCardsStart, resultCardsEnd);
  assert.match(resultCards, /consumeResultHandoff\(\)[\s\S]*openResultSiteFix/);
  assert.match(resultCards, /consumeResultHandoff\(\)[\s\S]*setSelectedResultActionID\(action\.id\)/);
  assert.doesNotMatch(resultCards, /citeloop-linked-card-pulse/);
});

test("Results switches handoff kinds without stale highlights or pending focus callbacks", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const lifecycleStart = source.indexOf("if (!requestedResultHandoffKey)");
  const lifecycleEnd = source.indexOf("useEffect(() => {", lifecycleStart + 1);
  const resultLifecycle = source.slice(lifecycleStart, lifecycleEnd);
  const watchStart = source.indexOf("observedWatchOpportunityHandoffRef.current !== requestedWatchOpportunityID");
  const watchEnd = source.indexOf("const attributionMeasuredActions", watchStart);
  const watchLifecycle = source.slice(watchStart, watchEnd);
  const clearWatchStart = source.indexOf("const clearWatchHandoff");
  const clearWatchEnd = source.indexOf("const consumeWatchHandoff", clearWatchStart);
  const clearWatch = source.slice(clearWatchStart, clearWatchEnd);

  assert.notEqual(lifecycleStart, -1, "Results handoff lifecycle must exist");
  assert.ok(
    resultLifecycle.indexOf("if (!requestedResultHandoffKey)") <
      resultLifecycle.indexOf("clearWatchHandoff()"),
    "query stripping without a new handoff must preserve the current highlight",
  );
  assert.match(resultLifecycle, /clearResultHandoffTimers\(\)/);
  assert.match(resultLifecycle, /clearWatchHandoff\(\)/);
  assert.match(resultLifecycle, /setHighlightedResultActionID\(null\)/);

  assert.notEqual(watchStart, -1, "watch handoff lifecycle must exist");
  assert.match(watchLifecycle, /clearResultHandoffTimers\(\)/);
  assert.match(watchLifecycle, /setHighlightedResultActionID\(null\)/);
  assert.ok(
    watchLifecycle.indexOf("observedWatchOpportunityHandoffRef.current !== requestedWatchOpportunityID") <
      watchLifecycle.indexOf("clearResultHandoffTimers()"),
    "same-ID watch refreshes must not repeatedly clear or replay handoff state",
  );

  assert.notEqual(clearWatchStart, -1, "watch handoff cleanup helper must exist");
  assert.match(clearWatch, /cancelAnimationFrame\(watchOpportunityHandoffFrameRef\.current\)/);
  assert.match(clearWatch, /setHighlightedWatchOpportunityID\(null\)/);
});
