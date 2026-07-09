// Sourced from ~/token-gate/frontend/src/composables/useModelWhitelist.ts.
export const tokengateOpenAIModelOptions = [
  "gpt-5.2",
  "gpt-5.2-2025-12-11",
  "gpt-5.2-chat-latest",
  "gpt-5.2-pro",
  "gpt-5.2-pro-2025-12-11",
  "gpt-5.5",
  "gpt-5.4",
  "gpt-5.4-mini",
  "gpt-5.4-2026-03-05",
  "gpt-5.3-codex",
  "gpt-5.3-codex-spark",
  "codex-auto-review",
  "gpt-4o-audio-preview",
  "gpt-4o-realtime-preview",
  "gpt-image-1",
  "gpt-image-1.5",
  "gpt-image-2",
] as const;

export const tokengateAnthropicModelOptions = [
  "claude-sonnet-5",
  "claude-opus-4-8",
  "claude-fable-5",
  "claude-3-5-sonnet-20241022",
  "claude-3-5-sonnet-20240620",
  "claude-3-5-haiku-20241022",
  "claude-3-7-sonnet-20250219",
  "claude-sonnet-4-20250514",
  "claude-opus-4-20250514",
  "claude-opus-4-1-20250805",
  "claude-sonnet-4-5-20250929",
  "claude-haiku-4-5-20251001",
  "claude-opus-4-5-20251101",
  "claude-opus-4-6",
  "claude-opus-4-7",
  "claude-sonnet-4-6",
] as const;

export function tokengateModelOptionsWithCurrent(options: readonly string[], current: string) {
  const trimmed = current.trim();
  if (trimmed === "" || options.includes(trimmed)) {
    return options;
  }
  return [trimmed, ...options];
}
