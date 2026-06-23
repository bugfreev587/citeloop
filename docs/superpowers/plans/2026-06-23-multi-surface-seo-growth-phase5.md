# Multi-Surface SEO Growth Phase 5 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade content action measurement windows from a flat `checkpoints_days` list to PRD-style baseline/checkpoint/metric objects and show the schedule in the existing action list.

**Architecture:** Reuse `content_actions.measurement_window`. The API computes the JSON window during `createSEOContentAction` from the action type and asset type; no new measurement table is introduced. The web UI reads either the new `checkpoints` array or the older `checkpoints_days` shape.

**Tech Stack:** Go API source-contract tests, existing SEO action handler, Next.js source-contract tests.

---

## Task 1: Backend Measurement Window Contract

**Files:**
- Create: `internal/api/measurement_window_contract_test.go`
- Modify: `internal/api/handlers_seo.go`

- [ ] **Step 1: Write failing backend contract test**

Create `internal/api/measurement_window_contract_test.go`:

```go
package api

import (
	"os"
	"strings"
	"testing"
)

func TestCreateSEOContentActionSchedulesStructuredMeasurementWindow(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"measurementWindowForAction",
		"baseline",
		"checkpoints",
		"primary_metric",
		"secondary_metrics",
		"status",
		"scheduled",
		"metadata_rewrite",
		"internal_link_patch",
		"external_distribution",
		"GEO citation-ready asset",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("measurement window contract missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Verify red**

Run:

```bash
go test ./internal/api -run MeasurementWindow -count=1
```

Expected: FAIL because the handler still uses `{"checkpoints_days":[7,14,28]}` directly.

- [ ] **Step 3: Compute structured measurement windows**

In `createSEOContentAction`, compute `assetType` before `CreateContentAction` and pass:

```go
MeasurementWindow: measurementWindowForAction(assetType, actionType),
```

Add helpers near `strPtrFrom`:

```go
func measurementWindowForAction(assetType, actionType string) json.RawMessage {
	days, primary, secondary := measurementPlanFor(assetType, actionType)
	return mustJSONLocal(map[string]any{
		"baseline":          map[string]any{"days": 28},
		"checkpoints":       checkpointObjects(days),
		"primary_metric":    primary,
		"secondary_metrics": secondary,
	})
}

func measurementPlanFor(assetType, actionType string) ([]int, string, []string) {
	text := strings.ToLower(strings.TrimSpace(assetType + " " + actionType))
	switch {
	case strings.Contains(text, "metadata_rewrite") || strings.Contains(text, "metadata") || strings.Contains(text, "title") || strings.Contains(text, "meta"):
		return []int{7, 14, 28}, "ctr", []string{"impressions", "clicks", "position"}
	case strings.Contains(text, "internal_link_patch") || strings.Contains(text, "internal link"):
		return []int{14, 28, 56}, "clicks", []string{"impressions", "position"}
	case strings.Contains(text, "external_distribution") || strings.Contains(text, "distribution") || strings.Contains(text, "syndication"):
		return []int{7, 14, 28}, "referral_sessions", []string{"brand_mentions", "backlinks"}
	case strings.Contains(text, "geo") || strings.Contains(text, "citation"):
		return []int{7, 14, 21, 28, 35, 42, 49, 56}, "project_owned_citations", []string{"brand_mentions", "competitor_citations"}
	case strings.Contains(text, "sitemap") || strings.Contains(text, "technical"):
		return []int{1, 7, 14, 28}, "indexed_status", []string{"http_status", "technical_issue_count"}
	default:
		return []int{14, 28, 56, 90}, "clicks", []string{"impressions", "ctr", "position"}
	}
}

func checkpointObjects(days []int) []map[string]any {
	out := make([]map[string]any, 0, len(days))
	for _, day := range days {
		out = append(out, map[string]any{"day": day, "status": "scheduled"})
	}
	return out
}
```

- [ ] **Step 4: Verify green**

Run:

```bash
go test ./internal/api -run MeasurementWindow -count=1
```

Expected: PASS.

## Task 2: Web Measurement Schedule Display Contract

**Files:**
- Create: `web/app/lib/measurement-window-contract.test.mjs`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Write failing web contract test**

Create `web/app/lib/measurement-window-contract.test.mjs`:

```js
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("SEO action list renders structured measurement checkpoints", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const snippet of [
    "measurementWindowLabel",
    "measurement_window?.checkpoints",
    "measurement_window?.checkpoints_days",
    "primary_metric",
    "D+",
    "Scheduled",
  ]) {
    assert.match(seo, new RegExp(snippet.replace(/[.*+?^${}()|[\\]\\\\]/g, "\\\\$&")));
  }
});
```

- [ ] **Step 2: Verify red**

Run:

```bash
cd web && node --test app/lib/measurement-window-contract.test.mjs
```

Expected: FAIL because the action list only displays `Scheduled` / `Not scheduled`.

- [ ] **Step 3: Add measurement label helper and use it in action cards**

In `seo-client.tsx`, add:

```tsx
function measurementWindowLabel(window: any) {
  const structured = Array.isArray(window?.checkpoints) ? window.checkpoints.map((checkpoint: any) => checkpoint?.day).filter(Boolean) : [];
  const legacy = Array.isArray(window?.checkpoints_days) ? window.checkpoints_days : [];
  const checkpoints = structured.length > 0 ? structured : legacy;
  if (checkpoints.length === 0) return "Not scheduled";
  const metric = window?.primary_metric ? `${window.primary_metric}: ` : "";
  return `${metric}${checkpoints.map((day: number | string) => `D+${day}`).join(" / ")}`;
}
```

Replace action card measurement display with:

```tsx
{measurementWindowLabel(action.measurement_window)}
```

- [ ] **Step 4: Verify green**

Run:

```bash
cd web && node --test app/lib/measurement-window-contract.test.mjs
```

Expected: PASS.

## Task 3: Phase 5 Gate and Commit

- [ ] **Step 1: Run full phase gate**

Run:

```bash
make sqlc
go test ./internal/api -run MeasurementWindow -count=1
cd web && node --test app/lib/measurement-window-contract.test.mjs
make test
make build
cd web && node --test app/lib/action-portfolio-contract.test.mjs app/lib/measurement-window-contract.test.mjs app/lib/multi-surface-seo-contract.test.mjs app/lib/surface-inventory-contract.test.mjs
cd web && VERCEL_ENV=preview npm run build
```

Expected: all commands exit 0.

- [ ] **Step 2: Commit Phase 5**

Run:

```bash
git add docs/superpowers/plans/2026-06-23-multi-surface-seo-growth-phase5.md internal/api/measurement_window_contract_test.go internal/api/handlers_seo.go web/app/lib/measurement-window-contract.test.mjs 'web/app/projects/[id]/seo/seo-client.tsx'
git commit -m "feat(seo): schedule action measurement checkpoints"
```

Expected: commit succeeds.

## Self-Review

- This phase reuses `content_actions.measurement_window`; it does not create `measurement_checkpoints`.
- This phase schedules checkpoints only; it does not implement outcome collection workers or causal attribution.
