import { redirect } from "next/navigation";

export default async function ActivityLogPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  redirect(`/projects/${id}/settings#activity`);
}
