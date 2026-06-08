import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadAdminAccessModule() {
  const source = await readFile(new URL("./admin-access.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

test("canUseInternalTools denies regular Clerk users in production", async () => {
  const { canUseInternalTools } = await loadAdminAccessModule();

  assert.equal(
    canUseInternalTools({
      userId: "user_regular",
      sessionClaims: {},
      adminUserIDs: "",
      clerkSecretKey: "sk_live_example",
    }),
    false,
  );
});

test("canUseInternalTools allows explicit admins and Clerk admin claims", async () => {
  const { canUseInternalTools } = await loadAdminAccessModule();

  assert.equal(
    canUseInternalTools({
      userId: "user_admin",
      sessionClaims: {},
      adminUserIDs: "user_one, user_admin",
      clerkSecretKey: "sk_live_example",
    }),
    true,
  );

  assert.equal(
    canUseInternalTools({
      userId: "user_role",
      sessionClaims: { org_role: "org:admin" },
      adminUserIDs: "",
      clerkSecretKey: "sk_live_example",
    }),
    true,
  );

  assert.equal(
    canUseInternalTools({
      userId: "user_perm",
      sessionClaims: { org_permissions: ["org:admin"] },
      adminUserIDs: "",
      clerkSecretKey: "sk_live_example",
    }),
    true,
  );
});

test("canUseInternalTools allows local development without Clerk", async () => {
  const { canUseInternalTools } = await loadAdminAccessModule();

  assert.equal(
    canUseInternalTools({
      userId: null,
      sessionClaims: null,
      adminUserIDs: "",
      clerkSecretKey: "",
    }),
    true,
  );
});
