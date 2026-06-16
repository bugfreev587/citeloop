"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ShieldCheck } from "lucide-react";
import { useApi } from "../lib/use-api";

// AdminDocsLink shows the Admin entry only to platform administrators (resolved
// server-side via the ADMINS allowlist), so ordinary users never see it.
export function AdminDocsLink() {
  const api = useApi();
  const [show, setShow] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api
      .getMe()
      .then((me) => {
        if (!cancelled) setShow(Boolean(me?.is_admin));
      })
      .catch(() => {
        if (!cancelled) setShow(false);
      });
    return () => {
      cancelled = true;
    };
  }, [api]);

  if (!show) return null;

  return (
    <Link
      href="/admin"
      className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50 hover:text-slate-950"
    >
      <ShieldCheck size={15} />
      Admin
    </Link>
  );
}
