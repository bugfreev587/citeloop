import { auth } from "@clerk/nextjs/server";
import { notFound } from "next/navigation";
import { SettingsClient } from "./settings-client";
import { clerkServerAuthConfigured, requireConfiguredClerk } from "../../../lib/auth-config";
import { canUseInternalTools } from "../../../lib/admin-access";

export default async function SettingsPage({ params }: { params: Promise<{ id: string }> }) {
  requireConfiguredClerk();

  const { id } = await params;
  if (clerkServerAuthConfigured) {
    const { userId, sessionClaims } = await auth();
    const canAccessSettings = canUseInternalTools({
      userId,
      sessionClaims,
      adminUserIDs: process.env.CITELOOP_ADMIN_USER_IDS,
      clerkSecretKey: process.env.CLERK_SECRET_KEY,
    });
    if (!canAccessSettings) {
      notFound();
    }
  }

  return <SettingsClient projectId={id} />;
}
