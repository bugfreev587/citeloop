"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useState } from "react";
import { AlertCircle, ArrowRight, CheckCircle2, Circle, Loader2, Plus } from "lucide-react";
import { Badge, Button, Field, Notice, TextInput, cx } from "./components/ui";
import type { Project } from "./lib/api";
import { useApi } from "./lib/use-api";

type StepState = "pending" | "active" | "done" | "error";

type OnboardingStep = {
  key: "project" | "insight" | "seo";
  label: string;
  state: StepState;
};

const initialSteps: OnboardingStep[] = [
  { key: "project", label: "Create project", state: "pending" },
  { key: "insight", label: "Start product profile job", state: "pending" },
  { key: "seo", label: "Start SEO baseline job", state: "pending" },
];

function normalizeSiteURL(value: string) {
  const trimmed = value.trim();
  if (!trimmed) return "";
  const withScheme = /^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;
  const parsed = new URL(withScheme);
  return parsed.toString().replace(/\/$/, "");
}

function updateStep(steps: OnboardingStep[], key: OnboardingStep["key"], state: StepState) {
  return steps.map((step) => (step.key === key ? { ...step, state } : step));
}

function StepIcon({ state }: { state: StepState }) {
  if (state === "done") return <CheckCircle2 size={15} className="text-green-600" />;
  if (state === "active") return <Loader2 size={15} className="animate-spin text-[#d93820]" />;
  if (state === "error") return <AlertCircle size={15} className="text-red-600" />;
  return <Circle size={15} className="text-slate-300" />;
}

export function ProjectCreateForm() {
  const api = useApi();
  const router = useRouter();
  const [siteURL, setSiteURL] = useState("");
  const [steps, setSteps] = useState(initialSteps);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createdProject, setCreatedProject] = useState<Project | null>(null);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setError(null);
    setCreatedProject(null);
    let normalizedURL = "";
    try {
      normalizedURL = normalizeSiteURL(siteURL);
    } catch {
      setError("Enter a valid service URL.");
      return;
    }
    if (!normalizedURL) {
      setError("Service URL is required.");
      return;
    }

    setBusy(true);
    setSteps(updateStep(initialSteps, "project", "active"));
    let project: Project | null = null;
    try {
      project = await api.createProject({ site_url: normalizedURL });
      setCreatedProject(project);
      setSteps((current) =>
        updateStep(updateStep(updateStep(current, "project", "done"), "insight", "done"), "seo", "done"),
      );
      router.push(`/projects/${project.id}`);
    } catch (e: any) {
      setSteps((current) => current.map((step) => (step.state === "active" ? { ...step, state: "error" } : step)));
      if (project) {
        setCreatedProject(project);
      }
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4">
      <div>
        <div className="flex items-center justify-between gap-3">
          <div className="text-sm font-bold text-slate-900">Connect service</div>
          {busy && <Badge tone="amber">Working</Badge>}
        </div>
        <div className="mt-1 text-sm text-slate-500">Start with the URL customers already see.</div>
      </div>
      {error && <Notice title="Onboarding stopped" detail={error} tone="red" />}
      {error && createdProject && (
        <Link
          href={`/projects/${createdProject.id}`}
          className="inline-flex h-10 items-center justify-center gap-2 rounded-xl border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700 transition-all duration-150 hover:bg-slate-50 hover:text-slate-950 active:scale-[0.97]"
        >
          Open project
          <ArrowRight size={16} />
        </Link>
      )}
      <Field label="Service URL">
        <TextInput
          value={siteURL}
          onChange={(event) => setSiteURL(event.target.value)}
          placeholder="https://unipost.dev"
          disabled={busy}
        />
      </Field>
      {(busy || steps.some((step) => step.state !== "pending")) && (
        <div className="grid gap-2 rounded-lg border border-slate-100 bg-slate-50 px-3 py-2">
          {steps.map((step) => (
            <div key={step.key} className={cx("flex items-center gap-2 text-sm", step.state === "pending" ? "text-slate-400" : "text-slate-700")}>
              <StepIcon state={step.state} />
              <span className="font-medium">{step.label}</span>
            </div>
          ))}
        </div>
      )}
      <Button disabled={busy} variant="primary" type="submit">
        <Plus size={16} />
        {busy ? "Connecting" : "Connect"}
      </Button>
    </form>
  );
}
