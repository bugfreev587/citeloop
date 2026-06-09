"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { Archive, ArrowRight, RefreshCw, RotateCcw, Trash2 } from "lucide-react";
import { Project } from "./lib/api";
import { useApi } from "./lib/use-api";
import { Badge, Button, EmptyState, Notice } from "./components/ui";

type ConfirmState = {
  action: "archive" | "delete";
  project: Project;
} | null;

type Message = { title: string; detail?: string; tone: "green" | "amber" | "red" | "neutral" } | null;

export function ProjectListClient({ initialProjects }: { initialProjects: Project[] }) {
  const api = useApi();
  const [activeProjects, setActiveProjects] = useState<Project[]>(initialProjects.filter((project) => project.status !== "archived"));
  const [archivedProjects, setArchivedProjects] = useState<Project[]>([]);
  const [archivedLoaded, setArchivedLoaded] = useState(false);
  const [showArchived, setShowArchived] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<ConfirmState>(null);
  const [message, setMessage] = useState<Message>(null);

  const archivedCount = useMemo(() => archivedProjects.length, [archivedProjects]);

  async function toggleArchived() {
    if (showArchived) {
      setShowArchived(false);
      return;
    }
    setShowArchived(true);
    if (archivedLoaded) return;
    setBusy("load-archived");
    setMessage(null);
    try {
      setArchivedProjects(await api.listProjects("archived"));
      setArchivedLoaded(true);
    } catch (e: any) {
      setMessage({ title: "Could not load archived projects", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function archiveProject(project: Project) {
    setBusy(`archive-${project.id}`);
    setMessage(null);
    try {
      const archived = await api.archiveProject(project.id);
      setActiveProjects((projects) => projects.filter((row) => row.id !== project.id));
      if (archivedLoaded) {
        setArchivedProjects((projects) => [archived, ...projects.filter((row) => row.id !== project.id)]);
      }
      setConfirm(null);
      setMessage({ title: "Project archived", detail: `${project.name} is hidden from the active list.`, tone: "amber" });
    } catch (e: any) {
      setMessage({ title: "Could not archive project", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function restoreProject(project: Project) {
    setBusy(`restore-${project.id}`);
    setMessage(null);
    try {
      const restored = await api.restoreProject(project.id);
      setArchivedProjects((projects) => projects.filter((row) => row.id !== project.id));
      setActiveProjects((projects) => [restored, ...projects.filter((row) => row.id !== project.id)]);
      setMessage({ title: "Project restored", detail: `${project.name} is active again.`, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not restore project", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function deleteProject(project: Project) {
    setBusy(`delete-${project.id}`);
    setMessage(null);
    try {
      await api.deleteProject(project.id);
      setActiveProjects((projects) => projects.filter((row) => row.id !== project.id));
      setArchivedProjects((projects) => projects.filter((row) => row.id !== project.id));
      setConfirm(null);
      setMessage({ title: "Project deleted", detail: `${project.name} was removed.`, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not delete project", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-xl font-bold leading-7 text-slate-900">Projects</h2>
          <Badge tone="neutral">{activeProjects.length} active</Badge>
          {showArchived && <Badge tone="amber">{archivedCount} archived</Badge>}
        </div>
        <Button size="sm" onClick={toggleArchived} disabled={busy === "load-archived"}>
          <Archive size={14} />
          {showArchived ? "Hide archived" : "Show archived"}
        </Button>
      </div>

      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      <div className="grid gap-3">
        {activeProjects.map((project) => (
          <ProjectCard
            key={project.id}
            project={project}
            confirm={confirm}
            busy={busy}
            onConfirm={setConfirm}
            onArchive={archiveProject}
            onDelete={deleteProject}
          />
        ))}
        {activeProjects.length === 0 && (
          <EmptyState title="No active projects" detail="Create a project or restore one from the archived list." />
        )}
      </div>

      {showArchived && (
        <section className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-bold uppercase tracking-wide text-slate-500">Archived</h3>
            <Button size="sm" variant="ghost" onClick={toggleArchived} disabled={busy === "load-archived"}>
              <RefreshCw size={14} />
              Hide
            </Button>
          </div>
          {busy === "load-archived" ? (
            <EmptyState title="Loading archived projects" detail="Fetching archived projects." />
          ) : archivedProjects.length === 0 ? (
            <EmptyState title="No archived projects" detail="Archived projects will appear here with restore controls." />
          ) : (
            <div className="grid gap-3">
              {archivedProjects.map((project) => (
                <ProjectCard
                  key={project.id}
                  project={project}
                  confirm={confirm}
                  busy={busy}
                  archived
                  onConfirm={setConfirm}
                  onRestore={restoreProject}
                  onDelete={deleteProject}
                />
              ))}
            </div>
          )}
        </section>
      )}
    </div>
  );
}

function ProjectCard({
  project,
  confirm,
  busy,
  archived = false,
  onConfirm,
  onArchive,
  onRestore,
  onDelete,
}: {
  project: Project;
  confirm: ConfirmState;
  busy: string | null;
  archived?: boolean;
  onConfirm: (state: ConfirmState) => void;
  onArchive?: (project: Project) => void;
  onRestore?: (project: Project) => void;
  onDelete: (project: Project) => void;
}) {
  const isConfirming = confirm?.project.id === project.id ? confirm.action : null;

  return (
    <div className="rounded-xl border border-slate-200 bg-white px-4 py-3 transition-colors hover:bg-slate-50">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <Link href={`/projects/${project.id}`} className="group min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <div className="truncate text-base font-bold text-slate-900">{project.name}</div>
            {archived && <Badge tone="amber">archived</Badge>}
          </div>
          <div className="mt-1 flex min-w-0 items-center gap-2 text-sm text-slate-500">
            <span className="truncate">/{project.slug}</span>
            <ArrowRight className="text-slate-400 transition-transform group-hover:translate-x-0.5 group-hover:text-[#d93820]" size={14} />
          </div>
        </Link>
        <div className="flex shrink-0 flex-wrap gap-2">
          {archived ? (
            <Button size="sm" onClick={() => onRestore?.(project)} disabled={busy === `restore-${project.id}`}>
              <RotateCcw size={14} />
              Restore
            </Button>
          ) : (
            <Button size="sm" onClick={() => onConfirm({ action: "archive", project })} disabled={busy === `archive-${project.id}`}>
              <Archive size={14} />
              Archive
            </Button>
          )}
          <Button size="sm" variant="danger" onClick={() => onConfirm({ action: "delete", project })} disabled={busy === `delete-${project.id}`}>
            <Trash2 size={14} />
            Delete
          </Button>
        </div>
      </div>

      {isConfirming && (
        <div className="mt-3 rounded-lg border border-amber-200 bg-amber-50 p-3">
          <div className="text-sm font-bold text-amber-950">
            {isConfirming === "archive" ? "Archive this project?" : "Delete this empty project?"}
          </div>
          <p className="mt-1 text-sm leading-6 text-amber-900">
            {isConfirming === "archive"
              ? "Archived projects are hidden from the active list and can be restored later."
              : "Delete only works for projects without generated, configured, or published operational data. Otherwise CiteLoop will block the delete and keep the project archived instead."}
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            {isConfirming === "archive" ? (
              <Button size="sm" onClick={() => onArchive?.(project)} disabled={!!busy}>
                Confirm archive
              </Button>
            ) : (
              <Button size="sm" variant="danger" onClick={() => onDelete(project)} disabled={!!busy}>
                Confirm delete
              </Button>
            )}
            <Button size="sm" variant="ghost" onClick={() => onConfirm(null)} disabled={!!busy}>
              Cancel
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
