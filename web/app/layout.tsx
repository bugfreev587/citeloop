import "./globals.css";
import type { Metadata } from "next";
import { ClerkProvider } from "@clerk/nextjs";
import { ToastProvider } from "./components/toast-provider";

export const metadata: Metadata = {
  title: "CiteLoop",
  description: "Turn your domain and Search Console data into a self-improving SEO/GEO growth loop.",
};

const initialThemeScript = `
(() => {
  try {
    const saved = window.localStorage.getItem("citeloop:theme");
    const theme = saved === "dark" || saved === "light" ? saved : "light";
    const root = document.documentElement;
    root.dataset.theme = theme;
    root.classList.toggle("dark", theme === "dark");
    root.classList.toggle("light", theme === "light");
    root.style.colorScheme = theme;
  } catch {
    document.documentElement.dataset.theme = "light";
    document.documentElement.classList.add("light");
    document.documentElement.classList.remove("dark");
    document.documentElement.style.colorScheme = "light";
  }
})();
`;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <ClerkProvider>
      <html lang="en" suppressHydrationWarning>
        <head>
          <script dangerouslySetInnerHTML={{ __html: initialThemeScript }} />
        </head>
        <body>
          <ToastProvider>{children}</ToastProvider>
        </body>
      </html>
    </ClerkProvider>
  );
}
