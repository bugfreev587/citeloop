export type ContentWorkflowStep = "plan" | "review" | "publish";

export const CONTENT_WORKFLOW_STEPS: ContentWorkflowStep[] = ["plan", "review", "publish"];

export type ContentWorkflowStepPosition = {
  step: ContentWorkflowStep;
  top: number | null;
};

export function selectActiveContentWorkflowStep(
  positions: ContentWorkflowStepPosition[],
  options: { marker: number; targetTopOffset: number },
): ContentWorkflowStep {
  let nextStep: ContentWorkflowStep = "plan";
  let bestDistance = Number.POSITIVE_INFINITY;

  for (const position of positions) {
    if (position.top == null || !Number.isFinite(position.top)) continue;
    if (position.top - options.marker > 0) continue;

    const distance = Math.abs(position.top - options.targetTopOffset);
    if (distance < bestDistance) {
      nextStep = position.step;
      bestDistance = distance;
    }
  }

  return nextStep;
}
