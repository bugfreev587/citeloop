"use client";

import { UserButton } from "@clerk/nextjs";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Activity,
  CheckCircle2,
  CircleHelp,
  Database,
  Home,
  ListChecks,
  PenLine,
  Search,
  Send,
} from "lucide-react";
import type { DeploymentBuild, DeploymentVersion, Project } from "../lib/api";
import { cx } from "./ui";

const navItems = [
  { label: "Home", href: "", icon: Home },
  { label: "Knowledge", href: "knowledge", icon: Database },
  { label: "Topics", href: "topics", icon: ListChecks },
  { label: "Review", href: "review", icon: PenLine },
  { label: "Publishing", href: "publishing", icon: Send },
  { label: "SEO", href: "seo", icon: Search },
  { label: "Runs", href: "runs", icon: Activity },
];

function projectHref(projectId: string, leaf: string) {
  return leaf ? `/projects/${projectId}/${leaf}` : `/projects/${projectId}`;
}

function isActive(pathname: string, projectId: string, leaf: string) {
  const href = projectHref(projectId, leaf);
  return leaf ? pathname.startsWith(href) : pathname === href;
}

export function ProjectShell({
  project,
  projectId,
  apiVersion,
  webBuild,
  children,
}: {
  project: Project | null;
  projectId: string;
  apiVersion: DeploymentVersion | null;
  webBuild: DeploymentBuild;
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const projectName = project?.name ?? "CiteLoop project";
  const budget = project?.config?.monthly_budget_usd ?? 50;

  return (
    <div className="min-h-[100dvh] bg-stone-100 text-slate-950 md:h-[100dvh] md:overflow-hidden">
      <aside className="fixed left-0 top-0 z-20 hidden h-[100dvh] w-[210px] flex-col border-r border-gray-200 bg-white px-3 py-4 md:flex">
        <Link href="/" className="mb-4 flex h-9 items-center gap-2 px-2 text-sm font-bold text-slate-900">
          <span className="grid h-7 w-7 place-items-center rounded-lg bg-slate-950 text-xs text-white">CL</span>
          CiteLoop
        </Link>

        <Link
          href={`/projects/${projectId}/review`}
          className="mb-4 flex h-10 w-[185px] items-center justify-center gap-2 rounded-xl bg-gradient-to-r from-[#d93820] to-[#f4503b] px-2 text-base font-medium text-white transition-all duration-150 active:scale-[0.97]"
        >
          <CheckCircle2 size={17} strokeWidth={2} />
          Review queue
        </Link>

        <nav className="grid gap-1">
          {navItems.map((item) => {
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
          <DeploymentSnapshot apiVersion={apiVersion} webBuild={webBuild} />
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
            href="/"
            className="flex h-8 w-[185px] items-center gap-2 rounded-lg px-2 text-sm font-medium text-slate-500 hover:bg-slate-50 hover:text-slate-900"
          >
            <CircleHelp size={16} />
            Help
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
          <Link href={`/projects/${projectId}/review`} className="text-sm font-semibold text-[#d93820]">
            Review
          </Link>
        </div>
        <div className="mt-3 flex gap-2 overflow-x-auto pb-1">
          {navItems.map((item) => (
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
        </div>
      </div>

      <main className="min-h-[100dvh] w-full px-4 pb-12 pt-6 md:h-[100dvh] md:overflow-y-auto md:pl-[234px] md:pr-6 lg:pr-8">
        <div className="mx-auto w-full max-w-[1480px]">{children}</div>
      </main>
    </div>
  );
}

function DeploymentSnapshot({
  apiVersion,
  webBuild,
}: {
  apiVersion: DeploymentVersion | null;
  webBuild: DeploymentBuild;
}) {
  const apiBuild = apiVersion?.build;
  const apiMigration = apiVersion?.database?.latest_migration || apiVersion?.database?.migration_status || "unknown";

  return (
    <div className="w-[185px] rounded-xl border border-slate-200 bg-white px-3 py-2 text-[11px] leading-5 text-slate-500">
      <div className="mb-1 font-semibold uppercase text-slate-400">Deployment</div>
      <DeploymentLine label="Web" build={webBuild} />
      <DeploymentLine label="API" build={apiBuild} />
      <div className="truncate" title={apiMigration}>
        DB {apiMigration}
      </div>
    </div>
  );
}

function DeploymentLine({ label, build }: { label: string; build?: DeploymentBuild }) {
  const sha = build?.commit_sha ? build.commit_sha.slice(0, 7) : "unknown";
  const ref = build?.commit_ref || build?.environment || "unknown";

  return (
    <div className="truncate" title={`${label} ${sha} ${ref}`}>
      {label} {sha} · {ref}
    </div>
  );
}
