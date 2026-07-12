"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { AlertTriangle, CheckCircle2, ChevronRight, Clipboard, Code2, Play, RefreshCw, Stethoscope, Wrench, X } from "lucide-react";
import { SEODoctorFinding, SEODoctorReport, SEODoctorRun, SiteFix } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, ButtonProgress, EmptyState, Notice, SectionHeader, cx, formatDate } from "../../../components/ui";
import { RightDrawer } from "../../../components/right-drawer";
import { useToast } from "../../../components/toast-provider";

type SeverityFilter = "all" | "P0" | "P1" | "P2" | "Info";

const severityOrder: Record<string, number> = { P0: 0, P1: 1, P2: 2, Info: 3 };

function isActiveRun(run?: SEODoctorRun | null) {
  return run?.status === "queued" || run?.status === "running";
}

function isLegacyHealthSentinel(finding: SEODoctorFinding) {
  return finding.issue_type === "no_active_technical_blockers";
}

function isActionableDoctorFinding(finding: SEODoctorFinding) {
  return finding.finding_kind !== "healthy" && !isLegacyHealthSentinel(finding);
}

function severityTone(severity: string): "red" | "amber" | "blue" | "neutral" {
  if (severity === "P0") return "red";
  if (severity === "P1") return "amber";
  if (severity === "P2") return "blue";
  return "neutral";
}

function findingKindLabel(finding: SEODoctorFinding) {
  return finding.finding_kind === "optimization" ? "Optimization" : "Broken";
}

function findingKindTone(finding: SEODoctorFinding): "red" | "blue" {
  return finding.finding_kind === "optimization" ? "blue" : "red";
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

function firstTimestamp(...values: unknown[]) {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return null;
}

function doctorRunTimestamp(run?: SEODoctorRun | null) {
  return firstTimestamp(run?.finished_at, run?.updated_at, run?.started_at, run?.created_at);
}

function doctorRunStageLabel(run?: SEODoctorRun | null) {
  if (!run) return "Ready";
  if (isActiveRun(run)) return `Running: ${run.stage || "checking"}`;
  if (run.status === "completed") return "Completed";
  return run.stage || run.status || "Ready";
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
  const rawDetails = repairRawDetails(finding);
  return textValue(rawDetails.canonical_url) ?? evidence.normalized_page_url ?? evidence.final_url ?? evidence.page_url ?? firstURL(finding);
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
  const observedPageMetadata = buildObservedPageMetadata(finding);

  return {
    page_role: pageRole,
    canonical_url: canonicalURL,
    schema_types: structuredDataSchemaTypes(pageRole),
    render_requirement: "Add server-rendered JSON-LD to the initial HTML so crawlers can read it without client-side interaction.",
    observed_page_metadata: observedPageMetadata,
    approved_metadata: buildApprovedStructuredDataMetadata(observedPageMetadata, canonicalURL),
    unresolved_fields: buildStructuredDataUnresolvedFields(observedPageMetadata),
    site_search_policy:
      observedPageMetadata.site_search_observed === true && observedPageMetadata.site_search_action_url
        ? "Only add SearchAction when the observed site search URL template is the real production search endpoint."
        : "No site search URL template was observed; omit potentialAction unless the site owner confirms a real production search endpoint.",
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

function repairRawDetails(finding: SEODoctorFinding) {
  const evidence = finding.evidence && typeof finding.evidence === "object" ? finding.evidence : {};
  const rawDetails = evidence.raw_details && typeof evidence.raw_details === "object" ? evidence.raw_details : {};
  return rawDetails;
}

function textValue(value: unknown) {
  return typeof value === "string" && value.trim() ? value.trim() : null;
}

function stringArray(value: unknown) {
  if (!Array.isArray(value)) return [];
  return uniqueStrings(value.map((entry) => (typeof entry === "string" ? entry : "")));
}

function firstText(...values: unknown[]) {
  for (const value of values) {
    const text = textValue(value);
    if (text) return text;
  }
  return null;
}

function inferredBrandFromTitle(title: unknown) {
  const text = textValue(title);
  if (!text) return null;
  return firstText(...text.split(/\s+\|\s+|\s+-\s+/));
}

function buildObservedPageMetadata(finding: SEODoctorFinding) {
  const rawDetails = repairRawDetails(finding);
  return compactRepairObject({
    title: textValue(rawDetails.page_title),
    meta_description: textValue(rawDetails.meta_description),
    application_name: textValue(rawDetails.application_name),
    og_site_name: textValue(rawDetails.og_site_name),
    og_title: textValue(rawDetails.og_title),
    og_description: textValue(rawDetails.og_description),
    og_image: textValue(rawDetails.og_image),
    canonical_url: textValue(rawDetails.canonical_url),
    og_url: textValue(rawDetails.og_url),
    html_lang: textValue(rawDetails.html_lang),
    logo_candidates: stringArray(rawDetails.logo_candidates),
    site_search_observed: typeof rawDetails.site_search_observed === "boolean" ? rawDetails.site_search_observed : null,
    site_search_action_url: textValue(rawDetails.site_search_action_url),
  });
}

function buildApprovedStructuredDataMetadata(observedPageMetadata: Record<string, any>, canonicalURL: string) {
  const logoCandidates = stringArray(observedPageMetadata.logo_candidates);
  return compactRepairObject({
    brandName: firstText(observedPageMetadata.og_site_name, observedPageMetadata.application_name, inferredBrandFromTitle(observedPageMetadata.title)),
    canonicalUrl: canonicalURL,
    logoUrl: firstText(...logoCandidates),
    description: firstText(observedPageMetadata.meta_description, observedPageMetadata.og_description),
    language: firstText(observedPageMetadata.html_lang),
    sameAs: stringArray(observedPageMetadata.same_as_candidates),
    contactPoint: textValue(observedPageMetadata.contact_point),
    hasSiteSearch: typeof observedPageMetadata.site_search_observed === "boolean" ? observedPageMetadata.site_search_observed : null,
  });
}

function buildStructuredDataUnresolvedFields(observedPageMetadata: Record<string, any>) {
  const unresolved: Array<{ field: string; instruction: string }> = [];
  if (!firstText(observedPageMetadata.og_site_name, inferredBrandFromTitle(observedPageMetadata.title))) {
    unresolved.push({ field: "name", instruction: "Confirm the production brand or product name from an approved source." });
  }
  if (stringArray(observedPageMetadata.logo_candidates).length === 0) {
    unresolved.push({ field: "logo", instruction: "Find a production logo URL that returns a 200 image response, or omit logo until confirmed." });
  }
  if (!firstText(observedPageMetadata.meta_description, observedPageMetadata.og_description)) {
    unresolved.push({ field: "description", instruction: "Use the page meta description or approved brand/product description; do not invent marketing copy." });
  }
  if (!firstText(observedPageMetadata.html_lang)) {
    unresolved.push({ field: "inLanguage", instruction: "Use the page html lang, locale config, or canonical locale before adding inLanguage." });
  }
  if (stringArray(observedPageMetadata.same_as_candidates).length === 0) {
    unresolved.push({ field: "sameAs", instruction: "Omit sameAs unless official social or profile URLs are available." });
  }
  if (!textValue(observedPageMetadata.contact_point)) {
    unresolved.push({ field: "contactPoint", instruction: "Omit contactPoint unless public support or contact details are available." });
  }
  if (observedPageMetadata.site_search_observed !== true || !textValue(observedPageMetadata.site_search_action_url)) {
    unresolved.push({ field: "potentialAction", instruction: "No site search URL template was observed; omit potentialAction unless the site has a real production search endpoint." });
  }
  return unresolved;
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
  const observedPageMetadata = buildObservedPageMetadata(finding);

  return compactRepairObject({
    page_url: evidence.page_url ?? firstURL(finding),
    normalized_page_url: evidence.normalized_page_url ?? finding.normalized_urls[0] ?? null,
    status: rawDetails.status ?? evidence.status ?? null,
    final_url: rawDetails.final_url ?? evidence.final_url ?? null,
    confidence: evidence.confidence_label ?? evidence.confidence ?? null,
    observed_page_metadata: observedPageMetadata,
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

export function DoctorClient({ projectId, initialFindingId }: { projectId: string; initialFindingId?: string }) {
  const api = useApi();
  const router = useRouter();
  const { notify } = useToast();
  const pendingRunNoticeID = useRef<string | null>(null);
  const initialSelectionHandled = useRef(false);
  const [report, setReport] = useState<SEODoctorReport | null>(null);
  const [siteFixes, setSiteFixes] = useState<SiteFix[]>([]);
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<SeverityFilter>("all");
  const [busyFindingID, setBusyFindingID] = useState<string | null>(null);
  const [busyKind, setBusyKind] = useState<"dismiss" | "add" | null>(null);
  const [selectedFindingID, setSelectedFindingID] = useState<string | null>(null);
  const surfaceRef = useRef<HTMLDivElement | null>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const [next, fixes] = await Promise.all([
        api.getSEODoctor(projectId),
        api.listDoctorSiteFixes(projectId).catch(() => []),
      ]);
      setReport(next);
      setSiteFixes(fixes);
      const pendingRunID = pendingRunNoticeID.current;
      if (pendingRunID && next.run?.id === pendingRunID && !isActiveRun(next.run)) {
        pendingRunNoticeID.current = null;
        if (next.run.status === "failed" || next.run.status === "blocked") {
          notify({ tone: "red", title: "Doctor did not finish", detail: next.run.error || next.run.block_reason || "Review the run status and try again." });
        } else {
          notify({ tone: "green", title: "Doctor refreshed", detail: "The report now reflects the latest crawl checks." });
        }
      }
      return next;
    } catch (err: any) {
      setError(err?.apiMessage || err?.message || "Could not load Doctor.");
      return null;
    } finally {
      setLoading(false);
    }
  }, [api, notify, projectId]);

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
    const findings = sortedFindings((report?.findings ?? []).filter(isActionableDoctorFinding));
    return filter === "all" ? findings : findings.filter((finding) => finding.severity === filter);
  }, [filter, report?.findings]);
  const healthyCoverage = run?.healthy_coverage ?? [];
  const progress = Math.max(0, Math.min(100, run?.progress_percent ?? 0));
  const healthScore = run?.health_score ?? report?.human_report?.health_score ?? null;
  const lastRunAt = doctorRunTimestamp(run);
  const stageLabel = doctorRunStageLabel(run);
  const repairCounts = siteFixes.reduce(
    (counts, fix) => {
      if (["proposed", "draft"].includes(fix.status)) counts.proposed += 1;
      else if (fix.status === "approved") counts.approved += 1;
      else if (["applying", "applied", "awaiting_deploy", "verifying", "failed_retryable", "reopened"].includes(fix.status)) counts.executing += 1;
      else if (fix.status === "verified") counts.verified += 1;
      else if (["failed_terminal", "terminated"].includes(fix.status)) counts.attention += 1;
      return counts;
    },
    { proposed: 0, approved: 0, executing: 0, verified: 0, attention: 0 },
  );
  const selectedFinding = useMemo(
    () => visibleFindings.find((finding) => finding.id === selectedFindingID) ?? null,
    [visibleFindings, selectedFindingID],
  );
  const selectedRepairJSON = useMemo(() => {
    return selectedFinding ? JSON.stringify(buildAIRepairPayload(selectedFinding), null, 2) : "";
  }, [selectedFinding]);

  useEffect(() => {
    if (loading || initialSelectionHandled.current) return;
    initialSelectionHandled.current = true;
    if (initialFindingId && (report?.findings ?? []).some((finding) => finding.id === initialFindingId && isActionableDoctorFinding(finding))) {
      setFilter("all");
      setSelectedFindingID(initialFindingId);
    }
  }, [initialFindingId, loading, report?.findings]);

  useEffect(() => {
    if (selectedFindingID && !selectedFinding) setSelectedFindingID(null);
  }, [selectedFindingID, selectedFinding]);

  async function runDoctor() {
    setRunning(true);
    setError(null);
    try {
      const nextRun = await api.startSEODoctorRun(projectId);
      pendingRunNoticeID.current = nextRun.id;
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
    setBusyKind("dismiss");
    try {
      await api.dismissSEODoctorFinding(projectId, finding.id);
      notify({ tone: "green", title: "Finding dismissed" });
      setSelectedFindingID(null);
      await refresh();
    } catch (err: any) {
      notify({ tone: "red", title: "Could not dismiss finding", detail: err?.apiMessage || err?.message });
    } finally {
      setBusyFindingID(null);
      setBusyKind(null);
    }
  }

  async function addToSiteFixes(finding: SEODoctorFinding) {
    setBusyFindingID(finding.id);
    setBusyKind("add");
    try {
      const fix = await api.createDoctorSiteFix(projectId, finding.id);
      notify({
        tone: "green",
        title: "Added to Site Fixes",
        detail: "Review, approve, apply, deploy, and verify the canonical Site Fix.",
      });
      setSelectedFindingID(null);
      router.push(`/projects/${projectId}/site-fixes?fix=${fix.id}`);
    } catch (err: any) {
      notify({ tone: "red", title: "Could not add to Site Fixes", detail: err?.apiMessage || err?.message });
    } finally {
      setBusyFindingID(null);
      setBusyKind(null);
    }
  }

  return (
    <>
      <div className="space-y-4" ref={surfaceRef}>
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
            <div className="mt-3 flex flex-wrap items-center gap-2 text-xs font-semibold text-slate-500">
              <span className="inline-flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-2.5 py-1.5">
                <span className="font-bold uppercase text-slate-400">Last run</span>
                <span>{lastRunAt ? formatDate(lastRunAt) : "Never"}</span>
              </span>
              <span className="inline-flex items-center rounded-lg border border-slate-200 px-2.5 py-1.5">Uses the latest crawl checks</span>
            </div>
          </div>
          <div className="grid gap-3 sm:grid-cols-[120px_1fr] lg:min-w-[420px]">
            <div className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-3">
              <div className="text-xs font-bold uppercase text-slate-400">Health</div>
              <div className={cx("mt-2 text-4xl font-bold leading-none", healthTone(healthScore))}>{healthScore ?? "-"}</div>
            </div>
            <div className="rounded-lg border border-slate-200 px-3 py-3">
              <div className="flex items-center justify-between gap-3">
                <div className="text-xs font-bold uppercase text-slate-400">{stageLabel}</div>
                <div className="text-xs font-bold text-slate-500">{progress}%</div>
              </div>
              <div className="mt-2 h-2 overflow-hidden rounded-full bg-slate-100">
                <div className="h-full rounded-full bg-[#d93820] transition-all duration-500" style={{ width: `${progress}%` }} />
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs font-semibold text-slate-500">
                <span>{isActiveRun(run) ? `${run?.pages_checked ?? 0} checked so far` : `Last result: ${run?.pages_checked ?? 0} checked`}</span>
                <span>{run?.issues_found ?? 0} issues</span>
                <span>Last run: {lastRunAt ? formatDate(lastRunAt) : "Never"}</span>
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
        <p className="mt-2 text-xs font-semibold leading-5 text-slate-500">
          Run Doctor refreshes this report from the latest crawl checks. Refresh SEO data first when you need a fresh recrawl.
        </p>
      </section>

      <section data-doctor-repair-loop className="space-y-3">
        <SectionHeader
          title="Doctor repair loop"
          eyebrow="Immediate verification"
          action={
            <a
              href={`/projects/${projectId}/site-fixes`}
              className="inline-flex h-8 items-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-bold text-slate-700 transition hover:bg-slate-50"
            >
              Open Site Fixes
            </a>
          }
        />
        <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
            {[
              { label: "Proposed", value: repairCounts.proposed, tone: "neutral" as const },
              { label: "Approved", value: repairCounts.approved, tone: "blue" as const },
              { label: "Applied / deploying", value: repairCounts.executing, tone: "amber" as const },
              { label: "Verified", value: repairCounts.verified, tone: "green" as const },
              { label: "Needs attention", value: repairCounts.attention, tone: "red" as const },
            ].map((item) => (
              <a
                key={item.label}
                href={`/projects/${projectId}/site-fixes`}
                className="rounded-lg border border-slate-100 bg-slate-50 px-3 py-3 transition hover:border-slate-300 hover:bg-white"
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="text-xs font-bold uppercase text-slate-500">{item.label}</span>
                  <Badge tone={item.tone}>{item.value}</Badge>
                </div>
              </a>
            ))}
          </div>
          <p className="mt-3 text-sm font-semibold leading-6 text-slate-600">
            Doctor ends when acceptance tests re-read the repaired evidence and mark the Site Fix verified. It never enters Growth Measuring.
          </p>
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
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {[0, 1, 2].map((item) => (
              <div key={item} className="h-48 animate-pulse rounded-xl border border-slate-200 bg-white p-4">
                <div className="h-4 w-24 rounded bg-slate-100" />
                <div className="mt-4 h-5 w-2/3 rounded bg-slate-100" />
                <div className="mt-3 h-4 w-full rounded bg-slate-100" />
              </div>
            ))}
          </div>
        ) : visibleFindings.length === 0 ? (
          <EmptyState title="No findings in this view" detail="Doctor has no active findings for the selected severity." />
        ) : (
          <div data-doctor-findings-grid className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {visibleFindings.map((finding) => (
              <button
                key={finding.id}
                type="button"
                data-doctor-finding-card
                aria-label={`Open finding details: ${finding.fix_intent || finding.issue_type}`}
                onClick={(event) => {
                  returnFocusRef.current = event.currentTarget;
                  setSelectedFindingID(finding.id);
                }}
                className={cx(
                  "group flex h-full min-h-[200px] w-full flex-col rounded-xl border bg-white p-4 text-left shadow-sm transition hover:border-slate-300 hover:bg-slate-50/60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px",
                  selectedFindingID === finding.id ? "border-slate-400 ring-2 ring-slate-200" : "border-slate-200",
                )}
              >
                <div className="flex h-full min-w-0 flex-col justify-between gap-4">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone={findingKindTone(finding)}>{findingKindLabel(finding)}</Badge>
                      <Badge tone={severityTone(finding.severity)}>{finding.severity}</Badge>
                      <Badge tone="neutral">{finding.category}</Badge>
                      {finding.status === "active" ? (
                        <AlertTriangle size={15} className="shrink-0 text-amber-600" />
                      ) : (
                        <CheckCircle2 size={15} className="shrink-0 text-green-600" />
                      )}
                    </div>
                    <h2 className="mt-2 line-clamp-2 text-base font-bold leading-6 text-slate-950">{finding.fix_intent || finding.issue_type}</h2>
                    <p className="mt-1 truncate text-xs font-bold text-slate-400">{finding.issue_type}</p>
                  </div>
                  <div>
                    <p className="line-clamp-2 text-sm font-semibold leading-5 text-slate-600">{finding.why_it_matters}</p>
                    <div className="mt-3 truncate rounded-lg bg-slate-50 px-3 py-2 text-xs font-semibold text-slate-500">{firstURL(finding)}</div>
                  </div>
                  <div className="mt-auto flex items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm font-semibold text-slate-700">
                    <span>{finding.status === "active" ? "Next: create Site Fix" : `Status: ${finding.status}`}</span>
                    <ChevronRight className="text-slate-400 transition group-hover:translate-x-0.5 group-hover:text-slate-600" size={17} />
                  </div>
                </div>
              </button>
            ))}
          </div>
        )}
      </section>

      <section className="space-y-3">
        <SectionHeader title="Healthy coverage" eyebrow="Checks that passed" />
        {healthyCoverage.length === 0 ? (
          <EmptyState title="No healthy coverage recorded" detail="Run Doctor to record structured passed, failed, and skipped coverage for each check." />
        ) : (
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {healthyCoverage.map((coverage) => (
              <article key={coverage.check} className="rounded-xl border border-slate-200 bg-white p-4">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <h2 className="font-bold text-slate-950">
                      {coverage.check.replaceAll("_", " ")}
                    </h2>
                    <p className="mt-1 text-xs font-semibold text-slate-500">Structured coverage, not actionable work</p>
                  </div>
                  <Badge tone={coverage.failed_urls.length ? "amber" : "green"}>
                    {coverage.passed_urls.length} passed
                  </Badge>
                </div>
                <div className="mt-3 flex flex-wrap gap-2 text-xs font-semibold text-slate-500">
                  <span>{coverage.checked_urls.length} checked</span>
                  <span>{coverage.failed_urls.length} failed</span>
                  <span>{coverage.skipped_urls.length} skipped</span>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>
      </div>

      <RightDrawer
        open={Boolean(selectedFinding)}
        onClose={() => setSelectedFindingID(null)}
        dataAttribute="data-doctor-finding-drawer"
        eyebrow="Finding details"
        title={selectedFinding?.fix_intent || selectedFinding?.issue_type || "Finding"}
        subtitle={selectedFinding ? firstURL(selectedFinding) : undefined}
        maxWidthClassName="max-w-3xl"
        surfaceRef={surfaceRef}
        returnFocusRef={returnFocusRef}
        badges={
          selectedFinding ? (
            <>
              <Badge tone={findingKindTone(selectedFinding)}>{findingKindLabel(selectedFinding)}</Badge>
              <Badge tone={severityTone(selectedFinding.severity)}>{selectedFinding.severity}</Badge>
              <Badge tone="neutral">{selectedFinding.category}</Badge>
              <Badge tone="neutral">{selectedFinding.issue_type}</Badge>
              <Badge tone={selectedFinding.status === "active" ? "amber" : "green"}>{selectedFinding.status}</Badge>
            </>
          ) : undefined
        }
        footer={
          selectedFinding ? (
            <>
              <Button
                size="sm"
                variant="ai"
                onClick={() => void addToSiteFixes(selectedFinding)}
                disabled={busyFindingID === selectedFinding.id || selectedFinding.status !== "active" || !isActionableDoctorFinding(selectedFinding)}
              >
                <ButtonProgress
                  busy={busyFindingID === selectedFinding.id && busyKind === "add"}
                  busyLabel="Adding"
                  idleIcon={<Wrench size={14} />}
                >
                  Add to Site Fixes
                </ButtonProgress>
              </Button>
              <Button size="sm" variant="outline" onClick={() => void copyAIRepairJSON(selectedFinding)}>
                <Clipboard size={14} />
                Copy fix JSON
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => void dismissFinding(selectedFinding)}
                disabled={busyFindingID === selectedFinding.id || selectedFinding.status !== "active"}
              >
                <ButtonProgress
                  busy={busyFindingID === selectedFinding.id && busyKind === "dismiss"}
                  busyLabel="Dismissing"
                  idleIcon={<X size={14} />}
                >
                  Dismiss
                </ButtonProgress>
              </Button>
            </>
          ) : undefined
        }
      >
        {selectedFinding && (
          <div className="space-y-5">
            <section className="rounded-xl border border-slate-200 bg-slate-50 p-4">
              <div className="text-xs font-bold uppercase text-slate-400">Why it matters</div>
              <p className="mt-2 text-sm font-semibold leading-6 text-slate-700">{selectedFinding.why_it_matters}</p>
            </section>

            <section className="rounded-xl border border-slate-200 p-4">
              <div className="text-xs font-bold uppercase text-slate-400">Suggested next step</div>
              <p className="mt-1 text-sm font-semibold leading-5 text-slate-700">{selectedFinding.developer_instructions}</p>
            </section>

            <section className="rounded-xl border border-slate-200 p-4">
              <div className="text-xs font-bold uppercase text-slate-400">Evidence</div>
              <div className="mt-2 grid gap-2 text-xs font-semibold text-slate-500">
                {findingEvidence(selectedFinding).map(([label, value]) => (
                  <div key={label} className="grid gap-1 sm:grid-cols-[110px_1fr]">
                    <span className="text-slate-400">{label}</span>
                    <span className="min-w-0 break-words text-slate-700">{String(value)}</span>
                  </div>
                ))}
              </div>
            </section>

            {selectedFinding.acceptance_tests.length > 0 && (
              <section className="rounded-xl border border-slate-200 p-4">
                <div className="text-xs font-bold uppercase text-slate-400">Verification</div>
                <ul className="mt-1 list-disc space-y-1 pl-5 text-sm font-semibold leading-5 text-slate-700">
                  {selectedFinding.acceptance_tests.slice(0, 5).map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </section>
            )}

            <section data-doctor-ai-payload className="overflow-hidden rounded-xl border border-cyan-200 bg-cyan-50">
              <div className="flex items-center gap-2 border-b border-cyan-100 px-4 py-3 text-xs font-bold uppercase text-cyan-800">
                <Code2 size={14} />
                AI coding fix JSON
              </div>
              <p className="px-4 pt-3 text-sm font-semibold leading-5 text-cyan-950">
                Copy this JSON into Codex or Claude Code. It names the affected page, diagnosis evidence, repair instructions, and verification checks for this finding.
              </p>
              <pre className="mt-3 max-h-80 overflow-auto bg-slate-950 p-4 text-xs leading-5 text-slate-100">{selectedRepairJSON}</pre>
            </section>
          </div>
        )}
      </RightDrawer>
    </>
  );
}
