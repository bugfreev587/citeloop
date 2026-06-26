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
  assert.match(menu, /Settings[\s\S]*size=\{25\}/);
  assert.match(menu, /KeyRound[\s\S]*size=\{23\}/);
  assert.match(menu, /Sun[\s\S]*size=\{22\}/);
  assert.match(menu, /bottom-full/);
  assert.match(menu, /border-t border-slate-200/);
  assert.doesNotMatch(menu, /Playbook|Get support|Follow us|Join the community|referral/);
});
