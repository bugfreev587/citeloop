# Multi-Surface SEO Growth Phase 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the existing GEO external surface endpoint and dashboard card into a generalized surface inventory that can store managed external, owned, and third-party surface metadata for later distribution, verification, and GEO observation work.

**Architecture:** Reuse `geo_external_surfaces` and the Phase 1 `UpdateGEOExternalSurfaceMetadata` query. Keep the existing `/geo/external-surfaces` route for compatibility, but let create requests persist source/canonical/indexability/publication/owner confidence/verification metadata after the normal URL upsert. The web client exposes the same fields through existing GEO overview state.

**Tech Stack:** Go API handlers/tests, sqlc generated query types, Next.js TypeScript API types, source-contract tests with `node:test`.

---

## Task 1: API Contract for Generalized Surface Metadata

**Files:**
- Create: `internal/api/surface_inventory_contract_test.go`
- Modify: `internal/api/handlers_geo_pr2.go`

- [ ] **Step 1: Write the failing source contract test**

Create `internal/api/surface_inventory_contract_test.go`:

```go
package api

import (
	"os"
	"strings"
	"testing"
)

func TestCreateGEOExternalSurfaceAcceptsGeneralizedSurfaceMetadata(t *testing.T) {
	raw, err := os.ReadFile("handlers_geo_pr2.go")
	if err != nil {
		t.Fatalf("read handlers_geo_pr2.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"SourceURL",
		"`json:\"source_url\"`",
		"CanonicalStatus",
		"`json:\"canonical_status\"`",
		"IndexabilityStatus",
		"`json:\"indexability_status\"`",
		"PublicationStatus",
		"`json:\"publication_status\"`",
		"OwnerConfidence",
		"`json:\"owner_confidence\"`",
		"VerificationSnapshot",
		"`json:\"verification_snapshot\"`",
		"RelatedActionIDs",
		"`json:\"related_action_ids\"`",
		"UpdateGEOExternalSurfaceMetadata",
		"rawOrExisting",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("createGEOExternalSurface missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Verify red**

Run:

```bash
go test ./internal/api -run GeneralizedSurface -count=1
```

Expected: FAIL because `createGEOExternalSurface` does not yet parse or persist the generalized metadata fields.

- [ ] **Step 3: Extend request parsing and metadata persistence**

In `internal/api/handlers_geo_pr2.go`, extend the `createGEOExternalSurface` request struct:

```go
var in struct {
	URL                  string          `json:"url"`
	NormalizedURL        string          `json:"normalized_url"`
	Platform             string          `json:"platform"`
	SurfaceType          string          `json:"surface_type"`
	OwnerType            string          `json:"owner_type"`
	CanonicalTargetURL   string          `json:"canonical_target_url"`
	BacklinkState        string          `json:"backlink_state"`
	SourceURL            string          `json:"source_url"`
	CanonicalStatus      string          `json:"canonical_status"`
	IndexabilityStatus   string          `json:"indexability_status"`
	PublicationStatus    string          `json:"publication_status"`
	OwnerConfidence      string          `json:"owner_confidence"`
	VerificationSnapshot json.RawMessage `json:"verification_snapshot"`
	RelatedActionIDs     json.RawMessage `json:"related_action_ids"`
}
```

After `UpsertGEOExternalSurface`, call `UpdateGEOExternalSurfaceMetadata`:

```go
row, err = s.Q.UpdateGEOExternalSurfaceMetadata(r.Context(), db.UpdateGEOExternalSurfaceMetadataParams{
	ID:                   row.ID,
	ProjectID:            projectID,
	SourceUrl:            strPtrFrom(in.SourceURL),
	CanonicalStatus:      textOr(in.CanonicalStatus, row.CanonicalStatus, "unknown"),
	IndexabilityStatus:   textOr(in.IndexabilityStatus, row.IndexabilityStatus, "unknown"),
	PublicationStatus:    textOr(in.PublicationStatus, row.PublicationStatus, "unknown"),
	OwnerConfidence:      ownerConfidenceOr(in.OwnerConfidence, row.OwnerConfidence),
	LastVerifiedAt:       pgtype.Timestamptz{},
	VerificationSnapshot: rawOrExisting(in.VerificationSnapshot, row.VerificationSnapshot, `{}`),
	RelatedActionIds:     rawOrExisting(in.RelatedActionIDs, row.RelatedActionIds, `[]`),
})
if err != nil {
	writeErr(w, http.StatusInternalServerError, err.Error())
	return
}
```

Add helpers near the existing helper functions in the same file:

```go
func textOr(value, existing, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(existing); trimmed != "" {
		return trimmed
	}
	return fallback
}

func ownerConfidenceOr(value, existing string) string {
	switch strings.TrimSpace(value) {
	case "high", "medium", "low":
		return strings.TrimSpace(value)
	}
	switch strings.TrimSpace(existing) {
	case "high", "medium", "low":
		return strings.TrimSpace(existing)
	}
	return "medium"
}

func rawOrExisting(raw, existing json.RawMessage, fallback string) json.RawMessage {
	if len(raw) > 0 && json.Valid(raw) {
		return raw
	}
	if len(existing) > 0 && json.Valid(existing) {
		return existing
	}
	return json.RawMessage(fallback)
}
```

- [ ] **Step 4: Verify green**

Run:

```bash
go test ./internal/api -run GeneralizedSurface -count=1
```

Expected: PASS.

## Task 2: Web API and Surface Inventory UI Contract

**Files:**
- Create: `web/app/lib/surface-inventory-contract.test.mjs`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Write the failing web contract test**

Create `web/app/lib/surface-inventory-contract.test.mjs`:

```js
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("GEO external surface type exposes generalized inventory metadata", () => {
  const api = read("lib/api.ts");
  for (const field of [
    "source_url?: string | null",
    "canonical_status: string",
    "indexability_status: string",
    "publication_status: string",
    "owner_confidence: string",
    "last_verified_at?: any",
    "verification_snapshot?: any",
    "related_action_ids: string[]",
  ]) {
    assert.match(api, new RegExp(field.replace(/[.*+?^${}()|[\\]\\\\]/g, "\\\\$&")));
  }
  for (const normalizer of [
    "source_url: data.source_url ?? null",
    "canonical_status: data.canonical_status ?? \"unknown\"",
    "indexability_status: data.indexability_status ?? \"unknown\"",
    "publication_status: data.publication_status ?? \"unknown\"",
    "owner_confidence: data.owner_confidence ?? \"medium\"",
    "related_action_ids: arrayFrom<string>(data.related_action_ids).map(String)",
  ]) {
    assert.match(api, new RegExp(normalizer.replace(/[.*+?^${}()|[\\]\\\\]/g, "\\\\$&")));
  }
  assert.match(api, /source_url\?: string/);
  assert.match(api, /publication_status\?: string/);
  assert.match(api, /owner_confidence\?: string/);
});

test("SEO visibility page renders generalized surface inventory controls", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const copy of ["Owner", "Platform", "Publication", "Indexability", "Canonical", "Confidence", "Source URL"]) {
    assert.match(seo, new RegExp(copy));
  }
  for (const state of ["surfaceOwnerType", "surfacePlatform", "surfacePublicationStatus", "surfaceIndexabilityStatus", "surfaceCanonicalStatus", "surfaceOwnerConfidence", "surfaceSourceURL"]) {
    assert.match(seo, new RegExp(state));
  }
  for (const payload of ["owner_type: surfaceOwnerType", "platform: surfacePlatform", "publication_status: surfacePublicationStatus", "indexability_status: surfaceIndexabilityStatus", "canonical_status: surfaceCanonicalStatus", "owner_confidence: surfaceOwnerConfidence", "source_url: surfaceSourceURL"]) {
    assert.match(seo, new RegExp(payload.replace(/[.*+?^${}()|[\\]\\\\]/g, "\\\\$&")));
  }
});
```

- [ ] **Step 2: Verify red**

Run:

```bash
cd web && node --test app/lib/surface-inventory-contract.test.mjs
```

Expected: FAIL because the web type, normalizer, request body, and surface card do not expose the generalized inventory fields.

- [ ] **Step 3: Extend `GEOExternalSurface` and its normalizer**

In `web/app/lib/api.ts`, extend `GEOExternalSurface`:

```ts
source_url?: string | null;
canonical_status: string;
indexability_status: string;
publication_status: string;
owner_confidence: string;
last_verified_at?: any;
verification_snapshot?: any;
related_action_ids: string[];
```

Update `normalizeGEOExternalSurface` to populate those fields:

```ts
source_url: data.source_url ?? null,
canonical_status: data.canonical_status ?? "unknown",
indexability_status: data.indexability_status ?? "unknown",
publication_status: data.publication_status ?? "unknown",
owner_confidence: data.owner_confidence ?? "medium",
last_verified_at: data.last_verified_at ?? undefined,
verification_snapshot: data.verification_snapshot ?? undefined,
related_action_ids: arrayFrom<string>(data.related_action_ids).map(String),
```

Update `createGEOExternalSurface` body type to accept:

```ts
body: {
  url: string;
  normalized_url?: string;
  platform?: string;
  surface_type?: string;
  owner_type?: string;
  canonical_target_url?: string;
  backlink_state?: string;
  source_url?: string;
  canonical_status?: string;
  indexability_status?: string;
  publication_status?: string;
  owner_confidence?: string;
  verification_snapshot?: any;
  related_action_ids?: string[];
},
```

- [ ] **Step 4: Extend the SEO visibility surface form**

In `web/app/projects/[id]/seo/seo-client.tsx`, add state next to `surfaceURL`:

```tsx
const [surfaceSourceURL, setSurfaceSourceURL] = useState("");
const [surfaceOwnerType, setSurfaceOwnerType] = useState("managed_external");
const [surfacePlatform, setSurfacePlatform] = useState("devto");
const [surfacePublicationStatus, setSurfacePublicationStatus] = useState("draft");
const [surfaceIndexabilityStatus, setSurfaceIndexabilityStatus] = useState("unknown");
const [surfaceCanonicalStatus, setSurfaceCanonicalStatus] = useState("unknown");
const [surfaceOwnerConfidence, setSurfaceOwnerConfidence] = useState("medium");
```

Update `addExternalSurface` to send the new metadata:

```tsx
await api.createGEOExternalSurface(projectId, {
  url: surfaceURL.trim(),
  owner_type: surfaceOwnerType,
  platform: surfacePlatform,
  publication_status: surfacePublicationStatus,
  indexability_status: surfaceIndexabilityStatus,
  canonical_status: surfaceCanonicalStatus,
  owner_confidence: surfaceOwnerConfidence,
  source_url: surfaceSourceURL.trim() || undefined,
  verification_snapshot: { source: "manual_inventory" },
});
setSurfaceURL("");
setSurfaceSourceURL("");
```

Replace the single URL row with compact inventory controls using native `select` elements styled like other dashboard inputs. Each select must have a visible label through the existing `Field` component:

```tsx
<Field label="Surface URL">
  <TextInput value={surfaceURL} onChange={(event) => setSurfaceURL(event.target.value)} placeholder="https://dev.to/team/source" />
</Field>
<Field label="Source URL">
  <TextInput value={surfaceSourceURL} onChange={(event) => setSurfaceSourceURL(event.target.value)} placeholder="https://example.com/blog/source" />
</Field>
<Field label="Owner">
  <select className="h-10 rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700" value={surfaceOwnerType} onChange={(event) => setSurfaceOwnerType(event.target.value)}>
    <option value="managed_external">Managed external</option>
    <option value="project">Owned</option>
    <option value="third_party">Third party</option>
  </select>
</Field>
```

Render each listed surface with platform, owner, publication status, indexability status, canonical status, and source URL when present.

- [ ] **Step 5: Verify green**

Run:

```bash
cd web && node --test app/lib/surface-inventory-contract.test.mjs
```

Expected: PASS.

## Task 3: Phase 3 Gate and Commit

- [ ] **Step 1: Run full phase gate**

Run:

```bash
make sqlc
go test ./internal/api -run GeneralizedSurface -count=1
cd web && node --test app/lib/surface-inventory-contract.test.mjs
make test
make build
cd web && node --test app/lib/multi-surface-seo-contract.test.mjs app/lib/surface-inventory-contract.test.mjs
cd web && VER_CEL_NOTE=unused VERCEL_ENV=preview npm run build
```

Expected: all commands exit 0. The `VERCEL_ENV=preview` prefix is required in this local worktree because production Clerk secrets are not present and `auth-config.ts` only bypasses unconfigured Clerk outside production Vercel.

- [ ] **Step 2: Commit Phase 3**

Run:

```bash
git add docs/superpowers/plans/2026-06-23-multi-surface-seo-growth-phase3.md internal/api/surface_inventory_contract_test.go internal/api/handlers_geo_pr2.go web/app/lib/api.ts web/app/lib/surface-inventory-contract.test.mjs 'web/app/projects/[id]/seo/seo-client.tsx'
git commit -m "feat(seo): generalize surface inventory metadata"
```

Expected: commit succeeds.

## Self-Review

- This phase implements the PRD surface inventory productization slice without renaming `geo_external_surfaces` or creating a parallel `seo_surfaces` table.
- It keeps owned project surfaces compatible with existing GEO code by preserving the historical `project` owner value for owned surfaces in the UI.
- It does not implement external publisher connectors, automated posting, or verification workers.
