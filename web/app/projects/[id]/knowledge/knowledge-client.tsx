"use client";

import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, ExternalLink, Loader2, Pencil, RefreshCw, Save, ShieldCheck, Wand2, X } from "lucide-react";
import { CrawlSummary, GenerationRun, InventoryItem, ProductProfile } from "../../../lib/api";
import { ProfileDraft, lines, profilePayloadFromAdvancedJSON, profilePayloadFromDraft } from "../../../lib/dashboard-ux-logic";
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

function readPath(source: Record<string, any>, path: string) {
  return path.split(".").reduce<any>((value, key) => (value && typeof value === "object" ? value[key] : undefined), source);
}

function asText(value: any) {
  if (typeof value === "string") return value;
  if (Array.isArray(value)) return value.map(String).join("\n");
  if (value == null) return "";
  return String(value);
}

function firstProfileValue(profile: Record<string, any>, paths: string[]) {
  for (const path of paths) {
    const value = readPath(profile, path);
    const text = asText(value).trim();
    if (text) return text;
  }
  return "";
}

function profileDraftFrom(profile: ProductProfile | null): ProfileDraft {
  const data = profile?.profile ?? {};
  return {
    positioning: firstProfileValue(data, ["positioning", "one_liner", "description", "summary", "about"]),
    icp: firstProfileValue(data, ["icp", "audience", "target_audience", "personas"]),
    value_props: firstProfileValue(data, ["value_props", "value_propositions", "benefits"]),
    features: firstProfileValue(data, ["features", "product_features"]),
    differentiators: firstProfileValue(data, ["differentiators", "differentiation"]),
    competitors: firstProfileValue(data, ["competitors", "alternatives"]),
    key_terms: firstProfileValue(data, ["key_terms", "keywords", "terms"]),
    tone: firstProfileValue(data, ["tone", "brand_voice", "voice.tone"]),
    banned_claims: firstProfileValue(data, ["banned_claims", "risky_claims", "guardrails.banned_claims"]),
    content_rules: firstProfileValue(data, ["content_rules", "rules", "style_rules", "voice.rules"]),
    advancedJSON: JSON.stringify(data, null, 2),
  };
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

export function ContextClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [landing, setLanding] = useState("");
  const [profile, setProfile] = useState<ProductProfile | null>(null);
  const [inventory, setInventory] = useState<InventoryItem[]>([]);
  const [crawlSummary, setCrawlSummary] = useState<CrawlSummary | null>(null);
  const [editingItemId, setEditingItemId] = useState<string | null>(null);
  const [itemDraft, setItemDraft] = useState<InventoryDraft | null>(null);
  const [profileDraft, setProfileDraft] = useState<ProfileDraft>(() => profileDraftFrom(null));
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);
  const [backgroundCrawl, setBackgroundCrawl] = useState(false);
  const bgBaselineRef = useRef(0);
  const bgAttemptsRef = useRef(0);

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
      setProfileDraft(profileDraftFrom(p));
    } catch (e: any) {
      setMessage({ title: "Context data unavailable", detail: e.message, tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // The full public crawl + evidence inventory runs in a detached background job after a
  // landing-only "quick profile". Poll until new source pages land, then refresh and notify,
  // so the user never has to guess whether the background work finished.
  useEffect(() => {
    if (!backgroundCrawl) return;
    let cancelled = false;
    const interval = window.setInterval(async () => {
      bgAttemptsRef.current += 1;
      try {
        const items = await api.listInventory(projectId);
        if (cancelled) return;
        if (items.length > bgBaselineRef.current) {
          setInventory(items);
          await refresh();
          if (cancelled) return;
          setBackgroundCrawl(false);
          setMessage({
            tone: "green",
            title: "Context updated",
            detail: `Background crawl finished — ${items.length} source page${items.length === 1 ? "" : "s"} and their evidence are ready.`,
          });
          return;
        }
      } catch {
        // transient failure; keep polling until the attempt cap
      }
      if (bgAttemptsRef.current >= 18) {
        // ~2.5 min cap: stop quietly so a slow/blocked crawl never polls forever.
        if (!cancelled) setBackgroundCrawl(false);
      }
    }, 8000);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, [backgroundCrawl, api, projectId, refresh]);

  const evidenceRows = useMemo(
    () =>
      inventory.flatMap((item) =>
        item.evidence_snippets.map((snippet, index) => ({
          id: `${item.id}-${index}`,
          item,
          claim: item.title || item.target_keyword || item.url,
          snippet: String(snippet),
        })),
      ),
    [inventory],
  );

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return inventory;
    return inventory.filter((item) =>
      [item.title, item.url, item.target_keyword, item.summary].some((value) => value?.toLowerCase().includes(q)),
    );
  }, [inventory, query]);

  const contextConfirmed = Boolean(profile?.profile?.context_confirmed_at || profile?.profile?.confirmed_at);
  const sourcePageCount = Math.max(inventory.length, profile?.source_urls?.length ?? 0);
  const contextStatus = !profile
    ? { label: "Needs setup", tone: "amber" as const, detail: "Connect a domain so CiteLoop can read public pages and build usable context." }
    : !contextConfirmed
      ? { label: "Needs confirmation", tone: "amber" as const, detail: "Review the extracted positioning, audience, evidence, and guardrails before relying on new drafts." }
      : evidenceRows.length === 0
        ? { label: "Needs evidence", tone: "amber" as const, detail: "Context exists, but safe claims still need source-backed evidence snippets." }
        : { label: "Ready", tone: "green" as const, detail: "CiteLoop has confirmed context and source-backed evidence for planning and review." };

  async function refreshContext(event: FormEvent) {
    event.preventDefault();
    if (!landing.trim()) return;
    setBusy("context");
    setMessage(null);
    try {
      const result = await api.runInsight(projectId, landing.trim());
      bgBaselineRef.current = inventory.length;
      bgAttemptsRef.current = 0;
      setBackgroundCrawl(Boolean(result.background_crawl));
      await refresh();
      if (result.crawl_summary && !result.background_crawl) setCrawlSummary(result.crawl_summary);
      setMessage({
        title: "Context refreshed",
        detail: result.background_crawl
          ? "Product profile ready. The full public crawl and evidence inventory are running in the background — this page updates automatically when they finish."
          : `Captured ${result.inventory_count} source items. ${result.crawl_summary?.truncated ? "Some pages were skipped by crawl bounds." : "Source scan completed within configured bounds."}`,
        tone: "green",
      });
    } catch (e: any) {
      setMessage({ title: "Context refresh failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function saveProfile(nextProfile?: Record<string, any>, success = "Context saved") {
    setBusy("profile");
    setMessage(null);
    try {
      const payload = nextProfile ?? profilePayloadFromDraft(profileDraft, profile?.profile ?? {});
      const updated = await api.updateProfile(projectId, {
        profile: payload,
        source_urls: profile?.source_urls ?? [],
      });
      setProfile(updated);
      setProfileDraft(profileDraftFrom(updated));
      setMessage({ title: success, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Context save failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function saveAdvancedProfile() {
    setBusy("profile");
    setMessage(null);
    try {
      const payload = profilePayloadFromAdvancedJSON(profileDraft.advancedJSON);
      const updated = await api.updateProfile(projectId, {
        profile: payload,
        source_urls: profile?.source_urls ?? [],
      });
      setProfile(updated);
      setProfileDraft(profileDraftFrom(updated));
      setMessage({ title: "Advanced context saved", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Advanced JSON save failed", detail: e.message, tone: "red" });
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
      setMessage({ title: "Source page saved", detail: updated.title ?? updated.url, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Source page save failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  function confirmContext() {
    saveProfile(
      {
        ...profilePayloadFromDraft(profileDraft, profile?.profile ?? {}),
        context_confirmed_at: new Date().toISOString(),
      },
      "Context confirmed",
    );
  }

  return (
    <div className="space-y-7">
      <section>
        <SectionHeader
          title="Context"
          eyebrow="Domain cognition"
          action={
            <Button disabled={!!busy} size="sm" onClick={refresh}>
              <RefreshCw size={14} />
              Refresh
            </Button>
          }
        />
        <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <Badge tone={contextStatus.tone}>{contextStatus.label}</Badge>
                {profile && <span className="text-sm font-semibold text-slate-500">Updated {formatDate(profile.updated_at)}</span>}
              </div>
              <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-600">
                This is how CiteLoop understands your domain and writes from source-backed evidence.
              </p>
            </div>
            <form onSubmit={refreshContext} className="grid gap-2 md:grid-cols-[minmax(220px,1fr)_auto]">
              <TextInput value={landing} onChange={(event) => setLanding(event.target.value)} placeholder="https://product-domain.com" />
              <Button disabled={busy === "context" || !landing.trim()} variant="primary" type="submit">
                <Wand2 size={16} />
                Refresh context
              </Button>
            </form>
          </div>

          <div className="grid gap-2 sm:grid-cols-4">
            <div className="rounded-lg bg-slate-50 px-3 py-2">
              <div className="text-xs font-bold uppercase text-slate-400">Source pages</div>
              <div className="mt-1 text-lg font-bold text-slate-900">{sourcePageCount}</div>
            </div>
            <div className="rounded-lg bg-slate-50 px-3 py-2">
              <div className="text-xs font-bold uppercase text-slate-400">Evidence</div>
              <div className="mt-1 text-lg font-bold text-slate-900">{evidenceRows.length}</div>
            </div>
            <div className="rounded-lg bg-slate-50 px-3 py-2">
              <div className="text-xs font-bold uppercase text-slate-400">Crawl warnings</div>
              <div className="mt-1 text-lg font-bold text-slate-900">{crawlSummary?.errors?.length ?? 0}</div>
            </div>
            <div className="rounded-lg bg-slate-50 px-3 py-2">
              <div className="text-xs font-bold uppercase text-slate-400">Last refreshed</div>
              <div className="mt-1 truncate text-sm font-bold text-slate-900">{formatDate(profile?.updated_at ?? null)}</div>
            </div>
          </div>
          <Notice title={contextStatus.label} detail={contextStatus.detail} tone={contextStatus.tone === "green" ? "green" : "amber"} />
        </div>
      </section>

      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      {backgroundCrawl && (
        <div className="flex items-center gap-2 rounded-xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm font-semibold text-blue-900">
          <Loader2 className="animate-spin" size={16} />
          Reading the rest of your site — source pages and evidence will appear here automatically.
        </div>
      )}

      {profile && !contextConfirmed && (
        <section>
          <SectionHeader title="First-run Context confirmation" eyebrow="Review before generating" />
          <div className="grid gap-4 rounded-xl border border-amber-200 bg-amber-50 p-4">
            <div className="flex gap-3">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-white text-amber-700">
                <ShieldCheck size={18} />
              </div>
              <div>
                <div className="font-bold text-amber-950">We read your domain. Confirm what CiteLoop should use.</div>
                <p className="mt-1 text-sm leading-5 text-amber-900">
                  Check positioning, ICP, evidence-backed claims, competitors, and banned claims. You can edit fields below before confirming.
                </p>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button disabled={!!busy} variant="primary" onClick={confirmContext}>
                <Check size={16} />
                Confirm context
              </Button>
              <a href="#domain-profile" className="inline-flex h-10 items-center rounded-xl border border-amber-200 bg-white px-3 text-sm font-medium text-amber-900 hover:bg-amber-100">
                Edit fields below
              </a>
            </div>
          </div>
        </section>
      )}

      <section>
        <SectionHeader title="Evidence library" action={<Badge tone={evidenceRows.length ? "green" : "amber"}>{evidenceRows.length}</Badge>} />
        {evidenceRows.length === 0 ? (
          <EmptyState title="No evidence snippets yet" detail="Refresh context or add evidence to source pages so claims can be safely reused in drafts." />
        ) : (
          <div className="grid gap-3">
            {evidenceRows.slice(0, 8).map((row) => (
              <div key={row.id} className="rounded-xl border border-slate-200 bg-white px-4 py-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="font-semibold text-slate-900">{row.claim}</div>
                  <Badge tone="green">Safe to use</Badge>
                </div>
                <p className="mt-2 text-sm leading-5 text-slate-600">{row.snippet}</p>
                <div className="mt-3 flex flex-wrap items-center gap-3 text-xs font-semibold text-slate-500">
                  <a href={row.item.url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-[#d93820]">
                    Source page <ExternalLink size={12} />
                  </a>
                  <button type="button" onClick={() => startInventoryEdit(row.item)} className="text-slate-600 hover:text-slate-950">
                    Edit evidence
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <section id="domain-profile">
        <SectionHeader title="Domain profile" />
        {!profile ? (
          <EmptyState title="Start by connecting your domain" detail="CiteLoop will read public pages, extract product facts, and build the context used for planning and review." />
        ) : (
          <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
            <Field label="Positioning">
              <TextArea rows={3} value={profileDraft.positioning} onChange={(event) => setProfileDraft({ ...profileDraft, positioning: event.target.value })} />
            </Field>
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="ICP">
                <TextArea rows={5} value={profileDraft.icp} onChange={(event) => setProfileDraft({ ...profileDraft, icp: event.target.value })} />
              </Field>
              <Field label="Value props">
                <TextArea rows={5} value={profileDraft.value_props} onChange={(event) => setProfileDraft({ ...profileDraft, value_props: event.target.value })} />
              </Field>
              <Field label="Features">
                <TextArea rows={5} value={profileDraft.features} onChange={(event) => setProfileDraft({ ...profileDraft, features: event.target.value })} />
              </Field>
              <Field label="Differentiators">
                <TextArea rows={5} value={profileDraft.differentiators} onChange={(event) => setProfileDraft({ ...profileDraft, differentiators: event.target.value })} />
              </Field>
              <Field label="Competitors">
                <TextArea rows={4} value={profileDraft.competitors} onChange={(event) => setProfileDraft({ ...profileDraft, competitors: event.target.value })} />
              </Field>
              <Field label="Key terms">
                <TextArea rows={4} value={profileDraft.key_terms} onChange={(event) => setProfileDraft({ ...profileDraft, key_terms: event.target.value })} />
              </Field>
            </div>
            <Button disabled={!!busy} variant="primary" className="w-fit" onClick={() => saveProfile()}>
              <Save size={16} />
              Save Domain profile
            </Button>
          </div>
        )}
      </section>

      <section>
        <SectionHeader title="Voice & rules" />
        {!profile ? (
          <EmptyState title="No voice rules yet" detail="Refresh context first, then set tone and banned claims before generating content." />
        ) : (
          <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
            <Field label="Tone">
              <TextArea rows={3} value={profileDraft.tone} onChange={(event) => setProfileDraft({ ...profileDraft, tone: event.target.value })} />
            </Field>
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="Banned claims" helper="Claims CiteLoop should avoid generating. One claim per line.">
                <TextArea rows={6} value={profileDraft.banned_claims} onChange={(event) => setProfileDraft({ ...profileDraft, banned_claims: event.target.value })} />
              </Field>
              <Field label="Content rules" helper="Style instructions and reviewer rules. One rule per line.">
                <TextArea rows={6} value={profileDraft.content_rules} onChange={(event) => setProfileDraft({ ...profileDraft, content_rules: event.target.value })} />
              </Field>
            </div>
            <Button disabled={!!busy} variant="primary" className="w-fit" onClick={() => saveProfile()}>
              <Save size={16} />
              Save Voice & rules
            </Button>
          </div>
        )}
      </section>

      <section>
        <SectionHeader title="Source pages" action={<Badge tone="neutral">{inventory.length}</Badge>} />
        <div className="mb-3">
          <TextInput value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search title, URL, keyword" />
        </div>
        {filtered.length === 0 ? (
          <EmptyState title="No source pages yet" detail="Source pages appear after CiteLoop refreshes context from your domain." />
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
                {editingItemId === item.id && itemDraft && (
                  <div className="mt-4 grid gap-3 border-t border-slate-100 pt-4">
                    <div className="grid gap-3 md:grid-cols-2">
                      <Field label="Title">
                        <TextInput value={itemDraft.title} onChange={(event) => setItemDraft({ ...itemDraft, title: event.target.value })} />
                      </Field>
                      <Field label="Target keyword">
                        <TextInput value={itemDraft.target_keyword} onChange={(event) => setItemDraft({ ...itemDraft, target_keyword: event.target.value })} />
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
                        <TextArea rows={4} value={itemDraft.evidence_snippets} onChange={(event) => setItemDraft({ ...itemDraft, evidence_snippets: event.target.value })} />
                      </Field>
                    </div>
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button disabled={!!busy} size="sm" variant="ghost" onClick={cancelInventoryEdit}>
                        <X size={14} />
                        Cancel
                      </Button>
                      <Button disabled={!!busy} size="sm" variant="primary" onClick={() => saveInventory(item)}>
                        <Check size={14} />
                        Save source page
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </section>

      {profile && (
        <section>
          <details className="rounded-xl border border-slate-200 bg-white p-4">
            <summary className="cursor-pointer text-sm font-bold text-slate-900">Advanced JSON</summary>
            <div className="mt-4 grid gap-3">
              <TextArea
                value={profileDraft.advancedJSON}
                onChange={(event) => setProfileDraft({ ...profileDraft, advancedJSON: event.target.value })}
                className="min-h-[240px] font-mono text-xs"
              />
              <Button disabled={!!busy} variant="outline" className="w-fit" onClick={saveAdvancedProfile}>
                <Save size={16} />
                Save advanced context
              </Button>
            </div>
          </details>
        </section>
      )}
    </div>
  );
}
