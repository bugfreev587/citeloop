"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { BarChart3, CheckCircle2, FileText, RefreshCw, Search, Settings, ShieldAlert } from "lucide-react";
import {
  AICrawlerAccessSnapshot,
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
} from "../../../lib/api";
import { visibilityLifecycleLabel } from "../../../lib/dashboard-ux-logic";
import { normalizeNumeric } from "../../../lib/normalize";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, ButtonProgress, EmptyState, Field, Notice, SectionHeader, TextInput, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

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
  if (text.includes("comparison")) return "comparison_page";
  if (text.includes("alternative")) return "alternative_page";
  if (text.includes("template") || text.includes("checklist")) return "template_or_checklist";
  if (text.includes("schema")) return "schema_patch";
  if (text.includes("internal link")) return "internal_link_patch";
  if (text.includes("metadata") || text.includes("title") || text.includes("meta")) return "metadata_rewrite";
  if (text.includes("sitemap")) return "sitemap_update";
  if (text.includes("geo")) return "glossary_definition";
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

type SEOClientMode = "opportunities" | "visibility";

export function OpportunitiesClient({ projectId }: { projectId: string }) {
  return <SEOClient projectId={projectId} mode="opportunities" />;
}

export function VisibilityClient({ projectId }: { projectId: string }) {
  return <SEOClient projectId={projectId} mode="visibility" />;
}

export function SEOClient({ projectId, mode = "opportunities" }: { projectId: string; mode?: SEOClientMode }) {
  const api = useApi();
  const [overview, setOverview] = useState<SEOOverview | null>(null);
  const [brief, setBrief] = useState<SEOBrief | null>(null);
  const [opportunities, setOpportunities] = useState<SEOOpportunity[]>([]);
  const [actions, setActions] = useState<SEOContentAction[]>([]);
  const [policy, setPolicy] = useState<SEOPolicy | null>(null);
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
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    setMessage(null);
    try {
      const [overviewData, settings, briefData, opps, actionRows, policyData, objectiveRows, planRows, safeModeRows, crawlerAudit, geoData, briefRows] = await Promise.all([
        api.getSEOOverview(projectId),
        api.getSEOSettings(projectId),
        api.getSEOBrief(projectId),
        api.listSEOOpportunities(projectId, { status: "open", limit: 50 }),
        api.listSEOContentActions(projectId, { limit: 50 }),
        api.getSEOPolicy(projectId),
        api.listSEOObjectives(projectId),
        api.listAutopilotPlans(projectId),
        api.listSafeModeEvents(projectId),
        api.getLatestGEOCrawlerAudit(projectId),
        api.getGEOOverview(projectId),
        api.listGEOAssetBriefs(projectId, { limit: 50 }),
      ]);
      setOverview(overviewData);
      setBrief(briefData);
      setOpportunities(opps);
      setActions(actionRows);
      setPolicy(policyData);
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
  const crawlerOkCount = crawlerSnapshots.filter((snapshot) => snapshot.access_state === "ok").length;
  const latestPortfolioPlan = plans[0] ?? null;

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
      const result = await api.observeGEOProvider(projectId, { engine: "Perplexity", max_prompts: 10 });
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
      setMessage({ title: "GEO brief converted", detail: result.topic?.title ?? brief.asset_type, tone: "green" });
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
        review_required: opp.risk_level !== "low",
      });
      setOpportunities((current) => current.filter((item) => item.id !== opp.id));
      setActions((current) => [action, ...current.filter((item) => item.id !== action.id)]);
      setMessage({ title: "Content action created", detail: opp.recommended_action ?? opp.type, tone: "green" });
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
    <div className="space-y-7">
      <SectionHeader
        title={mode === "opportunities" ? "Review opportunities" : "Measurement and diagnostics"}
        eyebrow={mode === "opportunities" ? "Find opportunities" : "Measure visibility"}
        action={
          <div className="flex flex-wrap gap-2">
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

      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      {mode === "opportunities" && (
      <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="space-y-3">
          <div className="rounded-xl border border-slate-200 bg-white p-4">
            <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
              <div>
                <Badge tone={opportunities.length ? "green" : "neutral"}>
                  {opportunities.length ? "Ready to review" : "No review needed"}
                </Badge>
                <h3 className="mt-3 text-2xl font-bold leading-8 text-slate-950">
                  {opportunities.length} {opportunities.length === 1 ? "opportunity" : "opportunities"} need review
                </h3>
                <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-600">
                  Choose the opportunities worth turning into content work. Add the useful ones to Content Plan and dismiss anything that is not a fit.
                </p>
              </div>
              <Badge tone={overview?.cold_start ? "amber" : "green"}>{capabilityLabel(visibilityMode)}</Badge>
            </div>
          </div>

          {opportunities.length === 0 ? (
            <EmptyState
              title="No opportunities to review"
              detail="Refresh or Sync after Context changes. New opportunities will appear here as review cards."
            />
          ) : (
            <div className="grid gap-3">
              {opportunities.map((opp, index) => {
                const addingToPlan = createActionBusy(opp);
                const dismissingOpportunity = dismissBusy(opp);
                const reviewingOpportunity = addingToPlan || dismissingOpportunity;
                return (
                <article key={opp.id} className="rounded-xl border border-slate-200 bg-white p-4">
                  <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge tone="blue">Opportunity {index + 1}</Badge>
                        <Badge tone={toneForRisk(opp.risk_level)}>{opp.risk_level ?? "risk unknown"}</Badge>
                        <span className="text-xs font-semibold uppercase text-slate-400">Score {metric(opp.priority_score)}</span>
                      </div>
                      <h3 className="mt-3 text-lg font-bold leading-6 text-slate-950">{opportunityTitle(opp)}</h3>
                      <p className="mt-2 text-sm leading-6 text-slate-600">
                        {opp.expected_impact || "Review this opportunity against your confirmed Context before adding it to the content backlog."}
                      </p>
                    </div>
                    <div className="flex shrink-0 flex-wrap gap-2">
                      <Button size="sm" variant="primary" onClick={() => createAction(opp)} disabled={reviewingOpportunity}>
                        <ButtonProgress busy={addingToPlan} busyLabel="Adding to plan" idleIcon={<FileText size={14} />}>
                          Add to Content Plan
                        </ButtonProgress>
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => dismiss(opp)} disabled={reviewingOpportunity}>
                        <ButtonProgress busy={dismissingOpportunity} busyLabel="Dismissing" idleIcon={null}>
                          Dismiss
                        </ButtonProgress>
                      </Button>
                    </div>
                  </div>
                  <div className="mt-4 grid gap-2 border-t border-slate-100 pt-3 text-sm md:grid-cols-[1fr_auto] md:items-center">
                    <div className="min-w-0">
                      <div className="text-xs font-semibold uppercase text-slate-400">Source page</div>
                      <div className="mt-1 truncate font-medium text-slate-700">{opp.page_url ?? opp.normalized_page_url ?? "Project domain"}</div>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Badge tone="neutral">{opp.type}</Badge>
                      <Badge tone={toneForStatus(opp.status)}>{visibilityLifecycleLabel(opp.status)}</Badge>
                    </div>
                  </div>
                </article>
                );
              })}
            </div>
          )}
        </div>

        <aside className="space-y-3">
          <div className="rounded-xl border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">What to review</div>
            <div className="mt-3 space-y-3 text-sm leading-6 text-slate-600">
              <p>Does the recommendation match the confirmed Context?</p>
              <p>Is the source page or evidence strong enough to support the claim?</p>
              <p>Would this create a useful topic for the next content backlog?</p>
            </div>
          </div>
          <div className="rounded-xl border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">Review result</div>
            <div className="mt-3 grid grid-cols-2 gap-3 text-sm">
              <div>
                <div className="text-2xl font-bold text-slate-950">{opportunities.length}</div>
                <div className="mt-1 text-slate-500">open</div>
              </div>
              <div>
                <div className="text-2xl font-bold text-slate-950">{actions.length}</div>
                <div className="mt-1 text-slate-500">in plan</div>
              </div>
            </div>
          </div>
          {visibilityBlockers.length > 0 && (
            <div className="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
              <div className="font-bold">Data note</div>
              <p className="mt-2 leading-6">
                Measurement signals are limited, but review can continue with context-backed opportunities.
              </p>
            </div>
          )}
        </aside>
      </section>
      )}

      {mode === "visibility" && (
        <div className="space-y-7">
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
              <Button size="sm" variant="danger" onClick={enterSafeMode} disabled={!!busy}>
                <ButtonProgress busy={busy === "safe-mode"} busyLabel="Enabling safe mode" idleIcon={<ShieldAlert size={14} />}>
                  Safe mode
                </ButtonProgress>
              </Button>
            </div>
          }
        />
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
      </section>
        </div>
      )}

      {mode === "opportunities" && (
      <details className="rounded-xl border border-slate-200 bg-white p-4">
        <summary className="flex cursor-pointer items-center justify-between gap-3 text-sm font-bold text-slate-900">
          <span>{brief?.title ?? "Visibility brief"}</span>
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

      {mode === "opportunities" && actions.length > 0 && (
      <section>
        <SectionHeader title="Content actions" action={<Badge tone="neutral">{actions.length}</Badge>} />
          <div className="grid gap-2">
            {actions.map((action) => (
              <div key={action.id} className="rounded-lg border border-slate-200 bg-white px-4 py-3">
                <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                  <div className="min-w-0">
                    <div className="truncate text-sm font-bold text-slate-900">{action.action_type}</div>
                    <div className="mt-1 truncate text-xs text-slate-500">{action.target_url ?? action.normalized_target_url ?? action.id}</div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge tone={toneForStatus(action.status)}>{action.status}</Badge>
                    <span className="text-xs text-slate-400">{formatDate(action.created_at ?? null)}</span>
                  </div>
                </div>
                <div className="mt-2 grid gap-2 text-xs text-slate-500 sm:grid-cols-4">
                  <div>
                    <span className="font-semibold text-slate-700">Asset</span>
                    <br />
                    {action.asset_type ?? "unspecified"}
                  </div>
                  <div>
                    <span className="font-semibold text-slate-700">Review</span>
                    <br />
                    {action.review_required === false ? "Optional" : "Required"}
                  </div>
                  <div>
                    <span className="font-semibold text-slate-700">Verification</span>
                    <br />
                    {action.verified_at ? "Verified" : action.verification_snapshot ? "Pending" : "Not started"}
                  </div>
                  <div>
                    <span className="font-semibold text-slate-700">Measurement</span>
                    <br />
                    {measurementWindowLabel(action.measurement_window)}
                  </div>
                </div>
                <div className="mt-3 flex flex-wrap gap-2">
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
              </div>
            ))}
          </div>
      </section>
      )}
    </div>
  );
}
