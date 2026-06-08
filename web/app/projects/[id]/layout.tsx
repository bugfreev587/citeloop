import { auth } from "@clerk/nextjs/server";
import { ProjectShell } from "../../components/project-shell";
import { createApi, DeploymentVersion, Project } from "../../lib/api";
import { getWebBuildInfo } from "../../lib/build-info";

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
  let apiVersion: DeploymentVersion | null = null;
  try {
    [project, apiVersion] = await Promise.all([api.getProject(id), api.getVersion()]);
  } catch {
    project = null;
    try {
      apiVersion = await api.getVersion();
    } catch {
      apiVersion = null;
    }
  }

  return (
    <ProjectShell project={project} projectId={id} apiVersion={apiVersion} webBuild={getWebBuildInfo()}>
      {children}
    </ProjectShell>
  );
}
