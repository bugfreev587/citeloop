import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const exists = (relativePath) => fs.existsSync(path.join(appRoot, relativePath));

test("api client exposes canonical read-only Doctor report, run, and finding methods", () => {
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
    "dismissSEODoctorFinding",
  ]) {
    assert.match(api, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.match(api, /`\/projects\/\$\{id\}\/doctor`/);
  assert.match(api, /`\/projects\/\$\{id\}\/doctor\/runs`/);
  assert.doesNotMatch(api, /\/seo\/doctor/);
  assert.doesNotMatch(api, /convertSEODoctorFinding/);
  assert.doesNotMatch(api, /startSEODoctorGrowthLoop/);
  assert.doesNotMatch(api, /start-growth-loop/);
});

test("project shell exposes Doctor under the Home section", () => {
  const shell = read("components/project-shell.tsx");
  const primaryBlock = shell.slice(shell.indexOf('id: "primary"'), shell.indexOf('id: "analysis"'));

  assert.match(primaryBlock, /label: "Home"[\s\S]*label: "Doctor"/);
  assert.match(shell, /Stethoscope/);
  assert.match(shell, /href: "doctor"/);
});

test("Doctor route renders read-only diagnosis with self-serve AI repair JSON", () => {
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
    "writeClipboardText",
    "selectedRepairFinding",
    "AI coding repair JSON",
    "Copy JSON",
    "acceptance_tests",
    "dismissSEODoctorFinding",
    "document.execCommand",
  ]) {
    assert.match(client, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  for (const forbidden of [
    "Codex",
    "Claude Code",
    "Create action",
    "convertSEODoctorFinding",
    "Start Growth Loop",
    "Select for Growth Loop",
    "selectedFindingIDs",
    "selectedGrowthLoopIDs",
    "startingGrowthLoop",
    "startGrowthLoop",
    "startSEODoctorGrowthLoop",
  ]) {
    assert.doesNotMatch(client, new RegExp(forbidden.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("AI repair JSON includes necessary website repair context without Growth Loop metadata", () => {
  const client = read("projects/[id]/doctor/doctor-client.tsx");
  const repairPayloadBlock = client.slice(client.indexOf("function repairEvidence"), client.indexOf("async function writeClipboardText"));
  const acceptanceBlock = client.slice(client.indexOf("function buildAIRepairAcceptanceTests"), client.indexOf("function buildAIRepairPayload"));

  for (const required of [
    "issue_type",
    "severity",
    "category",
    "affected_urls",
    "normalized_urls",
    "problem",
    "why_it_matters",
    "evidence",
    "page_url",
    "normalized_page_url",
    "status",
    "final_url",
    "confidence",
    "fix",
    "goal",
    "instructions",
    "likely_surfaces",
    "seo_contract",
    "risk_level",
    "acceptance_tests",
  ]) {
    assert.match(repairPayloadBlock, new RegExp(required.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  for (const internalOrGrowthMetadata of [
    "schema_version",
    "intended_tools",
    "run_id",
    "run_status",
    "run_stage",
    "health_score",
    "finding_key",
    "id: finding.id",
    "status: finding.status",
    "project_id",
    "first_seen_at",
    "last_seen_at",
    "review_required",
    "autofix_eligible",
    "linked_opportunity_id",
    "linked_content_action_id",
    "opportunity_id",
    "content_action_id",
    "Growth Loop",
    "start-growth-loop",
  ]) {
    assert.doesNotMatch(repairPayloadBlock, new RegExp(internalOrGrowthMetadata.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.doesNotMatch(acceptanceBlock, /finding_key/);
  assert.match(acceptanceBlock, /rerun Doctor or an equivalent crawler/);
});

test("structured data AI repair JSON includes the UNiPost-style schema contract", () => {
  const client = read("projects/[id]/doctor/doctor-client.tsx");
  const acceptanceBlock = client.slice(client.indexOf("function structuredDataAcceptanceTests"), client.indexOf("function buildAIRepairPayload"));

  for (const contract of [
    "seo_contract",
    "page_role",
    "homepage",
    "schema_types",
    "WebSite",
    "Organization",
    "WebPage",
    "field_sources",
    "canonical_url",
    "render_requirement",
    "server-rendered",
    "Google Rich Results Test",
    "Schema Markup Validator",
    "@context",
    "@type",
    "absolute production URLs",
    "trailing-slash format",
    "template placeholders",
    "staging",
    "logo",
  ]) {
    assert.match(client, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  for (const acceptance of [
    "server-rendered JSON-LD",
    "valid JSON without template placeholders",
    "Google Rich Results Test or Schema Markup Validator",
    "WebSite, Organization, and WebPage",
    "no localhost, staging, preview, or dev URLs",
  ]) {
    assert.match(acceptanceBlock, new RegExp(acceptance.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("Home fetches and renders a first-fold Doctor module", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  assert.match(workspace, /getSEODoctor\(projectId\)/);
  assert.match(workspace, /doctorReport/);
  assert.match(workspace, /Site health/);
  assert.match(workspace, /\/projects\/\$\{projectId\}\/doctor/);
  assert.match(workspace, /progress_percent/);
});
