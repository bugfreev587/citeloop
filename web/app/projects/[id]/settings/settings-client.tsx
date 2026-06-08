"use client";

import { useCallback, useEffect, useState } from "react";
import { Bell, GitBranch, Plus, RefreshCw, RotateCcw, Save, Send, Trash2 } from "lucide-react";
import {
  defaultProjectConfig,
  GitHubNextJSPublisherInput,
  NotificationChannel,
  NotificationChannelKind,
  NotificationDelivery,
  NotificationEvent,
  NotificationSubscription,
  PublisherConnection,
  ProjectConfig,
} from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, Field, Notice, SectionHeader, TextInput, TextArea, cx, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

function readableError(error: unknown) {
  const raw = error instanceof Error ? error.message : String(error ?? "");
  const normalized = raw.toLowerCase();
  if (normalized.includes("repo") && normalized.includes("base_url")) {
    return "Add the GitHub repository and public blog base URL before saving.";
  }
  if (normalized.includes("base_url") || normalized.includes("base url")) {
    return "Add the public blog base URL where published articles will be available.";
  }
  if (normalized.includes("repo") || normalized.includes("repository")) {
    return "Add the GitHub repository in owner/repo format.";
  }
  if (normalized.includes("webhook") && (normalized.includes("url") || normalized.includes("invalid"))) {
    return "Paste a valid Slack or Discord webhook URL for the selected channel type.";
  }
  if (normalized.includes("token") || normalized.includes("credential")) {
    return "Save a GitHub token with repository write access, then test the publisher connection.";
  }
  if (normalized.includes("permission") || normalized.includes("forbidden") || normalized.includes("403")) {
    return "This account or token does not have permission for that action.";
  }
  if (normalized.includes("notification_secret_key")) {
    return "Notification encryption is not configured for this environment.";
  }
  return raw || "Something went wrong. Please try again.";
}

function validWebhookURL(kind: NotificationChannelKind, url: string) {
  if (kind === "slack_webhook") return /^https:\/\/hooks\.slack\.com\/services\/\S+/i.test(url);
  return /^https:\/\/(?:discord|discordapp)\.com\/api\/webhooks\/\S+/i.test(url);
}

function webhookURLHint(kind: NotificationChannelKind) {
  return kind === "slack_webhook"
    ? "Paste a Slack incoming webhook URL that starts with https://hooks.slack.com/services/."
    : "Paste a Discord webhook URL that starts with https://discord.com/api/webhooks/.";
}

function summarizeConfigChanges(previous: ProjectConfig | null, next: ProjectConfig) {
  if (!previous) return "Saved cadence, budget, channel mix, crawl policy, and brand voice.";
  const changes: string[] = [];
  if (previous.cadence_per_week !== next.cadence_per_week) changes.push("cadence");
  if (previous.buffer_days !== next.buffer_days) changes.push("buffer days");
  if (previous.monthly_budget_usd !== next.monthly_budget_usd) changes.push("monthly budget");
  if (previous.channel_mix.blog !== next.channel_mix.blog || previous.channel_mix.syndication !== next.channel_mix.syndication) {
    changes.push("channel mix");
  }
  if ((previous.brand_voice ?? "") !== (next.brand_voice ?? "")) changes.push("brand voice");
  if (JSON.stringify(previous.crawl) !== JSON.stringify(next.crawl)) changes.push("crawl policy");
  if (changes.length === 0) return "No setting values changed.";
  const shown = changes.slice(0, 4);
  const suffix = changes.length > shown.length ? ` and ${changes.length - shown.length} more` : "";
  return `Updated ${shown.join(", ")}${suffix}.`;
}

function toInt(value: string, fallback: number) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function toFloat(value: string, fallback: number) {
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

const channelKinds: Array<{ value: NotificationChannelKind; label: string }> = [
  { value: "slack_webhook", label: "Slack" },
  { value: "discord_webhook", label: "Discord" },
];

const eventLabels: Record<string, string> = {
  "generation.failed": "Generation failed",
  "publish.failed": "Publish failed",
  "budget.stopped": "Budget stopped",
  "review.overdue": "Review overdue",
  "webhook.delivery.dead": "Webhook delivery dead",
  "seo.sync.failed": "SEO sync failed",
  "seo.auth.expired": "SEO auth expired",
  "seo.opportunity.ready": "SEO opportunity ready",
  "seo.brief.ready": "SEO brief ready",
  "seo.action.measurement_ready": "SEO measurement ready",
  "seo.indexing.anomaly": "SEO indexing anomaly",
};

const deliveryStatuses = [
  { value: "", label: "All" },
  { value: "pending", label: "Pending" },
  { value: "dead", label: "Dead" },
  { value: "sent", label: "Sent" },
];

const defaultPublisherDraft: GitHubNextJSPublisherInput = {
  label: "GitHub/Next.js",
  repo: "",
  branch: "citeloop-content",
  content_dir: "content/citeloop/blog",
  base_url: "",
  publish_mode: "publish",
  credential_ref: "",
};

export function SettingsClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [config, setConfig] = useState<ProjectConfig>(defaultProjectConfig());
  const [savedConfig, setSavedConfig] = useState<ProjectConfig | null>(null);
  const [publisherConnections, setPublisherConnections] = useState<PublisherConnection[]>([]);
  const [publisherDraft, setPublisherDraft] = useState<GitHubNextJSPublisherInput>(defaultPublisherDraft);
  const [publisherCredentialDraft, setPublisherCredentialDraft] = useState("");
  const [showPublisherAdvanced, setShowPublisherAdvanced] = useState(false);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [events, setEvents] = useState<NotificationEvent[]>([]);
  const [subscriptions, setSubscriptions] = useState<NotificationSubscription[]>([]);
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [deliveryStatus, setDeliveryStatus] = useState("");
  const [channelDraft, setChannelDraft] = useState<{ kind: NotificationChannelKind; label: string; url: string }>({
    kind: "slack_webhook",
    label: "Ops",
    url: "",
  });
  const [busy, setBusy] = useState(false);
  const [notificationBusy, setNotificationBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    try {
      const project = await api.getProject(projectId);
      setConfig(project.config);
      setSavedConfig(project.config);
    } catch (e: any) {
      setMessage({ title: "Settings unavailable", detail: readableError(e), tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const refreshPublisherConnections = useCallback(async () => {
    try {
      const nextConnections = await api.listPublisherConnections(projectId);
      setPublisherConnections(nextConnections);
      const github = nextConnections.find((connection) => connection.kind === "github_nextjs");
      if (github) {
        setPublisherDraft({
          label: github.label || "GitHub/Next.js",
          repo: github.config?.repo ?? "",
          branch: github.config?.branch ?? "citeloop-content",
          content_dir: github.config?.content_dir ?? "content/citeloop/blog",
          base_url: github.config?.base_url ?? "",
          publish_mode: github.config?.publish_mode ?? "publish",
          credential_ref: "",
        });
      }
    } catch (e: any) {
      setMessage({ title: "Publisher connections unavailable", detail: readableError(e), tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refreshPublisherConnections();
  }, [refreshPublisherConnections]);

  const refreshNotifications = useCallback(async () => {
    try {
      const [nextChannels, nextEvents, nextSubscriptions, nextDeliveries] = await Promise.all([
        api.listNotificationChannels(projectId),
        api.listNotificationEvents(projectId),
        api.listNotificationSubscriptions(projectId),
        api.listNotificationDeliveries(projectId, { status: deliveryStatus, limit: 25 }),
      ]);
      setChannels(nextChannels);
      setEvents(nextEvents);
      setSubscriptions(nextSubscriptions);
      setDeliveries(nextDeliveries);
    } catch (e: any) {
      setMessage({ title: "Notifications unavailable", detail: readableError(e), tone: "amber" });
    }
  }, [api, deliveryStatus, projectId]);

  useEffect(() => {
    refreshNotifications();
  }, [refreshNotifications]);

  function update(next: Partial<ProjectConfig>) {
    setConfig((current) => ({ ...current, ...next }));
  }

  async function save() {
    setBusy(true);
    setMessage(null);
    try {
      const fullPayload = {
        ...defaultProjectConfig(),
        ...config,
        crawl: { ...defaultProjectConfig().crawl, ...config.crawl },
        channel_mix: { ...defaultProjectConfig().channel_mix, ...config.channel_mix },
      };
      if (
        savedConfig &&
        fullPayload.monthly_budget_usd < savedConfig.monthly_budget_usd &&
        !window.confirm("Lowering the monthly budget can stop future automation earlier. Save this lower budget?")
      ) {
        setMessage({ title: "Save cancelled", detail: "Monthly budget was not changed.", tone: "amber" });
        return;
      }
      const summary = summarizeConfigChanges(savedConfig, fullPayload);
      await api.updateConfig(projectId, fullPayload);
      setConfig(fullPayload);
      setSavedConfig(fullPayload);
      setMessage({ title: "Settings saved", detail: summary, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Settings save failed", detail: readableError(e), tone: "red" });
    } finally {
      setBusy(false);
    }
  }

  async function createChannel() {
    const url = channelDraft.url.trim();
    if (!url) {
      setMessage({ title: "Webhook URL required", detail: webhookURLHint(channelDraft.kind), tone: "amber" });
      return;
    }
    if (!validWebhookURL(channelDraft.kind, url)) {
      setMessage({ title: "Webhook URL format looks wrong", detail: webhookURLHint(channelDraft.kind), tone: "amber" });
      return;
    }
    setNotificationBusy("create-channel");
    setMessage(null);
    try {
      await api.createNotificationChannel(projectId, {
        kind: channelDraft.kind,
        label: channelDraft.label.trim() || channelKinds.find((item) => item.value === channelDraft.kind)?.label || "Webhook",
        url,
      });
      setChannelDraft((current) => ({ ...current, url: "" }));
      setMessage({ title: "Notification channel saved", tone: "green" });
      await refreshNotifications();
    } catch (e: any) {
      setMessage({ title: "Channel save failed", detail: readableError(e), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function deleteChannel(channelID: string) {
    setNotificationBusy(`delete-${channelID}`);
    setMessage(null);
    try {
      await api.deleteNotificationChannel(projectId, channelID);
      setMessage({ title: "Notification channel deleted", tone: "green" });
      await refreshNotifications();
    } catch (e: any) {
      setMessage({ title: "Channel delete failed", detail: readableError(e), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function testChannel(channelID: string) {
    setNotificationBusy(`test-${channelID}`);
    setMessage(null);
    try {
      await api.testNotificationChannel(projectId, channelID);
      setMessage({ title: "Test notification sent", detail: "Channel is now verified.", tone: "green" });
      await refreshNotifications();
    } catch (e: any) {
      setMessage({ title: "Test notification failed", detail: readableError(e), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function savePublisherConnection() {
    const repo = publisherDraft.repo.trim();
    const baseURL = publisherDraft.base_url.trim();
    if (!repo && !baseURL) {
      setMessage({
        title: "Publisher settings incomplete",
        detail: "Add the GitHub repository and public blog base URL before saving.",
        tone: "amber",
      });
      return;
    }
    if (!repo) {
      setMessage({
        title: "Repository required",
        detail: "Add the GitHub repository in owner/repo format.",
        tone: "amber",
      });
      return;
    }
    if (!baseURL) {
      setMessage({
        title: "Base URL required",
        detail: "Add the public blog base URL where published articles will be available.",
        tone: "amber",
      });
      return;
    }
    setNotificationBusy("save-publisher");
    setMessage(null);
    try {
      let saved = await api.upsertGitHubNextJSPublisherConnection(projectId, {
        ...publisherDraft,
        repo,
        branch: publisherDraft.branch?.trim() || "citeloop-content",
        content_dir: publisherDraft.content_dir?.trim() || "content/citeloop/blog",
        base_url: baseURL,
        publish_mode: publisherDraft.publish_mode?.trim() || "publish",
        credential_ref: undefined,
      });
      if (publisherCredentialDraft.trim()) {
        saved = await api.upsertPublisherCredential(projectId, saved.id, {
          kind: "github_token",
          value: publisherCredentialDraft.trim(),
        });
        setPublisherCredentialDraft("");
      }
      setPublisherConnections((current) => {
        const rest = current.filter((connection) => connection.id !== saved.id && connection.kind !== saved.kind);
        return [saved, ...rest];
      });
      setMessage({
        title: "Publisher connection saved",
        detail: publisherCredentialDraft.trim() ? "Connection details and GitHub token were saved." : "Connection details were saved.",
        tone: "green",
      });
    } catch (e: any) {
      setMessage({ title: "Publisher save failed", detail: readableError(e), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function savePublisherCredential() {
    if (!githubPublisher) {
      setMessage({ title: "Save publisher first", tone: "amber" });
      return;
    }
    const value = publisherCredentialDraft.trim();
    if (!value) {
      setMessage({ title: "GitHub token required", tone: "amber" });
      return;
    }
    setNotificationBusy(`save-publisher-credential-${githubPublisher.id}`);
    setMessage(null);
    try {
      const saved = await api.upsertPublisherCredential(projectId, githubPublisher.id, { kind: "github_token", value });
      setPublisherCredentialDraft("");
      setPublisherConnections((current) => current.map((connection) => (connection.id === saved.id ? saved : connection)));
      setMessage({ title: "Publisher credential saved", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Credential save failed", detail: readableError(e), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function revokePublisherCredential() {
    if (!githubPublisher) return;
    setNotificationBusy(`revoke-publisher-credential-${githubPublisher.id}`);
    setMessage(null);
    try {
      const saved = await api.revokePublisherCredential(projectId, githubPublisher.id);
      setPublisherConnections((current) => current.map((connection) => (connection.id === saved.id ? saved : connection)));
      setMessage({ title: "Publisher credential revoked", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Credential revoke failed", detail: readableError(e), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function testPublisherConnection(connectionID: string) {
    setNotificationBusy(`test-publisher-${connectionID}`);
    setMessage(null);
    try {
      const tested = await api.testPublisherConnection(projectId, connectionID);
      setPublisherConnections((current) => current.map((connection) => (connection.id === tested.id ? tested : connection)));
      setMessage({ title: "Publisher connection verified", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Publisher test failed", detail: readableError(e), tone: "red" });
      await refreshPublisherConnections();
    } finally {
      setNotificationBusy(null);
    }
  }

  async function toggleSubscription(eventType: string, channelID: string, enabled: boolean) {
    setNotificationBusy(`sub-${eventType}-${channelID}`);
    setMessage(null);
    try {
      await api.upsertNotificationSubscription(projectId, { event_type: eventType, channel_id: channelID, enabled });
      await refreshNotifications();
    } catch (e: any) {
      setMessage({ title: "Subscription update failed", detail: readableError(e), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function retryDelivery(deliveryID: string) {
    setNotificationBusy(`retry-${deliveryID}`);
    setMessage(null);
    try {
      await api.retryNotificationDelivery(projectId, deliveryID);
      setMessage({ title: "Delivery queued", tone: "green" });
      await refreshNotifications();
    } catch (e: any) {
      setMessage({ title: "Delivery retry failed", detail: readableError(e), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  function subscriptionEnabled(eventType: string, channelID: string) {
    return subscriptions.some((sub) => sub.event_type === eventType && sub.channel_id === channelID && sub.enabled);
  }

  const githubPublisher = publisherConnections.find((connection) => connection.kind === "github_nextjs");

  return (
    <div className="space-y-7">
      <SectionHeader title="Settings" eyebrow="Project controls" />
      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      <SectionHeader title="General" eyebrow="Cadence and budget" />
      <section className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
        <div className="grid gap-4 md:grid-cols-3">
          <Field label="Cadence per week">
            <TextInput
              inputMode="numeric"
              value={config.cadence_per_week}
              onChange={(event) => update({ cadence_per_week: Math.max(1, toInt(event.target.value, 3)) })}
            />
          </Field>
          <Field label="Buffer days">
            <TextInput
              inputMode="numeric"
              value={config.buffer_days}
              onChange={(event) => update({ buffer_days: Math.max(0, toInt(event.target.value, 5)) })}
            />
          </Field>
          <Field label="Monthly budget USD">
            <TextInput
              inputMode="decimal"
              value={config.monthly_budget_usd}
              onChange={(event) => update({ monthly_budget_usd: Math.max(0, toFloat(event.target.value, 50)) })}
            />
          </Field>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <Field label="Blog mix">
            <TextInput
              inputMode="decimal"
              value={config.channel_mix.blog}
              onChange={(event) =>
                update({ channel_mix: { ...config.channel_mix, blog: Math.max(0, toFloat(event.target.value, 0.6)) } })
              }
            />
          </Field>
          <Field label="Syndication mix">
            <TextInput
              inputMode="decimal"
              value={config.channel_mix.syndication}
              onChange={(event) =>
                update({
                  channel_mix: { ...config.channel_mix, syndication: Math.max(0, toFloat(event.target.value, 0.4)) },
                })
              }
            />
          </Field>
        </div>

        <Field label="Brand voice">
          <TextArea
            value={config.brand_voice ?? ""}
            onChange={(event) => update({ brand_voice: event.target.value })}
            className="min-h-24"
            placeholder="Direct, evidence-backed, pragmatic."
          />
        </Field>
      </section>

      <section>
        <SectionHeader
          title="Publisher connection"
          eyebrow="Publishing"
          action={
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="primary" onClick={savePublisherConnection} disabled={notificationBusy === "save-publisher"}>
                <Save size={14} />
                Save publisher
              </Button>
              <Button size="sm" onClick={refreshPublisherConnections} disabled={notificationBusy === "save-publisher"}>
                <RefreshCw size={14} />
                Refresh
              </Button>
            </div>
          }
        />
        <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2 text-sm font-semibold text-slate-900">
              <GitBranch size={16} />
              GitHub/Next.js
            </div>
            <Badge tone={githubPublisher?.status === "connected" ? "green" : githubPublisher?.status === "error" ? "red" : "amber"}>
              {githubPublisher?.status ?? "missing"}
            </Badge>
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <Field label="Repository">
              <TextInput
                value={publisherDraft.repo}
                onChange={(event) => setPublisherDraft((current) => ({ ...current, repo: event.target.value }))}
                placeholder="owner/repo"
              />
            </Field>
            <Field label="Base URL">
              <TextInput
                value={publisherDraft.base_url}
                onChange={(event) => setPublisherDraft((current) => ({ ...current, base_url: event.target.value }))}
                placeholder="https://example.com/blog"
              />
            </Field>
            <Field label="GitHub access token" helper="Used only to write generated articles to the selected repository. The saved token is never shown again.">
              <TextInput
                value={publisherCredentialDraft}
                onChange={(event) => setPublisherCredentialDraft(event.target.value)}
                placeholder={githubPublisher?.credential_configured ? "Saved" : "ghp_..."}
                type="password"
                autoComplete="off"
              />
            </Field>
            <div className="flex items-end">
              <Badge tone={githubPublisher?.credential_configured ? "green" : "amber"}>
                {githubPublisher?.credential_configured ? "Credential saved" : "Credential missing"}
              </Badge>
            </div>
          </div>

          <div className="border-t border-slate-200 pt-2">
            <button
              type="button"
              onClick={() => setShowPublisherAdvanced((current) => !current)}
              className="flex w-full items-center justify-between py-2 text-left text-sm font-semibold text-slate-700"
            >
              Advanced publishing fields
              <span className="text-xs text-slate-500">{showPublisherAdvanced ? "Hide" : "Show"}</span>
            </button>
            {showPublisherAdvanced && (
              <div className="grid gap-4 pt-3 lg:grid-cols-3">
                <Field label="Branch">
                  <TextInput
                    value={publisherDraft.branch ?? ""}
                    onChange={(event) => setPublisherDraft((current) => ({ ...current, branch: event.target.value }))}
                    placeholder="citeloop-content"
                  />
                </Field>
                <Field label="Content path">
                  <TextInput
                    value={publisherDraft.content_dir ?? ""}
                    onChange={(event) => setPublisherDraft((current) => ({ ...current, content_dir: event.target.value }))}
                    placeholder="content/citeloop/blog"
                  />
                </Field>
                <Field label="Publish mode">
                  <TextInput
                    value={publisherDraft.publish_mode ?? "publish"}
                    onChange={(event) => setPublisherDraft((current) => ({ ...current, publish_mode: event.target.value }))}
                    placeholder="publish"
                  />
                </Field>
              </div>
            )}
          </div>

          {githubPublisher?.last_error && <Notice title="Publisher health check failed" detail={readableError(githubPublisher.last_error)} tone="red" />}

          <div className="flex flex-wrap gap-2">
            <Button variant="primary" onClick={savePublisherConnection} disabled={notificationBusy === "save-publisher"}>
              <Save size={16} />
              Save publisher
            </Button>
            <Button
              variant="outline"
              onClick={savePublisherCredential}
              disabled={!githubPublisher || !publisherCredentialDraft.trim() || notificationBusy === `save-publisher-credential-${githubPublisher?.id}`}
            >
              <Save size={16} />
              Save token
            </Button>
            <Button
              variant="outline"
              onClick={() => githubPublisher && testPublisherConnection(githubPublisher.id)}
              disabled={!githubPublisher || notificationBusy === `test-publisher-${githubPublisher?.id}`}
            >
              <Send size={16} />
              Test
            </Button>
            <Button
              variant="outline"
              onClick={revokePublisherCredential}
              disabled={!githubPublisher?.credential_configured || notificationBusy === `revoke-publisher-credential-${githubPublisher?.id}`}
            >
              <Trash2 size={16} />
              Revoke token
            </Button>
          </div>
        </div>
      </section>

      <section>
        <SectionHeader title="Crawl policy" />
        <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
          <div className="grid gap-4 md:grid-cols-3">
            <Field label="Max pages">
              <TextInput
                inputMode="numeric"
                value={config.crawl.max_pages}
                onChange={(event) =>
                  update({ crawl: { ...config.crawl, max_pages: Math.max(1, toInt(event.target.value, 200)) } })
                }
              />
            </Field>
            <Field label="Max depth">
              <TextInput
                inputMode="numeric"
                value={config.crawl.max_depth}
                onChange={(event) =>
                  update({ crawl: { ...config.crawl, max_depth: Math.max(1, toInt(event.target.value, 3)) } })
                }
              />
            </Field>
            <Field label="Request timeout ms">
              <TextInput
                inputMode="numeric"
                value={config.crawl.request_timeout_ms}
                onChange={(event) =>
                  update({
                    crawl: { ...config.crawl, request_timeout_ms: Math.max(1000, toInt(event.target.value, 8000)) },
                  })
                }
              />
            </Field>
            <Field label="Rate limit RPS">
              <TextInput
                inputMode="numeric"
                value={config.crawl.rate_limit_rps}
                onChange={(event) =>
                  update({ crawl: { ...config.crawl, rate_limit_rps: Math.max(1, toInt(event.target.value, 1)) } })
                }
              />
            </Field>
            <Field label="Sitemap URL cap">
              <TextInput
                inputMode="numeric"
                value={config.crawl.sitemap_url_cap}
                onChange={(event) =>
                  update({ crawl: { ...config.crawl, sitemap_url_cap: Math.max(1, toInt(event.target.value, 2000)) } })
                }
              />
            </Field>
          </div>

          <div className="flex flex-wrap gap-4 text-sm font-semibold text-slate-700">
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={config.crawl.same_origin_only}
                onChange={(event) => update({ crawl: { ...config.crawl, same_origin_only: event.target.checked } })}
              />
              Same origin only
            </label>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={config.crawl.respect_robots}
                onChange={(event) => update({ crawl: { ...config.crawl, respect_robots: event.target.checked } })}
              />
              Respect robots
            </label>
          </div>
        </div>
      </section>

      <section>
        <SectionHeader
          title="Notifications"
          eyebrow="Operations"
          action={
            <Button size="sm" onClick={refreshNotifications} disabled={!!notificationBusy}>
              <RefreshCw size={14} />
              Refresh
            </Button>
          }
        />

        <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
          <div className="flex items-center gap-2 text-sm font-semibold text-slate-900">
            <Bell size={16} />
            Channels
          </div>

          <div className="grid gap-3 lg:grid-cols-[220px_1fr_1fr_auto]">
            <div className="grid grid-cols-2 gap-2">
              {channelKinds.map((kind) => {
                const active = channelDraft.kind === kind.value;
                return (
                  <button
                    type="button"
                    key={kind.value}
                    onClick={() => setChannelDraft((current) => ({ ...current, kind: kind.value }))}
                    className={cx(
                      "h-10 rounded-lg border text-sm font-semibold transition-colors",
                      active ? "border-[#d93820] bg-red-50 text-[#b92f1c]" : "border-slate-200 text-slate-600 hover:bg-slate-50",
                    )}
                  >
                    {kind.label}
                  </button>
                );
              })}
            </div>
            <TextInput
              value={channelDraft.label}
              onChange={(event) => setChannelDraft((current) => ({ ...current, label: event.target.value }))}
              placeholder="Label"
            />
            <TextInput
              value={channelDraft.url}
              onChange={(event) => setChannelDraft((current) => ({ ...current, url: event.target.value }))}
              placeholder={channelDraft.kind === "slack_webhook" ? "https://hooks.slack.com/services/..." : "https://discord.com/api/webhooks/..."}
              type="password"
              autoComplete="off"
            />
            <Button variant="primary" onClick={createChannel} disabled={notificationBusy === "create-channel"}>
              <Plus size={16} />
              Add channel
            </Button>
          </div>
          <div className="text-xs font-medium text-slate-500">{webhookURLHint(channelDraft.kind)} Use Test after saving to verify delivery.</div>

          {channels.length === 0 ? (
            <div className="rounded-lg border border-dashed border-slate-200 px-4 py-5 text-sm font-semibold text-slate-500">
              No channels
            </div>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-slate-200">
              <table className="min-w-full divide-y divide-slate-200 text-sm">
                <thead className="bg-slate-50 text-left text-xs font-semibold uppercase text-slate-500">
                  <tr>
                    <th className="px-3 py-2">Label</th>
                    <th className="px-3 py-2">Kind</th>
                    <th className="px-3 py-2">Webhook</th>
                    <th className="px-3 py-2">Status</th>
                    <th className="px-3 py-2 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {channels.map((channel) => (
                    <tr key={channel.id}>
                      <td className="px-3 py-2 font-semibold text-slate-900">{channel.label || "Webhook"}</td>
                      <td className="px-3 py-2">
                        <Badge tone={channel.kind === "slack_webhook" ? "green" : "blue"}>
                          {channel.kind === "slack_webhook" ? "Slack" : "Discord"}
                        </Badge>
                      </td>
                      <td className="max-w-[320px] truncate px-3 py-2 text-slate-500">{channel.config?.redacted_url ?? "Redacted"}</td>
                      <td className="px-3 py-2">
                        <Badge tone={channel.verified_at ? "green" : "amber"}>
                          {channel.verified_at ? "Verified" : "Untested"}
                        </Badge>
                      </td>
                      <td className="px-3 py-2 text-right">
                        <div className="flex justify-end gap-2">
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => testChannel(channel.id)}
                            disabled={notificationBusy === `test-${channel.id}`}
                            title={channel.verified_at ? `Verified ${formatDate(channel.verified_at)}` : "Send test notification"}
                          >
                            <Send size={14} />
                            Test
                          </Button>
                          <Button
                            size="sm"
                            variant="danger"
                            onClick={() => deleteChannel(channel.id)}
                            disabled={notificationBusy === `delete-${channel.id}`}
                            title="Delete channel"
                          >
                            <Trash2 size={14} />
                            Delete
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </section>

      <section>
        <SectionHeader title="Notification subscriptions" />
        <div className="rounded-xl border border-slate-200 bg-white p-4">
          {channels.length === 0 ? (
            <div className="text-sm font-semibold text-slate-500">No channels</div>
          ) : (
            <div className="grid gap-3">
              {events.map((event) => (
                <div key={event.type} className="grid gap-3 rounded-lg border border-slate-200 p-3 lg:grid-cols-[220px_1fr]">
                  <div>
                    <div className="text-sm font-bold text-slate-900">{eventLabels[event.type] ?? event.type}</div>
                    <div className="mt-1 font-mono text-xs text-slate-500">{event.type}</div>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {channels.map((channel) => {
                      const checked = subscriptionEnabled(event.type, channel.id);
                      return (
                        <label
                          key={`${event.type}-${channel.id}`}
                          className={cx(
                            "flex h-9 items-center gap-2 rounded-lg border px-3 text-sm font-semibold transition-colors",
                            checked ? "border-green-200 bg-green-50 text-green-800" : "border-slate-200 text-slate-600",
                          )}
                        >
                          <input
                            type="checkbox"
                            checked={checked}
                            disabled={notificationBusy === `sub-${event.type}-${channel.id}`}
                            onChange={(changeEvent) => toggleSubscription(event.type, channel.id, changeEvent.target.checked)}
                          />
                          {channel.label || (channel.kind === "slack_webhook" ? "Slack" : "Discord")}
                        </label>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </section>

      <section>
        <SectionHeader
          title="Notification deliveries"
          action={
            <div className="flex gap-1 rounded-lg border border-slate-200 bg-white p-1">
              {deliveryStatuses.map((status) => (
                <button
                  type="button"
                  key={status.value}
                  onClick={() => setDeliveryStatus(status.value)}
                  className={cx(
                    "h-7 rounded-md px-2 text-xs font-semibold transition-colors",
                    deliveryStatus === status.value ? "bg-slate-900 text-white" : "text-slate-600 hover:bg-slate-100",
                  )}
                >
                  {status.label}
                </button>
              ))}
            </div>
          }
        />
        <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white">
          {deliveries.length === 0 ? (
            <div className="px-4 py-5 text-sm font-semibold text-slate-500">No deliveries</div>
          ) : (
            <table className="min-w-full divide-y divide-slate-200 text-sm">
              <thead className="bg-slate-50 text-left text-xs font-semibold uppercase text-slate-500">
                <tr>
                  <th className="px-3 py-2">Event</th>
                  <th className="px-3 py-2">Status</th>
                  <th className="px-3 py-2">Attempts</th>
                  <th className="px-3 py-2">Last error</th>
                  <th className="px-3 py-2">Created</th>
                  <th className="px-3 py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {deliveries.map((delivery) => (
                  <tr key={delivery.id}>
                    <td className="px-3 py-2">
                      <div className="font-semibold text-slate-900">{eventLabels[delivery.event_type] ?? delivery.event_type}</div>
                      <div className="max-w-[260px] truncate font-mono text-xs text-slate-500">{delivery.event_id}</div>
                    </td>
                    <td className="px-3 py-2">
                      <Badge tone={delivery.status === "sent" ? "green" : delivery.status === "dead" ? "red" : "amber"}>
                        {delivery.status}
                      </Badge>
                    </td>
                    <td className="px-3 py-2 text-slate-600">{delivery.attempts ?? 0}</td>
                    <td className="max-w-[360px] truncate px-3 py-2 text-slate-500">{delivery.last_error ?? "-"}</td>
                    <td className="px-3 py-2 text-slate-500">{formatDate(delivery.created_at)}</td>
                    <td className="px-3 py-2 text-right">
                      <Button
                        size="sm"
                        onClick={() => retryDelivery(delivery.id)}
                        disabled={delivery.status === "sent" || notificationBusy === `retry-${delivery.id}`}
                        title="Retry delivery"
                      >
                        <RotateCcw size={14} />
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </section>

      <Button disabled={busy} variant="primary" onClick={save}>
        <Save size={16} />
        Save settings
      </Button>
    </div>
  );
}
