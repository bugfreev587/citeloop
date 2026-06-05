"use client";

import { useCallback, useEffect, useState } from "react";
import { Save } from "lucide-react";
import { api, defaultProjectConfig, ProjectConfig } from "../../../lib/api";
import { Button, Field, Notice, SectionHeader, TextInput, TextArea } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

function toInt(value: string, fallback: number) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function toFloat(value: string, fallback: number) {
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

export function SettingsClient({ projectId }: { projectId: string }) {
  const [config, setConfig] = useState<ProjectConfig>(defaultProjectConfig());
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    try {
      const project = await api.getProject(projectId);
      setConfig(project.config);
    } catch (e: any) {
      setMessage({ title: "Settings unavailable", detail: e.message, tone: "amber" });
    }
  }, [projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

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

      <Button disabled={busy} variant="primary" onClick={save}>
        <Save size={16} />
        Save settings
      </Button>
    </div>
  );
}
