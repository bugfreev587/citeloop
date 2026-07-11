import { DoctorClient } from "./doctor-client";

export default async function DoctorPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ finding?: string | string[] }>;
}) {
  const [{ id }, query] = await Promise.all([params, searchParams]);
  const initialFindingId = typeof query.finding === "string" ? query.finding : undefined;
  return <DoctorClient projectId={id} initialFindingId={initialFindingId} />;
}
