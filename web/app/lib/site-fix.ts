// Shared Site Fix helpers: action-output text, direct-action detection, and the
// AI repair-payload builder. Extracted from seo-client.tsx so the dedicated Site
// Fixes page and the SEO/Results surfaces share one implementation.
import { SEOContentAction, ResultsAction } from "./api";
import { visibilityLifecycleLabel } from "./dashboard-ux-logic";
import type { SiteFix } from "./types";

export type SiteFixMeasurementPresentation = {
  label: string;
  detail: string;
  tone: "neutral" | "amber" | "green" | "red" | "blue";
  resultsHref: string;
  caution: string;
};

export function siteFixMeasurementPresentation(fix: SiteFix): SiteFixMeasurementPresentation {
  const summary = fix.measurement_summary;
  const resultsHref = summary?.results_deep_link ?? "";
  const caution = summary?.prospective_observation
    ? "Prospective observation: this started after deployment and supports quality monitoring, not directional attribution."
    : summary?.attribution_confidence === "low" || summary?.attribution_confidence === "none"
      ? "This is a low-confidence observation; review data quality before using it as directional evidence."
      : "";
  switch (fix.measurement_handoff_status) {
    case "not_applicable":
      return {
        label: "Verification only",
        detail: "This repair is complete at Verified and does not require a growth Results record.",
        tone: "neutral",
        resultsHref: "",
        caution: "",
      };
    case "pending":
      return {
        label: "Measurement pending",
        detail: "The verified repair is waiting for its independent Results measurement to start.",
        tone: "amber",
        resultsHref,
        caution,
      };
    case "started":
      return {
        label: "Measurement started",
        detail: summary?.status === "terminal" ? "The independent measurement reached a terminal outcome." : "The independent Results measurement is observing post-deploy signals.",
        tone: "blue",
        resultsHref,
        caution,
      };
    case "failed":
      return {
        label: "Measurement failed",
        detail: "The repair remains Verified, but the independent Results handoff needs attention.",
        tone: "red",
        resultsHref,
        caution,
      };
    case "not_started":
    default:
      return {
        label: fix.measurement_policy === "measurement_optional" ? "Measurement optional" : "Measurement not started",
        detail: fix.measurement_policy === "measurement_optional"
          ? "Verification is complete. Start a prospective observation only when ongoing Results monitoring is useful."
          : "Measurement begins only after the repair is Verified and a Results record exists.",
        tone: "neutral",
        resultsHref,
        caution,
      };
  }
}

function canonicalObject(value: unknown): Record<string, any> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, any>) : {};
}

export function canonicalSiteFixTarget(fix: SiteFix) {
  return fix.target_urls.find((url) => url.trim()) ?? "Project surface";
}

export function canonicalSiteFixTitle(fix: SiteFix) {
  const proposed = canonicalObject(fix.proposed_fix);
  return String(proposed.fix_intent ?? proposed.developer_instructions ?? `Doctor ${fix.finding_kind} fix`);
}

export function canonicalSiteFixStatusLabel(status: SiteFix["status"]) {
  switch (status) {
    case "proposed":
      return "Proposed";
    case "approved":
      return "Approved";
    case "preparing":
      return "Preparing";
    case "ready_to_apply":
      return "Ready to apply";
    case "applying":
      return "Applying";
    case "awaiting_deploy":
      return "Awaiting deploy";
    case "verifying":
      return "Verifying";
    case "failed_retryable":
      return "Verification needs retry";
    case "reopened":
      return "Reopened";
    case "verified":
      return "Verified";
    case "failed_terminal":
      return "Closed after failure";
    case "superseded":
      return "Superseded";
    case "migration_rolled_back":
      return "Migration rolled back";
  }
}

export function canonicalSiteFixNextAction(fix: SiteFix) {
  switch (fix.status) {
    case "proposed":
      return "Review the evidence and approve this fix.";
    case "approved":
    case "ready_to_apply":
      return "Create the approved repair pull request.";
    case "preparing":
      return "Pull request preparation is incomplete. Retry pull request creation to start a new audited generation attempt.";
    case "applying":
      return "Pull request creation is in progress; refresh for the latest repository handoff.";
    case "awaiting_deploy":
      return "The repair PR is merged. Wait for deployment, then start verification.";
    case "verifying":
      return "Verification is checking the acceptance criteria.";
    case "failed_retryable":
    case "reopened":
      return "Retry verification after addressing the failure evidence.";
    case "verified":
      return "Closed loop complete.";
    case "failed_terminal":
      return "This fix is closed. Create a new revision from Doctor if work is still required.";
    case "superseded":
      return "A newer Site Fix revision owns the next action.";
    case "migration_rolled_back":
      return "This migrated record is no longer active.";
  }
}

export function canonicalSiteFixAIJSON(fix: SiteFix) {
  return JSON.stringify(
    {
      site_fix_id: fix.id,
      doctor_finding_id: fix.doctor_finding_id,
      finding_kind: fix.finding_kind,
      target_urls: fix.target_urls,
      evidence_snapshot: fix.evidence_snapshot,
      proposed_fix: fix.proposed_fix,
      acceptance_tests: fix.acceptance_tests,
      verification_snapshot: fix.verification_snapshot,
    },
    null,
    2,
  );
}

export function compactOutcomeText(outcome: any) {
  if (!outcome || (typeof outcome === "object" && Object.keys(outcome).length === 0)) return "No outcome summary yet.";
  if (typeof outcome === "string") return outcome;
  if (typeof outcome === "object") {
    return Object.entries(outcome)
      .slice(0, 5)
      .map(([key, value]) => `${key}: ${typeof value === "object" ? JSON.stringify(value) : String(value)}`)
      .join(" / ");
  }
  return String(outcome);
}

export function hasNonEmptyStructuredValue(value: any) {
  if (!value) return false;
  if (typeof value === "string") {
    const trimmed = value.trim();
    return trimmed !== "" && trimmed !== "{}" && trimmed !== "[]" && trimmed !== "null";
  }
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === "object") return Object.keys(value).length > 0;
  return true;
}

export function hasActionVerificationSnapshot(action: SEOContentAction | ResultsAction) {
  return hasNonEmptyStructuredValue(action.verification_snapshot);
}

export function hasResultsExecutionEvidence(action: SEOContentAction | ResultsAction) {
  return Boolean(action.published_at || action.verified_at);
}

export function actionWhyNowText(action: SEOContentAction | ResultsAction) {
  const input = action.input_snapshot ?? {};
  const evidence = action.evidence_snapshot ?? {};
  const value =
    input.recommended_action ??
    input.query ??
    input.opportunity_type ??
    evidence.recommended_action ??
    evidence.query ??
    evidence.intent_type ??
    (action as ResultsAction).opportunity_recommended_action ??
    (action as ResultsAction).opportunity_query;
  return value ? String(value) : "Created from a reviewed visibility finding.";
}

export function actionSEOContributionText(action: SEOContentAction | ResultsAction) {
  const contribution = action.output_snapshot?.seo_geo_contribution;
  if (contribution) return String(contribution);
  const assetType = String(action.asset_type ?? "").toLowerCase();
  if (assetType === "metadata_rewrite") return "Improve SERP CTR and query-page relevance without publishing a new page.";
  if (assetType === "internal_link_patch") return "Move authority and context toward the target page so crawlers and answer engines can understand the cluster.";
  if (assetType === "schema_patch") return "Expose structured facts that search engines and answer engines can extract.";
  if (assetType === "sitemap_update" || assetType === "technical_fix") return "Improve crawl, indexability, and measurement reliability.";
  if (assetType.includes("glossary") || assetType.includes("geo")) return "Create answer-ready entities and citations for AI discovery surfaces.";
  return "Create or refresh an indexable asset that can earn rankings, citations, and downstream measurement.";
}

export function actionOutputTypeLabel(action: SEOContentAction | ResultsAction) {
  const outputType = String(action.output_snapshot?.output_type ?? action.diff_snapshot?.output_type ?? "").toLowerCase();
  if (outputType === "direct_patch") return "Direct patch";
  if (outputType === "technical_task") return "Technical task";
  const assetType = String(action.asset_type ?? "").toLowerCase();
  if (assetType.includes("patch") || assetType === "metadata_rewrite") return "Direct patch";
  if (assetType === "sitemap_update" || assetType === "technical_fix") return "Technical task";
  return "Topic-backed asset";
}

export const directActionAssetTypes = new Set(["internal_link_patch", "schema_patch", "sitemap_update", "technical_fix"]);

export function isDirectAction(action: SEOContentAction | ResultsAction) {
  const outputType = String(action.output_snapshot?.output_type ?? action.diff_snapshot?.output_type ?? "").toLowerCase();
  const assetType = String(action.asset_type ?? "").toLowerCase();
  return outputType === "direct_patch" || outputType === "technical_task" || directActionAssetTypes.has(assetType);
}

export function siteFixGitHubPRURL(action: SEOContentAction | ResultsAction) {
  const publisherResult = action.output_snapshot?.publisher_result;
  const url = publisherResult?.github_pr_url;
  return typeof url === "string" && url.trim() ? url.trim() : "";
}

export function siteFixPublisherResultStatus(action: SEOContentAction | ResultsAction) {
  const status = action.output_snapshot?.publisher_result?.status;
  return typeof status === "string" ? status.trim() : "";
}

export function siteFixVerificationLabel(action: SEOContentAction | ResultsAction) {
  if (!action.verified_at) return "";
  const source = String(action.verification_snapshot?.source ?? "").trim().toLowerCase();
  return source.startsWith("auto_") ? "Verified automatically" : "Verified";
}

export function siteFixPRLinkLabel(action: SEOContentAction | ResultsAction) {
  const result = action.output_snapshot?.publisher_result ?? {};
  const status = siteFixPublisherResultStatus(action);
  const state = String(result.github_pr_state ?? "").trim().toLowerCase();
  if (status === "github_pr_closed" || state === "closed") return "View closed PR";
  if (status === "github_pr_open" || state === "open") return "Open PR";
  if (status === "github_pr_merged" || state === "merged") return "View merged PR";
  if (action.verified_at || status === "verified") return "View merged PR";
  return "Open PR";
}

export function siteFixAlreadyMatchesSource(action: SEOContentAction | ResultsAction) {
  return siteFixPublisherResultStatus(action) === "already_applied";
}

export function siteFixFollowUpReason(action: SEOContentAction | ResultsAction) {
  const reason = action.output_snapshot?.publisher_result?.follow_up_reason;
  return typeof reason === "string" && reason.trim() ? reason.trim() : "";
}

export function actionPostExecutionText(action: SEOContentAction | ResultsAction) {
  if (action.status === "completed") return "Measurement complete";
  if (action.status === "measuring") return "Measuring impact";
  if (siteFixAlreadyMatchesSource(action)) return "Source already matches; verify production";
  if (action.verified_at) return "Applied or published and verified";
  if (action.status === "approved") return "Approved for execution";
  if (siteFixPublisherResultStatus(action) === "needs_follow_up") return siteFixFollowUpReason(action) || "Needs follow-up — merge or verify manually";
  if (siteFixPublisherResultStatus(action) === "github_pr_closed") return "PR closed without merging — reopen or dismiss";
  if (siteFixPublisherResultStatus(action) === "github_pr_merged") return "PR merged — verifying in production";
  if (action.status === "verification_pending") return "Waiting for production verification";
  if (action.status === "ready_for_review") return "Waiting for review";
  if (action.published_at) return "Published and waiting for verification";
  return action.status || "Queued";
}

export function actionOutputPreviewText(action: SEOContentAction | ResultsAction) {
  const output = action.output_snapshot ?? {};
  const diff = action.diff_snapshot ?? {};
  const directValue = output.deliverable ?? output.summary ?? output.title ?? output.proposed_copy ?? output.recommended_copy;
  if (directValue) return String(directValue);

  const proposedChanges = diff.proposed_changes;
  if (Array.isArray(proposedChanges) && proposedChanges.length > 0) {
    const change = proposedChanges[0];
    if (typeof change === "string") return change;
    if (change?.instruction) return String(change.instruction);
    if (change?.field && change?.after) return `${change.field}: ${change.after}`;
    if (change?.selector && change?.after) return `${change.selector}: ${change.after}`;
    return compactOutcomeText(change);
  }

  const checklist = diff.checklist ?? output.checklist;
  if (Array.isArray(checklist) && checklist.length > 0) {
    const item = checklist[0];
    if (typeof item === "string") return item;
    if (item?.task) return String(item.task);
    if (item?.instruction) return String(item.instruction);
    return compactOutcomeText(item);
  }

  const target = action.target_url ?? action.normalized_target_url;
  return target ? `Review proposed changes for ${target}.` : "Review the generated output before execution.";
}

export function firstProposedChange(action: SEOContentAction | ResultsAction) {
  const proposedChanges = action.diff_snapshot?.proposed_changes;
  if (!Array.isArray(proposedChanges) || proposedChanges.length === 0) return null;
  const first = proposedChanges[0];
  return first && typeof first === "object" && !Array.isArray(first) ? first : null;
}

export function stringArrayValue(value: any) {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string" && item.trim() !== "") : [];
}

export function siteFixAssetType(action: SEOContentAction | ResultsAction) {
  const change = firstProposedChange(action);
  return String(action.asset_type ?? change?.asset_type ?? "technical_fix").toLowerCase();
}

export function siteFixTargetURL(action: SEOContentAction | ResultsAction) {
  return action.target_url ?? action.normalized_target_url ?? action.diff_snapshot?.target_url ?? "";
}

export function siteFixTargetLabel(targetURL: string) {
  return targetURL || "the target URL";
}

export function normalizeMetadataKey(value: string) {
  return value.trim().toLowerCase().replace(/[_\-\s.]/g, "");
}

export function firstObservedMetadataStringIn(value: any, wanted: Set<string>): string {
  if (!value) return "";
  if (Array.isArray(value)) {
    for (const entry of value) {
      const found = firstObservedMetadataStringIn(entry, wanted);
      if (found) return found;
    }
    return "";
  }
  if (typeof value !== "object") return "";

  const record = value as Record<string, any>;
  for (const [key, entry] of Object.entries(record)) {
    if (!wanted.has(normalizeMetadataKey(key))) continue;
    if (typeof entry === "string" && entry.trim()) return entry.trim();
  }

  for (const preferred of ["observed_metadata", "metadata", "page_metadata", "seo_metadata", "open_graph", "opengraph"]) {
    const foundEntry = Object.entries(record).find(([key]) => normalizeMetadataKey(key) === normalizeMetadataKey(preferred));
    const found = firstObservedMetadataStringIn(foundEntry?.[1], wanted);
    if (found) return found;
  }

  for (const key of Object.keys(record).sort()) {
    const found = firstObservedMetadataStringIn(record[key], wanted);
    if (found) return found;
  }
  return "";
}

export function firstObservedMetadataString(value: any, aliases: string[]) {
  return firstObservedMetadataStringIn(value, new Set(aliases.map(normalizeMetadataKey)));
}

export function siteFixObservedMetadata(action: SEOContentAction | ResultsAction) {
  const diff = action.diff_snapshot ?? {};
  const output = action.output_snapshot ?? {};
  const evidence = action.evidence_snapshot ?? {};
  const change = firstProposedChange(action);
  const sources = [
    evidence.observed_metadata,
    output.observed_metadata,
    diff.observed_metadata,
    change?.observed_metadata,
    evidence.metadata,
    evidence.page_metadata,
    evidence.source_evidence,
    evidence,
  ];
  const fields = [
    { name: "canonical_url", aliases: ["canonical_url", "canonical", "canonicalUrl", "canonical_href", "canonicalHref"] },
    { name: "title", aliases: ["title", "page_title", "pageTitle"] },
    { name: "description", aliases: ["description", "meta_description", "metaDescription"] },
    { name: "og_title", aliases: ["og_title", "ogTitle"] },
    { name: "og_description", aliases: ["og_description", "ogDescription"] },
    { name: "og_image", aliases: ["og_image", "ogImage"] },
    { name: "brand_name", aliases: ["brand_name", "brandName", "site_name", "siteName", "og_site_name", "ogSiteName", "application_name", "applicationName"] },
  ];
  const observed: Record<string, string> = {};
  for (const field of fields) {
    for (const source of sources) {
      const found = firstObservedMetadataString(source, field.aliases);
      if (!found) continue;
      observed[field.name] = found;
      break;
    }
  }
  return observed;
}

export function siteFixCompactPayload(value: Record<string, any>) {
  return Object.fromEntries(
    Object.entries(value).filter(([, entry]) => {
      if (entry == null) return false;
      if (typeof entry === "string") return entry.trim() !== "";
      if (Array.isArray(entry)) return entry.length > 0;
      if (typeof entry === "object") return Object.keys(entry).length > 0;
      return true;
    }),
  );
}

export function metadataRewriteSources(action: SEOContentAction | ResultsAction) {
  const change = firstProposedChange(action);
  const diff = action.diff_snapshot ?? {};
  const output = action.output_snapshot ?? {};
  const input = action.input_snapshot ?? {};
  const evidence = action.evidence_snapshot ?? {};
  return [
    change,
    diff.ai_repair,
    output.ai_repair,
    evidence.source_evidence,
    evidence,
    input,
    diff,
    output,
  ].filter(Boolean);
}

export function firstSiteFixScalar(value: any) {
  if (typeof value === "string") return value.trim() || null;
  if (typeof value === "number" || typeof value === "boolean") return value;
  return null;
}

export function firstSiteFixValueIn(value: any, wanted: Set<string>): any {
  if (!value) return null;
  if (Array.isArray(value)) {
    for (const entry of value) {
      const found = firstSiteFixValueIn(entry, wanted);
      if (found != null) return found;
    }
    return null;
  }
  if (typeof value !== "object") return null;

  const record = value as Record<string, any>;
  for (const [key, entry] of Object.entries(record)) {
    if (!wanted.has(normalizeMetadataKey(key))) continue;
    const scalar = firstSiteFixScalar(entry);
    if (scalar != null) return scalar;
  }

  for (const preferred of ["observed", "observed_metadata", "metadata", "page_metadata", "seo_metadata", "technical", "raw_details", "opportunity", "proposed_change"]) {
    const foundEntry = Object.entries(record).find(([key]) => normalizeMetadataKey(key) === normalizeMetadataKey(preferred));
    const found = firstSiteFixValueIn(foundEntry?.[1], wanted);
    if (found != null) return found;
  }

  for (const key of Object.keys(record).sort()) {
    const found = firstSiteFixValueIn(record[key], wanted);
    if (found != null) return found;
  }
  return null;
}

export function firstSiteFixValue(sources: any[], aliases: string[]) {
  const wanted = new Set(aliases.map(normalizeMetadataKey));
  for (const source of sources) {
    const found = firstSiteFixValueIn(source, wanted);
    if (found != null) return found;
  }
  return null;
}

export function firstSiteFixValueInContainers(value: any, containerNames: Set<string>, aliases: Set<string>): any {
  if (!value) return null;
  if (Array.isArray(value)) {
    for (const entry of value) {
      const found = firstSiteFixValueInContainers(entry, containerNames, aliases);
      if (found != null) return found;
    }
    return null;
  }
  if (typeof value !== "object") return null;

  const record = value as Record<string, any>;
  for (const key of Object.keys(record).sort()) {
    if (!containerNames.has(normalizeMetadataKey(key))) continue;
    const found = firstSiteFixValueIn(record[key], aliases);
    if (found != null) return found;
  }
  for (const key of Object.keys(record).sort()) {
    const found = firstSiteFixValueInContainers(record[key], containerNames, aliases);
    if (found != null) return found;
  }
  return null;
}

export function firstMetadataRewriteProposedValue(sources: any[], aliases: string[]) {
  const containers = new Set(["proposed_change", "proposedChange", "proposed_metadata", "proposedMetadata", "metadata_rewrite", "metadataRewrite", "recommended_metadata", "recommendedMetadata", "recommendation"].map(normalizeMetadataKey));
  const wanted = new Set(aliases.map(normalizeMetadataKey));
  for (const source of sources) {
    const found = firstSiteFixValueInContainers(source, containers, wanted);
    if (found != null) return found;
  }
  return firstSiteFixValue(sources, aliases.filter((alias) => normalizeMetadataKey(alias).startsWith("proposed") || normalizeMetadataKey(alias).startsWith("recommended") || normalizeMetadataKey(alias).startsWith("new")));
}

export function firstMetadataRewriteObservedValue(sources: any[], aliases: string[]) {
  const containers = new Set(["observed", "observed_metadata", "observedMetadata", "metadata", "page_metadata", "pageMetadata", "seo_metadata", "seoMetadata", "technical", "raw_details", "rawDetails"].map(normalizeMetadataKey));
  const wanted = new Set(aliases.map(normalizeMetadataKey));
  for (const source of sources) {
    const found = firstSiteFixValueInContainers(source, containers, wanted);
    if (found != null) return found;
  }
  return firstSiteFixValue(sources, aliases.filter((alias) => normalizeMetadataKey(alias).startsWith("observed") || normalizeMetadataKey(alias).startsWith("current")));
}

export function metadataRewriteObservedSnapshot(action: SEOContentAction | ResultsAction) {
  const sources = metadataRewriteSources(action);
  return siteFixCompactPayload({
    status: firstMetadataRewriteObservedValue(sources, ["status", "http_status", "httpStatus", "status_code", "statusCode"]),
    title: firstMetadataRewriteObservedValue(sources, ["title", "page_title", "pageTitle", "current_title", "currentTitle", "observed_title", "observedTitle"]),
    meta_description: firstMetadataRewriteObservedValue(sources, ["meta_description", "metaDescription", "description", "current_meta_description", "currentMetaDescription", "observed_meta_description", "observedMetaDescription"]),
    canonical: firstMetadataRewriteObservedValue(sources, ["canonical", "canonical_url", "canonicalUrl", "canonical_href", "canonicalHref"]),
    robots: firstMetadataRewriteObservedValue(sources, ["robots", "robots_status", "robotsStatus", "robots_state", "robotsState", "meta_robots", "metaRobots", "indexability"]),
    observed_at: firstMetadataRewriteObservedValue(sources, ["observed_at", "observedAt", "checked_at", "checkedAt", "crawled_at", "crawledAt", "fetched_at", "fetchedAt"]),
  });
}

export function metadataRewriteOpportunityContext(action: SEOContentAction | ResultsAction) {
  const sources = metadataRewriteSources(action);
  return siteFixCompactPayload({
    query: (action as ResultsAction).opportunity_query ?? action.input_snapshot?.query ?? firstSiteFixValue(sources, ["query", "opportunity_query", "opportunityQuery"]),
    intent: firstSiteFixValue(sources, ["query_intent", "queryIntent", "intent", "intent_type", "intentType"]),
    problem_detail: firstSiteFixValue(sources, ["problem_detail", "problemDetail", "snippet_issue", "snippetIssue", "current_snippet_issue", "currentSnippetIssue", "issue_detail", "issueDetail"]),
    confidence: firstSiteFixValue(sources, ["confidence", "confidence_score", "confidenceScore"]),
    priority: firstSiteFixValue(sources, ["priority", "priority_score", "priorityScore"]),
    recommended_action: action.action_type,
  });
}

export function metadataRewriteProposedChange(action: SEOContentAction | ResultsAction) {
  const sources = metadataRewriteSources(action);
  return siteFixCompactPayload({
    title: firstMetadataRewriteProposedValue(sources, ["title", "proposed_title", "proposedTitle", "recommended_title", "recommendedTitle", "new_title", "newTitle"]),
    meta_description: firstMetadataRewriteProposedValue(sources, ["meta_description", "metaDescription", "description", "proposed_meta_description", "proposedMetaDescription", "recommended_meta_description", "recommendedMetaDescription", "new_meta_description", "newMetaDescription"]),
    seo_impact: firstMetadataRewriteProposedValue(sources, ["seo_impact", "seoImpact", "seo_contribution", "seoContribution"]),
    geo_impact: firstMetadataRewriteProposedValue(sources, ["geo_impact", "geoImpact", "geo_contribution", "geoContribution"]),
    content_support_required: firstMetadataRewriteProposedValue(sources, ["content_support_required", "contentSupportRequired", "requires_content_support", "requiresContentSupport"]),
    preserve: ["canonical", "indexability", "production URL"],
  });
}

export function isHomepageTarget(targetURL: string) {
  try {
    const parsed = new URL(targetURL);
    return parsed.pathname === "" || parsed.pathname === "/";
  } catch {
    return false;
  }
}

export function siteFixLikelySurfaces(assetType: string, targetURL: string) {
  const target = siteFixTargetLabel(targetURL);
  if (assetType === "schema_patch") {
    return [
      `Page route or template that renders ${target}`,
      "Shared SEO metadata or structured-data component used by that route",
      "Server-rendered head/layout file where JSON-LD can be emitted in initial HTML",
    ];
  }
  if (assetType === "internal_link_patch") {
    return [
      `Target page content for ${target}`,
      "Relevant source pages in the same topic cluster",
      "Navigation, related-content, or body-copy components that own internal links",
    ];
  }
  if (assetType === "sitemap_update") {
    return [
      "Production sitemap generator or sitemap.xml route",
      "Robots.txt sitemap declaration",
      `Canonical URL config for ${target}`,
    ];
  }
  return [
    `Page route, metadata config, or crawler-facing component for ${target}`,
    "Robots, canonical, redirect, sitemap, or server response configuration that controls discoverability",
  ];
}

export function siteFixImplementationSteps(assetType: string, actionType: string, targetURL: string) {
  if (assetType === "metadata_rewrite") {
    return [
      `Locate the page route, layout metadata, or SEO config that emits the production <title> and meta description for ${targetURL}.`,
      "Replace the existing title and meta description with the exact proposed_change values in this JSON.",
      "Preserve the canonical URL, indexability, and production host while checking OpenGraph and Twitter card metadata for intentional consistency.",
    ];
  }
  if (assetType === "schema_patch") {
    return [
      "Locate the route/template that renders the target URL and confirm whether JSON-LD already exists.",
      "Add or update server-rendered JSON-LD in a script[type=\"application/ld+json\"] block using real production page metadata.",
      "Preserve the canonical target URL, omit placeholder fields, and keep all URL fields absolute production URLs.",
    ];
  }
  if (assetType === "internal_link_patch") {
    return [
      "Identify source pages with topical relevance and enough body context to link naturally to the target URL.",
      "Add descriptive anchor text that matches the destination intent without keyword stuffing.",
      "Confirm the new links are crawlable HTML links and do not point through redirects or non-canonical URL variants.",
    ];
  }
  if (assetType === "sitemap_update") {
    return [
      "Locate the production sitemap generator and confirm the target URL inclusion or exclusion rule.",
      "Update sitemap and robots declarations so the canonical target URL is discoverable by crawlers.",
      "Keep generated sitemap URLs canonical, absolute, indexable, and free of staging or preview hosts.",
    ];
  }
  return [
    "Locate the code or configuration that controls the crawler-facing behavior for the target URL.",
    `Apply the requested site fix: ${actionType}.`,
    "Preserve canonical URLs, indexability, and production-only hosts while making the smallest safe change.",
  ];
}

export function siteFixDeduplicationRule(assetType: string) {
  if (assetType === "metadata_rewrite") {
    return "Update the existing title/meta description source of truth; check OpenGraph and Twitter card metadata for duplicates or conflicting values instead of adding parallel SEO signals.";
  }
  if (assetType === "schema_patch") {
    return "If JSON-LD already exists, update or extend the existing graph instead of adding duplicate Organization, WebSite, or WebPage nodes.";
  }
  if (assetType === "internal_link_patch") {
    return "If a crawlable canonical link to the target already exists on a source page, update anchor/context only when it improves clarity instead of adding duplicate boilerplate links.";
  }
  if (assetType === "sitemap_update") {
    return "Update the canonical sitemap entry or generation rule instead of adding duplicate URL variants.";
  }
  return "Update the existing crawler-facing signal when present instead of adding duplicate or conflicting signals.";
}

export function siteFixDoNot(assetType: string) {
  if (assetType === "schema_patch") {
    return [
      "Do not add unverified sameAs links.",
      "Do not add placeholder logo, address, founder, phone, or social profile fields.",
      "Do not inject JSON-LD only on the client after hydration.",
      "Do not change visible page content unless required.",
    ];
  }
  if (assetType === "internal_link_patch") {
    return [
      "Do not add links with generic anchor text such as click here.",
      "Do not point links at staging, preview, redirecting, or non-canonical URLs.",
      "Do not add duplicate navigation or footer links when contextual body links are the intended fix.",
    ];
  }
  return [
    "Do not add placeholder values or staging URLs.",
    "Do not change unrelated visible page content unless required.",
    "Do not create duplicate or conflicting SEO signals.",
  ];
}

export function siteFixHumanReview(assetType: string) {
  if (assetType === "schema_patch") {
    return {
      required: true,
      reason: "Structured data affects public search and entity interpretation and should use verified brand metadata only.",
      review_focus: ["brand name", "description", "canonical URL", "organization identity"],
    };
  }
  return {
    required: true,
    reason: "This fix changes crawler-facing production signals and should be reviewed before applying.",
    review_focus: ["target URL", "canonical URL", "production-only values"],
  };
}

export const siteFixSchemaGraphIDFragments = {
  organization: "#organization",
  website: "#website",
  webpage: "#webpage",
} as const;

export function siteFixSchemaFragmentID(targetURL: string, fragment: "organization" | "website" | "webpage") {
  const trimmed = targetURL.trim();
  const hash = siteFixSchemaGraphIDFragments[fragment];
  if (!trimmed) return hash;
  try {
    const parsed = new URL(trimmed);
    parsed.search = "";
    parsed.hash = hash;
    if (!parsed.pathname) parsed.pathname = "/";
    return parsed.toString();
  } catch {
    return `${trimmed.replace(/\/+$/, "")}/${hash}`;
  }
}

export function siteFixSchemaGraphGuidance(targetURL: string) {
  const webpageID = siteFixSchemaFragmentID(targetURL, "webpage");
  if (!isHomepageTarget(targetURL)) {
    return {
      recommended_shape: "Use one JSON-LD object with @context set to https://schema.org and an @graph array.",
      stable_ids: {
        WebPage: webpageID,
      },
      relationships: ["Use stable @id values so entities can reference each other without duplicating nodes."],
      example: {
        "@context": "https://schema.org",
        "@graph": [{ "@type": "WebPage", "@id": webpageID }],
      },
    };
  }
  const organizationID = siteFixSchemaFragmentID(targetURL, "organization");
  const websiteID = siteFixSchemaFragmentID(targetURL, "website");
  return {
    recommended_shape: "Use one JSON-LD object with @context set to https://schema.org and an @graph array.",
    stable_ids: {
      Organization: organizationID,
      WebSite: websiteID,
      WebPage: webpageID,
    },
    relationships: [
      "WebSite.publisher should reference the Organization @id.",
      "WebPage.isPartOf should reference the WebSite @id.",
      "WebPage.about or WebPage.publisher should reference the Organization @id when verified.",
    ],
    example: {
      "@context": "https://schema.org",
      "@graph": [
        { "@type": "Organization", "@id": organizationID },
        { "@type": "WebSite", "@id": websiteID },
        { "@type": "WebPage", "@id": webpageID },
      ],
    },
  };
}

export function siteFixPatchContract(assetType: string, targetURL: string) {
  if (assetType === "schema_patch") {
    return {
      change_type: "json_ld_schema_patch",
      target_url: targetURL,
      page_role: isHomepageTarget(targetURL) ? "homepage" : "web_page",
      schema_types: isHomepageTarget(targetURL) ? ["WebSite", "Organization", "WebPage"] : ["WebPage"],
      render_requirement: "JSON-LD must be present in the initial server-rendered HTML.",
      deduplication_rule: siteFixDeduplicationRule(assetType),
      graph_guidance: siteFixSchemaGraphGuidance(targetURL),
      do_not: siteFixDoNot(assetType),
      constraints: [
        "Use real production brand, page, and canonical metadata.",
        "Use absolute production URLs only.",
        "Omit fields that cannot be verified instead of shipping blank or placeholder values.",
        siteFixDeduplicationRule(assetType),
      ],
    };
  }
  if (assetType === "internal_link_patch") {
    return {
      change_type: "internal_link_patch",
      target_url: targetURL,
      constraints: [
        "Links must be crawlable HTML anchors.",
        "Anchor copy must describe the destination intent.",
        "Use canonical production URLs and avoid redirect chains.",
      ],
    };
  }
  if (assetType === "metadata_rewrite") {
    return {
      change_type: "metadata_rewrite",
      target_url: targetURL,
      constraints: [
        "Update the existing title and meta description signal instead of creating new page content.",
        "Use only reviewed production copy values from proposed_change.",
        "Do not use staging, preview, localhost, or placeholder URLs.",
        "Verify the exact crawler-facing values in production after deployment.",
      ],
      preserve: ["canonical", "indexability", "production URL"],
    };
  }
  return {
    change_type: assetType,
    target_url: targetURL,
    constraints: [
      "Make the smallest production-safe change that resolves the crawler-facing issue.",
      "Do not use staging, preview, localhost, or placeholder URLs.",
      "Verify the signal in production after deployment.",
    ],
  };
}

export function fallbackSiteFixAcceptanceTests(assetType: string, actionType: string, targetURL: string) {
  const target = siteFixTargetLabel(targetURL);
  if (assetType === "schema_patch") {
    return [
      `Inspect the initial HTML for ${target} and verify it includes server-rendered JSON-LD in a script[type=\"application/ld+json\"] element.`,
      "Parse every JSON-LD block as valid JSON and verify it has @context set to https://schema.org, a relevant @type, and no placeholders.",
      `Validate the JSON-LD with Schema Markup Validator for ${target} and resolve every parser error.`,
      "Use Google Rich Results Test only to confirm the page is readable and parser-error free; WebSite, Organization, and WebPage schema does not require rich result eligibility.",
    ];
  }
  if (assetType === "internal_link_patch") {
    return [
      `Fetch the updated source pages and confirm they contain crawlable HTML links to ${target}.`,
      "Verify anchor text is descriptive, unique enough to explain the destination, and does not duplicate existing boilerplate links.",
      "Confirm linked URLs resolve to canonical production URLs without redirect chains.",
    ];
  }
  if (assetType === "sitemap_update") {
    return [
      "Fetch the production sitemap and confirm it contains the canonical target URL when the page should be indexed.",
      "Fetch robots.txt and confirm it advertises the correct sitemap and does not block the target URL.",
      "Confirm the sitemap URL returns 200, valid XML, production hosts only, and no non-canonical variants.",
    ];
  }
  return [
    `Fetch ${target} and confirm the crawler-facing behavior now matches the requested site fix: ${actionType}.`,
    "Run the relevant SEO/technical check again and confirm the active finding no longer appears for the target URL.",
    "Confirm production pages still return the expected status, canonical URL, and indexability signals.",
  ];
}

export function metadataRewriteAcceptanceTests(action: SEOContentAction | ResultsAction, targetURL: string) {
  const proposed = metadataRewriteProposedChange(action);
  const observed = metadataRewriteObservedSnapshot(action);
  const tests: string[] = [];
  if (typeof proposed.title === "string" && proposed.title.trim()) {
    tests.push("Fetch " + targetURL + " and confirm the initial HTML <title> equals " + JSON.stringify(proposed.title.trim()) + ".");
  }
  if (typeof proposed.meta_description === "string" && proposed.meta_description.trim()) {
    tests.push("Fetch " + targetURL + " and confirm meta[name=\"description\"] equals " + JSON.stringify(proposed.meta_description.trim()) + ".");
  }
  if (typeof observed.canonical === "string" && observed.canonical.trim()) {
    tests.push(`Confirm canonical URL remains "${observed.canonical.trim()}".`);
  } else {
    tests.push(`Confirm canonical URL remains the production canonical URL for ${targetURL}.`);
  }
  tests.push(
    "Confirm the page remains indexable: no noindex robots meta, no blocking X-Robots-Tag, and robots.txt does not disallow the URL.",
    "Check OpenGraph and Twitter card title/description values for duplicate or conflicting metadata signals.",
    "Run the relevant SEO/technical check again and confirm the active finding no longer appears for the target URL.",
  );
  return tests;
}

export function siteFixAcceptanceTests(action: SEOContentAction | ResultsAction) {
  const diff = action.diff_snapshot ?? {};
  const change = firstProposedChange(action);
  const assetType = siteFixAssetType(action);
  if (assetType === "metadata_rewrite") return metadataRewriteAcceptanceTests(action, siteFixTargetURL(action));
  const direct = stringArrayValue(diff.acceptance_tests);
  if (direct.length > 0) return direct;
  const changeTests = stringArrayValue(change?.verification_steps);
  if (changeTests.length > 0) return changeTests;
  return fallbackSiteFixAcceptanceTests(assetType, action.action_type, siteFixTargetURL(action));
}

export function buildSiteFixAIPayload(action: SEOContentAction | ResultsAction) {
  const diff = action.diff_snapshot ?? {};
  const output = action.output_snapshot ?? {};
  const aiRepair = diff.ai_repair ?? output.ai_repair;
  if (hasNonEmptyStructuredValue(aiRepair)) return aiRepair;

  const change = firstProposedChange(action);
  const assetType = siteFixAssetType(action);
  const target = siteFixTargetURL(action);
  const implementationSteps = stringArrayValue(change?.implementation_steps);
  const likelySurfaces = stringArrayValue(change?.likely_surfaces);
  const observedMetadata = siteFixObservedMetadata(action);
  const metadataRewrite = assetType === "metadata_rewrite";
  const observed = metadataRewrite ? metadataRewriteObservedSnapshot(action) : {};
  const opportunity = metadataRewrite ? metadataRewriteOpportunityContext(action) : {};
  const proposedChange = metadataRewrite ? metadataRewriteProposedChange(action) : {};
  return {
    issue: {
      category: "site_fix",
      issue_type: assetType,
      affected_urls: target ? [target] : [],
      problem: action.action_type,
      why_it_matters: actionSEOContributionText(action),
    },
    ...(metadataRewrite ? { observed, opportunity, proposed_change: proposedChange } : {}),
    evidence: {
      page_url: target,
      opportunity_query: (action as ResultsAction).opportunity_query ?? action.input_snapshot?.query ?? null,
      recommended_action: metadataRewrite ? action.action_type : (action as ResultsAction).opportunity_recommended_action ?? action.input_snapshot?.recommended_action ?? action.action_type,
      proposed_changes: diff.proposed_changes ?? [],
      ...(Object.keys(observedMetadata).length > 0 ? { observed_metadata: observedMetadata } : {}),
    },
    fix: {
      goal: action.action_type,
      instructions: implementationSteps.length ? implementationSteps : siteFixImplementationSteps(assetType, action.action_type, target),
      likely_surfaces: likelySurfaces.length ? likelySurfaces : siteFixLikelySurfaces(assetType, target),
      seo_contract: change?.patch_contract ?? siteFixPatchContract(assetType, target),
      deduplication_rule: siteFixDeduplicationRule(assetType),
      do_not: siteFixDoNot(assetType),
      risk_level: action.risk_reasons?.risk_level ?? null,
    },
    acceptance_tests: siteFixAcceptanceTests(action),
    human_review: siteFixHumanReview(assetType),
  };
}

export function siteFixAIJSON(action: SEOContentAction | ResultsAction) {
  return JSON.stringify(buildSiteFixAIPayload(action), null, 2);
}

// Display helpers shared with the SEO/Results surfaces.
export function toneForStatus(status: string): "green" | "amber" | "red" | "neutral" {
  if (["open", "ready_for_review", "approved", "measuring", "ok", "connected"].includes(status)) return "green";
  if (["degraded", "accepted", "converted", "drafting"].includes(status)) return "amber";
  if (["error", "failed", "expired"].includes(status)) return "red";
  return "neutral";
}

export function measurementWindowLabel(measurement_window: any) {
  const structured = Array.isArray(measurement_window?.checkpoints)
    ? measurement_window.checkpoints.map((checkpoint: any) => checkpoint?.day).filter(Boolean)
    : [];
  const legacy = Array.isArray(measurement_window?.checkpoints_days) ? measurement_window.checkpoints_days : [];
  const checkpoints: Array<number | string> = structured.length > 0 ? structured : legacy;
  if (checkpoints.length === 0) return "Not scheduled";
  const metric = measurement_window?.primary_metric ? `${measurement_window.primary_metric}: ` : "";
  return `Scheduled: ${metric}${checkpoints.map((day) => `D+${day}`).join(" / ")}`;
}

export function approvalSourceLabel(source?: string | null) {
  switch (source) {
    case "autopilot_policy":
      return "Approved by Autopilot policy";
    case "manual":
      return "Created manually by user";
    case "retry_recovery":
      return "Retry of approved work";
    case "admin_import":
      return "Imported by admin";
    case "human_review":
    default:
      return "Human opportunity approval";
  }
}

export function humanizeInternalType(value: string) {
  const spaced = value.replace(/[_-]+/g, " ").trim();
  if (!spaced) return value;
  return spaced
    .replace(/\b(gsc|geo|ctr|seo|url)\b/gi, (match) => match.toUpperCase())
    .replace(/^[a-z]/, (match) => match.toUpperCase());
}

export function lifecycleStageLabel(stage: string) {
  switch (stage) {
    case "detected":
      return "Detected";
    case "added_to_plan":
      return "Added";
    case "planned":
      return "Topic planned";
    case "drafting":
      return "Drafting";
    case "ready_for_review":
      return "Review";
    case "approved":
      return "Approved";
    case "published_or_applied":
      return "Published/Applied";
    case "measuring":
      return "Measuring";
    case "learned":
      return "Learned";
    case "blocked":
      return "Blocked";
    default:
      return visibilityLifecycleLabel(stage);
  }
}

export function lifecycleStageTone(stage: string): "green" | "amber" | "red" | "neutral" {
  if (["learned", "published_or_applied", "measuring"].includes(stage)) return "green";
  if (["added_to_plan", "planned", "drafting", "ready_for_review", "approved"].includes(stage)) return "amber";
  if (stage === "blocked") return "red";
  return "neutral";
}
