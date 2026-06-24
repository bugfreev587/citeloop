import Link from "next/link";
import { UserButton } from "@clerk/nextjs";
import { auth } from "@clerk/nextjs/server";
import { ArrowLeft, Database, LogIn, PenLine, Send, UserPlus } from "lucide-react";
import { ProjectCreateForm } from "../project-create-form";
import { Badge, Notice } from "../components/ui";
import { clerkServerAuthConfigured, requireConfiguredClerk } from "../lib/auth-config";
import { createApi, Project } from "../lib/api";
import { ProjectManagementClient } from "./project-management-client";

export const dynamic = "force-dynamic";

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
      <div className="mx-auto max-w-5xl">
        <div className="mb-4 flex items-center justify-between gap-2">
          <Link
            href="/"
            className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50 hover:text-slate-950"
          >
            <ArrowLeft size={16} />
            Home
          </Link>
          {clerkServerAuthConfigured &&
            (signedOut ? (
              <div className="flex items-center gap-2">
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
                  Get started free
                </Link>
              </div>
            ) : (
              <UserButton />
            ))}
        </div>

        <div className="mb-8">
          <div className="mb-3 inline-flex h-8 items-center rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-600">
            Status, actions, timeline
          </div>
          <h1 className="max-w-2xl text-3xl font-bold leading-tight text-slate-950 md:text-5xl">
            CiteLoop control center
          </h1>
          <p className="mt-3 max-w-2xl text-base leading-7 text-slate-600">
            Open a project to see status, action, and timeline in one place before moving into deeper work.
          </p>
        </div>

        {error && (
          <div className="mb-4">
            {error.startsWith("401") || error.startsWith("403") ? (
              <Notice
                title="Not authorized"
                detail={`The API rejected this session (${error}). Sign in again; if it persists, the API's CLERK_SECRET_KEY does not match this app's Clerk instance.`}
                tone="amber"
              />
            ) : (
              <Notice
                title="API server unavailable"
                detail={`Could not reach the API (${error}). Start the Go service or set NEXT_PUBLIC_API_URL.`}
                tone="amber"
              />
            )}
          </div>
        )}

        <div className="grid gap-8 lg:grid-cols-[1fr_320px]">
          <section className="min-w-0">
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-xl font-bold leading-7 text-slate-900">Projects</h2>
              {!signedOut && <Badge tone="neutral">{projects.length} total</Badge>}
            </div>

            {signedOut ? (
              <div className="rounded-xl border border-slate-200 bg-white px-5 py-6">
                <div className="text-base font-bold text-slate-900">Sign in to see your projects</div>
                <p className="mt-1 text-sm text-slate-600">
                  Create an account or sign in to connect a product URL and start the content engine.
                </p>
                <div className="mt-4 flex gap-2">
                  <Link
                    href="/sign-up"
                    className="inline-flex h-9 items-center gap-2 rounded-lg bg-slate-900 px-3 text-sm font-semibold text-white transition-colors hover:bg-slate-700"
                  >
                    <UserPlus size={16} />
                    Create an account
                  </Link>
                  <Link
                    href="/sign-in"
                    className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50 hover:text-slate-950"
                  >
                    <LogIn size={16} />
                    Sign in
                  </Link>
                </div>
              </div>
            ) : (
              <ProjectManagementClient initialProjects={projects} />
            )}
          </section>

          <aside className="grid gap-4 self-start">
            {!signedOut && <ProjectCreateForm />}
            <div className="grid gap-2 rounded-xl border border-slate-200 bg-white p-4 text-sm text-slate-600">
              <div className="flex items-center gap-2 font-semibold text-slate-900">
                <Database size={16} />
                Context status
              </div>
              <div className="flex items-center gap-2 font-semibold text-slate-900">
                <PenLine size={16} />
                Next action
              </div>
              <div className="flex items-center gap-2 font-semibold text-slate-900">
                <Send size={16} />
                Publish timeline
              </div>
            </div>
          </aside>
        </div>
      </div>
    </main>
  );
}
