"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { CheckCircle2, ChevronRight, Clipboard, Code2, FileText, RefreshCw, Wrench, X } from "lucide-react";
import { PublisherConnection, ResultsAction, SEOContentAction, VisibilitySummary } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";
import { RightDrawer } from "../../../components/right-drawer";
import { Badge, Button, ButtonProgress, EmptyState, Notice, SectionHeader, formatDate } from "../../../components/ui";
import { deriveVisibilityLifecycleStage } from "../../../lib/visibility-lifecycle";
import {
  actionOutputPreviewText,
  actionOutputTypeLabel,
  actionPostExecutionText,
  actionSEOContributionText,
  actionWhyNowText,
  approvalSourceLabel,
  hasActionVerificationSnapshot,
  hasResultsExecutionEvidence,
  humanizeInternalType,
  isDirectAction,
  lifecycleStageLabel,
  lifecycleStageTone,
  measurementWindowLabel,
  siteFixAIJSON,
  siteFixAlreadyMatchesSource,
  siteFixGitHubPRURL,
  siteFixPRLinkLabel,
  siteFixVerificationLabel,
  toneForStatus,
} from "../../../lib/site-fix";

// A site-fix item is a content action, optionally enriched with the loop-summary
// fields (opportunity_status, topic_title, …) when it is present in both places.
type SiteFixAction = SEOContentAction & { opportunity_status?: string | null };

const TERMINAL_STATUSES = ["published", "measuring", "completed", "archived", "dismissed"];
const RESULT_STAGES = new Set(["published_or_applied", "measuring", "learned"]);

function normalizedStatus(value?: string | null) {
  return String(value ?? "").trim().toLowerCase();
}

// Mirror of seo-client's isVisibleLoopAction: hide archived/dismissed work and
// actions whose source opportunity was dismissed unless they already produced results.
function isVisibleSiteFix(action: SiteFixAction) {
  if (["archived", "dismissed"].includes(normalizedStatus(action.status))) return false;
  if (["archived", "dismissed"].includes(normalizedStatus(action.opportunity_status)) && !hasResultsExecutionEvidence(action)) return false;
  return true;
}

async function writeClipboardText(text: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // fall through to the textarea fallback
    }
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  document.body.appendChild(textarea);
  textarea.select();
  const copied = document.execCommand("copy");
  textarea.remove();
  if (!copied) throw new Error("Clipboard write failed.");
}

export function SiteFixesClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const { notify } = useToast();
  const [actions, setActions] = useState<SEOContentAction[]>([]);
  const [summary, setSummary] = useState<VisibilitySummary | null>(null);
  const [publisherConnections, setPublisherConnections] = useState<PublisherConnection[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [showAll, setShowAll] = useState(false);

  const surfaceRef = useRef<HTMLDivElement | null>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const [actionRows, summaryRow, connections] = await Promise.all([
        api.listSEOContentActions(projectId, { limit: 100 }),
        api.getVisibilitySummary(projectId).catch(() => null),
        api.listPublisherConnections(projectId).catch(() => []),
      ]);
      setActions(actionRows);
      setSummary(summaryRow);
      setPublisherConnections(connections);
    } catch (err: any) {
      setError(err?.apiMessage || err?.message || "Could not load Site Fixes.");
    } finally {
      setLoading(false);
    }
  }, [api, projectId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const hasConnectedGitHubPublisher = useMemo(
    () =>
      publisherConnections.some(
        (connection) => connection.kind === "github_nextjs" && connection.enabled && connection.status === "connected",
      ),
    [publisherConnections],
  );

  // Merge loop-summary actions (enriched with the full action when present) with
  // the remaining content actions — same shape seo-client derives.
  const loopActions = useMemo<SiteFixAction[]>(() => {
    const actionsByID = new Map(actions.map((action) => [action.id, action]));
    const summaryLoop = (summary?.actions_in_loop ?? []).map((summaryAction) => {
      const matching = actionsByID.get(summaryAction.id);
      return { ...summaryAction, ...(matching ?? {}) } as SiteFixAction;
    });
    const summaryIDs = new Set(summaryLoop.map((action) => action.id));
    const rest = actions.filter((action) => !summaryIDs.has(action.id)) as SiteFixAction[];
    return [...summaryLoop, ...rest];
  }, [actions, summary]);

  const visibleSiteFixes = useMemo(() => loopActions.filter(isVisibleSiteFix).filter((action) => isDirectAction(action)), [loopActions]);

  const activeAll = useMemo(
    () => visibleSiteFixes.filter((action) => !TERMINAL_STATUSES.includes(action.status)),
    [visibleSiteFixes],
  );
  const active = showAll ? activeAll : activeAll.slice(0, 9);

  const recentlyFixed = useMemo(
    () =>
      visibleSiteFixes
        .filter((action) => {
          const stage = deriveVisibilityLifecycleStage(action);
          return RESULT_STAGES.has(stage) || (stage === "blocked" && hasResultsExecutionEvidence(action));
        })
        .slice()
        .sort((a, b) => {
          const left = a.verified_at ?? a.published_at ?? a.created_at ?? "";
          const right = b.verified_at ?? b.published_at ?? b.created_at ?? "";
          return right.localeCompare(left);
        }),
    [visibleSiteFixes],
  );

  const selected = useMemo(
    () => (selectedID ? visibleSiteFixes.find((action) => action.id === selectedID) ?? null : null),
    [selectedID, visibleSiteFixes],
  );

  useEffect(() => {
    if (selectedID && !selected) setSelectedID(null);
  }, [selectedID, selected]);

  function patchAction(updated: SEOContentAction) {
    setActions((current) => current.map((item) => (item.id === updated.id ? updated : item)));
    setSummary((current) =>
      current
        ? { ...current, actions_in_loop: (current.actions_in_loop ?? []).map((item) => (item.id === updated.id ? { ...item, ...updated } : item)) }
        : current,
    );
  }

  function removeAction(id: string) {
    setActions((current) => current.filter((item) => item.id !== id));
    setSummary((current) =>
      current ? { ...current, actions_in_loop: (current.actions_in_loop ?? []).filter((item) => item.id !== id) } : current,
    );
  }

  async function verifyApplied(action: SiteFixAction) {
    setBusy(`verify-${action.id}`);
    try {
      const updated = await api.verifySEOContentAction(projectId, action.id, {
        status: "verified",
        verification_snapshot: { source: "manual_dashboard", status: "verified" },
      });
      patchAction(updated);
      notify({ tone: "green", title: "Site fix marked applied", detail: action.action_type });
    } catch (err: any) {
      notify({ tone: "red", title: "Could not mark applied", detail: err?.apiMessage || err?.message });
    } finally {
      setBusy(null);
    }
  }

  async function createPR(action: SiteFixAction) {
    setBusy(`pr-${action.id}`);
    try {
      const updated = await api.createSiteFixGitHubPR(projectId, action.id);
      patchAction(updated);
      if (siteFixAlreadyMatchesSource(updated)) {
        notify({ tone: "green", title: "Source already matches", detail: "No PR was needed. Verify production, then mark the Site Fix applied." });
      } else {
        notify({ tone: "green", title: "GitHub PR created", detail: siteFixGitHubPRURL(updated) || "Open it from this Site Fix once GitHub returns the PR URL." });
      }
    } catch (err: any) {
      notify({ tone: "red", title: "Could not create GitHub PR", detail: err?.apiMessage || err?.message });
    } finally {
      setBusy(null);
    }
  }

  async function dismissSiteFix(action: SiteFixAction) {
    setBusy(`dismiss-${action.id}`);
    try {
      const updated = await api.dismissSEOContentAction(projectId, action.id);
      removeAction(updated.id);
      setSelectedID(null);
      notify({ tone: "neutral", title: "Site fix dismissed", detail: action.action_type });
    } catch (err: any) {
      notify({ tone: "red", title: "Could not dismiss site fix", detail: err?.apiMessage || err?.message });
    } finally {
      setBusy(null);
    }
  }

  async function copyFixJSON(action: SiteFixAction) {
    try {
      await writeClipboardText(siteFixAIJSON(action));
      notify({ tone: "green", title: "Fix JSON copied", detail: "Paste it into Codex or Claude Code to apply the site fix." });
    } catch {
      notify({ tone: "red", title: "Could not copy fix JSON", detail: "Select the JSON in the drawer and copy it manually." });
    }
  }

  function renderCard(action: SiteFixAction) {
    const stage = deriveVisibilityLifecycleStage(action);
    const verificationLabel = siteFixVerificationLabel(action);
    return (
      <button
        key={action.id}
        type="button"
        data-site-fix-card
        aria-label={`Open site fix details: ${action.action_type}`}
        onClick={(event) => {
          returnFocusRef.current = event.currentTarget;
          setSelectedID(action.id);
        }}
        className={`group flex h-full min-h-[220px] w-full flex-col rounded-lg border bg-white p-4 text-left shadow-sm transition hover:border-slate-300 hover:bg-slate-50/60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px ${
          selectedID === action.id ? "border-slate-400 ring-2 ring-slate-200" : "border-slate-200"
        }`}
      >
        <div className="flex h-full min-w-0 flex-col justify-between gap-4">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone={lifecycleStageTone(stage)}>{lifecycleStageLabel(stage)}</Badge>
              <Badge tone="blue">Fix Site Issue</Badge>
              <Badge tone="neutral">{approvalSourceLabel(action.approval_source)}</Badge>
              <Badge tone={action.review_required === false ? "neutral" : "amber"}>
                {action.review_required === false ? "Review optional" : "Review required"}
              </Badge>
              {verificationLabel && <Badge tone="green">{verificationLabel}</Badge>}
            </div>
            <h3 className="mt-2 truncate text-base font-bold leading-6 text-slate-950">
              {action.action_type.includes("_") ? humanizeInternalType(action.action_type) : action.action_type}
            </h3>
            <p className="mt-1 truncate text-sm leading-5 text-slate-500">{action.target_url ?? action.normalized_target_url ?? action.id}</p>
          </div>
          <div className="grid gap-3 text-sm">
            <div>
              <div className="text-xs font-semibold uppercase text-slate-400">Why now</div>
              <div className="mt-1 line-clamp-2 font-medium leading-5 text-slate-700">{actionWhyNowText(action)}</div>
            </div>
            <div>
              <div className="text-xs font-semibold uppercase text-slate-400">Reviewable output</div>
              <div className="mt-1 line-clamp-2 font-medium leading-5 text-slate-700">{actionOutputPreviewText(action)}</div>
            </div>
          </div>
          <div className="mt-auto flex items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm font-semibold text-slate-700">
            <span>Open details</span>
            <ChevronRight className="text-slate-400 transition group-hover:translate-x-0.5 group-hover:text-slate-600" size={17} />
          </div>
        </div>
      </button>
    );
  }

  const drawerAction = selected;
  const prURL = drawerAction ? siteFixGitHubPRURL(drawerAction) : "";
  const prLabel = drawerAction ? siteFixPRLinkLabel(drawerAction) : "Open PR";
  const verificationLabel = drawerAction ? siteFixVerificationLabel(drawerAction) : "";
  const sourceAlreadyMatches = drawerAction ? siteFixAlreadyMatchesSource(drawerAction) : false;

  return (
    <>
      <div className="space-y-4" ref={surfaceRef}>
      {error && <Notice title="Site Fixes could not load" detail={error} tone="amber" />}

      <section className="rounded-xl border border-slate-200 bg-white px-4 py-4">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-slate-50 text-[#d93820] ring-1 ring-slate-100">
                <Wrench size={18} />
              </span>
              <Badge tone={activeAll.length ? "amber" : "neutral"}>{activeAll.length} to review</Badge>
            </div>
            <h1 className="mt-3 text-2xl font-bold leading-8 text-slate-950">Site Fixes</h1>
            <p className="mt-1 max-w-[72ch] text-sm font-semibold leading-5 text-slate-500">
              Approved schema, internal link, crawler, canonical, and metadata fixes. Create a source-backed GitHub PR, then track it through
              merged, applied, and verified.
            </p>
          </div>
          <Button onClick={() => void refresh()} disabled={loading}>
            <ButtonProgress busy={loading} busyLabel="Refreshing" idleIcon={<RefreshCw size={15} />}>
              Refresh
            </ButtonProgress>
          </Button>
        </div>
      </section>

      <section className="space-y-3">
        <SectionHeader title="To review" eyebrow="Approved site work" />
        {loading ? (
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {[0, 1, 2].map((item) => (
              <div key={item} className="h-56 animate-pulse rounded-lg border border-slate-200 bg-white p-4" />
            ))}
          </div>
        ) : active.length === 0 ? (
          <EmptyState
            title="No site fixes to review"
            detail="Approved schema, internal link, crawler, canonical, and metadata fixes will appear here — including fixes you add from Doctor."
          />
        ) : (
          <div data-site-fixes-grid className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {active.map(renderCard)}
            {!showAll && activeAll.length > active.length && (
              <button
                type="button"
                onClick={() => setShowAll(true)}
                className="rounded-lg border border-dashed border-slate-300 bg-white px-4 py-3 text-sm font-semibold text-slate-600 transition hover:border-slate-400 hover:bg-slate-50"
              >
                Show all site fixes ({activeAll.length})
              </button>
            )}
          </div>
        )}
      </section>

      {recentlyFixed.length > 0 && (
        <section className="space-y-3">
          <SectionHeader title="Recently fixed" eyebrow="Applied, measuring, or verified" />
          <div data-site-fixes-recent-grid className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {recentlyFixed.map(renderCard)}
          </div>
        </section>
      )}
      </div>

      <RightDrawer
        open={Boolean(drawerAction)}
        onClose={() => setSelectedID(null)}
        dataAttribute="data-site-fix-drawer"
        eyebrow="Review site fix details"
        title={drawerAction?.action_type ?? "Site fix"}
        subtitle={drawerAction ? drawerAction.target_url ?? drawerAction.normalized_target_url ?? drawerAction.id : undefined}
        surfaceRef={surfaceRef}
        returnFocusRef={returnFocusRef}
        badges={
          drawerAction ? (
            <>
              <Badge tone={lifecycleStageTone(deriveVisibilityLifecycleStage(drawerAction))}>
                {lifecycleStageLabel(deriveVisibilityLifecycleStage(drawerAction))}
              </Badge>
              <Badge tone="blue">{actionOutputTypeLabel(drawerAction)}</Badge>
              <Badge tone={toneForStatus(drawerAction.status)}>{drawerAction.status}</Badge>
              <Badge tone={drawerAction.review_required === false ? "neutral" : "amber"}>
                {drawerAction.review_required === false ? "Review optional" : "Review required"}
              </Badge>
              {verificationLabel && <Badge tone="green">{verificationLabel}</Badge>}
            </>
          ) : undefined
        }
        footer={
          drawerAction ? (
            <>
              {hasConnectedGitHubPublisher ? (
                prURL ? (
                  <Button
                    size="sm"
                    variant="ai"
                    className="min-w-[9.5rem] shrink-0 whitespace-nowrap px-4 sm:w-auto"
                    onClick={() => window.open(prURL, "_blank", "noopener,noreferrer")}
                  >
                    <FileText className="shrink-0" size={14} />
                    {prLabel}
                  </Button>
                ) : sourceAlreadyMatches ? (
                  <Button size="sm" variant="ai" className="min-w-[9.5rem] shrink-0 whitespace-nowrap px-4 sm:w-auto" disabled>
                    <CheckCircle2 className="shrink-0" size={14} />
                    Source matches
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    variant="ai"
                    className="min-w-[9.5rem] shrink-0 whitespace-nowrap px-4 sm:w-auto"
                    onClick={() => void createPR(drawerAction)}
                    disabled={!!busy}
                  >
                    <ButtonProgress busy={busy === `pr-${drawerAction.id}`} busyLabel="Creating PR" idleIcon={<FileText size={14} />}>
                      Create GitHub PR
                    </ButtonProgress>
                  </Button>
                )
              ) : (
                <Button
                  size="sm"
                  variant="ai"
                  className="min-w-[9.5rem] shrink-0 whitespace-nowrap px-4 sm:w-auto"
                  onClick={() => void copyFixJSON(drawerAction)}
                >
                  <Clipboard className="shrink-0" size={14} />
                  Copy fix JSON
                </Button>
              )}
              {!drawerAction.verified_at && (
                <Button size="sm" onClick={() => void verifyApplied(drawerAction)} disabled={!!busy}>
                  <ButtonProgress busy={busy === `verify-${drawerAction.id}`} busyLabel="Marking applied" idleIcon={<CheckCircle2 size={14} />}>
                    Mark applied
                  </ButtonProgress>
                </Button>
              )}
              <Button size="sm" variant="ghost" onClick={() => void dismissSiteFix(drawerAction)} disabled={!!busy}>
                <ButtonProgress busy={busy === `dismiss-${drawerAction.id}`} busyLabel="Dismissing" idleIcon={null}>
                  Dismiss
                </ButtonProgress>
              </Button>
            </>
          ) : undefined
        }
      >
        {drawerAction && (
          <div className="space-y-5">
            <section className="rounded-xl border border-slate-200 bg-slate-50 p-4">
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Reviewable output</div>
              <p className="mt-2 text-sm font-medium leading-6 text-slate-700">{actionOutputPreviewText(drawerAction)}</p>
            </section>

            <section className="rounded-xl border border-slate-200 p-4">
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Action timeline</div>
              <div className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
                <div>
                  <div className="text-xs font-semibold uppercase text-slate-400">Created</div>
                  <div className="mt-1 font-medium text-slate-700">{formatDate(drawerAction.created_at ?? null)}</div>
                </div>
                <div>
                  <div className="text-xs font-semibold uppercase text-slate-400">Approved</div>
                  <div className="mt-1 font-medium text-slate-700">{formatDate(drawerAction.approved_at ?? null)}</div>
                </div>
                <div>
                  <div className="text-xs font-semibold uppercase text-slate-400">Applied</div>
                  <div className="mt-1 font-medium text-slate-700">{formatDate(drawerAction.verified_at ?? drawerAction.published_at ?? null)}</div>
                </div>
                <div>
                  <div className="text-xs font-semibold uppercase text-slate-400">Last updated</div>
                  <div className="mt-1 font-medium text-slate-700">{formatDate(drawerAction.updated_at ?? null)}</div>
                </div>
              </div>
            </section>

            <section data-site-fix-ai-payload className="overflow-hidden rounded-xl border border-cyan-200 bg-cyan-50">
              <div className="flex flex-col gap-3 border-b border-cyan-100 px-4 py-4">
                <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.12em] text-cyan-800">
                  <Code2 size={14} />
                  AI coding fix JSON
                </div>
                <p className="text-sm font-semibold leading-5 text-cyan-950">
                  {sourceAlreadyMatches
                    ? "The mapped source already contains this fix. Verify production before marking it applied."
                    : hasConnectedGitHubPublisher
                      ? "Create a source-backed GitHub PR for this existing page when CiteLoop can map the fix to the published source file."
                      : "Copy this JSON into Codex or Claude Code. It names the target page, concrete patch contract, likely files or surfaces, and verification checks."}
                </p>
              </div>
              <pre className="max-h-80 overflow-auto bg-slate-950 p-4 text-xs leading-5 text-slate-100">{siteFixAIJSON(drawerAction)}</pre>
            </section>

            <section className="grid gap-3 text-sm sm:grid-cols-2">
              <div>
                <div className="text-xs font-semibold uppercase text-slate-400">Output type</div>
                <div className="mt-1 font-medium leading-5 text-slate-700">{actionOutputTypeLabel(drawerAction)}</div>
              </div>
              <div>
                <div className="text-xs font-semibold uppercase text-slate-400">Asset type</div>
                <div className="mt-1 break-words font-medium text-slate-700">{drawerAction.asset_type ?? "direct_action"}</div>
              </div>
              <div>
                <div className="text-xs font-semibold uppercase text-slate-400">Why now</div>
                <div className="mt-1 font-medium leading-5 text-slate-700">{actionWhyNowText(drawerAction)}</div>
              </div>
              <div>
                <div className="text-xs font-semibold uppercase text-slate-400">After execution</div>
                <div className="mt-1 font-medium leading-5 text-slate-700">{actionPostExecutionText(drawerAction)}</div>
              </div>
              <div className="sm:col-span-2">
                <div className="text-xs font-semibold uppercase text-slate-400">SEO/GEO contribution</div>
                <div className="mt-1 font-medium leading-5 text-slate-700">{actionSEOContributionText(drawerAction)}</div>
              </div>
            </section>

            <section className="rounded-xl border border-slate-200 p-4">
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Execution context</div>
              <div className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
                <div>
                  <div className="text-xs font-semibold uppercase text-slate-400">Target URL</div>
                  <div className="mt-1 break-words font-medium text-slate-700">{drawerAction.target_url ?? drawerAction.normalized_target_url ?? "No target URL yet."}</div>
                </div>
                <div>
                  <div className="text-xs font-semibold uppercase text-slate-400">Verification</div>
                  <div className="mt-1 font-medium text-slate-700">
                    {verificationLabel || (hasActionVerificationSnapshot(drawerAction) ? "Needs check" : "Not started")}
                  </div>
                </div>
                <div>
                  <div className="text-xs font-semibold uppercase text-slate-400">Baseline</div>
                  <div className="mt-1 break-words font-medium text-slate-700">{measurementWindowLabel(drawerAction.baseline_window)}</div>
                </div>
                <div>
                  <div className="text-xs font-semibold uppercase text-slate-400">Measurement</div>
                  <div className="mt-1 break-words font-medium text-slate-700">{measurementWindowLabel(drawerAction.measurement_window)}</div>
                </div>
              </div>
            </section>
          </div>
        )}
      </RightDrawer>
    </>
  );
}
