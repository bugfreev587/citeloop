"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import {
  Check,
  CheckCircle2,
  ChevronRight,
  Clipboard,
  Code2,
  ExternalLink,
  FileCode2,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Wrench,
} from "lucide-react";
import { RightDrawer } from "../../../components/right-drawer";
import { Badge, Button, ButtonProgress, EmptyState, Notice, SectionHeader, formatDate } from "../../../components/ui";
import {
  canonicalSiteFixAIJSON,
  canonicalSiteFixNextAction,
  canonicalSiteFixStatusLabel,
  canonicalSiteFixTarget,
  canonicalSiteFixTitle,
} from "../../../lib/site-fix";
import type { SiteChangeApplication, SiteFix } from "../../../lib/types";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";

const CLOSED_STATUSES = new Set(["verified", "failed_terminal", "superseded", "migration_rolled_back"]);

function statusTone(status: SiteFix["status"]): "neutral" | "red" | "amber" | "green" | "blue" | "violet" {
  if (status === "verified") return "green";
  if (status === "failed_terminal") return "red";
  if (status === "failed_retryable" || status === "reopened") return "amber";
  if (["approved", "ready_to_apply", "awaiting_deploy", "verifying"].includes(status)) return "blue";
  if (["preparing", "applying"].includes(status)) return "violet";
  return "neutral";
}

function prettyValue(value: unknown, fallback = "Not recorded yet.") {
  if (value == null || value === "") return fallback;
  if (typeof value === "string") return value;
  if (Array.isArray(value) && value.length === 0) return fallback;
  if (typeof value === "object" && Object.keys(value as object).length === 0) return fallback;
  return JSON.stringify(value, null, 2);
}

async function writeClipboardText(text: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Use the browser fallback below.
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

function lifecycleStep(fix: SiteFix) {
  if (fix.status === "verified") return 3;
  if (["preparing", "ready_to_apply", "applying", "awaiting_deploy", "verifying", "failed_retryable", "reopened"].includes(fix.status) || fix.applied_at || fix.application) return 2;
  if (fix.status === "approved" || fix.approved_at) return 1;
  return 0;
}

function LifecycleStrip({ fix }: { fix: SiteFix }) {
  const current = lifecycleStep(fix);
  const applicationStep = fix.status === "awaiting_deploy"
    ? "Awaiting deploy"
    : fix.status === "verifying"
      ? "Verifying"
      : fix.status === "failed_retryable" || fix.status === "reopened"
        ? "Verification retry"
        : "Applied / deploy";
  const steps = ["Finding", "Approved", "Applied / deploy", "Verified"];
  return (
    <ol aria-label="Site fix lifecycle" className="grid grid-cols-2 gap-2 sm:grid-cols-4">
      {steps.map((step, index) => {
        const isCurrent = index === current;
        const isComplete = index < current || (index === 3 && fix.status === "verified");
        return (
          <li
            key={`${index}-${step}`}
            aria-current={isCurrent ? "step" : undefined}
            className={`rounded-lg border px-3 py-2 text-xs font-semibold ${
              isComplete
                ? "border-emerald-200 bg-emerald-50 text-emerald-800"
                : isCurrent
                  ? "border-sky-200 bg-sky-50 text-sky-800"
                  : "border-slate-200 bg-slate-50 text-slate-500"
            }`}
          >
            <span className="flex items-center gap-1.5">
              {isComplete ? <Check aria-hidden="true" size={13} /> : <span aria-hidden="true">{index + 1}</span>}
              {index === 2 ? applicationStep : step}
            </span>
          </li>
        );
      })}
    </ol>
  );
}

function canonicalFixIDForAlias(fixes: SiteFix[], requestedID?: string) {
  if (!requestedID) return null;
  return fixes.find((fix) =>
    fix.id === requestedID ||
    fix.legacy_opportunity_id === requestedID ||
    fix.legacy_content_action_id === requestedID ||
    fix.legacy_aliases?.some((alias) => alias.object_id === requestedID),
  )?.id ?? null;
}

function DetailBlock({ title, value }: { title: string; value: unknown }) {
  return (
    <section className="rounded-xl border border-slate-200 bg-white p-4">
      <h4 className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">{title}</h4>
      <pre className="mt-3 whitespace-pre-wrap break-words font-sans text-sm leading-6 text-slate-700">{prettyValue(value)}</pre>
    </section>
  );
}

export function SiteFixesClient({ projectId, initialFixId }: { projectId: string; initialFixId?: string }) {
  const api = useApi();
  const { notify } = useToast();
  const [siteFixes, setSiteFixes] = useState<SiteFix[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const surfaceRef = useRef<HTMLDivElement | null>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const rows = await api.listDoctorSiteFixes(projectId);
      setSiteFixes(rows);
      const canonicalInitialFixID = canonicalFixIDForAlias(rows, initialFixId);
      if (canonicalInitialFixID) setSelectedID(canonicalInitialFixID);
    } catch (err: any) {
      setError(err?.apiMessage || err?.message || "Could not load Site Fixes.");
    } finally {
      setLoading(false);
    }
  }, [api, initialFixId, projectId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const sortedFixes = useMemo(
    () => siteFixes.slice().sort((a, b) => String(b.updated_at ?? b.created_at ?? "").localeCompare(String(a.updated_at ?? a.created_at ?? ""))),
    [siteFixes],
  );
  const active = sortedFixes.filter((fix) => !CLOSED_STATUSES.has(fix.status));
  const completed = sortedFixes.filter((fix) => CLOSED_STATUSES.has(fix.status));
  const selected = selectedID ? siteFixes.find((fix) => fix.id === selectedID) ?? null : null;

  useEffect(() => {
    if (selectedID && !selected) setSelectedID(null);
  }, [selected, selectedID]);

  function replaceFix(updated: SiteFix, application?: SiteChangeApplication) {
    setSiteFixes((current) =>
      current.map((existing) =>
        existing.id === updated.id
          ? {
              ...existing,
              ...updated,
              application: application ?? updated.application ?? existing.application,
              verifications: updated.verifications ?? existing.verifications,
              legacy_aliases: updated.legacy_aliases ?? existing.legacy_aliases,
            }
          : existing,
      ),
    );
  }

  async function approveFix(fix: SiteFix) {
    setBusy(`approve-${fix.id}`);
    try {
      const updated = await api.approveDoctorSiteFix(projectId, fix.id);
      replaceFix(updated);
      notify({ tone: "green", title: "Fix approved", detail: "The repair is ready for application." });
    } catch (err: any) {
      notify({ tone: "red", title: "Could not approve fix", detail: err?.apiMessage || err?.message });
    } finally {
      setBusy(null);
    }
  }

  async function applyFix(fix: SiteFix) {
    const retrying = fix.status === "preparing";
    setBusy(`apply-${fix.id}`);
    try {
      const result = await api.applyDoctorSiteFix(projectId, fix.id);
      replaceFix(result.site_fix, result.application);
      notify({
        tone: "green",
        title: retrying ? "Application retry started" : "Application started",
        detail: result.application.github_pr_url ? "A source change is ready for deploy review." : "Follow the application handoff shown here.",
      });
    } catch (err: any) {
      notify({ tone: "red", title: retrying ? "Could not retry apply" : "Could not apply fix", detail: err?.apiMessage || err?.message });
    } finally {
      setBusy(null);
    }
  }

  async function verifyFix(fix: SiteFix) {
    setBusy(`verify-${fix.id}`);
    try {
      const result = await api.verifyDoctorSiteFix(projectId, fix.id);
      replaceFix(result.site_fix, result.application);
      notify({ tone: "green", title: "Verification started", detail: "Doctor will check the acceptance evidence." });
    } catch (err: any) {
      notify({ tone: "red", title: "Could not verify fix", detail: err?.apiMessage || err?.message });
    } finally {
      setBusy(null);
    }
  }

  async function copyFixJSON(fix: SiteFix) {
    try {
      await writeClipboardText(canonicalSiteFixAIJSON(fix));
      notify({ tone: "green", title: "Fix JSON copied", detail: "Paste it into Codex or Claude Code to implement the repair." });
    } catch {
      notify({ tone: "red", title: "Could not copy fix JSON", detail: "Select the JSON in the drawer and copy it manually." });
    }
  }

  function openFix(fix: SiteFix, trigger: HTMLElement) {
    returnFocusRef.current = trigger;
    setSelectedID(fix.id);
  }

  function renderCard(fix: SiteFix) {
    return (
      <button
        key={fix.id}
        type="button"
        data-site-fix-card
        onClick={(event) => openFix(fix, event.currentTarget)}
        aria-label={`Review site fix details: ${canonicalSiteFixTitle(fix)}`}
        className="group flex min-h-56 w-full flex-col rounded-xl border border-slate-200 bg-white p-5 text-left shadow-sm transition hover:-translate-y-0.5 hover:border-slate-300 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-400"
      >
        <div className="flex w-full items-start justify-between gap-3">
          <div className="flex flex-wrap gap-2">
            <Badge tone={fix.finding_kind === "broken" ? "red" : "violet"}>{fix.finding_kind === "broken" ? "Broken" : "Optimization"}</Badge>
            <Badge tone={statusTone(fix.status)}>{canonicalSiteFixStatusLabel(fix.status)}</Badge>
          </div>
          <ChevronRight aria-hidden="true" className="shrink-0 text-slate-400 transition group-hover:translate-x-0.5" size={17} />
        </div>
        <h3 className="mt-4 line-clamp-3 text-base font-bold leading-6 text-slate-950">{canonicalSiteFixTitle(fix)}</h3>
        <p className="mt-2 line-clamp-2 break-all text-sm leading-5 text-slate-500">{canonicalSiteFixTarget(fix)}</p>
        <div className="mt-auto border-t border-slate-100 pt-4">
          <p className="line-clamp-2 text-xs leading-5 text-slate-600">{canonicalSiteFixNextAction(fix)}</p>
          <p className="mt-2 text-[11px] text-slate-400">Updated {formatDate(fix.updated_at ?? fix.created_at ?? null)}</p>
        </div>
      </button>
    );
  }

  const drawerApplication = selected?.application ?? null;
  const requiresManualApplyConfirmation = selected?.status === "applying" && drawerApplication?.status === "manual_apply_required";
  const retryingApply = selected?.status === "preparing";
  const canApprove = selected?.status === "proposed";
  const canApply = Boolean(selected && ["approved", "preparing", "ready_to_apply"].includes(selected.status));
  const canVerify = Boolean(selected && (["awaiting_deploy", "failed_retryable", "reopened"].includes(selected.status) || requiresManualApplyConfirmation));

  return (
    <>
      <div ref={surfaceRef} className="space-y-8 pb-12">
        <section className="rounded-2xl border border-slate-200 bg-gradient-to-br from-white to-slate-50 p-5 shadow-sm sm:p-7">
          <SectionHeader
            level="page"
            eyebrow="Doctor repair loop"
            title="Site Fixes"
            action={
              <Button size="sm" onClick={() => void refresh()} disabled={loading || busy === "refresh"}>
                <RefreshCw aria-hidden="true" size={14} /> Refresh
              </Button>
            }
          />
          <p className="max-w-3xl text-sm leading-6 text-slate-600">
            Every repair stays traceable from its Doctor finding through approval, application, deploy, and verification.
          </p>
          <div className="mt-5 grid gap-3 sm:grid-cols-3">
            <div className="rounded-xl border border-slate-200 bg-white p-4">
              <div className="text-2xl font-bold text-slate-950">{active.length}</div>
              <div className="mt-1 text-xs font-medium text-slate-500">Active fixes</div>
            </div>
            <div className="rounded-xl border border-slate-200 bg-white p-4">
              <div className="text-2xl font-bold text-slate-950">{siteFixes.filter((fix) => fix.status === "verified").length}</div>
              <div className="mt-1 text-xs font-medium text-slate-500">Verified fixes</div>
            </div>
            <div className="rounded-xl border border-slate-200 bg-white p-4">
              <div className="text-2xl font-bold text-slate-950">{siteFixes.filter((fix) => Boolean(fix.failure_reason)).length}</div>
              <div className="mt-1 text-xs font-medium text-slate-500">Needs attention</div>
            </div>
          </div>
        </section>

        {error && <Notice tone="red" title="Site Fixes could not load" detail={error} />}

        <section>
          <SectionHeader title="Active repair queue" eyebrow="Review and execute" />
          {loading ? (
            <div role="status" className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
              {[0, 1, 2].map((item) => <div key={item} className="h-56 animate-pulse rounded-xl border border-slate-200 bg-slate-100" />)}
            </div>
          ) : active.length === 0 ? (
            <EmptyState title="No active Site Fixes" detail="Create a Site Fix from an actionable Doctor finding to begin a repair loop." />
          ) : (
            <div data-site-fixes-grid className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{active.map(renderCard)}</div>
          )}
        </section>

        {!loading && completed.length > 0 && (
          <section>
            <SectionHeader title="Closed loops" eyebrow="Verified and historical" />
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{completed.map(renderCard)}</div>
          </section>
        )}
      </div>

      <RightDrawer
        open={Boolean(selected)}
        title={selected ? canonicalSiteFixTitle(selected) : "Site Fix"}
        eyebrow="Review site fix details"
        subtitle={selected ? canonicalSiteFixTarget(selected) : undefined}
        badges={
          selected ? (
            <>
              <Badge tone={selected.finding_kind === "broken" ? "red" : "violet"}>{selected.finding_kind === "broken" ? "Broken" : "Optimization"}</Badge>
              <Badge tone={statusTone(selected.status)}>{canonicalSiteFixStatusLabel(selected.status)}</Badge>
            </>
          ) : null
        }
        dataAttribute="site-fix-drawer"
        surfaceRef={surfaceRef}
        returnFocusRef={returnFocusRef}
        onClose={() => setSelectedID(null)}
        footer={
          selected ? (
            <>
              <Button onClick={() => void copyFixJSON(selected)}>
                <Clipboard aria-hidden="true" size={14} /> Copy fix JSON
              </Button>
              {canApprove && (
                <Button variant="primary" onClick={() => void approveFix(selected)} disabled={Boolean(busy)}>
                  <ButtonProgress busy={busy === `approve-${selected.id}`} busyLabel="Approving" idleIcon={<Check aria-hidden="true" size={14} />}>Approve fix</ButtonProgress>
                </Button>
              )}
              {canApply && (
                <Button variant="primary" onClick={() => void applyFix(selected)} disabled={Boolean(busy)}>
                  <ButtonProgress busy={busy === `apply-${selected.id}`} busyLabel={retryingApply ? "Retrying" : "Applying"} idleIcon={<Wrench aria-hidden="true" size={14} />}>
                    {selected.status === "preparing" ? "Retry apply" : "Apply fix"}
                  </ButtonProgress>
                </Button>
              )}
              {canVerify && (
                <Button variant="primary" onClick={() => void verifyFix(selected)} disabled={Boolean(busy)}>
                  <ButtonProgress busy={busy === `verify-${selected.id}`} busyLabel="Starting" idleIcon={<ShieldCheck aria-hidden="true" size={14} />}>
                    {requiresManualApplyConfirmation ? "I applied this manually — start verification" : "Verify fix"}
                  </ButtonProgress>
                </Button>
              )}
            </>
          ) : undefined
        }
      >
        {selected && (
          <div data-site-fix-drawer className="space-y-5">
            <LifecycleStrip fix={selected} />
            <Notice
              tone={selected.status === "verified" ? "green" : selected.failure_reason ? "amber" : "neutral"}
              title={canonicalSiteFixNextAction(selected)}
              detail={selected.failure_reason || undefined}
            />
            {requiresManualApplyConfirmation && (
              <Notice
                tone="amber"
                title="Manual application required"
                detail="Apply the proposed change to the target site first. Then confirm below to record it as applied and start evidence verification. Confirmation alone does not mark the fix verified."
              />
            )}

            <div className="grid gap-3 sm:grid-cols-3">
              <div className="rounded-lg bg-slate-50 p-3">
                <div className="text-xs font-semibold text-slate-500">Finding</div>
                <Link
                  className="mt-1 inline-flex break-all text-xs font-semibold text-sky-700 hover:text-sky-900"
                  href={`/projects/${projectId}/doctor?finding=${selected.doctor_finding_id}`}
                >
                  {selected.doctor_finding_id}
                </Link>
              </div>
              <div className="rounded-lg bg-slate-50 p-3">
                <div className="text-xs font-semibold text-slate-500">Retries</div>
                <div className="mt-1 text-sm font-bold text-slate-800">{selected.retry_count} / {selected.max_retries}</div>
              </div>
              <div className="rounded-lg bg-slate-50 p-3">
                <div className="text-xs font-semibold text-slate-500">Created</div>
                <div className="mt-1 text-xs text-slate-700">{formatDate(selected.created_at ?? null)}</div>
              </div>
            </div>

            {selected.failure_reason && (
              <section className="rounded-xl border border-amber-200 bg-amber-50 p-4">
                <div className="flex items-center gap-2 font-semibold text-amber-900"><RotateCcw aria-hidden="true" size={15} /> Retry evidence</div>
                <p className="mt-2 text-sm leading-6 text-amber-800">{selected.failure_reason}</p>
                <p className="mt-2 text-xs text-amber-700">retry_count: {selected.retry_count} of {selected.max_retries}</p>
              </section>
            )}

            <DetailBlock title="Evidence" value={selected.evidence_snapshot} />
            <DetailBlock title="Proposed fix" value={selected.proposed_fix} />
            <DetailBlock title="Acceptance checks" value={selected.acceptance_tests} />
            {(selected.legacy_opportunity_id || selected.legacy_content_action_id || selected.migration_batch_id || selected.legacy_aliases?.length) && (
              <DetailBlock
                title="Legacy provenance"
                value={{
                  legacy_opportunity_id: selected.legacy_opportunity_id,
                  legacy_content_action_id: selected.legacy_content_action_id,
                  migration_batch_id: selected.migration_batch_id,
                  legacy_aliases: selected.legacy_aliases,
                }}
              />
            )}

            <section className="rounded-xl border border-slate-200 bg-white p-4">
              <div className="flex items-center gap-2">
                <FileCode2 aria-hidden="true" size={16} className="text-slate-500" />
                <h4 className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">Application</h4>
              </div>
              {drawerApplication ? (
                <div className="mt-3 space-y-3 text-sm text-slate-700">
                  <div className="flex flex-wrap gap-2">
                    <Badge tone="blue">{drawerApplication.status || "Application created"}</Badge>
                    {drawerApplication.application_kind && <Badge>{drawerApplication.application_kind}</Badge>}
                  </div>
                  {drawerApplication.source_file_paths.length > 0 && <p className="break-all">Files: {drawerApplication.source_file_paths.join(", ")}</p>}
                  {drawerApplication.failure_reason && <Notice tone="amber" title="Application needs attention" detail={drawerApplication.failure_reason} />}
                  {drawerApplication.github_pr_url && (
                    <a className="inline-flex items-center gap-1.5 font-semibold text-sky-700 hover:text-sky-900" href={drawerApplication.github_pr_url} target="_blank" rel="noreferrer">
                      Open source change <ExternalLink aria-hidden="true" size={13} />
                    </a>
                  )}
                </div>
              ) : (
                <p className="mt-3 text-sm leading-6 text-slate-500">Application details appear after the approved repair is applied.</p>
              )}
            </section>

            <DetailBlock title="Verification" value={selected.verification_snapshot} />
            {selected.verifications && selected.verifications.length > 0 && (
              <DetailBlock title="Verification attempts" value={selected.verifications} />
            )}

            <section data-site-fix-ai-payload className="rounded-xl border border-cyan-200 bg-cyan-50/70 p-4">
              <div className="flex items-center gap-2 text-cyan-950">
                <Code2 aria-hidden="true" size={16} />
                <h4 className="text-sm font-bold">AI coding fix JSON</h4>
              </div>
              <p className="mt-2 text-xs leading-5 text-cyan-900">Copy this JSON into Codex or Claude Code when the repair needs implementation help.</p>
              <pre className="mt-3 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-slate-950 p-3 text-xs leading-5 text-slate-100">{canonicalSiteFixAIJSON(selected)}</pre>
            </section>

            {selected.status === "verified" && (
              <div className="flex items-center gap-2 rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-sm font-semibold text-emerald-800">
                <CheckCircle2 aria-hidden="true" size={17} /> Verified: the repair loop is closed.
              </div>
            )}
          </div>
        )}
      </RightDrawer>
    </>
  );
}
