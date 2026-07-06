import type { GenerationRun } from "./normalize";

function readableValue(value: unknown) {
  if (value == null) return "";
  if (typeof value === "string") return value.trim();
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return "";
  }
}

export function activityRawError(run: GenerationRun) {
  const output = run.output ?? {};
  const candidates = [
    run.error,
    output.error,
    output.raw_error,
    output.message,
    output.detail,
    output.reason,
    output.failure,
    output.failure_reason,
  ];
  const found = candidates.map(readableValue).find(Boolean);
  if (found) return found;
  if (run.status === "budget_stopped") return "Budget guardrail stopped this automation before it could finish.";
  if (run.output?.degraded) return "The run completed with degraded output quality, but no raw error was recorded.";
  return "No raw error was recorded for this automation run.";
}

export function activityIsAttentionRun(run: GenerationRun) {
  return ["error", "failed", "budget_stopped"].includes(run.status) || Boolean(run.output?.degraded);
}

export function isPlatformRuntimeFailure(run: GenerationRun) {
  if (!["error", "failed"].includes(run.status)) return false;
  const errorText = activityRawError(run).toLowerCase();
  if (!errorText) return false;

  const mentionsCompletionEndpoint = errorText.includes("/chat/completions") || errorText.includes("chat/completions");
  const mentionsRuntimeProvider = /\b(tokengate|llm|model provider|completion api|openai|anthropic|claude|gemini)\b/.test(errorText);
  const isConnectivityFailure =
    errorText.includes("context deadline exceeded") ||
    errorText.includes("deadline exceeded") ||
    errorText.includes("timed out") ||
    errorText.includes("timeout") ||
    errorText.includes("connection refused") ||
    errorText.includes("bad gateway") ||
    errorText.includes("service unavailable") ||
    errorText.includes("gateway timeout");

  return mentionsCompletionEndpoint || (mentionsRuntimeProvider && isConnectivityFailure);
}

function createdAtMillis(run: GenerationRun) {
  if (!run.created_at) return Number.NaN;
  return Date.parse(run.created_at);
}

function isInsightRun(run: GenerationRun) {
  return run.agent === "insight";
}

function isSuccessfulContextRefresh(run: GenerationRun) {
  if (!isInsightRun(run) || run.status !== "ok") return false;
  const input = run.input ?? {};
  const output = run.output ?? {};
  const step = readableValue(input.step);
  return (
    (step === "profile" && Boolean(output.profile || output.profile_stage)) ||
    (step === "crawl_summary" && Boolean(output.crawl_summary))
  );
}

export function isSupersededContextFailure(run: GenerationRun, runs: GenerationRun[]) {
  if (!isInsightRun(run) || !activityIsAttentionRun(run)) return false;
  const failedAt = createdAtMillis(run);
  if (!Number.isFinite(failedAt)) return false;
  return runs.some((candidate) => {
    if (candidate.id === run.id || !isSuccessfulContextRefresh(candidate)) return false;
    const succeededAt = createdAtMillis(candidate);
    return Number.isFinite(succeededAt) && succeededAt > failedAt;
  });
}

export function isUserVisibleActivityRun(run: GenerationRun, runs: GenerationRun[]) {
  return !isPlatformRuntimeFailure(run) && !isSupersededContextFailure(run, runs);
}

export function isUserAttentionRun(run: GenerationRun, runs: GenerationRun[]) {
  return isUserVisibleActivityRun(run, runs) && activityIsAttentionRun(run);
}

export function userVisibleActivityRuns(runs: GenerationRun[]) {
  return runs.filter((run) => isUserVisibleActivityRun(run, runs));
}

export function activePlatformRuntimeIncidents(runs: GenerationRun[]) {
  return runs.filter((run) => isPlatformRuntimeFailure(run) && !isSupersededContextFailure(run, runs));
}
