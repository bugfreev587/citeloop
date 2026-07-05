"use client";

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { cx } from "../../components/ui";
import { CONTENT_WORKFLOW_PATH_CHANGE_EVENT } from "../../lib/dashboard-routing";
import { ContentWorkflowStageHeaderActionContext } from "./content-workflow-stage-actions";
import { PublishingClient } from "./publishing/publishing-client";
import { ReviewClient } from "./review/review-client";
import { TopicsClient } from "./topics/topics-client";

export type ContentWorkflowStep = "plan" | "review" | "publish";

const WORKFLOW_STEPS: ContentWorkflowStep[] = ["plan", "review", "publish"];
const SECTION_IDS: Record<ContentWorkflowStep, string> = {
  plan: "content-workflow-plan",
  review: "content-workflow-review",
  publish: "content-workflow-publish",
};

const TARGET_TOP_OFFSET = 24;
const TARGET_ALIGNMENT_TOLERANCE = 8;
const TARGET_SETTLE_TIMEOUT_MS = 1_200;

type StageMeta = {
  stepLabel: string;
  title: string;
  eyebrow: string;
  toneClass: string;
  accentClass: string;
};

const STAGE_META = {
  plan: {
    stepLabel: "Step 1 of 3",
    title: "Content Plan",
    eyebrow: "Planned topics and action handoff",
    toneClass: "border-sky-200 bg-sky-100/70",
    accentClass: "bg-sky-500",
  },
  review: {
    stepLabel: "Step 2 of 3",
    title: "Review",
    eyebrow: "Approval gate and draft decisions",
    toneClass: "border-amber-200 bg-amber-100/70",
    accentClass: "bg-amber-500",
  },
  publish: {
    stepLabel: "Step 3 of 3",
    title: "Publish",
    eyebrow: "Ready content first. Choose where and when per post.",
    toneClass: "border-emerald-200 bg-emerald-100/70",
    accentClass: "bg-emerald-500",
  },
} satisfies Record<ContentWorkflowStep, StageMeta>;

function workflowHref(projectId: string, step: ContentWorkflowStep) {
  return `/projects/${projectId}/${step}`;
}

export function ContentWorkflowClient({
  projectId,
  initialStep,
}: {
  projectId: string;
  initialStep: ContentWorkflowStep;
}) {
  const activeStepRef = useRef<ContentWorkflowStep>(initialStep);
  const pendingTargetRef = useRef<ContentWorkflowStep | null>(null);
  const pendingStartedAtRef = useRef(0);
  const tickingRef = useRef(false);
  const sectionRefs = useRef<Record<ContentWorkflowStep, HTMLElement | null>>({
    plan: null,
    review: null,
    publish: null,
  });

  const setSectionRef = useCallback((step: ContentWorkflowStep) => {
    return (node: HTMLElement | null) => {
      sectionRefs.current[step] = node;
    };
  }, []);

  const sectionTopForStep = useCallback((step: ContentWorkflowStep) => {
    const section = sectionRefs.current[step];
    if (!section) return null;
    return Math.max(section.getBoundingClientRect().top + window.scrollY - TARGET_TOP_OFFSET, 0);
  }, []);

  const isStepAligned = useCallback((step: ContentWorkflowStep) => {
    const section = sectionRefs.current[step];
    if (!section) return false;
    if (step === "plan" && window.scrollY === 0) return section.getBoundingClientRect().top <= TARGET_TOP_OFFSET + TARGET_ALIGNMENT_TOLERANCE;
    return Math.abs(section.getBoundingClientRect().top - TARGET_TOP_OFFSET) <= TARGET_ALIGNMENT_TOLERANCE;
  }, []);

  const scrollToStep = useCallback((step: ContentWorkflowStep, behavior: ScrollBehavior = "auto") => {
    const top = sectionTopForStep(step);
    if (top == null) return false;
    window.scrollTo({ top, behavior });
    return true;
  }, [sectionTopForStep]);

  const syncPathToStep = useCallback((step: ContentWorkflowStep) => {
    activeStepRef.current = step;
    const nextHref = workflowHref(projectId, step);
    if (window.location.pathname !== nextHref) {
      window.history.replaceState(window.history.state, "", nextHref);
    }
    window.dispatchEvent(new CustomEvent(CONTENT_WORKFLOW_PATH_CHANGE_EVENT, { detail: { pathname: nextHref } }));
  }, [projectId]);

  const updateActiveStep = useCallback(() => {
    if (pendingTargetRef.current) {
      const pendingStep = pendingTargetRef.current;
      const elapsed = window.performance.now() - pendingStartedAtRef.current;
      if (!isStepAligned(pendingStep) && elapsed < TARGET_SETTLE_TIMEOUT_MS) {
        syncPathToStep(pendingStep);
        return;
      }
      pendingTargetRef.current = null;
    }

    const marker = window.innerHeight * 0.35;
    let nextStep: ContentWorkflowStep = "plan";

    for (const step of WORKFLOW_STEPS) {
      const section = sectionRefs.current[step];
      if (!section) continue;
      if (section.getBoundingClientRect().top - marker <= 0) {
        nextStep = step;
      }
    }

    if (activeStepRef.current === nextStep) return;
    syncPathToStep(nextStep);
  }, [isStepAligned, syncPathToStep]);

  useEffect(() => {
    pendingTargetRef.current = initialStep;
    pendingStartedAtRef.current = window.performance.now();
    syncPathToStep(initialStep);

    let frame = 0;
    const settleTarget = () => {
      scrollToStep(initialStep, "auto");
      if (isStepAligned(initialStep) || window.performance.now() - pendingStartedAtRef.current >= TARGET_SETTLE_TIMEOUT_MS) {
        pendingTargetRef.current = null;
        updateActiveStep();
        return;
      }
      frame = window.requestAnimationFrame(settleTarget);
    };

    frame = window.requestAnimationFrame(settleTarget);
    return () => window.cancelAnimationFrame(frame);
  }, [initialStep, isStepAligned, scrollToStep, syncPathToStep, updateActiveStep]);

  useEffect(() => {
    const scheduleUpdate = () => {
      if (tickingRef.current) return;
      tickingRef.current = true;
      window.requestAnimationFrame(() => {
        tickingRef.current = false;
        updateActiveStep();
      });
    };

    window.addEventListener("scroll", scheduleUpdate, { passive: true });
    window.addEventListener("resize", scheduleUpdate);
    const frame = window.requestAnimationFrame(updateActiveStep);

    return () => {
      window.cancelAnimationFrame(frame);
      window.removeEventListener("scroll", scheduleUpdate);
      window.removeEventListener("resize", scheduleUpdate);
    };
  }, [updateActiveStep]);

  return (
    <div data-content-workflow className="space-y-8">
      <WorkflowStage step="plan" setSectionRef={setSectionRef("plan")}>
        <TopicsClient projectId={projectId} />
      </WorkflowStage>

      <WorkflowStage step="review" setSectionRef={setSectionRef("review")}>
        <ReviewClient projectId={projectId} />
      </WorkflowStage>

      <WorkflowStage step="publish" setSectionRef={setSectionRef("publish")}>
        <PublishingClient projectId={projectId} />
      </WorkflowStage>
    </div>
  );
}

function WorkflowStage({
  step,
  setSectionRef,
  children,
}: {
  step: ContentWorkflowStep;
  setSectionRef: (node: HTMLElement | null) => void;
  children: ReactNode;
}) {
  const meta = STAGE_META[step];
  const [headerActionTarget, setHeaderActionTarget] = useState<HTMLElement | null>(null);

  return (
    <section
      id={SECTION_IDS[step]}
      ref={setSectionRef}
      data-content-workflow-section={step}
      data-content-workflow-stage-shell
      className={cx("scroll-mt-8 overflow-hidden rounded-2xl border px-4 py-5 sm:px-6", meta.toneClass)}
    >
      <div data-content-workflow-stage-accent className={cx("mb-4 h-1.5 w-16 rounded-full", meta.accentClass)} />
      <div className="mb-4 border-b border-white/70 pb-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <h1 data-content-workflow-stage-title className="text-[26px] font-bold leading-8 tracking-tight text-slate-900 sm:shrink-0 sm:text-[30px] sm:leading-9">
            {meta.title}
          </h1>
          <div
            ref={setHeaderActionTarget}
            data-content-workflow-stage-header-action
            className="flex min-h-9 min-w-0 items-center justify-start sm:flex-1 sm:justify-end"
          />
        </div>
        <div className="mt-3">
          <div data-content-workflow-stage-step className="text-sm font-bold uppercase tracking-[0.14em] text-slate-500">
            {meta.stepLabel}
          </div>
          <div className="mt-1 text-sm font-medium text-slate-500">{meta.eyebrow}</div>
        </div>
      </div>
      <ContentWorkflowStageHeaderActionContext.Provider value={headerActionTarget}>
        <div data-content-workflow-stage-body className="min-w-0">
          {children}
        </div>
      </ContentWorkflowStageHeaderActionContext.Provider>
    </section>
  );
}
