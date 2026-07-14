import type { ResultsFeedItem, ResultsSiteFixMeasurementDetail, ResultsSiteFixSummary } from "./api";

export type SiteFixOutcomeKey = "waiting" | "positive" | "negative" | "mixed" | "inconclusive" | "insufficient_data";
export type SiteFixQueueKey = "waiting" | "too_early" | "blocked" | "completed";

export type SiteFixDeepLinkState = {
  measurementID: string;
  status: "pending" | "resolved" | "failed";
  feedSettled: boolean;
  detailFailed: boolean;
  pinnedSummary: ResultsSiteFixSummary | null;
  detail: ResultsSiteFixMeasurementDetail | null;
};

export type SiteFixDeepLinkEvent =
  | { type: "feed_settled"; items: ResultsFeedItem[] }
  | { type: "detail_succeeded"; detail: ResultsSiteFixMeasurementDetail }
  | { type: "detail_failed" };

export function createSiteFixDeepLinkState(measurementID: string): SiteFixDeepLinkState {
  return {
    measurementID,
    status: "pending",
    feedSettled: false,
    detailFailed: false,
    pinnedSummary: null,
    detail: null,
  };
}

export function reduceSiteFixDeepLinkState(
  state: SiteFixDeepLinkState,
  event: SiteFixDeepLinkEvent,
): SiteFixDeepLinkState {
  if (state.status === "resolved") return state;
  if (event.type === "feed_settled" && state.feedSettled) return state;
  if (event.type === "detail_succeeded") {
    if (event.detail.measurement.id !== state.measurementID) return state;
    return {
      ...state,
      status: "resolved",
      pinnedSummary: event.detail.measurement,
      detail: event.detail,
    };
  }
  if (event.type === "detail_failed") {
    return {
      ...state,
      status: state.feedSettled ? "failed" : "pending",
      detailFailed: true,
    };
  }

  const match = event.items.find(
    (item): item is ResultsSiteFixSummary => item.source_type === "site_fix" && item.id === state.measurementID,
  );
  if (match) {
    return {
      ...state,
      status: "resolved",
      feedSettled: true,
      pinnedSummary: match,
    };
  }
  return {
    ...state,
    status: state.detailFailed ? "failed" : "pending",
    feedSettled: true,
  };
}

export function pinSiteFixResultsSummary(
  items: ResultsFeedItem[],
  pinnedSummary: ResultsSiteFixSummary | null,
): ResultsFeedItem[] {
  if (!pinnedSummary) return items;
  return [
    pinnedSummary,
    ...items.filter((item) => !(item.source_type === "site_fix" && item.id === pinnedSummary.id)),
  ];
}

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
