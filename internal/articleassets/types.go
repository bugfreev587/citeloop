package articleassets

import (
	"context"

	"github.com/google/uuid"
)

const (
	RoleHero           = "hero"
	RoleInline1        = "inline_1"
	RoleInline2        = "inline_2"
	RoleBenchmarkChart = "benchmark_chart"
)

type Brief struct {
	AssetType     string           `json:"asset_type"`
	Purpose       string           `json:"purpose"`
	Prompt        string           `json:"prompt"`
	AltText       string           `json:"alt_text"`
	Caption       string           `json:"caption,omitempty"`
	Roles         []string         `json:"roles,omitempty"`
	Revision      int32            `json:"revision,omitempty"`
	BenchmarkData []BenchmarkPoint `json:"benchmark_data,omitempty"`
}

type BenchmarkPoint struct {
	Label    string  `json:"label"`
	Value    float64 `json:"value"`
	SourceID string  `json:"source_id"`
}

type GenerateRequest struct {
	ProjectID    uuid.UUID
	ArticleID    uuid.UUID
	AssetID      uuid.UUID
	Role, Prompt string
}

type GenerateResult struct {
	Bytes                     []byte
	MimeType, Provider, Model string
	Width, Height             int32
}

type Provider interface {
	Generate(context.Context, GenerateRequest) (GenerateResult, error)
}
type Store interface {
	Put(context.Context, string, []byte, string) (string, error)
}
type Budget interface {
	Allow(context.Context, uuid.UUID) error
}
