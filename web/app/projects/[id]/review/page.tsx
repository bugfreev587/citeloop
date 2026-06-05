import { ReviewClient } from "./review-client";

export default async function ReviewPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <ReviewClient projectId={id} />;
}
