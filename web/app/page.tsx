import Link from "next/link";
import { UserButton } from "@clerk/nextjs";
import { auth } from "@clerk/nextjs/server";
import { ArrowRight } from "lucide-react";
import { JoinWithGoogleButton, LandingDashboardButton } from "./landing-auth-actions";
import { clerkServerAuthConfigured, requireConfiguredClerk } from "./lib/auth-config";
import { createApi, Project } from "./lib/api";

export const dynamic = "force-dynamic";

const loopMoves = [
  ["Discover", "Read the domain, GSC property, page inventory, queries, and technical signals."],
  ["Ship", "Create briefs, drafts, metadata fixes, page updates, and publishing jobs."],
  ["Learn", "Measure outcome windows and feed results back into the next growth plan."],
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
    <main className="min-h-[100dvh] overflow-hidden bg-[#f8f5ef] text-slate-950">
      <style>{`
        .landing-outer-track {
          transform-origin: 300px 300px;
          animation: landing-slow-spin 24s linear infinite;
        }

        .landing-orbit-dot {
          transform-origin: 300px 300px;
          animation: landing-fast-spin 7.5s linear infinite;
        }

        .landing-orbit-dot-secondary {
          animation-delay: -2.5s;
        }

        .landing-orbit-dot-tertiary {
          animation-delay: -5s;
        }

        .landing-segment {
          transform-origin: 300px 300px;
          animation: landing-segment-focus 9s cubic-bezier(.16,1,.3,1) infinite;
        }

        .landing-segment-ship {
          animation-delay: 3s;
        }

        .landing-segment-learn {
          animation-delay: 6s;
        }

        .landing-center-pulse {
          transform-origin: 300px 300px;
          animation: landing-center-breathe 4.5s cubic-bezier(.16,1,.3,1) infinite;
        }

        @keyframes landing-slow-spin {
          to { transform: rotate(360deg); }
        }

        @keyframes landing-fast-spin {
          to { transform: rotate(360deg); }
        }

        @keyframes landing-segment-focus {
          0%, 24%, 100% {
            filter: saturate(.94) brightness(.98);
            transform: scale(1);
          }
          9%, 16% {
            filter: saturate(1.08) brightness(1.04);
            transform: scale(1.012);
          }
        }

        @keyframes landing-center-breathe {
          0%, 100% { transform: scale(1); }
          50% { transform: scale(1.018); }
        }

        @media (prefers-reduced-motion: reduce) {
          .landing-outer-track,
          .landing-orbit-dot,
          .landing-segment,
          .landing-center-pulse {
            animation: none;
          }
        }
      `}</style>

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

        <section className="grid flex-1 items-center gap-10 py-12 lg:grid-cols-[minmax(0,430px)_minmax(0,1fr)] lg:gap-14 lg:py-9">
          <div className="max-w-xl">
            <div className="mb-5 inline-flex h-8 items-center rounded-full border border-[#efcfc4] bg-white/80 px-3 text-xs font-black tracking-[0.16em] text-[#d93820]">
              SEO/GEO GROWTH LOOP
            </div>
            <h1 className="text-4xl font-black leading-[1.02] tracking-tight text-slate-950 md:text-6xl">
              Turn your website into a self-improving growth loop.
            </h1>
            <p className="mt-5 max-w-[58ch] text-base leading-7 text-stone-700 md:text-lg">
              Connect your domain, Search Console, and publishing target. CiteLoop discovers what to improve, ships the
              work safely, and measures what moved.
            </p>
            <div className="mt-7 flex flex-wrap gap-3">
              {clerkServerAuthConfigured && signedOut && (
                <>
                  <Link
                    href="/sign-up"
                    className="inline-flex h-11 items-center justify-center gap-2 rounded-lg bg-slate-950 px-5 text-sm font-semibold text-white transition-colors hover:bg-slate-800 active:scale-[0.98]"
                  >
                    Start with your domain
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
            <div className="mt-7 grid max-w-xl gap-2 text-sm font-semibold text-stone-600 sm:grid-cols-3">
              <div className="rounded-lg border border-stone-200 bg-white/70 px-3 py-2">Domain</div>
              <div className="rounded-lg border border-stone-200 bg-white/70 px-3 py-2">Search Console</div>
              <div className="rounded-lg border border-stone-200 bg-white/70 px-3 py-2">Publisher</div>
            </div>
          </div>

          <div className="relative mx-auto w-full max-w-[650px]" aria-label="CiteLoop SEO GEO flywheel">
            <svg className="h-auto w-full overflow-visible" viewBox="0 0 600 600" role="img" aria-labelledby="flywheel-title flywheel-desc">
              <title id="flywheel-title">CiteLoop Growth Loop flywheel</title>
              <desc id="flywheel-desc">
                Domain and Search Console signals become opportunities, published assets, and measured outcomes.
              </desc>
              <defs>
                <path id="domain-gsc-label" d="M 165 105 A 230 230 0 0 1 435 105" />
                <path id="opportunities-label" d="M 515 194 A 245 245 0 0 1 500 415" />
                <path id="published-assets-label" d="M 430 548 A 265 265 0 0 1 170 548" />
                <path id="measured-outcomes-label" d="M 72 412 A 245 245 0 0 1 86 195" />
              </defs>

              <g className="landing-outer-track">
                <circle
                  cx="300"
                  cy="300"
                  r="278"
                  fill="none"
                  stroke="#dbe5ef"
                  strokeDasharray="512 68 512 68 512 68"
                  strokeLinecap="round"
                  strokeWidth="46"
                />
                <path d="M 513 151 L 551 151 L 535 191 Z" fill="#dbe5ef" opacity=".96" />
                <path d="M 485 514 L 521 535 L 480 551 Z" fill="#dbe5ef" opacity=".96" />
                <path d="M 52 374 L 52 330 L 87 356 Z" fill="#dbe5ef" opacity=".96" />
              </g>

              <text className="fill-[#33465a] text-[27px] font-black">
                <textPath href="#domain-gsc-label" startOffset="50%" textAnchor="middle">
                  Domain + GSC
                </textPath>
              </text>
              <text className="fill-[#33465a] text-[27px] font-black">
                <textPath href="#opportunities-label" startOffset="50%" textAnchor="middle">
                  Opportunities
                </textPath>
              </text>
              <text className="fill-[#33465a] text-[27px] font-black">
                <textPath href="#published-assets-label" startOffset="50%" textAnchor="middle">
                  Published assets
                </textPath>
              </text>
              <text className="fill-[#33465a] text-[27px] font-black">
                <textPath href="#measured-outcomes-label" startOffset="50%" textAnchor="middle">
                  Measured outcomes
                </textPath>
              </text>

              <path
                className="landing-segment"
                d="M 100.8 185 A 230 230 0 0 1 499.2 185 L 405.7 239 A 122 122 0 0 0 194.3 239 Z"
                fill="#f3bd5b"
                stroke="#26384b"
                strokeLinejoin="round"
                strokeWidth="5"
              />
              <path
                className="landing-segment landing-segment-ship"
                d="M 499.2 185 A 230 230 0 0 1 300 530 L 300 422 A 122 122 0 0 0 405.7 239 Z"
                fill="#0fb8a0"
                stroke="#26384b"
                strokeLinejoin="round"
                strokeWidth="5"
              />
              <path
                className="landing-segment landing-segment-learn"
                d="M 300 530 A 230 230 0 0 1 100.8 185 L 194.3 239 A 122 122 0 0 0 300 422 Z"
                fill="#0da2b3"
                stroke="#26384b"
                strokeLinejoin="round"
                strokeWidth="5"
              />

              <g className="drop-shadow-[0_16px_32px_rgba(15,23,42,0.16)]">
                <circle
                  className="landing-center-pulse"
                  cx="300"
                  cy="300"
                  r="126"
                  fill="#ff7159"
                  stroke="#26384b"
                  strokeWidth="22"
                />
                <text x="300" y="292" textAnchor="middle" className="fill-white text-[39px] font-black">
                  Growth
                </text>
                <text x="300" y="322" textAnchor="middle" className="fill-white/80 text-[15px] font-bold">
                  Loop
                </text>
              </g>

              <text x="300" y="185" textAnchor="middle" transform="rotate(5 300 185)" className="fill-white text-[48px] font-black">
                Discover
              </text>
              <text x="422" y="407" textAnchor="middle" transform="rotate(-58 422 407)" className="fill-white text-[48px] font-black">
                Ship
              </text>
              <text x="178" y="406" textAnchor="middle" transform="rotate(58 178 406)" className="fill-white text-[48px] font-black">
                Learn
              </text>

              <g className="landing-orbit-dot">
                <circle cx="300" cy="68" r="8" fill="#d93820" />
                <circle cx="300" cy="68" r="18" fill="#d93820" opacity=".12" />
              </g>
              <g className="landing-orbit-dot landing-orbit-dot-secondary">
                <circle cx="300" cy="68" r="6" fill="#0f766e" />
                <circle cx="300" cy="68" r="15" fill="#0f766e" opacity=".12" />
              </g>
              <g className="landing-orbit-dot landing-orbit-dot-tertiary">
                <circle cx="300" cy="68" r="5" fill="#f59e0b" />
                <circle cx="300" cy="68" r="13" fill="#f59e0b" opacity=".12" />
              </g>
            </svg>
            <p className="mx-auto -mt-4 max-w-sm text-center text-sm font-semibold leading-6 text-stone-600">
              Signals move around the wheel. Each result feeds the next opportunity.
            </p>
          </div>
        </section>

        <section className="grid gap-8 border-t border-stone-200 pb-10 pt-8 lg:grid-cols-[0.72fr_1fr]">
          <div>
            <div className="text-sm font-black text-[#d93820]">The product model</div>
            <h2 className="mt-2 text-2xl font-black leading-tight text-slate-950">One flywheel, three motions.</h2>
          </div>
          <div className="grid gap-3 md:grid-cols-3">
            {loopMoves.map(([title, detail]) => (
              <div key={title} className="rounded-xl border border-stone-200 bg-white/70 p-5">
                <h3 className="text-base font-black text-slate-950">{title}</h3>
                <p className="mt-2 text-sm font-semibold leading-6 text-stone-600">{detail}</p>
              </div>
            ))}
          </div>
        </section>

        <section className="grid gap-3 pb-10 md:grid-cols-4">
          {["Growth plan", "Page improvements", "Published assets", "Outcome reports"].map((item) => {
            return (
              <div key={item} className="rounded-xl border border-stone-200 bg-white/70 px-4 py-3 text-sm font-black text-slate-950">
                {item}
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
