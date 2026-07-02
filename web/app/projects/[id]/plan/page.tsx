import { ContentWorkflowClient } from "../content-workflow-client";

export default async function PlanPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <ContentWorkflowClient projectId={id} initialStep="plan" />;
}
