"use client";

import { useCallback, useEffect, useState } from "react";
import { Bell, Plus, RefreshCw, RotateCcw, Save, Send, Trash2 } from "lucide-react";
import {
  defaultProjectConfig,
  NotificationChannel,
  NotificationChannelKind,
  NotificationDelivery,
  NotificationEvent,
  NotificationSubscription,
  ProjectConfig,
} from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, Field, Notice, SectionHeader, TextInput, TextArea, cx, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

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
};

const deliveryStatuses = [
  { value: "", label: "All" },
  { value: "pending", label: "Pending" },
  { value: "dead", label: "Dead" },
  { value: "sent", label: "Sent" },
];

export function SettingsClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [config, setConfig] = useState<ProjectConfig>(defaultProjectConfig());
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
    } catch (e: any) {
      setMessage({ title: "Settings unavailable", detail: e.message, tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

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
      setMessage({ title: "Notifications unavailable", detail: e.message, tone: "amber" });
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
      await api.updateConfig(projectId, fullPayload);
      setConfig(fullPayload);
      setMessage({ title: "Settings saved", detail: "Full config payload was sent to avoid zeroing omitted fields.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Settings save failed", detail: e.message, tone: "red" });
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
      setMessage({ title: "Channel save failed", detail: e.message, tone: "red" });
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

  return (
    <div className="space-y-7">
      <SectionHeader title="Settings" eyebrow="Project config" />
      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      <Notice
        title="Config update is full-payload"
        detail="The current backend PUT /config replaces the entire config. This form always submits a complete payload and validates numeric fields through controlled inputs."
        tone="amber"
      />

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
              <Plus size={16} />
              Add
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
                            <Send size={14} />
                          </Button>
                          <Button
                            size="sm"
                            variant="danger"
                            onClick={() => deleteChannel(channel.id)}
                            disabled={notificationBusy === `delete-${channel.id}`}
                            title="Delete channel"
                          >
                            <Trash2 size={14} />
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
