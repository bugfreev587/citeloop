import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("SEO action API type exposes multi-surface metadata", () => {
  const api = read("lib/api.ts");
  for (const field of [
    "asset_type?: string | null",
    "target_surface_id?: string | null",
    "risk_reasons?: any",
    "evidence_snapshot?: any",
    "diff_snapshot?: any",
    "review_required?: boolean",
    "verified_at?: any",
    "verification_snapshot?: any",
    "measurement_window?: any",
  ]) {
    assert.ok(api.includes(field), `api.ts missing ${field}`);
  }
  assert.ok(
    api.includes("body: { action_type?: string; asset_type?: string; work_type?: string; review_required?: boolean } = {}"),
    "createSEOContentAction body type should accept asset_type and review_required",
  );
});

test("SEO action list renders asset, review, and verification metadata", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const copy of ["Asset", "Review", "Verification", "Measurement"]) {
    assert.ok(seo.includes(copy), `seo-client missing ${copy}`);
  }
  for (const field of [
    "action.asset_type",
    "action.review_required",
    "action.verification_snapshot",
    "action.measurement_window",
  ]) {
    assert.ok(seo.includes(field), `seo-client missing ${field}`);
  }
});
