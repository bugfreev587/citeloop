"use client";

import { useSignIn } from "@clerk/nextjs/legacy";
import { ArrowRight, Loader2, Moon, Sun } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import type { Project } from "./lib/api";
import { LAST_PROJECT_STORAGE_KEY, dashboardHrefForProjects } from "./lib/dashboard-routing";
import { applyThemeChoice, readStoredThemeChoice, saveThemeChoice, type ThemeChoice } from "./lib/theme";
import { cx } from "./components/ui";

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

export function LandingThemeToggle({ className = "" }: { className?: string }) {
  const [theme, setTheme] = useState<ThemeChoice>("light");

  useEffect(() => {
    const nextTheme = readStoredThemeChoice();
    setTheme(nextTheme);
    applyThemeChoice(nextTheme);
  }, []);

  function chooseTheme(nextTheme: ThemeChoice) {
    setTheme(nextTheme);
    saveThemeChoice(nextTheme);
  }

  return (
    <div
      aria-label="Theme"
      className={cx(
        "inline-grid h-10 grid-cols-2 items-center rounded-full border border-stone-200 bg-white/80 p-1 shadow-sm backdrop-blur dark:border-slate-700 dark:bg-slate-900/80",
        className,
      )}
    >
      <button
        type="button"
        aria-label="Use light mode"
        aria-pressed={theme === "light"}
        onClick={() => chooseTheme("light")}
        className={cx(
          "grid h-8 w-8 place-items-center rounded-full text-stone-500 transition-colors hover:bg-stone-100 hover:text-slate-950 focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec] dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-100 dark:focus-visible:ring-slate-600",
          theme === "light" && "bg-slate-950 text-white hover:bg-slate-950 hover:text-white dark:bg-slate-100 dark:text-slate-950 dark:hover:bg-slate-100 dark:hover:text-slate-950",
        )}
      >
        <Sun size={17} strokeWidth={1.8} aria-hidden="true" />
      </button>
      <button
        type="button"
        aria-label="Use dark mode"
        aria-pressed={theme === "dark"}
        onClick={() => chooseTheme("dark")}
        className={cx(
          "grid h-8 w-8 place-items-center rounded-full text-stone-500 transition-colors hover:bg-stone-100 hover:text-slate-950 focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec] dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-100 dark:focus-visible:ring-slate-600",
          theme === "dark" && "bg-slate-950 text-white hover:bg-slate-950 hover:text-white dark:bg-slate-100 dark:text-slate-950 dark:hover:bg-slate-100 dark:hover:text-slate-950",
        )}
      >
        <Moon size={16} strokeWidth={1.8} aria-hidden="true" />
      </button>
    </div>
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
