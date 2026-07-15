export type PlatformCapabilityLike = {
  platform: string;
  contract_id: string;
  contract_version: string;
  generation_supported: boolean;
  target_context_ready: boolean;
  connection_ready?: boolean;
  output_type: string;
  block_reasons: string[];
};

export type PlatformContextLike = {
  id: string;
  target_key: string;
  version: number;
  status: string;
  expires_at?: string | null;
};

export type PlatformTarget = {
  platform: string;
  target_key?: string;
  output_type: string;
  is_canonical?: boolean;
  platform_contract_id: string;
  platform_contract_version: string;
  target_context_id?: string;
  target_context_version?: number;
};

export type ExactTargetSelection = {
  canonical_target: PlatformTarget;
  target_platforms: PlatformTarget[];
  asset_type: string;
};

export type TargetSelectionValidation = { valid: boolean; errors: string[] };

const PLATFORM_LABELS: Record<string, string> = {
  blog: "Blog",
  dev_to: "Dev.to",
  hashnode: "Hashnode",
  medium: "Medium",
  linkedin: "LinkedIn",
  reddit: "Reddit",
  hacker_news: "Hacker News",
};

export function platformLabel(platform: string) {
  return PLATFORM_LABELS[platform] ?? platform;
}

export function targetFromCapability(capability: PlatformCapabilityLike, context?: PlatformContextLike | null): PlatformTarget {
  return {
    platform: capability.platform,
    target_key: context?.target_key || undefined,
    output_type: capability.output_type,
    is_canonical: capability.platform === "blog",
    platform_contract_id: capability.contract_id,
    platform_contract_version: capability.contract_version,
    target_context_id: context?.id,
    target_context_version: context?.version,
  };
}

export function initialTargetSelection(assetType: string, capabilities: PlatformCapabilityLike[]): ExactTargetSelection {
  const blog = capabilities.find((item) => item.platform === "blog");
  if (!blog) throw new Error("The active Blog platform contract is unavailable.");
  const canonical = targetFromCapability(blog);
  return { canonical_target: canonical, target_platforms: [canonical], asset_type: assetType };
}

export function togglePlatformTarget(
  selection: ExactTargetSelection,
  capability: PlatformCapabilityLike,
  selected: boolean,
  context?: PlatformContextLike | null,
): ExactTargetSelection {
  if (capability.platform === selection.canonical_target.platform && !selected) return selection;
  const without = selection.target_platforms.filter((item) => item.platform !== capability.platform || (item.target_key ?? "") !== (context?.target_key ?? ""));
  const targets = selected ? [...without, targetFromCapability(capability, context)] : without;
  targets.sort((left, right) => Number(Boolean(right.is_canonical)) - Number(Boolean(left.is_canonical)) || left.platform.localeCompare(right.platform));
  return { ...selection, target_platforms: targets };
}

export function validateTargetSelection(selection: ExactTargetSelection, capabilities: PlatformCapabilityLike[]): TargetSelectionValidation {
  const errors: string[] = [];
  const canonical = selection.target_platforms.find((target) => target.is_canonical);
  if (!canonical || canonical.platform !== selection.canonical_target.platform) errors.push("The canonical Blog target is required.");
  for (const target of selection.target_platforms) {
    const capability = capabilities.find((item) => item.platform === target.platform);
    if (!capability?.generation_supported) errors.push(`${platformLabel(target.platform)} does not support this asset type.`);
    if (target.platform !== "blog" && capability?.connection_ready === false) errors.push(`${platformLabel(target.platform)} connection must be enabled before generation.`);
    if (capability && !capability.target_context_ready) errors.push(`${platformLabel(target.platform)} rules or target context must be confirmed before generation.`);
    if (["reddit", "hashnode"].includes(target.platform) && (!target.target_context_id || !target.target_key)) errors.push(`${platformLabel(target.platform)} target context must be confirmed and pinned before generation.`);
  }
  return { valid: errors.length === 0, errors };
}

export function summarizeTargetSelection(selection: ExactTargetSelection): "blog" | "syndication" | "both" {
  const blog = selection.target_platforms.some((item) => item.platform === "blog");
  const external = selection.target_platforms.some((item) => item.platform !== "blog");
  if (blog && external) return "both";
  return blog ? "blog" : "syndication";
}

export function summarizeTargetSelectionPlatforms(selection: ExactTargetSelection) {
  return selection.target_platforms
    .map((target) => platformLabel(target.platform))
    .join(" + ");
}
