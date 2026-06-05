"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Archive, CalendarDays, RefreshCw, Wand2 } from "lucide-react";
import { api, Topic } from "../../../lib/api";
import { Badge, Button, EmptyState, Notice, SectionHeader, TextInput, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

export function TopicsClient({ projectId }: { projectId: string }) {
  const [topics, setTopics] = useState<Topic[]>([]);
  const [query, setQuery] = useState("");
  const [channel, setChannel] = useState("all");
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    try {
      setTopics(await api.listTopics(projectId));
    } catch (e: any) {
      setMessage({ title: "Topics unavailable", detail: e.message, tone: "amber" });
    }
  }, [projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return topics.filter((topic) => {
      const channelMatch = channel === "all" || topic.channel === channel;
      const queryMatch =
        !q ||
        [topic.title, topic.target_keyword, topic.target_prompt, topic.angle, topic.format].some((value) =>
          value?.toLowerCase().includes(q),
        );
      return channelMatch && queryMatch;
    });
  }, [channel, query, topics]);

  async function runStrategist() {
    setBusy("strategist");
    setMessage(null);
    try {
      const next = await api.runStrategist(projectId);
      setTopics(next);
      setMessage({ title: "Strategist completed", detail: `${next.length} topics returned.`, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Strategist failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function generate(topic: Topic) {
    setBusy(topic.id);
    setMessage(null);
    try {
      const articles = await api.generateTopic(projectId, topic.id);
      await refresh();
      setMessage({ title: "Topic generated", detail: `${articles.length} articles moved toward review.`, tone: "green" });
    } catch (e: any) {
      const isDuplicate = String(e.message).includes("duplicate") || String(e.message).includes("unique");
      setMessage({
        title: isDuplicate ? "Topic already has generated articles" : "Generate failed",
        detail: isDuplicate
          ? "The backend currently exposes duplicate generation as a raw database error. It should return existing articles or a friendly 409."
          : e.message,
        tone: isDuplicate ? "amber" : "red",
      });
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-7">
      <section>
        <SectionHeader
          title="Topics"
          eyebrow="Backlog and schedule intent"
          action={
            <Button disabled={!!busy} variant="primary" onClick={runStrategist}>
              <Wand2 size={16} />
              Run Strategist
            </Button>
          }
        />
        {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}
      </section>

      <section className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4">
        <div className="grid gap-2 md:grid-cols-[1fr_auto_auto]">
          <TextInput value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search topics" />
          <select
            value={channel}
            onChange={(event) => setChannel(event.target.value)}
            className="h-10 rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700"
          >
            <option value="all">All channels</option>
            <option value="blog">Blog</option>
            <option value="syndication">Syndication</option>
            <option value="both">Both</option>
          </select>
          <Button disabled={!!busy} onClick={refresh}>
            <RefreshCw size={16} />
            Refresh
          </Button>
        </div>
      </section>

      <section>
        <SectionHeader title="Backlog" action={<Badge tone="neutral">{filtered.length}</Badge>} />
        {filtered.length === 0 ? (
          <EmptyState title="No topics found" detail="Run Strategist or adjust filters to populate the backlog." />
        ) : (
          <div className="grid gap-2">
            {filtered.map((topic) => (
              <div key={topic.id} className="rounded-xl border border-slate-200 bg-white px-4 py-3">
                <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone="blue">{topic.channel}</Badge>
                      <Badge tone={topic.status === "backlog" ? "neutral" : "green"}>{topic.status}</Badge>
                      <span className="text-xs font-semibold text-slate-400">priority {topic.priority}</span>
                    </div>
                    <div className="mt-2 text-base font-bold text-slate-900">{topic.title}</div>
                    <div className="mt-1 text-sm text-slate-500">
                      {topic.target_keyword || topic.target_prompt || "No target keyword or prompt captured."}
                    </div>
                    <div className="mt-2 flex flex-wrap gap-3 text-xs font-semibold text-slate-400">
                      <span>{topic.format || "No format"}</span>
                      <span>{topic.angle || "No angle"}</span>
                      <span>{topic.internal_links.length} internal links</span>
                      <span>scheduled {formatDate(topic.scheduled_at)}</span>
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap gap-2">
                    <Button disabled={!!busy} size="sm" variant="primary" onClick={() => generate(topic)}>
                      <Wand2 size={14} />
                      Generate
                    </Button>
                    <Button disabled size="sm" title="Topic schedule route is not available yet.">
                      <CalendarDays size={14} />
                      Schedule
                    </Button>
                    <Button disabled size="sm" title="Topic archive route is not available yet.">
                      <Archive size={14} />
                      Archive
                    </Button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <Notice
        title="Topic edit contract needed"
        detail="The backend has query support for status and scheduled_at, but no HTTP route for title/channel/priority edits or archive yet."
        tone="amber"
      />
    </div>
  );
}
