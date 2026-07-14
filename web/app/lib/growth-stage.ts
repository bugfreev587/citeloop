export const GROWTH_STAGE_OPTIONS = [
  { key: "foundation", label: "Foundation", description: "Build essential topic coverage and citable owned assets." },
  { key: "traction", label: "Traction", description: "Act on emerging SEO and GEO demand." },
  { key: "scale", label: "Scale", description: "Expand proven themes across high-value content and platforms." },
  { key: "optimize", label: "Optimize", description: "Refresh declining assets and respond to competitive change." },
] as const;

export type GrowthStage = (typeof GROWTH_STAGE_OPTIONS)[number]["key"];

export function growthStageOption(stage: string) {
  return GROWTH_STAGE_OPTIONS.find((option) => option.key === stage) ?? GROWTH_STAGE_OPTIONS[0];
}

export function growthStageConfirmation(current: string, proposed: string, affected: number) {
  const from = growthStageOption(current);
  const to = growthStageOption(proposed);
  return [
    `Change Growth Stage from ${from.label} to ${to.label}?`,
    to.description,
    `${affected} active watchlist candidate${affected === 1 ? "" : "s"} will be rescored.`,
    "Accepted and in-progress Opportunities will not change.",
  ].join("\n\n");
}
