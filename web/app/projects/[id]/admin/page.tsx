import { auth } from "@clerk/nextjs/server";
import { notFound } from "next/navigation";
import { AdminClient } from "./admin-client";
import { canUseInternalTools } from "../../../lib/admin-access";

export default async function AdminPage() {
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

  return <AdminClient />;
}
