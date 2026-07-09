"use client";

import { useCallback, useEffect, useState } from "react";
import { AlertTriangle, CheckCircle2, Globe2, KeyRound, Loader2, PlugZap, RefreshCw, Save, ShieldCheck, Trash2, XCircle } from "lucide-react";
import { GenerationRun, GEOCredentialsStatus, GEOProviderScope, LLMCredentialsStatus, LLMModelProvider, LLMModelRoute, LLMRuntimeRole, ProviderTestResult } from "../../../lib/api";
import { activePlatformRuntimeIncidents, activityRawError } from "../../../lib/activity-runs";
import { tokengateAnthropicModelOptions, tokengateModelOptionsWithCurrent, tokengateOpenAIModelOptions } from "../../../lib/tokengate-models";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";
import { Badge, Button, ButtonProgress, cx, EmptyState, Field, formatDate, Notice, SectionHeader, SelectInput, TextInput } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type TestResult = ProviderTestResult | null;
type AdminTabId = "runtime" | "geo" | "incidents";
type RuntimeBusy = "save" | "test" | "delete" | null;
type GEOBusy = `save-${GEOProviderScope}` | `test-${GEOProviderScope}` | `delete-${GEOProviderScope}` | null;
type GEODraft = { apiKey: string; baseURL: string; model: string; enabled: boolean };
type RuntimeRoutes = Record<LLMRuntimeRole, LLMModelRoute>;

const defaultBaseURL = "https://tokengate-production.up.railway.app/v1";

const runtimeRoleConfigs: Array<{
  role: LLMRuntimeRole;
  label: string;
  helper: string;
  openAIPlaceholder: string;
  anthropicPlaceholder: string;
}> = [
  {
    role: "planning",
    label: "Planning",
    helper: "Context extraction, strategy, and opportunity finding.",
    openAIPlaceholder: "gpt-5.4",
    anthropicPlaceholder: "claude-sonnet-4-6",
  },
  {
    role: "writer",
    label: "AI writer",
    helper: "Content writer and article repair.",
    openAIPlaceholder: "gpt-5.4",
    anthropicPlaceholder: "claude-sonnet-4-6",
  },
  {
    role: "qa",
    label: "QA",
    helper: "Evidence checks and QA review.",
    openAIPlaceholder: "gpt-5.5",
    anthropicPlaceholder: "claude-opus-4-8",
  },
  {
    role: "site_fix",
    label: "Site Fix",
    helper: "AI fix JSON to source-controlled GitHub PRs.",
    openAIPlaceholder: "gpt-5.4",
    anthropicPlaceholder: "claude-opus-4-8",
  },
];

function defaultRuntimeRoute(role: LLMRuntimeRole): LLMModelRoute {
  const config = runtimeRoleConfigs.find((item) => item.role === role) ?? runtimeRoleConfigs[0];
  return {
    primary_provider: role === "site_fix" ? "anthropic" : "openai",
    openai_model_alias: config.openAIPlaceholder,
    anthropic_model_alias: config.anthropicPlaceholder,
    fallback_enabled: role !== "site_fix",
  };
}

function defaultRuntimeRoutes(): RuntimeRoutes {
  return runtimeRoleConfigs.reduce((acc, config) => {
    acc[config.role] = defaultRuntimeRoute(config.role);
    return acc;
  }, {} as RuntimeRoutes);
}

function normalizeRuntimeRoutes(status: LLMCredentialsStatus | null): RuntimeRoutes {
  const defaults = defaultRuntimeRoutes();
  const legacy: Partial<Record<LLMRuntimeRole, string | undefined>> = {
    planning: status?.model,
    writer: status?.writer_model,
    qa: status?.qa_model,
  };
  for (const config of runtimeRoleConfigs) {
    const saved = status?.routes?.[config.role];
    const base = defaults[config.role];
    const primary = saved?.primary_provider === "anthropic" || saved?.primary_provider === "openai" ? saved.primary_provider : base.primary_provider;
    defaults[config.role] = {
      primary_provider: primary,
      openai_model_alias: saved?.openai_model_alias || legacy[config.role] || base.openai_model_alias,
      anthropic_model_alias: saved?.anthropic_model_alias || base.anthropic_model_alias,
      fallback_enabled: typeof saved?.fallback_enabled === "boolean" ? saved.fallback_enabled : base.fallback_enabled,
    };
  }
  return defaults;
}

function selectedRuntimeModel(route: LLMModelRoute) {
  return route.primary_provider === "anthropic" ? route.anthropic_model_alias : route.openai_model_alias;
}

const adminTabs: Array<{ id: AdminTabId; title: string }> = [
  { id: "runtime", title: "Platform runtime" },
  { id: "geo", title: "GEO providers" },
  { id: "incidents", title: "Runtime incidents" },
];

const geoProviders: Array<{
  scope: GEOProviderScope;
  label: string;
  keyLabel: string;
  defaultModel: string;
  helper: string;
}> = [
  {
    scope: "perplexity",
    label: "Perplexity",
    keyLabel: "TokenGate key for Perplexity",
    defaultModel: "sonar-pro",
    helper: "Optional answer and citation observation through TokenGate. OpenAI or Anthropic can run GEO workflows when Perplexity is not configured.",
  },
  {
    scope: "openai",
    label: "OpenAI",
    keyLabel: "TokenGate key for OpenAI",
    defaultModel: "gpt-5.1",
    helper: "Reserved for GEO analysis and future citation-capable observer workflows through TokenGate.",
  },
  {
    scope: "anthropic",
    label: "Anthropic",
    keyLabel: "TokenGate key for Anthropic",
    defaultModel: "claude-sonnet-4-6",
    helper: "Reserved for GEO reasoning and brief quality workflows through TokenGate.",
  },
  {
    scope: "gemini",
    label: "Gemini",
    keyLabel: "TokenGate key for Gemini",
    defaultModel: "gemini-2.5-pro",
    helper: "Reserved for secondary GEO reasoning and future answer observation through TokenGate.",
  },
];

function adminTabFromHash(hash: string): AdminTabId {
  const tab = hash.replace(/^#/, "");
  if (tab === "geo" || tab === "incidents") return tab;
  return "runtime";
}

function emptyGeoStatuses() {
  return geoProviders.reduce<Record<GEOProviderScope, GEOCredentialsStatus>>((acc, provider) => {
    acc[provider.scope] = {
      scope: provider.scope,
      provider: "tokengate",
      configured: false,
      enabled: false,
      base_url: defaultBaseURL,
      model: provider.defaultModel,
    };
    return acc;
  }, {} as Record<GEOProviderScope, GEOCredentialsStatus>);
}

function emptyGeoDrafts() {
  return geoProviders.reduce<Record<GEOProviderScope, GEODraft>>((acc, provider) => {
    acc[provider.scope] = { apiKey: "", baseURL: defaultBaseURL, model: provider.defaultModel, enabled: false };
    return acc;
  }, {} as Record<GEOProviderScope, GEODraft>);
}

function indexGeoStatuses(statuses: GEOCredentialsStatus[]) {
  const indexed = emptyGeoStatuses();
  for (const status of statuses) {
    indexed[status.scope] = {
      ...indexed[status.scope],
      ...status,
      base_url: status.base_url || defaultBaseURL,
      model: status.model || indexed[status.scope].model,
    };
  }
  return indexed;
}

function providerLabel(scope: GEOProviderScope) {
  return geoProviders.find((provider) => provider.scope === scope)?.label ?? scope;
}

function defaultGeoModel(scope: GEOProviderScope) {
  return geoProviders.find((provider) => provider.scope === scope)?.defaultModel ?? "";
}

function ConnectionResult({ result }: { result: TestResult }) {
  if (!result) return null;
  const roleResults = Array.isArray(result.results) ? result.results : [];
  if (roleResults.length > 0) {
    return (
      <div
        className={cx(
          "rounded-lg border p-3 text-sm",
          result.ok ? "border-green-200 bg-green-50 text-green-900" : "border-red-200 bg-red-50 text-red-900",
        )}
      >
        <div className="inline-flex items-center gap-2 font-bold">
          {result.ok ? <CheckCircle2 size={15} /> : <XCircle size={15} />}
          {result.ok ? "All runtime routes OK" : "One or more runtime routes failed"}
        </div>
        <div className="mt-3 grid gap-2 md:grid-cols-2">
          {roleResults.map((item) => (
            <div key={String(item.role ?? item.label ?? item.model_alias)} className="rounded-md border border-white/60 bg-white/70 p-2 text-xs leading-5">
              <div className="flex items-center justify-between gap-2 font-bold">
                <span>{item.label || item.role}</span>
                {item.ok ? <CheckCircle2 size={14} /> : <XCircle size={14} />}
              </div>
              <div className="mt-1 text-slate-700">
                {item.primary_provider} · {item.model || item.model_alias || "model n/a"}
                {typeof item.latency_ms === "number" ? ` · ${item.latency_ms} ms` : ""}
              </div>
              {!item.ok && <div className="mt-1 break-words text-red-800">{item.error || "Unknown error"}</div>}
            </div>
          ))}
        </div>
      </div>
    );
  }
  return (
    <div
      className={cx(
        "rounded-lg border p-3 text-sm",
        result.ok ? "border-green-200 bg-green-50 text-green-900" : "border-red-200 bg-red-50 text-red-900",
      )}
    >
      <div className="inline-flex items-center gap-2 font-bold">
        {result.ok ? <CheckCircle2 size={15} /> : <XCircle size={15} />}
        {result.ok ? "Connection OK" : "Connection failed"}
      </div>
      <div className="mt-1 text-xs leading-5">
        {result.ok ? (
          <>
            {result.provider} · {result.model || "model n/a"}
            {typeof result.latency_ms === "number" ? ` · ${result.latency_ms} ms` : ""}
            {typeof result.cost_usd === "number" && result.cost_usd > 0 ? ` · $${result.cost_usd.toFixed(4)}` : ""}
            {result.sample ? ` · replied "${result.sample}"` : ""}
          </>
        ) : (
          result.error || "Unknown error"
        )}
      </div>
    </div>
  );
}

function RuntimeIncidentRow({ run }: { run: GenerationRun }) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <AlertTriangle className="text-red-500" size={16} />
            <span className="font-bold text-slate-950">{run.agent}</span>
            <Badge tone="red">{run.status}</Badge>
            {run.model && <Badge tone="neutral">{run.model}</Badge>}
          </div>
          <p className="mt-2 break-words text-sm leading-6 text-slate-700">{activityRawError(run)}</p>
        </div>
        <div className="shrink-0 text-xs font-semibold text-slate-400">{formatDate(run.created_at)}</div>
      </div>
    </div>
  );
}

export function AdminClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [access, setAccess] = useState<"loading" | "granted" | "denied">("loading");
  const [activeAdminTab, setActiveAdminTab] = useState<AdminTabId>(() => {
    if (typeof window === "undefined") return "runtime";
    return adminTabFromHash(window.location.hash);
  });
  const [status, setStatus] = useState<LLMCredentialsStatus | null>(null);
  const [apiKey, setAPIKey] = useState("");
  const [baseURL, setBaseURL] = useState(defaultBaseURL);
  const [runtimeRoutes, setRuntimeRoutes] = useState<RuntimeRoutes>(defaultRuntimeRoutes);
  const [geoStatuses, setGeoStatuses] = useState<Record<GEOProviderScope, GEOCredentialsStatus>>(emptyGeoStatuses);
  const [geoDrafts, setGeoDrafts] = useState<Record<GEOProviderScope, GEODraft>>(emptyGeoDrafts);
  const [runtimeIncidents, setRuntimeIncidents] = useState<GenerationRun[]>([]);
  const [busy, setBusy] = useState<RuntimeBusy>(null);
  const [geoBusy, setGeoBusy] = useState<GEOBusy>(null);
  const { notify } = useToast();
  const setMessage = (next: Message) => {
    if (next) notify(next);
  };
  const [testResult, setTestResult] = useState<TestResult>(null);
  const [geoTestResults, setGeoTestResults] = useState<Partial<Record<GEOProviderScope, TestResult>>>({});

  useEffect(() => {
    function syncTabFromHash() {
      setActiveAdminTab(adminTabFromHash(window.location.hash));
    }
    syncTabFromHash();
    window.addEventListener("hashchange", syncTabFromHash);
    return () => window.removeEventListener("hashchange", syncTabFromHash);
  }, []);

  function activateAdminTab(tabId: AdminTabId) {
    setActiveAdminTab(tabId);
    window.history.replaceState(null, "", `#${tabId}`);
  }

  function applyGeoStatuses(statuses: GEOCredentialsStatus[]) {
    const indexed = indexGeoStatuses(statuses);
    setGeoStatuses(indexed);
    setGeoDrafts((current) => {
      const next = { ...current };
      for (const provider of geoProviders) {
        const providerStatus = indexed[provider.scope];
        next[provider.scope] = {
          apiKey: "",
          baseURL: providerStatus.base_url || defaultBaseURL,
          model: providerStatus.model || provider.defaultModel,
          enabled: providerStatus.enabled,
        };
      }
      return next;
    });
  }

  const refresh = useCallback(async () => {
    try {
      const me = await api.getMe();
      if (!me.is_admin) {
        setAccess("denied");
        return;
      }
      setAccess("granted");
      const [next, nextGeo] = await Promise.all([api.getLLMCredentials(), api.listGEOCredentials()]);
      setStatus(next);
      setBaseURL(next.base_url || defaultBaseURL);
      setRuntimeRoutes(normalizeRuntimeRoutes(next));
      applyGeoStatuses(nextGeo);
      try {
        const runs = await api.listRuns(projectId, { limit: 100 });
        setRuntimeIncidents(activePlatformRuntimeIncidents(runs));
      } catch {
        setRuntimeIncidents([]);
      }
    } catch (e: any) {
      if (String(e.message).includes("403")) {
        setAccess("denied");
      } else {
        setAccess("granted");
        setMessage({ title: "Admin settings unavailable", detail: e.message, tone: "amber" });
      }
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  async function save() {
    setBusy("save");
    setMessage(null);
    setTestResult(null);
    try {
      const planningModel = selectedRuntimeModel(runtimeRoutes.planning);
      const writerModel = selectedRuntimeModel(runtimeRoutes.writer);
      const qaModel = selectedRuntimeModel(runtimeRoutes.qa);
      const next = await api.updateLLMCredentials({
        provider: "tokengate",
        api_key: apiKey,
        base_url: baseURL,
        model: planningModel,
        writer_model: writerModel,
        qa_model: qaModel,
        routes: runtimeRoutes,
      });
      setStatus(next);
      setBaseURL(next.base_url || defaultBaseURL);
      setRuntimeRoutes(normalizeRuntimeRoutes(next));
      setAPIKey("");
      setMessage({ title: "Runtime saved", detail: "The TokenGate key stays server-side; only the tail is shown.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Save failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function test() {
    setBusy("test");
    setMessage(null);
    setTestResult(null);
    try {
      setTestResult(await api.testLLMCredentials({ routes: runtimeRoutes }));
    } catch (e: any) {
      setTestResult({ ok: false, error: e.message });
    } finally {
      setBusy(null);
    }
  }

  async function remove() {
    if (!window.confirm("Remove the saved TokenGate runtime key? CiteLoop falls back to server-environment TokenGate settings until you save a new one.")) return;
    setBusy("delete");
    setMessage(null);
    setTestResult(null);
    try {
      const next = await api.deleteLLMCredentials();
      setStatus(next);
      setBaseURL(next.base_url || defaultBaseURL);
      setRuntimeRoutes(normalizeRuntimeRoutes(next));
      setAPIKey("");
      setMessage({ title: "Runtime key removed", detail: "CiteLoop now uses server-environment TokenGate settings until a key is saved.", tone: "amber" });
    } catch (e: any) {
      setMessage({ title: "Could not remove key", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  function updateRuntimeRoute(role: LLMRuntimeRole, next: Partial<LLMModelRoute>) {
    setRuntimeRoutes((current) => ({
      ...current,
      [role]: {
        ...current[role],
        ...next,
      },
    }));
  }

  function updateGeoDraft(scope: GEOProviderScope, next: Partial<GEODraft>) {
    setGeoDrafts((current) => ({ ...current, [scope]: { ...current[scope], ...next } }));
  }

  async function saveGeo(scope: GEOProviderScope) {
    const draft = geoDrafts[scope];
    setGeoBusy(`save-${scope}`);
    setMessage(null);
    setGeoTestResults((current) => ({ ...current, [scope]: null }));
    try {
      const saved = await api.updateGEOCredentials(scope, {
        provider: "tokengate",
        api_key: draft.apiKey,
        base_url: draft.baseURL,
        model: draft.model,
        enabled: draft.enabled,
      });
      setGeoStatuses((current) => ({ ...current, [scope]: saved }));
      setGeoDrafts((current) => ({
        ...current,
        [scope]: { apiKey: "", baseURL: saved.base_url || defaultBaseURL, model: saved.model || draft.model, enabled: saved.enabled },
      }));
      setMessage({ title: `${providerLabel(scope)} GEO provider saved`, detail: "The TokenGate key stays server-side; only the tail is shown.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "GEO provider save failed", detail: e.message, tone: "red" });
    } finally {
      setGeoBusy(null);
    }
  }

  async function testGeo(scope: GEOProviderScope) {
    setGeoBusy(`test-${scope}`);
    setMessage(null);
    setGeoTestResults((current) => ({ ...current, [scope]: null }));
    try {
      const result = await api.testGEOCredentials(scope);
      setGeoTestResults((current) => ({ ...current, [scope]: result }));
    } catch (e: any) {
      setGeoTestResults((current) => ({ ...current, [scope]: { ok: false, error: e.message } }));
    } finally {
      setGeoBusy(null);
    }
  }

  async function removeGeo(scope: GEOProviderScope) {
    if (!window.confirm(`Remove the saved TokenGate key for ${providerLabel(scope)}? GEO observation for this provider will be disabled until a new key is saved.`)) return;
    setGeoBusy(`delete-${scope}`);
    setMessage(null);
    setGeoTestResults((current) => ({ ...current, [scope]: null }));
    try {
      const removed = await api.deleteGEOCredentials(scope);
      setGeoStatuses((current) => ({ ...current, [scope]: removed }));
      setGeoDrafts((current) => ({
        ...current,
        [scope]: { apiKey: "", baseURL: removed.base_url || defaultBaseURL, model: removed.model || defaultGeoModel(scope), enabled: removed.enabled },
      }));
      setMessage({ title: `${providerLabel(scope)} GEO provider removed`, tone: "amber" });
    } catch (e: any) {
      setMessage({ title: "Could not remove GEO provider", detail: e.message, tone: "red" });
    } finally {
      setGeoBusy(null);
    }
  }

  const runtimeNeedsKey = !status?.configured;
  const selectedStatusLabel = status?.configured
    ? `TokenGate configured${status.key_tail ? ` ...${status.key_tail}` : ""}`
    : "Not configured";

  if (access === "loading") {
    return (
      <div className="grid min-h-[40vh] place-items-center text-sm text-slate-500">
        <span className="inline-flex items-center gap-2">
          <Loader2 size={16} className="animate-spin" />
          Checking admin access...
        </span>
      </div>
    );
  }

  if (access === "denied") {
    return (
      <div className="space-y-7">
        <SectionHeader title="Admin" eyebrow="Platform config" />
        <Notice title="Admin access required" detail="This area is limited to platform administrators." tone="amber" />
        <div className="rounded-xl border border-slate-200 bg-white p-5 text-sm text-slate-500">
          <ShieldCheck size={24} className="mb-3 text-slate-300" />
          Ask a platform administrator to add this account to the API admin allowlist.
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-7">
      <SectionHeader title="Admin" eyebrow="Platform config" />

      <div className="overflow-x-auto border-b border-slate-200">
        <div role="tablist" aria-label="Admin sections" className="flex min-w-max gap-6">
          {adminTabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              id={`admin-tab-${tab.id}`}
              role="tab"
              aria-selected={activeAdminTab === tab.id}
              aria-controls={`admin-panel-${tab.id}`}
              onClick={() => activateAdminTab(tab.id)}
              className={cx(
                "border-b-2 px-0 pb-3 pt-1 text-sm font-semibold transition-colors",
                activeAdminTab === tab.id ? "border-[#d93820] text-slate-950" : "border-transparent text-slate-500 hover:text-slate-900",
              )}
            >
              {tab.title}
            </button>
          ))}
        </div>
      </div>

      {activeAdminTab === "runtime" && (
        <section id="admin-panel-runtime" role="tabpanel" aria-labelledby="admin-tab-runtime" tabIndex={0} className="space-y-6">
          <SectionHeader title="Platform runtime" eyebrow="TokenGate model routing" />
          <section className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex items-center gap-2 text-sm font-semibold text-slate-900">
                <KeyRound size={16} />
                TokenGate API key
              </div>
              {status?.configured ? (
                <Badge tone="green">
                  <CheckCircle2 size={13} className="mr-1" />
                  {selectedStatusLabel}
                </Badge>
              ) : (
                <Badge tone="amber">Not configured</Badge>
              )}
            </div>

            <Field label="TokenGate API key" helper={runtimeNeedsKey ? "Required before CiteLoop can use live runtime generation." : "Leave blank to keep the existing key."}>
              <TextInput type="password" autoComplete="off" value={apiKey} placeholder="tg-..." onChange={(event) => setAPIKey(event.target.value)} />
            </Field>

            <Field label="Base URL" helper="Use the TokenGate API backend URL with /v1, not the dashboard URL.">
              <div className="relative">
                <Globe2 size={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
                <TextInput value={baseURL} className="w-full pl-9" placeholder={defaultBaseURL} onChange={(event) => setBaseURL(event.target.value)} />
              </div>
            </Field>

            <div className="grid gap-3">
              {runtimeRoleConfigs.map((config) => {
                const route = runtimeRoutes[config.role];
                return (
                  <section key={config.role} className="rounded-lg border border-slate-200 p-3">
                    <div className="grid gap-3 lg:grid-cols-[minmax(140px,0.8fr)_minmax(220px,1fr)_minmax(220px,1fr)_auto] lg:items-end">
                      <div>
                        <div className="text-sm font-bold text-slate-950">{config.label}</div>
                        <p className="mt-1 text-xs leading-5 text-slate-500">{config.helper}</p>
                        <div className="mt-2 inline-flex rounded-lg border border-slate-200 bg-slate-50 p-1">
                          {(["openai", "anthropic"] as LLMModelProvider[]).map((provider) => (
                            <button
                              key={provider}
                              type="button"
                              onClick={() => updateRuntimeRoute(config.role, { primary_provider: provider })}
                              className={cx(
                                "rounded-md px-2.5 py-1 text-xs font-bold transition-colors",
                                route.primary_provider === provider ? "bg-white text-slate-950 shadow-sm" : "text-slate-500 hover:text-slate-900",
                              )}
                              aria-pressed={route.primary_provider === provider}
                            >
                              {provider === "openai" ? "OpenAI" : "Anthropic"}
                            </button>
                          ))}
                        </div>
                      </div>
                      <Field label="OpenAI model" helper="Choose a TokenGate OpenAI-compatible model.">
                        <SelectInput
                          value={route.openai_model_alias}
                          className="w-full"
                          onChange={(event) => updateRuntimeRoute(config.role, { openai_model_alias: event.target.value })}
                        >
                          {tokengateModelOptionsWithCurrent(tokengateOpenAIModelOptions, route.openai_model_alias).map((model) => (
                            <option key={model} value={model}>
                              {model}
                            </option>
                          ))}
                        </SelectInput>
                      </Field>
                      <Field label="Anthropic model" helper="Choose a TokenGate Anthropic-compatible model.">
                        <SelectInput
                          value={route.anthropic_model_alias}
                          className="w-full"
                          onChange={(event) => updateRuntimeRoute(config.role, { anthropic_model_alias: event.target.value })}
                        >
                          {tokengateModelOptionsWithCurrent(tokengateAnthropicModelOptions, route.anthropic_model_alias).map((model) => (
                            <option key={model} value={model}>
                              {model}
                            </option>
                          ))}
                        </SelectInput>
                      </Field>
                      <label className="inline-flex min-w-[140px] items-center gap-2 pb-2 text-sm font-semibold text-slate-700">
                        <input
                          type="checkbox"
                          checked={route.fallback_enabled}
                          onChange={(event) => updateRuntimeRoute(config.role, { fallback_enabled: event.target.checked })}
                          className="h-4 w-4 rounded border-slate-300 text-[#d93820] focus:ring-[#d93820]"
                        />
                        Fallback
                      </label>
                    </div>
                  </section>
                );
              })}
            </div>

            <div className="flex flex-wrap gap-2">
              <Button disabled={busy !== null || (runtimeNeedsKey && apiKey.trim() === "") || baseURL.trim() === ""} variant="primary" onClick={save}>
                <ButtonProgress busy={busy === "save"} busyLabel="Saving" idleIcon={<Save size={16} />}>
                  Save credentials
                </ButtonProgress>
              </Button>
              <Button disabled={busy !== null || !status?.configured} onClick={test} title={status?.configured ? "Run a live connectivity check" : "Save a key first"}>
                <ButtonProgress busy={busy === "test"} busyLabel="Testing" idleIcon={<PlugZap size={16} />}>
                  Test connection
                </ButtonProgress>
              </Button>
              <Button disabled={busy !== null} onClick={refresh}>
                <RefreshCw size={16} />
                Refresh
              </Button>
              {status?.configured && (
                <Button disabled={busy !== null} variant="danger" onClick={remove}>
                  <ButtonProgress busy={busy === "delete"} busyLabel="Removing" idleIcon={<Trash2 size={16} />}>
                    Delete key
                  </ButtonProgress>
                </Button>
              )}
            </div>

            <ConnectionResult result={testResult} />
          </section>

          <Notice
            title="Secrets stay server-side"
            detail="Only the TokenGate base URL, model IDs, and key tail are returned to the browser. Saving takes effect immediately with no redeploy."
            tone="neutral"
          />
        </section>
      )}

      {activeAdminTab === "geo" && (
        <section id="admin-panel-geo" role="tabpanel" aria-labelledby="admin-tab-geo" tabIndex={0} className="space-y-6">
          <SectionHeader title="GEO providers" eyebrow="Answer and citation observation" />
          <Notice
            title="All provider keys come from TokenGate"
            detail="CiteLoop stores TokenGate-issued keys only. Provider routing is controlled by the model name configured for each provider in TokenGate."
            tone="neutral"
          />

          <div className="grid gap-4">
            {geoProviders.map((provider) => {
              const providerStatus = geoStatuses[provider.scope];
              const draft = geoDrafts[provider.scope];
              const needsKey = !providerStatus?.configured;
              const busySave = geoBusy === `save-${provider.scope}`;
              const busyTest = geoBusy === `test-${provider.scope}`;
              const busyDelete = geoBusy === `delete-${provider.scope}`;
              return (
                <section key={provider.scope} className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
                  <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                    <div>
                      <div className="flex items-center gap-2 text-sm font-bold text-slate-900">
                        <KeyRound size={16} />
                        {provider.label}
                      </div>
                      <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-500">{provider.helper}</p>
                    </div>
                    {providerStatus?.configured ? (
                      <Badge tone={providerStatus.enabled ? "green" : "amber"}>
                        {providerStatus.enabled ? "Enabled" : "Disabled"}
                        {providerStatus.key_tail ? ` ...${providerStatus.key_tail}` : ""}
                      </Badge>
                    ) : (
                      <Badge tone="amber">Not configured</Badge>
                    )}
                  </div>

                  <Field label={provider.keyLabel} helper={needsKey ? "Paste the TokenGate-issued key for this provider/model route." : "Leave blank to keep the existing key."}>
                    <TextInput
                      type="password"
                      autoComplete="off"
                      value={draft.apiKey}
                      placeholder="tg-..."
                      onChange={(event) => updateGeoDraft(provider.scope, { apiKey: event.target.value })}
                    />
                  </Field>

                  <div className="grid gap-3 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
                    <Field label="Base URL" helper="Use the TokenGate API backend URL with /v1.">
                      <div className="relative">
                        <Globe2 size={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
                        <TextInput
                          value={draft.baseURL}
                          className="w-full pl-9"
                          placeholder={defaultBaseURL}
                          onChange={(event) => updateGeoDraft(provider.scope, { baseURL: event.target.value })}
                        />
                      </div>
                    </Field>
                    <Field label="Model name" helper="TokenGate model or alias for this provider.">
                      <TextInput value={draft.model} placeholder={provider.defaultModel} onChange={(event) => updateGeoDraft(provider.scope, { model: event.target.value })} />
                    </Field>
                  </div>

                  <label className="inline-flex max-w-max items-center gap-2 text-sm font-semibold text-slate-700">
                    <input
                      type="checkbox"
                      checked={draft.enabled}
                      onChange={(event) => updateGeoDraft(provider.scope, { enabled: event.target.checked })}
                      className="h-4 w-4 rounded border-slate-300 text-[#d93820] focus:ring-[#d93820]"
                    />
                    Enabled for GEO workflows
                  </label>

                  <div className="flex flex-wrap gap-2">
                    <Button
                      disabled={geoBusy !== null || (needsKey && draft.apiKey.trim() === "") || draft.baseURL.trim() === "" || draft.model.trim() === ""}
                      variant="primary"
                      onClick={() => saveGeo(provider.scope)}
                    >
                      <ButtonProgress busy={busySave} busyLabel="Saving" idleIcon={<Save size={16} />}>
                        Save provider
                      </ButtonProgress>
                    </Button>
                    <Button disabled={geoBusy !== null || !providerStatus?.configured || !providerStatus.enabled} onClick={() => testGeo(provider.scope)}>
                      <ButtonProgress busy={busyTest} busyLabel="Testing" idleIcon={<PlugZap size={16} />}>
                        Test connection
                      </ButtonProgress>
                    </Button>
                    {providerStatus?.configured && (
                      <Button disabled={geoBusy !== null} variant="danger" onClick={() => removeGeo(provider.scope)}>
                        <ButtonProgress busy={busyDelete} busyLabel="Removing" idleIcon={<Trash2 size={16} />}>
                          Delete key
                        </ButtonProgress>
                      </Button>
                    )}
                  </div>

                  <ConnectionResult result={geoTestResults[provider.scope] ?? null} />
                </section>
              );
            })}
          </div>
        </section>
      )}

      {activeAdminTab === "incidents" && (
        <section id="admin-panel-incidents" role="tabpanel" aria-labelledby="admin-tab-incidents" tabIndex={0} className="space-y-6">
          <SectionHeader title="Runtime incidents" eyebrow="Platform AI operations" action={<Badge tone={runtimeIncidents.length ? "red" : "green"}>{runtimeIncidents.length}</Badge>} />
          <Notice
            title="Admin-only platform failures"
            detail="TokenGate and model-provider connectivity failures stay out of the customer Activity Log. Fix them from Platform runtime, then refresh context."
            tone="neutral"
          />
          {runtimeIncidents.length === 0 ? (
            <EmptyState title="No runtime incidents" detail="TokenGate and model-provider connectivity failures will appear here for administrators." />
          ) : (
            <div className="grid gap-3">
              {runtimeIncidents.map((run) => (
                <RuntimeIncidentRow key={run.id} run={run} />
              ))}
            </div>
          )}
        </section>
      )}
    </div>
  );
}
