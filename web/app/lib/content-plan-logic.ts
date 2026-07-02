export type PlanView = "list" | "grid" | "compact";

export type ContentPlanTopic = {
  id: string;
  channel: string;
  target_keyword: string | null;
  target_prompt: string | null;
  angle: string | null;
  format: string | null;
  priority: number;
  internal_links: any[];
  status: string;
  scheduled_at: string | null;
  created_at: string | null;
};

export function isBacklogStatus(status: string) {
  return status === "backlog" || status === "scheduled" || status === "generating";
}

export function topicPickScore(topic: ContentPlanTopic) {
  const briefComplete = Boolean((topic.target_keyword || topic.target_prompt) && topic.angle && topic.format);
  return (
    topicPriorityRank(topic.priority) * 10 +
    Math.min(topic.internal_links.length, 5) * 2 +
    (topic.channel === "both" ? 3 : 0) +
    (briefComplete ? 4 : 0)
  );
}

export function normalizedTopicPriority(priority: number) {
  if (!Number.isFinite(priority) || priority <= 0) return 0;
  if (priority > 10) return Math.min(10, Math.max(1, Math.ceil((100 - priority) / 10)));
  return Math.min(10, Math.max(1, Math.round(priority)));
}

export function topicPriorityRank(priority: number) {
  const normalized = normalizedTopicPriority(priority);
  return normalized > 0 ? 11 - normalized : 0;
}

export function topicWhy(topic: ContentPlanTopic) {
  if (topic.angle) return topic.angle;
  if (topic.target_prompt) return `Answers: ${topic.target_prompt}`;
  if (topic.target_keyword) return `Targets: ${topic.target_keyword}`;
  return "Generated from current context gaps and available evidence.";
}

export function topicPickSignal(topic: ContentPlanTopic) {
  if (topic.scheduled_at) return "Scheduled intent";
  if (topic.internal_links.length >= 3) return "Strong internal-link base";
  if (topic.channel === "both") return "Covers blog + syndication";
  if ((topic.target_keyword || topic.target_prompt) && topic.angle && topic.format) return "Complete brief";
  if (topic.priority > 0) return "Priority set by plan";
  return "Needs priority decision";
}

export function recommendedTopicIds(topics: ContentPlanTopic[]) {
  return [...topics]
    .filter((topic) => topic.status === "backlog" && !topic.scheduled_at)
    .sort((a, b) => {
      const score = topicPickScore(b) - topicPickScore(a);
      if (score !== 0) return score;
      return String(b.created_at ?? "").localeCompare(String(a.created_at ?? ""));
    })
    .slice(0, 3)
    .map((topic) => topic.id);
}

export function planHealthForTopics(topics: ContentPlanTopic[]) {
  const backlogTopics = topics.filter((topic) => isBacklogStatus(topic.status));
  return {
    backlog: backlogTopics.length,
    readyToDraft: backlogTopics.filter((topic) => topic.status === "backlog" && !topic.scheduled_at).length,
    scheduledIntent: backlogTopics.filter((topic) => topic.scheduled_at).length,
    needsPriority: backlogTopics.filter((topic) => topic.priority <= 0).length,
  };
}

function countLabel(count: number, singular: string, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`;
}

export function planPulseForTopics(topics: ContentPlanTopic[]) {
  const health = planHealthForTopics(topics);
  if (health.backlog === 0) {
    return {
      title: "No topics in the plan yet",
      detail: "Review opportunities or generate from domain to seed the first backlog.",
      tone: "neutral" as const,
    };
  }

  const readyWithoutPriority = Math.max(0, health.readyToDraft - health.needsPriority);
  const details = [
    readyWithoutPriority > 0 ? countLabel(readyWithoutPriority, "ready to draft", "ready to draft") : null,
    health.scheduledIntent > 0 ? countLabel(health.scheduledIntent, "scheduled", "scheduled") : null,
    health.needsPriority > 0 ? countLabel(health.needsPriority, "needs priority", "need priority") : null,
  ].filter(Boolean);

  return {
    title: countLabel(health.backlog, "topic") + " in the plan",
    detail: details.length > 0 ? `${details.join(", ")}.` : "CiteLoop is drafting from this backlog automatically.",
    tone: health.needsPriority > 0 ? ("amber" as const) : ("green" as const),
  };
}

export function topicCardSpanClass(view: PlanView, editing: boolean) {
  if (!editing || view === "list") return "";
  if (view === "grid") return "lg:col-span-2";
  return "lg:col-span-2 2xl:col-span-3";
}
