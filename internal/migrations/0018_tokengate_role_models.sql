-- CiteLoop always talks to TokenGate. Model IDs are TokenGate aliases and can
-- point at OpenAI, Claude, or another backend inside TokenGate.

alter table admin_llm_credentials
  add column if not exists model text not null default '',
  add column if not exists writer_model text not null default '',
  add column if not exists qa_model text not null default '';

update admin_llm_credentials
set provider = 'tokengate'
where provider <> 'tokengate';

alter table admin_llm_credentials
  drop constraint if exists admin_llm_credentials_provider_check;

alter table admin_llm_credentials
  add constraint admin_llm_credentials_provider_check
  check (provider = 'tokengate');
