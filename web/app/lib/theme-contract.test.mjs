import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const webRoot = path.resolve(appRoot, "..");
const readApp = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const readWeb = (relativePath) => fs.readFileSync(path.join(webRoot, relativePath), "utf8");

test("theme choice applies before hydration and enables Tailwind dark variants", () => {
  const layout = readApp("layout.tsx");
  const tailwind = readWeb("tailwind.config.ts");
  const menu = readApp("components/project-account-menu.tsx");
  const themeLib = readApp("lib/theme.ts");
  const landingActions = readApp("landing-auth-actions.tsx");
  const landingPage = readApp("page.tsx");

  assert.match(tailwind, /darkMode:\s*["']class["']/, "Tailwind dark variants should follow the html.dark class");
  assert.match(layout, /citeloop:theme/, "The initial theme should be read before React hydrates");
  assert.match(layout, /classList\.toggle\(["']dark["']/, "The initial theme should set html.dark");
  assert.match(layout, /classList\.toggle\(["']light["']/, "The initial theme should set html.light");
  assert.match(layout, /dataset\.theme/, "The initial theme should set html[data-theme]");
  assert.match(layout, /style\.colorScheme/, "The initial theme should align the browser color scheme");
  assert.match(themeLib, /THEME_STORAGE_KEY\s*=\s*"citeloop:theme"/, "Theme storage key should be shared");
  assert.match(themeLib, /classList\.toggle\(["']dark["']/, "Shared theme helper should update html.dark");
  assert.match(themeLib, /classList\.toggle\(["']light["']/, "Shared theme helper should update html.light");
  assert.match(themeLib, /style\.colorScheme/, "Shared theme helper should update color-scheme");
  assert.match(menu, /readStoredThemeChoice/, "The menu should read the shared theme state");
  assert.match(menu, /saveThemeChoice/, "The menu should save through the shared theme helper");
  assert.match(landingActions, /LandingThemeToggle/, "Landing should expose a theme toggle");
  assert.match(landingActions, /saveThemeChoice/, "Landing toggle should save through the shared theme helper");
  assert.match(landingPage, /LandingThemeToggle/, "Landing page should place the theme toggle in the header");
});

test("global stylesheet gives shared surfaces a dark-mode palette", () => {
  const css = readApp("globals.css");

  assert.match(css, /html\.dark,\s*html\[data-theme="dark"\]/, "Dark mode should work with both html.dark and data-theme");
  for (const cssVariable of [
    "--cl-page",
    "--cl-panel",
    "--cl-panel-muted",
    "--cl-border",
    "--cl-text-strong",
    "--cl-text-muted",
  ]) {
    assert.match(css, new RegExp(cssVariable), `Missing ${cssVariable} dark theme token`);
  }

  for (const utilityToken of [
    'class~="bg-white"',
    'class~="bg-stone-100"',
    'class~="bg-slate-50"',
    'class~="bg-amber-50"',
    'class~="bg-green-50"',
    'class~="bg-red-50"',
    'class~="bg-sky-50"',
    'class~="bg-violet-50"',
    'class~="border-slate-200"',
    'class~="border-amber-200"',
    'class~="border-red-200"',
    'class~="text-slate-950"',
    'class~="text-slate-900"',
    'class~="text-slate-600"',
    'class~="text-amber-900"',
    'class~="text-red-900"',
  ]) {
    assert.match(css, new RegExp(utilityToken.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")), `Missing dark override for ${utilityToken}`);
  }
});
