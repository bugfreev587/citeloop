# Doctor / Opportunities Phase 4 Visible Closed Loops Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Doctor and Opportunities visibly independent from first finding through their different terminal outcomes: immediate Site Fix verification for Doctor and delayed measurement/learning for Opportunities.

**Architecture:** Reuse the canonical Doctor report, Site Fix, Opportunities, Growth Action, measurement, and learning APIs already live after Phases 2–3. Phase 4 is a presentation and navigation slice: Doctor owns repair lifecycle summaries, Home renders two independent control centers, and Results explicitly separates immediate repair verification from delayed Growth attribution. No new writer, queue, database table, or provider call is introduced.

**Tech Stack:** Go/chi API contracts already in production; Next.js 15 App Router; React/TypeScript; existing `useApi`, `SiteFix`, `SEODoctorReport`, `VisibilitySummary`, `ResultsAction`, and contract-test patterns.

---

### Task 1: Show the Doctor repair lifecycle on Doctor

**Files:**
- Modify: `web/app/projects/[id]/doctor/doctor-client.tsx`
- Create: `web/app/lib/visible-closed-loops-contract.test.mjs`

- [ ] **Step 1: Write the failing Doctor lifecycle contract**

Add a source contract that requires Doctor to fetch canonical Site Fixes and render `Doctor repair loop`, `Proposed`, `Approved`, `Applied / deploying`, `Verified`, `Needs attention`, and a link to `/site-fixes`.

```js
test("Doctor exposes the canonical repair lifecycle", () => {
  assert.match(doctor, /api\.listDoctorSiteFixes\(projectId\)/);
  for (const label of ["Doctor repair loop", "Proposed", "Approved", "Applied / deploying", "Verified", "Needs attention"]) {
    assert.equal(doctor.includes(label), true, `Doctor missing ${label}`);
  }
  assert.match(doctor, /`\/projects\/\$\{projectId\}\/site-fixes`/);
});
```

- [ ] **Step 2: Run the contract and confirm RED**

Run: `cd web && node --test app/lib/visible-closed-loops-contract.test.mjs`

Expected: FAIL because Doctor does not fetch or summarize Site Fixes.

- [ ] **Step 3: Fetch the canonical Site Fix list with the Doctor report**

Import `SiteFix`, add `siteFixes` state, and make refresh load both independent read models without making one failure hide the other.

```tsx
const [siteFixes, setSiteFixes] = useState<SiteFix[]>([]);

const [next, fixes] = await Promise.all([
  api.getSEODoctor(projectId),
  api.listDoctorSiteFixes(projectId).catch(() => []),
]);
setReport(next);
setSiteFixes(fixes);
```

- [ ] **Step 4: Render lifecycle counts and current repair links**

Derive exact presentation buckets without inventing a Growth measurement state.

```tsx
const repairCounts = siteFixes.reduce(
  (counts, fix) => {
    if (["proposed", "draft"].includes(fix.status)) counts.proposed += 1;
    else if (fix.status === "approved") counts.approved += 1;
    else if (["applying", "applied", "awaiting_deploy", "verifying", "failed_retryable", "reopened"].includes(fix.status)) counts.executing += 1;
    else if (fix.status === "verified") counts.verified += 1;
    else if (["failed_terminal", "terminated"].includes(fix.status)) counts.attention += 1;
    return counts;
  },
  { proposed: 0, approved: 0, executing: 0, verified: 0, attention: 0 },
);
```

Render a compact section above Findings. Each non-zero stage links to `/projects/${projectId}/site-fixes`; copy must state that Doctor ends at verified and never enters Measuring.

- [ ] **Step 5: Verify and commit the Doctor slice**

Run:

```bash
cd web
npm test
npm run build
```

Expected: all Web tests and production build pass.

Commit: `feat: show Doctor repair lifecycle`

### Task 2: Split Home into independent Doctor and Opportunities control centers

**Files:**
- Modify: `web/app/projects/[id]/workspace.tsx`
- Modify: `web/app/lib/visible-closed-loops-contract.test.mjs`

- [ ] **Step 1: Write the failing Home two-line contract**

```js
test("Home renders independent Doctor and Opportunities control centers", () => {
  assert.match(workspace, /api\.listDoctorSiteFixes\(projectId\)/);
  assert.equal(workspace.includes("Doctor Control Center"), true);
  assert.equal(workspace.includes("Opportunities Control Center"), true);
  assert.equal(workspace.includes("Immediate verification"), true);
  assert.equal(workspace.includes("Delayed measurement"), true);
});
```

- [ ] **Step 2: Run the contract and confirm RED**

Run: `cd web && node --test app/lib/visible-closed-loops-contract.test.mjs`

Expected: FAIL because Home currently has one Growth Control Center.

- [ ] **Step 3: Load Site Fix state beside existing Doctor and Growth state**

Import `SiteFix`, add `doctorSiteFixes`, and append `api.listDoctorSiteFixes(projectId).catch(() => [])` to the existing Home refresh promise. Preserve existing visibility-summary ownership for Growth counts.

- [ ] **Step 4: Add a Doctor Control Center**

Render three compact linked cards:

```tsx
const doctorControlCards = [
  { title: "Findings", label: `${doctorIssueCount} active`, detail: "Broken and optimization findings from the latest Doctor evidence.", href: `/projects/${projectId}/doctor` },
  { title: "Site Fixes", label: `${activeDoctorFixCount} in repair`, detail: "Proposed through deploy and verification, with no Growth measurement window.", href: `/projects/${projectId}/site-fixes` },
  { title: "Immediate verification", label: `${verifiedDoctorFixCount} verified`, detail: "Doctor closes only after acceptance tests re-read the repaired evidence.", href: `/projects/${projectId}/site-fixes` },
];
```

- [ ] **Step 5: Rename the existing Growth control surface**

Rename `Growth Control Center` to `Opportunities Control Center`; retain Opportunities, Action Portfolio, Impact Reports, and Learning cards. Copy must use `Delayed measurement` and must not include Doctor findings or Site Fix counts.

- [ ] **Step 6: Verify and commit the Home slice**

Run `cd web && npm test && npm run build`.

Expected: all tests and build pass.

Commit: `feat: separate Home control centers`

### Task 3: Separate immediate repair verification from delayed Growth outcomes in Results

**Files:**
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `web/app/lib/visible-closed-loops-contract.test.mjs`

- [ ] **Step 1: Write the failing Results boundary contract**

```js
test("Results separates Doctor verification from Growth measurement", () => {
  assert.match(results, /api\.listDoctorSiteFixes\(projectId\)/);
  for (const label of ["Immediate verification", "Doctor repair outcomes", "Delayed growth outcomes", "Growth measurement & learning"]) {
    assert.equal(results.includes(label), true, `Results missing ${label}`);
  }
});
```

- [ ] **Step 2: Run the contract and confirm RED**

Run: `cd web && node --test app/lib/visible-closed-loops-contract.test.mjs`

Expected: FAIL because Results only renders Growth attribution.

- [ ] **Step 3: Load canonical Site Fixes as a separate read model**

Add `doctorSiteFixes` state and `api.listDoctorSiteFixes(projectId)` to refresh. Do not merge Site Fix rows into `resultsActions`, measurement counts, learning scoring, or Growth lifecycle counts.

- [ ] **Step 4: Render the two terminal contracts**

At the top of Results render:

```tsx
<section data-results-doctor-verification>
  <SectionHeader title="Doctor repair outcomes" eyebrow="Immediate verification" />
  <p>Applied Site Fixes close only after deploy and acceptance-test verification.</p>
</section>

<section data-results-growth-measurement>
  <SectionHeader title="Growth measurement & learning" eyebrow="Delayed growth outcomes" />
  <p>Published Growth Actions close at finite checkpoints with an outcome and learning or measurement-quality record.</p>
</section>
```

The Doctor section shows verified, retryable, deploying, and terminal-failure counts linked to Site Fixes. The existing Impact Reports and Action-level attribution remain under the Growth section.

- [ ] **Step 5: Verify and commit the Results slice**

Run `cd web && npm test && npm run build`.

Expected: all tests and build pass.

Commit: `feat: separate repair and growth outcomes`

### Task 4: Phase 4 production gate

**Files:**
- No source changes unless production behavior diverges.

- [ ] **Step 1: Run repository verification**

```bash
sqlc generate
go test ./...
go vet ./...
go build ./...
cd web
npm test
npm run build
```

Expected: all commands pass with no generated diff.

- [ ] **Step 2: Merge, deploy, and verify both runtimes**

Require GitHub Go/Web/Vercel checks, merge to `main`, Railway `SUCCESS` on the merge commit, and Vercel production `READY`. A repeatedly hanging optional review may be deferred under the user's skip rule only after self-review and required checks pass.

- [ ] **Step 3: Browser-verify the two independent lines**

Using the authenticated production Chrome session:

1. Home shows separate Doctor and Opportunities control centers.
2. Doctor shows Broken, Optimization, Healthy coverage, and the Site Fix lifecycle.
3. Opportunities shows a decision-ready hypothesis and delayed measurement policy without Doctor work.
4. Results shows Doctor immediate verification separately from Growth delayed outcome/learning.
5. No browser console errors occur.

- [ ] **Step 4: Exercise one Doctor repair loop**

Create a Site Fix from a real actionable Doctor finding, approve it, apply it through the configured publisher, wait for deploy, and verify acceptance evidence. Confirm the Site Fix ends at `verified`, does not create a Growth Opportunity/Action, and never enters Measuring. If the external repository or deploy cannot be coordinated after repeated attempts, record the exact fix ID and external blocker, defer only this proof, and continue Phase 5 under the user's skip rule.

- [ ] **Step 5: Reconfirm one Opportunities loop**

Run Opportunity Finding and verify one decision-ready Growth Opportunity, no Doctor finding/Site Fix created during the run, finite Growth Action deadline, terminal outcome, and learning or measurement-quality record.

- [ ] **Step 6: Record the Phase 4 gate**

Capture PR URL, merge commit, deployment IDs, Doctor run/finding/Site Fix IDs, Opportunity Finding event ID, Growth Action ID, terminal outcome ID, and browser proof. Phase 4 is complete only when all non-deferred checks pass.
