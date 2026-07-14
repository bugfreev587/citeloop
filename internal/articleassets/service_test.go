package articleassets

import (
	"context"
	"errors"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

func TestArticleAssetPlanEnforcesRolesIdentityReuseAndRevision(t *testing.T) {
	repo := newFakeRepository()
	service := Service{Repo: repo}
	article := db.Article{ID: uuid.New(), ProjectID: uuid.New()}
	brief := Brief{AssetType: "blog_post", Purpose: "Explain the workflow", Prompt: "A clear workflow diagram", AltText: "Workflow from discovery to publication", Roles: []string{RoleHero, RoleInline1, RoleInline2}}
	first, err := service.Plan(context.Background(), article, brief)
	if err != nil || len(first) != 3 {
		t.Fatalf("plan = %#v, %v", first, err)
	}
	second, err := service.Plan(context.Background(), article, brief)
	if err != nil || second[0].ID != first[0].ID {
		t.Fatalf("ready/planned identity was not reused: %#v %v", second, err)
	}
	brief.Revision = 2
	revised, err := service.Plan(context.Background(), article, brief)
	if err != nil || revised[0].ID == first[0].ID || revised[0].Revision != 2 {
		t.Fatalf("revision = %#v %v", revised, err)
	}
	brief.Roles = append(brief.Roles, RoleBenchmarkChart)
	if _, err := service.Plan(context.Background(), article, brief); err == nil {
		t.Fatal("more than three assets accepted")
	}
}

func TestArticleAssetPlanAllowsZeroForFAQAndGlossary(t *testing.T) {
	for _, assetType := range []string{"faq_answer_block", "glossary_definition"} {
		assets, err := (Service{Repo: newFakeRepository()}).Plan(context.Background(), db.Article{ID: uuid.New(), ProjectID: uuid.New()}, Brief{AssetType: assetType})
		if err != nil || len(assets) != 0 {
			t.Fatalf("%s = %#v %v", assetType, assets, err)
		}
	}
}

func TestArticleAssetGenerateStoresStableURLAndReusesReadyAsset(t *testing.T) {
	repo := newFakeRepository()
	provider := &fakeProvider{result: GenerateResult{Bytes: []byte("png"), MimeType: "image/png", Provider: "openai", Model: "gpt-image-1", Width: 1536, Height: 1024}}
	store := &fakeBlobStore{url: "https://cdn.example/assets/stable.png"}
	service := Service{Repo: repo, Provider: provider, Store: store}
	article := db.Article{ID: uuid.New(), ProjectID: uuid.New()}
	planned, _ := service.Plan(context.Background(), article, Brief{AssetType: "blog_post", Purpose: "Explain", Prompt: "diagram", AltText: "diagram", Roles: []string{RoleHero}})
	ready, err := service.Generate(context.Background(), article.ProjectID, planned[0].ID)
	if err != nil || ready.Status != "ready" || ready.StableUrl != store.url {
		t.Fatalf("ready = %#v %v", ready, err)
	}
	readyAgain, err := service.Generate(context.Background(), article.ProjectID, planned[0].ID)
	if err != nil || provider.calls != 1 || store.calls != 1 || readyAgain.StableUrl != ready.StableUrl {
		t.Fatalf("ready asset regenerated: %#v %v", readyAgain, err)
	}
	edited, err := service.Edit(context.Background(), article.ProjectID, planned[0].ID, "better alt", "better caption", true)
	if err != nil || edited.AltText != "better alt" || provider.calls != 1 {
		t.Fatalf("editorial edit regenerated: %#v %v", edited, err)
	}
}

func TestArticleAssetProviderOrBudgetFailureIsNonBlocking(t *testing.T) {
	for name, servicePatch := range map[string]func(*Service){
		"provider": func(s *Service) { s.Provider = &fakeProvider{err: errors.New("provider unavailable")} },
		"budget":   func(s *Service) { s.Provider = &fakeProvider{}; s.Budget = denyingBudget{} },
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository()
			service := Service{Repo: repo, Store: &fakeBlobStore{url: "x"}}
			servicePatch(&service)
			article := db.Article{ID: uuid.New(), ProjectID: uuid.New()}
			planned, _ := service.Plan(context.Background(), article, Brief{AssetType: "blog_post", Prompt: "diagram", Roles: []string{RoleHero}})
			failed, err := service.Generate(context.Background(), article.ProjectID, planned[0].ID)
			if err != nil || failed.Status != "failed" || failed.Error == "" {
				t.Fatalf("failure blocked text workflow: %#v %v", failed, err)
			}
		})
	}
}

func TestBenchmarkChartIsDeterministicAndRequiresCitedData(t *testing.T) {
	points := []BenchmarkPoint{{Label: "Baseline", Value: 42, SourceID: "gsc-window-1"}, {Label: "Current", Value: 64, SourceID: "gsc-window-2"}}
	first, err := RenderBenchmarkChart(points)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderBenchmarkChart(points)
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Bytes) != string(second.Bytes) || first.Provider != "deterministic" || first.MimeType != "image/svg+xml" {
		t.Fatalf("chart not deterministic: %#v", first)
	}
	if _, err := RenderBenchmarkChart([]BenchmarkPoint{{Label: "Uncited", Value: 3}}); err == nil {
		t.Fatal("uncited benchmark data accepted")
	}
}

type fakeRepository struct {
	rows       map[uuid.UUID]db.ArticleAsset
	byIdentity map[string]uuid.UUID
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{rows: map[uuid.UUID]db.ArticleAsset{}, byIdentity: map[string]uuid.UUID{}}
}
func (r *fakeRepository) Create(_ context.Context, p db.CreateArticleAssetParams) (db.ArticleAsset, error) {
	key := p.ArticleID.String() + p.Role + p.BriefHash + string(rune(p.Revision))
	if id := r.byIdentity[key]; id != uuid.Nil {
		return r.rows[id], nil
	}
	row := db.ArticleAsset{ID: uuid.New(), ProjectID: p.ProjectID, ArticleID: p.ArticleID, Role: p.Role, Status: "planned", Brief: p.Brief, BriefHash: p.BriefHash, Revision: p.Revision, Prompt: p.Prompt, AltText: p.AltText, Caption: p.Caption}
	r.rows[row.ID] = row
	r.byIdentity[key] = row.ID
	return row, nil
}
func (r *fakeRepository) Get(_ context.Context, projectID, id uuid.UUID) (db.ArticleAsset, error) {
	row, ok := r.rows[id]
	if !ok || row.ProjectID != projectID {
		return db.ArticleAsset{}, errors.New("missing")
	}
	return row, nil
}
func (r *fakeRepository) Start(_ context.Context, projectID, id uuid.UUID) (db.ArticleAsset, error) {
	row, _ := r.Get(context.Background(), projectID, id)
	row.Status = "generating"
	r.rows[id] = row
	return row, nil
}
func (r *fakeRepository) Ready(_ context.Context, p db.MarkArticleAssetReadyParams) (db.ArticleAsset, error) {
	row, _ := r.Get(context.Background(), p.ProjectID, p.ID)
	row.Status = "ready"
	row.Provider = p.Provider
	row.Model = p.Model
	row.MimeType = p.MimeType
	row.StorageKey = p.StorageKey
	row.StableUrl = p.StableUrl
	row.Width = p.Width
	row.Height = p.Height
	r.rows[row.ID] = row
	return row, nil
}
func (r *fakeRepository) Failed(_ context.Context, projectID, id uuid.UUID, message string) (db.ArticleAsset, error) {
	row, _ := r.Get(context.Background(), projectID, id)
	row.Status = "failed"
	row.Error = message
	r.rows[id] = row
	return row, nil
}
func (r *fakeRepository) Edit(_ context.Context, projectID, id uuid.UUID, alt, caption string, omitted bool) (db.ArticleAsset, error) {
	row, _ := r.Get(context.Background(), projectID, id)
	row.AltText = alt
	row.Caption = caption
	row.Omitted = omitted
	r.rows[id] = row
	return row, nil
}

type fakeProvider struct {
	result GenerateResult
	err    error
	calls  int
}

func (p *fakeProvider) Generate(context.Context, GenerateRequest) (GenerateResult, error) {
	p.calls++
	return p.result, p.err
}

type fakeBlobStore struct {
	url   string
	calls int
}

func (s *fakeBlobStore) Put(context.Context, string, []byte, string) (string, error) {
	s.calls++
	return s.url, nil
}

type denyingBudget struct{}

func (denyingBudget) Allow(context.Context, uuid.UUID) error {
	return errors.New("image budget exhausted")
}
