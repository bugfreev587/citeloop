import { redirect } from "next/navigation";

export default async function ProjectAdminPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  redirect(`/admin?from=${encodeURIComponent(id)}`);
}
