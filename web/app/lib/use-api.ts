"use client";

import { useAuth } from "@clerk/nextjs";
import { useMemo } from "react";
import { createApi } from "./api";

export function useApi() {
  const { getToken } = useAuth();
  return useMemo(() => createApi({ getToken }), [getToken]);
}
