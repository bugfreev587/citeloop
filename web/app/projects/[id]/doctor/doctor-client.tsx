"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, CheckCircle2, Clipboard, Code2, Play, RefreshCw, Stethoscope, X } from "lucide-react";
import { SEODoctorFinding, SEODoctorReport, SEODoctorRun } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, ButtonProgress, EmptyState, Notice, SectionHeader, cx, formatDate } from "../../../components/ui";
import { useToast } from "../../../components/toast-provider";

type SeverityFilter = "all" | "P0" | "P1" | "P2" | "Info";

const severityOrder: Record<string, number> = { P0: 0, P1: 1, P2: 2, Info: 3 };

function isActiveRun(run?: SEODoctorRun | null) {
  return run?.status === "queued" || run?.status === "running";
}

function severityTone(severity: string): "red" | "amber" | "blue" | "neutral" {
  if (severity === "P0") return "red";
  if (severity === "P1") return "amber";
  if (severity === "P2") return "blue";
  return "neutral";
}

function statusTone(status?: string): "red" | "amber" | "green" | "blue" | "neutral" {
  if (status === "blocked" || status === "failed") return "red";
  if (status === "completed") return "green";
  if (status === "queued" || status === "running") return "blue";
  return "neutral";
}

function healthTone(score?: number | null) {
  if (score == null) return "text-slate-400";
  if (score < 70) return "text-red-700";
  if (score < 90) return "text-amber-700";
  return "text-emerald-700";
}

function issueCounts(report: SEODoctorReport | null) {
  const counts = report?.human_report?.issue_counts ?? {};
  return {
    P0: Number(counts.P0 ?? 0),
    P1: Number(counts.P1 ?? 0),
    P2: Number(counts.P2 ?? 0),
    Info: Number(counts.Info ?? 0),
  };
}

function sortedFindings(findings: SEODoctorFinding[]) {
  return [...findings].sort((a, b) => {
    return (severityOrder[a.severity] ?? 4) - (severityOrder[b.severity] ?? 4) || a.issue_type.localeCompare(b.issue_type);
  });
}

function firstURL(finding: SEODoctorFinding) {
  return finding.affected_urls[0] || finding.normalized_urls[0] || "Project surface";
}

function uniqueStrings(values: string[]) {
  return Array.from(new Set(values.map((value) => value.trim()).filter(Boolean)));
}

function compactRepairObject(value: Record<string, any>) {
  return Object.fromEntries(
    Object.entries(value).filter(([, entry]) => {
      if (entry == null) return false;
      if (Array.isArray(entry)) return entry.length > 0;
      if (typeof entry === "string") return entry.trim().length > 0;
      return true;
    }),
  );
}

function parseRepairURL(value?: string | null) {
  if (!value) return null;
  try {
    return new URL(value);
  } catch {
    return null;
  }
}

function isStructuredDataIssue(finding: SEODoctorFinding) {
  return finding.category === "structured_data" || finding.issue_type.startsWith("structured_data_") || finding.issue_type === "unsafe_mdx_detected";
}

function repairCanonicalURL(finding: SEODoctorFinding) {
  const evidence = repairEvidence(finding);
  return evidence.normalized_page_url ?? evidence.final_url ?? evidence.page_url ?? firstURL(finding);
}

function repairPageRole(finding: SEODoctorFinding) {
  const url = parseRepairURL(repairCanonicalURL(finding));
  if (url && (url.pathname === "" || url.pathname === "/")) return "homepage";
  return "web_page";
}

function structuredDataSchemaTypes(pageRole: string) {
  if (pageRole === "homepage") return ["WebSite", "Organization", "WebPage"];
  return ["WebPage"];
}

function buildStructuredDataRepairContract(finding: SEODoctorFinding) {
  if (!isStructuredDataIssue(finding)) return null;

  const canonicalURL = repairCanonicalURL(finding);
  const pageRole = repairPageRole(finding);

  return {
    page_role: pageRole,
    canonical_url: canonicalURL,
    schema_types: structuredDataSchemaTypes(pageRole),
    render_requirement: "Add server-rendered JSON-LD to the initial HTML so crawlers can read it without client-side interaction.",
    field_sources: [
      { field: "name", source: "production site, product, or brand name for the affected page" },
      { field: "url", source: "canonical_url from this repair contract" },
      { field: "logo", source: "production logo asset URL that returns a 200 image response" },
      { field: "description", source: "page meta description or approved brand/product description" },
      { field: "sameAs", source: "official social or profile URLs if available; omit rather than invent" },
      { field: "contactPoint", source: "public support/contact details if available; omit rather than invent" },
      { field: "publisher", source: "the same Organization node when the page is published by the brand/site" },
      { field: "inLanguage", source: "page html lang, locale config, or canonical locale" },
      { field: "potentialAction", source: "site search URL template only when the site has search; omit otherwise" },
    ],
    canonical_rules: [
      "Use the production canonical URL and preserve the trailing-slash format chosen by the page canonical tag and redirects.",
      "Do not create competing /path and /path/ structured-data URLs for the same canonical page.",
      "If this is a localized page, use the locale-specific canonical URL and language instead of the default homepage values.",
    ],
    validation_tools: ["Google Rich Results Test", "Schema Markup Validator"],
    negative_checks: [
      "Do not add placeholder JSON-LD solely to silence the warning.",
      "Require @context to be https://schema.org and @type to match the page role.",
      "Use absolute production URLs for url, logo, sameAs, publisher, and potentialAction targets.",
      "Reject localhost, staging, preview, test, or dev URLs.",
      "Remove empty, unknown, or templated fields instead of shipping blank values.",
      "Verify logo and referenced image URLs return successful image responses.",
    ],
  };
}

function structuredDataAcceptanceTests(finding: SEODoctorFinding) {
  if (!isStructuredDataIssue(finding)) return [];

  const canonicalURL = repairCanonicalURL(finding);
  const pageRole = repairPageRole(finding);
  const schemaTypes = structuredDataSchemaTypes(pageRole);

  return [
    `Inspect the initial HTML for ${canonicalURL} and verify it contains server-rendered JSON-LD in a script[type="application/ld+json"] element that crawlers can read without client-side interaction.`,
    "Parse every JSON-LD block from the rendered HTML and verify each block is valid JSON without template placeholders.",
    "Validate the JSON-LD with Google Rich Results Test or Schema Markup Validator and resolve parser errors or unreadable schema warnings.",
    pageRole === "homepage"
      ? "For homepage pages, verify the JSON-LD models the page with WebSite, Organization, and WebPage schema types using real production brand metadata."
      : `Verify the JSON-LD @type matches the actual page role; expected baseline schema types: ${schemaTypes.join(", ")}.`,
    "Verify @context is https://schema.org, @type is present, all URL fields use absolute production URLs, there are no localhost, staging, preview, or dev URLs, no empty required fields, and any logo URL returns a 200 image response.",
  ];
}

function buildAIRepairAcceptanceTests(finding: SEODoctorFinding) {
  return uniqueStrings([
    ...(finding.acceptance_tests ?? []),
    ...structuredDataAcceptanceTests(finding),
    `After the code fix, rerun Doctor or an equivalent crawler and confirm no active "${finding.issue_type}" issue appears for the same affected URLs, normalized URLs, or equivalent canonical URL variants.`,
    "Confirm the underlying page response, metadata, schema, redirect, link, or crawl behavior that produced this finding now matches the intended SEO/GEO contract, not only that the card was dismissed.",
  ]);
}

function findingEvidence(finding: SEODoctorFinding): Array<[string, unknown]> {
  const evidence = finding.evidence && typeof finding.evidence === "object" ? finding.evidence : {};
  const rawDetails = evidence.raw_details && typeof evidence.raw_details === "object" ? evidence.raw_details : {};

  const rows: Array<[string, unknown]> = [
    ["Page", evidence.page_url ?? firstURL(finding)],
    ["Normalized", evidence.normalized_page_url ?? finding.normalized_urls[0]],
    ["Status", rawDetails.status ?? evidence.status],
    ["Final URL", rawDetails.final_url ?? evidence.final_url],
    ["Confidence", evidence.confidence_label ?? evidence.confidence],
  ];
  return rows.filter(([, value]) => value != null && `${value}`.trim() !== "");
}

function repairEvidence(finding: SEODoctorFinding) {
  const evidence = finding.evidence && typeof finding.evidence === "object" ? finding.evidence : {};
  const rawDetails = evidence.raw_details && typeof evidence.raw_details === "object" ? evidence.raw_details : {};

  return compactRepairObject({
    page_url: evidence.page_url ?? firstURL(finding),
    normalized_page_url: evidence.normalized_page_url ?? finding.normalized_urls[0] ?? null,
    status: rawDetails.status ?? evidence.status ?? null,
    final_url: rawDetails.final_url ?? evidence.final_url ?? null,
    confidence: evidence.confidence_label ?? evidence.confidence ?? null,
  });
}

function buildAIRepairPayload(finding: SEODoctorFinding) {
  return {
    issue: compactRepairObject({
      severity: finding.severity,
      category: finding.category,
      issue_type: finding.issue_type,
      affected_urls: finding.affected_urls,
      normalized_urls: finding.normalized_urls,
      problem: finding.fix_intent || finding.issue_type,
      why_it_matters: finding.why_it_matters,
    }),
    evidence: repairEvidence(finding),
    fix: compactRepairObject({
      goal: finding.fix_intent || finding.issue_type,
      instructions: finding.developer_instructions,
      likely_surfaces: finding.likely_files_or_surfaces,
      seo_contract: buildStructuredDataRepairContract(finding),
      risk_level: finding.risk_level,
    }),
    acceptance_tests: buildAIRepairAcceptanceTests(finding),
  };
}

async function writeClipboardText(text: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Fall through to the textarea fallback for browsers that block async clipboard writes.
    }
  }

  const textarea = document.createElement("textarea");
  const activeElement = document.activeElement instanceof HTMLElement ? document.activeElement : null;

  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  textarea.style.top = "0";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  textarea.setSelectionRange(0, text.length);

  const copied = document.execCommand("copy");
  textarea.remove();
  activeElement?.focus();

  if (!copied) {
    throw new Error("Clipboard write failed.");
  }
}

export function DoctorClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const { notify } = useToast();
  const [report, setReport] = useState<SEODoctorReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<SeverityFilter>("all");
  const [busyFindingID, setBusyFindingID] = useState<string | null>(null);
  const [selectedRepairFinding, setSelectedRepairFinding] = useState<SEODoctorFinding | null>(null);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const next = await api.getSEODoctor(projectId);
      setReport(next);
      return next;
    } catch (err: any) {
      setError(err?.apiMessage || err?.message || "Could not load Doctor.");
      return null;
    } finally {
      setLoading(false);
    }
  }, [api, projectId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!isActiveRun(report?.run)) return;
    const interval = window.setInterval(() => {
      void refresh();
    }, 2500);
    return () => window.clearInterval(interval);
  }, [refresh, report?.run?.id, report?.run?.status]);

  const run = report?.run ?? null;
  const counts = issueCounts(report);
  const visibleFindings = useMemo(() => {
    const findings = sortedFindings(report?.findings ?? []);
    return filter === "all" ? findings : findings.filter((finding) => finding.severity === filter);
  }, [filter, report?.findings]);
  const progress = Math.max(0, Math.min(100, run?.progress_percent ?? 0));
  const healthScore = run?.health_score ?? report?.human_report?.health_score ?? null;
  const selectedRepairJSON = useMemo(() => {
    return selectedRepairFinding ? JSON.stringify(buildAIRepairPayload(selectedRepairFinding), null, 2) : "";
  }, [selectedRepairFinding]);

  async function runDoctor() {
    setRunning(true);
    setError(null);
    try {
      const nextRun = await api.startSEODoctorRun(projectId);
      setReport((current) => ({ ...(current ?? { findings: [] }), run: nextRun }));
      notify({ tone: "green", title: "Doctor started", detail: "The report will update as checks complete." });
      window.setTimeout(() => void refresh(), 800);
    } catch (err: any) {
      setError(err?.apiMessage || err?.message || "Could not start Doctor.");
    } finally {
      setRunning(false);
    }
  }

  async function copyAIRepairJSON(finding: SEODoctorFinding) {
    try {
      await writeClipboardText(JSON.stringify(buildAIRepairPayload(finding), null, 2));
      notify({ tone: "green", title: "Repair JSON copied" });
    } catch {
      notify({ tone: "red", title: "Could not copy repair JSON", detail: "Select the JSON in the dialog and copy it manually." });
    }
  }

  async function dismissFinding(finding: SEODoctorFinding) {
    setBusyFindingID(finding.id);
    try {
      await api.dismissSEODoctorFinding(projectId, finding.id);
      notify({ tone: "green", title: "Finding dismissed" });
      await refresh();
    } catch (err: any) {
      notify({ tone: "red", title: "Could not dismiss finding", detail: err?.apiMessage || err?.message });
    } finally {
      setBusyFindingID(null);
    }
  }

  return (
    <div className="space-y-4">
      {error && <Notice title="Doctor could not load" detail={error} tone="amber" />}

      <section className="rounded-xl border border-slate-200 bg-white px-4 py-4">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-slate-50 text-[#d93820] ring-1 ring-slate-100">
                <Stethoscope size={18} />
              </span>
              <Badge tone={statusTone(run?.status)}>{run?.status ?? "not_run"}</Badge>
              {run?.trigger && <Badge tone="neutral">{run.trigger}</Badge>}
            </div>
            <h1 className="mt-3 text-2xl font-bold leading-8 text-slate-950">Site health</h1>
            <p className="mt-1 max-w-[72ch] text-sm font-semibold leading-5 text-slate-500">
              {report?.human_report?.summary ?? "Run Doctor to check crawl, index, metadata, schema, links, and report trust signals."}
            </p>
          </div>
          <div className="grid gap-3 sm:grid-cols-[120px_1fr] lg:min-w-[420px]">
            <div className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-3">
              <div className="text-xs font-bold uppercase text-slate-400">Health</div>
              <div className={cx("mt-2 text-4xl font-bold leading-none", healthTone(healthScore))}>{healthScore ?? "-"}</div>
            </div>
            <div className="rounded-lg border border-slate-200 px-3 py-3">
              <div className="flex items-center justify-between gap-3">
                <div className="text-xs font-bold uppercase text-slate-400">{run?.stage ?? "ready"}</div>
                <div className="text-xs font-bold text-slate-500">{progress}%</div>
              </div>
              <div className="mt-2 h-2 overflow-hidden rounded-full bg-slate-100">
                <div className="h-full rounded-full bg-[#d93820] transition-all duration-500" style={{ width: `${progress}%` }} />
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs font-semibold text-slate-500">
                <span>{run?.pages_checked ?? 0} checked</span>
                <span>{run?.issues_found ?? 0} issues</span>
                <span>{formatDate(run?.updated_at ?? null)}</span>
              </div>
            </div>
          </div>
        </div>
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <Button variant="primary" onClick={runDoctor} disabled={running || isActiveRun(run)}>
            <ButtonProgress busy={running || isActiveRun(run)} busyLabel={isActiveRun(run) ? "Running" : "Starting"} idleIcon={<Play size={15} />}>
              Run Doctor
            </ButtonProgress>
          </Button>
          <Button onClick={() => void refresh()} disabled={loading}>
            <ButtonProgress busy={loading} busyLabel="Refreshing" idleIcon={<RefreshCw size={15} />}>
              Refresh
            </ButtonProgress>
          </Button>
          {run?.block_reason && <Badge tone="red">{run.block_reason}</Badge>}
        </div>
      </section>

      <section className="grid gap-3 sm:grid-cols-4">
        {(["P0", "P1", "P2", "Info"] as const).map((severity) => (
          <button
            key={severity}
            type="button"
            onClick={() => setFilter(severity)}
            className={cx(
              "rounded-xl border bg-white px-4 py-3 text-left transition-colors hover:border-slate-300",
              filter === severity ? "border-[#d93820] ring-1 ring-[#d93820]" : "border-slate-200",
            )}
          >
            <div className="flex items-center justify-between">
              <Badge tone={severityTone(severity)}>{severity}</Badge>
              <span className="text-2xl font-bold text-slate-950">{counts[severity]}</span>
            </div>
            <div className="mt-2 text-xs font-semibold text-slate-500">{severity === "Info" ? "Notes" : "Active issues"}</div>
          </button>
        ))}
      </section>

      <section>
        <SectionHeader
          title="Findings"
          eyebrow="Grouped diagnostics"
          action={
            <Button size="sm" variant={filter === "all" ? "primary" : "outline"} onClick={() => setFilter("all")}>
              All findings
            </Button>
          }
        />
        {loading ? (
          <div className="grid gap-3 md:grid-cols-2">
            {[0, 1, 2, 3].map((item) => (
              <div key={item} className="h-40 animate-pulse rounded-xl border border-slate-200 bg-white p-4">
                <div className="h-4 w-24 rounded bg-slate-100" />
                <div className="mt-4 h-5 w-2/3 rounded bg-slate-100" />
                <div className="mt-3 h-4 w-full rounded bg-slate-100" />
              </div>
            ))}
          </div>
        ) : visibleFindings.length === 0 ? (
          <EmptyState title="No findings in this view" detail="Doctor has no active findings for the selected severity." />
        ) : (
          <div className="grid gap-3 lg:grid-cols-2">
            {visibleFindings.map((finding) => (
              <article key={finding.id} className="rounded-xl border border-slate-200 bg-white p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone={severityTone(finding.severity)}>{finding.severity}</Badge>
                      <Badge tone="neutral">{finding.category}</Badge>
                      <span className="truncate text-xs font-bold text-slate-400">{finding.issue_type}</span>
                    </div>
                    <h2 className="mt-3 text-base font-bold leading-6 text-slate-950">{finding.fix_intent || finding.issue_type}</h2>
                  </div>
                  {finding.status === "active" ? <AlertTriangle size={17} className="shrink-0 text-amber-600" /> : <CheckCircle2 size={17} className="shrink-0 text-green-600" />}
                </div>
                <p className="mt-2 line-clamp-2 text-sm font-semibold leading-5 text-slate-600">{finding.why_it_matters}</p>
                <div className="mt-3 rounded-lg bg-slate-50 px-3 py-2 text-xs font-semibold text-slate-500">{firstURL(finding)}</div>
                <div className="mt-3 rounded-lg border border-slate-100 px-3 py-2">
                  <div className="text-xs font-bold uppercase text-slate-400">Suggested next step</div>
                  <p className="mt-1 text-sm font-semibold leading-5 text-slate-700">{finding.developer_instructions}</p>
                </div>
                <div className="mt-3 grid gap-2 rounded-lg border border-slate-100 px-3 py-2 text-xs font-semibold text-slate-500">
                  {findingEvidence(finding).map(([label, value]) => (
                    <div key={label} className="grid gap-1 sm:grid-cols-[92px_1fr]">
                      <span className="text-slate-400">{label}</span>
                      <span className="min-w-0 break-words text-slate-700">{String(value)}</span>
                    </div>
                  ))}
                </div>
                {finding.acceptance_tests.length > 0 && (
                  <div className="mt-3 rounded-lg border border-slate-100 px-3 py-2">
                    <div className="text-xs font-bold uppercase text-slate-400">Verification</div>
                    <ul className="mt-1 space-y-1 text-sm font-semibold leading-5 text-slate-700">
                      {finding.acceptance_tests.slice(0, 3).map((item) => (
                        <li key={item}>{item}</li>
                      ))}
                    </ul>
                  </div>
                )}
                <div className="mt-4 flex flex-wrap items-center justify-between gap-2">
                  <Button size="sm" variant="ghost" onClick={() => void dismissFinding(finding)} disabled={busyFindingID === finding.id || finding.status !== "active"}>
                    <X size={14} />
                    Dismiss
                  </Button>
                  <Button
                    size="sm"
                    variant="ai"
                    onClick={() => setSelectedRepairFinding(finding)}
                    disabled={finding.status !== "active"}
                    className="ml-auto"
                  >
                    <Code2 size={14} />
                    Fix with AI
                  </Button>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      {selectedRepairFinding && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          <button
            type="button"
            aria-label="Close repair JSON"
            className="absolute inset-0 bg-slate-950/45"
            onClick={() => setSelectedRepairFinding(null)}
          />
          <section
            role="dialog"
            aria-modal="true"
            aria-labelledby="seo-doctor-ai-repair-title"
            className="relative z-10 flex max-h-[88vh] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-2xl"
          >
            <div className="flex flex-col gap-3 border-b border-slate-200 px-4 py-4 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0">
                <div className="text-xs font-bold uppercase text-cyan-700">AI coding repair JSON</div>
                <h2 id="seo-doctor-ai-repair-title" className="mt-1 text-xl font-bold leading-7 text-slate-950">
                  Fix with AI
                </h2>
                <p className="mt-1 max-w-[74ch] text-sm font-semibold leading-5 text-slate-500">
                  Copy this JSON into an AI coding tool. It includes the affected page, diagnosis evidence, repair instructions, and verification checks for this specific finding.
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Button size="sm" onClick={() => void copyAIRepairJSON(selectedRepairFinding)}>
                  <Clipboard size={14} />
                  Copy JSON
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setSelectedRepairFinding(null)}>
                  <X size={14} />
                  Close
                </Button>
              </div>
            </div>
            <pre className="max-h-[64vh] overflow-auto bg-slate-950 p-4 text-xs leading-5 text-slate-100">{selectedRepairJSON}</pre>
          </section>
        </div>
      )}
    </div>
  );
}
