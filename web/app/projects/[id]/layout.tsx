import { auth } from "@clerk/nextjs/server";
import { ProjectShell } from "../../components/project-shell";
import { clerkServerAuthConfigured, requireConfiguredClerk } from "../../lib/auth-config";
import { canUseInternalTools } from "../../lib/admin-access";
import { createApi, Project } from "../../lib/api";

export default async function ProjectLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ id: string }>;
}) {
  requireConfiguredClerk();

  const { id } = await params;
  let token: string | null = null;
  let canAccessSettings = true;
  if (clerkServerAuthConfigured) {
    const { getToken, userId, sessionClaims } = await auth();
    token = await getToken();
    canAccessSettings = canUseInternalTools({
      userId,
      sessionClaims,
      adminUserIDs: process.env.CITELOOP_ADMIN_USER_IDS,
      clerkSecretKey: process.env.CLERK_SECRET_KEY,
    });
  }
  const api = createApi(token ? { token } : undefined);
  let project: Project | null = null;
  try {
    project = await api.getProject(id);
  } catch {
    project = null;
  }

  return (
    <ProjectShell project={project} projectId={id} canAccessSettings={canAccessSettings}>
      {children}
    </ProjectShell>
  );
}
