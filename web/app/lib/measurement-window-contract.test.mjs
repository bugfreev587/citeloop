import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("Growth actions retain measurement checkpoints while canonical Site Fixes do not", () => {
  // Growth-owned action surfaces still use structured measurement checkpoints.
  const lib = read("lib/site-fix.ts");
  for (const snippet of [
    "measurementWindowLabel",
    "measurement_window?.checkpoints",
    "measurement_window?.checkpoints_days",
    "primary_metric",
    "D+",
    "Scheduled",
  ]) {
    assert.match(lib, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  // Doctor-owned repair loops end at verification and must not inherit Growth measurement state.
  const siteFixes = read("projects/[id]/site-fixes/site-fixes-client.tsx");
  assert.doesNotMatch(siteFixes, /measurementWindowLabel\(/);
  assert.doesNotMatch(siteFixes, /measurement_window/);
  assert.match(siteFixes, /canonicalSiteFixNextAction/);
  assert.match(siteFixes, /Verification/);
});
