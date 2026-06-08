import Link from "next/link";
import { UserButton } from "@clerk/nextjs";
import { auth } from "@clerk/nextjs/server";
import { ArrowRight, Database, PenLine, Send } from "lucide-react";
import { ProjectCreateForm } from "./project-create-form";
import { Badge, EmptyState, Notice } from "./components/ui";
import { createApi, Project } from "./lib/api";

export default async function Home() {
  const { getToken } = await auth();
  const token = await getToken();
  const api = createApi({ token });
  let projects: Project[] = [];
  let error: string | null = null;
  try {
    projects = await api.listProjects();
  } catch (e: any) {
    error = e.message;
  }

  return (
    <main className="min-h-[100dvh] bg-stone-100 px-4 py-8 text-slate-950">
      <div className="mx-auto grid max-w-5xl gap-8 lg:grid-cols-[1fr_320px]">
        <section className="min-w-0">
          <div className="mb-8">
            <div className="mb-4 flex justify-end">
              <UserButton />
            </div>
            <div className="mb-3 inline-flex h-8 items-center rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-600">
              SEO + GEO content engine
            </div>
            <h1 className="max-w-2xl text-3xl font-bold leading-tight text-slate-950 md:text-5xl">
              CiteLoop service console
            </h1>
            <p className="mt-3 max-w-2xl text-base leading-7 text-slate-600">
              Connect a product URL, build the profile, and review SEO + AEO progress from one dashboard.
            </p>
          </div>

          {error && (
            <div className="mb-4">
              <Notice
                title="API server unavailable"
                detail={`Could not reach the API (${error}). Start the Go service or set NEXT_PUBLIC_API_URL.`}
                tone="amber"
              />
            </div>
          )}

          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-xl font-bold leading-7 text-slate-900">Projects</h2>
            <Badge tone="neutral">{projects.length} total</Badge>
          </div>

          <div className="grid gap-3">
            {projects.map((project) => (
              <Link
                key={project.id}
                href={`/projects/${project.id}`}
                className="group flex min-h-[74px] items-center justify-between rounded-xl border border-slate-200 bg-white px-4 py-3 transition-colors hover:bg-slate-50"
              >
                <div className="min-w-0">
                  <div className="truncate text-base font-bold text-slate-900">{project.name}</div>
                  <div className="mt-1 truncate text-sm text-slate-500">/{project.slug}</div>
                </div>
                <ArrowRight
                  className="text-slate-400 transition-transform group-hover:translate-x-0.5 group-hover:text-[#d93820]"
                  size={18}
                />
              </Link>
            ))}
            {!error && projects.length === 0 && (
              <EmptyState
                title="No projects yet"
                detail="Connect your service URL to start the onboarding workflow."
              />
            )}
          </div>
        </section>

        <aside className="grid gap-4 self-start">
          <ProjectCreateForm />
          <div className="grid gap-2 rounded-xl border border-slate-200 bg-white p-4 text-sm text-slate-600">
            <div className="flex items-center gap-2 font-semibold text-slate-900">
              <Database size={16} />
              Knowledge
            </div>
            <div className="flex items-center gap-2 font-semibold text-slate-900">
              <PenLine size={16} />
              Human review gate
            </div>
            <div className="flex items-center gap-2 font-semibold text-slate-900">
              <Send size={16} />
              Manual distribution
            </div>
          </div>
        </aside>
      </div>
    </main>
  );
}
