"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { BarChart3, CheckCircle2, FileText, RefreshCw, Search, Settings, ShieldAlert, X } from "lucide-react";
import {
  ActionMeasurement,
  AICrawlerAccessSnapshot,
  AutopilotExecuteResult,
  AutopilotReadiness,
  GEOAssetBrief,
  GEOCompetitor,
  GEOOverview,
  GEOPrompt,
  SEOActionPlan,
  SEOBrief,
  SEOContentAction,
  SEOObjective,
  SEOOpportunity,
  SEOOverview,
  SEOPolicy,
  SafeModeEvent,
  ResultsAction,
  VisibilitySummary,
} from "../../../lib/api";
import { visibilityLifecycleLabel } from "../../../lib/dashboard-ux-logic";
import { normalizeNumeric } from "../../../lib/normalize";
import { deriveVisibilityLifecycleStage, visibilityLifecycleCounts } from "../../../lib/visibility-lifecycle";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";
import { Badge, Button, ButtonProgress, EmptyState, Field, Notice, SectionHeader, TextInput, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
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

function toneForStatus(status: string): "green" | "amber" | "red" | "neutral" {
  if (["open", "ready_for_review", "approved", "measuring", "ok", "connected"].includes(status)) return "green";
  if (["degraded", "accepted", "converted", "drafting"].includes(status)) return "amber";
  if (["error", "failed", "expired"].includes(status)) return "red";
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

function capabilityLabel(mode?: string) {
  if (mode === "customer_site_connected" || mode === "managed_content_connected") return "Connected";
  if (mode === "customer_site_pending_verification") return "Limited";
  return "Public crawl only";
}

function opportunityTitle(opportunity: SEOOpportunity) {
  return opportunity.recommended_action || opportunity.query || opportunity.page_url || opportunity.type || "Visibility opportunity";
}

function assetTypeForOpportunity(opportunity: SEOOpportunity) {
  const text = `${opportunity.type} ${opportunity.recommended_action ?? ""}`.toLowerCase();
  if (text.includes("schema")) return "schema_patch";
  if (text.includes("internal link")) return "internal_link_patch";
  if (text.includes("metadata") || text.includes("title") || text.includes("meta")) return "metadata_rewrite";
  if (text.includes("sitemap")) return "sitemap_update";
  if (text.includes("robots") || text.includes("canonical") || text.includes("crawler") || text.includes("technical")) return "technical_fix";
  if (text.includes("geo") || text.includes("citation") || text.includes("answer engine")) return "glossary_definition";
  if (text.includes("comparison")) return "comparison_page";
  if (text.includes("alternative")) return "alternative_page";
  if (text.includes("template") || text.includes("checklist")) return "template_or_checklist";
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

function measurementWindowLabel(measurement_window: any) {
  const structured = Array.isArray(measurement_window?.checkpoints)
    ? measurement_window.checkpoints.map((checkpoint: any) => checkpoint?.day).filter(Boolean)
    : [];
  const legacy = Array.isArray(measurement_window?.checkpoints_days) ? measurement_window.checkpoints_days : [];
  const checkpoints: Array<number | string> = structured.length > 0 ? structured : legacy;
  if (checkpoints.length === 0) return "Not scheduled";
  const metric = measurement_window?.primary_metric ? `${measurement_window.primary_metric}: ` : "";
  return `Scheduled: ${metric}${checkpoints.map((day) => `D+${day}`).join(" / ")}`;
}

function analysisSearchDataStatus(overview: SEOOverview | null, gscStatus: string) {
  const capabilityMode = overview?.capability_mode ?? "public_only";
  const integration = overview?.integrations.find((item) => item.provider === "google_search_console");
  if (integration?.status === "connected" || capabilityMode === "customer_site_connected" || capabilityMode === "managed_content_connected") {
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

function searchDataModeLabel(overview: SEOOverview | null, status: ReturnType<typeof analysisSearchDataStatus>) {
  if (status.label === "Backfilling Search Console") return "Backfilling Search Console";
  if (status.tone === "green" && overview?.cold_start) return "Low click depth";
  if (status.tone === "green") return "First-party search data";
  if (status.label === "Search data is stale") return "Last-known search data";
  return "Public crawl only";
}

function analysisCapabilityBadgeLabel(overview: SEOOverview | null, analysisStatus: ReturnType<typeof analysisSearchDataStatus>, visibilityMode: string) {
  if (analysisStatus.tone === "green" && overview?.cold_start) return "Connected, low data";
  if (analysisStatus.tone === "green") return "Connected";
  return capabilityLabel(visibilityMode);
}

function findingTypeLabel(opportunity: SEOOpportunity) {
  const text = `${opportunity.type} ${opportunity.recommended_action ?? ""}`.toLowerCase();
  if (text.includes("ctr") || text.includes("title") || text.includes("meta")) return "CTR opportunity";
  if (text.includes("decay") || text.includes("refresh")) return "Refresh candidate";
  if (text.includes("near") || text.includes("page_one") || text.includes("ranking")) return "Striking distance";
  if (text.includes("index") || text.includes("sitemap") || text.includes("robots") || text.includes("crawler")) return "Technical finding";
  if (text.includes("geo") || text.includes("citation")) return "AI citation gap";
  if (text.includes("competitive") || text.includes("comparison") || text.includes("alternative")) return "Market gap";
  if (text.includes("cold_start")) return "Cold-start finding";
  return "Growth finding";
}

function actionCtaForOpportunity(opportunity: SEOOpportunity) {
  const text = `${opportunity.type} ${opportunity.recommended_action ?? ""} ${opportunity.expected_impact ?? ""}`.toLowerCase();
  if (text.includes("internal link")) {
    return { label: "Create internal-link task", busyLabel: "Creating task" };
  }
  if (text.includes("geo") || text.includes("citation") || text.includes("answer engine")) {
    return { label: "Create GEO asset task", busyLabel: "Creating task" };
  }
  if (text.includes("index") || text.includes("sitemap") || text.includes("schema") || text.includes("crawler") || text.includes("robots") || text.includes("canonical")) {
    return { label: "Create technical task", busyLabel: "Creating task" };
  }
  if (text.includes("refresh") || text.includes("decay") || text.includes("ctr") || text.includes("title") || text.includes("meta") || text.includes("near")) {
    return { label: "Create refresh task", busyLabel: "Creating task" };
  }
  if (text.includes("watch") || text.includes("wait") || text.includes("monitor")) {
    return { label: "Watch", busyLabel: "Adding watch" };
  }
  return { label: "Create content task", busyLabel: "Creating task" };
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

function actionWhyNowText(action: SEOContentAction | ResultsAction) {
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

function actionSEOContributionText(action: SEOContentAction | ResultsAction) {
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

function actionOutputTypeLabel(action: SEOContentAction | ResultsAction) {
  const outputType = String(action.output_snapshot?.output_type ?? action.diff_snapshot?.output_type ?? "").toLowerCase();
  if (outputType === "direct_patch") return "Direct patch";
  if (outputType === "technical_task") return "Technical task";
  const assetType = String(action.asset_type ?? "").toLowerCase();
  if (assetType.includes("patch") || assetType === "metadata_rewrite") return "Direct patch";
  if (assetType === "sitemap_update" || assetType === "technical_fix") return "Technical task";
  return "Topic-backed asset";
}

function actionPostExecutionText(action: SEOContentAction | ResultsAction) {
  if (action.status === "completed") return "Measurement complete";
  if (action.status === "measuring") return "Measuring impact";
  if (action.verified_at) return "Applied or published and verified";
  if (action.status === "approved") return "Approved for execution";
  if (action.status === "ready_for_review") return "Waiting for review";
  if (action.published_at) return "Published and waiting for verification";
  return action.status || "Queued";
}

type ActionMeasurementKey = "waiting" | "positive" | "negative" | "inconclusive" | "insufficient_data";
type ActionMeasurementState = {
  key: ActionMeasurementKey;
  label: "Waiting" | "Positive" | "Negative" | "Inconclusive" | "Insufficient data";
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
    ["published", "measuring", "completed", "failed", "verification_failed", "recovery_required"].includes(action.status) ||
    Boolean(action.published_at || action.verified_at || action.verification_snapshot);
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
  if (["inconclusive", "neutral", "flat"].includes(rawResult) || action.status === "completed") {
    return { key: "inconclusive", label: "Inconclusive", tone: "amber", detail: actionOutcomeReason(action, "The measurement window closed without a clear positive or negative signal.") };
  }
  if (!hasMeasurementSignal) {
    return { key: "waiting", label: "Waiting", tone: "neutral", detail: "Action is waiting for publish or URL verification before measurement starts." };
  }
  return { key: "waiting", label: "Waiting", tone: "neutral", detail: "Published work is still inside the measurement window." };
}

function lifecycleStageLabel(stage: string) {
  switch (stage) {
    case "detected":
      return "Detected";
    case "added_to_plan":
      return "Added";
    case "planned":
      return "Planned";
    case "drafting":
      return "Drafting";
    case "ready_for_review":
      return "Review";
    case "approved":
      return "Approved";
    case "published_or_applied":
      return "Published";
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

function lifecycleStageTone(stage: string): "green" | "amber" | "red" | "neutral" {
  if (["learned", "published_or_applied", "measuring"].includes(stage)) return "green";
  if (["added_to_plan", "planned", "drafting", "ready_for_review", "approved"].includes(stage)) return "amber";
  if (stage === "blocked") return "red";
  return "neutral";
}

function loopActionTitle(action: SEOContentAction & { opportunity_recommended_action?: string | null; opportunity_query?: string | null; topic_title?: string | null }) {
  return action.topic_title || action.opportunity_recommended_action || action.opportunity_query || action.action_type || "Visibility action";
}

function compactOutcomeText(outcome: any) {
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

function GSCStatusMenu({
  projectId,
  overview,
  status,
  gscStatus,
  busy,
  onConnect,
}: {
  projectId: string;
  overview: SEOOverview | null;
  status: ReturnType<typeof analysisSearchDataStatus>;
  gscStatus: string;
  busy: string | null;
  onConnect: () => void;
}) {
  const compact = compactGSCStatus(status);
  const propertyLabel = overview?.property?.gsc_site_url ?? overview?.property?.site_url ?? "Select property";
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

type SEOClientMode = "analysis" | "results";

export function AnalysisClient({ projectId }: { projectId: string }) {
  return <SEOClient projectId={projectId} mode="analysis" />;
}

export function ResultsClient({ projectId }: { projectId: string }) {
  return <SEOClient projectId={projectId} mode="results" />;
}

export function SEOClient({ projectId, mode = "analysis" }: { projectId: string; mode?: SEOClientMode }) {
  const api = useApi();
  const [overview, setOverview] = useState<SEOOverview | null>(null);
  const [visibilitySummary, setVisibilitySummary] = useState<VisibilitySummary | null>(null);
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
  const [opportunityBusy, setOpportunityBusy] = useState<Record<string, "create" | "dismiss">>({});
  const [selectedOpportunityID, setSelectedOpportunityID] = useState<string | null>(null);
  const analysisSurfaceRef = useRef<HTMLDivElement | null>(null);
  const analysisDrawerRef = useRef<HTMLElement | null>(null);
  const analysisReturnFocusRef = useRef<HTMLElement | null>(null);
  const selectedOpportunity = useMemo(
    () => opportunities.find((opp) => opp.id === selectedOpportunityID) ?? null,
    [opportunities, selectedOpportunityID],
  );
  const { notify } = useToast();
  const setMessage = (next: Message) => {
    if (next) notify(next);
  };

  const refresh = useCallback(async () => {
    setMessage(null);
    try {
      const [overviewData, summaryData, settings, briefData, opps, actionRows, resultsRows, policyData, readinessData, objectiveRows, planRows, safeModeRows, crawlerAudit, geoData, briefRows] = await Promise.all([
        api.getSEOOverview(projectId),
        api.getVisibilitySummary(projectId),
        api.getSEOSettings(projectId),
        api.getSEOBrief(projectId),
        api.listSEOOpportunities(projectId, { status: "open", limit: 50 }),
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
      ]);
      setOverview(overviewData);
      setVisibilitySummary(summaryData);
      setBrief(briefData);
      setOpportunities(opps);
      setActions(actionRows);
      setResultsActions(resultsRows);
      setPolicy(policyData);
      setReadiness(readinessData);
      setObjectives(objectiveRows);
      setPlans(planRows);
      setSafeModes(safeModeRows);
      setCrawlerSnapshots(crawlerAudit.snapshots);
      setGeoOverview(geoData);
      setAssetBriefs(briefRows);
      setSiteURL(settings.property?.site_url ?? overviewData.property?.site_url ?? "");
    } catch (e: any) {
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
    return overview?.integrations.find((integration) => integration.provider === "google_search_console")?.status ?? "missing";
  }, [overview]);

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
  const visibilityMode = overview?.capability_mode ?? "public_only";
  const visibilityBlockers = [
    ...(overview?.data_source_warnings ?? []),
    ...(brief?.blockers ?? []),
    ...(brief?.geo_blockers ?? []),
  ].slice(0, 4);
  const analysisStatus = analysisSearchDataStatus(overview, gscStatus);
  const crawlerOkCount = crawlerSnapshots.filter((snapshot) => snapshot.access_state === "ok").length;
  const latestPortfolioPlan = plans[0] ?? null;
  const readinessGates = readiness?.gates ?? [];
  const blockedReadinessGates = readinessGates.filter((gate) => gate.blocking);
  const readinessTone = readiness?.ready_for_level_2 ? "green" : readiness ? "amber" : "neutral";
  const latestRecoveryPlans = executionResult?.recovery_plans ?? [];
  const summaryLoopActions = visibilitySummary?.actions_in_loop ?? [];
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
  const measuredActions = loopActions.filter((action) =>
    ["published", "measuring", "completed", "failed", "verification_failed", "recovery_required"].includes(action.status) ||
    Boolean(action.published_at || action.verified_at),
  );
  const resultActions = loopActions.filter((action) => !["archived", "dismissed"].includes(action.status));
  const attributionActions = resultsActions.length ? resultsActions.filter((action) => !["archived", "dismissed"].includes(action.status)) : resultActions;
  const attributionMeasuredActions = resultsActions.length
    ? resultsActions.filter(
        (action) =>
          ["published", "measuring", "completed", "failed", "verification_failed", "recovery_required"].includes(action.status) ||
          Boolean(action.published_at || action.verified_at),
      )
    : measuredActions;
  const outcomeCounts = attributionMeasuredActions.reduce(
    (counts, action) => {
      counts[actionMeasurementState(action).key] += 1;
      return counts;
    },
    { waiting: 0, positive: 0, negative: 0, inconclusive: 0, insufficient_data: 0 },
  );
  const measurementExceptions = attributionMeasuredActions.filter((action) => ["negative", "inconclusive", "insufficient_data"].includes(actionMeasurementState(action).key));
  const summaryLifecycleCounts = visibilitySummary?.lifecycle_counts;
  const loopLifecycleCounts = visibilityLifecycleCounts(loopActions);
  loopLifecycleCounts.detected = opportunities.length || summaryLifecycleCounts?.detected || 0;
  const loopActiveCount =
    loopLifecycleCounts.added_to_plan +
    loopLifecycleCounts.planned +
    loopLifecycleCounts.drafting +
    loopLifecycleCounts.ready_for_review +
    loopLifecycleCounts.approved +
    loopLifecycleCounts.published_or_applied +
    loopLifecycleCounts.measuring;
  const loopSummaryItems = [
    { key: "added_to_plan", label: "Added", value: loopLifecycleCounts.added_to_plan },
    { key: "planned", label: "Planned", value: loopLifecycleCounts.planned },
    { key: "drafting", label: "Drafting", value: loopLifecycleCounts.drafting },
    { key: "ready_for_review", label: "Review", value: loopLifecycleCounts.ready_for_review },
    { key: "published_or_applied", label: "Published", value: loopLifecycleCounts.published_or_applied },
    { key: "measuring", label: "Measuring", value: loopLifecycleCounts.measuring },
    { key: "learned", label: "Learned", value: loopLifecycleCounts.learned },
    { key: "blocked", label: "Blocked", value: loopLifecycleCounts.blocked },
  ];
  const loopPreviewActions = loopActions
    .filter((action) => !["learned"].includes(deriveVisibilityLifecycleStage(action)))
    .slice(0, 3);
  const analysisDataMode = searchDataModeLabel(overview, analysisStatus);
  const analysisCapabilityMode = analysisCapabilityBadgeLabel(overview, analysisStatus, visibilityMode);
  const searchSnapshotCards = [
    { label: "Clicks", value: metric(overview?.last_28_days?.clicks_28d), detail: "Last 28 days" },
    { label: "Impressions", value: metric(overview?.last_28_days?.impressions_28d), detail: "Last 28 days" },
    { label: "CTR", value: percent(overview?.last_28_days?.ctr_28d), detail: "Average click-through" },
    { label: "Position", value: metric(overview?.last_28_days?.position_28d, 1), detail: "Average rank" },
    { label: "Observed days", value: metric(overview?.last_28_days?.gsc_days_28d), detail: analysisDataMode },
  ];

  function createActionBusy(opp: SEOOpportunity) {
    return opportunityBusy[opp.id] === "create";
  }

  function dismissBusy(opp: SEOOpportunity) {
    return opportunityBusy[opp.id] === "dismiss";
  }

  function setOpportunityPending(id: string, value: "create" | "dismiss" | null) {
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
      const action = await api.createSEOContentAction(projectId, opp.id, {
        action_type: opp.recommended_action ?? undefined,
        asset_type: assetTypeForOpportunity(opp),
      });
      setOpportunities((current) => current.filter((item) => item.id !== opp.id));
      setActions((current) => [action, ...current.filter((item) => item.id !== action.id)]);
      setSelectedOpportunityID(null);
      setMessage({ title: "Visibility action created", detail: opp.recommended_action ?? opp.type, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not create action", detail: e.message, tone: "red" });
    } finally {
      setOpportunityPending(opp.id, null);
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
    <div ref={mode === "analysis" ? analysisSurfaceRef : undefined} className="space-y-7">
      <SectionHeader
        title={mode === "analysis" ? "Review analysis" : "Results"}
        eyebrow={mode === "analysis" ? "Analyze opportunities" : "Measurement and diagnostics"}
        action={
          <div className="flex flex-wrap gap-2">
            {mode === "analysis" && (
              <GSCStatusMenu
                projectId={projectId}
                overview={overview}
                status={analysisStatus}
                gscStatus={gscStatus}
                busy={busy}
                onConnect={startSearchConsoleOAuth}
              />
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
          <section className="rounded-xl border border-slate-200 bg-white p-4">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
              <div>
                <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Search performance snapshot</div>
                <h3 className="mt-2 text-xl font-bold leading-7 text-slate-950">Search signals for the next growth move</h3>
                <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-600">
                  {analysisStatus.detail}
                </p>
              </div>
              <div className="flex flex-wrap gap-2 lg:justify-end">
                <Badge tone={analysisStatus.tone}>{analysisDataMode}</Badge>
                <Badge tone={analysisStatus.tone === "green" && overview?.cold_start ? "amber" : analysisStatus.tone}>
                  {analysisCapabilityMode}
                </Badge>
              </div>
            </div>
            <div className="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
              {searchSnapshotCards.map((card) => (
                <div key={card.label} className="rounded-lg border border-slate-100 bg-slate-50 px-3 py-3">
                  <div className="text-[11px] font-semibold uppercase tracking-[0.12em] text-slate-400">{card.label}</div>
                  <div className="mt-2 font-mono text-xl font-bold text-slate-950">{card.value}</div>
                  <div className="mt-1 text-xs leading-5 text-slate-500">{card.detail}</div>
                </div>
              ))}
            </div>
          </section>

          <section data-analysis-growth-findings-section className="space-y-3">
            <SectionHeader
              title="Growth findings"
              eyebrow="Decision-ready recommendations"
              action={
                <div className="flex flex-wrap gap-2">
                  <Badge tone={opportunities.length ? "green" : "neutral"}>{opportunities.length ? "Ready to review" : "No review needed"}</Badge>
                  <Badge tone="neutral">{loopActiveCount} in loop</Badge>
                </div>
              }
            />

            <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
              <div className="min-w-0">
                {opportunities.length === 0 ? (
                  <EmptyState
                    title="No analysis to review"
                    detail="Refresh or Sync after Context changes. New findings will appear here when they need a decision."
                  />
                ) : (
                  <div className="grid gap-3 lg:grid-cols-2">
                    {opportunities.slice(0, 12).map((opp) => {
                      const cta = actionCtaForOpportunity(opp);
                      return (
                        <button
                          data-analysis-finding-card
                          key={opp.id}
                          type="button"
                          onClick={(event) => {
                            analysisReturnFocusRef.current = event.currentTarget;
                            setSelectedOpportunityID(opp.id);
                          }}
                          aria-label={`Open finding details: ${opportunityTitle(opp)}`}
                          className={`group min-w-0 rounded-xl border bg-white p-4 text-left shadow-sm transition hover:-translate-y-0.5 hover:border-slate-300 hover:shadow-md active:translate-y-0 ${
                            selectedOpportunityID === opp.id ? "border-slate-400 ring-2 ring-slate-200" : "border-slate-200"
                          }`}
                        >
                          <div className="flex flex-wrap items-center gap-2">
                            <Badge tone="blue">{findingTypeLabel(opp)}</Badge>
                            <Badge tone={toneForRisk(opp.risk_level)}>{opp.risk_level ?? "risk unknown"}</Badge>
                            <Badge tone="neutral">{sourceModeForOpportunity(opp, overview)}</Badge>
                          </div>
                          <div className="mt-3 flex items-start justify-between gap-3">
                            <h3 className="min-w-0 text-base font-bold leading-6 text-slate-950">{opportunityTitle(opp)}</h3>
                            <span className="shrink-0 font-mono text-xs font-bold uppercase text-slate-400">Score {metric(opp.priority_score)}</span>
                          </div>
                          <p className="mt-2 line-clamp-3 text-sm leading-6 text-slate-600">
                            {opp.expected_impact || "Review this finding against confirmed Context before creating downstream work."}
                          </p>
                          <div className="mt-4 grid gap-2 border-t border-slate-100 pt-3 text-xs leading-5 text-slate-500">
                            <div className="min-w-0 truncate">
                              <span className="font-semibold uppercase tracking-[0.1em] text-slate-400">Signal</span>{" "}
                              <span className="font-medium text-slate-700">{opp.query ?? opp.page_url ?? opp.normalized_page_url ?? "Project domain"}</span>
                            </div>
                            <div className="flex items-center justify-between gap-3">
                              <span className="truncate font-medium text-slate-500">{cta.label}</span>
                              <span className="font-semibold text-slate-700 transition group-hover:translate-x-0.5">Open details</span>
                            </div>
                          </div>
                        </button>
                      );
                    })}
                  </div>
                )}
              </div>

              <aside className="space-y-4">
                <div className="rounded-xl border border-slate-200 bg-white p-4">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Loop in motion</div>
                      <div className="mt-1 text-lg font-bold text-slate-950">Analysis already in execution</div>
                    </div>
                    <Badge tone={loopActiveCount ? "amber" : "neutral"}>{loopActiveCount}</Badge>
                  </div>
                  <div className="mt-3 grid grid-cols-2 gap-2 text-sm">
                    {loopSummaryItems.filter((item) => item.value > 0).slice(0, 6).map((item) => (
                      <div key={item.key} className="border-t border-slate-100 pt-2">
                        <div className="font-mono text-xl font-bold text-slate-950">{item.value}</div>
                        <div className="mt-1 text-xs font-medium text-slate-500">{item.label}</div>
                      </div>
                    ))}
                    {loopSummaryItems.every((item) => item.value === 0) && (
                      <div className="col-span-2 border-t border-slate-100 pt-3 text-sm leading-6 text-slate-500">
                        Reviewed analysis will appear here after it enters the content loop.
                      </div>
                    )}
                  </div>
                  {loopPreviewActions.length > 0 && (
                    <div className="mt-3 divide-y divide-slate-100 border-y border-slate-100">
                      {loopPreviewActions.map((action) => {
                        const stage = deriveVisibilityLifecycleStage(action);
                        return (
                          <div key={action.id} className="py-2">
                            <div className="flex items-start justify-between gap-2">
                              <div className="min-w-0">
                                <div className="truncate text-sm font-semibold text-slate-900">{loopActionTitle(action)}</div>
                                <div className="mt-1 truncate text-xs text-slate-500">{action.target_url ?? action.normalized_target_url ?? action.opportunity_page_url ?? action.id}</div>
                              </div>
                              <Badge tone={lifecycleStageTone(stage)}>{lifecycleStageLabel(stage)}</Badge>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  )}
                  <Link
                    href={`/projects/${projectId}/results`}
                    className="mt-3 inline-flex h-8 items-center rounded-lg border border-slate-200 px-3 text-xs font-semibold text-slate-700 transition hover:bg-slate-50"
                  >
                    View measurement
                  </Link>
                </div>

                {visibilityBlockers.length > 0 && (
                  <div className="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
                    <div className="font-bold">Data note</div>
                    <p className="mt-2 leading-6">
                      Measurement signals are limited, but review can continue with context-backed findings.
                    </p>
                  </div>
                )}
              </aside>
            </div>
          </section>

          <section data-analysis-autopilot-visible>
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
            <div className="mt-3 rounded-lg border border-slate-200 bg-white p-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <div className="text-sm font-bold text-slate-900">Recovery plan</div>
                  <p className="mt-1 text-sm text-slate-500">
                    Manual rollback required unless publisher rollback is available. Guarded execution records recovery metadata for every executed action.
                  </p>
                </div>
                <Badge tone={executionResult?.executed_actions.length ? "green" : "neutral"}>
                  {executionResult?.executed_actions.length ?? 0} executed
                </Badge>
              </div>
            </div>
          </section>
        </div>

        </>
      )}

      {mode === "results" && (
        <div className="space-y-7" data-results-actions={resultsActions.length}>
          <section>
            <SectionHeader
              title="Outcome summary"
              eyebrow="Published work"
              action={
                <Badge tone={measurementExceptions.length ? "amber" : attributionMeasuredActions.length ? "green" : "neutral"}>
                  {attributionMeasuredActions.length}
                </Badge>
              }
            />
            <div className="grid gap-3 md:grid-cols-5">
              {[
                { label: "Waiting", value: outcomeCounts.waiting, tone: "neutral" as const, detail: "Inside measurement window" },
                { label: "Positive", value: outcomeCounts.positive, tone: "green" as const, detail: "Signals improved" },
                { label: "Negative", value: outcomeCounts.negative, tone: "red" as const, detail: "Needs follow-up" },
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
              title="Action-level attribution"
              eyebrow="Measurement queue"
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
              <EmptyState title="No content actions are ready for verification yet" detail="Accepted, published, or URL-verified actions will appear here once they enter the loop." />
            ) : (
              <div className="grid gap-3">
                {(resultsActions.length ? attributionActions.slice(0, 12) : resultActions.slice(0, 12).map((action) => action)).map((action) => {
                  const state = actionMeasurementState(action);
                  const measurement = latestActionMeasurement(action);
                  const before = measurementMetricText(measurement, "before");
                  const after = measurementMetricText(measurement, "after");
                  const confounders = actionConfounders(action);
                  return (
                    <article key={action.id} className="rounded-xl border border-slate-200 bg-white p-4">
                      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <Badge tone={state.tone}>{state.label}</Badge>
                            <Badge tone={toneForStatus(action.status)}>{action.status}</Badge>
                          </div>
                          <h3 className="mt-3 text-lg font-bold leading-6 text-slate-950">{action.action_type}</h3>
                          <p className="mt-2 truncate text-sm leading-6 text-slate-600">{action.target_url ?? action.normalized_target_url ?? action.id}</p>
                        </div>
                        <div className="shrink-0 text-sm text-slate-500">
                          <div className="font-semibold text-slate-700">Published</div>
                          <div>{formatDate(action.published_at ?? null)}</div>
                        </div>
                      </div>
                      <div className="mt-4 grid gap-3 border-t border-slate-100 pt-3 text-sm md:grid-cols-3 xl:grid-cols-5">
                        <div>
                          <div className="text-xs font-semibold uppercase text-slate-400">Asset</div>
                          <div className="mt-1 font-medium text-slate-700">{action.asset_type ?? "unspecified"}</div>
                        </div>
                        <div>
                          <div className="text-xs font-semibold uppercase text-slate-400">Review</div>
                          <div className="mt-1 font-medium text-slate-700">{action.review_required === false ? "Optional" : "Required"}</div>
                        </div>
                        <div>
                          <div className="text-xs font-semibold uppercase text-slate-400">Verification</div>
                          <div className="mt-1 font-medium text-slate-700">
                            {action.verified_at ? "Verified" : action.verification_snapshot ? "Needs check" : "Not started"}
                          </div>
                        </div>
                        <div>
                          <div className="text-xs font-semibold uppercase text-slate-400">Measurement window</div>
                          <div className="mt-1 font-medium text-slate-700">{measurementWindowLabel(action.measurement_window)}</div>
                        </div>
                        <div>
                          <div className="text-xs font-semibold uppercase text-slate-400">Result</div>
                          <div className="mt-1 font-medium text-slate-700">{state.detail}</div>
                        </div>
                      </div>
                      <div className="mt-4 grid gap-3 border-t border-slate-100 pt-3 text-sm md:grid-cols-2 xl:grid-cols-4">
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
                      <div className="mt-4 grid gap-3 border-t border-slate-100 pt-3 text-sm md:grid-cols-2 xl:grid-cols-5">
                        <div>
                          <div className="text-xs font-semibold uppercase text-slate-400">Before</div>
                          <div className="mt-1 font-medium text-slate-700">{before}</div>
                        </div>
                        <div>
                          <div className="text-xs font-semibold uppercase text-slate-400">After</div>
                          <div className="mt-1 font-medium text-slate-700">{after}</div>
                        </div>
                        <div>
                          <div className="text-xs font-semibold uppercase text-slate-400">Outcome reason</div>
                          <div className="mt-1 font-medium text-slate-700">{state.detail}</div>
                        </div>
                        <div>
                          <div className="text-xs font-semibold uppercase text-slate-400">Attribution confidence</div>
                          <div className="mt-1 font-medium text-slate-700">{actionAttributionConfidence(action)}</div>
                        </div>
                        <div>
                          <div className="text-xs font-semibold uppercase text-slate-400">Confounders</div>
                          <div className="mt-1 font-medium text-slate-700">{confounders.length ? confounders.slice(0, 2).join(" / ") : "None noted"}</div>
                        </div>
                      </div>
                      <details className="mt-3 rounded-lg border border-slate-100 bg-slate-50 px-3 py-2">
                        <summary className="cursor-pointer text-sm font-semibold text-slate-700">Measurement details</summary>
                        <div className="mt-3 grid gap-2 text-xs leading-5 text-slate-600 md:grid-cols-2">
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
                            {action.verified_at ? formatDate(action.verified_at) : compactOutcomeText(action.verification_snapshot)}
                          </div>
                          <div>
                            <span className="font-semibold text-slate-800">Target URL</span>
                            <br />
                            {action.target_url ?? action.normalized_target_url ?? "No target URL yet."}
                          </div>
                        </div>
                      </details>
                      <div className="mt-3 flex flex-wrap gap-2 border-t border-slate-100 pt-3">
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
                    </article>
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
    {mode === "analysis" && selectedOpportunity && (() => {
      const addingToPlan = createActionBusy(selectedOpportunity);
      const dismissingOpportunity = dismissBusy(selectedOpportunity);
      const reviewingOpportunity = addingToPlan || dismissingOpportunity;
      const cta = actionCtaForOpportunity(selectedOpportunity);
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
            aria-labelledby="finding-details-title"
            className="absolute right-0 top-0 flex h-[100dvh] max-h-[100dvh] w-full max-w-2xl motion-safe:animate-[citeloop-drawer-panel-in_220ms_cubic-bezier(0.16,1,0.3,1)] flex-col overflow-hidden border-l border-slate-200 bg-white shadow-2xl"
          >
            <div className="flex items-start justify-between gap-4 border-b border-slate-100 p-5">
              <div className="min-w-0">
                <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Finding details</div>
                <h3 id="finding-details-title" className="mt-2 text-xl font-bold leading-7 text-slate-950">
                  {opportunityTitle(selectedOpportunity)}
                </h3>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <Badge tone="blue">{findingTypeLabel(selectedOpportunity)}</Badge>
                  <Badge tone={toneForRisk(selectedOpportunity.risk_level)}>{selectedOpportunity.risk_level ?? "risk unknown"}</Badge>
                  <Badge tone="neutral">{sourceModeForOpportunity(selectedOpportunity, overview)}</Badge>
                  <Badge tone={toneForStatus(selectedOpportunity.status)}>{visibilityLifecycleLabel(selectedOpportunity.status)}</Badge>
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
                  <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Expected impact</div>
                  <p className="mt-2 text-sm leading-6 text-slate-700">
                    {selectedOpportunity.expected_impact || "Review this finding against confirmed Context before creating downstream work."}
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
                  <div className="sm:col-span-2">
                    <div className="text-xs font-semibold uppercase text-slate-400">Source</div>
                    <div className="mt-1 break-words font-medium text-slate-700">
                      {selectedOpportunity.page_url ?? selectedOpportunity.normalized_page_url ?? "Project domain"}
                    </div>
                  </div>
                  <div className="sm:col-span-2">
                    <div className="text-xs font-semibold uppercase text-slate-400">Opportunity type</div>
                    <div className="mt-1 break-words font-medium text-slate-700">{selectedOpportunity.type}</div>
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
              className="shrink-0 flex flex-col gap-2 border-t border-slate-200 bg-white px-4 pb-[calc(1.5rem+env(safe-area-inset-bottom))] pt-4 sm:flex-row sm:justify-end"
            >
              <Button size="sm" variant="ghost" onClick={() => dismiss(selectedOpportunity)} disabled={reviewingOpportunity}>
                <ButtonProgress busy={dismissingOpportunity} busyLabel="Dismissing" idleIcon={null}>
                  Dismiss
                </ButtonProgress>
              </Button>
              <Button size="sm" variant="primary" onClick={() => createAction(selectedOpportunity)} disabled={reviewingOpportunity}>
                <ButtonProgress busy={addingToPlan} busyLabel={cta.busyLabel} idleIcon={<FileText size={14} />}>
                  {cta.label}
                </ButtonProgress>
              </Button>
            </div>
          </aside>
        </div>
      );
    })()}
    </>
  );
}
