import Link from "next/link";
import { UserButton } from "@clerk/nextjs";
import { auth } from "@clerk/nextjs/server";
import { ArrowRight, BarChart3, CheckCircle2, FileText, PenLine, Search, Send } from "lucide-react";
import { JoinWithGoogleButton, LandingDashboardButton } from "./landing-auth-actions";
import { clerkServerAuthConfigured, requireConfiguredClerk } from "./lib/auth-config";
import { createApi, Project } from "./lib/api";

export const dynamic = "force-dynamic";

const workflowSteps = [
  {
    title: "Connect your site data",
    detail: "Bring Search Console context and site evidence into one operating view.",
    icon: Search,
  },
  {
    title: "Prioritize the next wins",
    detail: "Find pages worth creating or improving before the content queue fills up.",
    icon: BarChart3,
  },
  {
    title: "Review, publish, measure",
    detail: "Move every opportunity through draft review, publishing, and results tracking.",
    icon: Send,
  },
];

export default async function Home() {
  requireConfiguredClerk();

  let token: string | null = null;
  if (clerkServerAuthConfigured) {
    const { getToken } = await auth();
    token = await getToken();
  }
  const signedOut = clerkServerAuthConfigured && !token;

  const api = createApi(token ? { token } : undefined);
  let projects: Project[] = [];
  let projectPrefetchFailed = false;
  if (!signedOut) {
    try {
      projects = await api.listProjects();
    } catch {
      projectPrefetchFailed = true;
    }
  }

  return (
    <main className="min-h-[100dvh] overflow-hidden bg-[#f7f2ea] text-slate-950">
      <div className="mx-auto flex min-h-[100dvh] w-full max-w-7xl flex-col px-4 py-5 sm:px-6 lg:px-8">
        <header className="flex flex-wrap items-center justify-between gap-3">
          <Link href="/" className="flex h-10 items-center gap-2 text-sm font-bold text-slate-950">
            <span className="grid h-8 w-8 place-items-center rounded-lg bg-slate-950 text-xs text-white">CL</span>
            CiteLoop
          </Link>

          {clerkServerAuthConfigured &&
            (signedOut ? (
              <div className="flex flex-wrap items-center justify-end gap-2">
                <JoinWithGoogleButton />
                <Link
                  href="/sign-up"
                  className="inline-flex h-10 items-center justify-center gap-2 rounded-lg bg-slate-950 px-4 text-sm font-semibold text-white transition-colors hover:bg-slate-800 active:scale-[0.98]"
                >
                  Start for free
                  <ArrowRight size={16} aria-hidden="true" />
                </Link>
              </div>
            ) : (
              <div className="flex items-center justify-end gap-3">
                <LandingDashboardButton initialProjects={projects} projectPrefetchFailed={projectPrefetchFailed} />
                <UserButton />
              </div>
            ))}
        </header>

        <section className="grid flex-1 items-center gap-10 py-14 lg:grid-cols-[minmax(0,1fr)_minmax(390px,0.86fr)] lg:py-10">
          <div className="max-w-3xl">
            <div className="mb-5 inline-flex h-8 items-center rounded-lg border border-[#efcfc4] bg-white/70 px-3 text-xs font-bold tracking-[0.18em] text-[#d93820]">
              SEO + GEO AUTOPILOT
            </div>
            <h1 className="max-w-3xl text-4xl font-black leading-[0.98] tracking-tight text-slate-950 md:text-6xl">
              The content engine that already knows your site.
            </h1>
            <p className="mt-5 max-w-2xl text-base leading-7 text-stone-700 md:text-lg">
              Connect your site data, find the pages worth creating or improving, and move each opportunity through
              review, publishing, and measurement.
            </p>
            <div className="mt-7 flex flex-wrap gap-3">
              {clerkServerAuthConfigured && signedOut && (
                <>
                  <Link
                    href="/sign-up"
                    className="inline-flex h-11 items-center justify-center gap-2 rounded-lg bg-slate-950 px-5 text-sm font-semibold text-white transition-colors hover:bg-slate-800 active:scale-[0.98]"
                  >
                    Start for free
                    <ArrowRight size={16} aria-hidden="true" />
                  </Link>
                  <JoinWithGoogleButton className="h-11 px-5" />
                </>
              )}
              {clerkServerAuthConfigured && !signedOut && (
                <LandingDashboardButton
                  initialProjects={projects}
                  projectPrefetchFailed={projectPrefetchFailed}
                  className="h-11 px-5"
                />
              )}
            </div>
          </div>

          <div className="relative">
            <div className="rounded-2xl border border-stone-200 bg-white p-4 shadow-[0_24px_70px_-34px_rgba(39,33,24,0.45)]">
              <div className="mb-4 flex items-center justify-between border-b border-stone-100 pb-3">
                <div>
                  <div className="text-xs font-bold tracking-[0.16em] text-stone-400">NEXT ACTION</div>
                  <div className="mt-1 text-lg font-black text-slate-950">Review 4 priority opportunities</div>
                </div>
                <div className="rounded-lg bg-[#fff5f2] px-3 py-1 text-sm font-bold text-[#d93820]">Live</div>
              </div>

              <div className="grid gap-3">
                <div className="rounded-xl border border-stone-200 bg-stone-50 p-4">
                  <div className="flex items-start gap-3">
                    <div className="grid h-9 w-9 place-items-center rounded-lg bg-slate-950 text-white">
                      <Search size={17} aria-hidden="true" />
                    </div>
                    <div>
                      <div className="text-sm font-bold text-slate-950">Search Console context connected</div>
                      <p className="mt-1 text-sm leading-6 text-stone-600">
                        CiteLoop is reading the queries, pages, and gaps that matter this week.
                      </p>
                    </div>
                  </div>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="rounded-xl border border-stone-200 bg-white p-4">
                    <div className="mb-3 flex items-center gap-2 text-sm font-bold text-slate-950">
                      <PenLine size={16} aria-hidden="true" />
                      Draft review
                    </div>
                    <div className="h-2 rounded-full bg-stone-100">
                      <div className="h-2 w-[68%] rounded-full bg-[#d93820]" />
                    </div>
                    <div className="mt-2 text-xs font-semibold text-stone-500">7 drafts checked against evidence</div>
                  </div>

                  <div className="rounded-xl border border-stone-200 bg-white p-4">
                    <div className="mb-3 flex items-center gap-2 text-sm font-bold text-slate-950">
                      <FileText size={16} aria-hidden="true" />
                      Publishing
                    </div>
                    <div className="grid gap-2 text-xs font-semibold text-stone-600">
                      <div className="flex items-center justify-between">
                        <span>Scheduled pages</span>
                        <span className="text-slate-950">4</span>
                      </div>
                      <div className="flex items-center justify-between">
                        <span>Measuring</span>
                        <span className="text-slate-950">12</span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="rounded-xl bg-slate-950 p-4 text-white">
                  <div className="flex items-center gap-2 text-sm font-bold">
                    <CheckCircle2 size={17} className="text-[#f4503b]" aria-hidden="true" />
                    From signal to shipped content
                  </div>
                  <p className="mt-2 text-sm leading-6 text-stone-300">
                    Priorities stay tied to evidence while the loop moves through review, publishing, and results.
                  </p>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section className="grid gap-3 pb-10 md:grid-cols-3">
          {workflowSteps.map((step) => {
            const Icon = step.icon;
            return (
              <div key={step.title} className="rounded-xl border border-stone-200 bg-white/70 p-5">
                <div className="mb-4 grid h-9 w-9 place-items-center rounded-lg bg-white text-[#d93820] shadow-sm">
                  <Icon size={17} aria-hidden="true" />
                </div>
                <h2 className="text-base font-black text-slate-950">{step.title}</h2>
                <p className="mt-2 text-sm leading-6 text-stone-600">{step.detail}</p>
              </div>
            );
          })}
        </section>

        <footer className="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-stone-200 py-5 text-sm text-stone-500">
          <span>CiteLoop</span>
          <Link href="/privacy" className="font-semibold text-stone-600 hover:text-slate-950">
            Privacy
          </Link>
          <Link href="/terms" className="font-semibold text-stone-600 hover:text-slate-950">
            Terms
          </Link>
        </footer>
      </div>
    </main>
  );
}
