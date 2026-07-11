import { SiteFixesClient } from "./site-fixes-client";

export default async function SiteFixesPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ fix?: string | string[] }>;
}) {
  const [{ id }, query] = await Promise.all([params, searchParams]);
  const initialFixId = typeof query.fix === "string" ? query.fix : undefined;
  return <SiteFixesClient projectId={id} initialFixId={initialFixId} />;
}
