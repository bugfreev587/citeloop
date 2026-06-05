"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { ExternalLink, RefreshCw, Save, Wand2 } from "lucide-react";
import { api, InventoryItem, ProductProfile } from "../../../lib/api";
import { Badge, Button, EmptyState, Field, Notice, SectionHeader, TextArea, TextInput, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

export function KnowledgeClient({ projectId }: { projectId: string }) {
  const [landing, setLanding] = useState("");
  const [profile, setProfile] = useState<ProductProfile | null>(null);
  const [inventory, setInventory] = useState<InventoryItem[]>([]);
  const [profileDraft, setProfileDraft] = useState("{}");
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    setMessage(null);
    try {
      const [p, items] = await Promise.all([
        api.getProfile(projectId).catch(() => null),
        api.listInventory(projectId).catch(() => []),
      ]);
      setProfile(p);
      setInventory(items);
      setProfileDraft(JSON.stringify(p?.profile ?? {}, null, 2));
    } catch (e: any) {
      setMessage({ title: "Knowledge data unavailable", detail: e.message, tone: "amber" });
    }
  }, [projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return inventory;
    return inventory.filter((item) =>
      [item.title, item.url, item.target_keyword, item.summary].some((value) => value?.toLowerCase().includes(q)),
    );
  }, [inventory, query]);

  async function runInsight(event: FormEvent) {
    event.preventDefault();
    if (!landing.trim()) return;
    setBusy("insight");
    setMessage(null);
    try {
      const result = await api.runInsight(projectId, landing.trim());
      await refresh();
      setMessage({
        title: "Insight completed",
        detail: `Inventory count: ${result.inventory_count}. Crawl summary is not returned by the current backend yet.`,
        tone: "green",
      });
    } catch (e: any) {
      setMessage({ title: "Insight failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function saveProfile() {
    setBusy("profile");
    setMessage(null);
    try {
      const parsed = JSON.parse(profileDraft);
      const updated = await api.updateProfile(projectId, {
        profile: parsed,
        source_urls: profile?.source_urls ?? [],
      });
      setProfile(updated);
      setMessage({ title: "Profile saved", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Profile save failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-7">
      <section>
        <SectionHeader title="Knowledge" eyebrow="Insight output" />
        <form onSubmit={runInsight} className="grid gap-2 rounded-xl border border-slate-200 bg-white p-4 md:grid-cols-[1fr_auto]">
          <TextInput
            value={landing}
            onChange={(event) => setLanding(event.target.value)}
            placeholder="https://landing-page-url/"
          />
          <Button disabled={busy === "insight" || !landing.trim()} variant="primary" type="submit">
            <Wand2 size={16} />
            Run Insight
          </Button>
        </form>
      </section>

      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      <section>
        <SectionHeader
          title="Product profile"
          action={
            <Button disabled={!!busy} size="sm" onClick={refresh}>
              <RefreshCw size={14} />
              Refresh
            </Button>
          }
        />
        {!profile ? (
          <EmptyState title="No active profile" detail="Run Insight to create product understanding for this project." />
        ) : (
          <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone="green">active v{profile.version}</Badge>
              <span className="text-sm text-slate-500">Updated {formatDate(profile.updated_at)}</span>
            </div>
            <Field label="Profile JSON" helper="Saving profile fields does not change inventory evidence snippets.">
              <TextArea
                value={profileDraft}
                onChange={(event) => setProfileDraft(event.target.value)}
                className="min-h-[320px] font-mono text-xs"
              />
            </Field>
            <Button disabled={!!busy} variant="primary" className="w-fit" onClick={saveProfile}>
              <Save size={16} />
              Save profile
            </Button>
          </div>
        )}
      </section>

      <section>
        <SectionHeader title="Crawl summary" />
        <Notice
          title="Crawl summary contract needed"
          detail="POST /insight currently returns profile and inventory_count only. Sitemap, skipped pages, errors, and truncated state need to be returned or persisted before this panel can show real data."
          tone="amber"
        />
      </section>

      <section>
        <SectionHeader title="Inventory" action={<Badge tone="neutral">{inventory.length}</Badge>} />
        <div className="mb-3">
          <TextInput value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search title, URL, keyword" />
        </div>
        {filtered.length === 0 ? (
          <EmptyState title="No inventory items" detail="Inventory appears after Insight captures existing or generated content." />
        ) : (
          <div className="grid gap-2">
            {filtered.map((item) => (
              <div key={item.id} className="rounded-lg border border-slate-200 bg-white px-4 py-3">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="truncate text-sm font-bold text-slate-900">{item.title || item.url}</div>
                    <div className="mt-1 truncate text-xs text-slate-500">{item.url}</div>
                  </div>
                  <Badge tone={item.source === "generated" ? "green" : "blue"}>{item.source}</Badge>
                </div>
                <p className="mt-2 text-sm leading-5 text-slate-600">{item.summary || "No summary captured."}</p>
                <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-slate-500">
                  <span>{item.target_keyword || "No keyword"}</span>
                  <span>{item.evidence_snippets.length} evidence snippets</span>
                  <a href={item.url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 font-semibold text-[#d93820]">
                    Open source <ExternalLink size={12} />
                  </a>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
