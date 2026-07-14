import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";

const root = join(process.cwd(), "app");
const read = (path) => readFileSync(join(root, path), "utf8");

test("Site Fix contracts own classification and a five-state measurement handoff", () => {
  const types = read("lib/types.ts");
  for (const marker of [
    "export type SiteFixType",
    "export type SiteFixImpactMode",
    "export type SiteFixMeasurementPolicy",
    "export type SiteFixMeasurementHandoffStatus",
    '"not_applicable"',
    '"not_started"',
    '"pending"',
    '"started"',
    '"failed"',
    "measurement_summary?: SiteFixMeasurementSummary | null",
    "measurement_handoff_status: SiteFixMeasurementHandoffStatus",
  ]) {
    assert.match(types, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("Site Fix presentation separates repair verification from optional Results measurement", () => {
  const client = read("projects/[id]/site-fixes/site-fixes-client.tsx");
  const presentation = read("lib/site-fix.ts");
  const progress = read("lib/site-fix-pr-progress.ts");
  assert.match(client, /siteFixMeasurementPresentation\(selected\)/);
  assert.match(client, /View Results/);
  for (const marker of [
    "Verification only",
    "Measurement pending",
    "Measurement started",
    "Measurement failed",
    "Prospective observation",
    "low-confidence",
  ]) {
  assert.match(presentation, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.match(progress, /return \["Finding", "Approved", "Applied \/ deploy", "Verified"\]/);
  assert.doesNotMatch(progress, /return \["Finding", "Approved", "Applied \/ deploy", "Measuring", "Verified"\]/);
});
