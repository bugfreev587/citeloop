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

test("Analysis handoff cards expose accessible names with title and destination", async () => {
  const source = await read("projects/[id]/seo/seo-client.tsx");

  assert.match(source, /aria-label=\{`Open "\$\{loopActionTitle\(action as any\)\}" in Content Plan`\}/);
  assert.match(source, /aria-label=\{`Open "\$\{loopActionTitle\(action as any\)\}" in Site Fixes`\}/);
});

test("Content Plan keeps Sent to Review handoff link cards for drafted topics", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");

  for (const marker of [
    "data-content-plan-recently-sent",
    "data-content-plan-sent-card",
    "Recently sent ({sentToReviewTopics.length})",
    "Sent to Review",
    "reviewArticleByTopic",
    'topic.status === "drafted" && reviewArticleByTopic[topic.id]',
  ]) {
    assert.equal(source.includes(marker), true, `topics-client.tsx missing ${marker}`);
  }

  assert.match(
    source,
    /review\?article=\$\{reviewArticleByTopic\[topic\.id\]\}/,
    "sent topic card must deep-link to the draft article in Review",
  );
  assert.match(source, /aria-label=\{`Open "\$\{topic\.title\}" in Review`\}/);
  assert.match(source, /<a[\s\S]{0,200}data-content-plan-sent-card/, "sent topic card must be a link, not a button or details");
});

test("Sent to Review cards carry no current-layer actions", async () => {
  const source = await read("projects/[id]/topics/topics-client.tsx");
  const sectionStart = source.indexOf("data-content-plan-recently-sent");
  const section = source.slice(sectionStart, source.indexOf("</details>", sectionStart));
  assert.ok(section.length > 0, "recently sent section must exist");

  for (const forbidden of ["Schedule", "Archive", "Draft now", "onClick", "aria-expanded", "RightDrawer"]) {
    assert.equal(section.includes(forbidden), false, `sent card section must not contain ${forbidden}`);
  }
});
