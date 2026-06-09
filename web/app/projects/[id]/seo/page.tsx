import { redirect } from "next/navigation";

export default async function SEOPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  redirect(`/projects/${id}/visibility`);
}
