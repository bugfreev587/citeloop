import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const read = (path) => readFile(new URL(path, import.meta.url), "utf8");
const doctor = await read("../projects/[id]/doctor/doctor-client.tsx");
const workspace = await read("../projects/[id]/workspace.tsx");
const results = await read("../projects/[id]/seo/seo-client.tsx");

test("Doctor exposes the canonical repair lifecycle", () => {
  assert.match(doctor, /api\.listDoctorSiteFixes\(projectId\)/);
  for (const label of ["Doctor repair loop", "Proposed", "Approved", "Applied / deploying", "Verified", "Needs attention"]) {
    assert.equal(doctor.includes(label), true, `Doctor missing ${label}`);
  }
  assert.match(doctor, /`\/projects\/\$\{projectId\}\/site-fixes`/);
});

test("Home renders independent Doctor and Opportunities control centers", () => {
  assert.match(workspace, /api\.listDoctorSiteFixes\(projectId\)/);
  assert.equal(workspace.includes("Doctor Control Center"), true);
  assert.equal(workspace.includes("Opportunities Control Center"), true);
  assert.equal(workspace.includes("Immediate verification"), true);
  assert.equal(workspace.includes("Delayed measurement"), true);
});

test("Results separates Doctor verification from Growth measurement", () => {
  assert.match(results, /api\.listDoctorSiteFixes\(projectId\)/);
  for (const label of ["Immediate verification", "Doctor repair outcomes", "Delayed growth outcomes", "Growth measurement & learning"]) {
    assert.equal(results.includes(label), true, `Results missing ${label}`);
  }
  assert.equal(results.includes("data-results-doctor-verification"), true);
  assert.equal(results.includes("data-results-growth-measurement"), true);
});
