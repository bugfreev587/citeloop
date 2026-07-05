"use client";

import type { ReactNode, RefObject } from "react";
import { useEffect, useId, useRef } from "react";
import { X } from "lucide-react";
import { cx } from "./ui";

const drawerFocusableSelector =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function RightDrawer({
  open,
  title,
  eyebrow,
  subtitle,
  badges,
  children,
  footer,
  footerLabel = "Drawer actions",
  closeLabel = "Close drawer",
  dataAttribute,
  maxWidthClassName = "max-w-2xl",
  surfaceRef,
  returnFocusRef,
  onClose,
}: {
  open: boolean;
  title: ReactNode;
  eyebrow?: ReactNode;
  subtitle?: ReactNode;
  badges?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  footerLabel?: string;
  closeLabel?: string;
  dataAttribute?: string;
  maxWidthClassName?: string;
  surfaceRef?: RefObject<HTMLElement | null>;
  returnFocusRef?: RefObject<HTMLElement | null>;
  onClose: () => void;
}) {
  const titleId = useId();
  const drawerRef = useRef<HTMLElement | null>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
        return;
      }
      if (event.key !== "Tab") return;

      const drawer = drawerRef.current;
      if (!drawer) return;
      const focusable = Array.from(drawer.querySelectorAll<HTMLElement>(drawerFocusableSelector)).filter(
        (element) => !element.hasAttribute("disabled") && element.getAttribute("aria-hidden") !== "true",
      );
      if (focusable.length === 0) {
        event.preventDefault();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onClose, open]);

  useEffect(() => {
    if (!open) return;

    previousFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const previousBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const closeButton = drawerRef.current?.querySelector<HTMLElement>("[data-drawer-close]");
    const firstFocusable = closeButton ?? drawerRef.current?.querySelector<HTMLElement>(drawerFocusableSelector);
    firstFocusable?.focus();

    if (surfaceRef?.current) {
      surfaceRef.current.setAttribute("aria-hidden", "true");
      surfaceRef.current.inert = true;
    }

    return () => {
      document.body.style.overflow = previousBodyOverflow;
      if (surfaceRef?.current) {
        surfaceRef.current.removeAttribute("aria-hidden");
        surfaceRef.current.inert = false;
      }
      const focusTarget = returnFocusRef?.current ?? previousFocusRef.current;
      if (focusTarget?.isConnected) {
        focusTarget.focus();
      }
    };
  }, [open, returnFocusRef, surfaceRef]);

  if (!open) return null;

  const dataProps = dataAttribute ? { [`data-${dataAttribute}`]: true } : {};

  return (
    <div className="fixed inset-0 z-50">
      <button
        type="button"
        aria-label={closeLabel}
        onClick={onClose}
        className="absolute inset-0 motion-safe:animate-[citeloop-drawer-scrim-in_180ms_ease-out] bg-slate-950/25"
      />
      <aside
        ref={drawerRef}
        {...dataProps}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className={cx(
          "absolute right-0 top-0 flex h-[100dvh] max-h-[100dvh] w-full motion-safe:animate-[citeloop-drawer-panel-in_220ms_cubic-bezier(0.16,1,0.3,1)] flex-col overflow-hidden border-l border-slate-200 bg-white shadow-2xl",
          maxWidthClassName,
        )}
      >
        <div className="flex items-start justify-between gap-4 border-b border-slate-100 p-5">
          <div className="min-w-0">
            {eyebrow && <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">{eyebrow}</div>}
            <h3 id={titleId} className="mt-2 break-words text-xl font-bold leading-7 text-slate-950">
              {title}
            </h3>
            {subtitle && <div className="mt-2 break-words text-sm leading-6 text-slate-600">{subtitle}</div>}
            {badges && <div className="mt-3 flex flex-wrap items-center gap-2">{badges}</div>}
          </div>
          <button
            type="button"
            data-drawer-close
            aria-label={closeLabel}
            onClick={onClose}
            className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-slate-200 text-slate-500 transition hover:bg-slate-50 active:translate-y-px"
          >
            <X size={16} />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-5">{children}</div>

        {footer && (
          <div
            aria-label={footerLabel}
            className="shrink-0 flex flex-col gap-2 border-t border-slate-200 bg-white px-4 pb-[calc(1.5rem+env(safe-area-inset-bottom))] pt-4 sm:flex-row sm:justify-end"
          >
            {footer}
          </div>
        )}
      </aside>
    </div>
  );
}
