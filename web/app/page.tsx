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

        .landing-outer-arc {
          fill: none;
          stroke: #dbe5ef;
          stroke-linecap: round;
          stroke-width: 46;
        }

        .landing-outer-arrow {
          fill: #dbe5ef;
          opacity: .96;
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

        .landing-flywheel:is(:hover, :focus-within) .landing-outer-track,
        .landing-flywheel:is(:hover, :focus-within) .landing-orbit-dot,
        .landing-flywheel:is(:hover, :focus-within) .landing-segment,
        .landing-flywheel:is(:hover, :focus-within) .landing-center-pulse {
          animation-play-state: paused;
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
              <div className="hidden flex-wrap items-center justify-end gap-2 sm:flex">
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
              <div className="hidden items-center justify-end gap-3 sm:flex">
                <LandingDashboardButton initialProjects={projects} projectPrefetchFailed={projectPrefetchFailed} />
                <UserButton />
              </div>
            ))}
        </header>

        <section className="grid flex-1 items-center gap-10 py-12 lg:grid-cols-[minmax(0,430px)_minmax(0,1fr)] lg:gap-14 lg:py-9">
          <div className="min-w-0 max-w-xl">
            <div className="mb-5 inline-flex h-8 items-center rounded-full border border-[#efcfc4] bg-white/80 px-3 text-xs font-black tracking-[0.16em] text-[#d93820]">
              SEO/GEO GROWTH LOOP
            </div>
            <h1
              aria-label="Turn your website into a self-improving growth loop."
              className="text-[2rem] font-black leading-[1.04] tracking-tight text-slate-950 break-words sm:text-4xl md:text-6xl"
            >
              <span className="block sm:inline">Turn your website</span>{" "}
              <span className="block sm:inline">into a self-improving</span>{" "}
              <span className="block sm:inline">growth loop.</span>
            </h1>
            <p className="mt-5 max-w-[31ch] text-sm leading-6 text-stone-700 sm:max-w-[58ch] sm:text-base sm:leading-7 md:text-lg">
              Connect your domain, Search Console, and publishing target. CiteLoop discovers what to improve, ships the
              work safely, and measures what moved.
            </p>
            <div className="mt-7 grid w-full max-w-sm grid-cols-1 gap-3 sm:flex sm:max-w-none">
              {clerkServerAuthConfigured && signedOut && (
                <>
                  <Link
                    href="/sign-up"
                    className="inline-flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-slate-950 px-5 text-sm font-semibold text-white transition-colors hover:bg-slate-800 active:scale-[0.98] sm:w-auto"
                  >
                    Start with your domain
                    <ArrowRight size={16} aria-hidden="true" />
                  </Link>
                  <JoinWithGoogleButton className="h-11 w-full px-5 sm:w-auto" />
                </>
              )}
              {clerkServerAuthConfigured && !signedOut && (
                <LandingDashboardButton
                  initialProjects={projects}
                  projectPrefetchFailed={projectPrefetchFailed}
                  className="h-11 w-full px-5 sm:w-auto"
                />
              )}
            </div>
            <div className="mt-7 grid max-w-xl gap-2 text-sm font-semibold text-stone-600 sm:grid-cols-3">
              <div className="rounded-lg border border-stone-200 bg-white/70 px-3 py-2">Domain</div>
              <div className="rounded-lg border border-stone-200 bg-white/70 px-3 py-2">Search Console</div>
              <div className="rounded-lg border border-stone-200 bg-white/70 px-3 py-2">Publisher</div>
            </div>
          </div>

          <div className="landing-flywheel relative mx-auto min-w-0 w-full max-w-[340px] sm:max-w-[650px]" aria-label="CiteLoop SEO GEO flywheel">
            <svg className="h-auto w-full overflow-hidden" viewBox="-28 -28 656 656" role="img" aria-labelledby="flywheel-title flywheel-desc">
              <title id="flywheel-title">CiteLoop Growth Loop flywheel</title>
              <desc id="flywheel-desc">
                Domain and Search Console signals become opportunities, published assets, and measured outcomes.
              </desc>
              <defs>
                <path id="domain-gsc-label" d="M 165 105 A 230 230 0 0 1 435 105" />
                <path id="opportunities-label" d="M 515 194 A 245 245 0 0 1 500 415" />
                <path id="published-assets-label" d="M 170 548 A 265 265 0 0 0 430 548" />
                <path id="measured-outcomes-label" d="M 72 412 A 245 245 0 0 1 86 195" />
                <path id="discover-segment-label" d="M 187 187 A 160 160 0 0 1 413 187" />
                <path id="ship-segment-label" d="M 365 483 A 195 195 0 0 0 490 256" />
                <path id="learn-segment-label" d="M 108 266 A 195 195 0 0 0 233 483" />
              </defs>

              <g className="landing-outer-track">
                <path className="landing-outer-arc" d="M 87 121 A 278 278 0 0 1 541 161" />
                <path className="landing-outer-arrow" d="M 514 132 L 560 158 L 532 194 Z" />
                <path className="landing-outer-arc" d="M 574 252 A 278 278 0 0 1 252 574" />
                <path className="landing-outer-arrow" d="M 261 551 L 220 568 L 253 599 Z" />
                <path className="landing-outer-arc" d="M 161 541 A 278 278 0 0 1 121 87" />
                <path className="landing-outer-arrow" d="M 128 112 L 145 67 L 98 76 Z" />
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

              <text className="landing-segment-label fill-white text-[46px] font-black">
                <textPath href="#discover-segment-label" startOffset="50%" textAnchor="middle">
                  Discover
                </textPath>
              </text>
              <text className="landing-segment-label fill-white text-[46px] font-black">
                <textPath href="#ship-segment-label" startOffset="50%" textAnchor="middle">
                  Ship
                </textPath>
              </text>
              <text className="landing-segment-label fill-white text-[46px] font-black">
                <textPath href="#learn-segment-label" startOffset="50%" textAnchor="middle">
                  Learn
                </textPath>
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
            <p className="mx-auto -mt-3 max-w-[30ch] text-center text-xs font-semibold leading-5 text-stone-600 sm:-mt-4 sm:max-w-sm sm:text-sm sm:leading-6">
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
