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
    "createDoctorSiteFix",
  ]) {
    assert.match(api, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.match(api, /`\/projects\/\$\{id\}\/doctor`/);
  assert.match(api, /`\/projects\/\$\{id\}\/doctor\/runs`/);
  assert.match(api, /\/doctor\/findings\/\$\{findingID\}\/site-fixes`/);
  assert.doesNotMatch(api, /\/seo\/doctor/);
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
    "Last run",
    "latest crawl checks",
    "Doctor refreshed",
    // Findings now live in a responsive card grid that opens a reusable right drawer
    // instead of the old centered "Fix with AI" modal.
    "data-doctor-findings-grid",
    "data-doctor-finding-card",
    "data-doctor-finding-drawer",
    "Finding details",
    "data-doctor-ai-payload",
    "AI coding fix JSON",
    "buildAIRepairPayload",
    "copyAIRepairJSON",
    "selectedRepairJSON",
    "writeClipboardText",
    "Copy fix JSON",
    "Dismiss",
    // A reviewed finding can be routed into the dedicated Site Fixes lifecycle.
    "Add to Site Fixes",
    "addToSiteFixes",
    "api.createDoctorSiteFix",
    "finding_kind",
    "Broken",
    "Optimization",
    "Healthy coverage",
    "acceptance_tests",
    "dismissSEODoctorFinding",
    "document.execCommand",
  ]) {
    assert.match(client, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  // RightDrawer marks surfaceRef inert while open and renders inline (no portal),
  // so the drawer must be a SIBLING of the inert surface — otherwise its own close
  // button becomes inert and the drawer cannot be closed by pointer.
  assert.match(client, /surfaceRef=\{surfaceRef\}/, "doctor drawer should inert the page surface");
  assert.match(client, /<\/div>\s*<RightDrawer/, "RightDrawer must render outside (sibling of) the surfaceRef div, not nested inside it");
  for (const forbidden of [
    // Old centered modal is gone; the drawer opens by clicking the card, not a
    // "Fix with AI" button.
    "seo-doctor-ai-repair-title",
    "Fix with AI",
    "setSelectedRepairFinding",
    "Start Growth Loop",
    "Select for Growth Loop",
    "selectedFindingIDs",
    "selectedGrowthLoopIDs",
    "startingGrowthLoop",
    "startGrowthLoop",
    "startSEODoctorGrowthLoop",
    "api.convertSEODoctorFinding",
    "SEOContentAction",
  ]) {
    assert.doesNotMatch(client, new RegExp(forbidden.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("Doctor separates active findings from recent Site Fix handoffs", () => {
  const client = read("projects/[id]/doctor/doctor-client.tsx");
  const logic = read("lib/doctor-recent-findings.ts");

  for (const contract of [
    "activeDoctorFindings",
    "recentDoctorFindingLinks",
    "siteFixHasCreatedPR",
    "doctor_link_dismissed_at",
    "github_pr_number",
    "github_pr_url",
    "pr_created_at",
  ]) {
    assert.match(logic, new RegExp(contract));
  }
  for (const contract of [
    "Recent Findings",
    "data-doctor-recent-findings-drawer",
    "dismissDoctorSiteFixLink",
    "Remove this link from Doctor?",
    "This only removes the link from Recent Findings. The Site Fix and any pull request are unchanged.",
    "Dismiss link",
    "site-fixes?fix=",
  ]) {
    assert.match(client, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.match(client, /disabled=\{recentFindingLinks\.length === 0\}/);
  assert.match(client, /api\.listDoctorSiteFixLinks\(projectId\)/);
  assert.match(client, /activeDoctorFindings\(actionableFindings, siteFixLinks\)/);
  assert.match(client, /recentDoctorFindingLinks\(actionableFindings, siteFixLinks\)/);
  assert.match(client, /setSiteFixes\(\(current\) => upsertDoctorSiteFix\(current, fix\)\)/);
  assert.match(client, /setSiteFixLinks\(\(current\) => upsertDoctorSiteFix\(current, fix\)\)/);
  assert.doesNotMatch(client, /router\.push\([^)]*site-fixes/);
  assert.doesNotMatch(client, /const router = useRouter\(\)/);
  assert.match(client, /setSelectedFindingID\(null\)[\s\S]*setRecentDrawerOpen\(true\)/);
  assert.match(client, /setRecentDrawerOpen\(false\)[\s\S]*setSelectedFindingID\(finding\.id\)/);
  assert.doesNotMatch(client, /listDoctorSiteFixes\(projectId\)\.catch/, "Site Fix load failures must not recreate handed-off findings");
  assert.match(client, /interactionSuspended=\{Boolean\(pendingRecentDismiss\)\}/);
  assert.match(client, /recentFallbackFocusRef/);
  assert.match(client, /fallbackFocusRef=\{recentFallbackFocusRef\}/);
  const drawer = read("components/right-drawer.tsx");
  assert.match(drawer, /!element\.matches\(":disabled"\)/);
  assert.match(drawer, /fallbackFocusRef\?\.current/);
});

test("Doctor prevents pre-mutation refresh responses from overwriting locally confirmed Site Fixes", () => {
  const client = read("projects/[id]/doctor/doctor-client.tsx");
  const refreshBlock = client.slice(client.indexOf("const refresh = useCallback"), client.indexOf("useEffect(() => {", client.indexOf("const refresh = useCallback")));
  const handoffBlock = client.slice(client.indexOf("async function addToSiteFixes"), client.indexOf("async function dismissRecentFindingLink"));

  assert.match(client, /const siteFixMutationGenerationRef = useRef\(0\)/);
  assert.match(refreshBlock, /const requestMutationGeneration = siteFixMutationGenerationRef\.current/);
  assert.match(
    refreshBlock,
    /if \(requestMutationGeneration === siteFixMutationGenerationRef\.current\) \{[\s\S]*setSiteFixes\(fixes\);[\s\S]*setSiteFixLinks\(links\);[\s\S]*\}/,
  );
  assert.match(
    handoffBlock,
    /siteFixMutationGenerationRef\.current \+= 1;[\s\S]*setSiteFixes\(\(current\) => upsertDoctorSiteFix\(current, fix\)\)/,
  );
});

test("Doctor treats the historical no-blockers sentinel as non-actionable healthy coverage", () => {
  const client = read("projects/[id]/doctor/doctor-client.tsx");

  assert.match(client, /function isLegacyHealthSentinel/);
  assert.match(client, /finding\.issue_type === "no_active_technical_blockers"/);
  assert.match(client, /function isActionableDoctorFinding/);
  assert.match(client, /filter\(isActionableDoctorFinding\)/);
  assert.match(client, /initialFindingId[\s\S]*activeFindings\.some/);
  assert.match(client, /disabled=\{[\s\S]*!isActionableDoctorFinding\(selectedFinding\)/);
});

test("Doctor finding deep links focus and persistently highlight the active card without opening a drawer", () => {
  const client = read("projects/[id]/doctor/doctor-client.tsx");
  const highlightCallIndex = client.indexOf("setHighlightedFindingID(initialFindingId)");
  const initialDeepLinkEffect = client.slice(client.lastIndexOf("useEffect(() => {", highlightCallIndex), client.indexOf("useEffect(() => {", highlightCallIndex));
  const focusEffect = client.slice(
    client.lastIndexOf("useEffect(() => {", client.indexOf("card.scrollIntoView")),
    client.indexOf("useEffect(() => {", client.indexOf("card.scrollIntoView")),
  );
  const cardClick = client.slice(
    client.indexOf("onClick={(event) => {", client.indexOf("data-doctor-finding-card")),
    client.indexOf("className={cx(", client.indexOf("data-doctor-finding-card")),
  );

  assert.match(client, /const handledInitialFindingIDRef = useRef<string \| null>\(null\)/);
  assert.match(initialDeepLinkEffect, /handledInitialFindingIDRef\.current === initialFindingId/);
  assert.match(initialDeepLinkEffect, /activeFindings\.some\(\(finding\) => finding\.id === initialFindingId\)[\s\S]*handledInitialFindingIDRef\.current = initialFindingId/);
  assert.match(initialDeepLinkEffect, /setHighlightedFindingID\(initialFindingId\)/);
  assert.doesNotMatch(initialDeepLinkEffect, /initialSelectionHandled\.current = true/);
  assert.doesNotMatch(initialDeepLinkEffect, /setSelectedFindingID\(initialFindingId\)/);
  assert.doesNotMatch(initialDeepLinkEffect, /setRecentDrawerOpen\(true\)/);
  assert.match(focusEffect, /window\.requestAnimationFrame/);
  assert.match(focusEffect, /window\.matchMedia\("\(prefers-reduced-motion: reduce\)"\)\.matches/);
  assert.match(focusEffect, /card\.scrollIntoView\(\{ behavior: reducedMotion \? "auto" : "smooth", block: "center" \}\)/);
  assert.match(focusEffect, /card\.focus\(\{ preventScroll: true \}\)/);
  assert.match(focusEffect, /window\.cancelAnimationFrame\(frame\)/);
  assert.match(cardClick, /setHighlightedFindingID\(null\)/);
  assert.match(client, /aria-current=\{highlightedFindingID === finding\.id \? "true" : undefined\}/);
  assert.match(client, /citeloop-handoff-card-selected/);
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

test("structured data AI repair JSON separates observed metadata from unresolved fields", () => {
  const client = read("projects/[id]/doctor/doctor-client.tsx");
  const structuredBlock = client.slice(
    client.indexOf("function buildStructuredDataRepairContract"),
    client.indexOf("function structuredDataAcceptanceTests"),
  );

  for (const contract of [
    "approved_metadata",
    "unresolved_fields",
    "observed_page_metadata",
    "brandName",
    "canonicalUrl",
    "logoUrl",
    "description",
    "language",
    "sameAs",
    "contactPoint",
    "hasSiteSearch",
    "site_search_policy",
    "No site search URL template was observed",
    "omit potentialAction",
  ]) {
    assert.match(structuredBlock, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(structuredBlock, /potentialAction:\s*\{/);
});

test("structured data repair canonical URL prefers the observed canonical tag", () => {
  const client = read("projects/[id]/doctor/doctor-client.tsx");
  const canonicalBlock = client.slice(client.indexOf("function repairCanonicalURL"), client.indexOf("function repairPageRole"));

  assert.match(canonicalBlock, /rawDetails\.canonical_url/);
  assert.match(canonicalBlock, /normalized_page_url/);
});

test("Home fetches and renders a first-fold Doctor module", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  assert.match(workspace, /getSEODoctor\(projectId\)/);
  assert.match(workspace, /doctorReport/);
  assert.match(workspace, /Site health/);
  assert.match(workspace, /\/projects\/\$\{projectId\}\/doctor/);
  assert.match(workspace, /progress_percent/);
});
