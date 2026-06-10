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
import { visibilityLifecycleLabel, visibilityLifecycleTone } from "../../../lib/dashboard-ux-logic";
import { normalizeNumeric } from "../../../lib/normalize";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, EmptyState, Field, Notice, SectionHeader, TextInput, cx, formatDate } from "../../../components/ui";

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

export function SEOClient({ projectId }: { projectId: string }) {
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
  const [objectiveName, setObjectiveName] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
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
  const loopRows = [
    ...opportunities.slice(0, 4).map((opportunity) => ({
      id: `opportunity-${opportunity.id}`,
      title: opportunityTitle(opportunity),
      stage: visibilityLifecycleLabel(opportunity.status),
      tone: visibilityLifecycleTone(opportunity.status),
      detail: opportunity.expected_impact || opportunity.page_url || opportunity.normalized_page_url || "Visibility signal",
    })),
    ...actions.slice(0, 3).map((action) => ({
      id: `action-${action.id}`,
      title: action.action_type,
      stage: visibilityLifecycleLabel(action.status),
      tone: visibilityLifecycleTone(action.status),
      detail: action.target_url || action.normalized_target_url || "Content action",
    })),
  ].slice(0, 5);
  const loopTotal = opportunities.length + actions.length;

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
      await api.createGEOExternalSurface(projectId, { url: surfaceURL.trim(), owner_type: "project" });
      setSurfaceURL("");
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
    setBusy(opp.id);
    setMessage(null);
    try {
      await api.createSEOContentAction(projectId, opp.id, { action_type: opp.recommended_action ?? undefined });
      await refresh();
      setMessage({ title: "Content action created", detail: opp.recommended_action ?? opp.type, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not create action", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function dismiss(opp: SEOOpportunity) {
    setBusy(opp.id);
    try {
      await api.dismissSEOOpportunity(projectId, opp.id);
      await refresh();
    } catch (e: any) {
      setMessage({ title: "Could not dismiss opportunity", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
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
        title="Visibility"
        eyebrow="SEO and AI-answer visibility for your domain"
        action={
          <div className="flex flex-wrap gap-2">
            <Button size="sm" onClick={refresh} disabled={!!busy}>
              <RefreshCw size={14} />
              Refresh
            </Button>
            <Button size="sm" variant="primary" onClick={runSync} disabled={!!busy}>
              <Search size={14} />
              Sync
            </Button>
          </div>
        }
      />

      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}
      {overview?.data_source_warnings?.map((warning) => (
        <Notice key={warning} title="SEO data warning" detail={warning} tone="amber" />
      ))}

      <section>
        <SectionHeader
          title="Visibility overview"
          eyebrow={capabilityLabel(visibilityMode)}
          action={<Badge tone={overview?.handoff_ready_for_autopilot ? "green" : "amber"}>{capabilityLabel(visibilityMode)}</Badge>}
        />
        <div className="grid gap-3 lg:grid-cols-[1fr_1fr]">
          <div className="rounded-xl border border-slate-200 bg-white p-4">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-sm font-bold text-slate-900">Top opportunities</div>
                <p className="mt-1 text-sm leading-5 text-slate-500">
                  The next content or crawl fixes most likely to improve SEO and AI-answer visibility.
                </p>
              </div>
              <Badge tone={opportunities.length ? "amber" : "neutral"}>{opportunities.length}</Badge>
            </div>
            <div className="mt-3 grid gap-2">
              {opportunities.slice(0, 3).map((opportunity) => (
                <div key={opportunity.id} className="rounded-lg border border-slate-100 px-3 py-2">
                  <div className="font-semibold text-slate-900">{opportunityTitle(opportunity)}</div>
                  <div className="mt-1 text-sm leading-5 text-slate-500">{opportunity.expected_impact || opportunity.type}</div>
                </div>
              ))}
              {opportunities.length === 0 && <div className="text-sm text-slate-500">No open opportunities. Refresh Visibility after new search or AI-answer signals arrive.</div>}
            </div>
          </div>
          <div className="rounded-xl border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">Major blockers</div>
            <div className="mt-3 grid gap-2">
              {visibilityBlockers.map((blocker) => (
                <Notice key={blocker} title="Visibility blocker" detail={blocker} tone="amber" />
              ))}
              {visibilityBlockers.length === 0 && <div className="text-sm text-slate-500">No major blockers detected from current public crawl and connected data.</div>}
            </div>
          </div>
        </div>
      </section>

      <section>
        <SectionHeader title="Search visibility" eyebrow={gscStatus === "connected" ? "Verified search data" : "Public crawl only"} />
        {gscStatus !== "connected" && (
          <Notice
            title="Search Console is not connected"
            detail="The numbers below are placeholders. Connect first-party search data to show verified clicks, impressions, CTR, and position."
            tone="amber"
          />
        )}
        <div className={cx("grid gap-3 md:grid-cols-4", gscStatus !== "connected" && "opacity-60")}>
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
              <Badge tone={overview?.cold_start ? "amber" : "green"}>{overview?.cold_start ? "limited" : "ready"}</Badge>
            </div>
            <p className="mt-1 text-sm leading-5 text-slate-500">Technical URLs</p>
          </div>
        </div>
      </section>

      <section>
        <SectionHeader title="AI visibility" eyebrow={showGeoScore ? "Observed answer coverage" : "Capability-aware status"} />
        <div className="grid gap-3 md:grid-cols-4">
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">{showGeoScore ? metric(geoScoreValue, 1) : "Insufficient data"}</div>
            <p className="mt-1 text-sm text-slate-500">Visibility score</p>
          </div>
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">{percent(geoCoverage)}</div>
            <p className="mt-1 text-sm text-slate-500">Answer coverage</p>
          </div>
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">{crawlerOkCount}/{crawlerSnapshots.length || 0}</div>
            <p className="mt-1 text-sm text-slate-500">AI crawler access</p>
          </div>
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="text-sm font-bold text-slate-900">{assetBriefs.length}</div>
            <p className="mt-1 text-sm text-slate-500">Citation-ready briefs</p>
          </div>
        </div>
      </section>

      <section>
        <SectionHeader title="Loop closure" eyebrow="Opportunity to content to measurement" />
        {loopRows.length === 0 ? (
          <EmptyState title="No active visibility loop yet" detail="Opportunity detected items will appear here, then move through Added to Content Plan, Drafted, Published, and Measuring impact." />
        ) : (
          <div className="grid gap-2">
            {loopRows.map((row) => (
              <div key={row.id} className="flex min-h-[46px] items-center justify-between gap-3 rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm">
                <div className="min-w-0">
                  <div className="truncate font-semibold text-slate-900">{row.title}</div>
                  <div className="mt-0.5 truncate text-[13px] font-semibold text-slate-400">{row.detail}</div>
                </div>
                <Badge tone={row.tone}>{row.stage}</Badge>
              </div>
            ))}
            {loopTotal > loopRows.length && (
              <div className="px-1 text-xs font-semibold text-slate-400">
                Showing {loopRows.length} of {loopTotal}. See Opportunities and Content actions below for the full list.
              </div>
            )}
          </div>
        )}
      </section>

      <details className="rounded-xl border border-slate-200 bg-white p-4">
        <summary className="cursor-pointer text-sm font-bold text-slate-900">Advanced diagnostics</summary>
        <div className="mt-5 space-y-7">
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
            <Settings size={14} />
            Save settings
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
                <RefreshCw size={14} />
                Audit
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
                <FileText size={14} />
                Prompts
              </Button>
              <Button size="sm" onClick={observeGEOProvider} disabled={!!busy || (geoOverview?.prompts.length ?? 0) === 0}>
                <Search size={14} />
                Provider
              </Button>
              <Button size="sm" onClick={analyzeGEOOpportunities} disabled={!!busy}>
                <BarChart3 size={14} />
                Analyze
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
                        {prompt.status === "active" ? "Pause" : "Activate"}
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
                        {competitor.status === "active" ? "Pause" : "Activate"}
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
          <div className="flex flex-col gap-2 md:flex-row">
            <TextInput value={surfaceURL} onChange={(event) => setSurfaceURL(event.target.value)} placeholder="https://dev.to/team/source" />
            <Button size="sm" onClick={addExternalSurface} disabled={busy === "geo-surface" || !surfaceURL.trim()}>
              <FileText size={14} />
              Surface
            </Button>
            <Button size="sm" onClick={monitorExternalSurfaces} disabled={!!busy || (geoOverview?.external_surfaces.length ?? 0) === 0}>
              <RefreshCw size={14} />
              Monitor
            </Button>
          </div>
          <div className="mt-3 grid gap-2 md:grid-cols-2">
            {(geoOverview?.external_surfaces ?? []).slice(0, 6).map((surface) => (
              <div key={surface.id} className="flex min-w-0 items-center justify-between gap-2 rounded-lg border border-slate-100 px-3 py-2">
                <span className="truncate text-sm text-slate-700">{surface.url}</span>
                <Badge tone={surface.owner_type === "project" ? "green" : "neutral"}>{surface.owner_type}</Badge>
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
                      <FileText size={14} />
                      Accept
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
                <BarChart3 size={14} />
                Plan
              </Button>
              <Button size="sm" variant="danger" onClick={enterSafeMode} disabled={!!busy}>
                <ShieldAlert size={14} />
                Safe mode
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
              <FileText size={14} />
              Objective
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
      </section>
        </div>
      </details>

      <section>
        <SectionHeader title={brief?.title ?? "Visibility brief"} action={<Badge tone={brief?.mode === "cold_start" ? "amber" : "green"}>{brief?.mode ?? "loading"}</Badge>} />
        {brief ? (
          <div className="rounded-lg border border-slate-200 bg-white p-4">
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
      </section>

      <section>
        <SectionHeader title="Opportunities" action={<Badge tone="blue">{opportunities.length}</Badge>} />
        {opportunities.length === 0 ? (
          <EmptyState title="No open opportunities" detail="Cold-start health checks and GSC-backed recommendations will appear here." />
        ) : (
          <div className="overflow-hidden rounded-lg border border-slate-200 bg-white">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-slate-200 bg-slate-50 text-xs uppercase text-slate-500">
                <tr>
                  <th className="px-4 py-3 font-semibold">Type</th>
                  <th className="px-4 py-3 font-semibold">Page</th>
                  <th className="px-4 py-3 font-semibold">Score</th>
                  <th className="px-4 py-3 font-semibold">Risk</th>
                  <th className="px-4 py-3 font-semibold">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {opportunities.map((opp) => (
                  <tr key={opp.id}>
                    <td className="px-4 py-3 font-medium text-slate-900">{opp.type}</td>
                    <td className="max-w-[260px] truncate px-4 py-3 text-slate-600">{opp.page_url ?? opp.normalized_page_url}</td>
                    <td className="px-4 py-3 text-slate-600">{metric(opp.priority_score)}</td>
                    <td className="px-4 py-3">
                      <Badge tone={toneForRisk(opp.risk_level)}>{opp.risk_level ?? "unknown"}</Badge>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-2">
                        <Button size="sm" onClick={() => createAction(opp)} disabled={busy === opp.id}>
                          <FileText size={14} />
                          Add to Content Plan
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => dismiss(opp)} disabled={busy === opp.id}>
                          Dismiss
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section>
        <SectionHeader title="Content actions" action={<Badge tone="neutral">{actions.length}</Badge>} />
        {actions.length === 0 ? (
          <EmptyState title="No SEO content actions" detail="Accepted opportunities become reviewable content actions here." />
        ) : (
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
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
