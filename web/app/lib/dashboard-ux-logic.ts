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

export type ActionableMomentumInput = {
  projectId: string;
  hasProfile: boolean;
  publishedThisMonthCount: number;
  approvedDraftCount: number;
  opportunitiesConvertedCount: number;
  readyToDistributeCount: number;
  activeLoopItemCount: number;
};

export type ActionableMomentumItem = {
  id: string;
  label: string;
  value: number;
  detail: string;
  href: string;
  actionLabel: string;
  tone: "green" | "amber" | "blue" | "neutral";
};

export type ActionableMomentumResult = {
  items: ActionableMomentumItem[];
  emptyAction: (WorkspaceAction & { actionLabel: string }) | null;
};

export type HomeEventInput = {
  projectId: string;
  liveActivities?: Array<{
    id: string;
    title: string;
    detail: string;
    href: string;
  }>;
  recentEvents?: Array<{
    id: string;
    title: string;
    detail: string;
    href: string;
  }>;
  nextEvent?: {
    title: string;
    detail: string;
    href: string;
  } | null;
  limit?: number;
};

export type HomeEventStreamItem = {
  id: string;
  kind: "live" | "recent" | "next";
  title: string;
  detail: string;
  href: string;
  timeLabel: string;
};

export type HomeEventStreamResult = {
  items: HomeEventStreamItem[];
  emptyAction: (WorkspaceAction & { actionLabel: string }) | null;
};

export type HomeSectionCandidate = {
  id: string;
  count: number;
  priority: number;
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

export function buildActionableMomentum(input: ActionableMomentumInput): ActionableMomentumResult {
  const candidates: ActionableMomentumItem[] = [
    {
      id: "ready-to-publish",
      label: "Ready to publish",
      value: input.readyToDistributeCount,
      detail: "approved variants can move now",
      href: `/projects/${input.projectId}/publish`,
      actionLabel: "Publish",
      tone: "amber",
    },
    {
      id: "published-this-month",
      label: "Published this month",
      value: input.publishedThisMonthCount,
      detail: "live assets feeding visibility",
      href: `/projects/${input.projectId}/visibility`,
      actionLabel: "View impact",
      tone: "green",
    },
    {
      id: "opportunities-converted",
      label: "Opportunities converted",
      value: input.opportunitiesConvertedCount,
      detail: "visibility gaps entered the loop",
      href: `/projects/${input.projectId}/visibility`,
      actionLabel: "Review loop",
      tone: "blue",
    },
    {
      id: "active-loop-items",
      label: "Active loop items",
      value: input.activeLoopItemCount,
      detail: "items moving from insight to impact",
      href: `/projects/${input.projectId}`,
      actionLabel: "Timeline",
      tone: "neutral",
    },
    {
      id: "approved-drafts",
      label: "Approved drafts",
      value: input.approvedDraftCount,
      detail: "approved drafts waiting on publish",
      href: `/projects/${input.projectId}/publish`,
      actionLabel: "Publish",
      tone: "amber",
    },
  ];
  const items = candidates.filter((item) => item.value > 0).slice(0, 4);

  if (items.length > 0) {
    return { items, emptyAction: null };
  }

  if (!input.hasProfile) {
    return {
      items: [],
      emptyAction: {
        title: "Context needs confirmation",
        detail: "Connect product facts and source evidence before CiteLoop can generate a plan.",
        href: `/projects/${input.projectId}/context`,
        actionLabel: "Open Context",
      },
    };
  }

  return {
    items: [],
    emptyAction: {
      title: "Context is ready",
      detail: "Generate the first content plan to start moving items through the loop.",
      href: `/projects/${input.projectId}/plan`,
      actionLabel: "Generate content plan",
    },
  };
}

export function buildHomeEventStream({
  projectId,
  liveActivities = [],
  recentEvents = [],
  nextEvent,
  limit = 5,
}: HomeEventInput): HomeEventStreamResult {
  const liveItems = liveActivities.map((event): HomeEventStreamItem => ({
    ...event,
    kind: "live",
    timeLabel: "Now",
  }));
  const recentItems = recentEvents.map((event): HomeEventStreamItem => ({
    ...event,
    kind: "recent",
    timeLabel: "Recent",
  }));
  const nextItems: HomeEventStreamItem[] = nextEvent
    ? [
        {
          id: "next-event",
          kind: "next",
          title: nextEvent.title,
          detail: nextEvent.detail,
          href: nextEvent.href,
          timeLabel: "Next",
        },
      ]
    : [];
  const items = [...liveItems, ...recentItems, ...nextItems].slice(0, limit);

  if (items.length > 0) {
    return { items, emptyAction: null };
  }

  return {
    items: [],
    emptyAction: {
      title: "All set for now",
      detail: "No live work or scheduled publish slot is waiting. Growth signals will appear here as the loop starts moving.",
      href: `/projects/${projectId}/context`,
      actionLabel: "Open context",
    },
  };
}

export function visibleHomeSectionIds(sections: HomeSectionCandidate[], options: { limit?: number } = {}) {
  const limit = options.limit ?? 2;
  const active = sections
    .filter((section) => section.count > 0)
    .sort((a, b) => b.priority - a.priority || a.id.localeCompare(b.id));

  return {
    visibleIds: active.slice(0, limit).map((section) => section.id),
    overflowIds: active.slice(limit).map((section) => section.id),
  };
}

export function sidebarPrimaryAction(input: NextWorkspaceActionInput): WorkspaceAction {
  const action = nextWorkspaceAction(input);
  if (!input.hasProfile) return action;
  if (input.failedPublishCount > 0) return action;
  if (input.hasBlockedDrafts) return { ...action, title: "Review blocked" };
  if (input.reviewCount > 0) return { ...action, title: "Review drafts" };
  if (input.readyCount > 0) return { ...action, title: "Distribute" };
  if (input.topicsCount === 0) return { ...action, title: "Create plan" };
  return {
    title: "Open Home",
    detail: "Start from the control center before jumping into deeper work.",
    href: `/projects/${input.projectId}`,
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
