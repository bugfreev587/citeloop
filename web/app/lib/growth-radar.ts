export type GrowthRadarTarget = { platform: string; target_key?: string };
export type GrowthRadarFunnel = {
  sources: { scheduled: number; succeeded: number; skipped: number; failed: number };
  evidence: { added: number; changed: number; reused: number; expired: number };
  terms: { accepted: number; rejected: number; held: number };
  prompts: { active: number; selected: number; rotated: number; targeted: number };
  candidates: { generated: number; duplicates: number; conflicts: number; watchlist: number; filtered: number; created: number };
  cost_usd: number;
  status: string;
  reasons: Record<string, number>;
  target_platforms?: GrowthRadarTarget[];
};

export type GrowthRadarUserResult = {
  kind: "empty" | "incomplete";
  tone: "neutral" | "amber";
  title: string;
  detail: string;
};

export function userFacingGrowthRadarResult(run: GrowthRadarFunnel): GrowthRadarUserResult | null {
  if (Number(run.candidates?.created ?? 0) > 0) return null;
  if (run.status === "degraded" || run.status === "failed") {
    return {
      kind: "incomplete",
      tone: "amber",
      title: "Opportunity finding couldn't finish",
      detail: "We couldn't complete every check. Please try again.",
    };
  }
  return {
    kind: "empty",
    tone: "neutral",
    title: "No new opportunities found",
    detail: "Your current opportunity queue is up to date. CiteLoop will keep looking as your site and market change.",
  };
}

export function summarizeGrowthRadarRun(run: GrowthRadarFunnel) {
  const created = Number(run.candidates?.created ?? 0);
  const degraded = run.status === "degraded" || run.status === "failed";
  return {
    health: created > 0 ? "created" : degraded ? "degraded_zero" : "healthy_zero",
    created,
    watchlist: Number(run.candidates?.watchlist ?? 0),
    rejected: Number(run.candidates?.duplicates ?? 0) + Number(run.candidates?.conflicts ?? 0) + Number(run.candidates?.filtered ?? 0),
    promptRotation: `${Number(run.prompts?.rotated ?? 0)} rotated · ${Number(run.prompts?.targeted ?? 0)} targeted`,
    cost: `$${Number(run.cost_usd ?? 0).toFixed(2)}`,
  };
}

export function explainZeroOpportunities(run: GrowthRadarFunnel): string[] {
  if (Number(run.candidates?.created ?? 0) > 0) return [];
  const reasons: string[] = [];
  if (run.reasons?.no_usable_evidence) reasons.push("No usable evidence was collected.");
  if (run.reasons?.brave_budget_exhausted) reasons.push("Search evidence budget was exhausted.");
  if (run.sources?.failed || run.sources?.skipped) reasons.push(`${Number(run.sources.failed ?? 0)} evidence sources failed; ${Number(run.sources.skipped ?? 0)} was skipped.`);
  if (!reasons.length && run.candidates?.watchlist) reasons.push(`${run.candidates.watchlist} candidate remained on the watchlist pending stronger evidence.`);
  if (!reasons.length && run.candidates?.filtered) reasons.push(`${run.candidates.filtered} candidate did not pass deterministic policy gates.`);
  if (!reasons.length) reasons.push("The run completed normally and found no new decision-ready gaps.");
  return reasons;
}

const platformLabels: Record<string, string> = { blog: "Blog", dev_to: "Dev.to", hashnode: "Hashnode", medium: "Medium", linkedin: "LinkedIn", reddit: "Reddit", hacker_news: "Hacker News" };
export function summarizeExactTargets(targets: GrowthRadarTarget[] = []): string {
  return targets.map((target) => `${platformLabels[target.platform] ?? target.platform}${target.target_key ? ` (${target.target_key})` : ""}`).join(" + ");
}
