import { auth } from "@clerk/nextjs/server";
import { notFound } from "next/navigation";
import { SettingsClient } from "./settings-client";
import { canUseInternalTools } from "../../../lib/admin-access";

export default async function SettingsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const { userId, sessionClaims } = await auth();
  if (
    !canUseInternalTools({
      userId,
      sessionClaims,
      adminUserIDs: process.env.CITELOOP_ADMIN_USER_IDS,
      clerkSecretKey: process.env.CLERK_SECRET_KEY,
    })
  ) {
    notFound();
  }

  return <SettingsClient projectId={id} />;
}
