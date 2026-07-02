import { DoctorClient } from "./doctor-client";

export default async function DoctorPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <DoctorClient projectId={id} />;
}
