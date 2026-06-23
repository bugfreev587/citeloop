# Multi-Surface SEO Growth Phase 2 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose Phase 1 multi-surface action metadata through SEO action creation, API types, and the existing Opportunities dashboard action list.

**Architecture:** Keep the existing `content_actions` workflow. The API accepts optional metadata on create, stores it through `UpdateContentActionExecutionMetadata`, and the web client renders the fields already returned by generated JSON tags.

**Tech Stack:** Go API handlers/tests, sqlc generated models from Phase 1, Next.js TypeScript API types, node contract tests.

---

## Task 1: API Contract for Multi-Surface Action Metadata

**Files:**
- Create: `internal/api/multi_surface_actions_contract_test.go`
- Modify: `internal/api/handlers_seo.go`

- [ ] **Step 1: Write failing source contract test**

Create `internal/api/multi_surface_actions_contract_test.go`:

```go
package api

import (
	"os"
	"strings"
	"testing"
)

func TestCreateSEOContentActionAcceptsMultiSurfaceMetadata(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"AssetType",
		"`json:\"asset_type\"`",
		"TargetSurfaceID",
		"`json:\"target_surface_id\"`",
		"RiskReasons",
		"`json:\"risk_reasons\"`",
		"EvidenceSnapshot",
		"`json:\"evidence_snapshot\"`",
		"DiffSnapshot",
		"`json:\"diff_snapshot\"`",
		"ReviewRequired",
		"`json:\"review_required\"`",
		"UpdateContentActionExecutionMetadata",
		"reviewRequired := true",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("createSEOContentAction missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Verify red**

Run: `go test ./internal/api -run MultiSurface -count=1`

Expected: FAIL because `handlers_seo.go` does not accept or persist these fields.

- [ ] **Step 3: Extend request parsing**

In `createSEOContentAction`, replace the request struct with:

```go
var in struct {
	ActionType       string          `json:"action_type"`
	AssetType        string          `json:"asset_type"`
	TargetSurfaceID  *uuid.UUID      `json:"target_surface_id"`
	RiskReasons      json.RawMessage `json:"risk_reasons"`
	EvidenceSnapshot json.RawMessage `json:"evidence_snapshot"`
	InputSnapshot    json.RawMessage `json:"input_snapshot"`
	OutputSnapshot   json.RawMessage `json:"output_snapshot"`
	DiffSnapshot     json.RawMessage `json:"diff_snapshot"`
	ReviewRequired   *bool           `json:"review_required"`
}
```

Add defaults after action creation:

```go
reviewRequired := true
if in.ReviewRequired != nil {
	reviewRequired = *in.ReviewRequired
}
targetSurfaceID := pgtype.UUID{}
if in.TargetSurfaceID != nil {
	targetSurfaceID = pgtype.UUID{Bytes: *in.TargetSurfaceID, Valid: true}
}
```

Call:

```go
action, err = s.Q.UpdateContentActionExecutionMetadata(r.Context(), db.UpdateContentActionExecutionMetadataParams{
	ID:                   action.ID,
	ProjectID:            projectID,
	AssetType:            pgtype.Text{String: strings.TrimSpace(in.AssetType), Valid: strings.TrimSpace(in.AssetType) != ""},
	TargetSurfaceID:      targetSurfaceID,
	RiskReasons:          rawOrDefault(in.RiskReasons, `[]`),
	EvidenceSnapshot:     rawOrDefault(in.EvidenceSnapshot, `{}`),
	InputSnapshot:        rawOrDefault(in.InputSnapshot, `{}`),
	OutputSnapshot:       rawOrDefault(in.OutputSnapshot, `{}`),
	DiffSnapshot:         rawOrDefault(in.DiffSnapshot, `{}`),
	ReviewRequired:       reviewRequired,
	VerificationSnapshot: json.RawMessage(`{}`),
})
```

- [ ] **Step 4: Verify green**

Run: `go test ./internal/api -run MultiSurface -count=1`

Expected: PASS.

## Task 2: Web API Types and Dashboard Contract

**Files:**
- Create: `web/app/lib/multi-surface-seo-contract.test.mjs`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Write failing web contract test**

Create `web/app/lib/multi-surface-seo-contract.test.mjs`:

```js
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("SEO action API type exposes multi-surface metadata", () => {
  const api = read("lib/api.ts");
  for (const field of [
    "asset_type?: string | null",
    "target_surface_id?: string | null",
    "risk_reasons?: any",
    "evidence_snapshot?: any",
    "diff_snapshot?: any",
    "review_required?: boolean",
    "verified_at?: any",
    "verification_snapshot?: any",
    "measurement_window?: any",
  ]) {
    assert.match(api, new RegExp(field.replace(/[.*+?^${}()|[\\]\\\\]/g, "\\\\$&")));
  }
  assert.match(api, /body: \{ action_type\?: string; asset_type\?: string; review_required\?: boolean \}/);
});

test("SEO action list renders asset, review, and verification metadata", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const copy of ["Asset", "Review", "Verification", "Measurement"]) {
    assert.match(seo, new RegExp(copy));
  }
  assert.match(seo, /action\.asset_type/);
  assert.match(seo, /action\.review_required/);
  assert.match(seo, /action\.verification_snapshot/);
  assert.match(seo, /action\.measurement_window/);
});
```

- [ ] **Step 2: Verify red**

Run: `cd web && node --test app/lib/multi-surface-seo-contract.test.mjs`

Expected: FAIL because the type and UI do not include these fields.

- [ ] **Step 3: Extend `SEOContentAction` and create body type**

In `web/app/lib/api.ts`, extend `SEOContentAction`:

```ts
asset_type?: string | null;
target_surface_id?: string | null;
risk_reasons?: any;
evidence_snapshot?: any;
input_snapshot?: any;
output_snapshot?: any;
diff_snapshot?: any;
review_required?: boolean;
approved_by?: string | null;
approved_at?: any;
verified_at?: any;
verification_snapshot?: any;
baseline_window?: any;
measurement_window?: any;
published_at?: any;
outcome_summary?: any;
```

Update `createSEOContentAction` body type:

```ts
body: { action_type?: string; asset_type?: string; review_required?: boolean } = {},
```

- [ ] **Step 4: Render metadata in action list**

In `web/app/projects/[id]/seo/seo-client.tsx`, add compact metadata rows inside the existing Content actions cards:

```tsx
<div className="mt-2 grid gap-2 text-xs text-slate-500 sm:grid-cols-4">
  <div><span className="font-semibold text-slate-700">Asset</span><br />{action.asset_type ?? "unspecified"}</div>
  <div><span className="font-semibold text-slate-700">Review</span><br />{action.review_required === false ? "Optional" : "Required"}</div>
  <div><span className="font-semibold text-slate-700">Verification</span><br />{action.verified_at ? "Verified" : action.verification_snapshot ? "Pending" : "Not started"}</div>
  <div><span className="font-semibold text-slate-700">Measurement</span><br />{action.measurement_window ? "Scheduled" : "Not scheduled"}</div>
</div>
```

- [ ] **Step 5: Verify green**

Run:

```bash
cd web && node --test app/lib/multi-surface-seo-contract.test.mjs
```

Expected: PASS.

## Task 3: Phase 2 Gate and Commit

- [ ] **Step 1: Run full phase gate**

Run:

```bash
make sqlc
make test
make build
cd web && npm install
cd web && npm run build
```

Expected: all exit 0.

- [ ] **Step 2: Commit Phase 2**

Run:

```bash
git add docs/superpowers/plans/2026-06-23-multi-surface-seo-growth-phase2.md internal/api web
git commit -m "feat(seo): expose multi-surface action metadata"
```

Expected: commit succeeds.

## Self-Review

- This plan has concrete red tests for API and web behavior.
- It does not implement new publisher connectors or a new CMS target.
- It keeps existing content action workflow and dashboard pages.
- It provides a hard phase gate before Phase 3 planning.
