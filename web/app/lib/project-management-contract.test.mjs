import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const exists = (relativePath) => fs.existsSync(path.join(appRoot, relativePath));

test("project shell opens account and projects management from the footer popover", () => {
  const shell = read("components/project-shell.tsx");
  const footer = shell.slice(shell.indexOf('className="mt-auto grid gap-2"'));

  assert.doesNotMatch(shell, /FolderKanban/);
  assert.doesNotMatch(shell, /function isProjectsActive/);
  assert.doesNotMatch(footer, />\s*Projects\s*</);
  assert.doesNotMatch(footer, /href="\/projects"/);
  assert.match(shell, /import \{ ProjectAccountMenu \} from "\.\/project-account-menu"/);
  assert.match(footer, /<ProjectAccountMenu[\s\S]*project=\{project\}[\s\S]*projectId=\{projectId\}[\s\S]*isPlatformAdmin=\{isPlatformAdmin\}/);
  assert.doesNotMatch(shell, /UserButton/);
});

test("project account menu owns project list and account actions", () => {
  assert.equal(
    exists("components/project-account-menu.tsx"),
    true,
    "Dashboard footer should own project management in an upward popover",
  );

  const menu = read("components/project-account-menu.tsx");
  for (const copy of [
    "Projects",
    "Account Settings",
    "Admin",
    "Theme",
    "Light",
    "Dark",
    "Log out",
  ]) {
    assert.match(menu, new RegExp(copy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.match(menu, /api\.listProjects\(\)/);
  assert.match(menu, /openUserProfile\(\)/);
  assert.match(menu, /signOut/);
  assert.match(menu, /Settings[\s\S]*size=\{20\}/);
  assert.match(menu, /KeyRound[\s\S]*size=\{19\}/);
  assert.match(menu, /Sun[\s\S]*size=\{18\}/);
  assert.match(menu, /bottom-full/);
  assert.match(menu, /border-t border-slate-200/);
  assert.doesNotMatch(menu, /Playbook|Get support|Follow us|Join the community|referral/);
});

test("project account menu uses compact popover sizing aligned with dashboard navigation", () => {
  const menu = read("components/project-account-menu.tsx");

  assert.match(menu, /w-\[320px\]/, "Popover should be narrower than the previous wide account panel");
  assert.match(menu, /text-\[13px\]/, "Primary menu rows should be one size smaller than the text-sm sidebar nav");
  assert.doesNotMatch(menu, /text-\[17px\]/, "Popover actions should not use oversized text");
  assert.match(menu, /min-h-\[40px\]/, "Account action rows should be more compact than the old 52px rows");
  assert.match(menu, /h-\[34px\]/, "Theme controls should be compact");
  assert.doesNotMatch(menu, /w-\[436px\]|min-h-\[52px\]|h-11/, "Old loose sizing should not return");
});
