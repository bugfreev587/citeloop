"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import {
  BookOpen,
  Database,
  Home,
  ListChecks,
  PenLine,
  Search,
  Send,
  Settings2,
  Target,
} from "lucide-react";
import { ProjectAccountMenu } from "./project-account-menu";
import { Project } from "../lib/api";
import { ProjectVisitRecorder } from "../project-visit-recorder";
import { useApi } from "../lib/use-api";
import { cx } from "./ui";

const navSections = [
  {
    id: "primary",
    label: null,
    items: [
      { label: "Home", href: "", icon: Home },
      { label: "Context", href: "context", icon: Database },
    ],
  },
  {
    id: "intelligence",
    label: "Intelligence",
    items: [{ label: "Analysis", href: "analysis", icon: Target }],
  },
  {
    id: "execution",
    label: "Execution",
    items: [
      { label: "Content Plan", href: "plan", icon: ListChecks },
      { label: "Review", href: "review", icon: PenLine },
      { label: "Publish", href: "publish", icon: Send },
    ],
  },
  {
    id: "outcomes",
    label: "Outcomes",
    items: [{ label: "Results", href: "results", icon: Search }],
  },
];

function projectHref(projectId: string, leaf: string) {
  return leaf ? `/projects/${projectId}/${leaf}` : `/projects/${projectId}`;
}

function isActive(pathname: string, projectId: string, leaf: string) {
  const href = projectHref(projectId, leaf);
  return leaf ? pathname.startsWith(href) : pathname === href;
}

function isDocsActive(pathname: string, projectId: string) {
  return pathname === "/docs" || pathname.startsWith("/docs/") || pathname.startsWith(`/projects/${projectId}/docs`);
}

export function ProjectShell({
  project,
  projectId,
  children,
}: {
  project: Project | null;
  projectId: string;
  children: React.ReactNode;
}) {
  const api = useApi();
  const pathname = usePathname();
  const budget = project?.config?.monthly_budget_usd ?? 50;
  const [isPlatformAdmin, setIsPlatformAdmin] = useState(false);
  useEffect(() => {
    let cancelled = false;
    api
      .getMe()
      .then((me) => {
        if (!cancelled) setIsPlatformAdmin(Boolean(me?.is_admin));
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [api]);
  const visibleNavSections = navSections
    .map((section) => ({
      ...section,
      items: section.items,
    }))
    .filter((section) => section.items.length > 0);
  const visibleNav = visibleNavSections.flatMap((section) => section.items);

  return (
    <div className="min-h-[100dvh] bg-stone-100 text-slate-950 dark:bg-[#0f1117] dark:text-slate-100">
      <ProjectVisitRecorder projectId={projectId} />
      <aside className="fixed left-0 top-0 z-20 hidden h-[100dvh] w-[210px] flex-col border-r border-gray-200 bg-white px-3 py-4 dark:border-slate-800 dark:bg-[#111827] md:flex">
        <Link href="/" className="mb-4 flex h-9 items-center gap-2 px-2 text-sm font-bold text-slate-900 dark:text-slate-100">
          <span className="grid h-7 w-7 place-items-center rounded-lg bg-slate-950 text-xs text-white dark:bg-slate-100 dark:text-slate-950">CL</span>
          CiteLoop
        </Link>

        <div className="mb-4 h-px w-[185px] bg-slate-200 dark:bg-slate-800" />

        <nav className="grid gap-5">
          {visibleNavSections.map((section, sectionIndex) => (
            <div key={section.id} className="grid gap-1">
              {section.label && (
                <div className={cx("px-2 text-[10px] font-bold tracking-[0.18em] text-slate-400", sectionIndex > 0 && "pt-1")}>
                  {section.label}
                </div>
              )}
              {section.items.map((item) => {
                const active = isActive(pathname, projectId, item.href);
                const Icon = item.icon;
                return (
                  <Link
                    key={item.label}
                    href={projectHref(projectId, item.href)}
                    className={cx(
                      "flex h-9 w-[185px] items-center gap-2.5 rounded-xl px-2 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-50 hover:text-slate-950 dark:text-slate-400 dark:hover:bg-slate-800/70 dark:hover:text-slate-100",
                      active && "bg-[#fff5f2] font-semibold text-[#d93820] dark:bg-[#2a1814] dark:text-[#ff8a72]",
                    )}
                  >
                    <Icon size={17} strokeWidth={active ? 2.2 : 2} />
                    {item.label}
                  </Link>
                );
              })}
            </div>
          ))}
        </nav>

        <div className="mt-auto grid gap-2">
          <div className="w-[185px] rounded-xl border border-slate-200 bg-white px-3 py-2 text-xs text-slate-500 dark:border-slate-800 dark:bg-[#151b26] dark:text-slate-400">
            <div className="flex items-center justify-between font-semibold text-slate-700 dark:text-slate-200">
              <span>Budget</span>
              <span>${budget}/mo</span>
            </div>
            <div className="mt-2 h-1.5 rounded-full bg-slate-100 dark:bg-slate-800">
              <div className="h-1.5 w-1/3 rounded-full bg-[#d93820]" />
            </div>
          </div>
          <Link
            href="/docs"
            className={cx(
              "flex h-8 w-[185px] items-center gap-2 rounded-lg px-2 text-sm font-medium text-slate-500 hover:bg-slate-50 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800/70 dark:hover:text-slate-100",
              isDocsActive(pathname, projectId) && "bg-slate-50 font-semibold text-[#d93820] dark:bg-slate-800 dark:text-[#ff8a72]",
            )}
          >
            <BookOpen size={16} />
            Docs
          </Link>
          <Link
            href={`/projects/${projectId}/settings`}
            className={cx(
              "flex h-8 w-[185px] items-center gap-2 rounded-lg px-2 text-sm font-medium text-slate-500 hover:bg-slate-50 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800/70 dark:hover:text-slate-100",
              isActive(pathname, projectId, "settings") && "bg-slate-50 font-semibold text-[#d93820] dark:bg-slate-800 dark:text-[#ff8a72]",
            )}
          >
            <Settings2 size={16} />
            Settings
          </Link>
          <ProjectAccountMenu project={project} projectId={projectId} isPlatformAdmin={isPlatformAdmin} />
        </div>
      </aside>

      <div className="border-b border-slate-200 bg-white px-4 py-3 dark:border-slate-800 dark:bg-[#111827] md:hidden">
        <div className="flex items-center justify-between">
          <Link href="/" className="font-bold text-slate-900 dark:text-slate-100">
            CiteLoop
          </Link>
        </div>
        <div className="mt-3 flex gap-2 overflow-x-auto pb-1">
          {visibleNav.map((item) => (
            <Link
              key={item.label}
              href={projectHref(projectId, item.href)}
              className={cx(
                "whitespace-nowrap rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-600 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300",
                isActive(pathname, projectId, item.href) && "border-[#d93820] text-[#d93820] dark:text-[#ff8a72]",
              )}
            >
              {item.label}
            </Link>
          ))}
          {isPlatformAdmin && (
            <Link
              href={`/projects/${projectId}/admin`}
              className={cx(
                "whitespace-nowrap rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-600 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300",
                isActive(pathname, projectId, "admin") && "border-[#d93820] text-[#d93820] dark:text-[#ff8a72]",
              )}
            >
              Admin
            </Link>
          )}
          <Link
            href="/docs"
            className={cx(
              "whitespace-nowrap rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-600 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300",
              isDocsActive(pathname, projectId) && "border-[#d93820] text-[#d93820] dark:text-[#ff8a72]",
            )}
          >
            Docs
          </Link>
          <Link
            href={`/projects/${projectId}/settings`}
            className={cx(
              "whitespace-nowrap rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-600 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300",
              isActive(pathname, projectId, "settings") && "border-[#d93820] text-[#d93820] dark:text-[#ff8a72]",
            )}
          >
            Settings
          </Link>
        </div>
      </div>

      <main
        className="mx-auto min-h-[100dvh] max-w-[1560px] px-4 pb-12 pt-8 md:pl-[220px] md:pr-8"
      >
        <div className="mx-auto max-w-[1320px]">{children}</div>
      </main>
    </div>
  );
}
