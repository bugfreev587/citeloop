import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

// PRD-CiteLoop-Opportunity-Review-and-Work-Queues 14.3.6/14.3.7 and
// PRD-CiteLoop-Workflow-Handoff-Link-Cards Phase 3: Review keeps a
// sent-to-publish link card, Publish keeps a view-results link card.

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
    "citeloop-linked-card-pulse",
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

test("Results opens the measurement item for a published article deep link", async () => {
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
    "window.setTimeout(() => setSelectedResultActionID(actionID), 900)",
    'highlighted ? "citeloop-linked-card-pulse border-[#d93820] ring-2 ring-[#d93820]/15"',
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  const focusStart = source.indexOf("const focusResultActionForHandoff");
  const focusEnd = source.indexOf("useEffect(() => {", focusStart);
  const focusBlock = source.slice(focusStart, focusEnd);
  assert.notEqual(focusStart, -1, "seo-client.tsx missing Results handoff focus helper");
  assert.notEqual(focusEnd, -1, "seo-client.tsx missing Results handoff focus helper boundary");
  assert.ok(
    focusBlock.indexOf("scrollIntoView") < focusBlock.indexOf("setSelectedResultActionID(actionID)"),
    "Results handoff should center and pulse the card before opening the drawer",
  );

  const actionHandoffStart = source.indexOf('if (mode !== "results" || !requestedResultActionID');
  const actionHandoffEnd = source.indexOf("// Publish handoff links land here", actionHandoffStart);
  const actionHandoffBlock = source.slice(actionHandoffStart, actionHandoffEnd);
  assert.equal(
    actionHandoffBlock.includes("focusResultActionForHandoff(requestedResultActionID)"),
    true,
    "Results ?action handoff should focus the linked card before opening drawer",
  );
  assert.equal(
    actionHandoffBlock.includes("setSelectedResultActionID(requestedResultActionID)"),
    false,
    "Results ?action handoff must not open the drawer before the card focus pulse",
  );

  const articleHandoffStart = source.indexOf('if (mode !== "results" || !requestedResultArticleID');
  const articleHandoffEnd = source.indexOf("useEffect(() => {", articleHandoffStart + 1);
  const articleHandoffBlock = source.slice(articleHandoffStart, articleHandoffEnd);
  assert.equal(
    articleHandoffBlock.includes("focusResultActionForHandoff(match.id)"),
    true,
    "Results ?article handoff should focus the linked card before opening drawer",
  );
  assert.equal(
    articleHandoffBlock.includes("setSelectedResultActionID(match.id)"),
    false,
    "Results ?article handoff must not open the drawer before the card focus pulse",
  );
});
