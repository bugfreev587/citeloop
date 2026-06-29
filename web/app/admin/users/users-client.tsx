"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, ArrowLeft, Loader2, RefreshCw, ShieldCheck, Trash2, UserRound } from "lucide-react";
import { AdminUser } from "../../lib/api";
import { useApi } from "../../lib/use-api";
import { useToast } from "../../components/toast-provider";
import { Badge, Button, ButtonProgress, EmptyState, Notice, SectionHeader, cx } from "../../components/ui";

type Access = "loading" | "granted" | "denied";

function formatDateTime(value: any) {
  if (!value) return "Not set";
  const raw = typeof value === "string" ? value : value.Time ?? value.time ?? value;
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return String(raw);
  return new Intl.DateTimeFormat("en", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(date);
}

function userLabel(user: AdminUser) {
  return user.owner_email || user.owner_id || "Unknown user";
}

function projectCountLabel(count: number) {
  return `${count} project${count === 1 ? "" : "s"}`;
}

export function UsersClient() {
  const api = useApi();
  const { notify } = useToast();
  const [access, setAccess] = useState<Access>("loading");
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [pendingDelete, setPendingDelete] = useState<AdminUser | null>(null);
  const [deletingOwnerID, setDeletingOwnerID] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const me = await api.getMe();
      if (!me.is_admin) {
        setAccess("denied");
        setUsers([]);
        return;
      }
      setAccess("granted");
      setUsers(await api.listAdminUsers());
    } catch (error: any) {
      if (String(error.message).includes("403")) {
        setAccess("denied");
        setUsers([]);
      } else {
        setAccess("granted");
        notify({ title: "Users unavailable", detail: error.message, tone: "red" });
      }
    } finally {
      setLoading(false);
    }
  }, [api, notify]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const projectCount = useMemo(() => users.reduce((sum, user) => sum + user.project_count, 0), [users]);

  function openDelete(user: AdminUser) {
    if (deletingOwnerID !== null) return;
    setPendingDelete(user);
  }

  function closeDelete() {
    if (deletingOwnerID !== null) return;
    setPendingDelete(null);
  }

  async function deleteUser() {
    if (!pendingDelete) return;
    const ownerID = pendingDelete.owner_id;
    setDeletingOwnerID(ownerID);
    try {
      const result = await api.deleteAdminUser(ownerID);
      setUsers((current) => current.filter((user) => user.owner_id !== ownerID));
      notify({
        title: "User data deleted",
        detail: `${userLabel(pendingDelete)} and ${projectCountLabel(result.deleted_projects)} were removed from CiteLoop.`,
        tone: "green",
      });
      setPendingDelete(null);
    } catch (error: any) {
      notify({ title: "Delete failed", detail: error.message, tone: "red" });
    } finally {
      setDeletingOwnerID(null);
    }
  }

  if (access === "loading") {
    return (
      <main className="grid min-h-[60vh] place-items-center text-sm text-slate-500">
        <span className="inline-flex items-center gap-2">
          <Loader2 size={16} className="animate-spin" />
          Checking admin access...
        </span>
      </main>
    );
  }

  if (access === "denied") {
    return (
      <main className="mx-auto max-w-md px-4 py-16 text-center">
        <ShieldCheck className="mx-auto text-slate-300" size={40} />
        <h1 className="mt-4 text-xl font-bold text-slate-900">Admin access required</h1>
        <p className="mt-2 text-sm leading-6 text-slate-500">
          This user management page is limited to platform administrators.
        </p>
        <Link href="/docs" className="mt-4 inline-flex items-center gap-2 text-sm font-semibold text-[#d93820] hover:underline">
          <ArrowLeft size={14} />
          Back to docs
        </Link>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-[1180px] px-4 py-6 md:px-6 md:py-8">
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <Link href="/admin" className="inline-flex items-center gap-2 text-sm font-semibold text-slate-500 hover:text-slate-900">
          <ArrowLeft size={15} />
          Admin
        </Link>
        <Badge tone="neutral">Users</Badge>
      </div>

      <SectionHeader
        title="Users"
        eyebrow="Owner-level management across all CiteLoop accounts"
        action={
          <Button disabled={loading || deletingOwnerID !== null} onClick={refresh}>
            <ButtonProgress busy={loading} busyLabel="Refreshing" idleIcon={<RefreshCw size={15} />}>
              Refresh
            </ButtonProgress>
          </Button>
        }
      />

      <div className="mb-4 grid gap-3 md:grid-cols-3">
        <div className="rounded-lg border border-slate-200 bg-white px-4 py-3">
          <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Users</div>
          <div className="mt-1 text-2xl font-bold text-slate-950">{users.length}</div>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white px-4 py-3">
          <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Projects</div>
          <div className="mt-1 text-2xl font-bold text-slate-950">{projectCount}</div>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white px-4 py-3">
          <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Scope</div>
          <div className="mt-1 text-sm font-semibold text-slate-700">All accounts</div>
        </div>
      </div>

      <Notice
        title="Deleting is permanent"
        detail="Deleting a user removes every CiteLoop project owned by that Owner ID and all project data below it. Clerk identity deletion is not performed here."
        tone="amber"
      />

      <section className="mt-4 overflow-hidden rounded-xl border border-slate-200 bg-white">
        {users.length === 0 && !loading ? (
          <div className="p-4">
            <EmptyState title="No users found" detail="There are no CiteLoop project owners in this environment." />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-slate-200 text-sm">
              <thead className="bg-slate-50 text-left text-xs font-bold uppercase tracking-[0.08em] text-slate-500">
                <tr>
                  <th className="px-4 py-3">Owner email</th>
                  <th className="px-4 py-3">Owner ID</th>
                  <th className="px-4 py-3">Projects</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3">Last updated at</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {users.map((user) => {
                  const deleting = deletingOwnerID === user.owner_id;
                  return (
                    <tr key={user.owner_id} className={cx("align-top", deleting && "bg-red-50/40")}>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-3">
                          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-500">
                            <UserRound size={16} />
                          </div>
                          {user.owner_email ? (
                            <span className="font-medium text-slate-800">{user.owner_email}</span>
                          ) : (
                            <Badge tone="amber">Unknown</Badge>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="max-w-[320px] break-all font-mono text-xs text-slate-500">{user.owner_id || "Unknown"}</div>
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 font-semibold text-slate-700">{user.project_count}</td>
                      <td className="whitespace-nowrap px-4 py-3 text-slate-600">{formatDateTime(user.created_at)}</td>
                      <td className="whitespace-nowrap px-4 py-3 text-slate-600">{formatDateTime(user.updated_at ?? user.created_at)}</td>
                      <td className="px-4 py-3">
                        <div className="flex justify-end">
                          <Button
                            size="sm"
                            variant="danger"
                            disabled={deletingOwnerID !== null}
                            onClick={() => openDelete(user)}
                          >
                            <ButtonProgress busy={deleting} busyLabel="Deleting" idleIcon={<Trash2 size={14} />}>
                              Delete
                            </ButtonProgress>
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {pendingDelete && (
        <div className="fixed inset-0 z-50 grid place-items-center bg-slate-950/40 p-4">
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="delete-user-title"
            className="w-full max-w-md rounded-xl border border-red-200 bg-white p-4 shadow-xl"
          >
            <div className="flex items-start gap-3">
              <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-red-50 text-red-700">
                <AlertTriangle size={18} />
              </div>
              <div className="min-w-0 flex-1">
                <h2 id="delete-user-title" className="text-base font-bold text-slate-950">
                  Delete {userLabel(pendingDelete)}
                </h2>
                <p className="mt-1 text-sm leading-6 text-slate-600">
                  This permanently removes {projectCountLabel(pendingDelete.project_count)} and all associated CiteLoop data for this Owner ID.
                </p>
                <div className="mt-3 break-all rounded-lg bg-slate-50 px-3 py-2 font-mono text-xs text-slate-500">
                  {pendingDelete.owner_id}
                </div>
              </div>
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <Button type="button" variant="outline" disabled={deletingOwnerID !== null} onClick={closeDelete}>
                Cancel
              </Button>
              <Button type="button" variant="danger" disabled={deletingOwnerID !== null} onClick={deleteUser}>
                <ButtonProgress busy={deletingOwnerID === pendingDelete.owner_id} busyLabel="Deleting" idleIcon={<Trash2 size={16} />}>
                  Delete
                </ButtonProgress>
              </Button>
            </div>
          </div>
        </div>
      )}
    </main>
  );
}
