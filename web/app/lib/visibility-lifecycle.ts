import type { VisibilityActionInLoop, VisibilityLifecycleCounts, VisibilityLifecycleStage } from "./api";

export type VisibilityLifecycleInput = Partial<
  Pick<
    VisibilityActionInLoop,
    | "status"
    | "lifecycle_stage"
    | "topic_id"
    | "topic_status"
    | "draft_article_id"
    | "draft_article_status"
    | "published_at"
    | "verified_at"
    | "outcome_summary"
    | "output_snapshot"
    | "diff_snapshot"
  >
>;

export const visibilityLifecycleStages: VisibilityLifecycleStage[] = [
  "detected",
  "added_to_plan",
  "planned",
  "drafting",
  "ready_for_review",
  "approved",
  "published_or_applied",
  "measuring",
  "learned",
  "blocked",
];

function rawHasData(value: any) {
  if (!value) return false;
  if (typeof value === "string") {
    const trimmed = value.trim();
    return trimmed !== "" && trimmed !== "{}" && trimmed !== "[]" && trimmed !== "null";
  }
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === "object") return Object.keys(value).length > 0;
  return true;
}

export function deriveVisibilityLifecycleStage(input: VisibilityLifecycleInput): VisibilityLifecycleStage {
  if (input.lifecycle_stage && visibilityLifecycleStages.includes(input.lifecycle_stage)) {
    return input.lifecycle_stage;
  }

  const status = String(input.status ?? "").trim().toLowerCase();
  const draftStatus = String(input.draft_article_status ?? "").trim().toLowerCase();

  if (["failed", "verification_failed", "recovery_required"].includes(status)) return "blocked";
  if (status === "completed") return "learned";
  if (status === "measuring") return "measuring";
  if (status === "published" || input.published_at || input.verified_at || draftStatus === "published") return "published_or_applied";
  if (status === "drafting") return "drafting";
  if (input.draft_article_id) return "ready_for_review";
  if (status === "approved") return input.topic_id ? "planned" : "approved";
  if (status === "ready_for_review") {
    if (rawHasData(input.diff_snapshot) || rawHasData(input.output_snapshot)) return "ready_for_review";
    if (input.topic_id || input.topic_status) return "planned";
    return "added_to_plan";
  }
  if (status === "open") return "detected";
  return "added_to_plan";
}

export function visibilityLifecycleCounts(items: VisibilityLifecycleInput[] = []): VisibilityLifecycleCounts {
  return items.reduce(
    (counts, item) => {
      counts[deriveVisibilityLifecycleStage(item)] += 1;
      return counts;
    },
    visibilityLifecycleStages.reduce((counts, stage) => {
      counts[stage] = 0;
      return counts;
    }, {} as VisibilityLifecycleCounts),
  );
}
