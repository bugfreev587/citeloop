"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { CheckCircle2, Loader2, Settings } from "lucide-react";
import { GSCConnection } from "../../../../../lib/api";
import { useApi } from "../../../../../lib/use-api";
import { Badge, Button, Notice } from "../../../../../components/ui";

type State =
  | { kind: "loading" }
  | { kind: "success"; connection: GSCConnection }
  | { kind: "error"; title: string; detail?: string };

export function GSCCallbackClient({
  projectId,
  code,
  state,
  error,
}: {
  projectId: string;
  code: string;
  state: string;
  error: string;
}) {
  const api = useApi();
  const [result, setResult] = useState<State>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    async function complete() {
      if (error) {
        setResult({
          kind: "error",
          title: "Search Console connection was not approved",
          detail: "You can restart the connection when you are ready to grant read-only Search Console access.",
        });
        return;
      }
      if (!code || !state) {
        setResult({
          kind: "error",
          title: "Search Console callback is missing information",
          detail: "Return to Settings and start the connection again.",
        });
        return;
      }
      try {
        const connection = await api.completeGSCOAuth(projectId, { code, state });
        if (!cancelled) setResult({ kind: "success", connection });
      } catch (e: any) {
        if (!cancelled) {
          setResult({
            kind: "error",
            title: "Search Console connection failed",
            detail: e.message,
          });
        }
      }
    }
    void complete();
    return () => {
      cancelled = true;
    };
  }, [api, code, error, projectId, state]);

  return (
    <div className="mx-auto grid max-w-2xl gap-4 rounded-xl border border-slate-200 bg-white p-5">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-600">
          {result.kind === "loading" ? <Loader2 className="animate-spin" size={18} /> : <CheckCircle2 size={18} />}
        </div>
        <div>
          <Badge tone={result.kind === "success" ? "green" : result.kind === "error" ? "amber" : "blue"}>
            Search Console
          </Badge>
          <h1 className="mt-2 text-2xl font-bold leading-8 text-slate-950">Finishing Search Console connection</h1>
          <p className="mt-1 text-sm leading-6 text-slate-600">
            CiteLoop is completing the read-only authorization and preparing your Search Console property choices.
          </p>
        </div>
      </div>

      {result.kind === "loading" && <Notice title="Connecting Search Console" detail="This usually takes a few seconds." tone="neutral" />}
      {result.kind === "error" && <Notice title={result.title} detail={result.detail} tone="amber" />}
      {result.kind === "success" && (
        <Notice
          title={result.connection.status === "connected" ? "Search Console connected" : "Choose a Search Console property"}
          detail={
            result.connection.status === "connected"
              ? `CiteLoop can now use ${result.connection.selected_property ?? "your selected property"} for first-party search data.`
              : "Your Google account is authorized. Select the matching property in Settings so CiteLoop knows which domain to measure."
          }
          tone={result.connection.status === "connected" ? "green" : "amber"}
        />
      )}

      {result.kind === "success" && result.connection.properties.length > 0 && (
        <div className="grid gap-2 rounded-lg border border-slate-100 bg-slate-50 p-3">
          <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Authorized properties</div>
          {result.connection.properties.slice(0, 4).map((property) => (
            <div key={property.site_url} className="flex items-center justify-between gap-3 rounded-md bg-white px-3 py-2 text-sm">
              <span className="truncate font-semibold text-slate-700">{property.site_url}</span>
              {property.recommended && <Badge tone="green">Recommended</Badge>}
            </div>
          ))}
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        <Link
          href={`/projects/${projectId}/settings`}
          className="inline-flex h-10 items-center justify-center gap-2 rounded-xl border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50"
        >
          <Settings size={16} />
          Return to Settings
        </Link>
        <Button variant="ghost" onClick={() => window.location.assign(`/projects/${projectId}/analysis`)}>
          Back to Opportunities
        </Button>
      </div>
    </div>
  );
}
