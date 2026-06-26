import "./globals.css";
import type { Metadata } from "next";
import { ClerkProvider } from "@clerk/nextjs";
import { ToastProvider } from "./components/toast-provider";

export const metadata: Metadata = {
  title: "CiteLoop",
  description: "Turn your domain and Search Console data into a self-improving SEO/GEO growth loop.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <ClerkProvider>
      <html lang="en">
        <body>
          <ToastProvider>{children}</ToastProvider>
        </body>
      </html>
    </ClerkProvider>
  );
}
