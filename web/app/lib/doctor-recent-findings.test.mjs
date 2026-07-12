import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadModule() {
  const source = await readFile(new URL("./doctor-recent-findings.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ES2020, target: ts.ScriptTarget.ES2020 },
  }).outputText;
  return import(`data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`);
}

const finding = (id) => ({ id, fix_intent: `Fix ${id}` });
const fix = (id, doctorFindingID, createdAt, overrides = {}) => ({
  id,
  doctor_finding_id: doctorFindingID,
  created_at: createdAt,
  status: "proposed",
  application: null,
  doctor_link_dismissed_at: null,
  doctor_link_dismissed_by: null,
  ...overrides,
});

test("Doctor active findings exclude every finding that has any Site Fix", async () => {
  const { activeDoctorFindings } = await loadModule();
  const findings = [finding("finding-a"), finding("finding-b"), finding("finding-c")];
  const fixes = [
    fix("fix-a", "finding-a", "2026-07-10T10:00:00Z", { doctor_link_dismissed_at: "2026-07-11T10:00:00Z" }),
    fix("fix-b", "finding-b", "2026-07-10T10:00:00Z", { application: { github_pr_url: "https://github.com/example/site/pull/18" } }),
  ];

  assert.deepEqual(activeDoctorFindings(findings, fixes).map((item) => item.id), ["finding-c"]);
});

test("Recent Findings uses only the latest non-dismissed no-PR Site Fix per finding", async () => {
  const { recentDoctorFindingLinks } = await loadModule();
  const findings = [finding("finding-a"), finding("finding-b"), finding("finding-c"), finding("finding-d")];
  const fixes = [
    fix("fix-a-old", "finding-a", "2026-07-10T10:00:00Z"),
    fix("fix-a-new", "finding-a", "2026-07-11T10:00:00Z", { status: "failed_terminal" }),
    fix("fix-b", "finding-b", "2026-07-11T10:00:00Z", { doctor_link_dismissed_at: "2026-07-12T10:00:00Z" }),
    fix("fix-c", "finding-c", "2026-07-11T10:00:00Z", { application: { github_pr_number: 42 } }),
  ];

  const recent = recentDoctorFindingLinks(findings, fixes);
  assert.deepEqual(recent.map((item) => [item.finding.id, item.siteFix.id]), [["finding-a", "fix-a-new"]]);
  assert.equal(recent[0].siteFix.status, "failed_terminal", "terminal fixes without PRs remain inspectable");
});

test("a new Site Fix revision can reappear after an older revision was dismissed", async () => {
  const { recentDoctorFindingLinks } = await loadModule();
  const recent = recentDoctorFindingLinks([finding("finding-a")], [
    fix("fix-old", "finding-a", "2026-07-10T10:00:00Z", { doctor_link_dismissed_at: "2026-07-10T11:00:00Z" }),
    fix("fix-new", "finding-a", "2026-07-12T10:00:00Z"),
  ]);

  assert.equal(recent.length, 1);
  assert.equal(recent[0].siteFix.id, "fix-new");
});

test("Recent Findings is ordered by newest Site Fix rather than finding severity order", async () => {
  const { recentDoctorFindingLinks } = await loadModule();
  const recent = recentDoctorFindingLinks([finding("older"), finding("newer")], [
    fix("fix-older", "older", "2026-07-10T10:00:00Z"),
    fix("fix-newer", "newer", "2026-07-12T10:00:00Z"),
  ]);

  assert.deepEqual(recent.map((item) => item.finding.id), ["newer", "older"]);
});

test("every persisted PR identity clears the Doctor forward link", async () => {
  const { siteFixHasCreatedPR } = await loadModule();
  assert.equal(siteFixHasCreatedPR(fix("url", "finding", null, { application: { github_pr_url: "https://github.com/example/site/pull/7" } })), true);
  assert.equal(siteFixHasCreatedPR(fix("number", "finding", null, { application: { github_pr_number: 7 } })), true);
  assert.equal(siteFixHasCreatedPR(fix("created", "finding", null, { application: { pr_created_at: "2026-07-12T10:00:00Z" } })), true);
  assert.equal(siteFixHasCreatedPR(fix("none", "finding", null, { application: { status: "failed" } })), false);
});
