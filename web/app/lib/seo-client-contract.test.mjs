import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("SEO page does not expose internal Google Search Console credential fields", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("GSC site URL"), false);
  assert.equal(source.includes("Credential ref"), false);
  assert.equal(source.includes("gsc_credential_ref"), false);
});
