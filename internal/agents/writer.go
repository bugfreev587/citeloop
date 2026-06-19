package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/platform"
	"github.com/citeloop/citeloop/internal/topicstate"
	"github.com/google/uuid"
)

const canonicalPlaceholder = "{{CANONICAL_URL}}" // backfilled at publish (§5.6)
const maxDraftRepairAttempts = 2
const profileGuardrailInstruction = "Profile fields named banned_claims are negative constraints, not approved facts. Do not repeat or imply banned_claims. Treat profile fields named content_rules as required style and compliance rules to follow."

// Writer generates the canonical article and, for syndication topics, one
// rewritten variant per platform (PRD §5.3). QA runs inline after each draft.
type Writer struct {
	Deps
	Log *slog.Logger
}

func NewWriter(d Deps, log *slog.Logger) *Writer {
	if log == nil {
		log = slog.Default()
	}
	return &Writer{Deps: d, Log: log}
}

// Generate produces articles for a topic and persists them in pending_review
// (or generating→pending_review). canonical is always produced; variants only
// for syndication/both channels.
func (w *Writer) Generate(ctx context.Context, projectID uuid.UUID, topic db.Topic) ([]db.Article, error) {
	profile, err := w.Q.GetActiveProfile(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("no active profile: %w", err)
	}
	var created []db.Article

	// 1) canonical (always)
	canon, err := w.writeArticle(ctx, projectID, topic, profile.Profile, "", true)
	if err != nil {
		return nil, err
	}
	created = append(created, *canon)

	// 2) variants for syndication / both
	if topic.Channel == "syndication" || topic.Channel == "both" {
		for _, p := range platform.SyndicationTargets {
			v, err := w.writeArticle(ctx, projectID, topic, profile.Profile, p.String(), false)
			if err != nil {
				w.Log.Warn("variant generation failed", "platform", p, "err", err)
				continue
			}
			created = append(created, *v)
		}
	}

	nextStatus, err := topicstate.Transition(topicstate.Status(topic.Status), topicstate.EventMarkDrafted)
	if err != nil {
		return created, err
	}
	if _, err := w.Q.UpdateTopicStatus(ctx, db.UpdateTopicStatusParams{ID: topic.ID, Status: string(nextStatus)}); err != nil {
		return created, err
	}
	return created, nil
}

func (w *Writer) writeArticle(ctx context.Context, projectID uuid.UUID, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool) (*db.Article, error) {
	out, resp, err := w.draft(ctx, topic, profileJSON, plat, canonical)
	recordRun(ctx, w.Q, projectID, agentWriter,
		map[string]any{"topic": topic.ID, "platform": plat, "canonical": canonical}, out, resp, err)
	if err != nil {
		return nil, err
	}
	out.SEOMeta = completeSEOMeta(topic, out.SEOMeta, plat, canonical)

	// QA: evidence mapping gate + scoring (§5.3)
	qaAgent := NewQA(w.Deps, w.Log)
	qa, qaResp, qaErr := qaAgent.Check(ctx, projectID, out.ContentMD, profileJSON)
	recordRun(ctx, w.Q, projectID, agentQA, map[string]any{"topic": topic.ID, "platform": plat}, qa, qaResp, qaErr)
	repairAttemptsUsed := 0
	for attempt := 1; attempt <= maxDraftRepairAttempts && draftNeedsRepair(out, qa, qaErr); attempt++ {
		repairAttemptsUsed++
		repaired, repairResp, repairErr := w.repairDraft(ctx, topic, profileJSON, plat, canonical, *out, qa, qaErr)
		recordRun(ctx, w.Q, projectID, agentWriter,
			map[string]any{"topic": topic.ID, "platform": plat, "canonical": canonical, "repair_attempt": attempt, "feedback": repairFeedback(qa, qaErr)},
			repaired, repairResp, repairErr)
		if repairErr != nil {
			qa = &QAOutput{QABlocking: true, Issues: []string{"ai repair failed: " + repairErr.Error()}}
			qaErr = nil
			break
		}
		out = repaired
		out.SEOMeta = completeSEOMeta(topic, out.SEOMeta, plat, canonical)
		qa, qaResp, qaErr = qaAgent.Check(ctx, projectID, out.ContentMD, profileJSON)
		recordRun(ctx, w.Q, projectID, agentQA,
			map[string]any{"topic": topic.ID, "platform": plat, "repair_attempt": attempt}, qa, qaResp, qaErr)
	}
	if qaErr != nil {
		// QA failure is non-fatal to drafting but forces human review after AI repair attempts.
		qa = &QAOutput{QABlocking: true, Issues: []string{"qa step failed after AI repair: " + qaErr.Error()}, CanAutoFix: false}
	}
	repairStatus, requiresHuman := repairOutcome(qa, int32(repairAttemptsUsed), maxDraftRepairAttempts)

	kind := "canonical"
	var platformPtr *string
	if !canonical {
		kind = "syndication_variant"
		p := plat
		platformPtr = &p
	}

	art, err := w.Q.CreateArticle(ctx, db.CreateArticleParams{
		ProjectID:             projectID,
		TopicID:               topic.ID,
		Kind:                  kind,
		Platform:              platformPtr,
		ContentMd:             out.ContentMD,
		SeoMeta:               toJSON(out.SEOMeta),
		GeoScore:              pgutil.Numeric(qa.GeoScore),
		SeoScore:              pgutil.Numeric(qa.SeoScore),
		QaIssues:              toJSON(qaIssues(qa)),
		QaBlocking:            qa.QABlocking,
		Status:                "pending_review",
		RepairAttempts:        int32(repairAttemptsUsed),
		RepairStatus:          repairStatus,
		RequiresHumanDecision: requiresHuman,
		HumanDecisionOptions:  toJSON(humanDecisionOptions(qa)),
		QaFeedback:            toJSON(qa),
	})
	if err != nil {
		return nil, err
	}
	return &art, nil
}

// RepairArticle applies the same AI feedback loop to an existing pending draft.
// It is the reviewer-facing escape hatch: feedback goes to the writer first, QA
// re-runs, and only the remaining unresolved choices stay in the human queue.
func (w *Writer) RepairArticle(ctx context.Context, projectID, articleID uuid.UUID) (db.Article, error) {
	art, err := w.Q.GetArticleForProject(ctx, db.GetArticleForProjectParams{ID: articleID, ProjectID: projectID})
	if err != nil {
		return db.Article{}, err
	}
	qa := qaFromArticle(art)
	if !shouldAttemptArticleRepair(art, maxDraftRepairAttempts) {
		return w.finishRepair(ctx, art, qa, "exhausted", "repair attempt limit reached", true)
	}
	started, err := w.Q.StartArticleRepairForProject(ctx, db.StartArticleRepairForProjectParams{
		ID:             articleID,
		ProjectID:      projectID,
		RepairAttempts: maxDraftRepairAttempts,
	})
	if err != nil {
		return w.finishRepair(ctx, art, qa, "exhausted", "repair attempt limit reached", true)
	}
	art = started
	topic, err := w.Q.GetTopicForProject(ctx, db.GetTopicForProjectParams{ID: art.TopicID, ProjectID: projectID})
	if err != nil {
		return db.Article{}, err
	}
	profile, err := w.Q.GetActiveProfile(ctx, projectID)
	if err != nil {
		return db.Article{}, fmt.Errorf("no active profile: %w", err)
	}
	canonical := art.Kind == "canonical"
	plat := ""
	if art.Platform != nil {
		plat = *art.Platform
	}
	var meta SEOMeta
	_ = json.Unmarshal(art.SeoMeta, &meta)
	out := &WriterOutput{
		ContentMD: art.ContentMd,
		SEOMeta:   completeSEOMeta(topic, meta, plat, canonical),
	}
	if len(qa.Issues) == 0 && art.QaBlocking {
		qa.Issues = []string{"draft is blocked by QA without structured issue details"}
	}
	qaAgent := NewQA(w.Deps, w.Log)
	var qaErr error
	repaired, repairResp, repairErr := w.repairDraft(ctx, topic, profile.Profile, plat, canonical, *out, qa, qaErr)
	recordRun(ctx, w.Q, projectID, agentWriter,
		map[string]any{"article": articleID, "topic": topic.ID, "platform": plat, "canonical": canonical, "repair_attempt": art.RepairAttempts, "feedback": repairFeedback(qa, qaErr)},
		repaired, repairResp, repairErr)
	if repairErr != nil {
		qa = &QAOutput{QABlocking: true, Issues: []string{"ai repair failed: " + repairErr.Error()}, BlockingReason: repairErr.Error(), CanAutoFix: false}
		return w.finishRepair(ctx, art, qa, "failed", repairErr.Error(), true)
	}
	out = repaired
	out.SEOMeta = completeSEOMeta(topic, out.SEOMeta, plat, canonical)
	qaResp, finalQAResp, finalQAErr := qaAgent.Check(ctx, projectID, out.ContentMD, profile.Profile)
	recordRun(ctx, w.Q, projectID, agentQA, map[string]any{"article": articleID, "topic": topic.ID, "platform": plat, "post_repair_check": true}, qaResp, finalQAResp, finalQAErr)
	if finalQAErr != nil {
		qa = &QAOutput{QABlocking: true, Issues: []string{"qa step failed after AI repair: " + finalQAErr.Error()}, BlockingReason: finalQAErr.Error(), CanAutoFix: true}
	} else {
		qa = qaResp
	}

	updated, err := w.Q.UpdateArticleContentForProject(ctx, db.UpdateArticleContentForProjectParams{
		ID:        articleID,
		ProjectID: projectID,
		ContentMd: out.ContentMD,
		SeoMeta:   toJSON(out.SEOMeta),
	})
	if err != nil {
		return db.Article{}, err
	}
	updated, err = w.Q.SetArticleQA(ctx, db.SetArticleQAParams{
		ID:         updated.ID,
		GeoScore:   pgutil.Numeric(qa.GeoScore),
		SeoScore:   pgutil.Numeric(qa.SeoScore),
		QaIssues:   toJSON(qaIssues(qa)),
		QaBlocking: qa.QABlocking,
		Status:     updated.Status,
		QaFeedback: toJSON(qa),
	})
	if err != nil {
		return db.Article{}, err
	}
	repairStatus, requiresHuman := repairOutcome(qa, updated.RepairAttempts, maxDraftRepairAttempts)
	return w.finishRepair(ctx, updated, qa, repairStatus, repairFailureReason(qa, repairStatus), requiresHuman)
}

// RepairArticleWithInstruction applies a specific human-chosen resolution to a
// draft and re-runs QA. Unlike RepairArticle it deliberately bypasses the
// repair-attempt/human-decision gate: the operator is explicitly choosing this
// fix from the Review decision panel, so a cleared QA result un-blocks the draft.
func (w *Writer) RepairArticleWithInstruction(ctx context.Context, projectID, articleID uuid.UUID, instruction string) (db.Article, error) {
	art, err := w.Q.GetArticleForProject(ctx, db.GetArticleForProjectParams{ID: articleID, ProjectID: projectID})
	if err != nil {
		return db.Article{}, err
	}
	topic, err := w.Q.GetTopicForProject(ctx, db.GetTopicForProjectParams{ID: art.TopicID, ProjectID: projectID})
	if err != nil {
		return db.Article{}, err
	}
	profile, err := w.Q.GetActiveProfile(ctx, projectID)
	if err != nil {
		return db.Article{}, fmt.Errorf("no active profile: %w", err)
	}
	canonical := art.Kind == "canonical"
	plat := ""
	if art.Platform != nil {
		plat = *art.Platform
	}
	var meta SEOMeta
	_ = json.Unmarshal(art.SeoMeta, &meta)
	out := &WriterOutput{ContentMD: art.ContentMd, SEOMeta: completeSEOMeta(topic, meta, plat, canonical)}

	qa := qaFromArticle(art)
	if strings.TrimSpace(instruction) != "" {
		qa.FixInstructions = append([]string{instruction}, qa.FixInstructions...)
		qa.BlockingReason = instruction
	}

	repaired, repairResp, repairErr := w.repairDraft(ctx, topic, profile.Profile, plat, canonical, *out, qa, nil)
	recordRun(ctx, w.Q, projectID, agentWriter,
		map[string]any{"article": articleID, "topic": topic.ID, "platform": plat, "apply_fix": instruction}, repaired, repairResp, repairErr)
	if repairErr != nil {
		failed := &QAOutput{QABlocking: true, Issues: []string{"ai fix failed: " + repairErr.Error()}, BlockingReason: repairErr.Error(), CanAutoFix: true}
		return w.finishRepair(ctx, art, failed, "failed", repairErr.Error(), true)
	}
	out = repaired
	out.SEOMeta = completeSEOMeta(topic, out.SEOMeta, plat, canonical)

	qaAgent := NewQA(w.Deps, w.Log)
	checked, finalQAResp, finalQAErr := qaAgent.Check(ctx, projectID, out.ContentMD, profile.Profile)
	recordRun(ctx, w.Q, projectID, agentQA, map[string]any{"article": articleID, "topic": topic.ID, "platform": plat, "post_apply_fix_check": true}, checked, finalQAResp, finalQAErr)
	if finalQAErr != nil {
		checked = &QAOutput{QABlocking: true, Issues: []string{"qa step failed after fix: " + finalQAErr.Error()}, BlockingReason: finalQAErr.Error(), CanAutoFix: true}
	}

	updated, err := w.Q.UpdateArticleContentForProject(ctx, db.UpdateArticleContentForProjectParams{
		ID:        articleID,
		ProjectID: projectID,
		ContentMd: out.ContentMD,
		SeoMeta:   toJSON(out.SEOMeta),
	})
	if err != nil {
		return db.Article{}, err
	}
	updated, err = w.Q.SetArticleQA(ctx, db.SetArticleQAParams{
		ID:         updated.ID,
		GeoScore:   pgutil.Numeric(checked.GeoScore),
		SeoScore:   pgutil.Numeric(checked.SeoScore),
		QaIssues:   toJSON(qaIssues(checked)),
		QaBlocking: checked.QABlocking,
		Status:     updated.Status,
		QaFeedback: toJSON(checked),
	})
	if err != nil {
		return db.Article{}, err
	}
	repairStatus, requiresHuman := repairOutcome(checked, updated.RepairAttempts, maxDraftRepairAttempts)
	return w.finishRepair(ctx, updated, checked, repairStatus, repairFailureReason(checked, repairStatus), requiresHuman)
}

func (w *Writer) draft(ctx context.Context, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool) (*WriterOutput, llm.CompletionResp, error) {
	canonicalInstr := writerCanonicalInstruction(plat, canonical)

	prompt := fmt.Sprintf(`[[WRITER]] Write a content article for this topic.
%s
Only state product facts supported by the profile. Return JSON: {content_md, seo_meta:{title,meta_description,slug,h1,target_keyword,canonical_url?}}.
If TARGET KEYWORD is empty, infer one concise primary search query from TOPIC and include it as seo_meta.target_keyword.
%s

TOPIC: %s
TARGET KEYWORD: %s
TARGET PROMPT: %s
ANGLE: %s / FORMAT: %s

PRODUCT PROFILE:
%s`, canonicalInstr, profileGuardrailInstruction, topic.Title, strDeref(topic.TargetKeyword), strDeref(topic.TargetPrompt),
		strDeref(topic.Angle), strDeref(topic.Format), clip(string(profileJSON), 3000))

	resp, err := w.LLM.Complete(ctx, llm.CompletionReq{
		System: "You are an expert SEO+GEO content writer.",
		Prompt: prompt, Model: llm.ModelClaudeSonnet, JSON: true, MaxTokens: 4096,
	})
	if err != nil {
		return nil, resp, err
	}
	out, err := extractWriterOutput(resp.Text)
	if err != nil {
		fallback, fallbackResp, fallbackErr := w.draftMarkdownFallback(ctx, topic, profileJSON, plat, canonical, canonicalInstr)
		if fallbackErr != nil {
			return nil, fallbackResp, fmt.Errorf("parse writer output: %w; markdown fallback failed: %w", err, fallbackErr)
		}
		return fallback, fallbackResp, nil
	}
	out.SEOMeta = completeSEOMeta(topic, out.SEOMeta, plat, canonical)
	return &out, resp, nil
}

func (w *Writer) repairDraft(ctx context.Context, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool, current WriterOutput, qa *QAOutput, qaErr error) (*WriterOutput, llm.CompletionResp, error) {
	current.SEOMeta = completeSEOMeta(topic, current.SEOMeta, plat, canonical)
	metaJSON, _ := json.MarshalIndent(current.SEOMeta, "", "  ")
	feedbackJSON, _ := json.MarshalIndent(map[string]any{
		"qa_error": qaErrorString(qaErr),
		"qa":       qa,
		"seo":      seoRepairFeedback(current.SEOMeta),
	}, "", "  ")
	prompt := fmt.Sprintf(`[[WRITER_REPAIR]] Revise this draft before human review.
Return JSON: {content_md, seo_meta:{title,meta_description,slug,h1,target_keyword,canonical_url?}}.

Rules:
- Resolve every QA/SEO feedback item that can be resolved from the topic, profile, evidence, or the current draft.
- If QA reports unmapped product claims, remove the claim or rewrite it so it only states facts supported by the profile/evidence.
- If target_keyword is missing, infer a concise primary search query from TOPIC and put it in seo_meta.target_keyword.
- Do not invent product capabilities, statistics, customer names, integrations, pricing, or guarantees.
- %s
- If the evidence is insufficient, make the article more conservative instead of asking the reviewer to edit.

%s

TOPIC: %s
TARGET KEYWORD: %s
TARGET PROMPT: %s
ANGLE: %s / FORMAT: %s

PRODUCT PROFILE:
%s

CURRENT SEO META:
%s

QA AND SEO FEEDBACK:
%s

CURRENT ARTICLE:
%s`, profileGuardrailInstruction, writerCanonicalInstruction(plat, canonical), topic.Title, strDeref(topic.TargetKeyword), strDeref(topic.TargetPrompt),
		strDeref(topic.Angle), strDeref(topic.Format), clip(string(profileJSON), 3000), string(metaJSON), string(feedbackJSON), clip(current.ContentMD, 7000))

	resp, err := w.LLM.Complete(ctx, llm.CompletionReq{
		System: "You are an expert SEO+GEO editor. Fix drafts using only supported facts and return valid JSON.",
		Prompt: prompt, Model: llm.ModelClaudeOpus, JSON: true, MaxTokens: 4096,
	})
	if err != nil {
		return nil, resp, err
	}
	out, err := extractWriterOutput(resp.Text)
	if err != nil {
		return nil, resp, err
	}
	out.SEOMeta = completeSEOMeta(topic, out.SEOMeta, plat, canonical)
	return &out, resp, nil
}

func (w *Writer) draftMarkdownFallback(ctx context.Context, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool, canonicalInstr string) (*WriterOutput, llm.CompletionResp, error) {
	prompt := fmt.Sprintf(`[[WRITER_MARKDOWN]] Write a content article for this topic.
%s
Only state product facts supported by the profile. Return only Markdown/MDX body text. Do not wrap the answer in a code fence. Do not return JSON or front matter.
Write a complete, concise 900-1400 word article. Prefer prose, tables, and short bullets over long code examples. If you include a code block, close it before continuing.
%s

TOPIC: %s
TARGET KEYWORD: %s
TARGET PROMPT: %s
ANGLE: %s / FORMAT: %s

PRODUCT PROFILE:
%s`, canonicalInstr, profileGuardrailInstruction, topic.Title, strDeref(topic.TargetKeyword), strDeref(topic.TargetPrompt),
		strDeref(topic.Angle), strDeref(topic.Format), clip(string(profileJSON), 3000))

	resp, err := w.LLM.Complete(ctx, llm.CompletionReq{
		System:    "You are an expert SEO+GEO content writer. Return only Markdown/MDX body text.",
		Prompt:    prompt,
		Model:     llm.ModelClaudeSonnet,
		MaxTokens: 8192,
	})
	if err != nil {
		return nil, resp, err
	}
	content := cleanMarkdownResponse(resp.Text)
	if content != "" && !strings.HasPrefix(strings.TrimSpace(content), "#") {
		content = "# " + topic.Title + "\n\n" + content
	}
	if !canonical && !platform.SupportsCanonical(platform.Platform(plat)) && !strings.Contains(content, canonicalPlaceholder) {
		content += "\n\nSource: " + canonicalPlaceholder
	}
	out := WriterOutput{
		ContentMD: content,
		SEOMeta:   seoMetaFromTopic(topic, plat, canonical),
	}
	if err := validateWriterOutput(out); err != nil {
		return nil, resp, err
	}
	return &out, resp, nil
}

func writerCanonicalInstruction(plat string, canonical bool) string {
	if canonical {
		return "This is the canonical SEO master copy. Include full on-page SEO (title, meta_description, slug, H1) and GEO strategy (self-contained blocks, stats, citations, Q&A, authoritative tone)."
	}
	if platform.SupportsCanonical(platform.Platform(plat)) {
		return fmt.Sprintf("Rewrite for %s. This platform supports rel=canonical; use %q as the canonical URL placeholder.", plat, canonicalPlaceholder)
	}
	return fmt.Sprintf("Rewrite for %s. This platform does NOT support rel=canonical; place a source link line in the body referencing %q. Do not set canonical_url.", plat, canonicalPlaceholder)
}

func seoMetaFromTopic(topic db.Topic, plat string, canonical bool) SEOMeta {
	meta := SEOMeta{
		Title:           topic.Title,
		MetaDescription: metaDescriptionFromTopic(topic),
		Slug:            slugify(topic.Title),
		H1:              topic.Title,
		TargetKeyword:   targetKeywordFromTopic(topic),
	}
	if !canonical && platform.SupportsCanonical(platform.Platform(plat)) {
		meta.CanonicalURL = canonicalPlaceholder
	}
	return meta
}

func completeSEOMeta(topic db.Topic, meta SEOMeta, plat string, canonical bool) SEOMeta {
	fallback := seoMetaFromTopic(topic, plat, canonical)
	if strings.TrimSpace(meta.Title) == "" {
		meta.Title = fallback.Title
	}
	if strings.TrimSpace(meta.MetaDescription) == "" {
		meta.MetaDescription = fallback.MetaDescription
	}
	if strings.TrimSpace(meta.Slug) == "" {
		meta.Slug = fallback.Slug
	}
	if strings.TrimSpace(meta.H1) == "" {
		meta.H1 = fallback.H1
	}
	if strings.TrimSpace(meta.TargetKeyword) == "" {
		meta.TargetKeyword = fallback.TargetKeyword
	}
	if !canonical && platform.SupportsCanonical(platform.Platform(plat)) && strings.TrimSpace(meta.CanonicalURL) == "" {
		meta.CanonicalURL = canonicalPlaceholder
	}
	return meta
}

func targetKeywordFromTopic(topic db.Topic) string {
	if kw := strings.TrimSpace(strDeref(topic.TargetKeyword)); kw != "" {
		return kw
	}
	if prompt := strings.TrimSpace(strDeref(topic.TargetPrompt)); prompt != "" {
		return prompt
	}
	return strings.TrimSpace(topic.Title)
}

func draftNeedsRepair(out *WriterOutput, qa *QAOutput, qaErr error) bool {
	if out == nil {
		return false
	}
	if strings.TrimSpace(out.SEOMeta.TargetKeyword) == "" {
		return true
	}
	if qaErr != nil {
		return true
	}
	return qa != nil && qa.QABlocking && qa.CanAutoFix
}

func shouldAttemptArticleRepair(art db.Article, maxAttempts int) bool {
	return !art.RequiresHumanDecision && art.RepairAttempts < int32(maxAttempts)
}

func repairOutcome(qa *QAOutput, attempts int32, maxAttempts int) (string, bool) {
	if qa == nil {
		// QA produced nothing — treat as an infrastructure failure the recovery
		// tick can retry, never a human decision.
		return "exhausted", false
	}
	if !qa.QABlocking {
		if attempts == 0 {
			return "idle", false
		}
		return "repaired", false
	}
	if attempts >= int32(maxAttempts) || !qa.CanAutoFix {
		return "exhausted", isGenuineHumanDecision(qa)
	}
	return "repaired", false
}

// isGenuineHumanDecision is true only when QA surfaced a real positioning
// decision a human must make. Unsupported product claims are editor work:
// CiteLoop should remove or rewrite them against confirmed Product Context
// instead of asking the operator to change Context for one draft.
func isGenuineHumanDecision(qa *QAOutput) bool {
	if qa == nil || !qa.QABlocking || qa.CanAutoFix {
		return false
	}
	return len(editorOnlyDecisionOptions(qa.HumanDecisionOptions)) > 0
}

func repairFailureReason(qa *QAOutput, status string) string {
	if status != "exhausted" && status != "failed" && status != "human_decision" {
		return ""
	}
	reason := ""
	if qa != nil {
		reason = strings.TrimSpace(qa.BlockingReason)
		if reason == "" && len(qa.Issues) > 0 {
			reason = strings.Join(qa.Issues, "; ")
		}
	}
	if reason == "" {
		reason = "automatic repair could not satisfy QA"
	}
	return reason
}

func humanDecisionOptions(qa *QAOutput) []HumanDecisionOption {
	if qa != nil {
		if options := editorOnlyDecisionOptions(qa.HumanDecisionOptions); len(options) > 0 {
			return options
		}
	}
	return []HumanDecisionOption{
		{Label: "Reject and regenerate", Description: "Discard this draft and send the topic back to backlog."},
		{Label: "Edit draft", Description: "Adjust the wording directly; saving re-runs QA."},
	}
}

func qaFromArticle(art db.Article) *QAOutput {
	var out QAOutput
	if len(art.QaFeedback) > 0 {
		if err := json.Unmarshal(art.QaFeedback, &out); err == nil {
			out = normalizeQAOutput(out)
			if len(out.Issues) > 0 || len(out.Claims) > 0 || len(out.BlockingIssues) > 0 || out.BlockingReason != "" {
				return &out
			}
		}
	}
	out = QAOutput{
		QABlocking:           art.QaBlocking,
		Issues:               articleIssueStrings(art.QaIssues),
		HumanDecisionOptions: []HumanDecisionOption{},
		BlockingIssues:       []QAFeedbackIssue{},
		FixInstructions:      []string{},
		CanAutoFix:           art.QaBlocking,
	}
	return &out
}

func (w *Writer) finishRepair(ctx context.Context, art db.Article, qa *QAOutput, status, reason string, requiresHuman bool) (db.Article, error) {
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	if requiresHuman && status != "failed" && status != "exhausted" {
		status = "human_decision"
	}
	return w.Q.FinishArticleRepairForProject(ctx, db.FinishArticleRepairForProjectParams{
		ID:                    art.ID,
		ProjectID:             art.ProjectID,
		RepairStatus:          status,
		RepairFailureReason:   reasonPtr,
		RequiresHumanDecision: requiresHuman,
		HumanDecisionOptions:  toJSON(humanDecisionOptions(qa)),
		QaFeedback:            toJSON(qa),
	})
}

func repairFeedback(qa *QAOutput, qaErr error) map[string]any {
	return map[string]any{
		"qa_error": qaErrorString(qaErr),
		"qa":       qa,
	}
}

func seoRepairFeedback(meta SEOMeta) []string {
	var feedback []string
	if strings.TrimSpace(meta.TargetKeyword) == "" {
		feedback = append(feedback, "seo_meta.target_keyword is missing")
	}
	if strings.TrimSpace(meta.Title) == "" {
		feedback = append(feedback, "seo_meta.title is missing")
	}
	if strings.TrimSpace(meta.MetaDescription) == "" {
		feedback = append(feedback, "seo_meta.meta_description is missing")
	}
	if strings.TrimSpace(meta.Slug) == "" {
		feedback = append(feedback, "seo_meta.slug is missing")
	}
	if strings.TrimSpace(meta.H1) == "" {
		feedback = append(feedback, "seo_meta.h1 is missing")
	}
	return feedback
}

func qaErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func articleIssueStrings(raw json.RawMessage) []string {
	var issues []string
	_ = json.Unmarshal(raw, &issues)
	return issues
}

func metaDescriptionFromTopic(topic db.Topic) string {
	for _, candidate := range []string{strDeref(topic.TargetPrompt), strDeref(topic.Angle), strDeref(topic.TargetKeyword)} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return clip(candidate, 155)
		}
	}
	return clip("A practical UniPost guide for "+topic.Title+".", 155)
}

func slugify(s string) string {
	var b strings.Builder
	previousDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			previousDash = false
		case !previousDash:
			b.WriteByte('-')
			previousDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func cleanMarkdownResponse(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			s = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	return strings.TrimSpace(s)
}

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
