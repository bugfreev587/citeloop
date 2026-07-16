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

test("Publish links published work to Results and focuses ?article= on the main Publish card", async () => {
  const source = await readFile(new URL("../projects/[id]/publishing/publishing-client.tsx", import.meta.url), "utf8");
  for (const expected of [
    "data-publish-published-section",
    "data-publish-recent-drawer-trigger",
    'dataAttribute="publish-recent-drawer"',
    "Recently Published",
    "data-publish-results-link",
    "data-publish-recent-card",
    "View Results",
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
  assert.equal(recentDrawer.includes("onClose={() => setDrawer(null)}"), true, "Recently Published card clicks should close the drawer");

  const publishedSectionStart = source.indexOf("function PublishedSection");
  const publishedSectionEnd = source.indexOf("function OperationalRows", publishedSectionStart);
  assert.notEqual(publishedSectionStart, -1, "publishing-client.tsx missing PublishedSection");
  assert.notEqual(publishedSectionEnd, -1, "publishing-client.tsx missing PublishedSection boundary");
  const publishedSection = source.slice(publishedSectionStart, publishedSectionEnd);
  assert.equal(publishedSection.includes("Open live article"), false, "Recently Published cards must not expose nested live-article actions");
  assert.equal(publishedSection.includes('target="_blank"'), false, "Recently Published cards must be single-destination cards without nested external links");
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
