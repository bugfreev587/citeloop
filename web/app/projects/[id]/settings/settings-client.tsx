"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Activity, Bell, CheckCircle2, GitBranch, Plus, RefreshCw, RotateCcw, Save, Search, Send, Trash2 } from "lucide-react";
import {
  defaultProjectConfig,
  GSCConnection,
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
import { Badge, Button, ButtonProgress, Field, Notice, SectionHeader, TextInput, TextArea, cx, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

function toInt(value: string, fallback: number) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function toFloat(value: string, fallback: number) {
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

// Map raw backend/API error strings to actionable user copy. Falls back to the
// raw message so nothing is hidden, but common validation/auth failures read
// like guidance instead of server jargon.
function friendlyError(raw: unknown) {
  const message = String(raw ?? "").trim();
  const lower = message.toLowerCase();
  if (lower.includes("repo") && lower.includes("base_url")) {
    return "Add both the GitHub repository (owner/repo) and your site's base URL before saving.";
  }
  if (lower.includes("base_url") || lower.includes("base url")) {
    return "Enter a valid site base URL, e.g. https://example.com.";
  }
  if (lower.includes("repo")) {
    return "Enter the GitHub repository as owner/repo.";
  }
  if (lower.includes("webhook") || (lower.includes("url") && lower.includes("required"))) {
    return "Enter a valid webhook URL (a Slack or Discord incoming webhook).";
  }
  if (lower.includes("token")) {
    return "The token was rejected. Check that it is valid and has write access to the repository.";
  }
  if (lower.includes("401") || lower.includes("403") || lower.includes("permission") || lower.includes("forbidden")) {
    return "Permission denied. Re-check the connected credentials and their access scope.";
  }
  if (lower.includes("404") || lower.includes("not found")) {
    return "Not found. Check the repository, branch, and content path.";
  }
  return message || "Something went wrong. Please try again.";
}

function isProjectScopedMissing(raw: unknown) {
  const lower = String(raw ?? "").toLowerCase();
  return lower.includes("404") && lower.includes("project not found");
}

function gscTone(status?: string): "green" | "amber" | "red" | "neutral" {
  if (status === "connected") return "green";
  if (["error", "expired", "revoked", "stale", "mismatch"].includes(status ?? "")) return "red";
  if (["property_selection_required", "missing", "backfilling"].includes(status ?? "")) return "amber";
  return "neutral";
}

function gscStatusLabel(status?: string) {
  if (status === "property_selection_required") return "select property";
  if (status === "backfilling") return "backfilling";
  if (status === "stale") return "stale";
  if (status === "mismatch") return "mismatch";
  return status ?? "missing";
}

function EmptyGSCProperties() {
  return (
    <div className="rounded-lg border border-dashed border-slate-200 bg-white px-3 py-4 text-sm">
      <div className="font-semibold text-slate-800">No authorized properties yet</div>
      <div className="mt-1 text-slate-500">Connect Search Console, then choose the property that matches this domain.</div>
    </div>
  );
}

function GSCSetupGuide({ siteURL }: { siteURL?: string }) {
  const target = siteURL?.trim() || "this product domain";
  const steps = [
    {
      title: "Set up Search Console property",
      detail: `Add a Domain property for ${target}. Domain properties cover all protocols and subdomains.`,
    },
    {
      title: "Verify DNS ownership",
      detail: "Copy Google's TXT record into your DNS provider and wait for Google to confirm ownership.",
    },
    {
      title: "Connect after verification",
      detail: "Return here, connect Search Console, and select the verified property for this project.",
    },
  ];
  return (
    <div className="grid gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="text-sm font-bold text-amber-950">Set up Search Console property</div>
          <p className="mt-1 max-w-2xl text-sm leading-5 text-amber-800">
            If this domain is not in your Google account yet, verify it in Search Console first. CiteLoop can only import properties Google already shows to you.
          </p>
        </div>
        <a
          href="https://search.google.com/search-console/welcome"
          target="_blank"
          rel="noreferrer"
          className="inline-flex h-9 shrink-0 items-center justify-center rounded-lg border border-amber-300 bg-white px-3 text-sm font-semibold text-amber-900 transition-colors hover:bg-amber-100"
        >
          Open Search Console
        </a>
      </div>
      <div className="grid gap-2 md:grid-cols-3">
        {steps.map((step, index) => (
          <div key={step.title} className="min-w-0 border-t border-amber-200 pt-2 md:border-l md:border-t-0 md:pl-3 md:pt-0">
            <div className="text-xs font-semibold uppercase text-amber-700">Step {index + 1}</div>
            <div className="mt-1 text-sm font-bold text-amber-950">{step.title}</div>
            <p className="mt-1 text-sm leading-5 text-amber-800">{step.detail}</p>
          </div>
        ))}
      </div>
    </div>
  );
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
  const [publisherConnections, setPublisherConnections] = useState<PublisherConnection[]>([]);
  const [publisherDraft, setPublisherDraft] = useState<GitHubNextJSPublisherInput>(defaultPublisherDraft);
  const [publisherCredentialDraft, setPublisherCredentialDraft] = useState("");
  const [gscConnection, setGSCConnection] = useState<GSCConnection | null>(null);
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
  const [gscBusy, setGSCBusy] = useState<string | null>(null);
  const [notificationBusy, setNotificationBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    try {
      const project = await api.getProject(projectId);
      setConfig(project.config);
    } catch (e: any) {
      setMessage({ title: "Settings unavailable", detail: e.message, tone: "amber" });
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
      if (isProjectScopedMissing(e.message)) {
        setPublisherConnections([]);
        return;
      }
      setMessage({ title: "Publisher connections unavailable", detail: friendlyError(e.message), tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refreshPublisherConnections();
  }, [refreshPublisherConnections]);

  const refreshGSCConnection = useCallback(async () => {
    try {
      const connection = await api.getGSCConnection(projectId);
      setGSCConnection(connection);
    } catch (e: any) {
      if (isProjectScopedMissing(e.message)) {
        setGSCConnection(null);
        return;
      }
      setMessage({ title: "Search Console connection unavailable", detail: friendlyError(e.message), tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refreshGSCConnection();
  }, [refreshGSCConnection]);

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
      if (isProjectScopedMissing(e.message)) {
        setChannels([]);
        setEvents([]);
        setSubscriptions([]);
        setDeliveries([]);
        return;
      }
      setMessage({ title: "Notifications unavailable", detail: friendlyError(e.message), tone: "amber" });
    }
  }, [api, deliveryStatus, projectId]);

  useEffect(() => {
    refreshNotifications();
  }, [refreshNotifications]);

  function update(next: Partial<ProjectConfig>) {
    setConfig((current) => ({ ...current, ...next }));
  }

  async function save() {
    if ((config.monthly_budget_usd ?? 0) <= 0) {
      const ok = window.confirm("Set the monthly budget to $0? This pauses all automated generation and SEO work until you raise it.");
      if (!ok) return;
    }
    setBusy(true);
    setMessage(null);
    try {
      const fullPayload = {
        ...defaultProjectConfig(),
        ...config,
        crawl: { ...defaultProjectConfig().crawl, ...config.crawl },
        channel_mix: { ...defaultProjectConfig().channel_mix, ...config.channel_mix },
      };
      await api.updateConfig(projectId, fullPayload);
      setConfig(fullPayload);
      setMessage({ title: "Settings saved", detail: "Cadence, budget, and crawl settings are updated.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Settings save failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setBusy(false);
    }
  }

  async function createChannel() {
    const url = channelDraft.url.trim();
    if (!url) {
      setMessage({ title: "Webhook URL required", tone: "amber" });
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
      setMessage({ title: "Channel save failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function deleteChannel(channelID: string) {
    if (!window.confirm("Delete this notification channel? Subscriptions using it will stop delivering.")) return;
    setNotificationBusy(`delete-${channelID}`);
    setMessage(null);
    try {
      await api.deleteNotificationChannel(projectId, channelID);
      setMessage({ title: "Notification channel deleted", tone: "green" });
      await refreshNotifications();
    } catch (e: any) {
      setMessage({ title: "Channel delete failed", detail: e.message, tone: "red" });
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
      setMessage({ title: "Test notification failed", detail: e.message, tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function savePublisherConnection() {
    setNotificationBusy("save-publisher");
    setMessage(null);
    try {
      let saved = await api.upsertGitHubNextJSPublisherConnection(projectId, {
        ...publisherDraft,
        repo: publisherDraft.repo.trim(),
        branch: publisherDraft.branch?.trim() || "citeloop-content",
        content_dir: publisherDraft.content_dir?.trim() || "content/citeloop/blog",
        base_url: publisherDraft.base_url.trim(),
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
      setMessage({ title: "Publisher connection saved", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Publisher save failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function savePublisherCredential() {
    if (!githubPublisher) {
      setMessage({ title: "Save publisher first", detail: "Create the GitHub/Next.js connection before saving a token.", tone: "amber" });
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
      const saved = await api.upsertPublisherCredential(projectId, githubPublisher.id, {
        kind: "github_token",
        value,
      });
      setPublisherCredentialDraft("");
      setPublisherConnections((current) => current.map((connection) => (connection.id === saved.id ? saved : connection)));
      setMessage({ title: "Publisher credential saved", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Credential save failed", detail: friendlyError(e.message), tone: "red" });
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
      setMessage({ title: "Credential revoke failed", detail: e.message, tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function startSearchConsoleOAuth() {
    setGSCBusy("connect");
    setMessage(null);
    try {
      const result = await api.startGSCOAuth(projectId);
      window.location.assign(result.authorization_url);
    } catch (e: any) {
      setMessage({ title: "Search Console connect failed", detail: friendlyError(e.message), tone: "red" });
      setGSCBusy(null);
    }
  }

  async function selectGSCProperty(siteURL: string) {
    setGSCBusy(`select-${siteURL}`);
    setMessage(null);
    try {
      const connection = await api.selectGSCProperty(projectId, { site_url: siteURL });
      setGSCConnection(connection);
      setMessage({ title: "Search Console property selected", detail: siteURL, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Property selection failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setGSCBusy(null);
    }
  }

  async function revokeGSCConnection() {
    if (!window.confirm("Disconnect Search Console? CiteLoop will keep working, but analysis will lose first-party search data.")) return;
    setGSCBusy("revoke");
    setMessage(null);
    try {
      const connection = await api.revokeGSCConnection(projectId);
      setGSCConnection(connection);
      setMessage({ title: "Search Console disconnected", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Search Console disconnect failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setGSCBusy(null);
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
      setMessage({ title: "Publisher test failed", detail: e.message, tone: "red" });
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
      setMessage({ title: "Subscription update failed", detail: e.message, tone: "red" });
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
      setMessage({ title: "Delivery retry failed", detail: e.message, tone: "red" });
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
      <SectionHeader title="Settings" eyebrow="Project config" />
      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      <section>
        <SectionHeader title="Activity Log" eyebrow="Automation audit" />
        <div className="flex flex-col gap-3 rounded-xl border border-slate-200 bg-white p-4 md:flex-row md:items-center md:justify-between">
          <div className="flex gap-3">
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-600">
              <Activity size={18} />
            </div>
            <div>
              <div className="text-sm font-bold text-slate-900">Review automation health when something needs attention.</div>
              <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-500">
                Failed, degraded, and budget-stopped activity lives here so primary navigation stays focused on user outcomes.
              </p>
            </div>
          </div>
          <Link
            href={`/projects/${projectId}/settings/activity`}
            className="inline-flex h-9 items-center justify-center rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50"
          >
            Open Activity Log
          </Link>
        </div>
      </section>

      <section>
        <SectionHeader
          title="Search Console connection"
          eyebrow="Analysis data"
          action={
            <Button size="sm" onClick={refreshGSCConnection} disabled={Boolean(gscBusy)}>
              <RefreshCw size={14} />
              Refresh
            </Button>
          }
        />
        <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
          <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div className="flex gap-3">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-600">
                <Search size={18} />
              </div>
              <div>
                <div className="text-sm font-bold text-slate-900">Connect Search Console for first-party search data.</div>
                <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-500">
                  CiteLoop uses the selected property for query, CTR, position, and content-decay signals in Analysis and Results.
                </p>
              </div>
            </div>
            <Badge tone={gscTone(gscConnection?.status)}>{gscStatusLabel(gscConnection?.status)}</Badge>
          </div>

          {gscConnection && !gscConnection.configured && (
            <Notice
              title="Google OAuth is not configured"
              detail="Add GOOGLE_OAUTH_CLIENT_ID, GOOGLE_OAUTH_CLIENT_SECRET, and PUBLIC_APP_URL before customers can connect Search Console."
              tone="amber"
            />
          )}

          {gscConnection?.last_error && <Notice title="Search Console needs attention" detail={gscConnection.last_error} tone="red" />}

          {(!gscConnection || gscConnection.status === "missing" || gscConnection.properties.length === 0) && <GSCSetupGuide siteURL={config.site_url} />}

          <div className="flex flex-wrap gap-2">
            <Button variant="primary" onClick={startSearchConsoleOAuth} disabled={Boolean(gscBusy) || gscConnection?.configured === false}>
              <ButtonProgress busy={gscBusy === "connect"} busyLabel="Opening Google" idleIcon={<Search size={16} />}>
                {gscConnection?.status === "connected" ? "Reconnect Search Console" : "Connect Search Console"}
              </ButtonProgress>
            </Button>
            <Button
              variant="outline"
              onClick={revokeGSCConnection}
              disabled={!gscConnection || gscConnection.status === "missing" || gscConnection.status === "revoked" || Boolean(gscBusy)}
            >
              <ButtonProgress busy={gscBusy === "revoke"} busyLabel="Disconnecting" idleIcon={<Trash2 size={16} />}>
                Disconnect
              </ButtonProgress>
            </Button>
          </div>

          <div className="grid gap-3 rounded-lg border border-slate-100 bg-slate-50 p-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <div className="text-sm font-bold text-slate-900">Authorized properties</div>
                <p className="mt-1 text-sm leading-5 text-slate-500">Select property that matches this project domain.</p>
              </div>
              {gscConnection?.recommended_property && <Badge tone="green">Recommended: {gscConnection.recommended_property}</Badge>}
            </div>

            {gscConnection && gscConnection.properties.length > 0 ? (
              <div className="grid gap-2">
                {gscConnection.properties.map((property) => {
                  const selected = gscConnection.selected_property === property.site_url;
                  return (
                    <div key={property.site_url} className="flex flex-col gap-2 rounded-lg bg-white px-3 py-3 sm:flex-row sm:items-center sm:justify-between">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="truncate text-sm font-semibold text-slate-900">{property.site_url}</span>
                          {property.recommended && <Badge tone="green">Recommended</Badge>}
                          {selected && <Badge tone="blue">Selected</Badge>}
                        </div>
                        <div className="mt-1 text-xs font-semibold text-slate-400">{property.permission_level || "permission available"}</div>
                      </div>
                      <Button size="sm" variant={selected ? "ghost" : "outline"} onClick={() => selectGSCProperty(property.site_url)} disabled={selected || Boolean(gscBusy)}>
                        <ButtonProgress busy={gscBusy === `select-${property.site_url}`} busyLabel="Selecting" idleIcon={<CheckCircle2 size={14} />}>
                          Select property
                        </ButtonProgress>
                      </Button>
                    </div>
                  );
                })}
              </div>
            ) : (
              <EmptyGSCProperties />
            )}
          </div>
        </div>
      </section>

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
            <Button size="sm" onClick={refreshPublisherConnections} disabled={notificationBusy === "save-publisher"}>
              <RefreshCw size={14} />
              Refresh
            </Button>
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
            <Field label="Base URL">
              <TextInput
                value={publisherDraft.base_url}
                onChange={(event) => setPublisherDraft((current) => ({ ...current, base_url: event.target.value }))}
                placeholder="https://example.com/blog"
              />
            </Field>
            <Field label="Publish mode">
              <TextInput
                value={publisherDraft.publish_mode ?? "publish"}
                onChange={(event) => setPublisherDraft((current) => ({ ...current, publish_mode: event.target.value }))}
                placeholder="publish"
              />
            </Field>
            <Field label="GitHub token">
              <div className="grid gap-2">
                <TextInput
                  type="password"
                  value={publisherCredentialDraft}
                  onChange={(event) => setPublisherCredentialDraft(event.target.value)}
                  placeholder={githubPublisher?.credential_configured ? "Saved" : "ghp_..."}
                  autoComplete="off"
                />
                <Badge tone={githubPublisher?.credential_configured ? "green" : "amber"}>
                  {githubPublisher?.credential_configured ? "Credential saved" : "Credential missing"}
                </Badge>
              </div>
            </Field>
          </div>

          <div className="border-t border-slate-100 pt-4">
            <div className="text-sm font-bold text-slate-900">CMS connector roadmap</div>
            <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-500">
              WordPress, Webflow, Shopify, and custom CMS connectors will start as gated drafts and move to OAuth publishing after verification is ready.
            </p>
            <div className="mt-3 grid gap-2 md:grid-cols-2">
              {["WordPress", "Webflow", "Shopify", "Custom CMS"].map((name) => (
                <div key={name} className="flex items-center justify-between gap-3 rounded-lg border border-slate-100 bg-slate-50 px-3 py-2">
                  <div className="min-w-0">
                    <div className="text-sm font-semibold text-slate-900">{name}</div>
                    <div className="mt-0.5 text-xs font-semibold uppercase text-slate-400">Draft-only until OAuth connector is ready</div>
                  </div>
                  <Badge tone="neutral">Roadmap</Badge>
                </div>
              ))}
            </div>
          </div>

          {githubPublisher?.last_error && <Notice title="Publisher health check failed" detail={githubPublisher.last_error} tone="red" />}


          <div className="flex flex-wrap gap-2">
            <Button variant="primary" onClick={savePublisherConnection} disabled={notificationBusy === "save-publisher"}>
              <ButtonProgress busy={notificationBusy === "save-publisher"} busyLabel="Saving publisher" idleIcon={<Save size={16} />}>
                Save publisher
              </ButtonProgress>
            </Button>
            <Button
              variant="outline"
              onClick={savePublisherCredential}
              disabled={!githubPublisher || !publisherCredentialDraft.trim() || notificationBusy === `save-publisher-credential-${githubPublisher?.id}`}
            >
              <ButtonProgress
                busy={notificationBusy === `save-publisher-credential-${githubPublisher?.id}`}
                busyLabel="Saving token"
                idleIcon={<Save size={16} />}
              >
                Save token
              </ButtonProgress>
            </Button>
            <Button
              variant="outline"
              onClick={() => githubPublisher && testPublisherConnection(githubPublisher.id)}
              disabled={!githubPublisher || notificationBusy === `test-publisher-${githubPublisher?.id}`}
            >
              <ButtonProgress busy={notificationBusy === `test-publisher-${githubPublisher?.id}`} busyLabel="Testing" idleIcon={<Send size={16} />}>
                Test
              </ButtonProgress>
            </Button>
            <Button
              variant="outline"
              onClick={revokePublisherCredential}
              disabled={!githubPublisher?.credential_configured || notificationBusy === `revoke-publisher-credential-${githubPublisher?.id}`}
            >
              <ButtonProgress
                busy={notificationBusy === `revoke-publisher-credential-${githubPublisher?.id}`}
                busyLabel="Revoking token"
                idleIcon={<Trash2 size={16} />}
              >
                Revoke token
              </ButtonProgress>
            </Button>
          </div>
        </div>
      </section>

      <section>
        <SectionHeader title="Crawl config" />
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
              placeholder="Webhook URL"
              type="password"
              autoComplete="off"
            />
            <Button variant="primary" onClick={createChannel} disabled={notificationBusy === "create-channel"}>
              <ButtonProgress busy={notificationBusy === "create-channel"} busyLabel="Adding channel" idleIcon={<Plus size={16} />}>
                Add
              </ButtonProgress>
            </Button>
          </div>

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
                            <ButtonProgress busy={notificationBusy === `test-${channel.id}`} busyLabel="Testing" idleIcon={<Send size={14} />} />
                          </Button>
                          <Button
                            size="sm"
                            variant="danger"
                            onClick={() => deleteChannel(channel.id)}
                            disabled={notificationBusy === `delete-${channel.id}`}
                            title="Delete channel"
                          >
                            <ButtonProgress busy={notificationBusy === `delete-${channel.id}`} busyLabel="Deleting" idleIcon={<Trash2 size={14} />} />
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
                        <ButtonProgress busy={notificationBusy === `retry-${delivery.id}`} busyLabel="Retrying" idleIcon={<RotateCcw size={14} />} />
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
        <ButtonProgress busy={busy} busyLabel="Saving settings" idleIcon={<Save size={16} />}>
          Save settings
        </ButtonProgress>
      </Button>
    </div>
  );
}
