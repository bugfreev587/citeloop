export type NextWorkspaceActionInput = {
  projectId: string;
  hasProfile: boolean;
  failedPublishCount: number;
  hasBlockedDrafts: boolean;
  reviewCount: number;
  readyCount: number;
  topicsCount: number;
};

export type WorkspaceAction = {
  title: string;
  detail: string;
  href: string;
};

export type ProfileDraft = {
  positioning: string;
  icp: string;
  value_props: string;
  features: string;
  differentiators: string;
  competitors: string;
  key_terms: string;
  tone: string;
  banned_claims: string;
  content_rules: string;
  advancedJSON: string;
};

export function lines(value: string) {
  return value
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

export function nextWorkspaceAction({
  projectId,
  hasProfile,
  failedPublishCount,
  hasBlockedDrafts,
  reviewCount,
  readyCount,
  topicsCount,
}: NextWorkspaceActionInput): WorkspaceAction {
  if (!hasProfile) {
    return {
      title: "Refresh context",
      detail: "Confirm product facts, evidence, and positioning before generating a content plan.",
      href: `/projects/${projectId}/context`,
    };
  }
  if (failedPublishCount > 0) {
    return {
      title: "Fix publishing",
      detail: "A canonical article could not be confirmed online, so related variants may stay locked.",
      href: `/projects/${projectId}/publish`,
    };
  }
  if (hasBlockedDrafts) {
    return {
      title: "Review blocked drafts",
      detail: "Some drafts need evidence or positioning fixes before they can be approved.",
      href: `/projects/${projectId}/review`,
    };
  }
  if (reviewCount > 0) {
    return {
      title: "Review drafts",
      detail: "Generated drafts are waiting for the human approval gate.",
      href: `/projects/${projectId}/review`,
    };
  }
  if (readyCount > 0) {
    return {
      title: "Distribute variants",
      detail: "Approved variants are ready after their canonical article went live.",
      href: `/projects/${projectId}/publish`,
    };
  }
  if (topicsCount === 0) {
    return {
      title: "Generate content plan",
      detail: "Create a first backlog from your domain context before drafting content.",
      href: `/projects/${projectId}/plan`,
    };
  }
  return {
    title: "Refresh context",
    detail: "Keep product facts, evidence, and positioning current before the next content cycle.",
    href: `/projects/${projectId}/context`,
  };
}

export function profilePayloadFromDraft(draft: ProfileDraft, baseProfile: Record<string, any> = {}) {
  const voice =
    baseProfile.voice && typeof baseProfile.voice === "object" && !Array.isArray(baseProfile.voice)
      ? baseProfile.voice
      : {};
  return {
    ...baseProfile,
    positioning: draft.positioning.trim(),
    icp: lines(draft.icp),
    value_props: lines(draft.value_props),
    features: lines(draft.features),
    differentiators: lines(draft.differentiators),
    competitors: lines(draft.competitors),
    key_terms: lines(draft.key_terms),
    tone: draft.tone.trim(),
    banned_claims: lines(draft.banned_claims),
    content_rules: lines(draft.content_rules),
    voice: {
      ...voice,
      tone: draft.tone.trim(),
      rules: lines(draft.content_rules),
    },
  };
}

export function profilePayloadFromAdvancedJSON(value: string) {
  return value.trim() ? JSON.parse(value) : {};
}

export function visibilityLifecycleLabel(status: string) {
  if (["accepted", "converted", "planned"].includes(status)) return "Added to Content Plan";
  if (status === "drafting") return "Draft in progress";
  if (["drafted", "ready_for_review", "in_review"].includes(status)) return "Draft waiting for review";
  if (status === "approved") return "Approved for publish";
  if (["published", "measuring"].includes(status)) return "Measuring impact";
  if (["completed", "done", "learned", "improved"].includes(status)) return "Loop closed";
  if (status === "stale") return "Needs re-check";
  if (status === "dismissed") return "Dismissed";
  if (["failed", "blocked"].includes(status)) return "Needs attention";
  return "Opportunity detected";
}

export function visibilityLifecycleTone(status: string): "green" | "amber" | "blue" | "neutral" | "red" {
  if (["completed", "done", "learned", "improved"].includes(status)) return "green";
  if (["accepted", "converted", "planned", "ready_for_review", "drafting", "drafted", "approved", "published", "measuring"].includes(status)) return "blue";
  if (["stale", "open"].includes(status)) return "amber";
  if (["dismissed", "archived"].includes(status)) return "neutral";
  if (["failed", "blocked"].includes(status)) return "red";
  return "amber";
}
