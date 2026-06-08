import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("project onboarding form offers a project link after partial creation failures", async () => {
  const source = await readFile(new URL("../project-create-form.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("createdProject"), true);
  assert.equal(source.includes("Open project"), true);
});

test("project onboarding form does not wait for long-running insight or SEO sync", async () => {
  const source = await readFile(new URL("../project-create-form.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("api.runInsight"), false);
  assert.equal(source.includes("api.syncSEO"), false);
  assert.equal(source.includes("router.push(`/projects/${project.id}`)"), true);
});
