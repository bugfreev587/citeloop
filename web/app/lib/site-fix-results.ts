import type { ResultsSiteFixSummary } from "./api";

export type SiteFixOutcomeKey = "waiting" | "positive" | "negative" | "mixed" | "inconclusive" | "insufficient_data";
export type SiteFixQueueKey = "waiting" | "too_early" | "blocked" | "completed";

export function siteFixMeasurementOutcomeState(
  siteFix: Pick<ResultsSiteFixSummary, "status" | "terminal_outcome">,
): { key: SiteFixOutcomeKey; attention: boolean } {
  const outcome = siteFix.terminal_outcome;
  if (siteFix.status !== "terminal" || !outcome) return { key: "waiting", attention: false };
  return {
    key: outcome,
    attention: ["negative", "mixed", "inconclusive", "insufficient_data"].includes(outcome),
  };
}

export function siteFixMeasurementQueueState(
  siteFix: Pick<ResultsSiteFixSummary, "status">,
): SiteFixQueueKey {
  switch (siteFix.status) {
    case "terminal":
      return "completed";
    case "baseline_blocked":
    case "failed_retryable":
    case "failed_terminal":
      return "blocked";
    case "observing":
      return "waiting";
    case "planned":
    case "ready":
    default:
      return "too_early";
  }
}
