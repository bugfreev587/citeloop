export type ContentPlanTopic = {
  id: string;
  channel: string;
  target_keyword: string | null;
  target_prompt: string | null;
  angle: string | null;
  format: string | null;
  priority: number;
  internal_links: any[];
  status: string;
  scheduled_at: string | null;
  created_at: string | null;
};

export type ContentPlanReviewDraft = {
  draft_article_id?: string | null;
  draft_article_status?: string | null;
};

export type ContentPlanLoopAction = {
  status?: string | null;
  lifecycle_stage?: string | null;
  opportunity_status?: string | null;
};

export type ContentPlanPublishStrategy = "blog" | "syndication" | "both";

export type ContentPlanPublishAction = {
  asset_type?: string | null;
  work_type?: string | null;
  action_type?: string | null;
  opportunity_type?: string | null;
  opportunity_recommended_action?: string | null;
  opportunity_expected_impact?: string | null;
  opportunity_query?: string | null;
  input_snapshot?: any;
  output_snapshot?: any;
  diff_snapshot?: any;
  evidence_snapshot?: any;
};

export type ContentPlanPageUpdateDraft = {
  status?: string | null;
  publisher_result?: any;
};

export function isPageUpdateAction(action: ContentPlanPublishAction | null | undefined) {
  if (!action) return false;
  const assetType = String(action.asset_type ?? "").trim().toLowerCase();
  const workType = String(action.work_type ?? "").trim().toLowerCase();
  const outputType = String(action.output_snapshot?.output_type ?? action.diff_snapshot?.output_type ?? "").trim().toLowerCase();
  return workType === "improve_page" || assetType === "page_update" || outputType === "page_update_diff";
}

export function contentPlanActionPublishControlsVisible(action: ContentPlanPublishAction | null | undefined) {
  return !isPageUpdateAction(action);
}

export function contentPlanActionPrimaryCTA(action: ContentPlanPublishAction | null | undefined) {
  return isPageUpdateAction(action) ? "Draft Update" : "Draft Content";
}

export function contentPlanActionBusyCTA(action: ContentPlanPublishAction | null | undefined) {
  return isPageUpdateAction(action) ? "Drafting update" : "Drafting";
}

export function contentPlanActionSurfaceLabel(action: ContentPlanPublishAction | null | undefined) {
  return isPageUpdateAction(action) ? "Page updates" : "Content briefs";
}

export function pageUpdateDraftIDForAction(action: ContentPlanPublishAction | null | undefined) {
  const raw =
    action?.output_snapshot?.page_update_draft_id ??
    action?.diff_snapshot?.page_update_draft_id ??
    action?.input_snapshot?.page_update_draft_id;
  return typeof raw === "string" && raw.trim() ? raw.trim() : null;
}

export function pageUpdateDraftGitHubPRURL(draft: ContentPlanPageUpdateDraft | null | undefined) {
  const raw = draft?.publisher_result?.github_pr_url;
  if (draft?.publisher_result?.mode !== "github_pr" || typeof raw !== "string") return null;
  const trimmed = raw.trim();
  return trimmed.startsWith("https://github.com/") ? trimmed : null;
}

export function pageUpdateDraftHasOpenGitHubPR(draft: ContentPlanPageUpdateDraft | null | undefined) {
  const url = pageUpdateDraftGitHubPRURL(draft);
  const resultStatus = String(draft?.publisher_result?.status ?? "").trim().toLowerCase();
  const prState = String(draft?.publisher_result?.github_pr_state ?? "").trim().toLowerCase();
  return Boolean(url && (resultStatus === "github_pr_open" || prState === "open"));
}

export function pageUpdateDraftPrimaryCTA(draft: ContentPlanPageUpdateDraft | null | undefined) {
  if (pageUpdateDraftHasOpenGitHubPR(draft)) return "Open PR";
  switch (draft?.status) {
    case "ready_for_review":
      return "Approve Update";
    case "approved":
      return "Apply Update";
    case "applied":
    case "verification_pending":
    case "manual_apply_required":
    case "needs_follow_up":
    case "verification_failed":
      return "Verify Update";
    case "verified":
      return "Verified";
    default:
      return "Draft Update";
  }
}

export function pageUpdateDraftBusyCTA(draft: ContentPlanPageUpdateDraft | null | undefined) {
  if (pageUpdateDraftHasOpenGitHubPR(draft)) return "Opening PR";
  switch (draft?.status) {
    case "ready_for_review":
      return "Approving";
    case "approved":
      return "Applying";
    case "applied":
    case "verification_pending":
    case "manual_apply_required":
    case "needs_follow_up":
    case "verification_failed":
      return "Verifying";
    default:
      return "Drafting update";
  }
}

export function pageUpdateDraftStatusTone(status: string | undefined): "neutral" | "red" | "amber" | "green" | "blue" {
  switch (status) {
    case "verified":
      return "green";
    case "ready_for_review":
    case "approved":
    case "applied":
    case "verification_pending":
      return "blue";
    case "manual_apply_required":
    case "needs_follow_up":
      return "amber";
    case "verification_failed":
    case "rejected":
      return "red";
    default:
      return "neutral";
  }
}

export function isBacklogStatus(status: string) {
  return status === "backlog" || status === "scheduled" || status === "generating";
}

export function hasReviewableDraft(action: ContentPlanReviewDraft | null | undefined) {
  const draftID = action?.draft_article_id?.trim();
  const draftStatus = action?.draft_article_status?.trim().toLowerCase();
  return Boolean(draftID) && draftStatus === "pending_review";
}

export function reviewArticleIDForAction(
  action: ContentPlanReviewDraft | null | undefined,
  topicPendingReviewArticleID?: string | null,
) {
  const topicArticleID = topicPendingReviewArticleID?.trim();
  if (topicArticleID) return topicArticleID;
  if (hasReviewableDraft(action)) return action?.draft_article_id?.trim() || null;
  return null;
}

const advancedDraftStatuses = new Set([
  "approved",
  "scheduled",
  "pending_url_verification",
  "published",
  "publish_failed",
  "ready_to_distribute",
  "distributed",
]);

export function hasAdvancedDraftHandoff(action: ContentPlanReviewDraft | null | undefined) {
  const draftID = action?.draft_article_id?.trim();
  const draftStatus = action?.draft_article_status?.trim().toLowerCase();
  return Boolean(draftID && draftStatus && advancedDraftStatuses.has(draftStatus));
}

const contentPlanLifecycleStages = new Set(["added_to_plan", "planned", "drafting", "ready_for_review"]);
const terminalContentActionStatuses = new Set(["returned", "dismissed"]);
const terminalContentOpportunityStatuses = new Set(["dismissed", "archived"]);

export function isActiveContentPlanLoopAction(action: ContentPlanLoopAction | null | undefined) {
  if (!action) return false;
  const status = String(action.status ?? "").trim().toLowerCase();
  const stage = String(action.lifecycle_stage ?? "").trim().toLowerCase();
  const opportunityStatus = String(action.opportunity_status ?? "").trim().toLowerCase();
  return (
    !terminalContentActionStatuses.has(status) &&
    !terminalContentOpportunityStatuses.has(opportunityStatus) &&
    contentPlanLifecycleStages.has(stage)
  );
}

export function normalizePublishStrategy(value: any): ContentPlanPublishStrategy | null {
  if (typeof value !== "string") return null;
  const normalized = value.trim().toLowerCase().replace(/[_-]/g, " ");
  switch (normalized) {
    case "blog":
    case "source":
    case "source article":
    case "canonical":
    case "owned site":
    case "owned site article":
      return "blog";
    case "syndication":
    case "syndicate":
    case "distribution":
    case "distribution draft":
      return "syndication";
    case "both":
    case "blog and syndication":
    case "blog + syndication":
    case "source and distribution":
      return "both";
    default:
      return null;
  }
}

function strategyFromSnapshot(snapshot: any): ContentPlanPublishStrategy | null {
  if (!snapshot || typeof snapshot !== "object" || Array.isArray(snapshot)) return null;
  for (const key of ["publish_strategy", "publish_to", "content_destination_strategy", "channel"]) {
    const strategy = normalizePublishStrategy(snapshot[key]);
    if (strategy) return strategy;
  }
  return null;
}

export function recommendedPublishStrategyForAction(action: ContentPlanPublishAction): ContentPlanPublishStrategy {
  for (const snapshot of [action.input_snapshot, action.output_snapshot, action.evidence_snapshot]) {
    const strategy = strategyFromSnapshot(snapshot);
    if (strategy) return strategy;
  }

  const text = [
    action.asset_type,
    action.work_type,
    action.action_type,
    action.opportunity_type,
    action.opportunity_recommended_action,
    action.opportunity_expected_impact,
    action.opportunity_query,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();

  const platformIntent = ["community", "reddit", "dev.to", "hashnode", "distribution", "syndication"].some((term) => text.includes(term));
  const ownedSiteValue = ["page", "guide", "comparison", "article", "glossary", "supporting section"].some((term) => text.includes(term));
  if (platformIntent) return ownedSiteValue ? "both" : "syndication";
  if (action.work_type === "improve_page" || text.includes("page_update") || text.includes("metadata_rewrite") || text.includes("refresh")) {
    return "blog";
  }
  if (action.work_type === "create_content") return "both";
  if (["comparison", "alternative", "guide", "glossary", "supporting section"].some((term) => text.includes(term))) {
    return "both";
  }
  return "blog";
}

export function publishStrategyLabel(strategy: ContentPlanPublishStrategy) {
  switch (strategy) {
    case "blog":
      return "Blog";
    case "syndication":
      return "Syndication";
    case "both":
      return "Both";
  }
}

export function publishStrategyReasonForAction(action: ContentPlanPublishAction, strategy = recommendedPublishStrategyForAction(action)) {
  const text = [
    action.asset_type,
    action.work_type,
    action.action_type,
    action.opportunity_type,
    action.opportunity_recommended_action,
    action.opportunity_expected_impact,
    action.opportunity_query,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
  if (strategy === "both") return "This brief can build owned-site authority and support external distribution.";
  if (strategy === "syndication") return "This brief has platform or community intent that fits external distribution.";
  if (text.includes("metadata") || text.includes("refresh") || action.work_type === "improve_page") {
    return "This brief improves an existing owned page, so the source article is the clearest target.";
  }
  return "Blog is the safest default when the publish strategy is low confidence.";
}

export function topicPickScore(topic: ContentPlanTopic) {
  const briefComplete = Boolean((topic.target_keyword || topic.target_prompt) && topic.angle && topic.format);
  return (
    topicPriorityRank(topic.priority) * 10 +
    Math.min(topic.internal_links.length, 5) * 2 +
    (topic.channel === "both" ? 3 : 0) +
    (briefComplete ? 4 : 0)
  );
}

export function normalizedTopicPriority(priority: number) {
  if (!Number.isFinite(priority) || priority <= 0) return 0;
  if (priority > 10) return Math.min(10, Math.max(1, Math.ceil((100 - priority) / 10)));
  return Math.min(10, Math.max(1, Math.round(priority)));
}

export function topicPriorityRank(priority: number) {
  const normalized = normalizedTopicPriority(priority);
  return normalized > 0 ? 11 - normalized : 0;
}

export function topicWhy(topic: ContentPlanTopic) {
  if (topic.angle) return topic.angle;
  if (topic.target_prompt) return `Answers: ${topic.target_prompt}`;
  if (topic.target_keyword) return `Targets: ${topic.target_keyword}`;
  return "Generated from current context gaps and available evidence.";
}

export function topicPickSignal(topic: ContentPlanTopic) {
  if (topic.scheduled_at) return "Scheduled intent";
  if (topic.internal_links.length >= 3) return "Strong internal-link base";
  if (topic.channel === "both") return "Covers blog + syndication";
  if ((topic.target_keyword || topic.target_prompt) && topic.angle && topic.format) return "Complete brief";
  if (topic.priority > 0) return "Priority set by plan";
  return "Needs priority decision";
}

export function recommendedTopicIds(topics: ContentPlanTopic[]) {
  return [...topics]
    .filter((topic) => topic.status === "backlog" && !topic.scheduled_at)
    .sort((a, b) => {
      const score = topicPickScore(b) - topicPickScore(a);
      if (score !== 0) return score;
      return String(b.created_at ?? "").localeCompare(String(a.created_at ?? ""));
    })
    .slice(0, 3)
    .map((topic) => topic.id);
}

export function planHealthForTopics(topics: ContentPlanTopic[]) {
  const backlogTopics = topics.filter((topic) => isBacklogStatus(topic.status));
  return {
    backlog: backlogTopics.length,
    readyToDraft: backlogTopics.filter((topic) => topic.status === "backlog" && !topic.scheduled_at).length,
    scheduledIntent: backlogTopics.filter((topic) => topic.scheduled_at).length,
    needsPriority: backlogTopics.filter((topic) => topic.priority <= 0).length,
  };
}
