"use client";

import { useCallback, useEffect, useState } from "react";
import { CheckCircle2, Globe2, KeyRound, Loader2, PlugZap, Save, ShieldCheck, Trash2, XCircle } from "lucide-react";
import { LLMCredentialsStatus, LLMProvider } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";
import { Badge, Button, ButtonProgress, cx, Field, Notice, SectionHeader, TextInput } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type TestResult = { ok: boolean; provider?: string; model?: string; latency_ms?: number; sample?: string; error?: string } | null;

const defaultBaseURLs: Record<Exclude<LLMProvider, "claude">, string> = {
  tokengate: "https://tokengate-production.up.railway.app/v1",
  openai: "https://api.openai.com/v1",
};

const providers: Array<{ value: LLMProvider; label: string; helper: string }> = [
  { value: "tokengate", label: "TokenGate", helper: "OpenAI-compatible gateway" },
  { value: "openai", label: "OpenAI", helper: "Chat Completions" },
  { value: "claude", label: "Anthropic Claude API", helper: "Anthropic API" },
];

function providerLabel(value: LLMProvider) {
  return providers.find((item) => item.value === value)?.label ?? "TokenGate";
}

function defaultBaseURL(provider: LLMProvider) {
  return provider === "claude" ? "" : defaultBaseURLs[provider];
}

export function AdminClient() {
  const api = useApi();
  const [access, setAccess] = useState<"loading" | "granted" | "denied">("loading");
  const [status, setStatus] = useState<LLMCredentialsStatus | null>(null);
  const [provider, setProvider] = useState<LLMProvider>("tokengate");
  const [apiKey, setAPIKey] = useState("");
  const [baseURL, setBaseURL] = useState(defaultBaseURLs.tokengate);
  const [busy, setBusy] = useState<"save" | "test" | "delete" | null>(null);
  const { notify } = useToast();
  const setMessage = (next: Message) => {
    if (next) notify(next);
  };
  const [testResult, setTestResult] = useState<TestResult>(null);

  const refresh = useCallback(async () => {
    try {
      const me = await api.getMe();
      if (!me.is_admin) {
        setAccess("denied");
        return;
      }
      setAccess("granted");
      const next = await api.getLLMCredentials();
      setStatus(next);
      setProvider(next.provider);
      setBaseURL(next.base_url || defaultBaseURL(next.provider));
    } catch (e: any) {
      if (String(e.message).includes("403")) {
        setAccess("denied");
      } else {
        setAccess("granted");
        setMessage({ title: "Admin settings unavailable", detail: e.message, tone: "amber" });
      }
    }
  }, [api]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  async function save() {
    setBusy("save");
    setMessage(null);
    setTestResult(null);
    try {
      const next = await api.updateLLMCredentials({
        provider,
        api_key: apiKey,
        base_url: provider === "claude" ? undefined : baseURL,
      });
      setStatus(next);
      setProvider(next.provider);
      setBaseURL(next.base_url || defaultBaseURL(next.provider));
      setAPIKey("");
      setMessage({ title: "Provider saved", detail: "The key is stored server-side; only the tail is shown. Run Test to confirm connectivity.", tone: "green" });
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
      setTestResult(await api.testLLMCredentials());
    } catch (e: any) {
      setTestResult({ ok: false, error: e.message });
    } finally {
      setBusy(null);
    }
  }

  async function remove() {
    if (!window.confirm("Remove the saved provider key? CiteLoop falls back to the server-environment provider until you save a new one.")) return;
    setBusy("delete");
    setMessage(null);
    setTestResult(null);
    try {
      const next = await api.deleteLLMCredentials();
      setStatus(next);
      setProvider(next.provider);
      setBaseURL(next.base_url || defaultBaseURL(next.provider));
      setAPIKey("");
      setMessage({ title: "Provider key removed", detail: "CiteLoop now uses the server-environment provider until a key is saved.", tone: "amber" });
    } catch (e: any) {
      setMessage({ title: "Could not remove key", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  const providerChanged = Boolean(status?.configured && status.provider !== provider);
  const needsKey = !status?.configured || providerChanged;
  const needsBaseURL = provider !== "claude";
  const selectedStatusLabel = status?.configured
    ? `${providerLabel(status.provider)} configured${status.key_tail ? ` ...${status.key_tail}` : ""}`
    : "Not configured";

  function selectProvider(value: LLMProvider) {
    setProvider(value);
    setTestResult(null);
    setBaseURL(status?.provider === value && status.base_url ? status.base_url : defaultBaseURL(value));
  }

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
        <SectionHeader title="Admin" eyebrow="LLM provider" />
        <Notice
          title="Admin access required"
          detail="This area is limited to platform administrators."
          tone="amber"
        />
        <div className="rounded-xl border border-slate-200 bg-white p-5 text-sm text-slate-500">
          <ShieldCheck size={24} className="mb-3 text-slate-300" />
          Ask a platform administrator to add this account to the API admin allowlist.
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-7">
      <SectionHeader title="Admin" eyebrow="LLM provider" />

      <section className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2 text-sm font-semibold text-slate-900">
            <KeyRound size={16} />
            API key
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

        <div className="grid gap-3 md:grid-cols-3">
          {providers.map((item) => {
            const active = provider === item.value;
            return (
              <button
                type="button"
                key={item.value}
                onClick={() => selectProvider(item.value)}
                className={cx(
                  "flex min-h-[68px] items-center justify-between rounded-xl border px-4 py-3 text-left transition-colors",
                  active ? "border-[#d93820] bg-red-50 text-slate-950" : "border-slate-200 bg-white text-slate-700 hover:bg-slate-50",
                )}
              >
                <span className="min-w-0">
                  <span className="block text-sm font-bold">{item.label}</span>
                  <span className="mt-1 block text-xs font-semibold text-slate-500">{item.helper}</span>
                  {status?.configured && status.provider === item.value && (
                    <span className="mt-1.5 inline-flex items-center gap-1 rounded bg-green-100 px-1.5 py-0.5 text-[11px] font-bold text-green-700">
                      <span className="h-1.5 w-1.5 rounded-full bg-green-500" />
                      Active{status.key_tail ? ` · ...${status.key_tail}` : ""}
                    </span>
                  )}
                </span>
                <span className={cx("ml-2 grid h-5 w-5 shrink-0 place-items-center rounded-full border", active ? "border-[#d93820] bg-[#d93820]" : "border-slate-300 bg-white")}>
                  {active && <span className="h-2 w-2 rounded-full bg-white" />}
                </span>
              </button>
            );
          })}
        </div>

        <Field
          label={provider === "claude" ? "Anthropic Claude API key" : `${providerLabel(provider)} API key`}
          helper={needsKey ? "Required for this provider." : "Leave blank to keep the existing key."}
        >
          <TextInput
            type="password"
            autoComplete="off"
            value={apiKey}
            placeholder={provider === "claude" ? "sk-ant-..." : "sk-..."}
            onChange={(event) => setAPIKey(event.target.value)}
          />
        </Field>

        {needsBaseURL && (
          <Field label="Base URL" helper="Use the API backend URL with /v1, not the dashboard URL.">
            <div className="relative">
              <Globe2 size={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
              <TextInput value={baseURL} className="w-full pl-9" placeholder={defaultBaseURL(provider)} onChange={(event) => setBaseURL(event.target.value)} />
            </div>
          </Field>
        )}

        <div className="flex flex-wrap gap-2">
          <Button
            disabled={busy !== null || (needsKey && apiKey.trim() === "") || (needsBaseURL && baseURL.trim() === "")}
            variant="primary"
            onClick={save}
          >
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

        {testResult && (
          <div
            className={cx(
              "rounded-lg border p-3 text-sm",
              testResult.ok ? "border-green-200 bg-green-50 text-green-900" : "border-red-200 bg-red-50 text-red-900",
            )}
          >
            <div className="inline-flex items-center gap-2 font-bold">
              {testResult.ok ? <CheckCircle2 size={15} /> : <XCircle size={15} />}
              {testResult.ok ? "Connection OK" : "Connection failed"}
            </div>
            <div className="mt-1 text-xs leading-5">
              {testResult.ok ? (
                <>
                  {testResult.provider} · {testResult.model || "model n/a"}
                  {typeof testResult.latency_ms === "number" ? ` · ${testResult.latency_ms} ms` : ""}
                  {testResult.sample ? ` · replied "${testResult.sample}"` : ""}
                </>
              ) : (
                testResult.error || "Unknown error"
              )}
            </div>
          </div>
        )}
      </section>

      <Notice
        title="Secrets stay server-side"
        detail="Only the provider, base URL, and key tail are returned to the browser. Saving takes effect immediately — no redeploy needed."
        tone="neutral"
      />
    </div>
  );
}
