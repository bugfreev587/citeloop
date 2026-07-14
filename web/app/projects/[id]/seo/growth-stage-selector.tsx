"use client";

import { useEffect, useId, useRef, useState } from "react";
import { Check, ChevronDown } from "lucide-react";
import { GROWTH_STAGE_OPTIONS, GrowthStage, growthStageOption } from "../../../lib/growth-stage";
import { cx } from "../../../components/ui";

export function GrowthStageSelector({
  value,
  disabled,
  onChange,
}: {
  value: GrowthStage;
  disabled?: boolean;
  onChange: (stage: GrowthStage) => void;
}) {
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(() => Math.max(0, GROWTH_STAGE_OPTIONS.findIndex((item) => item.key === value)));
  const rootRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const listboxId = useId();
  const selected = growthStageOption(value);

  useEffect(() => {
    setActiveIndex(Math.max(0, GROWTH_STAGE_OPTIONS.findIndex((item) => item.key === value)));
  }, [value]);

  useEffect(() => {
    if (!open) return;
    const close = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [open]);

  useEffect(() => {
    if (open) optionRefs.current[activeIndex]?.focus();
  }, [activeIndex, open]);

  function choose(stage: GrowthStage) {
    setOpen(false);
    window.requestAnimationFrame(() => triggerRef.current?.focus());
    onChange(stage);
  }

  function move(delta: number) {
    setActiveIndex((current) => (current + delta + GROWTH_STAGE_OPTIONS.length) % GROWTH_STAGE_OPTIONS.length);
  }

  return (
    <div ref={rootRef} data-growth-stage-selector className="relative min-w-[14rem]">
      <button
        ref={triggerRef}
        type="button"
        aria-label="Growth Stage"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listboxId}
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            setOpen(true);
            if (event.key === "ArrowUp") move(-1);
          }
        }}
        className="flex h-11 w-full items-center gap-3 rounded-xl border border-slate-200 bg-white px-3.5 text-left shadow-sm transition hover:border-slate-300 focus:outline-none focus:ring-2 focus:ring-slate-300 disabled:cursor-not-allowed disabled:opacity-50"
      >
        <span className="shrink-0 text-xs font-semibold text-slate-500">Growth Stage</span>
        <span className="min-w-0 flex-1 truncate text-[15px] font-bold text-slate-950">{selected.label}</span>
        <ChevronDown aria-hidden="true" size={16} className={cx("shrink-0 text-slate-500 transition-transform", open && "rotate-180")} />
      </button>

      {open && (
        <div
          id={listboxId}
          role="listbox"
          aria-label="Growth Stage"
          className="absolute right-0 z-30 mt-2 w-[min(24rem,calc(100vw-2rem))] overflow-hidden rounded-xl border border-slate-200 bg-white p-1.5 shadow-xl"
        >
          {GROWTH_STAGE_OPTIONS.map((option, index) => {
            const isSelected = option.key === value;
            return (
              <button
                key={option.key}
                ref={(node) => { optionRefs.current[index] = node; }}
                type="button"
                role="option"
                aria-selected={isSelected}
                tabIndex={index === activeIndex ? 0 : -1}
                onMouseEnter={() => setActiveIndex(index)}
                onClick={() => choose(option.key)}
                onKeyDown={(event) => {
                  if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                    event.preventDefault();
                    move(event.key === "ArrowDown" ? 1 : -1);
                  } else if (event.key === "Home" || event.key === "End") {
                    event.preventDefault();
                    setActiveIndex(event.key === "Home" ? 0 : GROWTH_STAGE_OPTIONS.length - 1);
                  } else if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    choose(option.key);
                  } else if (event.key === "Escape") {
                    event.preventDefault();
                    setOpen(false);
                    window.requestAnimationFrame(() => triggerRef.current?.focus());
                  }
                }}
                className={cx(
                  "flex w-full items-start gap-3 rounded-lg px-3 py-2.5 text-left outline-none transition",
                  index === activeIndex ? "bg-slate-100" : "hover:bg-slate-50",
                )}
              >
                <span className="min-w-0 flex-1">
                  <span className="block text-[15px] font-bold leading-5 text-slate-950">{option.label}</span>
                  <span className="mt-0.5 block text-xs leading-4 text-slate-500">{option.description}</span>
                </span>
                <Check aria-hidden="true" size={16} className={cx("mt-0.5 shrink-0 text-emerald-600", !isSelected && "invisible")} />
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
