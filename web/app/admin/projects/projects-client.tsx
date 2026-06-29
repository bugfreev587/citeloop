"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { ArrowLeft, ExternalLink, FolderKanban, Loader2, RefreshCw, ShieldCheck, Trash2 } from "lucide-react";
import { AdminProject } from "../../lib/api";
import { useApi } from "../../lib/use-api";
import { useToast } from "../../components/toast-provider";
import { Badge, Button, ButtonProgress, EmptyState, Notice, SectionHeader, cx } from "../../components/ui";

type Access = "loading" | "granted" | "denied";

function formatDateTime(value: any) {
  if (!value) return "Not set";
  const raw = typeof value === "string" ? value : value.Time ?? value.time ?? value;
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return String(raw);
  return new Intl.DateTimeFormat("en", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(date);
}

function projectSiteURL(project: AdminProject) {
  return project.config?.site_url?.trim() || "";
}

export function ProjectsClient() {
  const api = useApi();
  const { notify } = useToast();
  const [access, setAccess] = useState<Access>("loading");
  const [projects, setProjects] = useState<AdminProject[]>([]);
  const [loading, setLoading] = useState(true);
  const [deletingID, setDeletingID] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const me = await api.getMe();
      if (!me.is_admin) {
        setAccess("denied");
        setProjects([]);
        return;
      }
      setAccess("granted");
      setProjects(await api.listAdminProjects());
    } catch (error: any) {
      if (String(error.message).includes("403")) {
        setAccess("denied");
        setProjects([]);
      } else {
        setAccess("granted");
        notify({ title: "Projects unavailable", detail: error.message, tone: "red" });
      }
    } finally {
      setLoading(false);
    }
  }, [api, notify]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const ownerCount = useMemo(() => new Set(projects.map((project) => project.owner_id).filter(Boolean)).size, [projects]);

  async function deleteProject(project: AdminProject) {
    const ownerLabel = project.owner_email || project.owner_id || "unknown owner";
    if (
      !window.confirm(
        `Delete project "${project.name}" for ${ownerLabel}? This permanently removes the project and its generated work.`,
      )
    ) {
      return;
    }
    setDeletingID(project.id);
    try {
      await api.deleteAdminProject(project.id);
      setProjects((current) => current.filter((item) => item.id !== project.id));
      notify({ title: "Project deleted", detail: `${project.name} was removed.`, tone: "green" });
    } catch (error: any) {
      notify({ title: "Delete failed", detail: error.message, tone: "red" });
    } finally {
      setDeletingID(null);
    }
  }

  if (access === "loading") {
    return (
      <main className="grid min-h-[60vh] place-items-center text-sm text-slate-500">
        <span className="inline-flex items-center gap-2">
          <Loader2 size={16} className="animate-spin" />
          Checking admin access...
        </span>
      </main>
    );
  }

  if (access === "denied") {
    return (
      <main className="mx-auto max-w-md px-4 py-16 text-center">
        <ShieldCheck className="mx-auto text-slate-300" size={40} />
        <h1 className="mt-4 text-xl font-bold text-slate-900">Admin access required</h1>
        <p className="mt-2 text-sm leading-6 text-slate-500">
          This project management page is limited to platform administrators.
        </p>
        <Link href="/docs" className="mt-4 inline-flex items-center gap-2 text-sm font-semibold text-[#d93820] hover:underline">
          <ArrowLeft size={14} />
          Back to docs
        </Link>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-[1180px] px-4 py-6 md:px-6 md:py-8">
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <Link href="/admin" className="inline-flex items-center gap-2 text-sm font-semibold text-slate-500 hover:text-slate-900">
          <ArrowLeft size={15} />
          Admin
        </Link>
        <Badge tone="neutral">Projects</Badge>
      </div>

      <SectionHeader
        title="Projects"
        eyebrow="Admin management across all CiteLoop accounts"
        action={
          <Button disabled={loading || deletingID !== null} onClick={refresh}>
            <ButtonProgress busy={loading} busyLabel="Refreshing" idleIcon={<RefreshCw size={15} />}>
              Refresh
            </ButtonProgress>
          </Button>
        }
      />

      <div className="mb-4 grid gap-3 md:grid-cols-3">
        <div className="rounded-lg border border-slate-200 bg-white px-4 py-3">
          <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Projects</div>
          <div className="mt-1 text-2xl font-bold text-slate-950">{projects.length}</div>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white px-4 py-3">
          <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Owners</div>
          <div className="mt-1 text-2xl font-bold text-slate-950">{ownerCount}</div>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white px-4 py-3">
          <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Scope</div>
          <div className="mt-1 text-sm font-semibold text-slate-700">All accounts</div>
        </div>
      </div>

      <Notice
        title="Deleting is permanent"
        detail="This view can remove projects that belong to old development Clerk owner IDs as well as current production accounts. Use Owner ID when Owner email is unavailable."
        tone="amber"
      />

      <section className="mt-4 overflow-hidden rounded-xl border border-slate-200 bg-white">
        {projects.length === 0 && !loading ? (
          <div className="p-4">
            <EmptyState title="No projects found" detail="There are no CiteLoop projects in this environment." />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-slate-200 text-sm">
              <thead className="bg-slate-50 text-left text-xs font-bold uppercase tracking-[0.08em] text-slate-500">
                <tr>
                  <th className="px-4 py-3">Project</th>
                  <th className="px-4 py-3">Owner email</th>
                  <th className="px-4 py-3">Owner ID</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3">Last updated at</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {projects.map((project) => {
                  const siteURL = projectSiteURL(project);
                  const deleting = deletingID === project.id;
                  return (
                    <tr key={project.id} className={cx("align-top", deleting && "bg-red-50/40")}>
                      <td className="px-4 py-3">
                        <div className="flex items-start gap-3">
                          <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-500">
                            <FolderKanban size={16} />
                          </div>
                          <div className="min-w-[220px]">
                            <div className="font-semibold text-slate-950">{project.name}</div>
                            <div className="mt-1 font-mono text-xs text-slate-500">{project.id}</div>
                            {siteURL && (
                              <a href={siteURL} target="_blank" rel="noreferrer" className="mt-1 inline-flex items-center gap-1 text-xs font-semibold text-[#d93820] hover:underline">
                                {siteURL}
                                <ExternalLink size={12} />
                              </a>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        {project.owner_email ? (
                          <span className="font-medium text-slate-800">{project.owner_email}</span>
                        ) : (
                          <Badge tone="amber">Unknown</Badge>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <div className="max-w-[260px] break-all font-mono text-xs text-slate-500">{project.owner_id || "Unknown"}</div>
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-slate-600">{formatDateTime(project.created_at)}</td>
                      <td className="whitespace-nowrap px-4 py-3 text-slate-600">{formatDateTime(project.updated_at ?? project.created_at)}</td>
                      <td className="px-4 py-3">
                        <div className="flex justify-end gap-2">
                          <Link
                            href={`/projects/${project.id}`}
                            className="inline-flex h-8 items-center justify-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 transition-colors hover:bg-slate-50 hover:text-slate-950"
                          >
                            Dashboard
                          </Link>
                          <Button
                            size="sm"
                            variant="danger"
                            disabled={deletingID !== null}
                            onClick={() => deleteProject(project)}
                          >
                            <ButtonProgress busy={deleting} busyLabel="Deleting" idleIcon={<Trash2 size={14} />}>
                              Delete
                            </ButtonProgress>
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </main>
  );
}
