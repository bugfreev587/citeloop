"use client";

import { useClerk } from "@clerk/nextjs";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, KeyRound, Loader2, LogOut, Moon, Settings, Sun } from "lucide-react";
import type { Project } from "../lib/api";
import { LAST_PROJECT_STORAGE_KEY } from "../lib/dashboard-routing";
import { applyThemeChoice, readStoredThemeChoice, saveThemeChoice, type ThemeChoice } from "../lib/theme";
import { useApi } from "../lib/use-api";
import { cx } from "./ui";

type ProjectMenuItem = Pick<Project, "id" | "name" | "slug">;

function initials(project: Pick<Project, "name" | "slug"> | null, fallback = "CL") {
  const source = (project?.name || project?.slug || fallback).trim();
  return source.slice(0, 2).toUpperCase();
}

function uniqueProjects(projects: Project[], currentProject: ProjectMenuItem) {
  const byId = new Map<string, ProjectMenuItem>();
  byId.set(currentProject.id, currentProject);
  for (const project of projects) {
    byId.set(project.id, project);
  }
  return [...byId.values()];
}

export function ProjectAccountMenu({
  project,
  projectId,
  isPlatformAdmin,
}: {
  project: Project | null;
  projectId: string;
  isPlatformAdmin: boolean;
}) {
  const api = useApi();
  const router = useRouter();
  const { openUserProfile, signOut } = useClerk();
  const rootRef = useRef<HTMLDivElement | null>(null);
  const [open, setOpen] = useState(false);
  const [projects, setProjects] = useState<Project[]>([]);
  const [loadingProjects, setLoadingProjects] = useState(false);
  const [projectError, setProjectError] = useState<string | null>(null);
  const [theme, setTheme] = useState<ThemeChoice>("light");

  useEffect(() => {
    const nextTheme = readStoredThemeChoice();
    setTheme(nextTheme);
    applyThemeChoice(nextTheme);
  }, []);

  useEffect(() => {
    if (!open) return;

    let cancelled = false;
    setLoadingProjects(true);
    setProjectError(null);
    api.listProjects()
      .then((nextProjects) => {
        if (!cancelled) {
          setProjects(nextProjects);
        }
      })
      .catch((e: any) => {
        if (!cancelled) {
          setProjectError(e.message);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoadingProjects(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [api, open]);

  useEffect(() => {
    if (!open) return;

    function onPointerDown(event: PointerEvent) {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
      }
    }

    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const projectName = project?.name ?? "CiteLoop project";
  const projectSlug = project?.slug ?? projectId;
  const currentProject = useMemo(
    () => project ?? { id: projectId, name: projectName, slug: projectSlug },
    [project, projectId, projectName, projectSlug],
  );
  const visibleProjects = useMemo(() => uniqueProjects(projects, currentProject), [currentProject, projects]);

  function openProject(nextProject: ProjectMenuItem) {
    window.localStorage.setItem(LAST_PROJECT_STORAGE_KEY, nextProject.id);
    setOpen(false);
    router.push(`/projects/${nextProject.id}`);
  }

  function chooseTheme(nextTheme: ThemeChoice) {
    setTheme(nextTheme);
    saveThemeChoice(nextTheme);
  }

  function openAccountSettings() {
    setOpen(false);
    openUserProfile();
  }

  function openAdmin() {
    setOpen(false);
    router.push(`/projects/${projectId}/admin`);
  }

  function logOut() {
    setOpen(false);
    void signOut({ redirectUrl: "/" });
  }

  return (
    <div ref={rootRef} className="relative w-[185px]">
      {open && (
        <div
          aria-label="Account and projects menu"
          className="absolute bottom-full left-0 z-30 mb-2 w-[320px] max-w-[calc(100vw-2rem)] rounded-[20px] border border-[#dfe5ec] bg-white/[0.98] px-3 py-2 text-slate-950 shadow-[0_28px_72px_rgba(55,49,43,0.18)] dark:border-slate-700 dark:bg-[#111827]/[0.98] dark:text-slate-100 dark:shadow-black/50"
        >
          <div className="pb-2">
            <div className="mb-1.5 px-1 text-[10px] font-semibold uppercase tracking-[0.16em] text-stone-400">Projects</div>
            <div className="grid gap-1">
              {visibleProjects.map((item) => {
                const current = item.id === projectId;
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => openProject(item)}
                    className={cx(
                      "flex min-h-[44px] w-full items-center gap-2.5 rounded-[11px] px-2.5 py-1.5 text-left transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/70",
                      current && "border border-slate-200 bg-[#fff8f6] hover:bg-[#fff8f6] dark:border-slate-700 dark:bg-[#1f2937] dark:hover:bg-[#1f2937]",
                    )}
                  >
                    <span
                      className={cx(
                        "project-avatar grid h-8 w-8 shrink-0 place-items-center rounded-[9px] bg-[#241f1d] text-[11px] font-semibold text-white ring-1 ring-black/5 dark:bg-slate-100 dark:text-slate-950 dark:ring-white/10",
                      )}
                    >
                      {initials(item)}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[13px] font-normal leading-4 text-slate-950 dark:text-slate-100">{item.name}</span>
                      <span className="mt-0.5 block truncate text-[11px] font-normal leading-[14px] text-stone-500 dark:text-slate-400">/{item.slug}</span>
                    </span>
                    {current && (
                      <span className="rounded-full bg-green-50 px-1.5 py-0.5 text-[10px] font-normal text-green-700 dark:bg-emerald-950 dark:text-emerald-300">Current</span>
                    )}
                  </button>
                );
              })}
              {loadingProjects && (
                <div className="flex h-[38px] items-center gap-2 rounded-[11px] px-2.5 text-[12px] font-normal text-stone-500 dark:text-slate-400">
                  <Loader2 className="animate-spin" size={14} />
                  Loading projects
                </div>
              )}
              {projectError && (
                <div className="rounded-[11px] px-2.5 py-1.5 text-[12px] font-normal leading-4 text-amber-800 dark:text-amber-300">
                  Projects unavailable: {projectError}
                </div>
              )}
            </div>
          </div>

          <div className="border-t border-slate-200 py-2.5 dark:border-slate-700">
            <button
              type="button"
              onClick={openAccountSettings}
              className="flex min-h-[40px] w-full items-center gap-2 rounded-[11px] px-2 text-left text-[13px] font-normal text-slate-950 transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec] dark:text-slate-100 dark:hover:bg-slate-800/70 dark:focus-visible:ring-slate-600"
            >
              <span className="grid h-7 w-7 place-items-center text-slate-950 dark:text-slate-100">
                <Settings size={20} strokeWidth={1.8} />
              </span>
              Account Settings
            </button>
            {isPlatformAdmin && (
              <button
                type="button"
                onClick={openAdmin}
                className="flex min-h-[40px] w-full items-center gap-2 rounded-[11px] px-2 text-left text-[13px] font-normal text-slate-950 transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec] dark:text-slate-100 dark:hover:bg-slate-800/70 dark:focus-visible:ring-slate-600"
              >
                <span className="grid h-7 w-7 place-items-center text-slate-950 dark:text-slate-100">
                  <KeyRound size={19} strokeWidth={1.8} />
                </span>
                Admin
              </button>
            )}
          </div>

          <div className="border-t border-slate-200 py-2.5 dark:border-slate-700">
            <div className="mb-1.5 px-1 text-[10px] font-semibold uppercase tracking-[0.16em] text-stone-400">Theme</div>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => chooseTheme("light")}
                className={cx(
                  "flex h-[34px] items-center justify-center gap-2 rounded-[10px] text-[13px] font-normal text-slate-950 transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec] dark:text-slate-100 dark:hover:bg-slate-800/70 dark:focus-visible:ring-slate-600",
                  theme === "light" && "bg-[#f2f2f2] hover:bg-[#f2f2f2] dark:bg-slate-800 dark:hover:bg-slate-800",
                )}
              >
                <Sun size={18} strokeWidth={1.8} />
                Light
              </button>
              <button
                type="button"
                onClick={() => chooseTheme("dark")}
                className={cx(
                  "flex h-[34px] items-center justify-center gap-2 rounded-[10px] text-[13px] font-normal text-slate-950 transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec] dark:text-slate-100 dark:hover:bg-slate-800/70 dark:focus-visible:ring-slate-600",
                  theme === "dark" && "bg-[#f2f2f2] hover:bg-[#f2f2f2] dark:bg-slate-800 dark:hover:bg-slate-800",
                )}
              >
                <Moon size={17} strokeWidth={1.8} />
                Dark
              </button>
            </div>
          </div>

          <div className="border-t border-slate-200 pt-2.5 dark:border-slate-700">
            <button
              type="button"
              onClick={logOut}
              className="flex min-h-[40px] w-full items-center gap-2 rounded-[11px] px-2 text-left text-[13px] font-normal text-slate-950 transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec] dark:text-slate-100 dark:hover:bg-slate-800/70 dark:focus-visible:ring-slate-600"
            >
              <span className="grid h-7 w-7 place-items-center text-slate-950 dark:text-slate-100">
                <LogOut size={20} strokeWidth={1.8} />
              </span>
              Log out
            </button>
          </div>
        </div>
      )}

      <button
        type="button"
        aria-label={`Open account and projects menu for ${projectName}`}
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className="flex h-[58px] w-full items-center gap-2 rounded-2xl border border-slate-100 bg-white px-2 text-left shadow-sm transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec] dark:border-slate-800 dark:bg-[#111827] dark:hover:bg-slate-800 dark:focus-visible:ring-slate-600"
      >
        <span className="project-avatar grid h-10 w-10 shrink-0 place-items-center rounded-xl bg-[#241f1d] text-xs font-semibold text-white ring-1 ring-black/5 dark:bg-slate-100 dark:text-slate-950 dark:ring-white/10">
          {initials(project)}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium text-slate-950 dark:text-slate-100">Projects</span>
          <span className="mt-0.5 block truncate text-[11px] font-normal text-stone-500 dark:text-slate-400">
            {projectName} / {projectSlug}
          </span>
        </span>
        <ChevronDown
          size={18}
          className={cx("shrink-0 text-stone-500 transition-transform dark:text-slate-400", open && "rotate-180")}
        />
      </button>
    </div>
  );
}
