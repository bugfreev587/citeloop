import { GSCCallbackClient } from "./gsc-callback-client";

export default async function GSCCallbackPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ code?: string; state?: string; error?: string }>;
}) {
  const { id } = await params;
  const query = await searchParams;
  return <GSCCallbackClient projectId={id} code={query.code ?? ""} state={query.state ?? ""} error={query.error ?? ""} />;
}
