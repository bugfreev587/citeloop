import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const apiSource = await readFile(new URL("./api.ts", import.meta.url), "utf8");
const seoSource = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

test("Opportunities client uses the canonical loop APIs", () => {
  for (const route of [
    "/opportunities/runs",
    "/opportunities/status",
    "/opportunities${suffix}",
    "/growth-actions${suffix}",
    "/growth-actions/${actionID}/measurement",
    "/growth-learnings",
  ]) {
    assert.equal(apiSource.includes(route), true, `api.ts missing canonical route ${route}`);
  }
});

test("Opportunity review exposes the decision-ready hypothesis contract", () => {
  for (const label of [
    "Hypothesis",
    "Baseline",
    "Primary metric",
    "Expected direction",
    "Decision threshold",
    "Source freshness",
    "Measurement policy",
    "Absolute deadline",
  ]) {
    assert.equal(seoSource.includes(label), true, `seo-client.tsx missing ${label}`);
  }
});

test("Growth action details expose checkpoints, artifacts, outcomes, and learnings", () => {
  for (const label of ["Checkpoint role", "Linked artifact", "Growth learning", "Measurement quality", "Terminal reason"]) {
    assert.equal(seoSource.includes(label), true, `seo-client.tsx missing ${label}`);
  }
});

test("legacy discovery modes are not user-facing on Opportunities", () => {
  assert.equal(seoSource.includes("Signal Scan"), false);
  assert.equal(seoSource.includes("AI Discovery"), false);
});
