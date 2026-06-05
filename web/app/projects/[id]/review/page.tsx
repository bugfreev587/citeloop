import { ReviewClient } from "./review-client";

export default function ReviewPage({ params }: { params: { id: string } }) {
  return <ReviewClient projectId={params.id} />;
}
