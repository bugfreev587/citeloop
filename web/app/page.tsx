import Link from "next/link";
import { api } from "./lib/api";

export default async function Home() {
  let projects: Awaited<ReturnType<typeof api.listProjects>> = [];
  let error: string | null = null;
  try {
    projects = await api.listProjects();
  } catch (e: any) {
    error = e.message;
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Projects</h1>
      {error && (
        <div className="rounded border border-red-200 bg-red-50 p-4 text-sm text-red-700">
          Could not reach the API ({error}). Start the Go service and set NEXT_PUBLIC_API_URL.
        </div>
      )}
      <div className="grid gap-3">
        {projects.map((p) => (
          <Link
            key={p.id}
            href={`/projects/${p.id}`}
            className="rounded-lg border bg-white p-4 hover:border-neutral-400"
          >
            <div className="font-medium">{p.name}</div>
            <div className="text-xs text-neutral-500">/{p.slug}</div>
          </Link>
        ))}
        {!error && projects.length === 0 && (
          <div className="text-sm text-neutral-500">No projects yet.</div>
        )}
      </div>
    </div>
  );
}
