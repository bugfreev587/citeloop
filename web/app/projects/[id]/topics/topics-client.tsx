"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Archive, CalendarDays, Check, Pencil, RefreshCw, Wand2, X } from "lucide-react";
import { Topic } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, EmptyState, Field, Notice, SectionHeader, TextArea, TextInput, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type TopicDraft = {
  channel: string;
  title: string;
  target_keyword: string;
  target_prompt: string;
  angle: string;
  format: string;
  priority: string;
};

function toDateTimeLocal(value: string | null) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function fromDateTimeLocal(value: string) {
  const trimmed = value.trim();
  return trimmed ? new Date(trimmed).toISOString() : null;
}

function draftFromTopic(topic: Topic): TopicDraft {
  return {
    channel: topic.channel,
    title: topic.title,
    target_keyword: topic.target_keyword ?? "",
    target_prompt: topic.target_prompt ?? "",
    angle: topic.angle ?? "",
    format: topic.format ?? "",
    priority: String(topic.priority),
  };
}

export function TopicsClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [topics, setTopics] = useState<Topic[]>([]);
  const [scheduleDrafts, setScheduleDrafts] = useState<Record<string, string>>({});
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draft, setDraft] = useState<TopicDraft | null>(null);
  const [query, setQuery] = useState("");
  const [channel, setChannel] = useState("all");
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    try {
      const next = await api.listTopics(projectId);
      setTopics(next);
      setScheduleDrafts(Object.fromEntries(next.map((topic) => [topic.id, toDateTimeLocal(topic.scheduled_at)])));
    } catch (e: any) {
      setMessage({ title: "Topics unavailable", detail: e.message, tone: "amber" });
    }
  }, [api, projectId]);

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

  function replaceTopic(updated: Topic) {
    setTopics((current) => current.map((topic) => (topic.id === updated.id ? updated : topic)));
    setScheduleDrafts((current) => ({ ...current, [updated.id]: toDateTimeLocal(updated.scheduled_at) }));
  }

  function startEdit(topic: Topic) {
    setEditingId(topic.id);
    setDraft(draftFromTopic(topic));
    setMessage(null);
  }

  function cancelEdit() {
    setEditingId(null);
    setDraft(null);
  }

  async function saveEdit(topic: Topic) {
    if (!draft) return;
    const priority = Number.parseInt(draft.priority, 10);
    setBusy(`edit-${topic.id}`);
    setMessage(null);
    try {
      const updated = await api.updateTopic(projectId, topic.id, {
        channel: draft.channel,
        title: draft.title,
        target_keyword: draft.target_keyword,
        target_prompt: draft.target_prompt,
        angle: draft.angle,
        format: draft.format,
        priority: Number.isFinite(priority) ? priority : 0,
      });
      replaceTopic(updated);
      cancelEdit();
      setMessage({ title: "Topic saved", detail: updated.title, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Save failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function schedule(topic: Topic) {
    setBusy(`schedule-${topic.id}`);
    setMessage(null);
    try {
      const updated = await api.scheduleTopic(projectId, topic.id, fromDateTimeLocal(scheduleDrafts[topic.id] ?? ""));
      replaceTopic(updated);
      setMessage({ title: updated.scheduled_at ? "Topic scheduled" : "Schedule cleared", detail: updated.title, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Schedule failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function archive(topic: Topic) {
    setBusy(`archive-${topic.id}`);
    setMessage(null);
    try {
      const updated = await api.archiveTopic(projectId, topic.id);
      replaceTopic(updated);
      if (editingId === topic.id) cancelEdit();
      setMessage({ title: "Topic archived", detail: updated.title, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Archive failed", detail: e.message, tone: "red" });
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
      setMessage({
        title: "Generate failed",
        detail: e.message,
        tone: "red",
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
                      <Badge tone={topic.status === "archived" ? "amber" : topic.status === "backlog" ? "neutral" : "green"}>
                        {topic.status}
                      </Badge>
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
                    <Button
                      disabled={!!busy || topic.status === "archived"}
                      size="sm"
                      variant="ghost"
                      onClick={() => startEdit(topic)}
                    >
                      <Pencil size={14} />
                      Edit
                    </Button>
                    <Button disabled={!!busy || topic.status === "archived"} size="sm" variant="primary" onClick={() => generate(topic)}>
                      <Wand2 size={14} />
                      Generate
                    </Button>
                    <Button disabled={!!busy || topic.status === "archived"} size="sm" variant="danger" onClick={() => archive(topic)}>
                      <Archive size={14} />
                      Archive
                    </Button>
                  </div>
                </div>
                {editingId === topic.id && draft && (
                  <div className="mt-4 grid gap-3 border-t border-slate-100 pt-4">
                    <div className="grid gap-3 md:grid-cols-[160px_1fr_120px]">
                      <Field label="Channel">
                        <select
                          value={draft.channel}
                          onChange={(event) => setDraft({ ...draft, channel: event.target.value })}
                          className="h-10 rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700"
                        >
                          <option value="blog">Blog</option>
                          <option value="syndication">Syndication</option>
                          <option value="both">Both</option>
                        </select>
                      </Field>
                      <Field label="Title">
                        <TextInput value={draft.title} onChange={(event) => setDraft({ ...draft, title: event.target.value })} />
                      </Field>
                      <Field label="Priority">
                        <TextInput
                          type="number"
                          value={draft.priority}
                          onChange={(event) => setDraft({ ...draft, priority: event.target.value })}
                        />
                      </Field>
                    </div>
                    <div className="grid gap-3 md:grid-cols-2">
                      <Field label="Target keyword">
                        <TextInput
                          value={draft.target_keyword}
                          onChange={(event) => setDraft({ ...draft, target_keyword: event.target.value })}
                        />
                      </Field>
                      <Field label="Format">
                        <TextInput value={draft.format} onChange={(event) => setDraft({ ...draft, format: event.target.value })} />
                      </Field>
                    </div>
                    <div className="grid gap-3 md:grid-cols-2">
                      <Field label="Angle">
                        <TextArea rows={3} value={draft.angle} onChange={(event) => setDraft({ ...draft, angle: event.target.value })} />
                      </Field>
                      <Field label="Target prompt">
                        <TextArea
                          rows={3}
                          value={draft.target_prompt}
                          onChange={(event) => setDraft({ ...draft, target_prompt: event.target.value })}
                        />
                      </Field>
                    </div>
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button disabled={!!busy} size="sm" variant="ghost" onClick={cancelEdit}>
                        <X size={14} />
                        Cancel
                      </Button>
                      <Button disabled={!!busy || !draft.title.trim()} size="sm" variant="primary" onClick={() => saveEdit(topic)}>
                        <Check size={14} />
                        Save
                      </Button>
                    </div>
                  </div>
                )}
                <div className="mt-3 flex flex-col gap-2 border-t border-slate-100 pt-3 md:flex-row md:items-end md:justify-end">
                  <Field label="Scheduled at">
                    <TextInput
                      type="datetime-local"
                      value={scheduleDrafts[topic.id] ?? ""}
                      disabled={topic.status === "archived"}
                      onChange={(event) => setScheduleDrafts((current) => ({ ...current, [topic.id]: event.target.value }))}
                    />
                  </Field>
                  <Button disabled={!!busy || topic.status === "archived"} size="sm" onClick={() => schedule(topic)}>
                    <CalendarDays size={14} />
                    Schedule
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
