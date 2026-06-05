import { auth } from "@clerk/nextjs/server";
import { ProjectShell } from "../../components/project-shell";
import { createApi, Project } from "../../lib/api";

export default async function ProjectLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const { getToken } = await auth();
  const token = await getToken();
  const api = createApi({ token });
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
