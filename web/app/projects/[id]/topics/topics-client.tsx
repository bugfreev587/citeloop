"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Archive, CalendarDays, Check, Loader2, Pencil, RefreshCw, Wand2, X } from "lucide-react";
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

function isBacklogStatus(status: string) {
  return status === "backlog" || status === "scheduled" || status === "generating";
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
  const [generatingIds, setGeneratingIds] = useState<Record<string, boolean>>({});
  const [message, setMessage] = useState<Message>(null);
  const strategistRunning = busy === "strategist";

  const refresh = useCallback(async () => {
    try {
      const next = await api.listTopics(projectId);
      setTopics(next);
      setScheduleDrafts(Object.fromEntries(next.map((topic) => [topic.id, toDateTimeLocal(topic.scheduled_at)])));
      setGeneratingIds((current) => {
        const stillGenerating = new Set(next.filter((topic) => topic.status === "generating").map((topic) => topic.id));
        return Object.fromEntries(Object.entries(current).filter(([id]) => stillGenerating.has(id)));
      });
    } catch (e: any) {
      setMessage({ title: "Topics unavailable", detail: e.message, tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    const hasGenerating = Object.keys(generatingIds).length > 0 || topics.some((topic) => topic.status === "generating");
    if (!hasGenerating) return;
    const interval = window.setInterval(refresh, 10_000);
    return () => window.clearInterval(interval);
  }, [generatingIds, refresh, topics]);

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

  const backlogTopics = useMemo(() => filtered.filter((topic) => isBacklogStatus(topic.status)), [filtered]);

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
    const nextScheduledAt = fromDateTimeLocal(scheduleDrafts[topic.id] ?? "");
    if (nextScheduledAt === null && topic.scheduled_at) {
      const ok = window.confirm(
        `Clear the scheduled date for “${topic.title}”? It will no longer publish on a set date.`,
      );
      if (!ok) return;
    }
    setBusy(`schedule-${topic.id}`);
    setMessage(null);
    try {
      const updated = await api.scheduleTopic(projectId, topic.id, nextScheduledAt);
      replaceTopic(updated);
      setMessage({ title: updated.scheduled_at ? "Topic scheduled" : "Schedule cleared", detail: updated.title, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Schedule failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function archive(topic: Topic) {
    const ok = window.confirm(`Remove “${topic.title}” from the content plan? You can restore it later from the archived filter.`);
    if (!ok) return;
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
    setGeneratingIds((current) => ({ ...current, [topic.id]: true }));
    setMessage(null);
    let keepGenerating = false;
    try {
      const result = await api.generateTopic(projectId, topic.id);
      if (result.topic) {
        replaceTopic(result.topic);
      } else {
        await refresh();
      }
      if (result.status === "generating") {
        keepGenerating = true;
        setMessage({
          title: "Starting draft generation",
          detail: "Writer and QA are running in the background. Review queue will update when drafts are ready.",
          tone: "green",
        });
        return;
      }
      await refresh();
      const existing = result.articles?.length ?? 0;
      setMessage(
        existing > 0
          ? {
              title: "Draft already exists",
              detail: `This topic already has ${existing} draft${existing === 1 ? "" : "s"} in the review queue. Open Review to approve or regenerate.`,
              tone: "amber",
            }
          : { title: "Topic generated", detail: "Draft is ready in the review queue.", tone: "green" },
      );
    } catch (e: any) {
      setMessage({
        title: "Generate failed",
        detail: e.message,
        tone: "red",
      });
    } finally {
      if (!keepGenerating) {
        setGeneratingIds((current) => {
          const next = { ...current };
          delete next[topic.id];
          return next;
        });
      }
    }
  }

  return (
    <div className="space-y-7">
      <section>
        <SectionHeader
          title="Topics"
          eyebrow="Backlog and schedule intent"
          action={
            <Button aria-busy={strategistRunning} disabled={strategistRunning} variant="primary" onClick={runStrategist}>
              {strategistRunning ? <Loader2 className="animate-spin" size={16} /> : <Wand2 size={16} />}
              {strategistRunning ? "Running strategist" : "Run Strategist"}
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
          <Button disabled={strategistRunning} onClick={refresh}>
            <RefreshCw size={16} />
            Refresh
          </Button>
        </div>
      </section>

      <section>
        <SectionHeader title="Backlog" action={<Badge tone="neutral">{backlogTopics.length}</Badge>} />
        {backlogTopics.length === 0 ? (
          <EmptyState title="No backlog topics found" detail="Drafted topics move to Review; run Strategist or adjust filters to populate the backlog." />
        ) : (
          <div className="grid gap-2">
            {backlogTopics.map((topic) => {
              const isGenerating = Boolean(generatingIds[topic.id]) || topic.status === "generating";
              const editBusy = busy === `edit-${topic.id}`;
              const scheduleBusy = busy === `schedule-${topic.id}`;
              const archiveBusy = busy === `archive-${topic.id}`;
              const topicLocked = topic.status === "archived" || isGenerating;
              return (
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
                      disabled={topicLocked || editBusy}
                      size="sm"
                      variant="ghost"
                      onClick={() => startEdit(topic)}
                    >
                      <Pencil size={14} />
                      Edit
                    </Button>
                    <Button aria-busy={isGenerating} disabled={topicLocked} size="sm" variant="primary" onClick={() => generate(topic)}>
                      {isGenerating ? <Loader2 className="animate-spin" size={14} /> : <Wand2 size={14} />}
                      {isGenerating ? "Generating" : "Generate"}
                    </Button>
                    <Button disabled={topicLocked || archiveBusy} size="sm" variant="danger" onClick={() => archive(topic)}>
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
                      <Button disabled={editBusy} size="sm" variant="ghost" onClick={cancelEdit}>
                        <X size={14} />
                        Cancel
                      </Button>
                      <Button disabled={editBusy || !draft.title.trim()} size="sm" variant="primary" onClick={() => saveEdit(topic)}>
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
                  <Button disabled={topicLocked || scheduleBusy} size="sm" onClick={() => schedule(topic)}>
                    <CalendarDays size={14} />
                    Schedule
                  </Button>
                </div>
              </div>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}
