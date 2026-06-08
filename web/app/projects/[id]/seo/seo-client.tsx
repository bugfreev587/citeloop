"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { BarChart3, CheckCircle2, FileText, RefreshCw, Search, Settings, ShieldAlert } from "lucide-react";
import {
  AICrawlerAccessSnapshot,
  SEOActionPlan,
  SEOBrief,
  SEOContentAction,
  SEOObjective,
  SEOOpportunity,
  SEOOverview,
  SEOPolicy,
  SafeModeEvent,
} from "../../../lib/api";
import { normalizeNumeric } from "../../../lib/normalize";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, EmptyState, Field, Notice, SectionHeader, TextInput, formatDate } from "../../../components/ui";

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
  const [siteURL, setSiteURL] = useState("");
  const [objectiveName, setObjectiveName] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    setMessage(null);
    try {
      const [overviewData, settings, briefData, opps, actionRows, policyData, objectiveRows, planRows, safeModeRows, crawlerAudit] = await Promise.all([
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
        title="SEO"
        eyebrow="Operations loop"
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

      <section>
        <SectionHeader title={brief?.title ?? "SEO operating brief"} action={<Badge tone={brief?.mode === "cold_start" ? "amber" : "green"}>{brief?.mode ?? "loading"}</Badge>} />
        {brief ? (
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            {brief.blockers.length > 0 && (
              <div className="mb-4 grid gap-2">
                {brief.blockers.map((blocker) => (
                  <Notice key={blocker} title="Blocker" detail={blocker} tone="amber" />
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
                          Queue
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
