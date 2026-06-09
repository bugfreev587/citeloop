import { auth } from "@clerk/nextjs/server";
import { ProjectShell } from "../../components/project-shell";
import { createApi, Project } from "../../lib/api";

const clerkServerAuthConfigured = Boolean(process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY && process.env.CLERK_SECRET_KEY);

export default async function ProjectLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  let token: string | null = null;
  if (clerkServerAuthConfigured) {
    const { getToken } = await auth();
    token = await getToken();
  }
  const api = createApi(token ? { token } : undefined);
  let project: Project | null = null;
  try {
    project = await api.getProject(id);
  } catch {
    project = null;
  }

  return (
    <ProjectShell project={project} projectId={id}>
      {children}
    </ProjectShell>
  );
}
