"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { BarChart3, CheckCircle2, ChevronRight, Clipboard, FileText, History, RefreshCw, Search, Settings, ShieldAlert, Wrench, X } from "lucide-react";
import {
  ActionMeasurement,
  AICrawlerAccessSnapshot,
  AutopilotExecuteResult,
  AutopilotReadiness,
  GEOAssetBrief,
  GEOCompetitor,
  GSCConnection,
  GEOOverview,
  GEOPrompt,
  SEOActionPlan,
  SEOBrief,
  SEOContentAction,
  SEOIntegration,
  SEOObjective,
  SEOOpportunity,
  SEOOverview,
  OpportunityFindingStatus,
  PublisherConnection,
  SEOProperty,
  SEOPolicy,
  SEOWatchlistItem,
  SafeModeEvent,
  ResultsAction,
  VisibilityActionInLoop,
  VisibilityLifecycleStage,
  VisibilitySummary,
} from "../../../lib/api";
import { visibilityLifecycleLabel } from "../../../lib/dashboard-ux-logic";
import { normalizeNumeric } from "../../../lib/normalize";
import { deriveVisibilityLifecycleStage, visibilityLifecycleCounts } from "../../../lib/visibility-lifecycle";
import {
  actionOutputTypeLabel,
  actionPostExecutionText,
  actionSEOContributionText,
  actionWhyNowText,
  approvalSourceLabel,
  compactOutcomeText,
  directActionAssetTypes,
  hasActionVerificationSnapshot,
  hasResultsExecutionEvidence,
  humanizeInternalType,
  isDirectAction,
  lifecycleStageLabel,
  lifecycleStageTone,
  measurementWindowLabel,
  toneForStatus,
} from "../../../lib/site-fix";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";
import { RightDrawer } from "../../../components/right-drawer";
import { Badge, Button, ButtonProgress, EmptyState, Field, Notice, SectionHeader, TextInput, cx, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type LoopAction = SEOContentAction &
  Partial<
    Pick<
      VisibilityActionInLoop,
      | "lifecycle_stage"
      | "draft_article_id"
      | "draft_article_canonical_url"
      | "opportunity_page_url"
      | "opportunity_normalized_page_url"
      | "opportunity_query"
      | "opportunity_recommended_action"
      | "opportunity_status"
      | "topic_title"
    >
  >;
const drawerFocusableSelector =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

function metric(value: any, digits = 0) {
  const n = normalizeNumeric(value);
  if (n == null) return "-";
  return n.toLocaleString("en", { maximumFractionDigits: digits, minimumFractionDigits: digits });
}

function percent(value: any) {
  const n = normalizeNumeric(value);
  if (n == null) return "-";
  return `${(n * 100).toFixed(1)}%`;
}

function toneForRisk(risk?: string): "green" | "amber" | "red" | "neutral" {
  if (risk === "low") return "green";
  if (risk === "medium") return "amber";
  if (risk === "high") return "red";
  return "neutral";
}


function toneForSetupStatus(status?: string): "green" | "amber" | "red" | "neutral" {
  if (status === "connected") return "green";
  if (["not_started", "in_progress", "optional"].includes(status ?? "")) return "amber";
  if (["blocked", "error", "expired"].includes(status ?? "")) return "red";
  return "neutral";
}

function toneForRobots(state?: string): "green" | "amber" | "red" | "neutral" {
  if (state === "allowed") return "green";
  if (state === "disallowed") return "red";
  if (state === "unknown") return "amber";
  return "neutral";
}

function toneForAccess(state?: string): "green" | "amber" | "red" | "neutral" {
  if (state === "ok") return "green";
  if (["challenge", "rate_limited", "timeout"].includes(state ?? "")) return "amber";
  if (["blocked", "error"].includes(state ?? "")) return "red";
  return "neutral";
}

function toneForConfidence(confidence?: string): "green" | "amber" | "red" | "neutral" {
  if (confidence === "high") return "green";
  if (confidence === "medium") return "amber";
  if (confidence === "low") return "neutral";
  return "neutral";
}

function opportunityTitle(opportunity: SEOOpportunity) {
  return opportunity.recommended_action || opportunity.query || opportunity.page_url || opportunity.type || "Visibility opportunity";
}

function assetTypeForOpportunity(opportunity: SEOOpportunity) {
  const type = opportunity.type.toLowerCase();
  const text = `${opportunity.type} ${opportunity.recommended_action ?? ""}`.toLowerCase();
  const words = text.replace(/[_-]/g, " ");
  if (type === "schema_gap" || words.includes("schema")) return "schema_patch";
  if (type === "internal_link_gap" || words.includes("internal link")) return "internal_link_patch";
  if (words.includes("metadata") || words.includes("title") || words.includes("meta")) return "metadata_rewrite";
  if (words.includes("sitemap")) return "sitemap_update";
  if (type === "gsc_query_cannibalization" || words.includes("cannibal") || words.includes("consolidat")) return "technical_fix";
  if (type === "technical_visibility_issue" || words.includes("robots") || words.includes("canonical") || words.includes("crawler") || words.includes("technical")) return "technical_fix";
  if (words.includes("geo") || words.includes("citation") || words.includes("answer engine")) return "glossary_definition";
  if (words.includes("comparison")) return "comparison_page";
  if (words.includes("alternative")) return "alternative_page";
  if (words.includes("template") || words.includes("checklist")) return "template_or_checklist";
  return "blog_post";
}

function selectedPortfolioActions(plan: SEOActionPlan) {
  return plan.portfolio.selected_actions;
}

function measurementLabel(schedule: any) {
  const checkpoints: Array<number | string> = Array.isArray(schedule?.checkpoints) ? schedule.checkpoints : [];
  if (checkpoints.length === 0) return "Not scheduled";
  return checkpoints.map((day) => `D+${day}`).join(" / ");
}


function settledValue<T>(result: PromiseSettledResult<T>): T | null {
  return result.status === "fulfilled" ? result.value : null;
}

function analysisSearchDataStatus(overview: SEOOverview | null, gscStatus: string) {
  const capabilityMode = overview?.capability_mode ?? "public_only";
  const integration = overview?.integrations.find((item) => item.provider === "google_search_console");
  if (gscStatus === "connected") {
    return {
      tone: "green" as const,
      label: "Search Console connected",
      detail: "CiteLoop can use first-party search data when prioritizing recommendations.",
      action: null,
    };
  }
  if (gscStatus === "backfilling") {
    return {
      tone: "amber" as const,
      label: "Backfilling Search Console",
      detail: "CiteLoop is importing the first search data window. Analysis stays public-only until enough rows are ready.",
      action: null,
    };
  }
  if (gscStatus === "stale") {
    return {
      tone: "red" as const,
      label: "Search data is stale",
      detail: "Reconnect or sync Search Console before trusting fresh query, CTR, or position signals.",
      action: "Reconnect Search Console",
    };
  }
  if (gscStatus === "mismatch") {
    return {
      tone: "red" as const,
      label: "Property mismatch",
      detail: "The selected Search Console property no longer matches this project domain. Select the matching property before using private search data.",
      action: "Reconnect Search Console",
    };
  }
  if (["error", "expired", "revoked"].includes(gscStatus)) {
    return {
      tone: "red" as const,
      label: "Search Console needs attention",
      detail: "Reconnect Search Console before trusting fresh query, CTR, or position signals.",
      action: "Reconnect Search Console",
    };
  }
  if (integration?.status === "connected" || capabilityMode === "customer_site_connected" || capabilityMode === "managed_content_connected") {
    return {
      tone: "green" as const,
      label: "Search Console connected",
      detail: "CiteLoop can use first-party search data when prioritizing recommendations.",
      action: null,
    };
  }
  if (capabilityMode === "customer_site_pending_verification") {
    return {
      tone: "amber" as const,
      label: "Search Console verification pending",
      detail: "Finish site verification to unlock first-party query, CTR, position, and decay signals.",
      action: "Finish setup",
    };
  }
  return {
    tone: "amber" as const,
    label: "Search data not connected",
    detail: "CiteLoop can still review public opportunities. Connect Search Console for query, CTR, position, and content decay evidence.",
    action: "Connect Search Console",
  };
}

function compactGSCStatus(status: ReturnType<typeof analysisSearchDataStatus>) {
  const connected = status.tone === "green" || status.label === "Backfilling Search Console";
  return {
    label: connected ? "GSC Connected" : "GSC Not connected",
    tone: connected ? ("green" as const) : ("red" as const),
    dot: connected ? "bg-green-500" : "bg-red-500",
  };
}

function compactGA4Status(status?: string, propertyID?: string | null) {
  const savedPropertyID = propertyID?.trim();
  if (status === "connected") {
    return {
      label: "GA4 Connected",
      dot: "bg-green-500",
      detail: "GA4 engagement and key event data can be used in sync and measurement.",
    };
  }
  if (["error", "expired", "revoked"].includes(status ?? "")) {
    return {
      label: "GA4 Needs attention",
      dot: "bg-red-500",
      detail: "Open Google Connection settings to review the saved GA4 property and latest error.",
    };
  }
  if (savedPropertyID) {
    return {
      label: "GA4 Property saved",
      dot: "bg-amber-500",
      detail: "Run Sync after GA4 has collected data and the property is accessible to CiteLoop.",
    };
  }
  return {
    label: "GA4 Not connected",
    dot: "bg-slate-300",
    detail: "Add the numeric GA4 Property ID in Google Connection settings.",
  };
}

function searchDataModeLabel(overview: SEOOverview | null, status: ReturnType<typeof analysisSearchDataStatus>) {
  if (status.label === "Backfilling Search Console") return "Backfilling Search Console";
  if (status.tone === "green" && overview?.cold_start) return "Low click depth";
  if (status.tone === "green") return "First-party search data";
  if (status.label === "Search data is stale") return "Last-known search data";
  return "Public crawl only";
}

function findingTypeLabel(opportunity: SEOOpportunity) {
  const type = opportunity.type.toLowerCase();
  const text = `${opportunity.type} ${opportunity.recommended_action ?? ""}`.toLowerCase();
  const words = text.replace(/[_-]/g, " ");
  if (type === "internal_link_gap" || words.includes("internal link")) return "Internal-link gap";
  if (type === "schema_gap" || words.includes("schema")) return "Schema gap";
  if (type === "thin_evidence_page" || words.includes("thin evidence") || words.includes("evidence block")) return "Evidence gap";
  if (type === "gsc_query_cannibalization" || words.includes("cannibal") || words.includes("consolidat")) return "Cannibalization";
  if (words.includes("ctr") || words.includes("title") || words.includes("meta")) return "CTR opportunity";
  if (words.includes("decay") || words.includes("refresh")) return "Refresh candidate";
  if (words.includes("near") || words.includes("page one") || words.includes("ranking")) return "Striking distance";
  if (type === "technical_visibility_issue" || words.includes("index") || words.includes("sitemap") || words.includes("robots") || words.includes("crawler") || words.includes("technical")) return "Technical finding";
  if (words.includes("geo") || words.includes("citation")) return "AI citation gap";
  if (words.includes("competitive") || words.includes("comparison") || words.includes("alternative")) return "Market gap";
  if (words.includes("cold start")) return "Cold-start finding";
  return "Growth finding";
}

type OpportunityWorkType = "Create Content" | "Improve Page" | "Fix Site Issue";
type OpportunityDestination = "Content Plan" | "Site Fixes";

function opportunitySearchText(opportunity: SEOOpportunity) {
  return `${opportunity.type} ${opportunity.recommended_action ?? ""} ${opportunity.expected_impact ?? ""}`.toLowerCase().replace(/[_-]/g, " ");
}

function opportunityWorkType(opportunity: SEOOpportunity): OpportunityWorkType {
  const type = opportunity.type.toLowerCase();
  const words = opportunitySearchText(opportunity);
  if (
    type === "internal_link_gap" ||
    type === "schema_gap" ||
    type === "technical_visibility_issue" ||
    type === "geo_crawler_access_blocked" ||
    words.includes("internal link") ||
    words.includes("schema") ||
    words.includes("index") ||
    words.includes("sitemap") ||
    words.includes("crawler") ||
    words.includes("robots") ||
    words.includes("canonical")
  ) {
    return "Fix Site Issue";
  }
  if (
    type === "gsc_low_ctr_query" ||
    type === "gsc_striking_distance_query" ||
    type === "gsc_content_decay" ||
    type === "thin_evidence_page" ||
    type === "gsc_query_cannibalization" ||
    words.includes("refresh") ||
    words.includes("decay") ||
    words.includes("ctr") ||
    words.includes("title") ||
    words.includes("meta") ||
    words.includes("near") ||
    words.includes("cannibal") ||
    words.includes("consolidat") ||
    words.includes("evidence block") ||
    words.includes("source backed")
  ) {
    return "Improve Page";
  }
  return "Create Content";
}

function opportunityDestination(opportunity: SEOOpportunity): OpportunityDestination {
  return destinationForWorkType(opportunityWorkType(opportunity));
}

function destinationForWorkType(workType: OpportunityWorkType): OpportunityDestination {
  return workType === "Fix Site Issue" ? "Site Fixes" : "Content Plan";
}

function ctaForWorkType(workType: OpportunityWorkType) {
  if (workType === "Fix Site Issue") return { label: "Create Site Fix", busyLabel: "Creating site fix" };
  if (workType === "Improve Page") return { label: "Create Page Update", busyLabel: "Creating page update" };
  return { label: "Add to Content Plan", busyLabel: "Adding to plan" };
}

function opportunityPrimaryCTA(opportunity: SEOOpportunity) {
  return ctaForWorkType(opportunityWorkType(opportunity));
}

const workTypeKeys: Record<OpportunityWorkType, string> = {
  "Create Content": "create_content",
  "Improve Page": "improve_page",
  "Fix Site Issue": "fix_site_issue",
};

// Review drawer route override (PRD §6.2): content-route opportunities may be
// corrected between Create Content and Improve Page; technically certain site
// fixes stay locked to Site Fixes.
function allowedWorkTypesForOpportunity(opportunity: SEOOpportunity): OpportunityWorkType[] {
  if (opportunityWorkType(opportunity) === "Fix Site Issue") return ["Fix Site Issue"];
  return ["Create Content", "Improve Page"];
}

function workTypeLockReason(opportunity: SEOOpportunity) {
  return `This is a site fix because the finding (${humanizeInternalType(opportunity.type)}) must be corrected on the site itself.`;
}

// §5.3 approval copy per destination.
function approvalCopyForWorkType(workType: OpportunityWorkType) {
  if (workType === "Fix Site Issue") return "Approve to create a Site Fix.";
  return `Approve to send this to Content Plan.`;
}



function watchlistItemTitle(item: SEOWatchlistItem) {
  return (
    item.opportunity_recommended_action ??
    (item.opportunity_type ? humanizeInternalType(item.opportunity_type) : "Watched opportunity")
  );
}

function watchlistStatusLabel(status: string) {
  switch (status) {
    case "watching":
      return "Watching";
    case "due_for_review":
      return "Due for review";
    case "learned":
      return "Learned";
    case "closed":
      return "Closed";
    default:
      return humanizeInternalType(status);
  }
}

function watchlistStatusTone(status: string): "green" | "amber" | "red" | "neutral" {
  if (status === "due_for_review") return "amber";
  if (status === "learned") return "green";
  return "neutral";
}

function assetTypeForWorkType(opportunity: SEOOpportunity, workType: OpportunityWorkType) {
  if (workType === "Create Content") return "blog_post";
  if (workType === "Improve Page") return "page_update";
  const inferred = assetTypeForOpportunity(opportunity);
  return directActionAssetTypes.has(inferred) ? inferred : "technical_fix";
}

function actionCtaForOpportunity(opportunity: SEOOpportunity) {
  const type = opportunity.type.toLowerCase();
  const words = opportunitySearchText(opportunity);
  if (type === "gsc_query_cannibalization" || words.includes("cannibal") || words.includes("consolidat")) {
    return { label: "Create Page Update", busyLabel: "Creating page update" };
  }
  if (type === "thin_evidence_page" || words.includes("thin evidence") || words.includes("evidence block") || words.includes("source backed")) {
    return { label: "Create Page Update", busyLabel: "Creating page update" };
  }
  if (type === "internal_link_gap" || words.includes("internal link")) {
    return { label: "Create Site Fix", busyLabel: "Creating site fix" };
  }
  if (type === "schema_gap" || type === "technical_visibility_issue" || words.includes("index") || words.includes("sitemap") || words.includes("schema") || words.includes("crawler") || words.includes("robots") || words.includes("canonical")) {
    return { label: "Create Site Fix", busyLabel: "Creating site fix" };
  }
  if (words.includes("geo") || words.includes("citation") || words.includes("answer engine")) {
    return { label: "Add to Content Plan", busyLabel: "Adding to plan" };
  }
  if (words.includes("refresh") || words.includes("decay") || words.includes("ctr") || words.includes("title") || words.includes("meta") || words.includes("near")) {
    return { label: "Create Page Update", busyLabel: "Creating page update" };
  }
  if (words.includes("watch") || words.includes("wait") || words.includes("monitor")) {
    return { label: "Add to Content Plan", busyLabel: "Adding to plan" };
  }
  return opportunityPrimaryCTA(opportunity);
}

function destinationForAction(action: SEOContentAction | ResultsAction): OpportunityDestination {
  return isDirectAction(action) ? "Site Fixes" : "Content Plan";
}

function actionHandoffHref(projectId: string, action: SEOContentAction | ResultsAction) {
  return destinationForAction(action) === "Site Fixes" ? null : `/projects/${projectId}/plan?action=${action.id}`;
}

function actionHandoffLabel(action: SEOContentAction | ResultsAction) {
  const destination = destinationForAction(action);
  return destination === "Site Fixes" ? "View in Site Fixes" : "View in Content Plan";
}

function actionHandoffStatus(action: SEOContentAction | ResultsAction) {
  return destinationForAction(action) === "Site Fixes" ? "Sent to Site Fixes" : "Sent to Content Plan";
}

const resultLoopStages = new Set<VisibilityLifecycleStage>(["published_or_applied", "measuring", "learned"]);

function loopActionCurrentSurface(action: LoopAction) {
  const stage = deriveVisibilityLifecycleStage(action);
  if (resultLoopStages.has(stage)) return "Results";
  if (stage === "blocked" && hasResultsExecutionEvidence(action)) return "Results";
  if (stage === "ready_for_review" && action.draft_article_id) return "Review";
  if (destinationForAction(action) === "Site Fixes") return "Site Fixes";
  return "Content Plan";
}

function loopActionCurrentHref(projectId: string, action: LoopAction) {
  const surface = loopActionCurrentSurface(action);
  if (surface === "Review") return `/projects/${projectId}/review?article=${action.draft_article_id}`;
  if (surface === "Results") return `/projects/${projectId}/results?action=${action.id}`;
  if (surface === "Site Fixes") return `/projects/${projectId}/site-fixes`;
  return actionHandoffHref(projectId, action) ?? `/projects/${projectId}/plan?action=${action.id}`;
}

function loopActionCurrentLabel(action: LoopAction) {
  return loopActionCurrentSurface(action);
}

const inactiveLoopActionStatuses = new Set(["archived", "dismissed"]);
const inactiveLoopOpportunityStatuses = new Set(["archived", "dismissed"]);

function normalizedLifecycleStatus(status?: string | null) {
  return String(status ?? "").trim().toLowerCase();
}

function hasDismissedSourceOpportunity(action: LoopAction) {
  return inactiveLoopOpportunityStatuses.has(normalizedLifecycleStatus(action.opportunity_status));
}

function isVisibleLoopAction(action: LoopAction) {
  if (inactiveLoopActionStatuses.has(normalizedLifecycleStatus(action.status))) return false;
  if (hasDismissedSourceOpportunity(action) && !hasResultsExecutionEvidence(action)) return false;
  return true;
}

const activeHandoffStages = new Set(["added_to_plan", "planned", "drafting", "ready_for_review"]);

function isRecentlySentAction(action: SEOContentAction | ResultsAction) {
  if (["published", "measuring", "completed", "archived", "dismissed"].includes(action.status)) return false;
  if (!action.opportunity_id) return false;
  // Exit is event-driven only (PRD-CiteLoop-Workflow-Handoff-Link-Cards §2.2):
  // the card leaves when the downstream item advances past the handoff stages,
  // never on a time window or count cap.
  return activeHandoffStages.has(deriveVisibilityLifecycleStage(action));
}

function opportunityPriorityLabel(opportunity: SEOOpportunity) {
  const score = normalizeNumeric(opportunity.priority_score);
  if (score == null) return "Review priority";
  if (score >= 75) return "High priority";
  if (score >= 40) return "Medium priority";
  return "Low priority";
}

function toneForOpportunityPriority(opportunity: SEOOpportunity): "green" | "amber" | "red" | "neutral" {
  const score = normalizeNumeric(opportunity.priority_score);
  if (score == null) return "neutral";
  if (score >= 75) return "red";
  if (score >= 40) return "amber";
  return "neutral";
}

function sourceModeForOpportunity(opportunity: SEOOpportunity, overview: SEOOverview | null) {
  const text = `${opportunity.type} ${opportunity.evidence ? JSON.stringify(opportunity.evidence) : ""}`.toLowerCase();
  if (text.includes("geo")) return "GEO";
  if (overview?.capability_mode === "customer_site_connected" || overview?.capability_mode === "managed_content_connected") return "GSC";
  return "Public crawl";
}

function compactEvidenceText(evidence: any) {
  if (!evidence) return "No structured evidence yet.";
  if (typeof evidence === "string") return evidence;
  if (Array.isArray(evidence)) return evidence.slice(0, 3).map(String).join(" / ");
  if (typeof evidence === "object") {
    return Object.entries(evidence)
      .slice(0, 5)
      .map(([key, value]) => `${key}: ${typeof value === "object" ? JSON.stringify(value) : String(value)}`)
      .join(" / ");
  }
  return String(evidence);
}


type ActionMeasurementKey = "waiting" | "positive" | "negative" | "mixed" | "inconclusive" | "insufficient_data";
type ActionMeasurementState = {
  key: ActionMeasurementKey;
  label: "Waiting" | "Positive" | "Negative" | "Mixed" | "Inconclusive" | "Insufficient data";
  tone: "green" | "amber" | "red" | "neutral";
  detail: string;
};
type MeasurementQueueKey = "waiting" | "too_early" | "blocked" | "completed";
type MeasurementQueueState = {
  key: MeasurementQueueKey;
  label: "Waiting" | "Too early" | "Blocked" | "Completed";
  tone: "green" | "amber" | "red" | "neutral";
  detail: string;
};

function latestActionMeasurement(action: SEOContentAction | ResultsAction): ActionMeasurement | null {
  const resultAction = action as ResultsAction;
  return resultAction.latest_measurement ?? resultAction.measurements?.[0] ?? null;
}

function actionOutcomeReason(action: SEOContentAction | ResultsAction, fallback: string) {
  const measurement = latestActionMeasurement(action);
  return measurement?.outcome_reason || action.outcome_summary?.outcome_reason || fallback;
}

function actionAttributionConfidence(action: SEOContentAction | ResultsAction) {
  const measurement = latestActionMeasurement(action);
  return measurement?.attribution_confidence || action.outcome_summary?.attribution_confidence || "none";
}

function actionConfounders(action: SEOContentAction | ResultsAction) {
  const measurement = latestActionMeasurement(action);
  const raw = measurement?.confounders ?? action.outcome_summary?.confounders ?? [];
  if (Array.isArray(raw)) return raw.map(String).filter(Boolean);
  if (typeof raw === "string" && raw.trim()) return [raw.trim()];
  return [];
}

function measurementMetricText(measurement: ActionMeasurement | null, side: "before" | "after") {
  if (!measurement) return "-";
  const sources = [measurement.seo_metrics, measurement.ga4_metrics, measurement.geo_metrics, measurement.execution_metrics];
  for (const source of sources) {
    if (!source || typeof source !== "object") continue;
    const value = source[side] ?? source[`${side}_value`] ?? source[`${side}_metrics`];
    if (value == null || value === "") continue;
    if (typeof value === "number") return metric(value, 2);
    if (typeof value === "string") return value;
    return compactOutcomeText(value);
  }
  return "-";
}

function actionMeasurementState(action: SEOContentAction | ResultsAction): ActionMeasurementState {
  const measurement = latestActionMeasurement(action);
  const rawResult = String(
    measurement?.outcome_label ?? action.outcome_summary?.outcome_label ?? action.outcome_summary?.result ?? action.outcome_summary?.state ?? "",
  ).toLowerCase();
  const hasMeasurementSignal =
    Boolean(measurement) ||
    ["published", "measuring", "completed", "failed", "verification_failed", "recovery_required"].includes(action.status) ||
    Boolean(action.published_at || action.verified_at || hasActionVerificationSnapshot(action));
  if (rawResult === "insufficient_data") {
    return {
      key: "insufficient_data",
      label: "Insufficient data",
      tone: "amber",
      detail: actionOutcomeReason(action, "The checkpoint ran, but there is not enough before/after data for attribution yet."),
    };
  }
  if (["improved", "positive", "won", "up"].includes(rawResult)) {
    return { key: "positive", label: "Positive", tone: "green", detail: actionOutcomeReason(action, "Measured signals improved after publishing.") };
  }
  if (["worsened", "negative", "lost", "down"].includes(rawResult) || ["failed", "verification_failed", "recovery_required"].includes(action.status)) {
    return { key: "negative", label: "Negative", tone: "red", detail: actionOutcomeReason(action, "The result needs follow-up before it can be treated as a win.") };
  }
  if (["mixed", "partial", "split"].includes(rawResult)) {
    return { key: "mixed", label: "Mixed", tone: "amber", detail: actionOutcomeReason(action, "Some measured signals moved in different directions.") };
  }
  if (["inconclusive", "neutral", "flat"].includes(rawResult) || action.status === "completed") {
    return { key: "inconclusive", label: "Inconclusive", tone: "amber", detail: actionOutcomeReason(action, "The measurement window closed without a clear positive or negative signal.") };
  }
  if (!hasMeasurementSignal) {
    return { key: "waiting", label: "Waiting", tone: "neutral", detail: "Action is waiting for publish or URL verification before measurement starts." };
  }
  return { key: "waiting", label: "Waiting", tone: "neutral", detail: "Published work is still inside the measurement window." };
}

function measurementQueueState(action: SEOContentAction | ResultsAction): MeasurementQueueState {
  const measurement = latestActionMeasurement(action);
  const rawResult = String(
    measurement?.outcome_label ?? action.outcome_summary?.outcome_label ?? action.outcome_summary?.result ?? action.outcome_summary?.state ?? "",
  ).toLowerCase();
  if (["failed", "verification_failed", "recovery_required"].includes(action.status)) {
    return { key: "blocked", label: "Blocked", tone: "red", detail: "Measurement is blocked by execution, verification, or recovery work." };
  }
  if (measurement || rawResult || action.status === "completed") {
    return { key: "completed", label: "Completed", tone: "green", detail: "At least one measurement checkpoint has been computed." };
  }
  if (["published", "measuring"].includes(action.status) || Boolean(action.published_at || action.verified_at)) {
    return { key: "too_early", label: "Too early", tone: "amber", detail: "Published or applied work is still waiting for its first due checkpoint." };
  }
  return { key: "waiting", label: "Waiting", tone: "neutral", detail: "Action is waiting for publish or URL verification before measurement starts." };
}


function loopLifecycleSummaryLabel(stage: string) {
  return lifecycleStageLabel(stage);
}

function loopActionDestinationLabel(action: LoopAction) {
  return loopActionCurrentSurface(action);
}


function loopActionTitle(action: LoopAction) {
  return action.topic_title || action.opportunity_recommended_action || action.opportunity_query || action.action_type || "Visibility action";
}

function cleanResultString(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function resultPublishedArticleTitle(action: SEOContentAction | ResultsAction | LoopAction) {
  return (
    cleanResultString((action as ResultsAction | LoopAction).topic_title) ||
    cleanResultString(action.output_snapshot?.title) ||
    cleanResultString(action.input_snapshot?.title) ||
    cleanResultString(action.action_type) ||
    "Published work"
  );
}

function resultPublishedArticleUrl(action: SEOContentAction | ResultsAction | LoopAction) {
  return cleanResultString((action as ResultsAction | LoopAction).draft_article_canonical_url);
}

function resultSourceEvidenceUrl(action: SEOContentAction | ResultsAction | LoopAction) {
  const sourceURL = cleanResultString(action.target_url) || cleanResultString(action.normalized_target_url);
  const publishedURL = resultPublishedArticleUrl(action);
  return sourceURL && sourceURL !== publishedURL ? sourceURL : "";
}


function GSCStatusMenu({
  projectId,
  overview,
  gscConnection,
  status,
  gscStatus,
  busy,
  onConnect,
}: {
  projectId: string;
  overview: SEOOverview | null;
  gscConnection: GSCConnection | null;
  status: ReturnType<typeof analysisSearchDataStatus>;
  gscStatus: string;
  busy: string | null;
  onConnect: () => void;
}) {
  const compact = compactGSCStatus(status);
  const propertyLabel = gscConnection?.selected_property ?? overview?.property?.gsc_site_url ?? overview?.property?.site_url ?? "Select property";
  const dataMode = searchDataModeLabel(overview, status);
  const settingsHref = `/projects/${projectId}/settings#search-console`;
  const [gscMenuOpen, setGSCMenuOpen] = useState(false);
  const gscMenuRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!gscMenuOpen) return;

    const onPointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Node)) return;
      if (gscMenuRef.current?.contains(target)) return;
      setGSCMenuOpen(false);
    };

    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [gscMenuOpen]);

  return (
    <div ref={gscMenuRef} className="relative">
      <button
        type="button"
        aria-expanded={gscMenuOpen}
        onClick={() => setGSCMenuOpen((open) => !open)}
        className="flex h-8 cursor-pointer list-none items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 shadow-sm transition hover:border-slate-300"
      >
        <span className={`h-2 w-2 rounded-full ${compact.dot}`} aria-hidden="true" />
        {compact.label}
      </button>
      {gscMenuOpen && (
        <div className="absolute right-0 z-20 mt-2 w-[320px] max-w-[calc(100vw-2rem)] rounded-xl border border-slate-200 bg-white p-4 text-sm shadow-lg">
          <div className="flex items-start justify-between gap-3">
            <div>
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Search Console details</div>
              <div className="mt-1 font-bold text-slate-950">{status.label}</div>
            </div>
            <Badge tone={compact.tone}>{compact.label}</Badge>
          </div>
          <div className="mt-3 space-y-2 text-xs leading-5 text-slate-600">
            <div className="flex justify-between gap-3 border-t border-slate-100 pt-2">
              <span className="text-slate-400">Search Console property</span>
              <span className="max-w-[190px] truncate font-semibold text-slate-700">{propertyLabel}</span>
            </div>
            <div className="flex justify-between gap-3">
              <span className="text-slate-400">Data mode</span>
              <span className="font-semibold text-slate-700">{dataMode}</span>
            </div>
            <div className="flex justify-between gap-3">
              <span className="text-slate-400">Integration</span>
              <span className="font-semibold text-slate-700">{gscStatus}</span>
            </div>
          </div>
          <p className="mt-3 text-xs leading-5 text-slate-500">{status.detail}</p>
          <div className="mt-4 flex flex-wrap gap-2">
            {status.action && (
              <Button size="sm" variant="primary" onClick={onConnect} disabled={!!busy}>
                <ButtonProgress busy={busy === "gsc-oauth"} busyLabel="Opening Google" idleIcon={<Search size={14} />}>
                  {status.action}
                </ButtonProgress>
              </Button>
            )}
            <Link
              href={settingsHref}
              className="inline-flex h-8 items-center justify-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 transition hover:bg-slate-50"
            >
              Manage in Settings
            </Link>
          </div>
        </div>
      )}
    </div>
  );
}

function GA4StatusLink({
  projectId,
  status,
  propertyID,
}: {
  projectId: string;
  status?: string;
  propertyID?: string | null;
}) {
  const compact = compactGA4Status(status, propertyID);

  return (
    <Link
      href={`/projects/${projectId}/settings#search-console`}
      aria-label={`${compact.label}. ${compact.detail}`}
      title={compact.detail}
      className="flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 shadow-sm transition hover:border-slate-300"
    >
      <span className={`h-2 w-2 rounded-full ${compact.dot}`} aria-hidden="true" />
      {compact.label}
    </Link>
  );
}

function formatOpportunityFindingDuration(ms?: number) {
  const value = Number(ms ?? 0);
  if (!Number.isFinite(value) || value <= 0) return "Not recorded";
  if (value < 1000) return `${Math.round(value)} ms`;
  const seconds = Math.round(value / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return remainder ? `${minutes}m ${remainder}s` : `${minutes}m`;
}

function opportunityFindingModeLabel(status: OpportunityFindingStatus | null) {
  if (!status) return "Loading";
  if (status.source_mix === "signal_scan") return "Signal Scan";
  if (status.source_mix === "ai_discovery") return "AI Discovery";
  return "All";
}

function OpportunityFindingStatusPanel({
  status,
  busy,
  projectId,
  onRun,
}: {
  status: OpportunityFindingStatus | null;
  busy: string | null;
  projectId: string;
  onRun: () => void;
}) {
  const manualMode = Boolean(status?.manual_mode);
  const panelClass = manualMode
    ? "border-amber-200 bg-amber-50"
    : status
      ? "border-emerald-200 bg-emerald-50"
      : "border-slate-200 bg-slate-50";
  const lastRunLabel = status?.last_run?.started_at ? formatDate(status.last_run.started_at) : "Not run yet";
  const nextRunLabel = status?.next_finding_at ? formatDate(status.next_finding_at) : manualMode ? "Manual mode" : "Not scheduled";
  const durationLabel = formatOpportunityFindingDuration(status?.last_run?.duration_ms);
  const summary = status?.summary?.length
    ? status.summary
    : [{ label: "Signal Scan", detail: "Waiting for the first Opportunity Finding run.", tone: "neutral" }];
  const automationLabel =
    status?.ai_discovery_automation === "automatic"
      ? "Automatic"
      : status?.ai_discovery_automation === "manual"
        ? "Manual"
        : "Semi-automatic";

  return (
    <section data-analysis-opportunity-finding-status className={cx("rounded-xl border px-4 py-4 shadow-sm", panelClass)}>
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <div className="text-sm font-bold text-slate-950">Opportunity Finding</div>
            <Badge tone={manualMode ? "amber" : "green"}>{manualMode ? "Manual mode" : automationLabel}</Badge>
            <Badge tone="neutral">{opportunityFindingModeLabel(status)}</Badge>
          </div>
          <div className="mt-3 grid gap-3 text-sm sm:grid-cols-3">
            <div>
              <div className="text-xs font-bold uppercase text-slate-500">Last finding</div>
              <div className="mt-1 font-semibold text-slate-950">{lastRunLabel}</div>
            </div>
            <div>
              <div className="text-xs font-bold uppercase text-slate-500">Duration</div>
              <div className="mt-1 font-semibold text-slate-950">{durationLabel}</div>
            </div>
            <div>
              <div className="text-xs font-bold uppercase text-slate-500">Next finding</div>
              <div className="mt-1 font-semibold text-slate-950">{nextRunLabel}</div>
            </div>
          </div>
        </div>

        <div className="flex shrink-0 flex-wrap gap-2">
          <Button size="sm" variant="primary" onClick={onRun} disabled={!!busy}>
            <ButtonProgress busy={busy === "opportunity-finding"} busyLabel="Finding" idleIcon={<Search size={14} />}>
              Run finding
            </ButtonProgress>
          </Button>
          <Link
            href={`/projects/${projectId}/settings#opportunity-finding`}
            className="inline-flex h-8 items-center justify-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 transition hover:bg-slate-50"
          >
            Settings
          </Link>
        </div>
      </div>

      <div className="mt-4 grid gap-2 md:grid-cols-2 xl:grid-cols-5">
        {summary.slice(0, 5).map((item) => (
          <div key={`${item.label}-${item.detail}`} className="rounded-lg bg-white/75 px-3 py-2 ring-1 ring-white/80">
            <div className="text-xs font-bold uppercase text-slate-500">{item.label}</div>
            <div className="mt-1 text-sm font-semibold leading-5 text-slate-800">{item.detail}</div>
          </div>
        ))}
      </div>

      {status && (
        <div className="mt-3 flex flex-wrap gap-2 text-xs font-semibold text-slate-600">
          <span>{status.counts.open} open</span>
          <span>{status.counts.in_loop} in loop</span>
          <span>{status.counts.processed} already handled</span>
        </div>
      )}
    </section>
  );
}

type SEOClientMode = "analysis" | "results";

export function AnalysisClient({ projectId }: { projectId: string }) {
  return <SEOClient projectId={projectId} mode="analysis" />;
}

export function ResultsClient({ projectId }: { projectId: string }) {
  return <SEOClient projectId={projectId} mode="results" />;
}

export function SEOClient({ projectId, mode = "analysis" }: { projectId: string; mode?: SEOClientMode }) {
  const api = useApi();
  const router = useRouter();
  const searchParams = useSearchParams();
  const [overview, setOverview] = useState<SEOOverview | null>(null);
  const [gscConnection, setGSCConnection] = useState<GSCConnection | null>(null);
  const [visibilitySummary, setVisibilitySummary] = useState<VisibilitySummary | null>(null);
  const [opportunityFindingStatus, setOpportunityFindingStatus] = useState<OpportunityFindingStatus | null>(null);
  const [brief, setBrief] = useState<SEOBrief | null>(null);
  const [opportunities, setOpportunities] = useState<SEOOpportunity[]>([]);
  const [actions, setActions] = useState<SEOContentAction[]>([]);
  const [resultsActions, setResultsActions] = useState<ResultsAction[]>([]);
  const [policy, setPolicy] = useState<SEOPolicy | null>(null);
  const [readiness, setReadiness] = useState<AutopilotReadiness | null>(null);
  const [executionResult, setExecutionResult] = useState<AutopilotExecuteResult | null>(null);
  const [objectives, setObjectives] = useState<SEOObjective[]>([]);
  const [plans, setPlans] = useState<SEOActionPlan[]>([]);
  const [safeModes, setSafeModes] = useState<SafeModeEvent[]>([]);
  const [crawlerSnapshots, setCrawlerSnapshots] = useState<AICrawlerAccessSnapshot[]>([]);
  const [geoOverview, setGeoOverview] = useState<GEOOverview | null>(null);
  const [assetBriefs, setAssetBriefs] = useState<GEOAssetBrief[]>([]);
  const [publisherConnections, setPublisherConnections] = useState<PublisherConnection[]>([]);
  const [seoProperty, setSEOProperty] = useState<SEOProperty | null>(null);
  const [seoIntegrations, setSEOIntegrations] = useState<SEOIntegration[]>([]);
  const [siteURL, setSiteURL] = useState("");
  const [surfaceURL, setSurfaceURL] = useState("");
  const [surfaceSourceURL, setSurfaceSourceURL] = useState("");
  const [surfaceOwnerType, setSurfaceOwnerType] = useState("managed_external");
  const [surfacePlatform, setSurfacePlatform] = useState("devto");
  const [surfacePublicationStatus, setSurfacePublicationStatus] = useState("draft");
  const [surfaceIndexabilityStatus, setSurfaceIndexabilityStatus] = useState("unknown");
  const [surfaceCanonicalStatus, setSurfaceCanonicalStatus] = useState("unknown");
  const [surfaceOwnerConfidence, setSurfaceOwnerConfidence] = useState("medium");
  const [objectiveName, setObjectiveName] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [opportunityBusy, setOpportunityBusy] = useState<Record<string, "create" | "dismiss" | "snooze" | "watch">>({});
  const [routeOverrides, setRouteOverrides] = useState<Record<string, OpportunityWorkType>>({});
  const [watchlist, setWatchlist] = useState<SEOWatchlistItem[]>([]);
  const [selectedOpportunityID, setSelectedOpportunityID] = useState<string | null>(null);
  const [analysisRecentDrawer, setAnalysisRecentDrawer] = useState<"opportunities" | "site-fixes" | null>(null);
  const [selectedResultActionID, setSelectedResultActionID] = useState<string | null>(null);
  const [highlightedResultActionID, setHighlightedResultActionID] = useState<string | null>(null);
  const [selectedLoopStage, setSelectedLoopStage] = useState<VisibilityLifecycleStage | null>(null);
  const analysisSurfaceRef = useRef<HTMLDivElement | null>(null);
  const refreshSequenceRef = useRef(0);
  const analysisDrawerRef = useRef<HTMLElement | null>(null);
  const analysisReturnFocusRef = useRef<HTMLElement | null>(null);
  const resultsSurfaceRef = useRef<HTMLDivElement | null>(null);
  const resultDrawerRef = useRef<HTMLElement | null>(null);
  const resultReturnFocusRef = useRef<HTMLElement | null>(null);
  const consumedResultHandoffRef = useRef<string | null>(null);
  const resultHandoffTimersRef = useRef<number[]>([]);
  const selectedOpportunity = useMemo(
    () => opportunities.find((opp) => opp.id === selectedOpportunityID) ?? null,
    [opportunities, selectedOpportunityID],
  );
  const { notify } = useToast();
  const setMessage = (next: Message) => {
    if (next) notify(next);
  };

  const refresh = useCallback(async () => {
    const refreshSequence = refreshSequenceRef.current + 1;
    refreshSequenceRef.current = refreshSequence;
    setMessage(null);
    try {
      const [
        overviewResult,
        summaryResult,
        findingStatusResult,
        settingsResult,
        gscConnectionResult,
        briefResult,
        oppsResult,
        snoozedOppsResult,
        watchingOppsResult,
        watchlistRowsResult,
        actionRowsResult,
        resultsRowsResult,
        policyResult,
        readinessResult,
        objectiveRowsResult,
        planRowsResult,
        safeModeRowsResult,
        crawlerAuditResult,
        geoResult,
        briefRowsResult,
        publisherRowsResult,
      ] = await Promise.allSettled([
        api.getSEOOverview(projectId),
        api.getVisibilitySummary(projectId),
        api.getOpportunityFindingStatus(projectId),
        api.getSEOSettings(projectId),
        api.getGSCConnection(projectId),
        api.getSEOBrief(projectId),
        api.listSEOOpportunities(projectId, { status: "open", limit: 50 }),
        api.listSEOOpportunities(projectId, { status: "snoozed", limit: 20 }),
        api.listSEOOpportunities(projectId, { status: "watching", limit: 20 }),
        api.listSEOWatchlist(projectId, { limit: 50 }),
        api.listSEOContentActions(projectId, { limit: 50 }),
        api.listResultsActions(projectId, { limit: 50 }),
        api.getSEOPolicy(projectId),
        api.getAutopilotReadiness(projectId),
        api.listSEOObjectives(projectId),
        api.listAutopilotPlans(projectId),
        api.listSafeModeEvents(projectId),
        api.getLatestGEOCrawlerAudit(projectId),
        api.getGEOOverview(projectId),
        api.listGEOAssetBriefs(projectId, { limit: 50 }),
        api.listPublisherConnections(projectId),
      ]);
      if (refreshSequence !== refreshSequenceRef.current) return;

      const overviewData = settledValue(overviewResult);
      const summaryData = settledValue(summaryResult);
      const findingStatus = settledValue(findingStatusResult);
      const settings = settledValue(settingsResult);
      const gscConnectionData = settledValue(gscConnectionResult);
      const briefData = settledValue(briefResult);
      const opps = settledValue(oppsResult);
      const snoozedOpps = settledValue(snoozedOppsResult);
      const watchingOpps = settledValue(watchingOppsResult);
      const watchlistRows = settledValue(watchlistRowsResult);
      const actionRows = settledValue(actionRowsResult);
      const resultsRows = settledValue(resultsRowsResult);
      const policyData = settledValue(policyResult);
      const readinessData = settledValue(readinessResult);
      const objectiveRows = settledValue(objectiveRowsResult);
      const planRows = settledValue(planRowsResult);
      const safeModeRows = settledValue(safeModeRowsResult);
      const crawlerAudit = settledValue(crawlerAuditResult);
      const geoData = settledValue(geoResult);
      const briefRows = settledValue(briefRowsResult);
      const publisherRows = settledValue(publisherRowsResult);

      if (overviewData) setOverview(overviewData);
      if (summaryData) setVisibilitySummary(summaryData);
      if (findingStatus) setOpportunityFindingStatus(findingStatus);
      if (gscConnectionData) setGSCConnection(gscConnectionData);
      if (briefData) setBrief(briefData);
      if (opps && snoozedOpps && watchingOpps) setOpportunities([...opps, ...snoozedOpps, ...watchingOpps]);
      if (watchlistRows) setWatchlist(watchlistRows);
      if (actionRows) setActions(actionRows);
      if (resultsRows) setResultsActions(resultsRows);
      if (policyData) setPolicy(policyData);
      if (readinessData) setReadiness(readinessData);
      if (objectiveRows) setObjectives(objectiveRows);
      if (planRows) setPlans(planRows);
      if (safeModeRows) setSafeModes(safeModeRows);
      if (crawlerAudit) setCrawlerSnapshots(crawlerAudit.snapshots);
      if (geoData) setGeoOverview(geoData);
      if (briefRows) setAssetBriefs(briefRows);
      if (publisherRows) setPublisherConnections(publisherRows);
      if (settings || overviewData) {
        const nextProperty = settings?.property ?? overviewData?.property ?? null;
        setSEOProperty(nextProperty);
        setSEOIntegrations(settings?.integrations ?? overviewData?.integrations ?? []);
        setSiteURL(nextProperty?.site_url ?? "");
      }

      if (overviewResult.status === "rejected" && gscConnectionResult.status === "rejected" && summaryResult.status === "rejected") {
        const reason = overviewResult.reason instanceof Error ? overviewResult.reason.message : "CiteLoop API request failed";
        setMessage({ title: "SEO data unavailable", detail: reason, tone: "red" });
      }
    } catch (e: any) {
      if (refreshSequence !== refreshSequenceRef.current) return;
      setMessage({ title: "SEO data unavailable", detail: e.message, tone: "red" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    if (!selectedOpportunityID || selectedOpportunity) return;
    setSelectedOpportunityID(null);
  }, [selectedOpportunity, selectedOpportunityID]);

  useEffect(() => {
    if (!selectedOpportunity?.id) return;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setSelectedOpportunityID(null);
      if (event.key === "Tab") {
        const drawer = analysisDrawerRef.current;
        if (!drawer) return;
        const focusable = Array.from(drawer.querySelectorAll<HTMLElement>(drawerFocusableSelector)).filter(
          (element) => !element.hasAttribute("disabled") && element.getAttribute("aria-hidden") !== "true",
        );
        if (focusable.length === 0) {
          event.preventDefault();
          return;
        }
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    };

    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [selectedOpportunity?.id]);

  useEffect(() => {
    if (!selectedOpportunity?.id) return;

    const previousBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const closeButton = analysisDrawerRef.current?.querySelector<HTMLElement>("[data-drawer-close]");
    const firstFocusable = closeButton ?? analysisDrawerRef.current?.querySelector<HTMLElement>(drawerFocusableSelector);
    firstFocusable?.focus();
    if (analysisSurfaceRef.current) {
      analysisSurfaceRef.current.setAttribute("aria-hidden", "true");
      analysisSurfaceRef.current.inert = true;
    }
    return () => {
      document.body.style.overflow = previousBodyOverflow;
      if (analysisSurfaceRef.current) {
        analysisSurfaceRef.current.removeAttribute("aria-hidden");
        analysisSurfaceRef.current.inert = false;
      }
      if (analysisReturnFocusRef.current?.isConnected) {
        analysisReturnFocusRef.current?.focus();
      }
    };
  }, [selectedOpportunity?.id]);

  const gscStatus = useMemo(() => {
    return gscConnection?.status ?? overview?.integrations.find((integration) => integration.provider === "google_search_console")?.status ?? "missing";
  }, [gscConnection?.status, overview]);

  const hasConnectedGitHubPublisher = useMemo(
    () =>
      publisherConnections.some(
        (connection) =>
          connection.kind === "github_nextjs" &&
          connection.enabled &&
          connection.status === "connected",
      ),
    [publisherConnections],
  );

  const promptCountBySet = useMemo(() => {
    const counts = new Map<string, number>();
    for (const prompt of geoOverview?.prompts ?? []) {
      counts.set(prompt.prompt_set_id, (counts.get(prompt.prompt_set_id) ?? 0) + 1);
    }
    return counts;
  }, [geoOverview]);

  const geoCoverage = normalizeNumeric(geoOverview?.score?.coverage);
  const geoScoreValue = normalizeNumeric(geoOverview?.score?.score);
  const showGeoScore = Boolean(geoOverview?.score && geoCoverage != null && geoCoverage >= 0.3 && geoOverview.score.confidence !== "insufficient_data");
  const analysisStatus = analysisSearchDataStatus(overview, gscStatus);
  const ga4Integration = seoIntegrations.find((integration) => integration.provider === "google_analytics");
  const ga4PropertyID = seoProperty?.ga4_property_id ?? overview?.property?.ga4_property_id ?? null;
  const crawlerOkCount = crawlerSnapshots.filter((snapshot) => snapshot.access_state === "ok").length;
  const latestPortfolioPlan = plans[0] ?? null;
  const readinessGates = readiness?.gates ?? [];
  const blockedReadinessGates = readinessGates.filter((gate) => gate.blocking);
  const readinessTone = readiness?.ready_for_level_2 ? "green" : readiness ? "amber" : "neutral";
  const latestRecoveryPlans = executionResult?.recovery_plans ?? [];
  const actionsByID = new Map(actions.map((action) => [action.id, action]));
  const summaryLoopActions = (visibilitySummary?.actions_in_loop ?? []).map((summaryAction) => {
    const matchingAction = actionsByID.get(summaryAction.id);
    return matchingAction ? { ...summaryAction, ...matchingAction } : summaryAction;
  });
  const summaryLoopActionIds = new Set(summaryLoopActions.map((action) => action.id));
  const loopActions = [
    ...summaryLoopActions,
    ...actions
      .filter((action) => !summaryLoopActionIds.has(action.id))
      .map((action) => ({
        ...action,
        lifecycle_stage: deriveVisibilityLifecycleStage(action),
        opportunity_page_url: action.target_url ?? action.normalized_target_url ?? null,
        opportunity_normalized_page_url: action.normalized_target_url ?? null,
        opportunity_query: null,
        opportunity_recommended_action: action.action_type,
      })),
  ];
  const measuredActions = loopActions.filter((action) => !["archived", "dismissed"].includes(action.status) && hasResultsExecutionEvidence(action));
  const resultActions = loopActions.filter((action) => !["archived", "dismissed"].includes(action.status) && hasResultsExecutionEvidence(action));
  const attributionActions = resultsActions.length
    ? resultsActions.filter((action) => !["archived", "dismissed"].includes(action.status) && hasResultsExecutionEvidence(action))
    : resultActions;
  const selectedResultAction = useMemo(
    () => attributionActions.find((action) => action.id === selectedResultActionID) ?? null,
    [attributionActions, selectedResultActionID],
  );
  const requestedResultActionID = mode === "results" ? searchParams.get("action") : null;
  const requestedResultArticleID = mode === "results" ? searchParams.get("article") : null;
  const requestedWatchOpportunityID = mode === "results" ? searchParams.get("watch") : null;

  const clearResultHandoffTimers = useCallback(() => {
    resultHandoffTimersRef.current.forEach((timer) => window.clearTimeout(timer));
    resultHandoffTimersRef.current = [];
  }, []);

  const closeResultDrawer = useCallback(() => {
    clearResultHandoffTimers();
    setHighlightedResultActionID(null);
    setSelectedResultActionID(null);
    if (mode === "results" && (requestedResultActionID || requestedResultArticleID)) {
      router.replace(`/projects/${projectId}/results`, { scroll: false });
    }
  }, [clearResultHandoffTimers, mode, projectId, requestedResultActionID, requestedResultArticleID, router]);

  const focusResultActionForHandoff = useCallback(
    (actionID: string) => {
      clearResultHandoffTimers();
      setSelectedResultActionID(null);
      setHighlightedResultActionID(actionID);

      const focusTimer = window.setTimeout(() => {
        const target = document.querySelector<HTMLElement>(`[data-results-action-card="${actionID}"]`);
        if (!target) return;
        const prefersReducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches ?? false;
        target.scrollIntoView({ behavior: prefersReducedMotion ? "auto" : "smooth", block: "center" });
        target.focus({ preventScroll: true });
        resultReturnFocusRef.current = target;
      }, 120);
      const openTimer = window.setTimeout(() => setSelectedResultActionID(actionID), 900);
      const clearTimer = window.setTimeout(() => setHighlightedResultActionID(null), 2_350);
      resultHandoffTimersRef.current = [focusTimer, openTimer, clearTimer];
    },
    [clearResultHandoffTimers],
  );

  useEffect(() => clearResultHandoffTimers, [clearResultHandoffTimers]);

  useEffect(() => {
    if (mode !== "results" || !requestedWatchOpportunityID || watchlist.length === 0) return;
    const target = document.getElementById(`watchlist-item-${requestedWatchOpportunityID}`);
    if (!target) return;
    const prefersReducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches ?? false;
    target.scrollIntoView({ behavior: prefersReducedMotion ? "auto" : "smooth", block: "center" });
    target.focus({ preventScroll: true });
  }, [mode, requestedWatchOpportunityID, watchlist.length]);
  const attributionMeasuredActions = resultsActions.length
    ? resultsActions.filter((action) => !["archived", "dismissed"].includes(action.status) && hasResultsExecutionEvidence(action))
    : measuredActions;
  const outcomeCounts = attributionMeasuredActions.reduce(
    (counts, action) => {
      counts[actionMeasurementState(action).key] += 1;
      return counts;
    },
    { waiting: 0, positive: 0, negative: 0, mixed: 0, inconclusive: 0, insufficient_data: 0 },
  );
  const measurementQueueCounts = attributionActions.reduce(
    (counts, action) => {
      counts[measurementQueueState(action).key] += 1;
      return counts;
    },
    { waiting: 0, too_early: 0, blocked: 0, completed: 0 },
  );
  const measurementExceptions = attributionMeasuredActions.filter((action) => ["negative", "mixed", "inconclusive", "insufficient_data"].includes(actionMeasurementState(action).key));

  useEffect(() => {
    if (mode !== "results" || !requestedResultActionID || attributionActions.length === 0) return;
    const handoffKey = `action:${requestedResultActionID}`;
    if (consumedResultHandoffRef.current === handoffKey) return;
    if (attributionActions.some((action) => action.id === requestedResultActionID)) {
      consumedResultHandoffRef.current = handoffKey;
      focusResultActionForHandoff(requestedResultActionID);
      router.replace(`/projects/${projectId}/results`, { scroll: false });
    }
  }, [attributionActions, focusResultActionForHandoff, mode, projectId, requestedResultActionID, router]);

  // Publish handoff links land here with ?article=; open the measurement item
  // that belongs to the published draft.
  useEffect(() => {
    if (mode !== "results" || !requestedResultArticleID || attributionActions.length === 0) return;
    const handoffKey = `article:${requestedResultArticleID}`;
    if (consumedResultHandoffRef.current === handoffKey) return;
    const match = attributionActions.find((action) => action.draft_article_id === requestedResultArticleID);
    if (match) {
      consumedResultHandoffRef.current = handoffKey;
      focusResultActionForHandoff(match.id);
      router.replace(`/projects/${projectId}/results`, { scroll: false });
    }
  }, [attributionActions, focusResultActionForHandoff, mode, projectId, requestedResultArticleID, router]);

  useEffect(() => {
    if (mode !== "results" || requestedResultActionID || requestedResultArticleID) return;
    consumedResultHandoffRef.current = null;
  }, [mode, requestedResultActionID, requestedResultArticleID]);

  useEffect(() => {
    if (!selectedResultActionID || selectedResultAction) return;
    if (mode === "results" && selectedResultActionID === requestedResultActionID && attributionActions.length === 0) return;
    setSelectedResultActionID(null);
  }, [attributionActions.length, mode, requestedResultActionID, selectedResultAction, selectedResultActionID]);

  useEffect(() => {
    if (!selectedResultAction?.id) return;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeResultDrawer();
      if (event.key === "Tab") {
        const drawer = resultDrawerRef.current;
        if (!drawer) return;
        const focusable = Array.from(drawer.querySelectorAll<HTMLElement>(drawerFocusableSelector)).filter(
          (element) => !element.hasAttribute("disabled") && element.getAttribute("aria-hidden") !== "true",
        );
        if (focusable.length === 0) {
          event.preventDefault();
          return;
        }
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    };

    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [closeResultDrawer, selectedResultAction?.id]);

  useEffect(() => {
    if (!selectedResultAction?.id) return;

    const previousBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const closeButton = resultDrawerRef.current?.querySelector<HTMLElement>("[data-drawer-close]");
    const firstFocusable = closeButton ?? resultDrawerRef.current?.querySelector<HTMLElement>(drawerFocusableSelector);
    firstFocusable?.focus();
    if (resultsSurfaceRef.current) {
      resultsSurfaceRef.current.setAttribute("aria-hidden", "true");
      resultsSurfaceRef.current.inert = true;
    }
    return () => {
      document.body.style.overflow = previousBodyOverflow;
      if (resultsSurfaceRef.current) {
        resultsSurfaceRef.current.removeAttribute("aria-hidden");
        resultsSurfaceRef.current.inert = false;
      }
      if (resultReturnFocusRef.current?.isConnected) {
        resultReturnFocusRef.current?.focus();
      }
    };
  }, [selectedResultAction?.id]);

  const activeOpportunities = opportunities.filter((opportunity) => opportunity.status === "open");
  const summaryLifecycleCounts = visibilitySummary?.lifecycle_counts;
  const visibleLoopActions = loopActions.filter(isVisibleLoopAction);
  const loopLifecycleCounts = visibilityLifecycleCounts(visibleLoopActions);
  loopLifecycleCounts.detected = activeOpportunities.length || summaryLifecycleCounts?.detected || 0;
  const loopActiveCount =
    loopLifecycleCounts.added_to_plan +
    loopLifecycleCounts.planned +
    loopLifecycleCounts.drafting +
    loopLifecycleCounts.ready_for_review +
    loopLifecycleCounts.approved +
    loopLifecycleCounts.published_or_applied +
    loopLifecycleCounts.measuring;
  const loopSummaryItems: Array<{ key: VisibilityLifecycleStage; label: string; value: number }> = [
    { key: "added_to_plan", label: loopLifecycleSummaryLabel("added_to_plan"), value: loopLifecycleCounts.added_to_plan },
    { key: "planned", label: loopLifecycleSummaryLabel("planned"), value: loopLifecycleCounts.planned },
    { key: "drafting", label: loopLifecycleSummaryLabel("drafting"), value: loopLifecycleCounts.drafting },
    { key: "ready_for_review", label: loopLifecycleSummaryLabel("ready_for_review"), value: loopLifecycleCounts.ready_for_review },
    { key: "published_or_applied", label: loopLifecycleSummaryLabel("published_or_applied"), value: loopLifecycleCounts.published_or_applied },
    { key: "measuring", label: loopLifecycleSummaryLabel("measuring"), value: loopLifecycleCounts.measuring },
    { key: "learned", label: loopLifecycleSummaryLabel("learned"), value: loopLifecycleCounts.learned },
    { key: "blocked", label: loopLifecycleSummaryLabel("blocked"), value: loopLifecycleCounts.blocked },
  ];
  const selectedLoopActions = selectedLoopStage
    ? visibleLoopActions.filter((action) => deriveVisibilityLifecycleStage(action) === selectedLoopStage).slice(0, 6)
    : [];
  const selectedLoopSummaryItem = loopSummaryItems.find((item) => item.key === selectedLoopStage) ?? null;
  const loopStageDetailTitle = selectedLoopSummaryItem
    ? `${selectedLoopSummaryItem.value} ${selectedLoopSummaryItem.value === 1 ? "opportunity" : "opportunities"} in ${selectedLoopSummaryItem.label}`
    : "";

  useEffect(() => {
    if (!selectedLoopSummaryItem || selectedLoopSummaryItem.value > 0) return;
    setSelectedLoopStage(null);
  }, [selectedLoopSummaryItem]);

  const snoozedOpportunities = opportunities.filter((opportunity) => opportunity.status === "snoozed");
  const watchingOpportunityLinks = opportunities.filter((opportunity) => opportunity.status === "watching");
  const sentOpportunityLinks = visibleLoopActions
    .filter(isRecentlySentAction)
    .slice()
    .sort((a, b) => {
      const left = a.created_at ? new Date(a.created_at).getTime() : 0;
      const right = b.created_at ? new Date(b.created_at).getTime() : 0;
      return right - left;
    });
  const opportunityRecentCount = sentOpportunityLinks.length + watchingOpportunityLinks.length;
  function createActionBusy(opp: SEOOpportunity) {
    return opportunityBusy[opp.id] === "create";
  }

  function dismissBusy(opp: SEOOpportunity) {
    return opportunityBusy[opp.id] === "dismiss";
  }

  function setOpportunityPending(id: string, value: "create" | "dismiss" | "snooze" | "watch" | null) {
    setOpportunityBusy((current) => {
      const next = { ...current };
      if (value) {
        next[id] = value;
      } else {
        delete next[id];
      }
      return next;
    });
  }

  async function manualRefresh() {
    setBusy("refresh");
    setMessage(null);
    try {
      await refresh();
    } finally {
      setBusy(null);
    }
  }

  async function startSearchConsoleOAuth() {
    setBusy("gsc-oauth");
    setMessage(null);
    try {
      const result = await api.startGSCOAuth(projectId);
      window.location.assign(result.authorization_url);
    } catch (e: any) {
      setMessage({ title: "Could not connect Search Console", detail: e.message, tone: "red" });
      setBusy(null);
    }
  }

  async function saveSettings() {
    setBusy("settings");
    setMessage(null);
    try {
      await api.updateSEOSettings(projectId, {
        site_url: siteURL,
      });
      await refresh();
      setMessage({ title: "SEO settings saved", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not save SEO settings", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function runSync() {
    setBusy("sync");
    setMessage(null);
    try {
      const result = await api.syncSEO(projectId, siteURL);
      await refresh();
      setMessage({ title: "SEO sync complete", detail: `sync ${result.sync?.status}; analyze ${result.analyze?.status}`, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "SEO sync failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function runOpportunityFinding() {
    setBusy("opportunity-finding");
    setMessage(null);
    try {
      const result = await api.runOpportunityFinding(projectId);
      await refresh();
      setMessage({
        title: "Opportunity finding complete",
        detail: `${result.status.counts.open} open; ${result.status.counts.processed} already handled`,
        tone: "green",
      });
    } catch (e: any) {
      setMessage({ title: "Opportunity finding failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function runCrawlerAudit() {
    setBusy("geo-crawler");
    setMessage(null);
    try {
      const result = await api.runGEOCrawlerAudit(projectId);
      await refresh();
      setMessage({
        title: "GEO crawler audit complete",
        detail: `${result.checked_urls} URLs checked; ${result.created_blockers} blockers queued`,
        tone: "green",
      });
    } catch (e: any) {
      setMessage({ title: "GEO crawler audit failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function generatePromptSet() {
    setBusy("geo-prompts");
    setMessage(null);
    try {
      const result = await api.generateGEOPromptSet(projectId, { locale: "en-US", status: "active" });
      await refresh();
      setMessage({ title: "GEO prompt set generated", detail: `${result.prompts?.length ?? 0} prompts`, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not generate GEO prompts", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function addExternalSurface() {
    if (!surfaceURL.trim()) return;
    setBusy("geo-surface");
    setMessage(null);
    try {
      await api.createGEOExternalSurface(projectId, {
        url: surfaceURL.trim(),
        owner_type: surfaceOwnerType,
        platform: surfacePlatform,
        surface_type: "page",
        publication_status: surfacePublicationStatus,
        indexability_status: surfaceIndexabilityStatus,
        canonical_status: surfaceCanonicalStatus,
        owner_confidence: surfaceOwnerConfidence,
        source_url: surfaceSourceURL.trim() || undefined,
        backlink_state: "unknown",
        verification_snapshot: { source: "manual_inventory" },
      });
      setSurfaceURL("");
      setSurfaceSourceURL("");
      await refresh();
      setMessage({ title: "External surface saved", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not save external surface", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function analyzeGEOOpportunities() {
    setBusy("geo-analyze");
    setMessage(null);
    try {
      const result = await api.analyzeGEOOpportunities(projectId, { limit: 100 });
      await refresh();
      setMessage({
        title: "GEO analyzer complete",
        detail: `${result.opportunities?.length ?? 0} opportunities; ${result.asset_briefs?.length ?? 0} briefs`,
        tone: "green",
      });
    } catch (e: any) {
      setMessage({ title: "GEO analyzer failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function observeGEOProvider() {
    setBusy("geo-provider");
    setMessage(null);
    try {
      const result = await api.observeGEOProvider(projectId, { engine: "OpenAI", max_prompts: 10 });
      await refresh();
      const status = result.run?.status ?? "degraded";
      setMessage({
        title: status === "ok" ? "GEO provider observation complete" : "GEO provider observation degraded",
        detail: `${result.observations.length} observations; $${(result.cost_usd ?? 0).toFixed(2)} cost`,
        tone: status === "ok" ? "green" : "amber",
      });
    } catch (e: any) {
      setMessage({ title: "GEO provider observation failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function monitorExternalSurfaces() {
    setBusy("geo-surface-monitor");
    setMessage(null);
    try {
      const result = await api.monitorGEOExternalSurfaces(projectId, { limit: 25 });
      await refresh();
      setMessage({
        title: result.failed > 0 ? "External surface monitor degraded" : "External surfaces monitored",
        detail: `${result.checked} checked; ${result.failed} failed`,
        tone: result.failed > 0 ? "amber" : "green",
      });
    } catch (e: any) {
      setMessage({ title: "External surface monitor failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function updatePromptStatus(prompt: GEOPrompt, status: string) {
    setBusy(prompt.id);
    setMessage(null);
    try {
      await api.updateGEOPrompt(projectId, prompt.id, { status });
      await refresh();
      setMessage({ title: "GEO prompt updated", detail: status, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not update GEO prompt", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function updateCompetitorStatus(competitor: GEOCompetitor, status: string) {
    setBusy(competitor.id);
    setMessage(null);
    try {
      await api.updateGEOCompetitor(projectId, competitor.id, { status });
      await refresh();
      setMessage({ title: "GEO competitor updated", detail: status, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not update GEO competitor", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function acceptAssetBrief(brief: GEOAssetBrief) {
    setBusy(brief.id);
    setMessage(null);
    try {
      const result = await api.acceptGEOAssetBrief(projectId, brief.id);
      await refresh();
      const generating = result.topic?.status === "generating";
      setMessage({ title: generating ? "GEO draft generation started" : "GEO brief converted", detail: result.topic?.title ?? brief.asset_type, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not accept GEO brief", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function createAction(opp: SEOOpportunity) {
    setOpportunityPending(opp.id, "create");
    setMessage(null);
    try {
      const workType = routeOverrides[opp.id] ?? opportunityWorkType(opp);
      const destination = destinationForWorkType(workType);
      const action = await api.createSEOContentAction(projectId, opp.id, {
        action_type: opp.recommended_action ?? undefined,
        asset_type: assetTypeForWorkType(opp, workType),
        work_type: workTypeKeys[workType],
      });
      setOpportunities((current) => current.map((item) => (item.id === opp.id ? { ...item, status: "converted" } : item)));
      setActions((current) => [action, ...current.filter((item) => item.id !== action.id)]);
      setRouteOverrides((current) => {
        if (!(opp.id in current)) return current;
        const { [opp.id]: _cleared, ...rest } = current;
        return rest;
      });
      setSelectedOpportunityID(null);
      setMessage({ title: `Sent to ${destination}`, detail: opp.recommended_action ?? opp.type, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not create action", detail: e.message, tone: "red" });
    } finally {
      setOpportunityPending(opp.id, null);
    }
  }

  async function snoozeOpportunity(opp: SEOOpportunity, days: number) {
    setOpportunityPending(opp.id, "snooze");
    setMessage(null);
    try {
      const updated = await api.snoozeSEOOpportunity(projectId, opp.id, { days });
      setOpportunities((current) => current.map((item) => (item.id === opp.id ? { ...item, ...updated } : item)));
      setSelectedOpportunityID(null);
      setMessage({ title: `Snoozed for ${days} days`, detail: "It returns to Needs decision when the snooze ends.", tone: "neutral" });
    } catch (e: any) {
      setMessage({ title: "Could not snooze opportunity", detail: e.message, tone: "red" });
    } finally {
      setOpportunityPending(opp.id, null);
    }
  }

  async function unsnoozeOpportunity(opp: SEOOpportunity) {
    setOpportunityPending(opp.id, "snooze");
    setMessage(null);
    try {
      const updated = await api.unsnoozeSEOOpportunity(projectId, opp.id);
      setOpportunities((current) => current.map((item) => (item.id === opp.id ? { ...item, ...updated } : item)));
      setMessage({ title: "Opportunity back in the queue", detail: "It needs a decision again.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not unsnooze opportunity", detail: e.message, tone: "red" });
    } finally {
      setOpportunityPending(opp.id, null);
    }
  }

  async function watchOpportunity(opp: SEOOpportunity) {
    setOpportunityPending(opp.id, "watch");
    setMessage(null);
    try {
      const item = await api.watchSEOOpportunity(projectId, opp.id);
      setOpportunities((current) => current.map((existing) => (existing.id === opp.id ? { ...existing, status: "watching" } : existing)));
      setWatchlist((current) => [item, ...current.filter((existing) => existing.id !== item.id)]);
      setSelectedOpportunityID(null);
      setMessage({ title: "Watching in Results", detail: `No changes will be made. Review again in ${item.observation_window_days} days.`, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not watch opportunity", detail: e.message, tone: "red" });
    } finally {
      setOpportunityPending(opp.id, null);
    }
  }

  async function closeWatchlistItem(item: SEOWatchlistItem, status: "closed" | "learned") {
    setBusy(`watchlist-${item.id}-${status}`);
    setMessage(null);
    try {
      const updated = await api.closeSEOWatchlistItem(projectId, item.id, { status });
      setWatchlist((current) => current.map((existing) => (existing.id === updated.id ? updated : existing)));
      setMessage({ title: status === "learned" ? "Watchlist item marked learned" : "Watchlist item closed", detail: watchlistItemTitle(item), tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not update watchlist item", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function verifyAction(action: SEOContentAction, status: "verified" | "failed") {
    setBusy(`verify-${action.id}-${status}`);
    setMessage(null);
    try {
      const updated = await api.verifySEOContentAction(projectId, action.id, {
        status,
        verification_snapshot: { source: "manual_dashboard", status },
      });
      setActions((current) => current.map((item) => (item.id === updated.id ? updated : item)));
      setResultsActions((current) => current.map((item) => (item.id === updated.id ? { ...item, ...updated } : item)));
      setMessage({ title: status === "verified" ? "Action verified" : "Verification failed", detail: action.action_type, tone: status === "verified" ? "green" : "amber" });
    } catch (e: any) {
      setMessage({ title: "Could not update verification", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function dismiss(opp: SEOOpportunity) {
    setOpportunityPending(opp.id, "dismiss");
    setMessage(null);
    try {
      await api.dismissSEOOpportunity(projectId, opp.id);
      setOpportunities((current) => current.filter((item) => item.id !== opp.id));
      setSelectedOpportunityID(null);
    } catch (e: any) {
      setMessage({ title: "Could not dismiss opportunity", detail: e.message, tone: "red" });
    } finally {
      setOpportunityPending(opp.id, null);
    }
  }

  async function savePolicy(next: Partial<SEOPolicy>) {
    const current = policy;
    if (!current) return;
    setBusy("policy");
    setMessage(null);
    try {
      await api.updateSEOPolicy(projectId, { ...current, ...next });
      await refresh();
      setMessage({ title: "Autopilot policy saved", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not save autopilot policy", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function createObjective() {
    if (!objectiveName.trim()) return;
    setBusy("objective");
    setMessage(null);
    try {
      await api.createSEOObjective(projectId, { name: objectiveName.trim(), primary_metric: "clicks", time_horizon_days: 90 });
      setObjectiveName("");
      await refresh();
      setMessage({ title: "SEO objective created", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not create objective", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function generatePlan() {
    setBusy("plan");
    setMessage(null);
    try {
      const result = await api.generateAutopilotPlan(projectId);
      await refresh();
      setMessage({ title: "Autopilot plan generated", detail: `${result.plan?.actions?.length ?? 0} actions selected`, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not generate autopilot plan", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function executeLatestPlan() {
    if (!latestPortfolioPlan) return;
    setBusy("execute-autopilot");
    setMessage(null);
    try {
      const result = await api.executeAutopilotPlan(projectId, latestPortfolioPlan.id);
      setExecutionResult(result);
      await refresh();
      setMessage({
        title: "Guarded execution complete",
        detail: `${result.executed_actions.length} executed; ${result.deferred_actions.length} deferred`,
        tone: result.executed_actions.length > 0 ? "green" : "amber",
      });
    } catch (e: any) {
      setMessage({ title: "Could not execute guarded actions", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function enterSafeMode() {
    setBusy("safe-mode");
    setMessage(null);
    try {
      await api.enterSafeMode(projectId, { reason: "manual safe mode", trigger_source: "manual", entered_by: "human" });
      await refresh();
      setMessage({ title: "Safe mode enabled", tone: "amber" });
    } catch (e: any) {
      setMessage({ title: "Could not enter safe mode", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  return (
    <>
    <div ref={mode === "analysis" ? analysisSurfaceRef : mode === "results" ? resultsSurfaceRef : undefined} className="space-y-7">
      <SectionHeader
        title={mode === "analysis" ? "Opportunities" : "Impact Reports"}
        eyebrow={mode === "analysis" ? "Analysis" : "Results and learning"}
        action={
          <div className="flex flex-wrap gap-2">
            {mode === "analysis" && (
              <>
                <GSCStatusMenu
                  projectId={projectId}
                  overview={overview}
                  gscConnection={gscConnection}
                  status={analysisStatus}
                  gscStatus={gscStatus}
                  busy={busy}
                  onConnect={startSearchConsoleOAuth}
                />
                <GA4StatusLink projectId={projectId} status={ga4Integration?.status} propertyID={ga4PropertyID} />
              </>
            )}
            <Button size="sm" onClick={manualRefresh} disabled={!!busy}>
              <ButtonProgress busy={busy === "refresh"} busyLabel="Refreshing" idleIcon={<RefreshCw size={14} />}>
                Refresh
              </ButtonProgress>
            </Button>
            <Button size="sm" onClick={runSync} disabled={!!busy}>
              <ButtonProgress busy={busy === "sync"} busyLabel="Syncing" idleIcon={<Search size={14} />}>
                Sync
              </ButtonProgress>
            </Button>
          </div>
        }
      />

      {mode === "analysis" && (
        <>
        <div className="space-y-5">
          <OpportunityFindingStatusPanel
            status={opportunityFindingStatus}
            busy={busy}
            projectId={projectId}
            onRun={runOpportunityFinding}
          />

          <section data-analysis-growth-findings-section className="space-y-3">
            <SectionHeader
              title={`Opportunity Queue · ${activeOpportunities.length} need decision`}
              eyebrow="Decision-ready recommendations"
              action={
                <div className="flex flex-wrap gap-2">
                  <Badge tone={activeOpportunities.length ? "red" : "neutral"}>{activeOpportunities.length ? "Needs decision" : "No review needed"}</Badge>
                  <Badge tone="neutral">{loopActiveCount} in loop</Badge>
                  <Button
                    data-opportunity-recent-drawer-trigger
                    size="sm"
                    variant="outline"
                    onClick={() => setAnalysisRecentDrawer("opportunities")}
                    aria-label={`Open Recently Decided (${opportunityRecentCount})`}
                  >
                    <History size={14} />
                    Recently Decided
                    <Badge tone={opportunityRecentCount ? "green" : "neutral"}>{opportunityRecentCount}</Badge>
                  </Button>
                </div>
              }
            />

            {activeOpportunities.length === 0 ? (
              <EmptyState
                title="No opportunities to review"
                detail="Refresh or Sync after Context changes. New findings will appear here when they need a decision."
              />
            ) : (
              <div data-analysis-finding-grid className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                {activeOpportunities.slice(0, 12).map((opp) => {
                  const workType = opportunityWorkType(opp);
                  const destination = opportunityDestination(opp);
                  const cta = opportunityPrimaryCTA(opp);
                  return (
                    <button
                      data-analysis-finding-card
                      key={opp.id}
                      type="button"
                      onClick={(event) => {
                        analysisReturnFocusRef.current = event.currentTarget;
                        setSelectedOpportunityID(opp.id);
                      }}
                      aria-label={`Open opportunity details: ${opportunityTitle(opp)}`}
                      className={`group flex h-full min-h-[220px] w-full flex-col rounded-lg border bg-white p-4 text-left shadow-sm transition hover:border-slate-300 hover:bg-slate-50/60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px ${
                        selectedOpportunityID === opp.id ? "border-slate-400 ring-2 ring-slate-200" : "border-slate-200"
                      }`}
                    >
                      <div className="flex h-full min-w-0 flex-col justify-between gap-4">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <Badge tone="blue">{workType}</Badge>
                            <Badge tone={toneForOpportunityPriority(opp)}>{opportunityPriorityLabel(opp)}</Badge>
                            <Badge tone="red">Needs decision</Badge>
                            <Badge tone="neutral">Detected {formatDate(opp.created_at ?? null)}</Badge>
                          </div>
                          <h3 className="mt-2 line-clamp-2 text-base font-bold leading-6 text-slate-950">{opportunityTitle(opp)}</h3>
                          <p className="mt-1 line-clamp-3 text-sm leading-5 text-slate-600">
                            {opp.expected_impact || "Review this opportunity against confirmed Context before creating downstream work."}
                          </p>
                        </div>
                        <div className="grid gap-2 text-sm">
                          <div>
                            <div className="text-xs font-semibold uppercase text-slate-400">Approve sends this to</div>
                            <div className="mt-1 truncate font-medium text-slate-700">{destination}</div>
                          </div>
                          <div>
                            <div className="text-xs font-semibold uppercase text-slate-400">Next step</div>
                            <div className="mt-1 truncate font-medium text-slate-700">{cta.label}</div>
                          </div>
                        </div>
                        <div className="mt-auto flex items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm font-semibold text-slate-700">
                          <span className="text-xs text-slate-400">Open details before approval</span>
                          <span className="flex items-center gap-1">
                            Review
                            <ChevronRight className="text-slate-400 transition group-hover:translate-x-0.5 group-hover:text-slate-600" size={17} />
                          </span>
                        </div>
                      </div>
                    </button>
                  );
                })}
              </div>
            )}

            {snoozedOpportunities.length > 0 && (
              <details className="rounded-lg border border-slate-200 bg-white">
                <summary className="cursor-pointer px-4 py-3 text-sm font-bold text-slate-900 transition hover:bg-slate-50">
                  Snoozed ({snoozedOpportunities.length})
                </summary>
                <div className="grid gap-2 border-t border-slate-100 p-3">
                  {snoozedOpportunities.map((opp) => (
                    <div
                      key={opp.id}
                      data-opportunity-snoozed-card
                      className="flex min-w-0 flex-col gap-2 rounded-md border border-slate-100 bg-slate-50 p-3 sm:flex-row sm:items-center sm:justify-between"
                    >
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge tone="neutral">Snoozed</Badge>
                          {opp.snoozed_until && <Badge tone="neutral">Returns {formatDate(opp.snoozed_until)}</Badge>}
                        </div>
                        <h3 className="mt-2 truncate text-sm font-bold text-slate-950">{opportunityTitle(opp)}</h3>
                      </div>
                      <Button size="sm" variant="ghost" onClick={() => unsnoozeOpportunity(opp)} disabled={!!opportunityBusy[opp.id]}>
                        <ButtonProgress busy={opportunityBusy[opp.id] === "snooze"} busyLabel="Waking" idleIcon={null}>
                          Unsnooze
                        </ButtonProgress>
                      </Button>
                    </div>
                  ))}
                </div>
              </details>
            )}
          </section>


          <section data-analysis-loop-strip aria-label="Loop in motion for Content Plan and Site Fixes work through Published / Applied stages" className="space-y-3">
            <SectionHeader
              title="Loop in motion"
              eyebrow="Where reviewed opportunities are now"
              action={
                <Link
                  href={`/projects/${projectId}/results`}
                  className="inline-flex h-8 items-center rounded-lg border border-slate-200 px-3 text-xs font-semibold text-slate-700 transition hover:bg-slate-50"
                >
                  View results
                </Link>
              }
            />
            <div className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
              <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-8">
                {loopSummaryItems.map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    data-loop-stage-card
                    aria-pressed={selectedLoopStage === item.key}
                    disabled={item.value === 0}
                    onClick={() => setSelectedLoopStage((current) => (current === item.key ? null : item.key))}
                    className={cx(
                      "rounded-md border px-3 py-3 text-left transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px disabled:cursor-not-allowed disabled:opacity-55",
                      selectedLoopStage === item.key
                        ? "border-[#d93820]/40 bg-[#fff4f1] shadow-[inset_0_0_0_1px_rgba(217,56,32,0.12)]"
                        : "border-slate-100 bg-slate-50 hover:border-slate-300 hover:bg-white",
                    )}
                  >
                    <div className="font-mono text-xl font-bold text-slate-950">{item.value}</div>
                    <div className="mt-1 truncate text-xs font-semibold text-slate-500">{item.label}</div>
                  </button>
                ))}
              </div>
              {selectedLoopStage && selectedLoopActions.length > 0 && (
                <div className="mt-4 border-t border-slate-100 pt-3">
                  <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                    <div className="text-sm font-bold text-slate-950">{loopStageDetailTitle}</div>
                    <button
                      type="button"
                      onClick={() => setSelectedLoopStage(null)}
                      className="text-xs font-semibold text-slate-500 transition hover:text-slate-800"
                    >
                      Clear
                    </button>
                  </div>
                  <div className="grid gap-2 lg:grid-cols-3">
                    {selectedLoopActions.map((action) => {
                      const stage = deriveVisibilityLifecycleStage(action);
                      const href = loopActionCurrentHref(projectId, action);
                      const label = loopActionCurrentLabel(action);
                      const content = (
                        <div className="flex min-h-[78px] flex-col justify-between gap-2">
                          <div className="min-w-0">
                            <div className="flex items-start justify-between gap-2">
                              <div className="flex flex-wrap items-center gap-1">
                                <Badge tone={loopActionDestinationLabel(action) === "Site Fixes" ? "blue" : "violet"}>
                                  {loopActionDestinationLabel(action) === "Site Fixes" ? (
                                    <Wrench size={12} className="mr-1" />
                                  ) : (
                                    <FileText size={12} className="mr-1" />
                                  )}
                                  {loopActionDestinationLabel(action)}
                                </Badge>
                                <Badge tone={lifecycleStageTone(stage)}>{lifecycleStageLabel(stage)}</Badge>
                              </div>
                              <ChevronRight className="mt-0.5 shrink-0 text-slate-400 transition group-hover:translate-x-0.5 group-hover:text-slate-600" size={16} />
                            </div>
                            <div className="mt-2 truncate text-sm font-semibold text-slate-900">{loopActionTitle(action)}</div>
                            <div className="mt-1 truncate text-xs text-slate-500">
                              {action.target_url ?? action.normalized_target_url ?? action.opportunity_page_url ?? action.id}
                            </div>
                          </div>
                          <div className="text-xs font-semibold text-slate-600">Open current location: {label}</div>
                        </div>
                      );
                      return (
                        <Link
                          key={action.id}
                          data-loop-action-card
                          href={href}
                          className="group block rounded-md border border-slate-100 bg-slate-50 px-3 py-2 text-left transition hover:border-slate-300 hover:bg-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px"
                        >
                          {content}
                        </Link>
                      );
                    })}
                  </div>
                </div>
              )}
              {!selectedLoopStage && (
                <p className="mt-3 text-sm leading-6 text-slate-500">Reviewed Content Plan and Site Fixes work will appear here after it enters the loop.</p>
              )}
            </div>
          </section>

          {readiness && !readiness.ready_for_level_2 && (
            <section className="rounded-[8px] border border-slate-200 bg-white px-5 py-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <p className="text-sm font-bold text-slate-950">Finish automation setup in Settings</p>
                  <p className="mt-1 text-sm text-slate-600">
                    Opportunity review can continue. Guarded execution waits for policy, publisher, notification, budget, safe mode, kill switch, and recovery gates.
                  </p>
                </div>
                <Link
                  href={`/projects/${projectId}/settings#automation`}
                  className="inline-flex h-8 items-center justify-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 transition-colors hover:bg-slate-50"
                >
                  Open Settings
                </Link>
              </div>
            </section>
          )}
        </div>

        </>
      )}

      {mode === "analysis" && (
        <>
          <RightDrawer
            open={analysisRecentDrawer === "opportunities"}
            title="Recently Decided"
            eyebrow="Opportunity Queue"
            subtitle="Opportunity decisions that already moved into the next workflow surface."
            dataAttribute="opportunity-recent-drawer"
            maxWidthClassName="max-w-5xl"
            onClose={() => setAnalysisRecentDrawer(null)}
          >
            {opportunityRecentCount === 0 ? (
              <EmptyState title="No recent decisions" detail="Approved, watched, or routed opportunities will appear here after they leave the active queue." />
            ) : (
              <div data-opportunity-handoff-grid className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                {sentOpportunityLinks.map((action) => {
                  const label = loopActionCurrentLabel(action as LoopAction);
                  const href = loopActionCurrentHref(projectId, action as LoopAction);
                  const content = (
                    <div className="flex h-full min-w-0 flex-col justify-between gap-4">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge tone="green">{actionHandoffStatus(action)}</Badge>
                          <Badge tone="neutral">{approvalSourceLabel(action.approval_source)}</Badge>
                        </div>
                        <h3 className="mt-3 line-clamp-2 text-base font-bold leading-6 text-slate-950">{loopActionTitle(action as any)}</h3>
                        <p className="mt-2 line-clamp-2 break-all text-sm leading-5 text-slate-500">
                          {action.target_url ?? action.normalized_target_url ?? "Approved work is ready in the next queue."}
                        </p>
                      </div>
                      <div className="mt-auto flex items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm font-semibold text-slate-700">
                        <span>View in {label}</span>
                        <ChevronRight size={17} className="text-slate-400 transition group-hover:translate-x-0.5 group-hover:text-slate-600" />
                      </div>
                    </div>
                  );


                  return (
                    <Link
                      key={action.id}
                      data-opportunity-handoff-card
                      aria-label={`Open "${loopActionTitle(action as any)}" in ${label}`}
                      href={href}
                      className="group flex h-full min-h-[220px] w-full flex-col rounded-lg border border-slate-200 bg-white p-4 text-left shadow-sm transition hover:border-slate-300 hover:bg-slate-50/60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px"
                    >
                      {content}
                    </Link>
                  );
                })}
                {watchingOpportunityLinks.map((opp) => {
                  const watchItem = watchlist.find((item) => item.source_opportunity_id === opp.id);
                  return (
                    <Link
                      key={opp.id}
                      data-opportunity-handoff-card
                      href={`/projects/${projectId}/results?watch=${opp.id}`}
                      className="group flex h-full min-h-[220px] w-full flex-col rounded-lg border border-slate-200 bg-white p-4 text-left shadow-sm transition hover:border-slate-300 hover:bg-slate-50/60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px"
                    >
                      <div className="flex h-full min-w-0 flex-col justify-between gap-4">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <Badge tone="green">Watching in Results</Badge>
                            <Badge tone="neutral">No changes planned</Badge>
                          </div>
                          <h3 className="mt-3 line-clamp-2 text-base font-bold leading-6 text-slate-950">{opportunityTitle(opp)}</h3>
                          <p className="mt-2 line-clamp-2 text-sm leading-5 text-slate-500">
                            {watchItem?.due_at ? `Observation window ends ${formatDate(watchItem.due_at)}.` : "Observing signals before deciding on work."}
                          </p>
                        </div>
                        <div className="mt-auto flex items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm font-semibold text-slate-700">
                          <span>View in Results</span>
                          <ChevronRight size={17} className="text-slate-400 transition group-hover:translate-x-0.5 group-hover:text-slate-600" />
                        </div>
                      </div>
                    </Link>
                  );
                })}
              </div>
            )}
          </RightDrawer>
        </>
      )}

      {mode === "results" && (
        <div className="space-y-7" data-results-actions={resultsActions.length}>
          <section>
            <SectionHeader
              title="Impact Reports"
              eyebrow="Outcome summary - Published work"
              action={
                <Badge tone={measurementExceptions.length ? "amber" : attributionMeasuredActions.length ? "green" : "neutral"}>
                  {attributionMeasuredActions.length}
                </Badge>
              }
            />
            <div className="grid gap-3 md:grid-cols-3 xl:grid-cols-6">
              {[
                { label: "Waiting", value: outcomeCounts.waiting, tone: "neutral" as const, detail: "Inside measurement window" },
                { label: "Positive", value: outcomeCounts.positive, tone: "green" as const, detail: "Signals improved" },
                { label: "Negative", value: outcomeCounts.negative, tone: "red" as const, detail: "Needs follow-up" },
                { label: "Mixed", value: outcomeCounts.mixed, tone: "amber" as const, detail: "Signals are split" },
                { label: "Insufficient data", value: outcomeCounts.insufficient_data, tone: "amber" as const, detail: "insufficient_data" },
                { label: "Inconclusive", value: outcomeCounts.inconclusive, tone: "amber" as const, detail: "No clear signal yet" },
              ].map((item) => (
                <div key={item.label} data-state={item.detail === "insufficient_data" ? "insufficient_data" : undefined} className="rounded-lg border border-slate-200 bg-white p-4">
                  <Badge tone={item.tone}>{item.label}</Badge>
                  <div className="mt-3 text-2xl font-bold text-slate-950">{item.value}</div>
                  <p className="mt-1 text-sm leading-5 text-slate-500">{item.detail === "insufficient_data" ? "Not enough comparable data" : item.detail}</p>
                </div>
              ))}
            </div>
          </section>

          <section>
            <SectionHeader
              title="Measurement queue"
              eyebrow="Checkpoint status"
              action={<Badge tone={measurementQueueCounts.blocked ? "red" : measurementQueueCounts.completed ? "green" : "neutral"}>{attributionActions.length}</Badge>}
            />
            <div className="grid gap-3 md:grid-cols-4">
              {[
                { label: "Waiting", value: measurementQueueCounts.waiting, tone: "neutral" as const, detail: "Not published or verified yet" },
                { label: "Too early", value: measurementQueueCounts.too_early, tone: "amber" as const, detail: "Published, checkpoint not due" },
                { label: "Blocked", value: measurementQueueCounts.blocked, tone: "red" as const, detail: "Execution or verification issue" },
                { label: "Completed", value: measurementQueueCounts.completed, tone: "green" as const, detail: "Checkpoint computed" },
              ].map((item) => (
                <div key={item.label} className="rounded-lg border border-slate-200 bg-white p-4">
                  <Badge tone={item.tone}>{item.label}</Badge>
                  <div className="mt-3 text-2xl font-bold text-slate-950">{item.value}</div>
                  <p className="mt-1 text-sm leading-5 text-slate-500">{item.detail}</p>
                </div>
              ))}
            </div>
          </section>

          <section id="results-watchlist" data-results-watchlist>
            <SectionHeader
              title="Watchlist"
              eyebrow="Watch-only opportunities"
              action={<Badge tone={watchlist.some((item) => item.status === "due_for_review") ? "amber" : "neutral"}>{watchlist.filter((item) => item.status === "watching" || item.status === "due_for_review").length} watching</Badge>}
            />
            {watchlist.length === 0 ? (
              <EmptyState
                title="Nothing on the watchlist"
                detail="Choose Watch in Results on an opportunity to observe its signals without creating work."
              />
            ) : (
              <div className="grid gap-2">
                {watchlist.map((item) => {
                  const highlighted = requestedWatchOpportunityID === item.source_opportunity_id;
                  const closed = item.status === "closed" || item.status === "learned";
                  return (
                    <div
                      key={item.id}
                      id={`watchlist-item-${item.source_opportunity_id}`}
                      data-watchlist-card
                      tabIndex={-1}
                      className={cx(
                        "flex min-w-0 flex-col gap-2 rounded-lg border bg-white p-4 shadow-sm sm:flex-row sm:items-center sm:justify-between",
                        highlighted ? "citeloop-linked-card-pulse border-[#d93820] ring-2 ring-[#d93820]/15" : "border-slate-200",
                      )}
                    >
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge tone={watchlistStatusTone(item.status)}>{watchlistStatusLabel(item.status)}</Badge>
                          <Badge tone="neutral">Watch Result</Badge>
                          {item.due_at && !closed && <Badge tone="neutral">Review due {formatDate(item.due_at)}</Badge>}
                        </div>
                        <h3 className="mt-2 truncate text-sm font-bold text-slate-950">{watchlistItemTitle(item)}</h3>
                        <p className="mt-1 truncate text-xs text-slate-500">
                          {item.opportunity_page_url ?? item.opportunity_query ?? `Observation window: ${item.observation_window_days} days`}
                        </p>
                      </div>
                      {!closed && (
                        <div className="flex shrink-0 items-center gap-2">
                          <Button size="sm" variant="ghost" onClick={() => closeWatchlistItem(item, "learned")} disabled={!!busy}>
                            <ButtonProgress busy={busy === `watchlist-${item.id}-learned`} busyLabel="Saving" idleIcon={null}>
                              Mark learned
                            </ButtonProgress>
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => closeWatchlistItem(item, "closed")} disabled={!!busy}>
                            <ButtonProgress busy={busy === `watchlist-${item.id}-closed`} busyLabel="Closing" idleIcon={null}>
                              Close
                            </ButtonProgress>
                          </Button>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </section>

          <section>
            <SectionHeader
              title="Learning signal"
              eyebrow="Conservative learning"
              action={<Badge tone={measurementExceptions.length ? "amber" : "neutral"}>Policy-gated</Badge>}
            />
            <div className="grid gap-3 md:grid-cols-3">
              <div className="rounded-lg border border-slate-200 bg-white p-4">
                <Badge tone="blue">Prioritization input</Badge>
                <div className="mt-3 text-lg font-bold leading-6 text-slate-950">Use measured outcomes to rank the next opportunities.</div>
                <p className="mt-2 text-sm leading-5 text-slate-500">
                  Completed work can influence backlog order, expected impact, and follow-up timing.
                </p>
              </div>
              <div className="rounded-lg border border-slate-200 bg-white p-4">
                <Badge tone={measurementExceptions.length ? "amber" : "green"}>{measurementExceptions.length}</Badge>
                <div className="mt-3 text-lg font-bold leading-6 text-slate-950">Results needing attention</div>
                <p className="mt-2 text-sm leading-5 text-slate-500">
                  Negative, mixed, inconclusive, or insufficient data outcomes stay visible for review before the next action.
                </p>
              </div>
              <div className="rounded-lg border border-slate-200 bg-white p-4">
                <Badge tone="neutral">No auto-risk</Badge>
                <div className="mt-3 text-lg font-bold leading-6 text-slate-950">Policy gates still decide execution.</div>
                <p className="mt-2 text-sm leading-5 text-slate-500">
                  Conservative learning informs prioritization, but it does not auto-change risky behavior without review gates.
                </p>
              </div>
            </div>
          </section>

          <section>
            <SectionHeader
              title="Action-level attribution"
              eyebrow="Impact reports"
              action={
                <div className="flex items-center gap-2">
                  <Badge tone="neutral">{attributionActions.length}</Badge>
                  <Button
                    size="sm"
                    onClick={async () => {
                      setBusy("results-recompute");
                      setMessage(null);
                      try {
                        const result = await api.recomputeResults(projectId);
                        const freshRows = await api.listResultsActions(projectId, { limit: 50 });
                        setResultsActions(freshRows.length ? freshRows : result.actions);
                        setMessage({ title: "Results recomputed", detail: result.status, tone: "green" });
                      } catch (e: any) {
                        setMessage({ title: "Could not recompute results", detail: e.message, tone: "red" });
                      } finally {
                        setBusy(null);
                      }
                    }}
                    disabled={busy === "results-recompute"}
                  >
                    <ButtonProgress busy={busy === "results-recompute"} busyLabel="Recomputing" idleIcon={<RefreshCw size={14} />}>
                      Recompute
                    </ButtonProgress>
                  </Button>
                </div>
              }
            />
            {attributionActions.length === 0 ? (
              <EmptyState title="No published or applied actions are ready for attribution yet" detail="Published or URL-verified actions will appear here once they enter the loop." />
            ) : (
              <div data-results-action-grid className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                {(resultsActions.length ? attributionActions.slice(0, 12) : resultActions.slice(0, 12).map((action) => action)).map((action) => {
                  const state = actionMeasurementState(action);
                  const queue = measurementQueueState(action);
                  const publishedTitle = resultPublishedArticleTitle(action);
                  const publishedURL = resultPublishedArticleUrl(action);
                  const sourceURL = resultSourceEvidenceUrl(action);
                  const highlighted = highlightedResultActionID === action.id;
                  return (
                    <button
                      key={action.id}
                      type="button"
                      data-results-action-card={action.id}
                      aria-label={`Open attribution details: ${publishedTitle}`}
                      onClick={(event) => {
                        clearResultHandoffTimers();
                        setHighlightedResultActionID(null);
                        resultReturnFocusRef.current = event.currentTarget;
                        setSelectedResultActionID(action.id);
                      }}
                      className={cx(
                        "group flex h-full min-h-[220px] w-full flex-col rounded-lg border bg-white p-4 text-left shadow-sm transition hover:border-slate-300 hover:bg-slate-50/60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px",
                        highlighted ? "citeloop-linked-card-pulse border-[#d93820] ring-2 ring-[#d93820]/15" : selectedResultActionID === action.id ? "border-slate-400 ring-2 ring-slate-200" : "border-slate-200",
                      )}
                    >
                      <div className="flex h-full min-w-0 flex-col justify-between gap-4">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <Badge tone={state.tone}>{state.label}</Badge>
                            <Badge tone={queue.tone}>{queue.label}</Badge>
                            <Badge tone={toneForStatus(action.status)}>{action.status}</Badge>
                          </div>
                          <h3 className="mt-3 line-clamp-2 text-lg font-bold leading-6 text-slate-950">{publishedTitle}</h3>
                          <p className="mt-2 line-clamp-2 break-all text-sm leading-5 text-slate-600">{publishedURL || sourceURL || action.id}</p>
                          {sourceURL && (
                            <p className="mt-1 line-clamp-1 break-all text-xs font-medium leading-5 text-slate-400">Source: {sourceURL}</p>
                          )}
                        </div>
                        <div className="grid gap-3 text-sm">
                          <div>
                            <div className="text-xs font-semibold uppercase text-slate-400">Published / applied</div>
                            <div className="mt-1 font-medium text-slate-700">{formatDate(action.published_at ?? action.verified_at ?? null)}</div>
                          </div>
                          <div>
                            <div className="text-xs font-semibold uppercase text-slate-400">Measurement window</div>
                            <div className="mt-1 font-medium text-slate-700">{measurementWindowLabel(action.measurement_window)}</div>
                          </div>
                          <div>
                            <div className="text-xs font-semibold uppercase text-slate-400">Outcome reason</div>
                            <div className="mt-1 line-clamp-2 font-medium leading-5 text-slate-700">{state.detail}</div>
                          </div>
                          <div>
                            <div className="text-xs font-semibold uppercase text-slate-400">Checkpoint state</div>
                            <div className="mt-1 line-clamp-2 font-medium leading-5 text-slate-700">{queue.detail}</div>
                          </div>
                        </div>
                        <div className="mt-auto flex items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm font-semibold text-slate-700">
                          <span>Open details</span>
                          <ChevronRight className="text-slate-400 transition group-hover:translate-x-0.5 group-hover:text-slate-600" size={17} />
                        </div>
                      </div>
                    </button>
                  );
                })}
              </div>
            )}
          </section>

          <section>
            <SectionHeader title="AI citation signals" action={<Badge tone={showGeoScore ? "green" : "neutral"}>{geoOverview?.score?.confidence ?? "insufficient_data"}</Badge>} />
            <div className="grid gap-3 md:grid-cols-4">
              <div className="rounded-lg border border-slate-200 bg-white p-4">
                <div className="text-sm font-bold text-slate-900">{showGeoScore ? metric(geoScoreValue, 1) : "Insufficient data"}</div>
                <p className="mt-1 text-sm text-slate-500">Visibility score</p>
              </div>
              <div className="rounded-lg border border-slate-200 bg-white p-4">
                <div className="text-sm font-bold text-slate-900">{percent(geoCoverage)}</div>
                <p className="mt-1 text-sm text-slate-500">Coverage</p>
              </div>
              <div className="rounded-lg border border-slate-200 bg-white p-4">
                <div className="text-sm font-bold text-slate-900">{geoOverview?.prompts.length ?? 0}</div>
                <p className="mt-1 text-sm text-slate-500">Prompts</p>
              </div>
              <div className="rounded-lg border border-slate-200 bg-white p-4">
                <div className="text-sm font-bold text-slate-900">{assetBriefs.length}</div>
                <p className="mt-1 text-sm text-slate-500">Asset briefs</p>
              </div>
            </div>
          </section>

          <details className="rounded-xl border border-slate-200 bg-white p-4">
            <summary className="cursor-pointer text-sm font-bold text-slate-900">Advanced diagnostics</summary>
            <div className="mt-4 space-y-7">
      <section>
        <SectionHeader
          title="Setup"
          eyebrow={overview?.capability_mode ?? "public_only"}
          action={
            <Badge tone={overview?.handoff_ready_for_autopilot ? "green" : "amber"}>
              {overview?.handoff_ready_for_autopilot ? "ready" : "limited"}
            </Badge>
          }
        />
        {overview?.setup_checklist?.length ? (
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {overview.setup_checklist.map((item) => (
              <div key={item.key} className="rounded-lg border border-slate-200 bg-white p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="text-sm font-bold text-slate-900">{item.label}</div>
                    {item.capability_impact && <p className="mt-1 text-sm leading-5 text-slate-500">{item.capability_impact}</p>}
                  </div>
                  <Badge tone={toneForSetupStatus(item.status)}>{item.status}</Badge>
                </div>
                {item.next_action && <p className="mt-3 text-sm font-semibold leading-5 text-slate-700">{item.next_action}</p>}
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="No setup checklist" detail="Refresh SEO data after creating the project." />
        )}
      </section>

      <div className="grid gap-3 md:grid-cols-4">
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <BarChart3 className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">{metric(overview?.last_28_days?.clicks_28d)}</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">Clicks, 28d</p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <Search className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">{metric(overview?.last_28_days?.impressions_28d)}</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">Impressions, 28d</p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <CheckCircle2 className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">{percent(overview?.last_28_days?.ctr_28d)}</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">CTR, 28d</p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <ShieldAlert className="mb-3 text-slate-400" size={18} />
          <div className="flex items-center gap-2 text-sm font-bold text-slate-900">
            {overview?.technical?.checked_urls ?? 0}
            <Badge tone={overview?.cold_start ? "amber" : "green"}>{overview?.cold_start ? "cold" : "ready"}</Badge>
          </div>
          <p className="mt-1 text-sm leading-5 text-slate-500">Technical URLs</p>
        </div>
      </div>

      <section>
        <SectionHeader title="Settings" action={<Badge tone={toneForStatus(gscStatus)}>{gscStatus}</Badge>} />
        <div className="grid gap-3 rounded-lg border border-slate-200 bg-white p-4">
          <Field label="Site URL">
            <TextInput value={siteURL} onChange={(e) => setSiteURL(e.target.value)} placeholder="https://dev.unipost.dev" />
          </Field>
          {gscStatus === "missing" && (
            <Notice
              title="Search Console not connected"
              detail="CiteLoop is using the public site until an internal admin connects first-party search data."
              tone="amber"
            />
          )}
          <Button size="sm" onClick={saveSettings} disabled={busy === "settings" || !siteURL}>
            <ButtonProgress busy={busy === "settings"} busyLabel="Saving settings" idleIcon={<Settings size={14} />}>
              Save settings
            </ButtonProgress>
          </Button>
        </div>
      </section>

      <section>
        <SectionHeader
          title="GEO crawler access"
          action={
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone={crawlerSnapshots.some((snapshot) => snapshot.robots_state === "disallowed") ? "red" : crawlerSnapshots.length ? "green" : "neutral"}>
                {crawlerSnapshots.length}
              </Badge>
              <Button size="sm" onClick={runCrawlerAudit} disabled={!!busy || !siteURL}>
                <ButtonProgress busy={busy === "geo-crawler"} busyLabel="Auditing" idleIcon={<RefreshCw size={14} />}>
                  Audit
                </ButtonProgress>
              </Button>
            </div>
          }
        />
        {crawlerSnapshots.length === 0 ? (
          <EmptyState title="No crawler audit snapshots" detail="Run audit after saving a site URL." />
        ) : (
          <div className="overflow-x-auto rounded-lg border border-slate-200 bg-white">
            <table className="w-full min-w-[840px] text-left text-sm">
              <thead className="border-b border-slate-200 bg-slate-50 text-xs uppercase text-slate-500">
                <tr>
                  <th className="px-4 py-3 font-semibold">Crawler</th>
                  <th className="px-4 py-3 font-semibold">Page</th>
                  <th className="px-4 py-3 font-semibold">Robots</th>
                  <th className="px-4 py-3 font-semibold">Access</th>
                  <th className="px-4 py-3 font-semibold">Confidence</th>
                  <th className="px-4 py-3 font-semibold">Source</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {crawlerSnapshots.slice(0, 40).map((snapshot) => (
                  <tr key={`${snapshot.normalized_page_url}-${snapshot.target_user_agent}-${snapshot.evidence_type}`}>
                    <td className="whitespace-nowrap px-4 py-3 font-medium text-slate-900">{snapshot.target_user_agent}</td>
                    <td className="max-w-[340px] truncate px-4 py-3 text-slate-600">{snapshot.page_url || snapshot.normalized_page_url}</td>
                    <td className="px-4 py-3">
                      <Badge tone={toneForRobots(snapshot.robots_state)}>{snapshot.robots_state}</Badge>
                    </td>
                    <td className="px-4 py-3">
                      <Badge tone={toneForAccess(snapshot.access_state)}>{snapshot.access_state}</Badge>
                    </td>
                    <td className="px-4 py-3">
                      <Badge tone={toneForConfidence(snapshot.confidence)}>{snapshot.confidence}</Badge>
                    </td>
                    <td className="px-4 py-3">
                      <Badge tone={snapshot.inferred ? "amber" : "green"}>{snapshot.inferred ? "inferred" : "observed"}</Badge>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section>
        <SectionHeader
          title="GEO visibility"
          action={
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone={geoOverview?.score?.confidence === "high" ? "green" : geoOverview?.score ? "amber" : "neutral"}>
                {geoOverview?.score?.confidence ?? "insufficient_data"}
              </Badge>
              <Button size="sm" onClick={generatePromptSet} disabled={!!busy}>
                <ButtonProgress busy={busy === "geo-prompts"} busyLabel="Generating prompts" idleIcon={<FileText size={14} />}>
                  Prompts
                </ButtonProgress>
              </Button>
              <Button size="sm" onClick={observeGEOProvider} disabled={!!busy || (geoOverview?.prompts.length ?? 0) === 0}>
                <ButtonProgress busy={busy === "geo-provider"} busyLabel="Observing provider" idleIcon={<Search size={14} />}>
                  Provider
                </ButtonProgress>
              </Button>
              <Button size="sm" onClick={analyzeGEOOpportunities} disabled={!!busy}>
                <ButtonProgress busy={busy === "geo-analyze"} busyLabel="Analyzing" idleIcon={<BarChart3 size={14} />}>
                  Analyze
                </ButtonProgress>
              </Button>
            </div>
          }
        />
        <div className="grid gap-3 md:grid-cols-4">
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">{showGeoScore ? metric(geoScoreValue, 1) : "Insufficient data"}</div>
            <p className="mt-1 text-sm text-slate-500">Visibility score</p>
          </div>
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">{percent(geoCoverage)}</div>
            <p className="mt-1 text-sm text-slate-500">Coverage</p>
          </div>
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">{geoOverview?.prompts.length ?? 0}</div>
            <p className="mt-1 text-sm text-slate-500">Prompts</p>
          </div>
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">{assetBriefs.length}</div>
            <p className="mt-1 text-sm text-slate-500">Asset briefs</p>
          </div>
        </div>
        <div className="mt-3 grid gap-3 lg:grid-cols-2">
          <div className="overflow-hidden rounded-lg border border-slate-200 bg-white">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-slate-200 bg-slate-50 text-xs uppercase text-slate-500">
                <tr>
                  <th className="px-4 py-3 font-semibold">Prompt set</th>
                  <th className="px-4 py-3 font-semibold">Status</th>
                  <th className="px-4 py-3 font-semibold">Prompts</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {(geoOverview?.prompt_sets ?? []).slice(0, 8).map((set) => (
                  <tr key={set.id}>
                    <td className="px-4 py-3 font-medium text-slate-900">{set.name}</td>
                    <td className="px-4 py-3">
                      <Badge tone={toneForStatus(set.status)}>{set.status}</Badge>
                    </td>
                    <td className="px-4 py-3 text-slate-600">{promptCountBySet.get(set.id) ?? 0}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {(geoOverview?.prompt_sets.length ?? 0) === 0 && <div className="p-4"><EmptyState title="No prompt sets" detail="Generate prompts from the active profile." /></div>}
          </div>
          <div className="overflow-hidden rounded-lg border border-slate-200 bg-white">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-slate-200 bg-slate-50 text-xs uppercase text-slate-500">
                <tr>
                  <th className="px-4 py-3 font-semibold">Prompt</th>
                  <th className="px-4 py-3 font-semibold">Status</th>
                  <th className="px-4 py-3 font-semibold">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {(geoOverview?.prompts ?? []).slice(0, 8).map((prompt) => (
                  <tr key={prompt.id}>
                    <td className="max-w-[280px] truncate px-4 py-3 font-medium text-slate-900">{prompt.prompt_text}</td>
                    <td className="px-4 py-3">
                      <Badge tone={toneForStatus(prompt.status)}>{prompt.status}</Badge>
                    </td>
                    <td className="px-4 py-3">
                      <Button
                        size="sm"
                        onClick={() => updatePromptStatus(prompt, prompt.status === "active" ? "paused" : "active")}
                        disabled={busy === prompt.id}
                      >
                        <ButtonProgress busy={busy === prompt.id} busyLabel="Updating" idleIcon={null}>
                          {prompt.status === "active" ? "Pause" : "Activate"}
                        </ButtonProgress>
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {(geoOverview?.prompts.length ?? 0) === 0 && <div className="p-4"><EmptyState title="No prompts" detail="Generate prompts from the active profile." /></div>}
          </div>
          <div className="overflow-hidden rounded-lg border border-slate-200 bg-white">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-slate-200 bg-slate-50 text-xs uppercase text-slate-500">
                <tr>
                  <th className="px-4 py-3 font-semibold">Prompt</th>
                  <th className="px-4 py-3 font-semibold">Engine</th>
                  <th className="px-4 py-3 font-semibold">Citations</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {(geoOverview?.observations ?? []).slice(0, 8).map((observation) => (
                  <tr key={observation.id}>
                    <td className="max-w-[240px] truncate px-4 py-3 text-slate-700">{observation.answer_summary || observation.prompt_id || observation.id}</td>
                    <td className="px-4 py-3 text-slate-600">{observation.engine}</td>
                    <td className="px-4 py-3">
                      <Badge tone={observation.project_citation_count > 0 ? "green" : "amber"}>{observation.project_citation_count}</Badge>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {(geoOverview?.observations.length ?? 0) === 0 && <div className="p-4"><EmptyState title="No observations" detail="Manual fixture observations will appear here." /></div>}
          </div>
          <div className="overflow-hidden rounded-lg border border-slate-200 bg-white">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-slate-200 bg-slate-50 text-xs uppercase text-slate-500">
                <tr>
                  <th className="px-4 py-3 font-semibold">Competitor</th>
                  <th className="px-4 py-3 font-semibold">Status</th>
                  <th className="px-4 py-3 font-semibold">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {(geoOverview?.competitors ?? []).slice(0, 8).map((competitor) => (
                  <tr key={competitor.id}>
                    <td className="max-w-[220px] truncate px-4 py-3 font-medium text-slate-900">{competitor.name}</td>
                    <td className="px-4 py-3">
                      <Badge tone={toneForStatus(competitor.status)}>{competitor.status}</Badge>
                    </td>
                    <td className="px-4 py-3">
                      <Button
                        size="sm"
                        onClick={() => updateCompetitorStatus(competitor, competitor.status === "active" ? "paused" : "active")}
                        disabled={busy === competitor.id}
                      >
                        <ButtonProgress busy={busy === competitor.id} busyLabel="Updating" idleIcon={null}>
                          {competitor.status === "active" ? "Pause" : "Activate"}
                        </ButtonProgress>
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {(geoOverview?.competitors.length ?? 0) === 0 && <div className="p-4"><EmptyState title="No competitors" detail="Generate prompts from a profile with competitors." /></div>}
          </div>
        </div>
        <div className="mt-3 rounded-lg border border-slate-200 bg-white p-4">
          <div className="grid gap-3 lg:grid-cols-[minmax(0,1.5fr)_minmax(0,1.5fr)_repeat(5,minmax(120px,1fr))]">
            <Field label="Surface URL">
              <TextInput value={surfaceURL} onChange={(event) => setSurfaceURL(event.target.value)} placeholder="https://dev.to/team/source" />
            </Field>
            <Field label="Source URL">
              <TextInput value={surfaceSourceURL} onChange={(event) => setSurfaceSourceURL(event.target.value)} placeholder="https://example.com/blog/source" />
            </Field>
            <Field label="Owner">
              <select className="h-10 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700" value={surfaceOwnerType} onChange={(event) => setSurfaceOwnerType(event.target.value)}>
                <option value="managed_external">Managed external</option>
                <option value="project">Owned</option>
                <option value="third_party">Third party</option>
              </select>
            </Field>
            <Field label="Platform">
              <select className="h-10 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700" value={surfacePlatform} onChange={(event) => setSurfacePlatform(event.target.value)}>
                <option value="devto">Dev.to</option>
                <option value="hashnode">Hashnode</option>
                <option value="medium">Medium</option>
                <option value="linkedin">LinkedIn</option>
                <option value="github">GitHub</option>
                <option value="product_hunt">Product Hunt</option>
                <option value="site">Site</option>
              </select>
            </Field>
            <Field label="Publication">
              <select className="h-10 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700" value={surfacePublicationStatus} onChange={(event) => setSurfacePublicationStatus(event.target.value)}>
                <option value="draft">Draft</option>
                <option value="published">Published</option>
                <option value="observed">Observed</option>
                <option value="unknown">Unknown</option>
              </select>
            </Field>
            <Field label="Indexability">
              <select className="h-10 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700" value={surfaceIndexabilityStatus} onChange={(event) => setSurfaceIndexabilityStatus(event.target.value)}>
                <option value="unknown">Unknown</option>
                <option value="indexable">Indexable</option>
                <option value="noindex">Noindex</option>
                <option value="blocked">Blocked</option>
              </select>
            </Field>
            <Field label="Canonical">
              <select className="h-10 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700" value={surfaceCanonicalStatus} onChange={(event) => setSurfaceCanonicalStatus(event.target.value)}>
                <option value="unknown">Unknown</option>
                <option value="canonical">Canonical</option>
                <option value="source_linked">Source linked</option>
                <option value="missing">Missing</option>
                <option value="mismatch">Mismatch</option>
              </select>
            </Field>
            <Field label="Confidence">
              <select className="h-10 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700" value={surfaceOwnerConfidence} onChange={(event) => setSurfaceOwnerConfidence(event.target.value)}>
                <option value="medium">Medium</option>
                <option value="high">High</option>
                <option value="low">Low</option>
              </select>
            </Field>
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            <Button size="sm" onClick={addExternalSurface} disabled={busy === "geo-surface" || !surfaceURL.trim()}>
              <ButtonProgress busy={busy === "geo-surface"} busyLabel="Saving surface" idleIcon={<FileText size={14} />}>
                Surface
              </ButtonProgress>
            </Button>
            <Button size="sm" onClick={monitorExternalSurfaces} disabled={!!busy || (geoOverview?.external_surfaces.length ?? 0) === 0}>
              <ButtonProgress busy={busy === "geo-surface-monitor"} busyLabel="Monitoring" idleIcon={<RefreshCw size={14} />}>
                Monitor
              </ButtonProgress>
            </Button>
          </div>
          <div className="mt-4 divide-y divide-slate-100">
            {(geoOverview?.external_surfaces ?? []).slice(0, 6).map((surface) => (
              <div key={surface.id} className="py-3">
                <div className="flex min-w-0 items-center justify-between gap-2">
                  <span className="truncate text-sm font-medium text-slate-900">{surface.url}</span>
                  <Badge tone={surface.owner_type === "project" ? "green" : "neutral"}>
                    {surface.owner_type === "project" ? "owned" : surface.owner_type}
                  </Badge>
                </div>
                <div className="mt-2 grid gap-2 text-xs text-slate-500 sm:grid-cols-5">
                  <div>
                    <span className="font-semibold text-slate-700">Platform</span>
                    <br />
                    {surface.platform}
                  </div>
                  <div>
                    <span className="font-semibold text-slate-700">Publication</span>
                    <br />
                    {surface.publication_status}
                  </div>
                  <div>
                    <span className="font-semibold text-slate-700">Indexability</span>
                    <br />
                    {surface.indexability_status}
                  </div>
                  <div>
                    <span className="font-semibold text-slate-700">Canonical</span>
                    <br />
                    {surface.canonical_status}
                  </div>
                  <div>
                    <span className="font-semibold text-slate-700">Confidence</span>
                    <br />
                    {surface.owner_confidence}
                  </div>
                </div>
                {surface.source_url && <div className="mt-2 truncate text-xs text-slate-500">Source URL: {surface.source_url}</div>}
              </div>
            ))}
          </div>
        </div>
        <div className="mt-3 overflow-hidden rounded-lg border border-slate-200 bg-white">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-slate-200 bg-slate-50 text-xs uppercase text-slate-500">
              <tr>
                <th className="px-4 py-3 font-semibold">Asset brief</th>
                <th className="px-4 py-3 font-semibold">Status</th>
                <th className="px-4 py-3 font-semibold">Surface</th>
                <th className="px-4 py-3 font-semibold">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {assetBriefs.slice(0, 8).map((brief) => (
                <tr key={brief.id}>
                  <td className="max-w-[320px] truncate px-4 py-3 font-medium text-slate-900">
                    {brief.target_prompts[0] ?? brief.asset_type}
                  </td>
                  <td className="px-4 py-3">
                    <Badge tone={toneForStatus(brief.status)}>{brief.status}</Badge>
                  </td>
                  <td className="px-4 py-3 text-slate-600">{brief.publication_surface}</td>
                  <td className="px-4 py-3">
                    <Button size="sm" onClick={() => acceptAssetBrief(brief)} disabled={busy === brief.id || !["draft", "ready_for_review", "accepted"].includes(brief.status)}>
                      <ButtonProgress busy={busy === brief.id} busyLabel="Accepting" idleIcon={<FileText size={14} />}>
                        Accept
                      </ButtonProgress>
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {assetBriefs.length === 0 && <div className="p-4"><EmptyState title="No asset briefs" detail="Run analyzer after observations are available." /></div>}
        </div>
      </section>

      <section>
        <SectionHeader
          title="Autopilot"
          action={
            <div className="flex flex-wrap gap-2">
              <Badge tone={policy?.safe_mode_enabled || safeModes.some((event) => !event.exited_at) ? "amber" : "neutral"}>
                L{policy?.autopilot_level ?? 0}
              </Badge>
              <Button size="sm" onClick={generatePlan} disabled={!!busy}>
                <ButtonProgress busy={busy === "plan"} busyLabel="Generating plan" idleIcon={<BarChart3 size={14} />}>
                  Plan
                </ButtonProgress>
              </Button>
              <Button size="sm" onClick={executeLatestPlan} disabled={!!busy || !latestPortfolioPlan || !readiness?.ready_for_level_2}>
                <ButtonProgress busy={busy === "execute-autopilot"} busyLabel="Executing" idleIcon={<CheckCircle2 size={14} />}>
                  Execute guarded actions
                </ButtonProgress>
              </Button>
              <Button size="sm" variant="danger" onClick={enterSafeMode} disabled={!!busy}>
                <ButtonProgress busy={busy === "safe-mode"} busyLabel="Enabling safe mode" idleIcon={<ShieldAlert size={14} />}>
                  Safe mode
                </ButtonProgress>
              </Button>
            </div>
          }
        />
        <div className="grid gap-3 rounded-lg border border-slate-200 bg-white p-4 md:grid-cols-[1fr_1.4fr]">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <div className="text-sm font-bold text-slate-900">Readiness</div>
              <Badge tone={readinessTone}>{readiness?.ready_for_level_2 ? "Ready for Level 2" : "Blocked gates"}</Badge>
            </div>
            <p className="mt-2 text-sm text-slate-500">
              Level 2 only runs low-risk actions after policy, publisher, notification, budget, safe mode, kill switch, and recovery gates pass.
            </p>
          </div>
          <div className="grid gap-2">
            <div className="text-xs font-bold uppercase tracking-wide text-slate-500">Blocked gates</div>
            {blockedReadinessGates.length > 0 ? (
              blockedReadinessGates.slice(0, 4).map((gate) => (
                <div key={gate.key} className="rounded-md border border-slate-200 px-3 py-2 text-sm">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-semibold text-slate-900">{gate.label}</span>
                    <Badge tone={toneForSetupStatus(gate.status)}>{gate.status}</Badge>
                  </div>
                  <p className="mt-1 text-xs leading-5 text-slate-500">{gate.next_action}</p>
                </div>
              ))
            ) : (
              <div className="rounded-md border border-emerald-100 bg-emerald-50 px-3 py-2 text-sm font-semibold text-emerald-700">
                Ready for Level 2
              </div>
            )}
          </div>
        </div>
        <div className="grid gap-3 rounded-lg border border-slate-200 bg-white p-4 md:grid-cols-4">
          <Field label="Level">
            <TextInput
              type="number"
              min={0}
              max={4}
              value={policy?.autopilot_level ?? 0}
              onChange={(event) => savePolicy({ autopilot_level: Math.max(0, Math.min(4, Number(event.target.value || 0))) })}
            />
          </Field>
          <Field label="Weekly limit">
            <TextInput
              type="number"
              min={1}
              value={policy?.weekly_action_limit ?? 5}
              onChange={(event) => savePolicy({ weekly_action_limit: Math.max(1, Number(event.target.value || 5)) })}
            />
          </Field>
          <Field label="Low-clicks">
            <TextInput
              type="number"
              min={0}
              value={policy?.low_traffic_clicks_28d_threshold ?? 10}
              onChange={(event) => savePolicy({ low_traffic_clicks_28d_threshold: Math.max(0, Number(event.target.value || 10)) })}
            />
          </Field>
          <Field label="Low-impressions">
            <TextInput
              type="number"
              min={0}
              value={policy?.low_traffic_impressions_28d_threshold ?? 500}
              onChange={(event) => savePolicy({ low_traffic_impressions_28d_threshold: Math.max(0, Number(event.target.value || 500)) })}
            />
          </Field>
          <label className="flex items-center gap-2 rounded-lg border border-slate-200 px-3 py-2 text-sm font-semibold text-slate-700">
            <input
              type="checkbox"
              checked={Boolean(policy?.kill_switch_enabled)}
              onChange={(event) => savePolicy({ kill_switch_enabled: event.target.checked })}
            />
            Kill switch
          </label>
          <div className="md:col-span-3 flex gap-2">
            <TextInput value={objectiveName} onChange={(event) => setObjectiveName(event.target.value)} placeholder="Grow qualified blog clicks" />
            <Button size="sm" onClick={createObjective} disabled={busy === "objective" || !objectiveName.trim()}>
              <ButtonProgress busy={busy === "objective"} busyLabel="Creating objective" idleIcon={<FileText size={14} />}>
                Objective
              </ButtonProgress>
            </Button>
          </div>
        </div>
        <div className="mt-3 grid gap-3 md:grid-cols-3">
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">{objectives.length}</div>
            <p className="mt-1 text-sm text-slate-500">Objectives</p>
          </div>
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">{plans.length}</div>
            <p className="mt-1 text-sm text-slate-500">Plans</p>
          </div>
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">{safeModes.filter((event) => !event.exited_at).length}</div>
            <p className="mt-1 text-sm text-slate-500">Open safe mode</p>
          </div>
        </div>
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
                  {selectedPortfolioActions(latestPortfolioPlan).slice(0, 8).map((action, index) => (
                    <div key={`${action.opportunity_id ?? index}`} className="py-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge tone="neutral">{action.action_bucket}</Badge>
                        <Badge tone={toneForRisk(action.risk_level)}>{action.risk_level}</Badge>
                        {action.review_required && <Badge tone="amber">Review required</Badge>}
                      </div>
                      <div className="mt-2 text-sm font-semibold text-slate-900">{action.recommended_action ?? action.type}</div>
                      <div className="mt-1 text-xs text-slate-500">Measurement: {measurementLabel(action.measurement_schedule)}</div>
                      <div className="mt-1 text-xs text-slate-500">Recovery plan: Manual rollback required unless publisher rollback is available.</div>
                    </div>
                  ))}
                  {selectedPortfolioActions(latestPortfolioPlan).length === 0 && (
                    <div className="py-3 text-sm text-slate-500">No selected actions in this portfolio.</div>
                  )}
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
                  <div className="flex items-center justify-between border-b border-slate-100 py-2">
                    <span>Review required</span>
                    <span className="font-semibold text-slate-900">{latestPortfolioPlan.portfolio.required_approvals.length}</span>
                  </div>
                  <div className="flex items-center justify-between py-2">
                    <span>Measurement</span>
                    <span className="font-semibold text-slate-900">{latestPortfolioPlan.portfolio.measurement_schedule.length}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}
        {executionResult && (
          <div className="mt-3 rounded-lg border border-slate-200 bg-white p-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <div className="text-sm font-bold text-slate-900">Recovery plan</div>
                <p className="mt-1 text-sm text-slate-500">Guarded execution attached rollback or manual recovery metadata to each executed action.</p>
              </div>
              <Badge tone={executionResult.executed_actions.length > 0 ? "green" : "amber"}>
                {executionResult.executed_actions.length} executed
              </Badge>
            </div>
            <div className="mt-3 grid gap-2 text-sm text-slate-600">
              {latestRecoveryPlans.slice(0, 3).map((plan, index) => (
                <div key={`${plan.action_id ?? index}`} className="rounded-md border border-slate-200 px-3 py-2">
                  <div className="font-semibold text-slate-900">
                    {plan.manual_rollback_required ? "Manual rollback required" : "Publisher rollback available"}
                  </div>
                  <div className="mt-1 text-xs text-slate-500">{Array.isArray(plan.recovery_plan) ? plan.recovery_plan[0] : "Recovery plan recorded."}</div>
                </div>
              ))}
              {latestRecoveryPlans.length === 0 && <div className="text-sm text-slate-500">No executed recovery plans in the latest run.</div>}
            </div>
          </div>
        )}
      </section>
            </div>
          </details>
        </div>
      )}

      {mode === "analysis" && (
      <details className="rounded-xl border border-slate-200 bg-white p-4">
        <summary className="flex cursor-pointer items-center justify-between gap-3 text-sm font-bold text-slate-900">
          <span>{brief?.title ?? "Weekly analysis brief"}</span>
          <Badge tone={brief?.mode === "cold_start" ? "amber" : "green"}>{brief?.mode ?? "loading"}</Badge>
        </summary>
        <div className="mt-4">
        {brief ? (
          <div className="rounded-lg border border-slate-200 bg-slate-50 p-4">
            {brief.blockers.length > 0 && (
              <div className="mb-4 grid gap-2">
                {brief.blockers.map((blocker) => (
                  <Notice key={blocker} title="Blocker" detail={blocker} tone="amber" />
                ))}
              </div>
            )}
            {brief.geo_blockers.length > 0 && (
              <div className="mb-4 grid gap-2">
                {brief.geo_blockers.map((blocker) => (
                  <Notice key={blocker} title="GEO blocker" detail={blocker} tone="amber" />
                ))}
              </div>
            )}
            {brief.geo_opportunities.length > 0 && (
              <div className="mb-4 grid gap-2">
                <div className="text-xs font-semibold uppercase text-slate-500">Top GEO opportunities</div>
                {brief.geo_opportunities.slice(0, 5).map((opp) => (
                  <div key={opp.id} className="rounded-lg border border-slate-100 px-3 py-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone={toneForRisk(opp.risk_level)}>{opp.risk_level ?? "risk"}</Badge>
                      <span className="text-sm font-semibold text-slate-900">{opp.type}</span>
                      <span className="text-xs text-slate-500">{opp.recommended_action ?? "review"}</span>
                    </div>
                    <div className="mt-1 truncate text-sm text-slate-500">{opp.query ?? opp.page_url ?? opp.normalized_page_url}</div>
                  </div>
                ))}
              </div>
            )}
            {brief.actions.length === 0 ? (
              <EmptyState title="No brief actions yet" detail="Sync and analyzer output will appear here after enough data is available." />
            ) : (
              <div className="grid gap-2">
                {brief.actions.slice(0, 10).map((opp) => (
                  <div key={opp.id} className="rounded-lg border border-slate-100 px-3 py-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone={toneForRisk(opp.risk_level)}>{opp.risk_level ?? "risk"}</Badge>
                      <span className="text-sm font-semibold text-slate-900">{opp.type}</span>
                      <span className="text-xs text-slate-500">{opp.recommended_action ?? "review"}</span>
                    </div>
                    <div className="mt-1 truncate text-sm text-slate-500">{opp.page_url ?? opp.normalized_page_url}</div>
                  </div>
                ))}
              </div>
            )}
          </div>
        ) : (
          <EmptyState title="Loading brief" detail="Fetching latest SEO brief." />
        )}
        </div>
      </details>
      )}
    </div>
    {mode === "results" && selectedResultAction && (() => {
      const action = selectedResultAction;
      const state = actionMeasurementState(action);
      const queue = measurementQueueState(action);
      const measurement = latestActionMeasurement(action);
      const before = measurementMetricText(measurement, "before");
      const after = measurementMetricText(measurement, "after");
      const confounders = actionConfounders(action);
      const publishedTitle = resultPublishedArticleTitle(action);
      const publishedURL = resultPublishedArticleUrl(action);
      const sourceURL = resultSourceEvidenceUrl(action);

      return (
        <div className="fixed inset-0 z-30">
          <button
            type="button"
            aria-label="Close attribution details"
            onClick={closeResultDrawer}
            className="absolute inset-0 motion-safe:animate-[citeloop-drawer-scrim-in_180ms_ease-out] bg-slate-950/25"
          />
          <aside
            ref={resultDrawerRef}
            data-results-drawer
            role="dialog"
            aria-modal="true"
            aria-labelledby="result-details-title"
            className="absolute right-0 top-0 flex h-[100dvh] max-h-[100dvh] w-full max-w-2xl motion-safe:animate-[citeloop-drawer-panel-in_220ms_cubic-bezier(0.16,1,0.3,1)] flex-col overflow-hidden border-l border-slate-200 bg-white shadow-2xl"
          >
            <div className="flex items-start justify-between gap-4 border-b border-slate-100 p-5">
              <div className="min-w-0">
                <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Attribution report</div>
                <h3 id="result-details-title" className="mt-2 text-xl font-bold leading-7 text-slate-950">
                  {publishedTitle}
                </h3>
                <p className="mt-2 break-words text-sm leading-5 text-slate-500">
                  {publishedURL || sourceURL || action.id}
                </p>
                {sourceURL && <p className="mt-1 break-words text-xs font-medium leading-5 text-slate-400">Source action: {action.action_type} - {sourceURL}</p>}
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <Badge tone={state.tone}>{state.label}</Badge>
                  <Badge tone={queue.tone}>{queue.label}</Badge>
                  <Badge tone={toneForStatus(action.status)}>{action.status}</Badge>
                </div>
              </div>
              <button
                type="button"
                data-drawer-close
                aria-label="Close attribution details"
                onClick={closeResultDrawer}
                className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-slate-200 text-slate-500 transition hover:bg-slate-50 active:translate-y-px"
              >
                <X size={16} />
              </button>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-5">
              <div className="space-y-5">
                <section className="grid gap-3 text-sm sm:grid-cols-2">
                  <div className="rounded-lg border border-slate-200 p-3">
                    <div className="text-xs font-semibold uppercase text-slate-400">Published / applied</div>
                    <div className="mt-1 font-medium text-slate-700">{formatDate(action.published_at ?? action.verified_at ?? null)}</div>
                  </div>
                  <div className="rounded-lg border border-slate-200 p-3">
                    <div className="text-xs font-semibold uppercase text-slate-400">Measurement window</div>
                    <div className="mt-1 font-medium text-slate-700">{measurementWindowLabel(action.measurement_window)}</div>
                  </div>
                </section>

                <section className="rounded-xl border border-slate-200 p-4">
                  <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Action timeline</div>
                  <div className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Created</div>
                      <div className="mt-1 font-medium text-slate-700">{formatDate(action.created_at ?? null)}</div>
                    </div>
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Approved</div>
                      <div className="mt-1 font-medium text-slate-700">{formatDate(action.approved_at ?? null)}</div>
                    </div>
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Published / applied</div>
                      <div className="mt-1 font-medium text-slate-700">{formatDate(action.published_at ?? action.verified_at ?? null)}</div>
                    </div>
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Verified</div>
                      <div className="mt-1 font-medium text-slate-700">{formatDate(action.verified_at ?? null)}</div>
                    </div>
                  </div>
                </section>

                <section className="grid gap-3 text-sm sm:grid-cols-2">
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">Asset</div>
                    <div className="mt-1 break-words font-medium text-slate-700">{action.asset_type ?? "unspecified"}</div>
                  </div>
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">Review</div>
                    <div className="mt-1 font-medium text-slate-700">{action.review_required === false ? "Optional" : "Required"}</div>
                  </div>
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">Verification</div>
                    <div className="mt-1 font-medium text-slate-700">
                      {action.verified_at ? "Verified" : hasActionVerificationSnapshot(action) ? "Needs check" : "Not started"}
                    </div>
                  </div>
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">Checkpoint state</div>
                    <div className="mt-1 font-medium leading-5 text-slate-700">{queue.detail}</div>
                  </div>
                </section>

                <section className="rounded-xl border border-slate-200 bg-slate-50 p-4">
                  <div className="grid gap-4 text-sm sm:grid-cols-2">
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Why now</div>
                      <div className="mt-1 font-medium leading-5 text-slate-700">{actionWhyNowText(action)}</div>
                    </div>
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">SEO/GEO contribution</div>
                      <div className="mt-1 font-medium leading-5 text-slate-700">{actionSEOContributionText(action)}</div>
                    </div>
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Output type</div>
                      <div className="mt-1 font-medium leading-5 text-slate-700">{actionOutputTypeLabel(action)}</div>
                    </div>
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">After execution</div>
                      <div className="mt-1 font-medium leading-5 text-slate-700">{actionPostExecutionText(action)}</div>
                    </div>
                  </div>
                </section>

                <section className="grid gap-3 text-sm sm:grid-cols-2">
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">Before</div>
                    <div className="mt-1 break-words font-medium text-slate-700">{before}</div>
                  </div>
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">After</div>
                    <div className="mt-1 break-words font-medium text-slate-700">{after}</div>
                  </div>
                  <div className="sm:col-span-2">
                    <div className="text-xs font-semibold uppercase text-slate-400">Outcome reason</div>
                    <div className="mt-1 break-words font-medium leading-5 text-slate-700">{state.detail}</div>
                  </div>
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">Attribution confidence</div>
                    <div className="mt-1 font-medium text-slate-700">{actionAttributionConfidence(action)}</div>
                  </div>
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">Confounders</div>
                    <div className="mt-1 break-words font-medium text-slate-700">{confounders.length ? confounders.slice(0, 3).join(" / ") : "None noted"}</div>
                  </div>
                </section>

                <section className="rounded-xl border border-slate-200 p-4">
                  <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Measurement details</div>
                  <div className="mt-3 grid gap-3 text-sm leading-5 text-slate-600 sm:grid-cols-2">
                    <div>
                      <span className="font-semibold text-slate-800">Outcome</span>
                      <br />
                      {compactOutcomeText(action.outcome_summary)}
                    </div>
                    <div>
                      <span className="font-semibold text-slate-800">Measurement window</span>
                      <br />
                      {measurementWindowLabel(action.measurement_window)}
                    </div>
                    <div>
                      <span className="font-semibold text-slate-800">Verification</span>
                      <br />
                      {action.verified_at ? formatDate(action.verified_at) : hasActionVerificationSnapshot(action) ? compactOutcomeText(action.verification_snapshot) : "Not started"}
                    </div>
                    <div>
                      <span className="font-semibold text-slate-800">Target URL</span>
                      <br />
                      {action.target_url ?? action.normalized_target_url ?? "No target URL yet."}
                    </div>
                  </div>
                </section>
              </div>
            </div>

            <div
              aria-label="Drawer actions"
              className="shrink-0 flex flex-col gap-2 border-t border-slate-200 bg-white px-4 pb-[calc(1.5rem+env(safe-area-inset-bottom))] pt-4 sm:flex-row sm:justify-end"
            >
              <Button size="sm" onClick={() => verifyAction(action, "verified")} disabled={busy === `verify-${action.id}-verified`}>
                <ButtonProgress busy={busy === `verify-${action.id}-verified`} busyLabel="Verifying" idleIcon={<CheckCircle2 size={14} />}>
                  Manual verify
                </ButtonProgress>
              </Button>
              <Button size="sm" variant="danger" onClick={() => verifyAction(action, "failed")} disabled={busy === `verify-${action.id}-failed`}>
                <ButtonProgress busy={busy === `verify-${action.id}-failed`} busyLabel="Marking failed" idleIcon={null}>
                  Verification failed
                </ButtonProgress>
              </Button>
            </div>
          </aside>
        </div>
      );
    })()}
    {mode === "analysis" && selectedOpportunity && (() => {
      const addingToPlan = createActionBusy(selectedOpportunity);
      const dismissingOpportunity = dismissBusy(selectedOpportunity);
      const snoozingOpportunity = opportunityBusy[selectedOpportunity.id] === "snooze";
      const watchingOpportunity = opportunityBusy[selectedOpportunity.id] === "watch";
      const reviewingOpportunity = addingToPlan || dismissingOpportunity || snoozingOpportunity || watchingOpportunity;
      const recommendedWorkType = opportunityWorkType(selectedOpportunity);
      const allowedWorkTypes = allowedWorkTypesForOpportunity(selectedOpportunity);
      const workType = routeOverrides[selectedOpportunity.id] ?? recommendedWorkType;
      const destination = destinationForWorkType(workType);
      const cta = ctaForWorkType(workType);
      const evidence = selectedOpportunity.evidence;
      const dataSourceNotes =
        evidence && typeof evidence === "object" && !Array.isArray(evidence) && "data_source_notes" in evidence
          ? String((evidence as Record<string, any>).data_source_notes)
          : "No additional data notes.";

      return (
        <div className="fixed inset-0 z-30">
          <button
            type="button"
            aria-label="Close finding details"
            onClick={() => setSelectedOpportunityID(null)}
            className="absolute inset-0 motion-safe:animate-[citeloop-drawer-scrim-in_180ms_ease-out] bg-slate-950/25"
          />
          <aside
            ref={analysisDrawerRef}
            data-analysis-drawer
            role="dialog"
            aria-modal="true"
            aria-labelledby="opportunity-details-title"
            className="absolute right-0 top-0 flex h-[100dvh] max-h-[100dvh] w-full max-w-2xl motion-safe:animate-[citeloop-drawer-panel-in_220ms_cubic-bezier(0.16,1,0.3,1)] flex-col overflow-hidden border-l border-slate-200 bg-white shadow-2xl"
          >
            <div className="flex items-start justify-between gap-4 border-b border-slate-100 p-5">
              <div className="min-w-0">
                <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Opportunity details</div>
                <h3 id="opportunity-details-title" className="mt-2 text-xl font-bold leading-7 text-slate-950">
                  {opportunityTitle(selectedOpportunity)}
                </h3>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <Badge tone="blue">{workType}</Badge>
                  <Badge tone="neutral">{destination}</Badge>
                  <Badge tone="red">Needs decision</Badge>
                </div>
              </div>
              <button
                type="button"
                data-drawer-close
                aria-label="Close finding details"
                onClick={() => setSelectedOpportunityID(null)}
                className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-slate-200 text-slate-500 transition hover:bg-slate-50 active:translate-y-px"
              >
                <X size={16} />
              </button>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-5">
              <div className="space-y-5">
                <section className="rounded-xl border border-slate-200 bg-slate-50 p-4">
                  <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Next step</div>
                  <p className="mt-2 text-sm font-semibold leading-6 text-slate-900">
                    {approvalCopyForWorkType(workType)}
                  </p>
                  <p className="mt-1 text-sm leading-6 text-slate-600">
                    {cta.label} keeps this work in the right queue instead of creating a generic task.
                  </p>
                </section>

                <section className="rounded-xl border border-slate-200 p-4">
                  <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Work type</div>
                  {allowedWorkTypes.length > 1 ? (
                    <>
                      <div className="mt-2 flex flex-wrap gap-2" role="group" aria-label="Choose work type">
                        {allowedWorkTypes.map((option) => (
                          <button
                            key={option}
                            type="button"
                            data-work-type-option
                            aria-pressed={option === workType}
                            onClick={() =>
                              setRouteOverrides((current) => ({ ...current, [selectedOpportunity.id]: option }))
                            }
                            className={cx(
                              "rounded-lg border px-3 py-2 text-sm font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820]",
                              option === workType
                                ? "border-slate-900 bg-slate-900 text-white"
                                : "border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50",
                            )}
                          >
                            {option}
                            {option === recommendedWorkType ? " · Recommended" : ""}
                          </button>
                        ))}
                      </div>
                      <p className="mt-2 text-xs leading-5 text-slate-500">
                        {workType === recommendedWorkType
                          ? "System recommendation. Switch the work type if this fits another queue better."
                          : "You changed the route. The CTA and destination follow your choice."}
                      </p>
                    </>
                  ) : (
                    <p className="mt-2 text-sm leading-6 text-slate-700">
                      {workType} · {workTypeLockReason(selectedOpportunity)}
                    </p>
                  )}
                </section>

                <section className="rounded-xl border border-slate-200 p-4">
                  <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Opportunity timeline</div>
                  <div className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Detected</div>
                      <div className="mt-1 font-medium text-slate-700">{formatDate(selectedOpportunity.created_at ?? null)}</div>
                    </div>
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Last updated</div>
                      <div className="mt-1 font-medium text-slate-700">{formatDate(selectedOpportunity.updated_at ?? null)}</div>
                    </div>
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Snoozed until</div>
                      <div className="mt-1 font-medium text-slate-700">{formatDate(selectedOpportunity.snoozed_until ?? null)}</div>
                    </div>
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Unsnoozed</div>
                      <div className="mt-1 font-medium text-slate-700">{formatDate(selectedOpportunity.unsnoozed_at ?? null)}</div>
                    </div>
                  </div>
                </section>

                <section className="grid gap-3 text-sm sm:grid-cols-3">
                  <div className="rounded-lg border border-slate-200 p-3">
                    <div className="text-xs font-semibold uppercase text-slate-400">Work type</div>
                    <div className="mt-1 font-medium leading-5 text-slate-700">{workType}</div>
                  </div>
                  <div className="rounded-lg border border-slate-200 p-3">
                    <div className="text-xs font-semibold uppercase text-slate-400">Destination</div>
                    <div className="mt-1 font-medium leading-5 text-slate-700">{destination}</div>
                  </div>
                  <div className="rounded-lg border border-slate-200 p-3">
                    <div className="text-xs font-semibold uppercase text-slate-400">Approval source</div>
                    <div className="mt-1 font-medium leading-5 text-slate-700">Human opportunity approval</div>
                  </div>
                </section>

                <section className="rounded-xl border border-slate-200 p-4">
                  <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Expected impact</div>
                  <p className="mt-2 text-sm leading-6 text-slate-700">
                    {selectedOpportunity.expected_impact || "Review this opportunity against confirmed Context before creating downstream work."}
                  </p>
                </section>

                <section className="grid gap-3 sm:grid-cols-2">
                  <div className="rounded-lg border border-slate-200 p-3">
                    <div className="text-xs font-semibold uppercase text-slate-400">Score</div>
                    <div className="mt-2 font-mono text-2xl font-bold text-slate-950">{metric(selectedOpportunity.priority_score)}</div>
                  </div>
                  <div className="rounded-lg border border-slate-200 p-3">
                    <div className="text-xs font-semibold uppercase text-slate-400">Confidence</div>
                    <div className="mt-2 font-mono text-2xl font-bold text-slate-950">{metric(selectedOpportunity.confidence, 2)}</div>
                  </div>
                </section>

                <section className="grid gap-3 text-sm sm:grid-cols-2">
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">Query</div>
                    <div className="mt-1 break-words font-medium text-slate-700">{selectedOpportunity.query ?? "Not query-specific"}</div>
                  </div>
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">Effort</div>
                    <div className="mt-1 font-medium text-slate-700">{selectedOpportunity.effort ?? "Unknown"}</div>
                  </div>
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">Risk</div>
                    <div className="mt-1 font-medium text-slate-700">{selectedOpportunity.risk_level ?? "Unknown"}</div>
                  </div>
                  <div>
                    <div className="text-xs font-semibold uppercase text-slate-400">Evidence source</div>
                    <div className="mt-1 font-medium text-slate-700">{sourceModeForOpportunity(selectedOpportunity, overview)}</div>
                  </div>
                  <div className="sm:col-span-2">
                    <div className="text-xs font-semibold uppercase text-slate-400">Source</div>
                    <div className="mt-1 break-words font-medium text-slate-700">
                      {selectedOpportunity.page_url ?? selectedOpportunity.normalized_page_url ?? "Project domain"}
                    </div>
                  </div>
                  <div className="sm:col-span-2">
                    <div className="text-xs font-semibold uppercase text-slate-400">Opportunity type</div>
                    <div className="mt-1 break-words font-medium text-slate-700">{humanizeInternalType(selectedOpportunity.type)}</div>
                  </div>
                </section>

                <section className="rounded-xl border border-slate-200 p-4">
                  <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Evidence</div>
                  <p className="mt-2 text-sm leading-6 text-slate-700">{compactEvidenceText(selectedOpportunity.evidence)}</p>
                  <div className="mt-4 border-t border-slate-100 pt-3">
                    <div className="text-xs font-semibold uppercase text-slate-400">Data notes</div>
                    <p className="mt-1 text-sm leading-6 text-slate-600">{dataSourceNotes}</p>
                  </div>
                </section>
              </div>
            </div>

            <div
              aria-label="Drawer actions"
              className="shrink-0 flex flex-col gap-3 border-t border-slate-200 bg-white px-4 pb-[calc(1.5rem+env(safe-area-inset-bottom))] pt-4"
            >
              <div className="flex flex-wrap items-center gap-2" aria-label="Wait instead of approving">
                <span className="text-xs font-semibold uppercase tracking-[0.1em] text-slate-400">Not now:</span>
                {[7, 14, 30].map((days) => (
                  <Button key={days} size="sm" variant="ghost" onClick={() => snoozeOpportunity(selectedOpportunity, days)} disabled={reviewingOpportunity}>
                    <ButtonProgress busy={snoozingOpportunity} busyLabel="Snoozing" idleIcon={null}>
                      Snooze {days}d
                    </ButtonProgress>
                  </Button>
                ))}
                <Button size="sm" variant="ghost" onClick={() => watchOpportunity(selectedOpportunity)} disabled={reviewingOpportunity}>
                  <ButtonProgress busy={watchingOpportunity} busyLabel="Adding to watchlist" idleIcon={null}>
                    Watch in Results
                  </ButtonProgress>
                </Button>
              </div>
              <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
                <Button size="sm" variant="ghost" onClick={() => dismiss(selectedOpportunity)} disabled={reviewingOpportunity}>
                  <ButtonProgress busy={dismissingOpportunity} busyLabel="Dismissing" idleIcon={null}>
                    Dismiss finding
                  </ButtonProgress>
                </Button>
                <Button size="sm" variant="primary" onClick={() => createAction(selectedOpportunity)} disabled={reviewingOpportunity}>
                  <ButtonProgress busy={addingToPlan} busyLabel={cta.busyLabel} idleIcon={<FileText size={14} />}>
                    {cta.label}
                  </ButtonProgress>
                </Button>
              </div>
            </div>
          </aside>
        </div>
      );
    })()}
    </>
  );
}
