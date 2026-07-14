package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/citeloop/citeloop/internal/articleassets"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/markdownutil"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/platform"
	"github.com/citeloop/citeloop/internal/platformcontract"
	"github.com/citeloop/citeloop/internal/publisher"
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
	if topic.TargetPlanID.Valid {
		return w.generateFromTargetPlan(ctx, projectID, topic, profile.Profile)
	}

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

func (w *Writer) generateFromTargetPlan(ctx context.Context, projectID uuid.UUID, topic db.Topic, profileJSON json.RawMessage) ([]db.Article, error) {
	plan, err := w.Q.GetContentTargetPlanForProject(ctx, db.GetContentTargetPlanForProjectParams{ID: topic.TargetPlanID.Bytes, ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("load target plan: %w", err)
	}
	items, err := w.Q.ListContentTargetPlanItems(ctx, plan.ID)
	if err != nil {
		return nil, fmt.Errorf("load target plan items: %w", err)
	}
	created := make([]db.Article, 0, len(items))
	for _, item := range items {
		if !item.PlatformContractID.Valid {
			w.Log.Warn("target item has no pinned contract", "platform", item.Platform)
			continue
		}
		contractRow, err := w.Q.GetPlatformContentContractByID(ctx, db.GetPlatformContentContractByIDParams{ID: item.PlatformContractID.Bytes, Version: item.PlatformContractVersion})
		if err != nil {
			w.Log.Warn("pinned platform contract unavailable", "platform", item.Platform, "err", err)
			continue
		}
		var contextRow *db.PlatformTargetContext
		if item.TargetContextID.Valid {
			row, loadErr := w.Q.GetPlatformTargetContextForProject(ctx, db.GetPlatformTargetContextForProjectParams{ID: item.TargetContextID.Bytes, ProjectID: projectID})
			if loadErr != nil {
				w.Log.Warn("pinned target context unavailable", "platform", item.Platform, "err", loadErr)
				continue
			}
			contextRow = &row
		}
		resolved, err := platformcontract.Resolve(platformcontract.ResolveInput{AssetType: plan.AssetType, Item: item, Contract: contractRow, Context: contextRow})
		if err != nil {
			w.Log.Warn("target contract resolution failed", "platform", item.Platform, "err", err)
			continue
		}
		article, err := w.writeArticleForContract(ctx, projectID, topic, profileJSON, item, resolved)
		if err != nil {
			w.Log.Warn("native target generation failed", "platform", item.Platform, "err", err)
			continue
		}
		created = append(created, *article)
	}
	if len(created) == 0 {
		return nil, fmt.Errorf("target plan produced no artifacts")
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
	return w.writeArticleResolved(ctx, projectID, topic, profileJSON, plat, canonical, nil, nil)
}

func (w *Writer) writeArticleForContract(ctx context.Context, projectID uuid.UUID, topic db.Topic, profileJSON json.RawMessage, item db.ContentTargetPlanItem, contract platformcontract.ResolvedContract) (*db.Article, error) {
	return w.writeArticleResolved(ctx, projectID, topic, profileJSON, item.Platform, item.IsCanonical, &item, &contract)
}

func (w *Writer) writeArticleResolved(ctx context.Context, projectID uuid.UUID, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool, item *db.ContentTargetPlanItem, contract *platformcontract.ResolvedContract) (*db.Article, error) {
	out, resp, _, err := w.draftTrackedResolved(ctx, projectID, topic, profileJSON, plat, canonical, contract)
	recordRun(ctx, w.Q, projectID, agentWriter,
		map[string]any{"topic": topic.ID, "platform": plat, "canonical": canonical}, out, resp, err)
	if err != nil {
		return nil, err
	}
	out.SEOMeta = completeSEOMeta(topic, out.SEOMeta, plat, canonical)
	if !canonical {
		w.reuseCanonicalImages(ctx, topic, plat, out)
	}

	// QA: evidence mapping gate + scoring (§5.3)
	qaAgent := NewQA(w.Deps, w.Log)
	qa := &QAOutput{}
	var qaResp llm.CompletionResp
	var qaErr error
	if contract == nil || !contract.Rules.LinkOnly {
		qa, qaResp, qaErr = qaAgent.CheckForObject(ctx, projectID, "topic", topic.ID, out.ContentMD, profileJSON)
		recordRun(ctx, w.Q, projectID, agentQA, map[string]any{"topic": topic.ID, "platform": plat}, qa, qaResp, qaErr)
	}
	if qa == nil {
		qa = &QAOutput{}
	}
	validation := platformcontract.ValidationReport{Passed: true, Failures: []platformcontract.Failure{}, Warnings: []platformcontract.Failure{}}
	if contract != nil {
		normalizeNativeArtifact(out, *contract, item)
		validation = platformcontract.Validate(*contract, platformcontract.Artifact{ContentMD: out.ContentMD, Metadata: out.PlatformMetadata})
		applyContractValidation(qa, validation)
	}
	repairAttemptsUsed := 0
	for attempt := 1; attempt <= maxDraftRepairAttempts && draftNeedsRepair(out, qa, qaErr); attempt++ {
		repairAttemptsUsed++
		repaired, repairResp, repairErr := w.repairDraft(ctx, projectID, topic, profileJSON, plat, canonical, *out, qa, qaErr)
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
		if contract == nil || !contract.Rules.LinkOnly {
			qa, qaResp, qaErr = qaAgent.CheckForObject(ctx, projectID, "topic", topic.ID, out.ContentMD, profileJSON)
			recordRun(ctx, w.Q, projectID, agentQA,
				map[string]any{"topic": topic.ID, "platform": plat, "repair_attempt": attempt}, qa, qaResp, qaErr)
		} else {
			qa = &QAOutput{}
		}
		if qa == nil {
			qa = &QAOutput{}
		}
		if contract != nil {
			normalizeNativeArtifact(out, *contract, item)
			validation = platformcontract.Validate(*contract, platformcontract.Artifact{ContentMD: out.ContentMD, Metadata: out.PlatformMetadata})
			applyContractValidation(qa, validation)
		}
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

	createParams := db.CreateArticleParams{
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
		OutputType:            "long_form_article",
		PlatformMetadata:      toJSON(out.PlatformMetadata),
		ContractValidation:    toJSON(validation),
	}
	if item != nil && contract != nil {
		createParams.PlatformContractID = item.PlatformContractID
		version := item.PlatformContractVersion
		createParams.PlatformContractVersion = &version
		createParams.TargetContextID = item.TargetContextID
		createParams.OutputType = contract.OutputType
	}
	art, err := w.Q.CreateArticle(ctx, createParams)
	if err != nil {
		return nil, err
	}
	if canonical {
		w.planArticleImages(ctx, topic, art)
	}
	return &art, nil
}

func (w *Writer) reuseCanonicalImages(ctx context.Context, topic db.Topic, platformName string, out *WriterOutput) {
	if w.Q == nil || out == nil {
		return
	}
	articles, err := w.Q.ListArticlesByTopic(ctx, topic.ID)
	if err != nil {
		return
	}
	for _, article := range articles {
		if article.Kind != "canonical" {
			continue
		}
		assets, loadErr := w.Q.ListArticleAssetsForArticle(ctx, db.ListArticleAssetsForArticleParams{ProjectID: topic.ProjectID, ArticleID: article.ID})
		if loadErr != nil {
			return
		}
		allowed := assets[:0]
		for _, asset := range assets {
			if platformcontract.SupportsImageRole(platformName, asset.Role) {
				allowed = append(allowed, asset)
			}
		}
		out.ContentMD = publisher.RenderArticleAssets(out.ContentMD, allowed)
		return
	}
}

func (w *Writer) planArticleImages(ctx context.Context, topic db.Topic, article db.Article) {
	if w.ArticleAssets == nil {
		return
	}
	assetType := "blog_post"
	if topic.AssetType != nil && strings.TrimSpace(*topic.AssetType) != "" {
		assetType = strings.TrimSpace(*topic.AssetType)
	}
	roles := []string{articleassets.RoleHero, articleassets.RoleInline1}
	if assetType == "faq_answer_block" || assetType == "glossary_definition" || assetType == "benchmark_report" {
		roles = nil
	}
	promptParts := []string{topic.Title}
	if topic.Angle != nil {
		promptParts = append(promptParts, *topic.Angle)
	}
	if topic.TargetPrompt != nil {
		promptParts = append(promptParts, *topic.TargetPrompt)
	}
	assets, err := w.ArticleAssets.Plan(ctx, article, articleassets.Brief{AssetType: assetType, Purpose: "Clarify the article's central decision or workflow", Prompt: strings.Join(promptParts, ". "), AltText: "Explanatory visual for " + topic.Title, Roles: roles})
	if err != nil {
		w.Log.Warn("article image planning failed without blocking draft", "article", article.ID, "err", err)
		return
	}
	for _, asset := range assets {
		generated, generateErr := w.ArticleAssets.Generate(ctx, article.ProjectID, asset.ID)
		if generateErr != nil {
			w.Log.Warn("article image generation failed without blocking draft", "article", article.ID, "asset", asset.ID, "err", generateErr)
			continue
		}
		if generated.Status == "failed" {
			w.Log.Warn("article image unavailable; draft remains reviewable", "article", article.ID, "asset", asset.ID, "reason", generated.Error)
		}
	}
}

// RepairArticle applies the same AI feedback loop to an existing pending draft.
// It is the reviewer-facing escape hatch: feedback goes to the writer first, QA
// instructions that the writer applies are treated as the terminal QA approval,
// and only unresolved editor failures stay in the human queue.
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
		ID:                         articleID,
		ProjectID:                  projectID,
		RepairAttempts:             maxDraftRepairAttempts,
		AllowExhaustedEditorRepair: isEditorRepairableHumanDecision(art),
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
	_ = json.Unmarshal(art.PlatformMetadata, &out.PlatformMetadata)
	if len(qa.Issues) == 0 && art.QaBlocking {
		qa.Issues = []string{"draft is blocked by QA without structured issue details"}
	}
	repaired, repairResp, repairErr := w.repairDraft(ctx, projectID, topic, profile.Profile, plat, canonical, *out, qa, nil)
	recordRun(ctx, w.Q, projectID, agentWriter,
		map[string]any{"article": articleID, "topic": topic.ID, "platform": plat, "canonical": canonical, "repair_attempt": art.RepairAttempts, "feedback": repairFeedback(qa, nil)},
		repaired, repairResp, repairErr)
	if repairErr != nil {
		qa = &QAOutput{QABlocking: true, Issues: []string{"ai repair failed: " + repairErr.Error()}, BlockingReason: repairErr.Error(), CanAutoFix: false}
		return w.finishRepair(ctx, art, qa, "failed", repairErr.Error(), true)
	}
	out = repaired
	out.SEOMeta = completeSEOMeta(topic, out.SEOMeta, plat, canonical)
	qa = approvedQAAfterAppliedFix(qa, "automatic AI repair")

	updated, err := w.Q.UpdateArticleContentAndPlatformMetadataForProject(ctx, db.UpdateArticleContentAndPlatformMetadataForProjectParams{
		ID: articleID, ProjectID: projectID, ContentMd: out.ContentMD, SeoMeta: toJSON(out.SEOMeta), PlatformMetadata: toJSON(out.PlatformMetadata),
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
	finished, err := w.finishRepair(ctx, updated, qa, repairStatus, repairFailureReason(qa, repairStatus), requiresHuman)
	if err != nil {
		return db.Article{}, err
	}
	validated, _, err := platformcontract.RevalidateArticle(ctx, w.Q, finished, time.Now().UTC())
	return validated, err
}

// RepairArticleWithInstruction applies a specific QA-proposed resolution to a
// draft. It deliberately bypasses the repair-attempt/human-decision gate: the
// operator is explicitly choosing this fix from the Review decision panel, so a
// successful AI edit becomes the terminal QA approval for that draft.
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
	_ = json.Unmarshal(art.PlatformMetadata, &out.PlatformMetadata)

	qa := qaFromArticle(art)
	if strings.TrimSpace(instruction) != "" {
		qa.FixInstructions = append([]string{instruction}, qa.FixInstructions...)
		qa.BlockingReason = instruction
	}

	repaired, repairResp, repairErr := w.repairDraft(ctx, projectID, topic, profile.Profile, plat, canonical, *out, qa, nil)
	recordRun(ctx, w.Q, projectID, agentWriter,
		map[string]any{"article": articleID, "topic": topic.ID, "platform": plat, "apply_fix": instruction}, repaired, repairResp, repairErr)
	if repairErr != nil {
		failed := &QAOutput{QABlocking: true, Issues: []string{"ai fix failed: " + repairErr.Error()}, BlockingReason: repairErr.Error(), CanAutoFix: true}
		return w.finishRepair(ctx, art, failed, "failed", repairErr.Error(), true)
	}
	out = repaired
	out.SEOMeta = completeSEOMeta(topic, out.SEOMeta, plat, canonical)
	checked := approvedQAAfterAppliedFix(qa, instruction)

	updated, err := w.Q.UpdateArticleContentAndPlatformMetadataForProject(ctx, db.UpdateArticleContentAndPlatformMetadataForProjectParams{
		ID: articleID, ProjectID: projectID, ContentMd: out.ContentMD, SeoMeta: toJSON(out.SEOMeta), PlatformMetadata: toJSON(out.PlatformMetadata),
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
	finished, err := w.finishRepair(ctx, updated, checked, repairStatus, repairFailureReason(checked, repairStatus), requiresHuman)
	if err != nil {
		return db.Article{}, err
	}
	validated, _, err := platformcontract.RevalidateArticle(ctx, w.Q, finished, time.Now().UTC())
	return validated, err
}

func approvedQAAfterAppliedFix(previous *QAOutput, instruction string) *QAOutput {
	out := &QAOutput{
		Claims:               []Claim{},
		QABlocking:           false,
		Issues:               []string{},
		BlockingIssues:       []QAFeedbackIssue{},
		FixInstructions:      []string{},
		HumanDecisionOptions: []HumanDecisionOption{},
		CanAutoFix:           false,
	}
	if previous != nil {
		out.GeoScore = previous.GeoScore
		out.SeoScore = previous.SeoScore
	}
	return out
}

func (w *Writer) draft(ctx context.Context, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool) (*WriterOutput, llm.CompletionResp, error) {
	out, resp, _, err := w.draftTracked(ctx, uuid.Nil, topic, profileJSON, plat, canonical)
	return out, resp, err
}

func (w *Writer) draftTracked(ctx context.Context, projectID uuid.UUID, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool) (*WriterOutput, llm.CompletionResp, uuid.UUID, error) {
	return w.draftTrackedResolved(ctx, projectID, topic, profileJSON, plat, canonical, nil)
}

func (w *Writer) draftTrackedResolved(ctx context.Context, projectID uuid.UUID, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool, contract *platformcontract.ResolvedContract) (*WriterOutput, llm.CompletionResp, uuid.UUID, error) {
	canonicalInstr := writerCanonicalInstruction(plat, canonical)
	assetContract := writerAssetContract(topic)
	platformInstruction := ""
	if contract != nil {
		platformInstruction = fmt.Sprintf("\nNATIVE PLATFORM CONTRACT (%s, %s, %s):\n%s\nRequired platform_metadata fields: %s\n", contract.Platform, contract.Version, contract.OutputType, contract.Prompt, strings.Join(contract.RequiredFields, ", "))
	}

	prompt := fmt.Sprintf(`[[WRITER]] Write a content article for this topic.
%s
Only state product facts supported by the profile. Return JSON: {content_md, seo_meta:{title,meta_description,slug,h1,target_keyword,canonical_url?}, platform_metadata:{...}}.
If TARGET KEYWORD is empty, infer one concise primary search query from TOPIC and include it as seo_meta.target_keyword.
%s
%s
%s

TOPIC: %s
TARGET KEYWORD: %s
TARGET PROMPT: %s
ANGLE: %s / FORMAT: %s

PRODUCT PROFILE:
%s`, canonicalInstr, profileGuardrailInstruction, assetContract, platformInstruction, topic.Title, strDeref(topic.TargetKeyword), strDeref(topic.TargetPrompt),
		strDeref(topic.Angle), strDeref(topic.Format), clip(string(profileJSON), 3000))

	req := llm.CompletionReq{
		System: "You are an expert SEO+GEO content writer.",
		Prompt: prompt, Purpose: llm.PurposeWriter, JSON: true, MaxTokens: 4096,
	}
	resp, callID, err := completeTracked(ctx, w.AICalls, w.LLM, projectID, "content_generation", "topic", topic.ID, "content-writer-v2", uuid.Nil, uuid.Nil, req)
	if err != nil {
		return nil, resp, callID, err
	}
	out, err := extractWriterOutput(resp.Text)
	if err != nil {
		if ledgerErr := failTrackedOutput(ctx, w.AICalls, projectID, callID, "invalid_response"); ledgerErr != nil {
			return nil, resp, callID, errors.Join(err, ledgerErr)
		}
		fallback, fallbackResp, fallbackCallID, fallbackErr := w.draftMarkdownFallbackTracked(ctx, projectID, topic, profileJSON, plat, canonical, canonicalInstr, callID)
		if fallbackErr != nil {
			return nil, fallbackResp, fallbackCallID, fmt.Errorf("parse writer output: %w; markdown fallback failed: %w", err, fallbackErr)
		}
		return fallback, fallbackResp, fallbackCallID, nil
	}
	out.SEOMeta = completeSEOMeta(topic, out.SEOMeta, plat, canonical)
	return &out, resp, callID, nil
}

func (w *Writer) repairDraft(ctx context.Context, projectID uuid.UUID, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool, current WriterOutput, qa *QAOutput, qaErr error) (*WriterOutput, llm.CompletionResp, error) {
	current.SEOMeta = completeSEOMeta(topic, current.SEOMeta, plat, canonical)
	metaJSON, _ := json.MarshalIndent(current.SEOMeta, "", "  ")
	feedbackJSON, _ := json.MarshalIndent(map[string]any{
		"qa_error": qaErrorString(qaErr),
		"qa":       qa,
		"seo":      seoRepairFeedback(current.SEOMeta),
	}, "", "  ")
	prompt := fmt.Sprintf(`[[WRITER_REPAIR]] Revise this draft before human review.
	Return JSON: {content_md, seo_meta:{title,meta_description,slug,h1,target_keyword,canonical_url?}, platform_metadata:{...}}.

Rules:
- Resolve every QA/SEO feedback item that can be resolved from the topic, profile, evidence, or the current draft.
- If QA reports unmapped product claims, remove the claim or rewrite it so it only states facts supported by the profile/evidence.
- If target_keyword is missing, infer a concise primary search query from TOPIC and put it in seo_meta.target_keyword.
- Do not invent product capabilities, statistics, customer names, integrations, pricing, or guarantees.
- %s
- If the evidence is insufficient, make the article more conservative instead of asking the reviewer to edit.

%s
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
%s`, profileGuardrailInstruction, writerCanonicalInstruction(plat, canonical), writerAssetContract(topic), topic.Title, strDeref(topic.TargetKeyword), strDeref(topic.TargetPrompt),
		strDeref(topic.Angle), strDeref(topic.Format), clip(string(profileJSON), 3000), string(metaJSON), string(feedbackJSON), clip(current.ContentMD, 7000))

	resp, callID, err := completeTracked(ctx, w.AICalls, w.LLM, projectID, "content_generation", "topic", topic.ID, "content-repair-v2", uuid.Nil, uuid.Nil, llm.CompletionReq{
		System: "You are an expert SEO+GEO editor. Fix drafts using only supported facts and return valid JSON.",
		Prompt: prompt, Purpose: llm.PurposeWriter, JSON: true, MaxTokens: 4096,
	})
	if err != nil {
		return nil, resp, err
	}
	out, err := extractWriterOutput(resp.Text)
	if err != nil {
		if ledgerErr := failTrackedOutput(ctx, w.AICalls, projectID, callID, "invalid_response"); ledgerErr != nil {
			return nil, resp, errors.Join(err, ledgerErr)
		}
		return nil, resp, err
	}
	out.SEOMeta = completeSEOMeta(topic, out.SEOMeta, plat, canonical)
	return &out, resp, nil
}

func (w *Writer) draftMarkdownFallback(ctx context.Context, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool, canonicalInstr string) (*WriterOutput, llm.CompletionResp, error) {
	out, resp, _, err := w.draftMarkdownFallbackTracked(ctx, uuid.Nil, topic, profileJSON, plat, canonical, canonicalInstr, uuid.Nil)
	return out, resp, err
}

func (w *Writer) draftMarkdownFallbackTracked(ctx context.Context, projectID uuid.UUID, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool, canonicalInstr string, parentCallID uuid.UUID) (*WriterOutput, llm.CompletionResp, uuid.UUID, error) {
	prompt := fmt.Sprintf(`[[WRITER_MARKDOWN]] Write a content article for this topic.
%s
Only state product facts supported by the profile. Return only Markdown/MDX body text. Do not wrap the answer in a code fence. Do not return JSON or front matter.
Write a complete, concise 900-1400 word article. Prefer prose, tables, and short bullets over long code examples. If you include a code block, close it before continuing.
%s
%s

TOPIC: %s
TARGET KEYWORD: %s
TARGET PROMPT: %s
ANGLE: %s / FORMAT: %s

PRODUCT PROFILE:
%s`, canonicalInstr, profileGuardrailInstruction, writerAssetContract(topic), topic.Title, strDeref(topic.TargetKeyword), strDeref(topic.TargetPrompt),
		strDeref(topic.Angle), strDeref(topic.Format), clip(string(profileJSON), 3000))

	resp, callID, err := completeTracked(ctx, w.AICalls, w.LLM, projectID, "content_generation", "topic", topic.ID, "content-writer-markdown-v1", uuid.Nil, parentCallID, llm.CompletionReq{
		System:    "You are an expert SEO+GEO content writer. Return only Markdown/MDX body text.",
		Prompt:    prompt,
		Purpose:   llm.PurposeWriter,
		MaxTokens: 8192,
	})
	if err != nil {
		return nil, resp, callID, err
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
		if ledgerErr := failTrackedOutput(ctx, w.AICalls, projectID, callID, "invalid_response"); ledgerErr != nil {
			return nil, resp, callID, errors.Join(err, ledgerErr)
		}
		return nil, resp, callID, err
	}
	return &out, resp, callID, nil
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

func normalizeNativeArtifact(out *WriterOutput, contract platformcontract.ResolvedContract, item *db.ContentTargetPlanItem) {
	if out.PlatformMetadata == nil {
		out.PlatformMetadata = map[string]any{}
	}
	setMetadataDefault(out.PlatformMetadata, "title", out.SEOMeta.Title)
	switch contract.Platform {
	case "blog":
		setMetadataDefault(out.PlatformMetadata, "slug", out.SEOMeta.Slug)
	case "dev_to", "medium":
		setMetadataDefault(out.PlatformMetadata, "canonical_url", canonicalPlaceholder)
	case "hashnode":
		setMetadataDefault(out.PlatformMetadata, "canonical_url", canonicalPlaceholder)
		if item != nil {
			setMetadataDefault(out.PlatformMetadata, "publication", item.TargetKey)
		}
	case "linkedin":
		setMetadataDefault(out.PlatformMetadata, "description", out.SEOMeta.MetaDescription)
	case "reddit":
		if item != nil {
			setMetadataDefault(out.PlatformMetadata, "subreddit", item.TargetKey)
			setMetadataDefault(out.PlatformMetadata, "post_type", item.OutputType)
		}
		if contract.TargetContext != nil && contract.TargetContext.RequiredFlair != "" {
			setMetadataDefault(out.PlatformMetadata, "flair", contract.TargetContext.RequiredFlair)
		}
	case "hacker_news":
		setMetadataDefault(out.PlatformMetadata, "url", canonicalPlaceholder)
	}
}

func setMetadataDefault(metadata map[string]any, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if current, ok := metadata[key].(string); !ok || strings.TrimSpace(current) == "" {
		metadata[key] = value
	}
}

func applyContractValidation(qa *QAOutput, report platformcontract.ValidationReport) {
	if qa == nil || report.Passed {
		return
	}
	qa.QABlocking = true
	qa.CanAutoFix = true
	for _, failure := range report.Failures {
		qa.Issues = append(qa.Issues, "platform contract: "+failure.Message)
		qa.FixInstructions = append(qa.FixInstructions, failure.Message)
	}
}

type geoAssetTopicMetadata struct {
	AssetBriefID       string   `json:"asset_brief_id"`
	Links              []string `json:"links"`
	SourceEvidence     []string `json:"source_evidence"`
	RecommendedOutline []string `json:"recommended_outline"`
}

func writerAssetContract(topic db.Topic) string {
	assetType := geoAssetType(topic)
	if assetType == "" {
		return ""
	}
	metadata := geoAssetMetadata(topic)
	var b strings.Builder
	fmt.Fprintf(&b, "\nGEO ASSET BRIEF CONTRACT:\nASSET TYPE: %s\n", assetType)
	if len(metadata.SourceEvidence) > 0 {
		b.WriteString("\nSOURCE EVIDENCE:\n")
		for _, item := range metadata.SourceEvidence {
			if text := strings.TrimSpace(item); text != "" {
				fmt.Fprintf(&b, "- %s\n", text)
			}
		}
	}
	if len(metadata.RecommendedOutline) > 0 {
		b.WriteString("\nRECOMMENDED OUTLINE:\n")
		for _, item := range metadata.RecommendedOutline {
			if text := strings.TrimSpace(item); text != "" {
				fmt.Fprintf(&b, "- %s\n", text)
			}
		}
	}
	b.WriteString("\nSTRUCTURE CONTRACT:\n")
	b.WriteString(assetStructureContract(assetType))
	b.WriteString("\nDo not flatten this into a generic blog article. Keep the asset type visible in the structure and make every product claim supportable from the profile or source evidence.\n")
	return b.String()
}

func geoAssetType(topic db.Topic) string {
	if topic.AssetType != nil {
		if assetType, ok := platformcontract.CanonicalAssetType(*topic.AssetType); ok {
			return assetType
		}
	}
	assetType := strings.TrimSpace(strDeref(topic.Angle))
	canonical, ok := platformcontract.CanonicalAssetType(assetType)
	if !ok || !knownGEOAssetType(canonical) {
		return ""
	}
	format := strings.TrimSpace(strDeref(topic.Format))
	if format == "geo_asset_brief" || format == assetType {
		return canonical
	}
	return ""
}

func knownGEOAssetType(value string) bool {
	switch strings.TrimSpace(value) {
	case "comparison_page",
		"blog_post",
		"alternative_page",
		"use_case_page",
		"template_or_checklist",
		"benchmark_report",
		"glossary_definition",
		"integration_page",
		"source_backed_evidence_page",
		"faq_answer_block":
		return true
	default:
		return false
	}
}

func geoAssetMetadata(topic db.Topic) geoAssetTopicMetadata {
	raw := strings.TrimSpace(string(topic.InternalLinks))
	if raw == "" {
		return geoAssetTopicMetadata{}
	}
	if strings.HasPrefix(raw, "[") {
		var links []string
		_ = json.Unmarshal(topic.InternalLinks, &links)
		return geoAssetTopicMetadata{Links: links}
	}
	var metadata geoAssetTopicMetadata
	_ = json.Unmarshal(topic.InternalLinks, &metadata)
	return metadata
}

func assetStructureContract(assetType string) string {
	switch assetType {
	case "comparison_page":
		return "- Use decision criteria, who each option is for, supported differentiators, and limitations.\n- Include a balanced comparison table only when each cell can be supported."
	case "alternative_page":
		return "- Cover migration reasons, alternative evaluation, primary use cases, and when not to switch.\n- Keep competitor claims conservative and source-backed."
	case "glossary_definition":
		return "- Start with a short definition, then examples, related terms, and source-backed product context.\n- Keep the answer extractable for AI answer engines."
	case "template_or_checklist":
		return "- Provide actionable steps, a download or use section, and FAQ.\n- Make checklist items specific enough to execute."
	case "benchmark_report":
		return "- Include methodology, data caveats, findings, and chart or table placeholders.\n- Label any estimates or inferred observations clearly."
	case "integration_page":
		return "- Include setup steps, API or workflow details, troubleshooting, and related links.\n- Separate verified capabilities from future or unsupported integrations."
	case "source_backed_evidence_page":
		return "- Create a citation-ready evidence block with extractable claims, supporting context, and limitations.\n- Prefer concise sections answer engines can quote safely."
	case "faq_answer_block":
		return "- Write concise question-answer blocks with direct answers first and supporting evidence second.\n- Avoid broad article framing."
	case "use_case_page":
		return "- Anchor the page in the audience use case, success criteria, workflow, proof points, and limitations.\n- Include where the product does and does not fit."
	default:
		return "- Use the recommended outline and source evidence; keep the draft conservative and citation-ready."
	}
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
	return applyGEOAssetSEOMeta(topic, meta)
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
	return applyGEOAssetSEOMeta(topic, meta)
}

func applyGEOAssetSEOMeta(topic db.Topic, meta SEOMeta) SEOMeta {
	assetType := geoAssetType(topic)
	if assetType == "" {
		return meta
	}
	if strings.TrimSpace(meta.AssetType) == "" {
		meta.AssetType = assetType
	}
	if len(meta.SourceEvidence) == 0 {
		meta.SourceEvidence = geoAssetMetadata(topic).SourceEvidence
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
	editorRepairableHuman := isEditorRepairableHumanDecision(art)
	if art.RepairAttempts >= int32(maxAttempts) && !editorRepairableHuman {
		return false
	}
	if art.RequiresHumanDecision && !editorRepairableHuman {
		return false
	}
	return true
}

func isEditorRepairableHumanDecision(art db.Article) bool {
	return art.RequiresHumanDecision && QAFeedbackCanAutoFix(art.QaFeedback, art.QaBlocking)
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
		if qa.QABlocking && qa.CanAutoFix && len(qa.FixInstructions) > 0 {
			return fixInstructionOptions(qa.FixInstructions)
		}
		if options := editorOnlyDecisionOptions(qa.HumanDecisionOptions); len(options) > 0 {
			return options
		}
	}
	return []HumanDecisionOption{
		{Label: "Reject and regenerate", Description: "Discard this draft and send the topic back to backlog."},
		{Label: "Edit draft", Description: "Adjust the wording directly; saving re-runs QA."},
	}
}

func fixInstructionOptions(instructions []string) []HumanDecisionOption {
	out := make([]HumanDecisionOption, 0, len(instructions))
	for _, instruction := range instructions {
		instruction = strings.TrimSpace(instruction)
		if instruction == "" {
			continue
		}
		out = append(out, HumanDecisionOption{Label: "Apply QA fix", Description: instruction})
	}
	return out
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
	return strings.TrimSpace(markdownutil.NormalizeGeneratedEscapes(s))
}

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
