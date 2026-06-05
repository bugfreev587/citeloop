package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

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
Return JSON: {claims:[{claim,mapped,evidence}], qa_blocking, geo_score, seo_score, issues[]}.

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
	var out QAOutput
	if err := extractJSON(resp.Text, &out); err != nil {
		return nil, resp, fmt.Errorf("parse qa: %w", err)
	}
	// Defense in depth: any unmapped claim forces blocking regardless of the
	// model's own flag (§5.3 acceptance).
	for _, c := range out.Claims {
		if !c.Mapped {
			out.QABlocking = true
		}
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
