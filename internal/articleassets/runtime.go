package articleassets

import (
	"context"
	"errors"
	"net/http"

	"github.com/citeloop/citeloop/internal/admin"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RuntimeOpenAIProvider struct {
	Pool   *pgxpool.Pool
	Secret string
	Client *http.Client
}

func (p RuntimeOpenAIProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	credential, err := admin.LoadImageCredentials(ctx, p.Pool, p.Secret)
	if err != nil {
		return GenerateResult{}, err
	}
	if credential == nil || !credential.Enabled {
		return GenerateResult{}, errors.New("OpenAI image credential is not configured or enabled")
	}
	return (OpenAIProvider{APIKey: credential.APIKey, BaseURL: credential.BaseURL, Model: credential.Model, Client: p.Client}).Generate(ctx, req)
}

type DailyBudget struct {
	Q                            *db.Queries
	MaxCount                     int
	EstimatedCostUSD, MaxCostUSD float64
}

func (b DailyBudget) Allow(ctx context.Context, projectID uuid.UUID) error {
	maxCount := b.MaxCount
	if maxCount <= 0 {
		maxCount = 2
	}
	estimate := b.EstimatedCostUSD
	if estimate <= 0 {
		estimate = .08
	}
	maxCost := b.MaxCostUSD
	if maxCost <= 0 {
		maxCost = .20
	}
	if b.Q != nil {
		project, err := b.Q.GetProject(ctx, projectID)
		if err != nil {
			return err
		}
		cfg, err := config.Parse(project.Config)
		if err != nil {
			return err
		}
		maxCount, maxCost = cfg.ImageDailyCountBudget, cfg.ImageDailyCostBudgetUSD
		if maxCount <= 0 || maxCost <= 0 {
			return errors.New("daily image generation budget is disabled")
		}
	}
	count, err := b.Q.CountArticleAssetsGeneratedToday(ctx, projectID)
	if err != nil {
		return err
	}
	if count >= int64(maxCount) || (float64(count)+1)*estimate > maxCost {
		return errors.New("daily image generation budget exhausted")
	}
	return nil
}
