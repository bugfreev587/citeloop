import Link from "next/link";
import {
  ArrowRight,
  BookOpen,
  CheckCircle2,
  Code2,
  FileText,
  Gauge,
  ListChecks,
  RefreshCw,
  Search,
  Settings,
  ShieldCheck,
  Target,
  UploadCloud,
} from "lucide-react";
import { AdminDocsLink } from "./admin-link";

const navGroups = [
  {
    title: "Using CiteLoop",
    items: [
      { label: "Dashboard Quickstart", href: "#overview", active: true },
      { label: "Prerequisite", href: "#prerequisite" },
      { label: "The four steps", href: "#the-four-steps" },
      { label: "Install and initialize", href: "#install-and-initialize" },
    ],
  },
  {
    title: "Concepts",
    items: [
      { label: "Core concepts", href: "#core-concepts" },
      { label: "Workflow model", href: "#workflow-model" },
      { label: "Dashboard pages", href: "#dashboard-pages" },
    ],
  },
  {
    title: "Operations",
    items: [
      { label: "Common states and signals", href: "#common-states-and-signals" },
      { label: "Limits and expectations", href: "#limits-and-expectations" },
      { label: "Next steps", href: "#next-steps" },
    ],
  },
];

const onThisPage = [
  { label: "Overview", href: "#overview", icon: FileText },
  { label: "Prerequisite", href: "#prerequisite", icon: CheckCircle2 },
  { label: "The four steps", href: "#the-four-steps", icon: ListChecks },
  { label: "At a glance", href: "#at-a-glance", icon: Gauge },
  { label: "Install and initialize", href: "#install-and-initialize", icon: Code2 },
  { label: "Review gate", href: "#workflow-model", icon: ShieldCheck },
  { label: "Publish", href: "#dashboard-pages", icon: UploadCloud },
  { label: "Results", href: "#common-states-and-signals", icon: Search },
  { label: "Activity Log", href: "#common-states-and-signals", icon: Settings },
];

const quickstartBadges = ["Dashboard-first", "Evidence-backed", "One review gate", "SEO + GEO"];

const loopSteps = [
  "Read your domain",
  "Build context",
  "Review opportunities",
  "Plan content",
  "Generate drafts",
  "Check evidence",
  "Review once",
  "Publish and distribute",
  "Measure results",
  "Feed opportunities back into the plan",
];

const fourSteps = [
  {
    title: "Create a project",
    detail: "Start with a public domain so CiteLoop can build a workspace around real product evidence.",
    href: "/",
  },
  {
    title: "Confirm context",
    detail: "Review product facts, source pages, rules, and crawl boundaries before planning content.",
    href: "/",
  },
  {
    title: "Review opportunities",
    detail: "Keep the recommendations that should enter the plan; ignore the rest.",
    href: "/",
  },
  {
    title: "Approve and publish",
    detail: "Use the review gate once, then move canonical content toward publishing and distribution.",
    href: "/",
  },
];

const glanceRows = [
  ["Account owner", "You or your team, starting from one product domain."],
  ["Context source", "Public pages, confirmed facts, evidence snippets, and product rules."],
  ["Human gate", "Review is the one approval step before content can move toward publishing."],
  ["What you get back", "Opportunity recommendations, content briefs, evidence-backed drafts, canonical URLs, and variants."],
  ["Best for", "Teams that want a steady SEO and GEO content loop without inventing private metrics."],
  ["Need audit detail?", "Use Settings > Activity Log for background events, failures, and degraded checks."],
];

const startPaths = [
  {
    title: "Set up context",
    detail: "Connect a domain, let CiteLoop read public pages, then confirm the product facts and evidence it should use.",
    href: "/",
  },
  {
    title: "Create a content plan",
    detail: "Review opportunities, then turn the ones you keep into topics, schedules, and content intent.",
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
  ["Opportunities", "Decision-ready recommendations that need review before they enter the plan."],
  ["Content Plan", "The backlog of reviewed topics, angles, schedules, and generation intent."],
  ["Canonical", "The primary article published on your main content surface."],
  ["Variant", "A rewritten version prepared for a distribution surface after the canonical URL exists."],
  ["Distribution / Syndication", "The semi-manual channel path for Dev.to, Hashnode, LinkedIn, forums, and other surfaces."],
  ["Review gate", "The only human approval step before publishable content can go live."],
  ["Results", "Measurement and diagnostics for SEO, GEO, crawler access, and AI-answer signals."],
  ["Settings > Activity Log", "The advanced audit trail for degraded checks, failures, and automation details."],
];

const dashboardPages = [
  ["Home", "Shows the current next action, loop momentum, context health, and what needs attention now."],
  ["Context", "Shows what CiteLoop believes about your domain and the evidence behind publishable claims."],
  ["Opportunities", "Reviews automatically generated recommendations before they become content work."],
  ["Content Plan", "Turns accepted opportunities into topics, schedules, and drafting intent."],
  ["Review", "Groups drafts that need approval and explains evidence issues in reviewer language."],
  ["Publish", "Tracks canonical publishing, URL verification, and distribution-ready variants."],
  ["Results", "Shows measurement coverage, diagnostics, crawler access, and GEO visibility signals."],
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

const limits = [
  ["Public pages only", "CiteLoop reads public pages and respects crawl boundaries. It does not bypass login walls or robots rules."],
  ["No invented metrics", "Without Search Console or GA4, CiteLoop does not fake CTR, position, conversion, or private analytics."],
  ["Provider unavailable is not failure", "A degraded answer-engine check lowers confidence. It does not prove visibility is bad."],
  ["Syndication is semi-manual in V1", "CiteLoop prepares variants and compose links, while third-party posting still needs human handling."],
];

const roles = [
  ["Your role", "Provide the domain, confirm context, approve drafts, and handle distribution decisions when needed."],
  ["CiteLoop role", "Read, analyze, plan, write, check evidence, publish canonical content, prepare variants, and measure results."],
];

const structuredHandoff = `project
  -> context profile
  -> accepted opportunities
  -> content plan
  -> evidence-backed draft
  -> approved canonical article
  -> distribution variants
  -> result signals`;

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
    <section id={id} className="scroll-mt-24 border-t border-slate-200 py-10 first:border-t-0 first:pt-0">
      {eyebrow && <div className="mb-2 text-xs font-bold uppercase tracking-[0.14em] text-slate-400">{eyebrow}</div>}
      <h2 className="text-2xl font-bold leading-8 tracking-tight text-slate-950">{title}</h2>
      <div className="mt-4 space-y-4 text-[15px] leading-7 text-slate-600">{children}</div>
    </section>
  );
}

function DefinitionRow({ term, detail }: { term: string; detail: string }) {
  return (
    <div className="grid gap-2 border-t border-slate-100 py-3 first:border-t-0 sm:grid-cols-[190px_1fr]">
      <dt className="text-sm font-bold text-slate-950">{term}</dt>
      <dd className="text-sm leading-6 text-slate-600">{detail}</dd>
    </div>
  );
}

function QuickstartCard({
  index,
  title,
  detail,
  href,
}: {
  index: number;
  title: string;
  detail: string;
  href: string;
}) {
  return (
    <Link
      href={href}
      className="group grid min-h-[150px] content-between rounded-lg border border-slate-200 bg-white p-4 transition-all duration-150 hover:-translate-y-0.5 hover:border-slate-300 hover:shadow-sm active:translate-y-0"
    >
      <div>
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs font-bold uppercase tracking-[0.16em] text-slate-400">
            {String(index).padStart(2, "0")}
          </span>
          <ArrowRight size={15} className="text-slate-300 transition-transform group-hover:translate-x-0.5" />
        </div>
        <div className="mt-3 text-base font-bold text-slate-950">{title}</div>
        <p className="mt-2 text-sm leading-6 text-slate-600">{detail}</p>
      </div>
    </Link>
  );
}

function NavColumn() {
  return (
    <aside className="hidden lg:block">
      <nav className="sticky top-4 grid gap-7 text-sm">
        {navGroups.map((group) => (
          <div key={group.title}>
            <div className="mb-2 px-2 text-xs font-bold uppercase tracking-[0.14em] text-slate-400">{group.title}</div>
            <div className="grid gap-0.5">
              {group.items.map((item) => (
                <a
                  key={item.href}
                  href={item.href}
                  className={
                    item.active
                      ? "rounded-lg bg-white px-2 py-1.5 font-bold text-slate-950 shadow-sm ring-1 ring-slate-200"
                      : "rounded-lg px-2 py-1.5 font-semibold text-slate-500 transition-colors hover:bg-white hover:text-slate-950"
                  }
                >
                  {item.label}
                </a>
              ))}
            </div>
          </div>
        ))}
      </nav>
    </aside>
  );
}

function PageRail() {
  return (
    <aside className="hidden xl:block">
      <div className="sticky top-4">
        <div className="mb-3 text-xs font-bold uppercase tracking-[0.14em] text-slate-400">On This Page</div>
        <div className="grid gap-2 text-sm">
          {onThisPage.map((item) => {
            const Icon = item.icon;
            return (
              <a
                key={item.href + item.label}
                href={item.href}
                className="flex items-center gap-2 rounded-lg py-1.5 font-semibold text-slate-500 transition-colors hover:text-slate-950"
              >
                <Icon size={14} />
                <span>{item.label}</span>
              </a>
            );
          })}
        </div>
      </div>
    </aside>
  );
}

export default function DocsPage() {
  return (
    <main className="min-h-[100dvh] bg-[#f7f6f2] text-slate-950">
      <header className="sticky top-0 z-20 border-b border-slate-200/80 bg-[#f7f6f2]/95 px-4 py-3 backdrop-blur md:px-6">
        <div className="mx-auto flex max-w-[1280px] flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex items-center justify-between gap-3">
            <Link href="/" className="flex h-9 items-center gap-2 text-sm font-bold text-slate-900">
              <span className="grid h-7 w-7 place-items-center rounded-lg bg-slate-950 text-xs text-white">CL</span>
              CiteLoop
            </Link>
            <div className="flex items-center gap-2 sm:hidden">
              <AdminDocsLink />
              <Link
                href="/"
                className="inline-flex h-9 items-center gap-2 rounded-lg bg-slate-950 px-3 text-sm font-semibold text-white transition-colors hover:bg-slate-800"
              >
                Start
                <ArrowRight size={15} />
              </Link>
            </div>
          </div>

          <div className="flex min-w-0 flex-1 items-center gap-3 lg:max-w-[560px]">
            <div className="flex h-10 min-w-0 flex-1 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm text-slate-500 shadow-sm">
              <Search size={16} className="shrink-0 text-slate-400" />
              <span className="truncate">Search docs</span>
              <span className="ml-auto hidden rounded-md border border-slate-200 px-1.5 py-0.5 text-[11px] font-bold text-slate-400 sm:block">
                Cmd K
              </span>
            </div>
            <div className="hidden gap-2 sm:flex">
              <AdminDocsLink />
              <Link
                href="/"
                className="inline-flex h-10 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50 hover:text-slate-950"
              >
                Create your first project
                <ArrowRight size={15} />
              </Link>
            </div>
          </div>
        </div>
      </header>

      <div className="mx-auto grid max-w-[1280px] gap-8 px-4 py-6 md:px-6 lg:grid-cols-[220px_minmax(0,1fr)] xl:grid-cols-[220px_minmax(0,1fr)_210px]">
        <NavColumn />

        <article className="min-w-0 bg-white px-5 py-6 shadow-sm ring-1 ring-slate-200 md:px-10 md:py-10">
          <section id="overview" className="scroll-mt-24">
            <div className="mb-2 text-xs font-bold uppercase tracking-[0.14em] text-[#d93820]">Overview</div>
            <h1 className="text-3xl font-bold leading-tight tracking-tight text-slate-950 md:text-5xl">
              Dashboard Quickstart
            </h1>
            <p className="mt-4 max-w-2xl text-base leading-7 text-slate-600">
              How CiteLoop turns your domain into evidence-backed SEO and GEO content.
            </p>

            <div className="mt-7 border-y border-slate-200 py-4">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-bold text-slate-950">Interactive quickstart</span>
                {quickstartBadges.map((badge) => (
                  <span
                    key={badge}
                    className="rounded-md bg-slate-50 px-2 py-1 text-xs font-bold text-slate-600 ring-1 ring-slate-200"
                  >
                    {badge}
                  </span>
                ))}
              </div>
            </div>

            <div className="mt-6 grid gap-3 md:grid-cols-2">
              {roles.map(([title, detail]) => (
                <div key={title} className="rounded-lg border border-slate-200 bg-slate-50 p-4">
                  <div className="text-sm font-bold text-slate-950">{title}</div>
                  <p className="mt-2 text-sm leading-6 text-slate-600">{detail}</p>
                </div>
              ))}
            </div>
          </section>

          <Section id="prerequisite" title="Prerequisite">
            <p>
              Create a project from a public product domain. CiteLoop needs enough accessible source material to build
              context, find recommendations, and keep every draft tied to evidence.
            </p>
            <ul className="grid gap-2 text-sm leading-6 text-slate-600">
              <li className="flex gap-2">
                <CheckCircle2 className="mt-1 shrink-0 text-[#d93820]" size={16} />
                <span>Use a crawlable homepage, docs site, changelog, or product marketing site.</span>
              </li>
              <li className="flex gap-2">
                <CheckCircle2 className="mt-1 shrink-0 text-[#d93820]" size={16} />
                <span>Confirm product facts in Context before generated content becomes publishable.</span>
              </li>
              <li className="flex gap-2">
                <CheckCircle2 className="mt-1 shrink-0 text-[#d93820]" size={16} />
                <span>Review opportunities before recommendations enter the Content Plan.</span>
              </li>
            </ul>
          </Section>

          <Section id="the-four-steps" title="The four steps">
            <div className="grid gap-3 md:grid-cols-2">
              {fourSteps.map((step, index) => (
                <QuickstartCard key={step.title} index={index + 1} {...step} />
              ))}
            </div>
          </Section>

          <Section id="at-a-glance" title="At a glance">
            <div className="overflow-hidden rounded-lg border border-slate-200">
              <table className="w-full border-collapse text-left text-sm">
                <tbody className="divide-y divide-slate-100">
                  {glanceRows.map(([question, answer]) => (
                    <tr key={question} className="align-top">
                      <th className="w-[34%] bg-slate-50 px-4 py-3 font-bold text-slate-950">{question}</th>
                      <td className="px-4 py-3 leading-6 text-slate-600">{answer}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Section>

          <Section id="install-and-initialize" title="Install and initialize">
            <div className="overflow-hidden rounded-lg border border-slate-200">
              <div className="flex flex-wrap gap-2 border-b border-slate-200 bg-slate-50 p-2 text-xs font-bold">
                <span className="rounded-md bg-white px-2 py-1 text-slate-950 ring-1 ring-slate-200">Dashboard-first</span>
                <span className="rounded-md px-2 py-1 text-slate-500">API-style handoff</span>
              </div>
              <div className="grid gap-0 md:grid-cols-[1fr_1.1fr]">
                <div className="border-b border-slate-200 p-4 md:border-b-0 md:border-r">
                  <h3 className="text-base font-bold text-slate-950">Initialize from the dashboard</h3>
                  <p className="mt-2 text-sm leading-6 text-slate-600">
                    Open CiteLoop, create a project, then use Opportunities, Content Plan, Review, Publish, and Results for daily work.
                  </p>
                </div>
                <pre className="overflow-x-auto bg-slate-950 p-4 text-sm leading-6 text-slate-100">
                  <code>{structuredHandoff}</code>
                </pre>
              </div>
            </div>
          </Section>

          <Section id="start-here" title="Start here">
            <p>
              Pick the path that matches where you are in the product loop. A new user can start from this page before
              a project exists; project-specific links appear after a project has been created.
            </p>
            <div className="grid gap-3 md:grid-cols-3">
              {startPaths.map((path, index) => (
                <QuickstartCard key={path.title} index={index + 1} {...path} />
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
            <div className="grid gap-4 md:grid-cols-2">
              <div className="rounded-lg border border-slate-200 p-4">
                <div className="flex items-center gap-2 text-sm font-bold text-slate-950">
                  <Gauge size={16} />
                  Home is the live source of next action
                </div>
                <p className="mt-2 text-sm leading-6 text-slate-600">
                  Docs explains why actions exist. Home decides what you should do now from current context health,
                  review load, publishing state, and result signals.
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
            <div className="mt-5 rounded-lg border border-slate-200 bg-slate-50 p-4">
              <div className="mb-3 flex items-center justify-between gap-3">
                <div className="text-sm font-bold text-slate-950">The CiteLoop loop</div>
                <div className="rounded-md bg-white px-2 py-1 text-xs font-bold text-[#d93820] ring-1 ring-slate-200">
                  Opportunities feed planning
                </div>
              </div>
              <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                {loopSteps.map((step, index) => (
                  <div key={step} className="min-h-[76px] rounded-lg border border-slate-200 bg-white px-3 py-3">
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
                <div
                  key={term}
                  className="grid gap-1 rounded-lg border border-slate-200 px-4 py-3 sm:grid-cols-[190px_1fr]"
                >
                  <div className="text-sm font-bold text-slate-950">{term}</div>
                  <div className="text-sm leading-6 text-slate-600">{detail}</div>
                </div>
              ))}
            </div>
          </Section>

          <Section id="limits-and-expectations" title="Limits and expectations">
            <div className="grid gap-3">
              {limits.map(([term, detail]) => (
                <DefinitionRow key={term} term={term} detail={detail} />
              ))}
            </div>
          </Section>

          <Section id="next-steps" title="Next steps">
            <div className="grid gap-3 md:grid-cols-[1.2fr_1fr]">
              <Link
                href="/"
                className="group flex min-h-[86px] items-center justify-between gap-4 rounded-lg border border-slate-900 bg-slate-950 px-4 py-3 text-white transition-colors hover:bg-slate-900"
              >
                <div>
                  <div className="text-base font-bold">Create your first project</div>
                  <div className="mt-1 text-sm leading-6 text-slate-300">Start from a domain and let CiteLoop build context.</div>
                </div>
                <ArrowRight size={18} className="shrink-0 transition-transform group-hover:translate-x-0.5" />
              </Link>
              <div className="rounded-lg border border-slate-200 bg-white px-4 py-3">
                <div className="text-sm font-bold text-slate-950">After a project exists</div>
                <p className="mt-1 text-sm leading-6 text-slate-600">
                  Use Opportunities, Content Plan, Review, Publish, and Results for daily work. Docs, Context, and Settings stay in the utility area.
                </p>
              </div>
            </div>
          </Section>
        </article>

        <PageRail />
      </div>
    </main>
  );
}
