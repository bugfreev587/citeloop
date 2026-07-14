import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("growth stages expose the four approved manual modes", () => {
  const helper = read("lib/growth-stage.ts");
  for (const value of ["foundation", "traction", "scale", "optimize"]) assert.match(helper, new RegExp(`key: "${value}"`));
  for (const label of ["Foundation", "Traction", "Scale", "Optimize"]) assert.match(helper, new RegExp(`label: "${label}"`));
  assert.match(helper, /Accepted and in-progress Opportunities will not change/);
  assert.match(helper, /active watchlist candidate/);
});

test("Opportunity header owns version-safe stage selection and default notice", () => {
  const client = read("projects/[id]/seo/seo-client.tsx");
  const selector = read("projects/[id]/seo/growth-stage-selector.tsx");
  assert.match(`${client}\n${selector}`, /data-growth-stage-selector/);
  assert.match(client, /Default stage — confirm selection/);
  assert.match(client, /expected_version: growthStage\.setting_version/);
  assert.match(client, /growthStageConfirmation/);
  assert.match(client, /api\.updateGrowthStage/);
  assert.match(client, /rescore_status === "failed"/);
});

test("stage API uses the project Opportunity boundary", () => {
  const api = read("lib/api.ts");
  assert.match(api, /getGrowthStage:[\s\S]*\/opportunities\/stage/);
  assert.match(api, /updateGrowthStage:[\s\S]*expected_version/);
});
