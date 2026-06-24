"use client";

import {
  AlertTriangle,
  CheckCircle2,
  Info,
  type LucideIcon,
  X,
  XCircle,
} from "lucide-react";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { cx } from "./ui";

export type ToastTone = "neutral" | "red" | "green" | "amber";

type ToastInput = {
  title: string;
  detail?: string;
  tone?: ToastTone;
  duration?: number;
};

type ToastItem = {
  id: string;
  title: string;
  detail?: string;
  tone: ToastTone;
  duration: number;
};

type ToastContextValue = {
  notify: (input: ToastInput) => string;
  clearToasts: () => void;
  dismissToast: (id: string) => void;
};

const ToastContext = createContext<ToastContextValue | null>(null);

const toastDurations: Record<ToastTone, number> = {
  neutral: 3000,
  green: 3000,
  amber: 4000,
  red: 5000,
};

const toneStyles: Record<
  ToastTone,
  {
    accent: string;
    icon: string;
    shell: string;
    Icon: LucideIcon;
  }
> = {
  neutral: {
    accent: "#64748b",
    icon: "text-slate-500",
    shell: "border-slate-200 bg-white text-slate-900 shadow-slate-950/10",
    Icon: Info,
  },
  green: {
    accent: "#16a34a",
    icon: "text-green-600",
    shell: "border-green-100 bg-green-50 text-green-950 shadow-green-950/10",
    Icon: CheckCircle2,
  },
  amber: {
    accent: "#d97706",
    icon: "text-amber-600",
    shell: "border-amber-100 bg-amber-50 text-amber-950 shadow-amber-950/10",
    Icon: AlertTriangle,
  },
  red: {
    accent: "#dc2626",
    icon: "text-red-600",
    shell: "border-red-100 bg-red-50 text-red-950 shadow-red-950/10",
    Icon: XCircle,
  },
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const timeoutRefs = useRef(new Map<string, number>());

  const dismissToast = useCallback((id: string) => {
    const timeout = timeoutRefs.current.get(id);
    if (timeout) window.clearTimeout(timeout);
    timeoutRefs.current.delete(id);
    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);

  const clearToasts = useCallback(() => {
    timeoutRefs.current.forEach((timeout) => window.clearTimeout(timeout));
    timeoutRefs.current.clear();
    setToasts([]);
  }, []);

  const notify = useCallback(
    (input: ToastInput) => {
      const tone = input.tone ?? "neutral";
      const duration = input.duration ?? toastDurations[tone];
      const id = `toast-${Date.now()}-${Math.random().toString(36).slice(2)}`;
      const toast: ToastItem = {
        id,
        title: input.title,
        detail: input.detail,
        tone,
        duration,
      };

      setToasts((current) => {
        const next = [toast, ...current].slice(0, 4);
        for (const item of current) {
          if (!next.some((nextToast) => nextToast.id === item.id)) {
            const timeout = timeoutRefs.current.get(item.id);
            if (timeout) window.clearTimeout(timeout);
            timeoutRefs.current.delete(item.id);
          }
        }
        return next;
      });

      const timeout = window.setTimeout(() => dismissToast(id), duration);
      timeoutRefs.current.set(id, timeout);
      return id;
    },
    [dismissToast],
  );

  useEffect(() => {
    return () => {
      timeoutRefs.current.forEach((timeout) => window.clearTimeout(timeout));
      timeoutRefs.current.clear();
    };
  }, []);

  const value = useMemo(
    () => ({ notify, clearToasts, dismissToast }),
    [clearToasts, dismissToast, notify],
  );

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div
        className="pointer-events-none fixed right-4 top-4 z-[100] flex w-[min(420px,calc(100vw-2rem))] flex-col gap-3 sm:right-6 sm:top-6"
        aria-live="polite"
        aria-atomic="false"
      >
        {toasts.map((toast) => {
          const style = toneStyles[toast.tone];
          const Icon = style.Icon;
          return (
            <div
              key={toast.id}
              role={toast.tone === "red" ? "alert" : "status"}
              className={cx(
                "toast-card-shell pointer-events-auto relative overflow-hidden rounded-xl border px-4 py-3 shadow-lg",
                style.shell,
              )}
              style={
                {
                  "--toast-duration": `${toast.duration}ms`,
                  "--toast-accent": style.accent,
                } as CSSProperties
              }
            >
              <span aria-hidden="true" className="toast-progress-edge toast-progress-top" />
              <span aria-hidden="true" className="toast-progress-edge toast-progress-right" />
              <span aria-hidden="true" className="toast-progress-edge toast-progress-bottom" />
              <span aria-hidden="true" className="toast-progress-edge toast-progress-left" />
              <div className="relative flex items-start gap-3">
                <Icon aria-hidden="true" className={cx("mt-0.5 h-5 w-5 flex-none", style.icon)} />
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-bold leading-5">{toast.title}</div>
                  {toast.detail && <div className="mt-1 text-sm leading-5 opacity-75">{toast.detail}</div>}
                </div>
                <button
                  type="button"
                  aria-label="Dismiss notification"
                  className="rounded-md p-1 text-current opacity-45 transition hover:bg-black/5 hover:opacity-80"
                  onClick={() => dismissToast(toast.id)}
                >
                  <X aria-hidden="true" size={16} />
                </button>
              </div>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) throw new Error("useToast must be used inside ToastProvider");
  return context;
}
