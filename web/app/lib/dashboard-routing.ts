type ProjectLike = {
  id: string;
};

export const LAST_PROJECT_STORAGE_KEY = "citeloop:last-project-id";
export const CONTENT_WORKFLOW_PATH_CHANGE_EVENT = "citeloop:content-workflow-path-change";

export function dashboardHrefForProjects(
  projects: ProjectLike[],
  storedProjectId: string | null,
  projectPrefetchFailed = false,
) {
  if (projectPrefetchFailed || projects.length === 0) {
    return "/projects";
  }

  const storedProject = storedProjectId ? projects.find((project) => project.id === storedProjectId) : null;
  return `/projects/${storedProject?.id ?? projects[0].id}`;
}
