import { UserButton } from "@clerk/nextjs";
import { auth } from "@clerk/nextjs/server";
import Link from "next/link";
import { ArrowLeft, LogIn, UserPlus } from "lucide-react";
import { ProjectCreateForm } from "../project-create-form";
import { Badge, Notice } from "../components/ui";
import { clerkServerAuthConfigured, requireConfiguredClerk } from "../lib/auth-config";
import { createApi, type Project } from "../lib/api";
import { ProjectManagementClient } from "./project-management-client";

export default async function ProjectsPage() {
  requireConfiguredClerk();

  let token: string | null = null;
  if (clerkServerAuthConfigured) {
    const { getToken } = await auth();
    token = await getToken();
  }
  const signedOut = clerkServerAuthConfigured && !token;

  const api = createApi(token ? { token } : undefined);
  let projects: Project[] = [];
  let error: string | null = null;
  if (!signedOut) {
    try {
      projects = await api.listProjects();
    } catch (e: any) {
      error = e.message;
    }
  }

  return (
    <main className="min-h-[100dvh] bg-stone-100 px-4 py-8 text-slate-950">
      <div className="mx-auto grid max-w-6xl gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
        <section className="min-w-0">
          <div className="mb-6 flex items-center justify-between gap-3">
            <Link
              href="/"
              className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50 hover:text-slate-950"
            >
              <ArrowLeft size={16} />
              Home
            </Link>
            {clerkServerAuthConfigured &&
              (signedOut ? (
                <div className="flex gap-2">
                  <Link
                    href="/sign-in"
                    className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50 hover:text-slate-950"
                  >
                    <LogIn size={16} />
                    Sign in
                  </Link>
                  <Link
                    href="/sign-up"
                    className="inline-flex h-9 items-center gap-2 rounded-lg bg-slate-900 px-3 text-sm font-semibold text-white transition-colors hover:bg-slate-700"
                  >
                    <UserPlus size={16} />
                    Create account
                  </Link>
                </div>
              ) : (
                <UserButton />
              ))}
          </div>

          <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
            <div>
              <div className="mb-2 inline-flex h-7 items-center rounded-md border border-slate-200 bg-white px-2 text-xs font-bold uppercase tracking-[0.04em] text-slate-500">
                Project management
              </div>
              <h1 className="text-3xl font-bold leading-tight text-slate-950 md:text-4xl">Projects</h1>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-600">
                Manage every product domain connected to this account.
              </p>
            </div>
            {!signedOut && <Badge tone="neutral">{projects.length} total</Badge>}
          </div>

          {error && (
            <div className="mb-4">
              <Notice title="Could not load projects" detail={error} tone="amber" />
            </div>
          )}

          {signedOut ? (
            <div className="rounded-xl border border-slate-200 bg-white px-5 py-6">
              <div className="text-base font-bold text-slate-900">Sign in to manage projects</div>
              <p className="mt-1 text-sm text-slate-600">
                Use your CiteLoop account to add product domains or permanently delete project data.
              </p>
            </div>
          ) : (
            <ProjectManagementClient initialProjects={projects} />
          )}
        </section>

        <aside className="self-start">
          {!signedOut && <ProjectCreateForm />}
        </aside>
      </div>
    </main>
  );
}
