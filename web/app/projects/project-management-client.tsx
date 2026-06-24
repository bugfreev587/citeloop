"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { AlertTriangle, ArrowRight, FolderKanban, Loader2, Trash2, X } from "lucide-react";
import { Button, EmptyState, Notice, TextInput, cx } from "../components/ui";
import type { Project } from "../lib/api";
import { LAST_PROJECT_STORAGE_KEY } from "../lib/dashboard-routing";
import { useApi } from "../lib/use-api";

function initials(project: Project) {
  const source = (project.name || project.slug || "Project").trim();
  return source.slice(0, 2).toUpperCase();
}

export function ProjectManagementClient({ initialProjects }: { initialProjects: Project[] }) {
  const api = useApi();
  const router = useRouter();
  const [projects, setProjects] = useState(initialProjects);
  const [pendingDelete, setPendingDelete] = useState<Project | null>(null);
  const [confirmSlug, setConfirmSlug] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canDelete = Boolean(pendingDelete && confirmSlug === pendingDelete.slug && !deleting);

  function openDelete(project: Project) {
    setError(null);
    setConfirmSlug("");
    setPendingDelete(project);
  }

  function closeDelete() {
    if (deleting) return;
    setPendingDelete(null);
    setConfirmSlug("");
    setError(null);
  }

  async function hardDeleteProject() {
    if (!pendingDelete || !canDelete) return;
    setDeleting(true);
    setError(null);
    try {
      await api.deleteProject(pendingDelete.id);
      if (window.localStorage.getItem(LAST_PROJECT_STORAGE_KEY) === pendingDelete.id) {
        window.localStorage.removeItem(LAST_PROJECT_STORAGE_KEY);
      }
      setProjects((current) => current.filter((project) => project.id !== pendingDelete.id));
      setPendingDelete(null);
      setConfirmSlug("");
      router.refresh();
    } catch (e: any) {
      setError(e.message);
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div className="grid gap-4">
      {error && <Notice title="Project was not deleted" detail={error} tone="red" />}

      {projects.length === 0 ? (
        <EmptyState title="No projects yet" detail="Connect a product domain to create this account's first project." />
      ) : (
        <div className="grid gap-3">
          {projects.map((project) => (
            <div
              key={project.id}
              className={cx(
                "grid gap-3 rounded-xl border border-slate-200 bg-white px-4 py-3 md:grid-cols-[minmax(0,1fr)_auto]",
                pendingDelete?.id === project.id && "border-red-200 bg-red-50/30",
              )}
            >
              <div className="flex min-w-0 items-center gap-3">
                <div className="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-slate-100 text-sm font-bold text-slate-700">
                  {initials(project)}
                </div>
                <div className="min-w-0">
                  <div className="truncate text-base font-bold text-slate-900">{project.name}</div>
                  <div className="truncate text-sm text-slate-500">/{project.slug}</div>
                </div>
              </div>
              <div className="flex items-center gap-2 md:justify-end">
                <Link
                  href={`/projects/${project.id}`}
                  className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50 hover:text-slate-950"
                >
                  Open
                  <ArrowRight size={16} />
                </Link>
                <Button
                  type="button"
                  size="sm"
                  variant="danger"
                  onClick={() => openDelete(project)}
                  aria-label={`Delete project ${project.name}`}
                >
                  <Trash2 size={15} />
                  Delete project
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {pendingDelete && (
        <div className="rounded-xl border border-red-200 bg-white p-4 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-red-50 text-red-700">
              <AlertTriangle size={18} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="text-base font-bold text-slate-950">Permanently delete {pendingDelete.name}</div>
                  <p className="mt-1 text-sm leading-6 text-slate-600">
                    This permanently deletes the project and all associated data.
                  </p>
                </div>
                <Button type="button" size="sm" variant="ghost" onClick={closeDelete} aria-label="Cancel delete">
                  <X size={15} />
                </Button>
              </div>

              <div className="mt-4 grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
                <TextInput
                  value={confirmSlug}
                  onChange={(event) => setConfirmSlug(event.target.value)}
                  placeholder={`Type ${pendingDelete.slug} to confirm`}
                  aria-label="Confirm project slug"
                  disabled={deleting}
                />
                <Button type="button" variant="danger" onClick={hardDeleteProject} disabled={!canDelete}>
                  {deleting ? <Loader2 className="animate-spin" size={16} /> : <Trash2 size={16} />}
                  Permanently delete
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      <div className="rounded-xl border border-slate-200 bg-white px-4 py-3 text-sm text-slate-600">
        <div className="flex items-center gap-2 font-semibold text-slate-900">
          <FolderKanban size={16} />
          One account, multiple product domains
        </div>
        <p className="mt-1 leading-6">
          Each project keeps its own context, content plan, review queue, publishing state, and visibility data.
        </p>
      </div>
    </div>
  );
}
