alter table admin_llm_credentials
  add column if not exists model_routes jsonb not null default '{}'::jsonb;
