import "./globals.css";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "CiteLoop",
  description: "SEO + GEO automated content engine",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
