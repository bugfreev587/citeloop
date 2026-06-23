# Multi-Surface SEO Growth Phase 4 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn existing autopilot plans into the PRD action portfolio shape and show the latest portfolio grouped by bucket, risk, review need, and measurement schedule in the existing SEO dashboard.

**Architecture:** Keep `seo_action_plans` as the storage table. `generateAutopilotPlan` writes a richer JSON document into `seo_action_plans.actions`; the web API normalizer accepts both the new object shape and older raw arrays. The dashboard renders a compact portfolio view in the existing Autopilot section.

**Tech Stack:** Go API source-contract tests, existing autopilot planner handler, Next.js TypeScript API normalizers, source-contract tests with `node:test`.

---

## Task 1: Backend Portfolio Document Contract

**Files:**
- Create: `internal/api/action_portfolio_contract_test.go`
- Modify: `internal/api/handlers_autopilot.go`

- [ ] **Step 1: Write failing backend contract test**

Create `internal/api/action_portfolio_contract_test.go`:

```go
package api

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateAutopilotPlanWritesActionPortfolioDocument(t *testing.T) {
	raw, err := os.ReadFile("handlers_autopilot.go")
	if err != nil {
		t.Fatalf("read handlers_autopilot.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"actionPortfolioDocument",
		"selected_actions",
		"deferred_actions",
		"rejected_actions",
		"reason_codes",
		"policy_snapshot",
		"budget_snapshot",
		"risk_summary",
		"required_approvals",
		"measurement_schedule",
		"action_bucket",
		"review_required",
		"measurementScheduleForAction",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("autopilot plan missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Verify red**

Run:

```bash
go test ./internal/api -run ActionPortfolio -count=1
```

Expected: FAIL because the handler still stores the selected action array directly.

- [ ] **Step 3: Enrich selected actions**

In `generateAutopilotPlan`, compute `autoPublishAllowed` once and add these keys to each selected action map:

```go
autoPublishAllowed := policy.AutopilotLevel >= 2 && result.Level == autopilot.RiskLow && !policy.KillSwitchEnabled && !policy.SafeModeEnabled
actionBucket := actionBucketFor(valueOr(opp.RecommendedAction, opp.Type), opp.Type)
measurementSchedule := measurementScheduleForAction(actionBucket)
selected = append(selected, map[string]any{
	"opportunity_id":         opp.ID,
	"type":                   opp.Type,
	"recommended_action":     opp.RecommendedAction,
	"action_bucket":          actionBucket,
	"risk_level":             result.Level,
	"risk_reasons":           result.Reasons,
	"classifier_version":     result.ClassifierVersion,
	"auto_publish_allowed":   autoPublishAllowed,
	"review_required":        !autoPublishAllowed,
	"measurement_schedule":   measurementSchedule,
})
```

- [ ] **Step 4: Store portfolio document**

Before `CreateSEOActionPlan`, add:

```go
portfolio := actionPortfolioDocument(selected, []map[string]any{}, policy, now)
```

Change `Actions:` from `mustJSONLocal(selected)` to:

```go
Actions: mustJSONLocal(portfolio),
```

Add helper functions near `aggregateRisk`:

```go
func actionPortfolioDocument(selected, rejected []map[string]any, policy db.SeoPolicy, now time.Time) map[string]any {
	riskSummary := map[string]int{"low": 0, "medium": 0, "high": 0}
	requiredApprovals := []map[string]any{}
	measurementSchedule := []map[string]any{}
	for _, action := range selected {
		risk := riskLevelString(action["risk_level"])
		if _, ok := riskSummary[risk]; ok {
			riskSummary[risk]++
		}
		if required, _ := action["review_required"].(bool); required {
			requiredApprovals = append(requiredApprovals, map[string]any{
				"opportunity_id": action["opportunity_id"],
				"risk_level":     risk,
				"action_bucket":  action["action_bucket"],
			})
		}
		if schedule, ok := action["measurement_schedule"].(map[string]any); ok {
			measurementSchedule = append(measurementSchedule, schedule)
		}
	}
	return map[string]any{
		"selected_actions":     selected,
		"deferred_actions":     []map[string]any{},
		"rejected_actions":     rejected,
		"reason_codes":         map[string]any{"selection": "open_opportunity_priority"},
		"policy_snapshot":      map[string]any{"autopilot_level": policy.AutopilotLevel, "weekly_action_limit": policy.WeeklyActionLimit, "safe_mode_enabled": policy.SafeModeEnabled, "kill_switch_enabled": policy.KillSwitchEnabled},
		"budget_snapshot":      map[string]any{"expected_effort": len(selected)},
		"risk_summary":         riskSummary,
		"required_approvals":   requiredApprovals,
		"measurement_schedule": measurementSchedule,
		"generated_at":         now,
	}
}
```

Also add `riskLevelString`, `actionBucketFor`, and `measurementScheduleForAction` helpers using deterministic string matching. Buckets must include `create new asset`, `refresh existing page`, `rewrite title/meta`, `add internal links`, `add structured data`, `submit/update sitemap`, `distribute canonical variant`, and `monitor external mention`.

- [ ] **Step 5: Verify green**

Run:

```bash
go test ./internal/api -run ActionPortfolio -count=1
```

Expected: PASS.

## Task 2: Web Portfolio Normalizer and Dashboard Contract

**Files:**
- Create: `web/app/lib/action-portfolio-contract.test.mjs`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Write failing web contract test**

Create `web/app/lib/action-portfolio-contract.test.mjs`:

```js
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("SEO action plan exposes normalized portfolio shape", () => {
  const api = read("lib/api.ts");
  for (const snippet of [
    "export type SEOActionPortfolioItem",
    "export type SEOActionPortfolio",
    "portfolio: SEOActionPortfolio",
    "normalizeSEOActionPlan",
    "selected_actions",
    "deferred_actions",
    "rejected_actions",
    "risk_summary",
    "required_approvals",
    "measurement_schedule",
    "action_bucket",
    "review_required",
    "listAutopilotPlans: async",
    "normalizeSEOActionPlan",
  ]) {
    assert.match(api, new RegExp(snippet.replace(/[.*+?^${}()|[\\]\\\\]/g, "\\\\$&")));
  }
});

test("SEO dashboard renders action portfolio groups", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const snippet of [
    "Action portfolio",
    "Selected actions",
    "Risk summary",
    "Review required",
    "Measurement",
    "latestPortfolioPlan",
    "plan.portfolio.selected_actions",
    "action.action_bucket",
    "action.risk_level",
    "action.review_required",
  ]) {
    assert.match(seo, new RegExp(snippet.replace(/[.*+?^${}()|[\\]\\\\]/g, "\\\\$&")));
  }
});
```

- [ ] **Step 2: Verify red**

Run:

```bash
cd web && node --test app/lib/action-portfolio-contract.test.mjs
```

Expected: FAIL because plans are returned as raw arrays and the dashboard does not show portfolio groups.

- [ ] **Step 3: Add portfolio types and normalizer**

In `web/app/lib/api.ts`, add:

```ts
export type SEOActionPortfolioItem = {
  opportunity_id?: string;
  type: string;
  recommended_action?: string | null;
  action_bucket: string;
  asset_type?: string | null;
  risk_level: string;
  risk_reasons: string[];
  classifier_version?: string;
  auto_publish_allowed: boolean;
  review_required: boolean;
  measurement_schedule?: any;
};

export type SEOActionPortfolio = {
  selected_actions: SEOActionPortfolioItem[];
  deferred_actions: SEOActionPortfolioItem[];
  rejected_actions: SEOActionPortfolioItem[];
  reason_codes: Record<string, any>;
  policy_snapshot: Record<string, any>;
  budget_snapshot: Record<string, any>;
  risk_summary: Record<string, number>;
  required_approvals: any[];
  measurement_schedule: any[];
};
```

Extend `SEOActionPlan` with:

```ts
portfolio: SEOActionPortfolio;
```

Add `normalizeSEOActionPlan(raw)` that accepts both `raw.actions` as an array and `raw.actions` as an object with `selected_actions`. Update both `generateAutopilotPlan` and `listAutopilotPlans` to use it.

- [ ] **Step 4: Render latest action portfolio**

In `web/app/projects/[id]/seo/seo-client.tsx`, add:

```tsx
const latestPortfolioPlan = plans[0] ?? null;
```

Under the Autopilot summary cards, render latest portfolio details:

```tsx
{latestPortfolioPlan && (
  <div className="mt-3 rounded-lg border border-slate-200 bg-white p-4">
    <div className="flex items-center justify-between gap-3">
      <div>
        <div className="text-base font-bold text-slate-900">Action portfolio</div>
        <p className="text-sm text-slate-500">Selected actions grouped by bucket, risk, review, and measurement.</p>
      </div>
      <Badge tone={toneForRisk(latestPortfolioPlan.aggregate_risk)}>{latestPortfolioPlan.aggregate_risk}</Badge>
    </div>
    <div className="mt-4 grid gap-3 lg:grid-cols-[1.5fr_1fr]">
      <div>
        <div className="mb-2 text-sm font-bold text-slate-900">Selected actions</div>
        <div className="divide-y divide-slate-100">
          {latestPortfolioPlan.portfolio.selected_actions.slice(0, 8).map((action, index) => (
            <div key={`${action.opportunity_id ?? index}`} className="py-3">
              <div className="flex flex-wrap items-center gap-2">
                <Badge tone="neutral">{action.action_bucket}</Badge>
                <Badge tone={toneForRisk(action.risk_level)}>{action.risk_level}</Badge>
                {action.review_required && <Badge tone="amber">Review required</Badge>}
              </div>
              <div className="mt-2 text-sm font-semibold text-slate-900">{action.recommended_action ?? action.type}</div>
              <div className="mt-1 text-xs text-slate-500">Measurement: {action.measurement_schedule?.checkpoints?.join(" / ") ?? "Not scheduled"}</div>
            </div>
          ))}
        </div>
      </div>
      <div>
        <div className="mb-2 text-sm font-bold text-slate-900">Risk summary</div>
        <div className="grid gap-2 text-sm text-slate-600">
          {Object.entries(latestPortfolioPlan.portfolio.risk_summary).map(([risk, count]) => (
            <div key={risk} className="flex items-center justify-between border-b border-slate-100 py-2">
              <span>{risk}</span>
              <span className="font-semibold text-slate-900">{count}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  </div>
)}
```

- [ ] **Step 5: Verify green**

Run:

```bash
cd web && node --test app/lib/action-portfolio-contract.test.mjs
```

Expected: PASS.

## Task 3: Phase 4 Gate and Commit

- [ ] **Step 1: Run full phase gate**

Run:

```bash
make sqlc
go test ./internal/api -run ActionPortfolio -count=1
cd web && node --test app/lib/action-portfolio-contract.test.mjs
make test
make build
cd web && node --test app/lib/action-portfolio-contract.test.mjs app/lib/multi-surface-seo-contract.test.mjs app/lib/surface-inventory-contract.test.mjs
cd web && VERCEL_ENV=preview npm run build
```

Expected: all commands exit 0.

- [ ] **Step 2: Commit Phase 4**

Run:

```bash
git add docs/superpowers/plans/2026-06-23-multi-surface-seo-growth-phase4.md internal/api/action_portfolio_contract_test.go internal/api/handlers_autopilot.go web/app/lib/action-portfolio-contract.test.mjs web/app/lib/api.ts 'web/app/projects/[id]/seo/seo-client.tsx'
git commit -m "feat(seo): render autopilot action portfolios"
```

Expected: commit succeeds.

## Self-Review

- This phase reuses `seo_action_plans` and `autopilot_runs`; it does not create `action_portfolios`.
- New portfolio output remains backward compatible with older plans whose `actions` field is an array.
- This phase does not add approval mutations, external publisher connectors, or measurement workers.
