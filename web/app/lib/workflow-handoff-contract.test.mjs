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
    "Sent to Publish (",
    "View in Publish",
    "publish?article=${article.id}",
    "data-review-handoff-card",
  ]) {
    assert.equal(source.includes(expected), true, `review-client.tsx missing ${expected}`);
  }
  const handoffCardStart = source.indexOf("data-review-handoff-card");
  const handoffCardEnd = source.indexOf("</Link>", handoffCardStart);
  const handoffCard = source.slice(handoffCardStart, handoffCardEnd);
  assert.equal(handoffCard.includes("onApprove"), false, "sent-to-publish link cards must not re-expose approve");
});

test("Publish lanes link published work to Results and focus ?article= on the main Publish card", async () => {
  const source = await readFile(new URL("../projects/[id]/publishing/publishing-client.tsx", import.meta.url), "utf8");
  for (const expected of [
    "data-publish-results-link",
    "View Results",
    "results?article=${article.id}",
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
  assert.equal(handoffEffect.includes("scrollIntoView"), true, "Review handoff should scroll the main Publish card into view");
});

test("Results opens the measurement item for a published article deep link", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  for (const expected of [
    "requestedResultArticleID",
    'searchParams.get("article")',
    "draft_article_id === requestedResultArticleID",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
});
