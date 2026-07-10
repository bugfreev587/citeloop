import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("SEO action list renders structured measurement checkpoints", () => {
  // The measurement checkpoint label logic moved to the shared site-fix helper.
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
  // The Site Fixes surface renders the structured checkpoint label.
  const siteFixes = read("projects/[id]/site-fixes/site-fixes-client.tsx");
  assert.match(siteFixes, /measurementWindowLabel\(/);
});
