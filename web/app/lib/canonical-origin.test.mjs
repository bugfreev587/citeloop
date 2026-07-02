import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadCanonicalOriginModule() {
  const source = await readFile(new URL("./canonical-origin.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

test("canonicalAppURLForRequest moves production alias GitHub callbacks to citeloop.app", async () => {
  const { canonicalAppURLForRequest } = await loadCanonicalOriginModule();

  assert.equal(
    canonicalAppURLForRequest(
      "https://citeloop.vercel.app/integrations/github/callback?installation_id=140775789&setup_action=update&state=project-1",
    ),
    "https://citeloop.app/integrations/github/callback?installation_id=140775789&setup_action=update&state=project-1",
  );

  const redirectedSignIn = canonicalAppURLForRequest(
    "https://citeloop.vercel.app/sign-in?redirect_url=https%3A%2F%2Fciteloop.vercel.app%2Fintegrations%2Fgithub%2Fcallback%3Finstallation_id%3D140775789%26setup_action%3Dupdate%26state%3Dproject-1",
  );
  assert.equal(
    redirectedSignIn,
    "https://citeloop.app/sign-in?redirect_url=https%3A%2F%2Fciteloop.app%2Fintegrations%2Fgithub%2Fcallback%3Finstallation_id%3D140775789%26setup_action%3Dupdate%26state%3Dproject-1",
  );
});

test("canonicalAppURLForRequest leaves canonical and preview URLs alone", async () => {
  const { canonicalAppURLForRequest } = await loadCanonicalOriginModule();

  assert.equal(
    canonicalAppURLForRequest(
      "https://citeloop.app/integrations/github/callback?installation_id=140775789&setup_action=update&state=project-1",
    ),
    null,
  );
  assert.equal(
    canonicalAppURLForRequest(
      "https://citeloop-git-codex-fix-xiaobo-yus-projects.vercel.app/integrations/github/callback?installation_id=140775789&state=project-1",
    ),
    null,
  );
});
