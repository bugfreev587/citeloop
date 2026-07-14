package geo

import (
	"context"
	"log/slog"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/crawl"
)

func (s Service) EnrichCompetitiveSeedURL(ctx context.Context, rawURL string) (crawl.SeedURLEnrichment, error) {
	crawler := crawl.New(config.Default().Crawl, slog.Default())
	report, err := crawler.EnrichSeedURL(ctx, rawURL)
	if report == nil {
		return crawl.SeedURLEnrichment{}, err
	}
	return *report, err
}
