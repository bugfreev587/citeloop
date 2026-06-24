import { StaticGSCCallbackClient } from "./gsc-callback-client";

export default async function StaticGSCCallbackPage({
  searchParams,
}: {
  searchParams: Promise<{ code?: string; state?: string; error?: string }>;
}) {
  const query = await searchParams;
  return <StaticGSCCallbackClient code={query.code ?? ""} state={query.state ?? ""} error={query.error ?? ""} />;
}
