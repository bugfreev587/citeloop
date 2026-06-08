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

const maxQAFixAttempts int32 = 3

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
3) REPAIR GUIDANCE: if qa_blocking=true, provide blocking_reasons[] and fix_instructions[] that an AI editor can execute without asking the reviewer to rewrite the article.
Return JSON: {claims:[{claim,mapped,evidence}], qa_blocking, geo_score, seo_score, issues[], status, blocking_reasons[], fix_instructions[], human_decision_hints[], confidence}.

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
	normalizeQAOutput(&out)
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

func (qa *QA) compactCheck(ctx context.Context, profileJSON json.RawMessage, evidence, contentMD string) (*QAOutput, llm.CompletionResp, error) {
	prompt := fmt.Sprintf(`[[QA_COMPACT]] Audit this article. Return only this compact JSON object shape:
{"claims":[{"claim":"short product claim","mapped":true,"evidence":"profile or evidence snippet"}],"qa_blocking":false,"geo_score":0.5,"seo_score":0.5,"issues":[],"status":"passed","blocking_reasons":[],"fix_instructions":[],"human_decision_hints":[],"confidence":"medium"}

Rules:
- claims must be an array, even if empty.
- issues must be an array, even if empty.
- blocking_reasons, fix_instructions, and human_decision_hints must be arrays, even if empty.
- scores must be numbers from 0 to 1.
- Set qa_blocking=true if an important product claim cannot be mapped.
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
	normalizeQAOutput(&out)
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
	return qa.requalifyWithAttempt(ctx, projectID, art, art.QaAttemptCount)
}

// FixArticle lets the backend consume QA feedback itself. It revises the draft,
// reruns QA, and repeats up to the remaining attempt budget. It stops early when
// QA passes or when the same failure fingerprint repeats, then returns explicit
// human options instead of asking the reviewer to hunt through the draft.
func (qa *QA) FixArticle(ctx context.Context, projectID, articleID uuid.UUID) (db.Article, error) {
	art, err := qa.Q.GetArticle(ctx, articleID)
	if err != nil {
		return db.Article{}, err
	}
	if art.ProjectID != projectID {
		return db.Article{}, fmt.Errorf("article not found")
	}
	if art.QaStatus == QAStatusPassed && !art.QaBlocking {
		return art, nil
	}
	if art.QaAttemptCount >= maxQAFixAttempts {
		return qa.markNeedsHumanDecision(ctx, art, NeedsHumanDecisionState(articleStateFromDB(art), art.QaAttemptCount))
	}

	profile, err := qa.Q.GetActiveProfile(ctx, projectID)
	if err != nil {
		return db.Article{}, fmt.Errorf("no active profile: %w", err)
	}
	topic, _ := qa.Q.GetTopicForProject(ctx, db.GetTopicForProjectParams{ID: art.TopicID, ProjectID: projectID})
	previousFingerprint := strPtrValue(art.QaFailureFingerprint)
	current := art

	for current.QaAttemptCount < maxQAFixAttempts {
		nextAttempt := current.QaAttemptCount + 1
		fix, resp, fixErr := qa.reviseForQA(ctx, current, topic, profile.Profile, nextAttempt)
		recordRun(ctx, qa.Q, projectID, agentWriter, map[string]any{
			"ai_fix":      articleID,
			"attempt":     nextAttempt,
			"qa_status":   current.QaStatus,
			"qa_issues":   current.QaIssues,
			"topic_title": topic.Title,
		}, fix, resp, fixErr)
		if fixErr != nil {
			current.QaAttemptCount = nextAttempt
			state := NeedsHumanDecisionState(ArticleQAState{
				Status:             QAStatusNeedsHumanDecision,
				FailureKind:        "ai_fix_failed",
				FailureMessage:     fixErr.Error(),
				FailureFingerprint: qaFingerprint("ai_fix_failed", fixErr.Error(), nil),
			}, nextAttempt)
			return qa.markNeedsHumanDecision(ctx, current, state)
		}

		updated, err := qa.Q.UpdateArticleContentForProject(ctx, db.UpdateArticleContentForProjectParams{
			ID:        articleID,
			ContentMd: fix.ContentMD,
			SeoMeta:   toJSON(fix.SEOMeta),
			ProjectID: projectID,
		})
		if err != nil {
			return db.Article{}, err
		}

		current, err = qa.requalifyWithAttempt(ctx, projectID, updated, nextAttempt)
		if err != nil {
			return db.Article{}, err
		}
		if current.QaStatus == QAStatusPassed && !current.QaBlocking {
			return current, nil
		}
		nextFingerprint := strPtrValue(current.QaFailureFingerprint)
		if nextFingerprint != "" && nextFingerprint == previousFingerprint {
			return qa.markNeedsHumanDecision(ctx, current, NeedsHumanDecisionState(articleStateFromDB(current), current.QaAttemptCount))
		}
		previousFingerprint = nextFingerprint
	}
	return qa.markNeedsHumanDecision(ctx, current, NeedsHumanDecisionState(articleStateFromDB(current), current.QaAttemptCount))
}

func (qa *QA) requalifyWithAttempt(ctx context.Context, projectID uuid.UUID, art db.Article, attemptCount int32) (db.Article, error) {
	profile, err := qa.Q.GetActiveProfile(ctx, projectID)
	if err != nil {
		return db.Article{}, fmt.Errorf("no active profile: %w", err)
	}
	out, resp, qerr := qa.Check(ctx, projectID, art.ContentMd, profile.Profile)
	recordRun(ctx, qa.Q, projectID, agentQA, map[string]any{"requalify": art.ID, "attempt": attemptCount}, out, resp, qerr)
	state := BuildArticleQAState(out, qerr)
	if qerr != nil {
		// On QA failure, keep it blocking — never silently clear (§5.5).
		out = &QAOutput{Claims: []Claim{}, QABlocking: true, Issues: []string{"qa re-check failed: " + qerr.Error()}}
	}
	return qa.Q.SetArticleQA(ctx, setArticleQAParams(art.ID, art.Status, out, state, attemptCount))
}

func (qa *QA) reviseForQA(ctx context.Context, art db.Article, topic db.Topic, profileJSON json.RawMessage, attempt int32) (*AIFixOutput, llm.CompletionResp, error) {
	prompt := fmt.Sprintf(`[[AI_QA_FIX]] Revise this draft so it can pass QA with minimal human involvement.
Return JSON: {content_md, seo_meta:{title,meta_description,slug,h1,canonical_url?}, resolution_summary, human_decision_hints[]}.

Rules:
- Fix QA feedback first. Remove or qualify unsupported product claims unless profile or evidence supports them.
- If the SEO contribution says the keyword is missing, infer and use the target keyword from TOPIC TARGET KEYWORD. If it is blank, infer a conservative keyword from the topic title.
- Preserve true product facts. Do not invent product capabilities.
- Keep the article complete Markdown/MDX and keep code fences balanced.
- If a real human decision is required, still return the best safe revision and put concise choices in human_decision_hints.

ATTEMPT: %d of %d

TOPIC TITLE: %s
TOPIC TARGET KEYWORD: %s
TOPIC TARGET PROMPT: %s

CURRENT SEO META:
%s

CURRENT QA STATUS:
status=%s
failure_kind=%s
failure_message=%s
issues=%s

PRODUCT PROFILE:
%s

CURRENT ARTICLE:
%s`, attempt, maxQAFixAttempts, topic.Title, strDeref(topic.TargetKeyword), strDeref(topic.TargetPrompt),
		clip(string(art.SeoMeta), 1200), art.QaStatus, strPtrValue(art.QaFailureKind),
		strPtrValue(art.QaFailureMessage), clip(string(art.QaIssues), 1600),
		clip(string(profileJSON), 3000), clip(art.ContentMd, 7000))

	resp, err := qa.LLM.Complete(ctx, llm.CompletionReq{
		System: "You are a careful SEO/GEO editor. Fix QA feedback before asking a human.",
		Prompt: prompt, JSON: true, MaxTokens: 4096,
	})
	if err != nil {
		return nil, resp, err
	}
	out, err := extractAIFixOutput(resp.Text)
	if err != nil {
		return nil, resp, err
	}
	if len(out.HumanDecisionHints) > 0 {
		out.ResolutionSummary = strings.TrimSpace(out.ResolutionSummary)
	}
	return &out, resp, nil
}

func (qa *QA) markNeedsHumanDecision(ctx context.Context, art db.Article, state ArticleQAState) (db.Article, error) {
	return qa.Q.MarkArticleNeedsHumanDecision(ctx, db.MarkArticleNeedsHumanDecisionParams{
		ID:                   art.ID,
		QaFailureKind:        ptr(state.FailureKind),
		QaFailureMessage:     ptr(state.FailureMessage),
		QaFailureFingerprint: ptr(state.FailureFingerprint),
		QaHumanOptions:       toJSON(state.HumanOptions),
		QaAttemptCount:       art.QaAttemptCount,
		ProjectID:            art.ProjectID,
	})
}

func qaIssues(qa *QAOutput) []string {
	if qa == nil {
		return []string{}
	}
	issues := append([]string{}, qa.Issues...)
	for _, c := range qa.Claims {
		if !c.Mapped {
			issues = append(issues, "unmapped product claim: "+c.Claim)
		}
	}
	return issues
}

func setArticleQAParams(articleID uuid.UUID, status string, out *QAOutput, state ArticleQAState, attemptCount int32) db.SetArticleQAParams {
	if out == nil {
		out = &QAOutput{Claims: []Claim{}, QABlocking: true, Issues: []string{state.FailureMessage}}
	}
	normalizeQAOutput(out)
	return db.SetArticleQAParams{
		ID:                   articleID,
		GeoScore:             pgutil.Numeric(out.GeoScore),
		SeoScore:             pgutil.Numeric(out.SeoScore),
		QaIssues:             toJSON(qaIssues(out)),
		QaBlocking:           state.Status != QAStatusPassed,
		Status:               status,
		QaStatus:             state.Status,
		QaFailureKind:        ptr(state.FailureKind),
		QaFailureMessage:     ptr(state.FailureMessage),
		QaFailureFingerprint: ptr(state.FailureFingerprint),
		QaAttemptCount:       attemptCount,
		QaHumanOptions:       toJSON(state.HumanOptions),
	}
}

func articleStateFromDB(art db.Article) ArticleQAState {
	var options []string
	_ = json.Unmarshal(art.QaHumanOptions, &options)
	return ArticleQAState{
		Status:             art.QaStatus,
		FailureKind:        strPtrValue(art.QaFailureKind),
		FailureMessage:     strPtrValue(art.QaFailureMessage),
		FailureFingerprint: strPtrValue(art.QaFailureFingerprint),
		HumanOptions:       options,
	}
}

func strPtrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
