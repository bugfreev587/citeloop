import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, TextareaHTMLAttributes } from "react";
import { Loader2 } from "lucide-react";

export function cx(...values: Array<string | false | null | undefined>) {
  return values.filter(Boolean).join(" ");
}

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "outline" | "ghost" | "danger";
  size?: "sm" | "md";
};

export function Button({ className, variant = "outline", size = "md", ...props }: ButtonProps) {
  const variants = {
    primary:
      "border-transparent bg-gradient-to-r from-[#d93820] to-[#f4503b] text-white hover:brightness-[1.02]",
    outline: "border-slate-200 bg-white text-slate-700 hover:bg-slate-50 hover:text-slate-950",
    ghost: "border-transparent bg-transparent text-slate-500 hover:bg-slate-100 hover:text-slate-950",
    danger: "border-red-200 bg-white text-red-700 hover:bg-red-50",
  };
  const sizes = {
    sm: "h-8 rounded-lg px-3 text-xs",
    md: "h-10 rounded-xl px-3 text-sm",
  };

  return (
    <button
      {...props}
      className={cx(
        "inline-flex items-center justify-center gap-2 border font-medium transition-all duration-150 active:scale-[0.97] disabled:cursor-not-allowed disabled:opacity-45",
        variants[variant],
        sizes[size],
        className,
      )}
    />
  );
}

export function ButtonProgress({
  busy,
  busyLabel,
  idleIcon,
  children,
  size = 14,
}: {
  busy: boolean;
  busyLabel: string;
  idleIcon: ReactNode;
  children?: ReactNode;
  size?: number;
}) {
  return (
    <>
      {busy ? <Loader2 aria-hidden="true" className="animate-spin" size={size} /> : idleIcon}
      {(busy || children) && (
        <span role={busy ? "status" : undefined} aria-live={busy ? "polite" : undefined}>
          {busy ? busyLabel : children}
        </span>
      )}
    </>
  );
}

export function Badge({
  children,
  tone = "neutral",
  className,
}: {
  children: React.ReactNode;
  tone?: "neutral" | "red" | "amber" | "green" | "blue" | "violet";
  className?: string;
}) {
  const tones = {
    neutral: "bg-slate-100 text-slate-600",
    red: "bg-red-50 text-red-700 ring-red-100",
    amber: "bg-amber-50 text-amber-800 ring-amber-100",
    green: "bg-green-50 text-green-700 ring-green-100",
    blue: "bg-sky-50 text-sky-700 ring-sky-100",
    violet: "bg-violet-50 text-violet-700 ring-violet-100",
  };

  return (
    <span
      className={cx(
        "inline-flex h-6 items-center rounded-md px-2 text-xs font-semibold ring-1 ring-inset",
        tones[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}

export function SectionHeader({
  title,
  action,
  eyebrow,
}: {
  title: string;
  action?: React.ReactNode;
  eyebrow?: string;
}) {
  return (
    <div className="mb-3 flex min-h-8 items-center justify-between gap-3">
      <div>
        {eyebrow && <div className="mb-0.5 text-[13px] font-semibold text-slate-500">{eyebrow}</div>}
        <h2 className="text-xl font-bold leading-7 text-slate-900">{title}</h2>
      </div>
      {action}
    </div>
  );
}

export function EmptyState({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="rounded-lg border border-dashed border-slate-200 bg-white px-4 py-5 text-sm">
      <div className="font-semibold text-slate-800">{title}</div>
      <div className="mt-1 text-slate-500">{detail}</div>
    </div>
  );
}

export function Notice({
  title,
  detail,
  tone = "neutral",
}: {
  title: string;
  detail?: string;
  tone?: "neutral" | "red" | "amber" | "green";
}) {
  const tones = {
    neutral: "border-slate-200 bg-white text-slate-700",
    red: "border-red-200 bg-red-50 text-red-800",
    amber: "border-amber-200 bg-amber-50 text-amber-900",
    green: "border-green-200 bg-green-50 text-green-800",
  };
  return (
    <div className={cx("rounded-lg border px-4 py-3 text-sm", tones[tone])}>
      <div className="font-semibold">{title}</div>
      {detail && <div className="mt-1 opacity-80">{detail}</div>}
    </div>
  );
}

export function TextInput({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={cx(
        "h-10 rounded-lg border border-slate-200 bg-white px-3 text-sm text-slate-900 transition-colors placeholder:text-slate-400 focus:border-slate-400",
        className,
      )}
    />
  );
}

export function TextArea({ className, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      {...props}
      className={cx(
        "rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 transition-colors placeholder:text-slate-400 focus:border-slate-400",
        className,
      )}
    />
  );
}

export function Field({
  label,
  helper,
  children,
}: {
  label: string;
  helper?: string;
  children: React.ReactNode;
}) {
  return (
    <label className="grid gap-2 text-sm">
      <span className="font-semibold text-slate-700">{label}</span>
      {children}
      {helper && <span className="text-xs text-slate-500">{helper}</span>}
    </label>
  );
}

export function formatDate(value: string | null) {
  if (!value) return "Not set";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("en", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(date);
}

export function formatScore(value: number | null) {
  if (value == null) return "-";
  return value.toFixed(1);
}
