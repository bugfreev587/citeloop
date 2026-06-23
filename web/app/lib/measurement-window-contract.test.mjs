import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("SEO action list renders structured measurement checkpoints", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const snippet of [
    "measurementWindowLabel",
    "measurement_window?.checkpoints",
    "measurement_window?.checkpoints_days",
    "primary_metric",
    "D+",
    "Scheduled",
  ]) {
    assert.match(seo, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});
