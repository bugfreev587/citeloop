export function consumeHandoffSearchParams(...names: string[]) {
  if (typeof window === "undefined" || names.length === 0) return;

  const url = new URL(window.location.href);
  let changed = false;
  for (const name of names) {
    if (!url.searchParams.has(name)) continue;
    url.searchParams.delete(name);
    changed = true;
  }
  if (!changed) return;

  window.history.replaceState(window.history.state, "", url.pathname + url.search + url.hash);
}
