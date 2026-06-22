"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useCallback, useEffect, useState } from "react";
import { ArrowLeft, CheckCircle2, GitBranch, Loader2, XCircle } from "lucide-react";
import { GithubRepo } from "../../../lib/api";
import { deriveGitHubBranch, derivePublishTarget, normalizeDomain } from "../../../lib/publisher-target";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, ButtonProgress, Field, Notice, SectionHeader, TextInput } from "../../../components/ui";

type Phase = "linking" | "picking" | "saving" | "done" | "error";

function GithubCallbackInner() {
  const api = useApi();
  const router = useRouter();
  const params = useSearchParams();

  // GitHub redirects here after an App install with the installation_id and the
  // state we sent (the project id). setup_action is "install" or "update".
  const installationID = params.get("installation_id") ?? "";
  const projectID = params.get("state") ?? "";

  const [phase, setPhase] = useState<Phase>("linking");
  const [error, setError] = useState<string | null>(null);
  const [repos, setRepos] = useState<GithubRepo[]>([]);
  const [repo, setRepo] = useState("");
  const [branch, setBranch] = useState("");
  const [contentDir, setContentDir] = useState("content/citeloop/blog");
  const [baseURL, setBaseURL] = useState("");
  const [baseTouched, setBaseTouched] = useState(false);
  const [branchTouched, setBranchTouched] = useState(false);
  const [siteURL, setSiteURL] = useState("");

  const publishingHref = projectID ? `/projects/${projectID}/publishing` : "/";

  const link = useCallback(async () => {
    if (!installationID || !projectID) {
      setPhase("error");
      setError("GitHub redirect was missing the installation or project reference. Start the connection again from the Platforms drawer.");
      return;
    }
    setPhase("linking");
    setError(null);
    try {
      const { repositories } = await api.storeGithubInstallation(projectID, installationID);
      setRepos(repositories);
      let nextContentDir = "content/citeloop/blog";
      if (repositories.length > 0) {
        setRepo(repositories[0].full_name);
      }
      // Default the Site base URL from this project's own configured domain —
      // the repo is being connected to publish THIS project's posts.
      try {
        const project = await api.getProject(projectID);
        // Prefer the configured domain; fall back to the project name when it is
        // itself a hostname (e.g. a project literally named "staging.unipost.dev").
        const domain = normalizeDomain(project.config?.site_url ?? "") || normalizeDomain(project.name ?? "");
        setSiteURL(domain);
        if (domain) {
          const target = derivePublishTarget(domain, nextContentDir);
          setBaseURL(target.baseURL);
          if (target.branch) setBranch(target.branch);
        }
      } catch {
        // Project lookup is best-effort; the field stays empty + editable.
      }
      setPhase("picking");
    } catch (e: any) {
      setPhase("error");
      setError(e.message ?? "Could not record the GitHub installation.");
    }
  }, [api, installationID, projectID]);

  useEffect(() => {
    link();
  }, [link]);

  function chooseRepo(fullName: string) {
    setRepo(fullName);
  }

  async function save() {
    setPhase("saving");
    setError(null);
    try {
      await api.selectGithubRepo(projectID, {
        repo: repo.trim(),
        branch: branch.trim(),
        content_dir: contentDir.trim() || "content/citeloop/blog",
        base_url: baseURL.trim(),
      });
      setPhase("done");
      router.push(`${publishingHref}?github=connected`);
    } catch (e: any) {
      setPhase("picking");
      setError(e.message ?? "Could not save the selected repository.");
    }
  }

  return (
    <main className="mx-auto max-w-[640px] px-4 py-8 md:px-6 md:py-12">
      <div className="mb-5 flex items-center justify-between gap-3">
        <Link href={publishingHref} className="inline-flex items-center gap-2 text-sm font-semibold text-slate-500 hover:text-slate-900">
          <ArrowLeft size={15} />
          Publishing
        </Link>
        <Badge tone="neutral">
          <GitBranch size={13} className="mr-1" />
          GitHub
        </Badge>
      </div>

      <SectionHeader title="Connect GitHub" eyebrow="Pick the repository CiteLoop publishes canonical articles into" />

      {error && <Notice title="Something went wrong" detail={error} tone="red" />}

      {phase === "linking" && (
        <div className="mt-6 grid place-items-center rounded-xl border border-slate-200 bg-white py-12 text-sm text-slate-500">
          <span className="inline-flex items-center gap-2">
            <Loader2 size={16} className="animate-spin" />
            Linking your GitHub installation…
          </span>
        </div>
      )}

      {phase === "error" && (
        <div className="mt-6 rounded-xl border border-red-200 bg-red-50 p-5 text-sm text-red-900">
          <div className="inline-flex items-center gap-2 font-bold">
            <XCircle size={16} />
            Connection failed
          </div>
          <div className="mt-1 leading-6">{error}</div>
          <Link href={publishingHref} className="mt-4 inline-flex items-center gap-2 text-sm font-semibold text-[#d93820] hover:underline">
            <ArrowLeft size={14} />
            Back to Publishing
          </Link>
        </div>
      )}

      {(phase === "picking" || phase === "saving") && (
        <section className="mt-6 grid gap-4 rounded-xl border border-slate-200 bg-white p-5">
          <Notice
            title="Installation linked"
            detail={`CiteLoop can now publish to ${repos.length} repositor${repos.length === 1 ? "y" : "ies"} via the GitHub App — no token to manage.`}
            tone="green"
          />

          <Field label="Repository" helper="Only repositories you granted the CiteLoop App access to appear here.">
            {repos.length === 0 ? (
              <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
                The installation has no accessible repositories. Re-run the install and grant access to at least one repo.
              </div>
            ) : (
              <select
                value={repo}
                onChange={(event) => chooseRepo(event.target.value)}
                className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none focus:border-slate-400"
              >
                {repos.map((r) => (
                  <option key={r.full_name} value={r.full_name}>
                    {r.full_name}
                    {r.private ? " (private)" : ""}
                  </option>
                ))}
              </select>
            )}
          </Field>

          <Field
            label="Site base URL"
            helper={
              siteURL
                ? `Prefilled from this project's domain. Posts publish to <base>/<slug> — edit if your blog lives elsewhere.`
                : "The public URL where published articles live (posts publish to <base>/<slug>)."
            }
          >
            <TextInput
              value={baseURL}
              placeholder="https://example.com/blog"
              onChange={(event) => {
                setBaseTouched(true);
                const next = event.target.value;
                setBaseURL(next);
                const nextBranch = deriveGitHubBranch(next);
                if (nextBranch && !branchTouched) setBranch(nextBranch);
              }}
            />
          </Field>

          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Branch" helper={siteURL ? "Matched from this project's domain when possible." : "Choose the branch that deploys this site."}>
              <TextInput
                value={branch}
                onChange={(event) => {
                  setBranchTouched(true);
                  setBranch(event.target.value);
                }}
              />
            </Field>
            <Field label="Content directory">
              <TextInput
                value={contentDir}
                onChange={(event) => {
                  const next = event.target.value;
                  setContentDir(next);
                  // Keep the derived base URL in sync until the operator edits it.
                  if (!baseTouched && siteURL) setBaseURL(derivePublishTarget(siteURL, next).baseURL);
                }}
              />
            </Field>
          </div>

          <div className="flex justify-end">
            <Button disabled={phase === "saving" || !repo.trim() || !branch.trim() || !baseURL.trim()} variant="primary" onClick={save}>
              <ButtonProgress busy={phase === "saving"} busyLabel="Saving" idleIcon={<CheckCircle2 size={16} />}>
                Finish connecting
              </ButtonProgress>
            </Button>
          </div>
        </section>
      )}

      {phase === "done" && (
        <div className="mt-6 grid place-items-center rounded-xl border border-emerald-200 bg-emerald-50 py-12 text-sm text-emerald-800">
          <span className="inline-flex items-center gap-2 font-semibold">
            <CheckCircle2 size={16} />
            Connected — taking you back to Publishing…
          </span>
        </div>
      )}
    </main>
  );
}

export default function GithubCallbackPage() {
  return (
    <Suspense
      fallback={
        <main className="grid min-h-[60vh] place-items-center text-sm text-slate-500">
          <span className="inline-flex items-center gap-2">
            <Loader2 size={16} className="animate-spin" />
            Connecting GitHub…
          </span>
        </main>
      }
    >
      <GithubCallbackInner />
    </Suspense>
  );
}
