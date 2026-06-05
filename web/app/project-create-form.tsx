"use client";

import { useRouter } from "next/navigation";
import { FormEvent, useState } from "react";
import { Plus } from "lucide-react";
import { Button, Field, Notice, TextInput } from "./components/ui";
import { api } from "./lib/api";

function slugify(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}

export function ProjectCreateForm() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setError(null);
    const finalSlug = slugify(slug || name);
    if (!name.trim() || !finalSlug) {
      setError("Project name and slug are required.");
      return;
    }

    setBusy(true);
    try {
      const project = await api.createProject({ name: name.trim(), slug: finalSlug });
      router.push(`/projects/${project.id}`);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4">
      <div>
        <div className="text-sm font-bold text-slate-900">Create project</div>
        <div className="mt-1 text-sm text-slate-500">Start with a clean CiteLoop project shell.</div>
      </div>
      {error && <Notice title="Could not create project" detail={error} tone="red" />}
      <Field label="Project name">
        <TextInput value={name} onChange={(event) => setName(event.target.value)} placeholder="UniPost" />
      </Field>
      <Field label="Slug">
        <TextInput
          value={slug}
          onChange={(event) => setSlug(event.target.value)}
          placeholder={slugify(name) || "unipost"}
        />
      </Field>
      <Button disabled={busy} variant="primary" type="submit">
        <Plus size={16} />
        {busy ? "Creating" : "Create project"}
      </Button>
    </form>
  );
}
