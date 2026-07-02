import type { AutopilotReadiness, AutopilotReadinessGate } from "./api";

export type HumanActionPriority = "P0" | "P1" | "P2";
export type HumanActionCategory = "Blocking now" | "Needs review" | "Improves results" | "Warnings";
export type HumanActionTone = "red" | "amber" | "blue" | "green" | "neutral";

export type ReadinessHumanAction = {
  id: string;
  dedupeKey: string;
  title: string;
  detail: string;
  href: string;
  cta: string;
  category: HumanActionCategory;
  priority: HumanActionPriority;
  rank: number;
  tone: HumanActionTone;
  count?: number;
};

type GateAction = {
  title: string;
  cta: string;
  href: (projectId: string) => string;
  category: HumanActionCategory;
  priorityWhenLevel2: HumanActionPriority;
  priorityBeforeLevel2: HumanActionPriority;
  rank: number;
};

// `AutopilotReadiness.gates` is the canonical source for automation readiness. When
// `SEOOverview.setup_checklist` describes the same capability, its Home card must collapse
// onto these readiness dedupe keys so we never render two competing CTAs for one capability.
export const SETUP_DEDUPE_KEYS: Record<string, string> = {
  search_data: "readiness:search_read",
  publisher_write: "readiness:publisher_write",
  notification_write: "readiness:notification_write",
};

export const SETUP_SOURCE_PRECEDENCE =
  "AutopilotReadiness.gates overrides SEOOverview.setup_checklist for shared setup cards";

export const READINESS_GATE_ACTIONS: Record<string, GateAction> = {
  search_read: {
    title: "Connect Search Console",
    cta: "Connect Search Console",
    href: (projectId) => `/projects/${projectId}/settings#search-console`,
    category: "Improves results",
    priorityWhenLevel2: "P0",
    priorityBeforeLevel2: "P2",
    rank: 82,
  },
  publisher_write: {
    title: "Connect publisher",
    cta: "Connect publisher",
    href: (projectId) => `/projects/${projectId}/settings#publisher`,
    category: "Blocking now",
    priorityWhenLevel2: "P0",
    priorityBeforeLevel2: "P2",
    rank: 24,
  },
  notification_write: {
    title: "Create notification channel",
    cta: "Set notifications",
    href: (projectId) => `/projects/${projectId}/settings#notifications`,
    category: "Blocking now",
    priorityWhenLevel2: "P0",
    priorityBeforeLevel2: "P2",
    rank: 26,
  },
  autopilot_policy_confirmed: {
    title: "Review Autopilot policy",
    cta: "Review policy",
    href: (projectId) => `/projects/${projectId}/settings#automation-policy`,
    category: "Needs review",
    priorityWhenLevel2: "P0",
    priorityBeforeLevel2: "P1",
    rank: 28,
  },
  monthly_budget_configured: {
    // Budget gate reads SEOPolicy.monthly_budget_limit, edited in Settings Automation policy,
    // NOT the separate project-config budget field. The CTA must land on #automation-policy.
    title: "Set Autopilot budget",
    cta: "Set Autopilot budget",
    href: (projectId) => `/projects/${projectId}/settings#automation-policy`,
    category: "Blocking now",
    priorityWhenLevel2: "P0",
    priorityBeforeLevel2: "P2",
    rank: 30,
  },
  safe_mode_clear: {
    title: "Resolve safe mode",
    cta: "Review safe mode",
    href: (projectId) => `/projects/${projectId}/settings#automation-policy`,
    category: "Blocking now",
    priorityWhenLevel2: "P0",
    priorityBeforeLevel2: "P0",
    rank: 12,
  },
  kill_switch_clear: {
    title: "Automation kill switch is on",
    cta: "Review kill switch",
    href: (projectId) => `/projects/${projectId}/settings#automation-policy`,
    category: "Blocking now",
    priorityWhenLevel2: "P0",
    priorityBeforeLevel2: "P0",
    rank: 10,
  },
  rollback_or_recovery_ready: {
    title: "Confirm recovery plan",
    cta: "Confirm recovery plan",
    href: (projectId) => `/projects/${projectId}/settings#recovery-plan`,
    category: "Needs review",
    priorityWhenLevel2: "P0",
    priorityBeforeLevel2: "P2",
    rank: 32,
  },
};

export function toneForHumanPriority(priority: HumanActionPriority): HumanActionTone {
  if (priority === "P0") return "red";
  if (priority === "P1") return "amber";
  return "blue";
}

export function buildReadinessHumanActions(input: {
  projectId: string;
  readiness: AutopilotReadiness | null;
  canAccessSettings: boolean;
}): ReadinessHumanAction[] {
  const { projectId, readiness, canAccessSettings } = input;
  if (!readiness || !canAccessSettings) return [];

  return readiness.gates
    .filter((gate): gate is AutopilotReadinessGate => Boolean(gate?.blocking))
    .map((gate) => {
      const action = READINESS_GATE_ACTIONS[gate.key];
      if (!action) return null;
      const priority = readiness.autopilot_level >= 2 ? action.priorityWhenLevel2 : action.priorityBeforeLevel2;
      const detail =
        gate.key === "monthly_budget_configured"
          ? `${gate.reason} Set SEOPolicy.monthly_budget_limit in Automation policy.`
          : gate.reason;
      return {
        id: `readiness-${gate.key}`,
        dedupeKey: `readiness:${gate.key}`,
        title: action.title,
        detail,
        href: action.href(projectId),
        cta: action.cta,
        category: priority === "P0" ? "Blocking now" : action.category,
        priority,
        rank: action.rank,
        tone: toneForHumanPriority(priority),
      };
    })
    .filter((item): item is ReadinessHumanAction => Boolean(item));
}

export type ReadinessGateAction = { href: string; cta: string; rank: number };

// Deterministic fix destination for a single readiness gate, independent of Home priority.
// Settings uses this to turn every blocked gate into a one-click "go fix it" link.
export function readinessGateActionFor(key: string, projectId: string): ReadinessGateAction | null {
  const action = READINESS_GATE_ACTIONS[key];
  if (!action) return null;
  return { href: action.href(projectId), cta: action.cta, rank: action.rank };
}

export function dedupeHumanActions<T extends { dedupeKey?: string; id: string }>(items: T[]): T[] {
  const seen = new Set<string>();
  const deduped: T[] = [];
  for (const item of items) {
    const key = item.dedupeKey ?? item.id;
    if (seen.has(key)) continue;
    seen.add(key);
    deduped.push(item);
  }
  return deduped;
}
