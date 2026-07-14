package articleassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

type repository interface {
	Create(context.Context, db.CreateArticleAssetParams) (db.ArticleAsset, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (db.ArticleAsset, error)
	Start(context.Context, uuid.UUID, uuid.UUID) (db.ArticleAsset, error)
	Ready(context.Context, db.MarkArticleAssetReadyParams) (db.ArticleAsset, error)
	Failed(context.Context, uuid.UUID, uuid.UUID, string) (db.ArticleAsset, error)
	Edit(context.Context, uuid.UUID, uuid.UUID, string, string, bool) (db.ArticleAsset, error)
}

type Service struct {
	Repo     repository
	Provider Provider
	Store    Store
	Budget   Budget
}

func NewService(q *db.Queries, provider Provider, store Store, budget Budget) *Service {
	return &Service{Repo: postgresRepository{q: q}, Provider: provider, Store: store, Budget: budget}
}

func (s Service) Plan(ctx context.Context, article db.Article, brief Brief) ([]db.ArticleAsset, error) {
	if s.Repo == nil || article.ID == uuid.Nil || article.ProjectID == uuid.Nil {
		return nil, errors.New("article asset repository and article are required")
	}
	assetType := strings.ToLower(strings.TrimSpace(brief.AssetType))
	if assetType == "faq_answer_block" || assetType == "glossary_definition" {
		return []db.ArticleAsset{}, nil
	}
	roles, err := normalizeRoles(brief.Roles)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return []db.ArticleAsset{}, nil
	}
	brief.AssetType, brief.Roles = assetType, roles
	if brief.Revision <= 0 {
		brief.Revision = 1
	}
	encoded, err := json.Marshal(brief)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(encoded)
	briefHash := hex.EncodeToString(digest[:])
	assets := make([]db.ArticleAsset, 0, len(roles))
	for _, role := range roles {
		asset, err := s.Repo.Create(ctx, db.CreateArticleAssetParams{ProjectID: article.ProjectID, ArticleID: article.ID, Role: role, Brief: encoded, BriefHash: briefHash, Revision: brief.Revision, Prompt: strings.TrimSpace(brief.Prompt), AltText: strings.TrimSpace(brief.AltText), Caption: strings.TrimSpace(brief.Caption)})
		if err != nil {
			return nil, fmt.Errorf("plan article %s asset %s: %w", article.ID, role, err)
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func (s Service) Generate(ctx context.Context, projectID, assetID uuid.UUID) (db.ArticleAsset, error) {
	if s.Repo == nil {
		return db.ArticleAsset{}, errors.New("article asset repository is required")
	}
	asset, err := s.Repo.Get(ctx, projectID, assetID)
	if err != nil {
		return db.ArticleAsset{}, err
	}
	if asset.Status == "ready" {
		return asset, nil
	}
	asset, err = s.Repo.Start(ctx, projectID, assetID)
	if err != nil {
		return db.ArticleAsset{}, err
	}
	fail := func(cause error) (db.ArticleAsset, error) {
		failed, markErr := s.Repo.Failed(ctx, projectID, assetID, cause.Error())
		if markErr != nil {
			return db.ArticleAsset{}, fmt.Errorf("%v; persist image failure: %w", cause, markErr)
		}
		return failed, nil
	}
	var generated GenerateResult
	if asset.Role == RoleBenchmarkChart {
		var brief Brief
		if err := json.Unmarshal(asset.Brief, &brief); err != nil {
			return fail(errors.New("stored benchmark brief is invalid"))
		}
		generated, err = RenderBenchmarkChart(brief.BenchmarkData)
		if err != nil {
			return fail(err)
		}
	} else if s.Budget != nil {
		if err := s.Budget.Allow(ctx, projectID); err != nil {
			return fail(err)
		}
	}
	if s.Store == nil || (asset.Role != RoleBenchmarkChart && s.Provider == nil) {
		return fail(errors.New("image provider or stable asset store is not configured"))
	}
	if asset.Role != RoleBenchmarkChart {
		generated, err = s.Provider.Generate(ctx, GenerateRequest{ProjectID: projectID, ArticleID: asset.ArticleID, AssetID: asset.ID, Role: asset.Role, Prompt: asset.Prompt})
		if err != nil {
			return fail(err)
		}
	}
	if len(generated.Bytes) == 0 || generated.MimeType == "" {
		return fail(errors.New("image provider returned an empty result"))
	}
	storageKey := fmt.Sprintf("article-assets/%s/%s/r%d", asset.ArticleID, asset.ID, asset.Revision)
	stableURL, err := s.Store.Put(ctx, storageKey, generated.Bytes, generated.MimeType)
	if err != nil {
		return fail(err)
	}
	if strings.TrimSpace(stableURL) == "" {
		return fail(errors.New("asset store returned an empty stable URL"))
	}
	return s.Repo.Ready(ctx, db.MarkArticleAssetReadyParams{Provider: generated.Provider, Model: generated.Model, MimeType: generated.MimeType, StorageKey: storageKey, StableUrl: stableURL, Width: generated.Width, Height: generated.Height, ID: asset.ID, ProjectID: projectID})
}

func (s Service) Edit(ctx context.Context, projectID, assetID uuid.UUID, altText, caption string, omitted bool) (db.ArticleAsset, error) {
	if s.Repo == nil {
		return db.ArticleAsset{}, errors.New("article asset repository is required")
	}
	return s.Repo.Edit(ctx, projectID, assetID, strings.TrimSpace(altText), strings.TrimSpace(caption), omitted)
}

func normalizeRoles(input []string) ([]string, error) {
	if len(input) > 3 {
		return nil, errors.New("an article may plan at most one hero and two inline assets")
	}
	allowed := map[string]bool{RoleHero: true, RoleInline1: true, RoleInline2: true, RoleBenchmarkChart: true}
	seen := map[string]bool{}
	roles := make([]string, 0, len(input))
	for _, raw := range input {
		role := strings.ToLower(strings.TrimSpace(raw))
		if !allowed[role] {
			return nil, fmt.Errorf("unsupported article asset role %q", raw)
		}
		if seen[role] {
			return nil, fmt.Errorf("duplicate article asset role %q", role)
		}
		seen[role] = true
		roles = append(roles, role)
	}
	return roles, nil
}

type postgresRepository struct{ q *db.Queries }

func (r postgresRepository) Create(ctx context.Context, p db.CreateArticleAssetParams) (db.ArticleAsset, error) {
	return r.q.CreateArticleAsset(ctx, p)
}
func (r postgresRepository) Get(ctx context.Context, p, id uuid.UUID) (db.ArticleAsset, error) {
	return r.q.GetArticleAssetForProject(ctx, db.GetArticleAssetForProjectParams{ID: id, ProjectID: p})
}
func (r postgresRepository) Start(ctx context.Context, p, id uuid.UUID) (db.ArticleAsset, error) {
	return r.q.StartArticleAssetGeneration(ctx, db.StartArticleAssetGenerationParams{ID: id, ProjectID: p})
}
func (r postgresRepository) Ready(ctx context.Context, p db.MarkArticleAssetReadyParams) (db.ArticleAsset, error) {
	return r.q.MarkArticleAssetReady(ctx, p)
}
func (r postgresRepository) Failed(ctx context.Context, p, id uuid.UUID, message string) (db.ArticleAsset, error) {
	return r.q.MarkArticleAssetFailed(ctx, db.MarkArticleAssetFailedParams{Error: message, ID: id, ProjectID: p})
}
func (r postgresRepository) Edit(ctx context.Context, p, id uuid.UUID, alt, caption string, omitted bool) (db.ArticleAsset, error) {
	return r.q.UpdateArticleAssetEditorial(ctx, db.UpdateArticleAssetEditorialParams{AltText: alt, Caption: caption, Omitted: omitted, ID: id, ProjectID: p})
}
