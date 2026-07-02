import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const exists = (relativePath) => fs.existsSync(path.join(appRoot, relativePath));

test("api client exposes SEO Doctor report, run, finding, and action methods", () => {
  const api = read("lib/api.ts");

  for (const contract of [
    "export type SEODoctorRunStatus",
    "export type SEODoctorStage",
    "export type SEODoctorRun",
    "export type SEODoctorFinding",
    "export type SEODoctorReport",
    "normalizeSEODoctorReport",
    "getSEODoctor",
    "getLatestSEODoctor",
    "startSEODoctorRun",
    "getSEODoctorRun",
    "listSEODoctorRunFindings",
    "convertSEODoctorFinding",
    "dismissSEODoctorFinding",
  ]) {
    assert.match(api, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("project shell exposes Doctor under the Home section", () => {
  const shell = read("components/project-shell.tsx");
  const primaryBlock = shell.slice(shell.indexOf('id: "primary"'), shell.indexOf('id: "analysis"'));

  assert.match(primaryBlock, /label: "Home"[\s\S]*label: "Doctor"/);
  assert.match(shell, /Stethoscope/);
  assert.match(shell, /href: "doctor"/);
});

test("Doctor route renders a client page with progress and per-finding AI repair handoff affordances", () => {
  assert.equal(exists("projects/[id]/doctor/page.tsx"), true, "doctor route should exist");
  assert.equal(exists("projects/[id]/doctor/doctor-client.tsx"), true, "doctor client should exist");
  const page = read("projects/[id]/doctor/page.tsx");
  const client = read("projects/[id]/doctor/doctor-client.tsx");

  assert.match(page, /DoctorClient/);
  for (const contract of [
    "progress_percent",
    "pages_checked",
    "Run Doctor",
    "Fix with AI",
    "buildAIRepairPayload",
    "copyAIRepairJSON",
    "selectedRepairFinding",
    "Codex",
    "Claude Code",
    "acceptance_tests",
    "rerun SEO Doctor",
    "convertSEODoctorFinding",
    "dismissSEODoctorFinding",
  ]) {
    assert.match(client, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.doesNotMatch(client, /Structured handoff/);
  assert.doesNotMatch(client, /title="AI coding report"/);
});

test("Home fetches and renders a first-fold Doctor module", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  assert.match(workspace, /getSEODoctor\(projectId\)/);
  assert.match(workspace, /doctorReport/);
  assert.match(workspace, /Site health/);
  assert.match(workspace, /\/projects\/\$\{projectId\}\/doctor/);
  assert.match(workspace, /progress_percent/);
});
