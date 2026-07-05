"use client";

import { createContext, useContext, type ReactNode } from "react";
import { createPortal } from "react-dom";

export const ContentWorkflowStageHeaderActionContext = createContext<HTMLElement | null>(null);

export function ContentWorkflowStageHeaderAction({ children }: { children: ReactNode }) {
  const target = useContext(ContentWorkflowStageHeaderActionContext);
  return target ? createPortal(children, target) : null;
}
