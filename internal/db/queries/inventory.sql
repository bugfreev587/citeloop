-- name: UpsertInventory :one
insert into content_inventory
  (project_id, url, title, target_keyword, topics, summary, evidence_snippets, source)
values ($1, $2, $3, $4, $5, $6, $7, $8)
on conflict (project_id, url) do update set
  title = excluded.title,
  target_keyword = excluded.target_keyword,
  topics = excluded.topics,
  summary = excluded.summary,
  evidence_snippets = excluded.evidence_snippets,
  source = excluded.source,
  captured_at = now()
returning *;

-- name: ListInventory :many
select * from content_inventory
where project_id = $1
order by captured_at desc;

-- name: GetInventoryItem :one
select * from content_inventory where id = $1;

-- name: UpdateInventoryItem :one
update content_inventory set
  title = $2, target_keyword = $3, topics = $4, summary = $5
where id = $1
returning *;

-- name: DeleteInventoryItem :exec
delete from content_inventory where id = $1;
