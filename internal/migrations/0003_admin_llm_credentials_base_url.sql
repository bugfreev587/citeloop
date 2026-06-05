-- Let administrator-managed LLM credentials target TokenGate or another
-- OpenAI-compatible gateway by storing the provider base URL.

alter table admin_llm_credentials
  add column if not exists base_url text not null default '';

alter table admin_llm_credentials
  drop constraint if exists admin_llm_credentials_provider_check;

alter table admin_llm_credentials
  add constraint admin_llm_credentials_provider_check
  check (provider in ('tokengate','openai','claude'));
