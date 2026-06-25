# TokenGate Role Models Design

## Goal

CiteLoop should always call TokenGate's OpenAI-compatible API and let TokenGate decide whether the underlying model is OpenAI, Claude, or another provider. The Admin surface should configure TokenGate credentials and model IDs globally, with separate model choices for writing and QA.

## Current State

The runtime already has administrator-managed LLM credentials and a project Admin page. That flow still exposes provider choices (`tokengate`, `openai`, `claude`) and uses one environment-level model for every OpenAI-compatible call. Writer and QA share one `llm.Provider`, so they cannot use different models.

## Design

The platform stores and returns only TokenGate settings:

- API key
- TokenGate base URL
- default model
- writer model
- QA model

The backend keeps the existing credential row for compatibility but saves `provider='tokengate'` on every Admin update. Role-specific model columns live on `admin_llm_credentials`. Existing rows use environment defaults until an administrator saves explicit models.

`llm.CompletionReq` gains a purpose field. Writer requests mark `writer`, QA requests mark `qa`, and all other calls use the default purpose. The Admin runtime provider selects the model by purpose:

- `writer` uses `writer_model`, falling back to `default_model`
- `qa` uses `qa_model`, falling back to `default_model`
- default uses `default_model`
- if the chosen model is blank, it falls back to `TOKENGATE_MODEL`

All selected models are passed to `llm.NewOpenAIChat` with the saved TokenGate API key and base URL.

## UI

The Admin page removes the OpenAI and Claude provider cards. It shows one TokenGate credential panel with fields for base URL, API key, default model, writer model, and QA model. The page continues to return only redacted credential status, not raw secrets.

## Testing

Backend tests cover TokenGate-only credential updates, model preservation/fallbacks, and role-based provider selection. Frontend tests cover status normalization, update payload shape, and Admin UI source contracts that prevent reintroducing provider switching.
