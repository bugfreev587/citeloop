-- Global administrator LLM credentials. API keys are stored server-side and are
-- never returned by the HTTP API.

create table admin_llm_credentials (
  singleton  boolean primary key default true check (singleton),
  provider   text not null check (provider in ('openai','claude')),
  api_key    text not null,
  updated_at timestamptz not null default now()
);
