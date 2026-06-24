"use client";

import { useSignIn } from "@clerk/nextjs/legacy";
import { ArrowRight, Loader2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";
import type { Project } from "./lib/api";
import { LAST_PROJECT_STORAGE_KEY, dashboardHrefForProjects } from "./lib/dashboard-routing";

const baseButtonClass =
  "inline-flex h-10 items-center justify-center gap-2 rounded-lg px-4 text-sm font-semibold transition-colors active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-70";

export function JoinWithGoogleButton({ className = "" }: { className?: string }) {
  const { isLoaded, signIn } = useSignIn();
  const [busy, setBusy] = useState(false);

  async function joinWithGoogle() {
    if (!isLoaded || !signIn || busy) return;
    setBusy(true);
    try {
      await signIn.authenticateWithRedirect({
        strategy: "oauth_google",
        redirectUrl: "/sign-in/sso-callback",
        redirectUrlComplete: "/",
      });
    } catch {
      setBusy(false);
    }
  }

  return (
    <button
      type="button"
      onClick={joinWithGoogle}
      disabled={!isLoaded || busy}
      className={`${baseButtonClass} border border-stone-300 bg-white text-stone-800 hover:border-stone-400 hover:bg-stone-50 ${className}`}
    >
      {busy && <Loader2 className="animate-spin" size={16} aria-hidden="true" />}
      {busy ? "Opening Google..." : "Join with Google"}
    </button>
  );
}

export function LandingDashboardButton({
  initialProjects,
  projectPrefetchFailed = false,
  className = "",
}: {
  initialProjects: Project[];
  projectPrefetchFailed?: boolean;
  className?: string;
}) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);

  function openDashboard() {
    if (busy) return;
    setBusy(true);
    const storedProjectId =
      typeof window === "undefined" ? null : window.localStorage.getItem(LAST_PROJECT_STORAGE_KEY);
    router.push(dashboardHrefForProjects(initialProjects, storedProjectId, projectPrefetchFailed));
  }

  return (
    <button
      type="button"
      onClick={openDashboard}
      disabled={busy}
      className={`${baseButtonClass} bg-slate-950 text-white hover:bg-slate-800 ${className}`}
    >
      {busy ? <Loader2 className="animate-spin" size={16} aria-hidden="true" /> : <ArrowRight size={16} aria-hidden="true" />}
      {busy ? "Opening..." : "Dashboard"}
    </button>
  );
}
