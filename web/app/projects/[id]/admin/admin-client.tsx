"use client";

import { useCallback, useEffect, useState } from "react";
import { CheckCircle2, Globe2, KeyRound, Loader2, PlugZap, Save, ShieldCheck, Trash2, XCircle } from "lucide-react";
import { LLMCredentialsStatus } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";
import { Badge, Button, ButtonProgress, cx, Field, Notice, SectionHeader, TextInput } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type TestResult = { ok: boolean; provider?: string; model?: string; latency_ms?: number; sample?: string; error?: string } | null;

const defaultBaseURL = "https://tokengate-production.up.railway.app/v1";

export function AdminClient() {
  const api = useApi();
  const [access, setAccess] = useState<"loading" | "granted" | "denied">("loading");
  const [status, setStatus] = useState<LLMCredentialsStatus | null>(null);
  const [apiKey, setAPIKey] = useState("");
  const [baseURL, setBaseURL] = useState(defaultBaseURL);
  const [model, setModel] = useState("");
  const [writerModel, setWriterModel] = useState("");
  const [qaModel, setQAModel] = useState("");
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
      setBaseURL(next.base_url || defaultBaseURL);
      setModel(next.model ?? "");
      setWriterModel(next.writer_model ?? "");
      setQAModel(next.qa_model ?? "");
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
        provider: "tokengate",
        api_key: apiKey,
        base_url: baseURL,
        model,
        writer_model: writerModel,
        qa_model: qaModel,
      });
      setStatus(next);
      setBaseURL(next.base_url || defaultBaseURL);
      setModel(next.model ?? "");
      setWriterModel(next.writer_model ?? "");
      setQAModel(next.qa_model ?? "");
      setAPIKey("");
      setMessage({ title: "TokenGate saved", detail: "The key is stored server-side; only the tail is shown. Run Test to confirm connectivity.", tone: "green" });
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
    if (!window.confirm("Remove the saved TokenGate key? CiteLoop falls back to server-environment TokenGate settings until you save a new one.")) return;
    setBusy("delete");
    setMessage(null);
    setTestResult(null);
    try {
      const next = await api.deleteLLMCredentials();
      setStatus(next);
      setBaseURL(next.base_url || defaultBaseURL);
      setModel(next.model ?? "");
      setWriterModel(next.writer_model ?? "");
      setQAModel(next.qa_model ?? "");
      setAPIKey("");
      setMessage({ title: "TokenGate key removed", detail: "CiteLoop now uses server-environment TokenGate settings until a key is saved.", tone: "amber" });
    } catch (e: any) {
      setMessage({ title: "Could not remove key", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  const needsKey = !status?.configured;
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
        <SectionHeader title="Admin" eyebrow="TokenGate" />
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
      <SectionHeader title="Admin" eyebrow="TokenGate" />

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

        <Field
          label="TokenGate API key"
          helper={needsKey ? "Required before CiteLoop can use live generation." : "Leave blank to keep the existing key."}
        >
          <TextInput
            type="password"
            autoComplete="off"
            value={apiKey}
            placeholder="sk-..."
            onChange={(event) => setAPIKey(event.target.value)}
          />
        </Field>

        <Field label="Base URL" helper="Use the TokenGate API backend URL with /v1, not the dashboard URL.">
          <div className="relative">
            <Globe2 size={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
            <TextInput value={baseURL} className="w-full pl-9" placeholder={defaultBaseURL} onChange={(event) => setBaseURL(event.target.value)} />
          </div>
        </Field>

        <div className="grid gap-3 md:grid-cols-3">
          <Field label="Default model" helper="Used by context extraction and planning. Falls back to TOKENGATE_MODEL when blank.">
            <TextInput value={model} placeholder="gpt-5.1" onChange={(event) => setModel(event.target.value)} />
          </Field>
          <Field label="Writer model" helper="Used for draft generation and AI repair. Falls back to the default model.">
            <TextInput value={writerModel} placeholder={model || "gpt-5.1-mini"} onChange={(event) => setWriterModel(event.target.value)} />
          </Field>
          <Field label="QA model" helper="Used for evidence checks and review requalification. Falls back to the default model.">
            <TextInput value={qaModel} placeholder={model || "gpt-5.1"} onChange={(event) => setQAModel(event.target.value)} />
          </Field>
        </div>

        <div className="flex flex-wrap gap-2">
          <Button
            disabled={busy !== null || (needsKey && apiKey.trim() === "") || baseURL.trim() === ""}
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
        detail="Only the TokenGate base URL, model IDs, and key tail are returned to the browser. Saving takes effect immediately — no redeploy needed."
        tone="neutral"
      />
    </div>
  );
}
