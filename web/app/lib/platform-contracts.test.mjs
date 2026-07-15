import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadModule() {
  const source = await readFile(new URL("./platform-contracts.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ES2020, target: ts.ScriptTarget.ES2020 } }).outputText;
  return import(`data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`);
}

const capability = (platform, overrides = {}) => ({
  platform, contract_id: `${platform}-contract`, contract_version: "v1", generation_supported: true,
  target_context_ready: true, connection_ready: true, publish_mode: "manual", output_type: "long_form_article",
  canonical_required: platform !== "blog", source_url_required_before_publish: platform !== "blog",
  image_roles_supported: [], block_reasons: [], ...overrides,
});

test("exact selection keeps canonical blog immutable and derives legacy summary", async () => {
  const { initialTargetSelection, summarizeTargetSelectionPlatforms, togglePlatformTarget, summarizeTargetSelection } = await loadModule();
  const capabilities = [capability("blog"), capability("dev_to")];
  let selection = initialTargetSelection("blog_post", capabilities);
  selection = togglePlatformTarget(selection, capabilities[1], true);
  assert.deepEqual(selection.target_platforms.map((item) => item.platform), ["blog", "dev_to"]);
  assert.equal(summarizeTargetSelection(selection), "both");
  assert.equal(summarizeTargetSelectionPlatforms(selection), "Blog + Dev.to");
  selection = togglePlatformTarget(selection, capabilities[0], false);
  assert.equal(selection.target_platforms[0].platform, "blog");
});

test("validation explains incompatible and stale-context targets", async () => {
  const { initialTargetSelection, togglePlatformTarget, validateTargetSelection } = await loadModule();
  const capabilities = [capability("blog"), capability("reddit", { target_context_ready: false, block_reasons: ["target_context_required"] })];
  let selection = initialTargetSelection("blog_post", capabilities);
  selection = togglePlatformTarget(selection, capabilities[1], true);
  const validation = validateTargetSelection(selection, capabilities);
  assert.equal(validation.valid, false);
  assert.match(validation.errors.join(" "), /Reddit rules/i);
});

test("validation blocks non-blog targets whose publisher connection is not ready", async () => {
  const { initialTargetSelection, togglePlatformTarget, validateTargetSelection } = await loadModule();
  const capabilities = [capability("blog"), capability("medium", { connection_ready: false })];
  let selection = initialTargetSelection("blog_post", capabilities);
  selection = togglePlatformTarget(selection, capabilities[1], true);
  const validation = validateTargetSelection(selection, capabilities);
  assert.equal(validation.valid, false);
  assert.match(validation.errors.join(" "), /Medium connection/i);
});

test("Reddit exact target pins the confirmed context revision", async () => {
  const { targetFromCapability } = await loadModule();
  const target = targetFromCapability(capability("reddit", { output_type: "community_post" }), {
    id: "ctx-1", target_key: "r/saas", version: 3, status: "confirmed", expires_at: "2026-08-01T00:00:00Z",
  });
  assert.equal(target.target_key, "r/saas");
  assert.equal(target.target_context_id, "ctx-1");
  assert.equal(target.target_context_version, 3);
});
