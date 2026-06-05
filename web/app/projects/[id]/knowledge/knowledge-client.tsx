"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Check, ExternalLink, Pencil, RefreshCw, Save, Wand2, X } from "lucide-react";
import { CrawlSummary, GenerationRun, InventoryItem, ProductProfile } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, EmptyState, Field, Notice, SectionHeader, TextArea, TextInput, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type InventoryDraft = {
  title: string;
  target_keyword: string;
  summary: string;
  topics: string;
  evidence_snippets: string;
};

function lines(value: string) {
  return value
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

function inventoryDraft(item: InventoryItem): InventoryDraft {
  return {
    title: item.title ?? "",
    target_keyword: item.target_keyword ?? "",
    summary: item.summary ?? "",
    topics: item.topics.map(String).join("\n"),
    evidence_snippets: item.evidence_snippets.map(String).join("\n"),
  };
}

function latestCrawlSummary(runs: GenerationRun[]): CrawlSummary | null {
  for (const run of runs) {
    const summary = run.output?.crawl_summary;
    if (summary && typeof summary === "object") return summary as CrawlSummary;
  }
  return null;
}

export function KnowledgeClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [landing, setLanding] = useState("");
  const [profile, setProfile] = useState<ProductProfile | null>(null);
  const [inventory, setInventory] = useState<InventoryItem[]>([]);
  const [crawlSummary, setCrawlSummary] = useState<CrawlSummary | null>(null);
  const [editingItemId, setEditingItemId] = useState<string | null>(null);
  const [itemDraft, setItemDraft] = useState<InventoryDraft | null>(null);
  const [profileDraft, setProfileDraft] = useState("{}");
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    setMessage(null);
    try {
      const [p, items, runs] = await Promise.all([
        api.getProfile(projectId).catch(() => null),
        api.listInventory(projectId).catch(() => []),
        api.listRuns(projectId, { agent: "insight", status: "ok", limit: 100 }).catch(() => []),
      ]);
      setProfile(p);
      setInventory(items);
      setCrawlSummary(latestCrawlSummary(runs));
      setProfileDraft(JSON.stringify(p?.profile ?? {}, null, 2));
    } catch (e: any) {
      setMessage({ title: "Knowledge data unavailable", detail: e.message, tone: "amber" });
    }
  }, [api, projectId]);

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
      if (result.crawl_summary) setCrawlSummary(result.crawl_summary);
      setMessage({
        title: "Insight completed",
        detail: `Inventory count: ${result.inventory_count}. ${result.crawl_summary?.truncated ? "Crawl was truncated." : "Crawl completed within configured bounds."}`,
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

  function startInventoryEdit(item: InventoryItem) {
    setEditingItemId(item.id);
    setItemDraft(inventoryDraft(item));
    setMessage(null);
  }

  function cancelInventoryEdit() {
    setEditingItemId(null);
    setItemDraft(null);
  }

  async function saveInventory(item: InventoryItem) {
    if (!itemDraft) return;
    setBusy(`inventory-${item.id}`);
    setMessage(null);
    try {
      const updated = await api.updateInventory(projectId, item.id, {
        title: itemDraft.title,
        target_keyword: itemDraft.target_keyword,
        topics: lines(itemDraft.topics),
        summary: itemDraft.summary,
        evidence_snippets: lines(itemDraft.evidence_snippets),
      });
      setInventory((current) => current.map((entry) => (entry.id === updated.id ? updated : entry)));
      cancelInventoryEdit();
      setMessage({ title: "Inventory saved", detail: updated.title ?? updated.url, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Inventory save failed", detail: e.message, tone: "red" });
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
        {!crawlSummary ? (
          <EmptyState title="No crawl summary" detail="Run Insight to capture crawl bounds, skipped pages, and fetched inventory." />
        ) : (
          <div className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4">
            <div className="grid gap-2 sm:grid-cols-4">
              <div className="rounded-lg bg-slate-50 px-3 py-2">
                <div className="text-xs font-semibold text-slate-500">Discovered</div>
                <div className="text-lg font-bold text-slate-900">{crawlSummary.discovered_count ?? 0}</div>
              </div>
              <div className="rounded-lg bg-slate-50 px-3 py-2">
                <div className="text-xs font-semibold text-slate-500">Fetched</div>
                <div className="text-lg font-bold text-slate-900">{crawlSummary.fetched_count ?? 0}</div>
              </div>
              <div className="rounded-lg bg-slate-50 px-3 py-2">
                <div className="text-xs font-semibold text-slate-500">Inventory</div>
                <div className="text-lg font-bold text-slate-900">{crawlSummary.inventory_count ?? 0}</div>
              </div>
              <div className="rounded-lg bg-slate-50 px-3 py-2">
                <div className="text-xs font-semibold text-slate-500">Errors</div>
                <div className="text-lg font-bold text-slate-900">{crawlSummary.errors?.length ?? 0}</div>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <Badge tone={crawlSummary.truncated ? "amber" : "green"}>{crawlSummary.truncated ? "truncated" : "complete"}</Badge>
              {crawlSummary.landing_url && <span className="truncate font-semibold text-slate-500">{crawlSummary.landing_url}</span>}
            </div>
            {(crawlSummary.sample_urls?.length ?? 0) > 0 && (
              <div className="grid gap-1 text-sm">
                <div className="font-semibold text-slate-700">Fetched URLs</div>
                {crawlSummary.sample_urls?.slice(0, 6).map((url) => (
                  <a key={url} href={url} target="_blank" rel="noopener noreferrer" className="truncate text-slate-500 hover:text-[#d93820]">
                    {url}
                  </a>
                ))}
              </div>
            )}
            {(crawlSummary.errors?.length ?? 0) > 0 && (
              <div className="grid gap-1 text-sm">
                <div className="font-semibold text-red-700">Skipped pages</div>
                {crawlSummary.errors?.slice(0, 4).map((error) => (
                  <div key={error} className="rounded-lg bg-red-50 px-3 py-2 text-red-800">
                    {error}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
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
                  <div className="flex shrink-0 items-center gap-2">
                    <Badge tone={item.source === "generated" ? "green" : "blue"}>{item.source}</Badge>
                    <Button disabled={!!busy} size="sm" variant="ghost" onClick={() => startInventoryEdit(item)}>
                      <Pencil size={14} />
                      Edit
                    </Button>
                  </div>
                </div>
                <p className="mt-2 text-sm leading-5 text-slate-600">{item.summary || "No summary captured."}</p>
                <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-slate-500">
                  <span>{item.target_keyword || "No keyword"}</span>
                  <span>{item.evidence_snippets.length} evidence snippets</span>
                  <a href={item.url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 font-semibold text-[#d93820]">
                    Open source <ExternalLink size={12} />
                  </a>
                </div>
                {item.evidence_snippets.length > 0 && (
                  <div className="mt-3 grid gap-1">
                    {item.evidence_snippets.slice(0, 4).map((snippet) => (
                      <div key={String(snippet)} className="rounded-lg bg-slate-50 px-3 py-2 text-sm leading-5 text-slate-600">
                        {String(snippet)}
                      </div>
                    ))}
                  </div>
                )}
                {editingItemId === item.id && itemDraft && (
                  <div className="mt-4 grid gap-3 border-t border-slate-100 pt-4">
                    <div className="grid gap-3 md:grid-cols-2">
                      <Field label="Title">
                        <TextInput value={itemDraft.title} onChange={(event) => setItemDraft({ ...itemDraft, title: event.target.value })} />
                      </Field>
                      <Field label="Target keyword">
                        <TextInput
                          value={itemDraft.target_keyword}
                          onChange={(event) => setItemDraft({ ...itemDraft, target_keyword: event.target.value })}
                        />
                      </Field>
                    </div>
                    <Field label="Summary">
                      <TextArea rows={3} value={itemDraft.summary} onChange={(event) => setItemDraft({ ...itemDraft, summary: event.target.value })} />
                    </Field>
                    <div className="grid gap-3 md:grid-cols-2">
                      <Field label="Topics">
                        <TextArea rows={4} value={itemDraft.topics} onChange={(event) => setItemDraft({ ...itemDraft, topics: event.target.value })} />
                      </Field>
                      <Field label="Evidence snippets">
                        <TextArea
                          rows={4}
                          value={itemDraft.evidence_snippets}
                          onChange={(event) => setItemDraft({ ...itemDraft, evidence_snippets: event.target.value })}
                        />
                      </Field>
                    </div>
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button disabled={!!busy} size="sm" variant="ghost" onClick={cancelInventoryEdit}>
                        <X size={14} />
                        Cancel
                      </Button>
                      <Button disabled={!!busy} size="sm" variant="primary" onClick={() => saveInventory(item)}>
                        <Check size={14} />
                        Save
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
