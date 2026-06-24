"use client";

import { useEffect } from "react";
import { LAST_PROJECT_STORAGE_KEY } from "./lib/dashboard-routing";

export function ProjectVisitRecorder({ projectId }: { projectId: string }) {
  useEffect(() => {
    window.localStorage.setItem(LAST_PROJECT_STORAGE_KEY, projectId);
  }, [projectId]);

  return null;
}
