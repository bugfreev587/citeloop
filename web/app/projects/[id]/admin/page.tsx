import { AdminClient } from "./admin-client";

export default async function AdminPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <AdminClient projectId={id} />;
}
