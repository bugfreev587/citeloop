"use client";

import Link from "next/link";
import { UserButton, useAuth } from "@clerk/nextjs";
import { useSignIn } from "@clerk/nextjs/legacy";
import { ArrowRight, Loader2, Moon, Sun } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
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

  const isDark = theme === "dark";
  const nextTheme: ThemeChoice = isDark ? "light" : "dark";
  const Icon = isDark ? Moon : Sun;
  const label = isDark ? "Switch to light theme" : "Switch to dark theme";

  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={() => chooseTheme(nextTheme)}
      className={cx(
        "inline-flex h-10 w-16 items-center justify-center rounded-full border border-stone-200 bg-white/80 text-[#d93820] shadow-sm backdrop-blur transition-colors hover:border-stone-300 hover:bg-white focus:outline-none focus-visible:ring-2 focus-visible:ring-[#dfe5ec] active:scale-[0.98] dark:border-slate-700 dark:bg-slate-900/80 dark:text-slate-100 dark:hover:border-slate-600 dark:hover:bg-slate-800 dark:focus-visible:ring-slate-600",
        className,
      )}
    >
      <Icon size={17} strokeWidth={1.9} aria-hidden="true" />
    </button>
  );
}

export function LandingDashboardButton({
  className = "",
}: {
  className?: string;
}) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);

  function openDashboard() {
    if (busy) return;
    setBusy(true);
    const storedProjectId =
      typeof window === "undefined" ? null : window.localStorage.getItem(LAST_PROJECT_STORAGE_KEY);
    router.push(dashboardHrefForProjects([], storedProjectId, true));
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

export function LandingHeaderActions() {
  const { isLoaded, isSignedIn } = useAuth();
  const showAuthenticatedActions = isLoaded && isSignedIn;

  return (
    <div className="hidden flex-wrap items-center justify-end gap-2 sm:flex">
      <LandingThemeToggle />
      {showAuthenticatedActions ? (
        <>
          <LandingDashboardButton />
          <UserButton />
        </>
      ) : (
        <>
          <JoinWithGoogleButton />
          <Link
            href="/sign-up"
            className="inline-flex h-10 items-center justify-center gap-2 rounded-lg bg-slate-950 px-4 text-sm font-semibold text-white transition-colors hover:bg-slate-800 active:scale-[0.98]"
          >
            Start for free
            <ArrowRight size={16} aria-hidden="true" />
          </Link>
        </>
      )}
    </div>
  );
}

export function LandingHeroActions() {
  const { isLoaded, isSignedIn } = useAuth();
  const showAuthenticatedActions = isLoaded && isSignedIn;

  return (
    <>
      {showAuthenticatedActions ? (
        <LandingDashboardButton className="h-11 w-full px-5 sm:w-auto" />
      ) : (
        <>
          <Link
            href="/sign-up"
            className="inline-flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-slate-950 px-5 text-sm font-semibold text-white transition-colors hover:bg-slate-800 active:scale-[0.98] sm:w-auto"
          >
            Start with your domain
            <ArrowRight size={16} aria-hidden="true" />
          </Link>
          <JoinWithGoogleButton className="h-11 w-full px-5 sm:w-auto" />
        </>
      )}
    </>
  );
}
