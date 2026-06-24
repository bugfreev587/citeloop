import { SettingsClient } from "./settings-client";
import { requireConfiguredClerk } from "../../../lib/auth-config";

export default async function SettingsPage({ params }: { params: Promise<{ id: string }> }) {
  requireConfiguredClerk();

  const { id } = await params;
  return <SettingsClient projectId={id} />;
}
