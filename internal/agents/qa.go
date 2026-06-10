package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
)

// QA is the two-layer QA step (PRD §5.3): evidence mapping (the real blocking
// gate) + LLM scoring. Reused by the Writer at draft time and by the review
// edit flow to honestly re-check after a human edits content — clearing
// qa_blocking only ever happens by actually re-running this, never by flipping
// a flag (§5.5).
type QA struct {
	Deps
	Log *slog.Logger
}

func NewQA(d Deps, log *slog.Logger) *QA {
	if log == nil {
		log = slog.Default()
	}
	return &QA{Deps: d, Log: log}
}

// Check audits content against the project's profile + inventory evidence.
func (qa *QA) Check(ctx context.Context, projectID uuid.UUID, contentMD string, profileJSON json.RawMessage) (*QAOutput, llm.CompletionResp, error) {
	inv, _ := qa.Q.ListInventory(ctx, projectID)
	var evidence string
	for _, it := range inv {
		var snips []string
		_ = json.Unmarshal(it.EvidenceSnippets, &snips)
		for _, s := range snips {
			evidence += "- " + s + "\n"
		}
	}
	prompt := fmt.Sprintf(`[[QA]] Audit this article. Two layers:
1) EVIDENCE MAPPING (blocking): extract every factual claim about the product. Each must map to the profile.features, source content, or evidence snippets. Any claim that cannot be mapped sets qa_blocking=true.
2) SCORING: geo_score and seo_score in [0,1], plus issues[].
3) EDITOR FEEDBACK: if qa_blocking=true, provide exact fix_instructions for the AI editor and human_decision_options for unresolved cases.
Return JSON: {claims:[{claim,mapped,evidence}], qa_blocking, geo_score, seo_score, issues[], blocking_issues:[{code,severity,message,claim?}], fix_instructions[], human_decision_options:[{label,description}], blocking_reason, can_auto_fix}.

Blocking standards:
- Block only factual product claims that cannot be mapped to profile/features/source/evidence, malformed content that prevents review, or missing required SEO metadata.
- Do not block for style preferences, internal-link opportunities, or non-critical SEO improvements.
- can_auto_fix=true when the editor can safely remove, rewrite, or normalize the draft using the available evidence.
- can_auto_fix=false only when the product evidence itself is missing or a human must choose a positioning decision.

PROFILE:
%s

EVIDENCE SNIPPETS:
%s

ARTICLE:
%s`, clip(string(profileJSON), 2000), clip(evidence, 3000), clip(contentMD, 6000))

	resp, err := qa.LLM.Complete(ctx, llm.CompletionReq{
		System: "You are a strict fact-checking QA auditor. Unmapped product claims are blocking.",
		Prompt: prompt, JSON: true, MaxTokens: 2000,
	})
	if err != nil {
		return nil, resp, err
	}
	out, err := extractQAOutput(resp.Text)
	if err != nil {
		fallback, fallbackResp, fallbackErr := qa.compactCheck(ctx, profileJSON, evidence, contentMD)
		if fallbackErr != nil {
			return nil, fallbackResp, fmt.Errorf("parse qa: %w; compact fallback failed: %w", err, fallbackErr)
		}
		out = *fallback
		resp = fallbackResp
	}
	// Defense in depth: any unmapped claim forces blocking regardless of the
	// model's own flag (§5.3 acceptance).
	for _, c := range out.Claims {
		if !c.Mapped {
			out.QABlocking = true
		}
	}
	enforceQAGate(&out)
	enforceBannedClaims(&out, profileJSON, contentMD)
	return &out, resp, nil
}

func enforceQAGate(out *QAOutput) {
	if out == nil {
		return
	}
	if out.GeoScore < 0.75 || out.SeoScore < 0.75 {
		out.QABlocking = true
		out.Issues = append(out.Issues, "qa score below publish threshold")
	}
}

// enforceBannedClaims is a deterministic guardrail: if the draft literally
// contains a claim the project marked as banned (brand/legal guardrail in the
// profile), block regardless of the model's judgment. It is auto-fixable — the
// editor loop is instructed to strip banned_claims — so the writer can resolve
// it without a human in most cases (§5.3 evidence safety).
func enforceBannedClaims(out *QAOutput, profileJSON json.RawMessage, contentMD string) {
	if out == nil {
		return
	}
	var p struct {
		BannedClaims []string `json:"banned_claims"`
	}
	if err := json.Unmarshal(profileJSON, &p); err != nil {
		return
	}
	body := strings.ToLower(contentMD)
	for _, claim := range p.BannedClaims {
		claim = strings.TrimSpace(claim)
		if claim == "" {
			continue
		}
		if strings.Contains(body, strings.ToLower(claim)) {
			out.QABlocking = true
			out.CanAutoFix = true
			issue := "banned claim present: " + claim
			out.Issues = append(out.Issues, issue)
			if strings.TrimSpace(out.BlockingReason) == "" {
				out.BlockingReason = issue
			}
		}
	}
}

func (qa *QA) compactCheck(ctx context.Context, profileJSON json.RawMessage, evidence, contentMD string) (*QAOutput, llm.CompletionResp, error) {
	prompt := fmt.Sprintf(`[[QA_COMPACT]] Audit this article. Return only this compact JSON object shape:
{"claims":[{"claim":"short product claim","mapped":true,"evidence":"profile or evidence snippet"}],"qa_blocking":false,"geo_score":0.5,"seo_score":0.5,"issues":[],"blocking_issues":[],"fix_instructions":[],"human_decision_options":[],"blocking_reason":"","can_auto_fix":false}

Rules:
- claims must be an array, even if empty.
- issues must be an array, even if empty.
- blocking_issues, fix_instructions, and human_decision_options must be arrays.
- scores must be numbers from 0 to 1.
- Set qa_blocking=true if an important product claim cannot be mapped.
- Set can_auto_fix=true only when a safe editor rewrite can resolve the blocking issue using available evidence.
- Keep each claim under 120 characters.

PROFILE:
%s

EVIDENCE SNIPPETS:
%s

ARTICLE EXCERPT:
%s`, clip(string(profileJSON), 1200), clip(evidence, 1200), clip(contentMD, 2500))

	resp, err := qa.LLM.Complete(ctx, llm.CompletionReq{
		System: "You are a strict fact-checking QA auditor. Return only compact JSON.",
		Prompt: prompt, JSON: true, MaxTokens: 1200,
	})
	if err != nil {
		return nil, resp, err
	}
	out, err := extractQAOutput(resp.Text)
	if err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

// Requalify re-runs QA on an existing article's current content and persists the
// recomputed scores, issues, and qa_blocking. This is the only path that can
// clear qa_blocking — used after a reviewer edits content (§5.5). Status is
// preserved. Returns the updated article.
func (qa *QA) Requalify(ctx context.Context, projectID, articleID uuid.UUID) (db.Article, error) {
	art, err := qa.Q.GetArticle(ctx, articleID)
	if err != nil {
		return db.Article{}, err
	}
	profile, err := qa.Q.GetActiveProfile(ctx, projectID)
	if err != nil {
		return db.Article{}, fmt.Errorf("no active profile: %w", err)
	}
	out, resp, qerr := qa.Check(ctx, projectID, art.ContentMd, profile.Profile)
	recordRun(ctx, qa.Q, projectID, agentQA, map[string]any{"requalify": articleID}, out, resp, qerr)
	if qerr != nil {
		// On QA failure, keep it blocking — never silently clear (§5.5).
		out = &QAOutput{QABlocking: true, Issues: []string{"qa re-check failed: " + qerr.Error()}}
	}
	return qa.Q.SetArticleQA(ctx, db.SetArticleQAParams{
		ID:         articleID,
		GeoScore:   pgutil.Numeric(out.GeoScore),
		SeoScore:   pgutil.Numeric(out.SeoScore),
		QaIssues:   toJSON(qaIssues(out)),
		QaBlocking: out.QABlocking,
		Status:     art.Status,
		QaFeedback: toJSON(out),
	})
}

func qaIssues(qa *QAOutput) []string {
	issues := append([]string{}, qa.Issues...)
	for _, c := range qa.Claims {
		if !c.Mapped {
			issues = append(issues, "unmapped product claim: "+c.Claim)
		}
	}
	return issues
}
