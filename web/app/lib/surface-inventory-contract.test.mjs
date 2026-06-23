import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("GEO external surface type exposes generalized inventory metadata", () => {
  const api = read("lib/api.ts");
  for (const field of [
    "source_url?: string | null",
    "canonical_status: string",
    "indexability_status: string",
    "publication_status: string",
    "owner_confidence: string",
    "last_verified_at?: any",
    "verification_snapshot?: any",
    "related_action_ids: string[]",
  ]) {
    assert.match(api, new RegExp(field.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  for (const normalizer of [
    "source_url: data.source_url ?? null",
    "canonical_status: data.canonical_status ?? \"unknown\"",
    "indexability_status: data.indexability_status ?? \"unknown\"",
    "publication_status: data.publication_status ?? \"unknown\"",
    "owner_confidence: data.owner_confidence ?? \"medium\"",
    "related_action_ids: arrayFrom<string>(data.related_action_ids).map(String)",
  ]) {
    assert.match(api, new RegExp(normalizer.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.match(api, /source_url\?: string/);
  assert.match(api, /publication_status\?: string/);
  assert.match(api, /owner_confidence\?: string/);
});

test("SEO visibility page renders generalized surface inventory controls", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const copy of ["Owner", "Platform", "Publication", "Indexability", "Canonical", "Confidence", "Source URL"]) {
    assert.match(seo, new RegExp(copy));
  }
  for (const state of [
    "surfaceOwnerType",
    "surfacePlatform",
    "surfacePublicationStatus",
    "surfaceIndexabilityStatus",
    "surfaceCanonicalStatus",
    "surfaceOwnerConfidence",
    "surfaceSourceURL",
  ]) {
    assert.match(seo, new RegExp(state));
  }
  for (const payload of [
    "owner_type: surfaceOwnerType",
    "platform: surfacePlatform",
    "publication_status: surfacePublicationStatus",
    "indexability_status: surfaceIndexabilityStatus",
    "canonical_status: surfaceCanonicalStatus",
    "owner_confidence: surfaceOwnerConfidence",
    "source_url: surfaceSourceURL",
  ]) {
    assert.match(seo, new RegExp(payload.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});
