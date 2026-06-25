# Dashboard Home Measurement Clarity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Home dashboard distinguish workflow counts from Search Console performance metrics so users no longer read `Results 24` as "24 result items."

**Architecture:** Keep the existing `workspace.tsx` Home structure and contract tests. Replace the final pipeline stage with a workflow-object count for measurement, and keep clicks/impressions in the metric strip where performance metrics belong.

**Tech Stack:** Next.js App Router, React, Tailwind CSS, Node test runner contract tests.

---

### Task 1: Lock Home Pipeline Semantics

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Modify: `web/app/projects/[id]/workspace.tsx`

- [ ] **Step 1: Write the failing contract test**

Add assertions that the Home pipeline labels the final stage as `Measurement`, does not use `Results`, and does not bind that stage to `clicks28d`.

Run: `cd web && npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`
Expected: FAIL because the current component still uses the previous result-stage semantics.

- [ ] **Step 2: Implement the minimal Home change**

In `workspace.tsx`, change the final pipeline stage to:

```tsx
{
  label: "Measurement",
  metricValue: measuringActions,
  statusLabel: measuringActions > 0 ? "Measuring impact" : searchDataConnected ? "Ready for impact data" : "Connect for proof",
  tone: measuringActions > 0 || searchDataConnected ? "green" : "amber",
  href: `/projects/${projectId}/results`,
}
```

Keep `clicks28d` and `impressions28d` only in the `Organic traffic` metric card.

- [ ] **Step 3: Run the focused contract test**

Run: `cd web && npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`
Expected: PASS.

### Task 2: Verify The Frontend Surface

**Files:**
- Verify: `web/app/projects/[id]/workspace.tsx`
- Verify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Run typecheck**

Run: `cd web && npm run typecheck`
Expected: exit code 0.

- [ ] **Step 2: Run the web build**

Run: `cd web && npm run build`
Expected: exit code 0.

- [ ] **Step 3: Inspect the diff**

Run: `git diff -- web/app/projects/[id]/workspace.tsx web/app/lib/dashboard-ux-phase1-contract.test.mjs`
Expected: only the Home measurement semantics and associated tests changed.
