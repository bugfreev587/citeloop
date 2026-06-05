"use client";

import { useCallback, useEffect, useState } from "react";
import { CheckCircle2, Globe2, KeyRound, Save } from "lucide-react";
import { LLMCredentialsStatus, LLMProvider } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, cx, Field, Notice, SectionHeader, TextInput } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

const defaultBaseURLs: Record<Exclude<LLMProvider, "claude">, string> = {
  tokengate: "https://tokengate-production.up.railway.app/v1",
  openai: "https://api.openai.com/v1",
};

const providers: Array<{ value: LLMProvider; label: string; helper: string }> = [
  { value: "tokengate", label: "TokenGate", helper: "OpenAI-compatible gateway" },
  { value: "openai", label: "OpenAI", helper: "Chat Completions" },
  { value: "claude", label: "Claude Code", helper: "Anthropic API" },
];

function providerLabel(value: LLMProvider) {
  return providers.find((item) => item.value === value)?.label ?? "TokenGate";
}

function defaultBaseURL(provider: LLMProvider) {
  return provider === "claude" ? "" : defaultBaseURLs[provider];
}

export function AdminClient() {
  const api = useApi();
  const [status, setStatus] = useState<LLMCredentialsStatus | null>(null);
  const [provider, setProvider] = useState<LLMProvider>("tokengate");
  const [apiKey, setAPIKey] = useState("");
  const [baseURL, setBaseURL] = useState(defaultBaseURLs.tokengate);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    try {
      const next = await api.getLLMCredentials();
      setStatus(next);
      setProvider(next.provider);
      setBaseURL(next.base_url || defaultBaseURL(next.provider));
    } catch (e: any) {
      setMessage({ title: "Admin settings unavailable", detail: e.message, tone: "amber" });
    }
  }, [api]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  async function save() {
    setBusy(true);
    setMessage(null);
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
      setMessage({ title: "LLM credentials saved", detail: "The key is stored server-side and only the tail is shown.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Save failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(false);
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
    setBaseURL(status?.provider === value && status.base_url ? status.base_url : defaultBaseURL(value));
  }

  return (
    <div className="space-y-7">
      <SectionHeader title="Admin" eyebrow="LLM provider" />
      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

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
                <span>
                  <span className="block text-sm font-bold">{item.label}</span>
                  <span className="mt-1 block text-xs font-semibold text-slate-500">{item.helper}</span>
                </span>
                <span
                  className={cx(
                    "grid h-5 w-5 place-items-center rounded-full border",
                    active ? "border-[#d93820] bg-[#d93820]" : "border-slate-300 bg-white",
                  )}
                >
                  {active && <span className="h-2 w-2 rounded-full bg-white" />}
                </span>
              </button>
            );
          })}
        </div>

        <Field
          label={provider === "claude" ? "Claude Code / Anthropic API key" : `${providerLabel(provider)} API key`}
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
          <Field
            label="Base URL"
            helper="Use the API backend URL with /v1, not the dashboard URL."
          >
            <div className="relative">
              <Globe2 size={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
              <TextInput
                value={baseURL}
                className="w-full pl-9"
                placeholder={defaultBaseURL(provider)}
                onChange={(event) => setBaseURL(event.target.value)}
              />
            </div>
          </Field>
        )}

        <div className="flex flex-wrap gap-2">
          <Button
            disabled={busy || (needsKey && apiKey.trim() === "") || (needsBaseURL && baseURL.trim() === "")}
            variant="primary"
            onClick={save}
          >
            <Save size={16} />
            Save credentials
          </Button>
          <Button disabled={busy} onClick={refresh}>
            Refresh
          </Button>
        </div>
      </section>

      <Notice
        title="Secrets stay server-side"
        detail="Only the provider, base URL, and key tail are returned to the browser after saving."
        tone="neutral"
      />
    </div>
  );
}
