export const THEME_STORAGE_KEY = "citeloop:theme";

export type ThemeChoice = "light" | "dark";

export function isThemeChoice(value: string | null): value is ThemeChoice {
  return value === "light" || value === "dark";
}

export function applyThemeChoice(nextTheme: ThemeChoice) {
  if (typeof document === "undefined") return;

  const root = document.documentElement;
  root.dataset.theme = nextTheme;
  root.classList.toggle("dark", nextTheme === "dark");
  root.classList.toggle("light", nextTheme === "light");
  root.style.colorScheme = nextTheme;
}

export function readStoredThemeChoice(fallback: ThemeChoice = "light") {
  if (typeof window === "undefined") return fallback;

  const saved = window.localStorage.getItem(THEME_STORAGE_KEY);
  return isThemeChoice(saved) ? saved : fallback;
}

export function saveThemeChoice(nextTheme: ThemeChoice) {
  applyThemeChoice(nextTheme);
  window.localStorage.setItem(THEME_STORAGE_KEY, nextTheme);
}
