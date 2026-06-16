import Link from "next/link";
import {
  ArrowRight,
  BookOpen,
  CheckCircle2,
  FileText,
  Gauge,
  ListChecks,
  RefreshCw,
  Search,
  Settings,
  ShieldCheck,
  UploadCloud,
} from "lucide-react";
import { AdminDocsLink } from "./admin-link";

const docsNav = [
  { label: "Overview", href: "#overview" },
  { label: "Start here", href: "#start-here" },
  { label: "Core concepts", href: "#core-concepts" },
  { label: "Workflow model", href: "#workflow-model" },
  { label: "Dashboard pages", href: "#dashboard-pages" },
  { label: "Common states and signals", href: "#common-states-and-signals" },
  { label: "Limits and expectations", href: "#limits-and-expectations" },
  { label: "Next steps", href: "#next-steps" },
];

const loopSteps = [
  "Read your domain",
  "Build context",
  "Plan content",
  "Generate drafts",
  "Check evidence",
  "Review once",
  "Publish and distribute",
  "Measure visibility",
  "Feed opportunities back into the plan",
];

const startPaths = [
  {
    title: "Set up context",
    detail: "Connect a domain, let CiteLoop read public pages, then confirm the product facts and evidence it should use.",
    href: "/",
  },
  {
    title: "Create a content plan",
    detail: "Turn context and visibility gaps into topics, schedules, and content intent before drafting starts.",
    href: "/",
  },
  {
    title: "Review and publish",
    detail: "Approve evidence-backed drafts once, then let CiteLoop publish canonical content and prepare variants.",
    href: "/",
  },
];

const concepts = [
  ["Project", "The domain-level workspace CiteLoop reads, plans, reviews, publishes, and measures."],
  ["Context", "The product profile, evidence library, source pages, voice, and rules used for every draft."],
  ["Content Plan", "The backlog of topics, visibility opportunities, angles, schedules, and generation intent."],
  ["Canonical", "The primary article published on your main content surface."],
  ["Variant", "A rewritten version prepared for a distribution surface after the canonical URL exists."],
  ["Distribution / Syndication", "The semi-manual channel path for Dev.to, Hashnode, LinkedIn, forums, and other surfaces."],
  ["Review gate", "The only human approval step before publishable content can go live."],
  ["Visibility", "SEO and AI-answer signals that identify opportunities and feed the loop back into planning."],
  ["Settings > Activity Log", "The advanced audit trail for degraded checks, failures, and automation details."],
];

const dashboardPages = [
  ["Home", "Shows the current next action, loop momentum, context health, and what needs attention now."],
  ["Context", "Shows what CiteLoop believes about your domain and the evidence behind publishable claims."],
  ["Content Plan", "Turns context and visibility gaps into topics, opportunities, and schedules."],
  ["Review", "Groups drafts that need approval and explains evidence issues in reviewer language."],
  ["Publish", "Tracks canonical publishing, URL verification, and distribution-ready variants."],
  ["Visibility", "Summarizes SEO and AI-answer visibility, opportunities, confidence, and loop closure."],
  ["Settings", "Controls cadence, budget, automation, crawl boundaries, publisher connections, and notifications."],
  ["Settings > Activity Log", "Keeps run details available for audit without making them a daily navigation item."],
];

const signals = [
  ["Needs review", "A draft is waiting for the one human gate before it can move toward publishing."],
  ["Evidence blocked", "A claim needs support in Context or must be edited before approval."],
  ["Scheduled or published", "Canonical content has moved from approved intent into the publishing lane."],
  ["Ready to distribute", "Variants unlock only after the canonical URL is confirmed."],
  ["Visibility degraded", "A provider or signal is limited; this is not the same as a negative visibility result."],
  ["Human decision needed", "The system has enough information to ask for judgment instead of guessing."],
  ["Activity log entry", "A background event exists for audit, cost, failure, or degraded automation context."],
];

const roles = [
  ["Your role", "Provide the domain, confirm context, approve drafts, and handle distribution decisions when needed."],
  ["CiteLoop role", "Read, plan, write, check evidence, publish canonical content, prepare variants, and measure visibility."],
];

function Section({
  id,
  title,
  children,
  eyebrow,
}: {
  id: string;
  title: string;
  children: React.ReactNode;
  eyebrow?: string;
}) {
  return (
    <section id={id} className="scroll-mt-8 border-t border-slate-200 py-8 first:border-t-0 first:pt-0">
      {eyebrow && <div className="mb-2 text-xs font-bold uppercase tracking-[0.14em] text-slate-400">{eyebrow}</div>}
      <h2 className="text-2xl font-bold leading-8 tracking-tight text-slate-950">{title}</h2>
      <div className="mt-4 space-y-4 text-[15px] leading-7 text-slate-600">{children}</div>
    </section>
  );
}

function PathCard({ title, detail, href }: { title: string; detail: string; href: string }) {
  return (
    <Link
      href={href}
      className="group grid min-h-[154px] content-between rounded-xl border border-slate-200 bg-white p-4 transition-colors hover:border-slate-300 hover:bg-slate-50"
    >
      <div>
        <div className="text-base font-bold text-slate-950">{title}</div>
        <p className="mt-2 text-sm leading-6 text-slate-600">{detail}</p>
      </div>
      <div className="mt-4 inline-flex items-center gap-2 text-sm font-bold text-[#d93820]">
        Start here
        <ArrowRight size={15} className="transition-transform group-hover:translate-x-0.5" />
      </div>
    </Link>
  );
}

function DefinitionRow({ term, detail }: { term: string; detail: string }) {
  return (
    <div className="grid gap-2 border-t border-slate-100 py-3 first:border-t-0 sm:grid-cols-[170px_1fr]">
      <dt className="text-sm font-bold text-slate-950">{term}</dt>
      <dd className="text-sm leading-6 text-slate-600">{detail}</dd>
    </div>
  );
}

export default function DocsPage() {
  return (
    <main className="min-h-[100dvh] bg-stone-100 px-4 py-5 text-slate-950 md:px-6 md:py-8">
      <div className="mx-auto max-w-[1180px]">
        <header className="mb-6 flex flex-col gap-4 border-b border-slate-200 pb-5 sm:flex-row sm:items-center sm:justify-between">
          <Link href="/" className="flex h-9 items-center gap-2 text-sm font-bold text-slate-900">
            <span className="grid h-7 w-7 place-items-center rounded-lg bg-slate-950 text-xs text-white">CL</span>
            CiteLoop
          </Link>
          <div className="flex flex-wrap gap-2">
            <AdminDocsLink />
            <Link
              href="/"
              className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50 hover:text-slate-950"
            >
              Create your first project
              <ArrowRight size={15} />
            </Link>
          </div>
        </header>

        <div className="grid gap-6 lg:grid-cols-[190px_minmax(0,1fr)_190px]">
          <aside className="hidden lg:block">
            <nav className="sticky top-6 grid gap-1 text-sm">
              <div className="mb-2 flex items-center gap-2 px-2 text-xs font-bold uppercase tracking-[0.14em] text-slate-400">
                <BookOpen size={14} />
                CiteLoop Docs
              </div>
              {docsNav.map((item) => (
                <a
                  key={item.href}
                  href={item.href}
                  className="rounded-lg px-2 py-1.5 font-semibold text-slate-500 transition-colors hover:bg-white hover:text-slate-950"
                >
                  {item.label}
                </a>
              ))}
            </nav>
          </aside>

          <article className="min-w-0 rounded-xl border border-slate-200 bg-white px-5 py-6 shadow-sm md:px-8 md:py-8">
            <section id="overview" className="scroll-mt-8">
              <div className="mb-2 text-xs font-bold uppercase tracking-[0.14em] text-slate-400">CiteLoop Docs</div>
              <h1 className="text-3xl font-bold leading-tight tracking-tight text-slate-950 md:text-4xl">Overview</h1>
              <p className="mt-3 max-w-2xl text-base leading-7 text-slate-600">
                How CiteLoop turns your domain into evidence-backed SEO and GEO content.
              </p>

              <div className="mt-6 rounded-xl border border-slate-200 bg-stone-50 p-4">
                <div className="mb-3 flex items-center justify-between gap-3">
                  <div className="text-sm font-bold text-slate-950">The CiteLoop loop</div>
                  <div className="rounded-md bg-white px-2 py-1 text-xs font-bold text-[#d93820] ring-1 ring-slate-200">
                    Visibility feeds planning
                  </div>
                </div>
                <div className="grid gap-2 sm:grid-cols-3">
                  {loopSteps.map((step, index) => (
                    <div
                      key={step}
                      className="relative min-h-[74px] rounded-lg border border-slate-200 bg-white px-3 py-3"
                    >
                      <div className="text-xs font-bold text-slate-400">{String(index + 1).padStart(2, "0")}</div>
                      <div className="mt-1 text-sm font-bold leading-5 text-slate-950">{step}</div>
                      {index === loopSteps.length - 1 && (
                        <div className="mt-2 inline-flex items-center gap-1 rounded-md bg-red-50 px-2 py-1 text-xs font-bold text-[#d93820]">
                          <RefreshCw size={12} />
                          back to Content Plan
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>

              <div className="mt-5 grid gap-3 md:grid-cols-2">
                {roles.map(([title, detail]) => (
                  <div key={title} className="rounded-lg border border-slate-200 bg-white p-4">
                    <div className="text-sm font-bold text-slate-950">{title}</div>
                    <p className="mt-2 text-sm leading-6 text-slate-600">{detail}</p>
                  </div>
                ))}
              </div>
            </section>

            <Section id="start-here" title="Start here">
              <p>
                Pick the path that matches where you are in the product loop. A new user can start from this page before
                a project exists; project-specific links appear after a project has been created.
              </p>
              <div className="grid gap-3 md:grid-cols-[1fr_1fr] xl:grid-cols-[1fr_1fr_1fr]">
                {startPaths.map((path) => (
                  <PathCard key={path.title} {...path} />
                ))}
              </div>
            </Section>

            <Section id="core-concepts" title="Core concepts">
              <dl className="rounded-lg border border-slate-200 bg-white px-4">
                {concepts.map(([term, detail]) => (
                  <DefinitionRow key={term} term={term} detail={detail} />
                ))}
              </dl>
            </Section>

            <Section id="workflow-model" title="Workflow model">
              <div className="grid gap-4 md:grid-cols-[1fr_1fr]">
                <div className="rounded-lg border border-slate-200 p-4">
                  <div className="flex items-center gap-2 text-sm font-bold text-slate-950">
                    <Gauge size={16} />
                    Home is the live source of next action
                  </div>
                  <p className="mt-2 text-sm leading-6 text-slate-600">
                    Docs explains why actions exist. Home decides what you should do now from current context health,
                    review load, publishing state, and visibility signals.
                  </p>
                </div>
                <div className="rounded-lg border border-slate-200 p-4">
                  <div className="flex items-center gap-2 text-sm font-bold text-slate-950">
                    <ShieldCheck size={16} />
                    Review is the only human gate
                  </div>
                  <p className="mt-2 text-sm leading-6 text-slate-600">
                    CiteLoop can prepare content automatically, but publishable content requires one approval step.
                    Evidence blocked drafts must be corrected before approval.
                  </p>
                </div>
              </div>
            </Section>

            <Section id="dashboard-pages" title="Pages in the dashboard">
              <div className="grid gap-3">
                {dashboardPages.map(([term, detail]) => (
                  <DefinitionRow key={term} term={term} detail={detail} />
                ))}
              </div>
            </Section>

            <Section id="common-states-and-signals" title="Common states and signals">
              <p>
                CiteLoop docs explain stable user-facing concepts instead of hand-maintaining a second list of internal
                status strings. Exact labels should come from shared status mappings in product code.
              </p>
              <div className="grid gap-2">
                {signals.map(([term, detail]) => (
                  <div key={term} className="grid gap-1 rounded-lg border border-slate-200 px-4 py-3 sm:grid-cols-[190px_1fr]">
                    <div className="text-sm font-bold text-slate-950">{term}</div>
                    <div className="text-sm leading-6 text-slate-600">{detail}</div>
                  </div>
                ))}
              </div>
            </Section>

            <Section id="limits-and-expectations" title="Limits and expectations">
              <div className="grid gap-3">
                {[
                  ["Public pages only", "CiteLoop reads public pages and respects crawl boundaries. It does not bypass login walls or robots rules."],
                  ["No invented metrics", "Without Search Console or GA4, CiteLoop does not fake CTR, position, conversion, or private analytics."],
                  ["Provider unavailable is not failure", "A degraded answer-engine check lowers confidence. It does not prove visibility is bad."],
                  ["Syndication is semi-manual in V1", "CiteLoop prepares variants and compose links, while third-party posting still needs human handling."],
                ].map(([term, detail]) => (
                  <DefinitionRow key={term} term={term} detail={detail} />
                ))}
              </div>
            </Section>

            <Section id="next-steps" title="Next steps">
              <div className="grid gap-3 md:grid-cols-[1.2fr_1fr]">
                <Link
                  href="/"
                  className="group flex min-h-[86px] items-center justify-between gap-4 rounded-xl border border-slate-200 bg-slate-950 px-4 py-3 text-white transition-colors hover:bg-slate-900"
                >
                  <div>
                    <div className="text-base font-bold">Create your first project</div>
                    <div className="mt-1 text-sm leading-6 text-slate-300">Start from a domain and let CiteLoop build context.</div>
                  </div>
                  <ArrowRight size={18} className="shrink-0 transition-transform group-hover:translate-x-0.5" />
                </Link>
                <div className="rounded-xl border border-slate-200 bg-white px-4 py-3">
                  <div className="text-sm font-bold text-slate-950">After a project exists</div>
                  <p className="mt-1 text-sm leading-6 text-slate-600">
                    Open Context, Content Plan, Review, Publish, Visibility, and Settings from the project sidebar.
                  </p>
                </div>
              </div>
            </Section>
          </article>

          <aside className="hidden lg:block">
            <div className="sticky top-6 rounded-xl border border-slate-200 bg-white p-4">
              <div className="mb-3 text-xs font-bold uppercase tracking-[0.14em] text-slate-400">On This Page</div>
              <div className="grid gap-2 text-sm">
                {[
                  [FileText, "Overview"],
                  [ListChecks, "Start here"],
                  [CheckCircle2, "Review gate"],
                  [UploadCloud, "Publish"],
                  [Search, "Visibility"],
                  [Settings, "Activity Log"],
                ].map(([Icon, label]) => {
                  const IconComponent = Icon;
                  return (
                    <div key={label as string} className="flex items-center gap-2 text-slate-600">
                      <IconComponent size={14} />
                      <span className="font-semibold">{label as string}</span>
                    </div>
                  );
                })}
              </div>
            </div>
          </aside>
        </div>
      </div>
    </main>
  );
}

