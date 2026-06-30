-- Global administrator GEO provider credentials. Every key stored here is a
-- TokenGate-issued key; provider-specific routing is selected inside TokenGate
-- by the configured model name.

create table if not exists admin_geo_provider_credentials (
  scope text not null primary key check (scope in ('perplexity','openai','anthropic','gemini')),
  provider text not null default 'tokengate' check (provider = 'tokengate'),
  api_key text not null,
  base_url text not null default '',
  model text not null default '',
  enabled boolean not null default true,
  updated_at timestamptz not null default now()
);
