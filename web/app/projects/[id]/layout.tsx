import { auth } from "@clerk/nextjs/server";
import { ProjectShell } from "../../components/project-shell";
import { clerkServerAuthConfigured, requireConfiguredClerk } from "../../lib/auth-config";
import { createApi, friendlyApiError, isProjectMissingError, Project } from "../../lib/api";

function isRecoverableProjectLoadError(error: unknown) {
  return error instanceof Error && error.message === "CiteLoop API request timed out";
}

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
  if (clerkServerAuthConfigured) {
    const { getToken } = await auth();
    token = await getToken();
  }
  const api = createApi(token ? { token } : undefined);
  let project: Project | null = null;
  let projectLoadError: string | null = null;
  let recoverableProjectLoadError = false;
  try {
    project = await api.getProject(id);
  } catch (error) {
    project = null;
    if (!isProjectMissingError(error)) {
      projectLoadError = friendlyApiError(error);
      recoverableProjectLoadError = isRecoverableProjectLoadError(error);
    }
  }
  const shouldRenderProjectChildren = Boolean(project) || recoverableProjectLoadError;

  return (
    <ProjectShell project={project} projectId={id} projectLoadError={projectLoadError}>
      {shouldRenderProjectChildren ? children : null}
    </ProjectShell>
  );
}
