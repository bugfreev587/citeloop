"use client";

import { useClerk } from "@clerk/nextjs";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, KeyRound, Loader2, LogOut, Moon, Settings, Sun } from "lucide-react";
import type { Project } from "../lib/api";
import { LAST_PROJECT_STORAGE_KEY } from "../lib/dashboard-routing";
import { useApi } from "../lib/use-api";
import { cx } from "./ui";

type ThemeChoice = "light" | "dark";
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
    const saved = window.localStorage.getItem("citeloop:theme");
    if (saved === "dark" || saved === "light") {
      setTheme(saved);
      document.documentElement.dataset.theme = saved;
    }
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
    window.localStorage.setItem("citeloop:theme", nextTheme);
    document.documentElement.dataset.theme = nextTheme;
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
          className="absolute bottom-full left-0 z-30 mb-3 w-[436px] max-w-[calc(100vw-2rem)] rounded-[26px] border border-[#dfe5ec] bg-white/[0.98] px-4 py-3 text-slate-950 shadow-[0_34px_90px_rgba(55,49,43,0.21)]"
        >
          <div className="pb-3">
            <div className="mb-2 px-1 text-[12px] font-semibold uppercase tracking-[0.14em] text-stone-400">Projects</div>
            <div className="grid gap-1">
              {visibleProjects.map((item) => {
                const current = item.id === projectId;
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => openProject(item)}
                    className={cx(
                      "flex min-h-12 w-full items-center gap-3 rounded-[13px] px-3 py-2 text-left transition-colors hover:bg-slate-50",
                      current && "border border-slate-200 bg-[#fff8f6] hover:bg-[#fff8f6]",
                    )}
                  >
                    <span
                      className={cx(
                        "grid h-9 w-9 shrink-0 place-items-center rounded-[10px] bg-stone-100 text-xs font-semibold text-stone-700",
                        current && "bg-[#241f1d] text-white",
                      )}
                    >
                      {initials(item)}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[16px] font-medium leading-5 text-slate-950">{item.name}</span>
                      <span className="mt-0.5 block truncate text-[13px] font-normal text-stone-500">/{item.slug}</span>
                    </span>
                    {current && (
                      <span className="rounded-full bg-green-50 px-2 py-0.5 text-[11px] font-medium text-green-700">Current</span>
                    )}
                  </button>
                );
              })}
              {loadingProjects && (
                <div className="flex h-11 items-center gap-2 rounded-[13px] px-3 text-sm font-normal text-stone-500">
                  <Loader2 className="animate-spin" size={16} />
                  Loading projects
                </div>
              )}
              {projectError && (
                <div className="rounded-[13px] px-3 py-2 text-sm font-normal leading-5 text-amber-800">
                  Projects unavailable: {projectError}
                </div>
              )}
            </div>
          </div>

          <div className="border-t border-slate-200 py-3">
            <button
              type="button"
              onClick={openAccountSettings}
              className="flex min-h-[52px] w-full items-center gap-3 rounded-[13px] px-3 text-left text-[17px] font-medium text-slate-950 transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec]"
            >
              <span className="grid h-9 w-9 place-items-center text-slate-950">
                <Settings size={25} strokeWidth={1.8} />
              </span>
              Account Settings
            </button>
            {isPlatformAdmin && (
              <button
                type="button"
                onClick={openAdmin}
                className="flex min-h-[52px] w-full items-center gap-3 rounded-[13px] px-3 text-left text-[17px] font-medium text-slate-950 transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec]"
              >
                <span className="grid h-9 w-9 place-items-center text-slate-950">
                  <KeyRound size={23} strokeWidth={1.8} />
                </span>
                Admin
              </button>
            )}
          </div>

          <div className="border-t border-slate-200 py-3">
            <div className="mb-2 px-1 text-[12px] font-semibold uppercase tracking-[0.14em] text-stone-400">Theme</div>
            <div className="grid grid-cols-2 gap-3">
              <button
                type="button"
                onClick={() => chooseTheme("light")}
                className={cx(
                  "flex h-11 items-center justify-center gap-2.5 rounded-xl text-[17px] font-medium text-slate-950 transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec]",
                  theme === "light" && "bg-stone-100 hover:bg-stone-100",
                )}
              >
                <Sun size={22} strokeWidth={1.8} />
                Light
              </button>
              <button
                type="button"
                onClick={() => chooseTheme("dark")}
                className={cx(
                  "flex h-11 items-center justify-center gap-2.5 rounded-xl text-[17px] font-medium text-slate-950 transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec]",
                  theme === "dark" && "bg-stone-100 hover:bg-stone-100",
                )}
              >
                <Moon size={20} strokeWidth={1.8} />
                Dark
              </button>
            </div>
          </div>

          <div className="border-t border-slate-200 pt-3">
            <button
              type="button"
              onClick={logOut}
              className="flex min-h-[52px] w-full items-center gap-3 rounded-[13px] px-3 text-left text-[17px] font-medium text-slate-950 transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec]"
            >
              <span className="grid h-9 w-9 place-items-center text-slate-950">
                <LogOut size={24} strokeWidth={1.8} />
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
        className="flex h-[58px] w-full items-center gap-2 rounded-2xl border border-slate-100 bg-white px-2 text-left shadow-sm transition-colors hover:bg-slate-50 outline-none focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec]"
      >
        <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl bg-[#241f1d] text-xs font-semibold text-white">
          {initials(project)}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium text-slate-950">Projects</span>
          <span className="mt-0.5 block truncate text-[11px] font-normal text-stone-500">
            {projectName} / {projectSlug}
          </span>
        </span>
        <ChevronDown
          size={18}
          className={cx("shrink-0 text-stone-500 transition-transform", open && "rotate-180")}
        />
      </button>
    </div>
  );
}
