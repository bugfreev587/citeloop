import { ProjectShell } from "../../components/project-shell";
import { api, Project } from "../../lib/api";

export default async function ProjectLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: { id: string };
}) {
  let project: Project | null = null;
  try {
    project = await api.getProject(params.id);
  } catch {
    project = null;
  }

  return (
    <ProjectShell project={project} projectId={params.id}>
      {children}
    </ProjectShell>
  );
}
