import { SettingsClient } from "./settings-client";

export default function SettingsPage({ params }: { params: { id: string } }) {
  return <SettingsClient projectId={params.id} />;
}
