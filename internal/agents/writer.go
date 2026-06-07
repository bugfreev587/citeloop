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
	"github.com/google/uuid"
)

const canonicalPlaceholder = "{{CANONICAL_URL}}" // backfilled at publish (§5.6)

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

	// mark topic drafted
	_, _ = w.Q.UpdateTopicStatus(ctx, db.UpdateTopicStatusParams{ID: topic.ID, Status: "drafted"})
	return created, nil
}

func (w *Writer) writeArticle(ctx context.Context, projectID uuid.UUID, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool) (*db.Article, error) {
	out, resp, err := w.draft(ctx, topic, profileJSON, plat, canonical)
	recordRun(ctx, w.Q, projectID, agentWriter,
		map[string]any{"topic": topic.ID, "platform": plat, "canonical": canonical}, out, resp, err)
	if err != nil {
		return nil, err
	}

	// QA: evidence mapping gate + scoring (§5.3)
	qaAgent := NewQA(w.Deps, w.Log)
	qa, qaResp, qaErr := qaAgent.Check(ctx, projectID, out.ContentMD, profileJSON)
	recordRun(ctx, w.Q, projectID, agentQA, map[string]any{"topic": topic.ID, "platform": plat}, qa, qaResp, qaErr)
	if qaErr != nil {
		// QA failure is non-fatal to drafting but forces human review.
		qa = &QAOutput{QABlocking: true, Issues: []string{"qa step failed: " + qaErr.Error()}}
	}

	kind := "canonical"
	var platformPtr *string
	if !canonical {
		kind = "syndication_variant"
		p := plat
		platformPtr = &p
	}

	art, err := w.Q.CreateArticle(ctx, db.CreateArticleParams{
		ProjectID:  projectID,
		TopicID:    topic.ID,
		Kind:       kind,
		Platform:   platformPtr,
		ContentMd:  out.ContentMD,
		SeoMeta:    toJSON(out.SEOMeta),
		GeoScore:   pgutil.Numeric(qa.GeoScore),
		SeoScore:   pgutil.Numeric(qa.SeoScore),
		QaIssues:   toJSON(qaIssues(qa)),
		QaBlocking: qa.QABlocking,
		Status:     "pending_review",
	})
	if err != nil {
		return nil, err
	}
	return &art, nil
}

func (w *Writer) draft(ctx context.Context, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool) (*WriterOutput, llm.CompletionResp, error) {
	canonicalInstr := writerCanonicalInstruction(plat, canonical)

	prompt := fmt.Sprintf(`[[WRITER]] Write a content article for this topic.
%s
Only state product facts supported by the profile. Return JSON: {content_md, seo_meta:{title,meta_description,slug,h1,canonical_url?}}.

TOPIC: %s
TARGET KEYWORD: %s
TARGET PROMPT: %s
ANGLE: %s / FORMAT: %s

PRODUCT PROFILE:
%s`, canonicalInstr, topic.Title, strDeref(topic.TargetKeyword), strDeref(topic.TargetPrompt),
		strDeref(topic.Angle), strDeref(topic.Format), clip(string(profileJSON), 3000))

	resp, err := w.LLM.Complete(ctx, llm.CompletionReq{
		System: "You are an expert SEO+GEO content writer.",
		Prompt: prompt, JSON: true, MaxTokens: 4096,
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
	return &out, resp, nil
}

func (w *Writer) draftMarkdownFallback(ctx context.Context, topic db.Topic, profileJSON json.RawMessage, plat string, canonical bool, canonicalInstr string) (*WriterOutput, llm.CompletionResp, error) {
	prompt := fmt.Sprintf(`[[WRITER_MARKDOWN]] Write a content article for this topic.
%s
Only state product facts supported by the profile. Return only Markdown/MDX body text. Do not wrap the answer in a code fence. Do not return JSON or front matter.
Write a complete, concise 900-1400 word article. Prefer prose, tables, and short bullets over long code examples. If you include a code block, close it before continuing.

TOPIC: %s
TARGET KEYWORD: %s
TARGET PROMPT: %s
ANGLE: %s / FORMAT: %s

PRODUCT PROFILE:
%s`, canonicalInstr, topic.Title, strDeref(topic.TargetKeyword), strDeref(topic.TargetPrompt),
		strDeref(topic.Angle), strDeref(topic.Format), clip(string(profileJSON), 3000))

	resp, err := w.LLM.Complete(ctx, llm.CompletionReq{
		System:    "You are an expert SEO+GEO content writer. Return only Markdown/MDX body text.",
		Prompt:    prompt,
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
	}
	if !canonical && platform.SupportsCanonical(platform.Platform(plat)) {
		meta.CanonicalURL = canonicalPlaceholder
	}
	return meta
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
