package agents

import (
	"context"
	"testing"
)

func TestValidateWriterOutputRejectsEmptyDraft(t *testing.T) {
	err := validateWriterOutput(WriterOutput{})
	if err == nil {
		t.Fatal("empty writer output must be invalid")
	}
}

func TestValidateWriterOutputRequiresBasicSEOMeta(t *testing.T) {
	out := WriterOutput{
		ContentMD: "Useful draft body",
		SEOMeta: SEOMeta{
			Title:           "Useful title",
			MetaDescription: "Useful meta description",
			Slug:            "",
			H1:              "Useful H1",
		},
	}
	if err := validateWriterOutput(out); err == nil {
		t.Fatal("writer output without slug must be invalid")
	}
}

func TestValidateWriterOutputRejectsUnclosedCodeFence(t *testing.T) {
	out := WriterOutput{
		ContentMD: "# Useful draft\n\n```js\nconsole.log('unfinished')",
		SEOMeta: SEOMeta{
			Title:           "Useful title",
			MetaDescription: "Useful meta description",
			Slug:            "useful-title",
			H1:              "Useful H1",
		},
	}
	if err := validateWriterOutput(out); err == nil {
		t.Fatal("writer output with unclosed code fence must be invalid")
	}
}

func TestValidateQAOutputRejectsEmptyObject(t *testing.T) {
	err := validateQAOutput(QAOutput{})
	if err == nil {
		t.Fatal("empty QA output must be invalid")
	}
}

func TestExtractWriterOutputSkipsInvalidObjectBeforeValidDraft(t *testing.T) {
	raw := `first:
{}
then:
{"content_md":"Useful draft body","seo_meta":{"title":"Useful title","meta_description":"Useful meta description","slug":"useful-title","h1":"Useful H1"}}`

	out, err := extractWriterOutput(raw)
	if err != nil {
		t.Fatalf("extractWriterOutput: %v", err)
	}
	if out.SEOMeta.Slug != "useful-title" {
		t.Fatalf("slug = %q", out.SEOMeta.Slug)
	}
}

func TestExtractQAOutputSkipsInvalidObjectBeforeValidQA(t *testing.T) {
	raw := `first:
{}
then:
{"claims":[],"qa_blocking":false,"geo_score":0.5,"seo_score":0.6,"issues":[]}`

	out, err := extractQAOutput(raw)
	if err != nil {
		t.Fatalf("extractQAOutput: %v", err)
	}
	if out.GeoScore != 0.5 || out.SeoScore != 0.6 {
		t.Fatalf("scores = %v/%v", out.GeoScore, out.SeoScore)
	}
}

func TestQACompactCheckParsesValidFallback(t *testing.T) {
	provider := &sequenceLLM{resps: []string{
		`{"claims":[],"qa_blocking":false,"geo_score":0.7,"seo_score":0.8,"issues":[]}`,
	}}
	qa := NewQA(Deps{LLM: provider}, nil)

	out, _, err := qa.compactCheck(context.Background(), []byte(`{"features":["hosted OAuth"]}`), "Hosted OAuth keeps tokens secure.", "UniPost supports hosted OAuth.")
	if err != nil {
		t.Fatalf("compactCheck: %v", err)
	}
	if out.QABlocking {
		t.Fatal("compact fallback should preserve model qa_blocking=false")
	}
}

func TestQAOutputLowScoresForceBlocking(t *testing.T) {
	out := &QAOutput{
		Claims:     []Claim{{Claim: "UniPost supports every social platform.", Mapped: true, Evidence: "feature"}},
		QABlocking: false,
		GeoScore:   0.6,
		SeoScore:   0.7,
		Issues:     []string{},
	}

	enforceQAGate(out)

	if !out.QABlocking {
		t.Fatal("low QA scores must force qa_blocking")
	}
	if len(out.Issues) == 0 {
		t.Fatal("low-score gate should add a reviewer-visible issue")
	}
}

func TestBuildArticleQAStateClassifiesParseFailure(t *testing.T) {
	state := BuildArticleQAState(nil, assertAnError("missing claims"))

	if state.Status != QAStatusParseFailed {
		t.Fatalf("status = %q", state.Status)
	}
	if state.FailureKind != "parse_qa" {
		t.Fatalf("failure kind = %q", state.FailureKind)
	}
	if state.FailureFingerprint == "" {
		t.Fatal("parse failures need a stable fingerprint")
	}
	if len(state.HumanOptions) == 0 {
		t.Fatal("parse failures should return human decision options")
	}
}

func TestBuildArticleQAStateClassifiesBlockingWithFixInstructions(t *testing.T) {
	out := &QAOutput{
		Claims:     []Claim{{Claim: "UniPost supports auto-posting to every channel.", Mapped: false}},
		QABlocking: true,
		GeoScore:   0.8,
		SeoScore:   0.9,
		Issues:     []string{"unsupported channel claim"},
	}

	state := BuildArticleQAState(out, nil)

	if state.Status != QAStatusBlocking {
		t.Fatalf("status = %q", state.Status)
	}
	if state.FailureKind != "qa_blocking" {
		t.Fatalf("failure kind = %q", state.FailureKind)
	}
	if len(out.FixInstructions) == 0 {
		t.Fatal("blocking QA should produce AI editor fix instructions")
	}
}

func TestNeedsHumanDecisionStateStopsLoops(t *testing.T) {
	state := NeedsHumanDecisionState(ArticleQAState{
		FailureKind:    "qa_blocking",
		FailureMessage: "same unsupported claim",
	}, 3)

	if state.Status != QAStatusNeedsHumanDecision {
		t.Fatalf("status = %q", state.Status)
	}
	if len(state.HumanOptions) == 0 {
		t.Fatal("human decision state should include choices")
	}
}

type assertAnError string

func (e assertAnError) Error() string { return string(e) }
