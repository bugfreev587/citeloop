# Token Provider OpenAI Fallback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Retry TokenGate/OpenAI-compatible calls with OpenAI model IDs when a Claude model is rejected by an OpenAI-routed provider.

**Architecture:** Keep CiteLoop's Claude-first defaults intact, but add a narrow retry inside `internal/llm.OpenAIChat`. When the gateway returns a 400 saying a `claude-*` model is unsupported, retry the same request once with `gpt-5.1` for writer/default calls and `gpt-5.5` for QA calls. Update Admin placeholders so operators see the intended OpenAI fallback models.

**Tech Stack:** Go `net/http` LLM client, Go unit tests, Next.js/React Admin UI placeholders.

---

### Task 1: Add Regression Test For Unsupported Claude Retry

**Files:**
- Modify: `internal/llm/openai_test.go`

- [x] **Step 1: Write the failing test**

Add this test above `TestOpenAIChatCompleteRequiresAPIKey`:

```go
func TestOpenAIChatCompleteRetriesUnsupportedClaudeModelWithOpenAIFallback(t *testing.T) {
	tests := []struct {
		name         string
		req          CompletionReq
		wantFallback string
	}{
		{
			name:         "writer uses gpt-5.1",
			req:          CompletionReq{Prompt: "write", Purpose: PurposeWriter, Model: "claude-sonnet-4-6"},
			wantFallback: "gpt-5.1",
		},
		{
			name:         "qa uses gpt-5.5",
			req:          CompletionReq{Prompt: "check", Purpose: PurposeQA, Model: "claude-opus-4-8"},
			wantFallback: "gpt-5.5",
		},
		{
			name:         "default uses gpt-5.1",
			req:          CompletionReq{Prompt: "plan", Model: "claude-sonnet-4-6"},
			wantFallback: "gpt-5.1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotModels []string
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				model := body["model"].(string)
				gotModels = append(gotModels, model)
				if len(gotModels) == 1 {
					return &http.Response{
						StatusCode: 400,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body: io.NopCloser(strings.NewReader(`{"error":{"message":"The '` + model + `' model is not supported when using Codex with a ChatGPT account.","type":"invalid_request_error"}}`)),
					}, nil
				}
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{
						"id": "chatcmpl_retry",
						"model": "` + tc.wantFallback + `",
						"choices": [{"message": {"role": "assistant", "content": "ok"}}],
						"usage": {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}
					}`)),
				}, nil
			})

			p := NewOpenAIChat("tg-test-key", "https://tokengate.example/v1", "claude-sonnet-4-6")
			p.client = &http.Client{Transport: transport}
			resp, err := p.Complete(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("complete: %v", err)
			}
			if len(gotModels) != 2 {
				t.Fatalf("models sent = %#v, want initial and retry", gotModels)
			}
			if gotModels[1] != tc.wantFallback {
				t.Fatalf("fallback model = %q, want %q", gotModels[1], tc.wantFallback)
			}
			if resp.Model != tc.wantFallback {
				t.Fatalf("resp model = %q, want %q", resp.Model, tc.wantFallback)
			}
		})
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm -run TestOpenAIChatCompleteRetriesUnsupportedClaudeModelWithOpenAIFallback -count=1`

Expected: FAIL because `OpenAIChat.Complete` returns the first 400 error and never retries.

### Task 2: Implement Narrow Retry

**Files:**
- Modify: `internal/llm/llm.go`
- Modify: `internal/llm/openai.go`

- [x] **Step 1: Add fallback model constants**

Add these constants in `internal/llm/llm.go` near `DefaultTokenGateModel`:

```go
DefaultOpenAIModel       = "gpt-5.1"
DefaultOpenAIWriterModel = "gpt-5.1"
DefaultOpenAIQAModel     = "gpt-5.5"
```

- [x] **Step 2: Add retry helpers**

Add helpers in `internal/llm/openai.go`:

```go
func fallbackOpenAIModel(purpose CompletionPurpose) string {
	switch purpose {
	case PurposeQA:
		return DefaultOpenAIQAModel
	case PurposeWriter:
		return DefaultOpenAIWriterModel
	default:
		return DefaultOpenAIModel
	}
}

func shouldRetryUnsupportedClaudeModel(status int, model string, raw []byte) bool {
	if status != http.StatusBadRequest || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "claude-") {
		return false
	}
	msg := strings.ToLower(string(raw))
	return strings.Contains(msg, "not supported") &&
		(strings.Contains(msg, "chatgpt account") || strings.Contains(msg, "openai") || strings.Contains(msg, "codex"))
}
```

- [x] **Step 3: Refactor request sending and retry once**

Extract the existing HTTP send/decode body into a helper that accepts a model string, then in `Complete` call it with the requested model. If it returns a retryable unsupported-Claude error, call it once more with `fallbackOpenAIModel(req.Purpose)`. Do not retry if the fallback is blank or equals the original model.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/llm -run TestOpenAIChatCompleteRetriesUnsupportedClaudeModelWithOpenAIFallback -count=1`

Expected: PASS.

### Task 3: Align Admin Model Placeholders

**Files:**
- Modify: `web/app/admin/page.tsx`
- Modify: `web/app/projects/[id]/admin/admin-client.tsx`

- [x] **Step 1: Update placeholders**

Change writer placeholders from `gpt-5.1-mini` to `gpt-5.1` and QA placeholders from `gpt-5.1` to `gpt-5.5`.

- [x] **Step 2: Verify no snapshot contract depends on old placeholder values**

Run: `npm --prefix web test -- --runInBand` if the project defines a test script. If not, run the existing Node contract tests directly with the repo's package script.

### Task 4: Full Verification And PR

**Files:**
- Verify all touched files.

- [x] **Step 1: Run backend tests**

Run: `go test ./...`

Expected: all packages pass.

- [x] **Step 2: Run frontend tests**

Run: `npm test --prefix web`

Expected: all web tests pass.

- [ ] **Step 3: Commit, push, open PR, merge, deploy, verify production**

Use the repository's GitHub/Vercel/Railway deployment flow. Production verification must exercise Admin Test connection or equivalent API behavior after deploy.
