"use client";

import { useCallback, useEffect, useRef } from "react";
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

  const scrollToStep = useCallback((step: ContentWorkflowStep, behavior: ScrollBehavior = "auto") => {
    const section = sectionRefs.current[step];
    if (!section) return;
    const top = Math.max(section.getBoundingClientRect().top + window.scrollY - 24, 0);
    window.scrollTo({ top, behavior });
  }, []);

  const updateActiveStep = useCallback(() => {
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
    activeStepRef.current = nextStep;

    const nextHref = workflowHref(projectId, nextStep);
    if (window.location.pathname !== nextHref) {
      window.history.replaceState(window.history.state, "", nextHref);
    }
  }, [projectId]);

  useEffect(() => {
    activeStepRef.current = initialStep;
    const frame = window.requestAnimationFrame(() => {
      scrollToStep(initialStep, "auto");
      updateActiveStep();
    });
    return () => window.cancelAnimationFrame(frame);
  }, [initialStep, scrollToStep, updateActiveStep]);

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
    <div data-content-workflow className="space-y-10">
      <section
        id={SECTION_IDS.plan}
        ref={setSectionRef("plan")}
        data-content-workflow-section="plan"
        className="scroll-mt-8"
      >
        <TopicsClient projectId={projectId} />
      </section>

      <section
        id={SECTION_IDS.review}
        ref={setSectionRef("review")}
        data-content-workflow-section="review"
        className="scroll-mt-8 border-t border-slate-200 pt-10"
      >
        <ReviewClient projectId={projectId} />
      </section>

      <section
        id={SECTION_IDS.publish}
        ref={setSectionRef("publish")}
        data-content-workflow-section="publish"
        className="scroll-mt-8 border-t border-slate-200 pt-10"
      >
        <PublishingClient projectId={projectId} />
      </section>
    </div>
  );
}
