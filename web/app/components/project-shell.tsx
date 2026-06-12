"use client";

import { UserButton } from "@clerk/nextjs";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import {
  BookOpen,
  Database,
  FolderKanban,
  Home,
  KeyRound,
  ListChecks,
  PenLine,
  Search,
  Send,
  Settings2,
} from "lucide-react";
import { Project } from "../lib/api";
import { sidebarPrimaryAction, type NextWorkspaceActionInput, type WorkspaceAction } from "../lib/dashboard-ux-logic";
import { useApi } from "../lib/use-api";
import { cx } from "./ui";

const navItems = [
  { label: "Home", href: "", icon: Home },
  { label: "Context", href: "context", icon: Database },
  { label: "Content Plan", href: "plan", icon: ListChecks },
  { label: "Review", href: "review", icon: PenLine },
  { label: "Publish", href: "publish", icon: Send },
  { label: "Visibility", href: "visibility", icon: Search },
  { label: "Settings", href: "settings", icon: Settings2 },
  { label: "Admin", href: "admin", icon: KeyRound },
];

const adminOnlyNavLeaves = new Set(["settings", "admin"]);

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

function isProjectsActive(pathname: string) {
  return pathname === "/projects";
}

export function ProjectShell({
  project,
  projectId,
  canAccessSettings = true,
  children,
}: {
  project: Project | null;
  projectId: string;
  canAccessSettings?: boolean;
  children: React.ReactNode;
}) {
  const api = useApi();
  const pathname = usePathname();
  const projectName = project?.name ?? "CiteLoop project";
  const budget = project?.config?.monthly_budget_usd ?? 50;
  const [actionSummary, setActionSummary] = useState<NextWorkspaceActionInput | null>(null);
  // Internal routes are admin-gated server-side; hide entries that would only hit a 404.
  const visibleNav = navItems.filter((item) => !adminOnlyNavLeaves.has(item.href) || canAccessSettings);
  const primaryAction: WorkspaceAction = useMemo(() => {
    if (!actionSummary) {
      return {
        title: "Open Home",
        detail: "Start from the control center before jumping into deeper work.",
        href: `/projects/${projectId}`,
      };
    }
    return sidebarPrimaryAction(actionSummary);
  }, [actionSummary, projectId]);
  const PrimaryIcon = primaryAction.href.endsWith("/publish")
    ? Send
    : primaryAction.href.endsWith("/review")
      ? PenLine
      : primaryAction.href.endsWith("/plan")
        ? ListChecks
        : primaryAction.href.endsWith("/context")
          ? Database
          : Home;

  useEffect(() => {
    let cancelled = false;

    async function loadPrimaryAction() {
      const [profile, failedPublish, review, ready, topics] = await Promise.all([
        api.getProfile(projectId).catch(() => null),
        api.listArticles(projectId, "publish_failed").catch(() => []),
        api.listReview(projectId).catch(() => []),
        api.listDistribute(projectId).catch(() => []),
        api.listTopics(projectId).catch(() => []),
      ]);
      if (cancelled) return;
      const reviewArticles = review.flatMap((group) => group.articles);
      const profilePayload: Record<string, any> | null =
        profile?.profile && typeof profile.profile === "object" ? profile.profile : null;
      setActionSummary({
        projectId,
        hasProfile: Boolean(profile),
        contextConfirmed: Boolean(profilePayload?.context_confirmed_at || profilePayload?.confirmed_at),
        failedPublishCount: failedPublish.length,
        hasBlockedDrafts: reviewArticles.some((article) => article.qa_blocking),
        reviewCount: reviewArticles.length,
        readyCount: ready.length,
        topicsCount: topics.length,
      });
    }

    loadPrimaryAction();
    return () => {
      cancelled = true;
    };
  }, [api, projectId]);

  return (
    <div className="min-h-[100dvh] bg-stone-100 text-slate-950">
      <aside className="fixed left-0 top-0 z-20 hidden h-[100dvh] w-[210px] flex-col border-r border-gray-200 bg-white px-3 py-4 md:flex">
        <Link href="/" className="mb-4 flex h-9 items-center gap-2 px-2 text-sm font-bold text-slate-900">
          <span className="grid h-7 w-7 place-items-center rounded-lg bg-slate-950 text-xs text-white">CL</span>
          CiteLoop
        </Link>

        <Link
          href={primaryAction.href}
          title={primaryAction.detail}
          className="mb-4 flex h-10 w-[185px] items-center justify-start gap-2 overflow-hidden rounded-xl bg-gradient-to-r from-[#d93820] to-[#f4503b] px-3 text-sm font-semibold text-white transition-all duration-150 active:scale-[0.97]"
        >
          <PrimaryIcon className="shrink-0" size={17} strokeWidth={2} />
          <span className="min-w-0 truncate whitespace-nowrap">{primaryAction.title}</span>
        </Link>

        <nav className="grid gap-1">
          {visibleNav.map((item) => {
            const active = isActive(pathname, projectId, item.href);
            const Icon = item.icon;
            return (
              <Link
                key={item.label}
                href={projectHref(projectId, item.href)}
                className={cx(
                  "flex h-9 w-[185px] items-center gap-2.5 rounded-xl px-2 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-50 hover:text-slate-950",
                  active && "bg-white font-semibold text-[#d93820]",
                )}
              >
                <Icon size={17} strokeWidth={active ? 2.2 : 2} />
                {item.label}
              </Link>
            );
          })}
        </nav>

        <div className="mt-auto grid gap-2">
          <div className="w-[185px] rounded-xl border border-slate-200 bg-white px-3 py-2 text-xs text-slate-500">
            <div className="flex items-center justify-between font-semibold text-slate-700">
              <span>Budget</span>
              <span>${budget}/mo</span>
            </div>
            <div className="mt-2 h-1.5 rounded-full bg-slate-100">
              <div className="h-1.5 w-1/3 rounded-full bg-[#d93820]" />
            </div>
          </div>
          <Link
            href="/projects"
            className={cx(
              "flex h-8 w-[185px] items-center gap-2 rounded-lg px-2 text-sm font-medium text-slate-500 hover:bg-slate-50 hover:text-slate-900",
              isProjectsActive(pathname) && "bg-slate-50 font-semibold text-[#d93820]",
            )}
          >
            <FolderKanban size={16} />
            Projects
          </Link>
          <Link
            href="/docs"
            className={cx(
              "flex h-8 w-[185px] items-center gap-2 rounded-lg px-2 text-sm font-medium text-slate-500 hover:bg-slate-50 hover:text-slate-900",
              isDocsActive(pathname, projectId) && "bg-slate-50 font-semibold text-[#d93820]",
            )}
          >
            <BookOpen size={16} />
            Docs
          </Link>
          <div className="flex h-[52px] w-[185px] items-center gap-3 rounded-xl border border-slate-100 bg-white px-2 shadow-sm">
            <div className="grid h-8 w-8 place-items-center rounded-lg bg-slate-100 text-xs font-bold text-slate-700">
              {projectName.slice(0, 2).toUpperCase()}
            </div>
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold text-slate-900">{projectName}</div>
              <div className="truncate text-xs text-slate-400">/{project?.slug ?? projectId}</div>
            </div>
            <div className="ml-auto">
              <UserButton />
            </div>
          </div>
        </div>
      </aside>

      <div className="border-b border-slate-200 bg-white px-4 py-3 md:hidden">
        <div className="flex items-center justify-between">
          <Link href="/" className="font-bold text-slate-900">
            CiteLoop
          </Link>
          <Link href={primaryAction.href} className="text-sm font-semibold text-[#d93820]">
            {primaryAction.title}
          </Link>
        </div>
        <div className="mt-3 flex gap-2 overflow-x-auto pb-1">
          {visibleNav.map((item) => (
            <Link
              key={item.label}
              href={projectHref(projectId, item.href)}
              className={cx(
                "whitespace-nowrap rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-600",
                isActive(pathname, projectId, item.href) && "border-[#d93820] text-[#d93820]",
              )}
            >
              {item.label}
            </Link>
          ))}
          <Link
            href="/projects"
            className={cx(
              "whitespace-nowrap rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-600",
              isProjectsActive(pathname) && "border-[#d93820] text-[#d93820]",
            )}
          >
            Projects
          </Link>
          <Link
            href="/docs"
            className={cx(
              "whitespace-nowrap rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-600",
              isDocsActive(pathname, projectId) && "border-[#d93820] text-[#d93820]",
            )}
          >
            Docs
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
